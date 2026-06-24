package usecase

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"sync"
	"testing"

	"github.com/arbuz/ai-arbuz-provider-api/internal/domain"
	"github.com/arbuz/ai-arbuz-provider-api/internal/ports"
)

// ---- fakes satisfying the ports the Proxy depends on ----
// Kept in one place so proxy tests stay focused on behavior, not plumbing.

type fakeProviderRepo struct{ p *domain.Provider }

func (f *fakeProviderRepo) Create(context.Context, *domain.Provider) error { return nil }
func (f *fakeProviderRepo) Update(context.Context, *domain.Provider) error { return nil }
func (f *fakeProviderRepo) Delete(context.Context, domain.ID) error         { return nil }
func (f *fakeProviderRepo) Get(_ context.Context, _ domain.ID) (*domain.Provider, error) {
	if f.p == nil {
		return nil, domain.ErrNotFound
	}
	return f.p, nil
}
func (f *fakeProviderRepo) List(context.Context) ([]domain.Provider, error) { return nil, nil }

type fakeUpstreamRepo struct {
	keys []domain.UpstreamKey
	hc   map[domain.ID]domain.UpstreamHealth
	mu   sync.Mutex
}

func (f *fakeUpstreamRepo) Create(context.Context, *domain.UpstreamKey) error { return nil }
func (f *fakeUpstreamRepo) Update(context.Context, *domain.UpstreamKey) error { return nil }
func (f *fakeUpstreamRepo) Delete(context.Context, domain.ID) error            { return nil }
func (f *fakeUpstreamRepo) Get(context.Context, domain.ID) (*domain.UpstreamKey, error) {
	return nil, domain.ErrNotFound
}
func (f *fakeUpstreamRepo) List(context.Context) ([]domain.UpstreamKey, error) { return f.keys, nil }
func (f *fakeUpstreamRepo) ListByProvider(context.Context, domain.ID) ([]domain.UpstreamKey, error) {
	return f.keys, nil
}
func (f *fakeUpstreamRepo) SetHealth(_ context.Context, id domain.ID, h domain.UpstreamHealth) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.hc == nil {
		f.hc = map[domain.ID]domain.UpstreamHealth{}
	}
	f.hc[id] = h
	return nil
}

type fakeIssuedRepo struct{ k *domain.IssuedKey }

func (f *fakeIssuedRepo) Create(context.Context, *domain.IssuedKey) error { return nil }
func (f *fakeIssuedRepo) Update(context.Context, *domain.IssuedKey) error { return nil }
func (f *fakeIssuedRepo) Delete(context.Context, domain.ID) error         { return nil }
func (f *fakeIssuedRepo) Get(context.Context, domain.ID) (*domain.IssuedKey, error) {
	return nil, domain.ErrNotFound
}
func (f *fakeIssuedRepo) GetByTokenHash(_ context.Context, h string) (*domain.IssuedKey, error) {
	if f.k == nil || hashToken("test-key") != h {
		return nil, domain.ErrNotFound
	}
	return f.k, nil
}
func (f *fakeIssuedRepo) List(context.Context) ([]domain.IssuedKey, error)            { return nil, nil }
func (f *fakeIssuedRepo) ListByProvider(context.Context, domain.ID) ([]domain.IssuedKey, error) {
	return nil, nil
}
func (f *fakeIssuedRepo) MarkUsed(context.Context, domain.ID, domain.Time) error { return nil }

type fakeSecretStore struct{}

func (fakeSecretStore) Encrypt(_ context.Context, p []byte) ([]byte, error) { return append([]byte("enc:"), p...), nil }
func (fakeSecretStore) Decrypt(_ context.Context, c []byte) ([]byte, error) {
	return bytes.TrimPrefix(c, []byte("enc:")), nil
}

// fakeLogRepo is shared with limiter_test.go (same package).

// fakeUpstreamClient returns canned responses and records the keys it called.
type fakeUpstreamClient struct {
	mu        sync.Mutex
	called    []string // base URLs called, in order
	respBody  []byte
	err       error // error to return (overrides respBody)
	failFirst bool  // if true, the FIRST call returns err, later calls succeed
	calledN   int
}

func (c *fakeUpstreamClient) Do(_ context.Context, baseURL, _ string, _ ports.UpstreamRequest) (*ports.UpstreamResponse, error) {
	c.mu.Lock()
	c.called = append(c.called, baseURL)
	c.calledN++
	n := c.calledN
	c.mu.Unlock()
	if c.failFirst && n == 1 {
		return nil, c.err
	}
	if !c.failFirst && c.err != nil {
		return nil, c.err
	}
	return &ports.UpstreamResponse{StatusCode: 200, Body: c.respBody, PromptTokens: 5, CompletionTokens: 7}, nil
}
func (c *fakeUpstreamClient) ListModels(context.Context, string, string, domain.Format) ([]string, error) {
	return nil, nil
}

// fakeConverter is an identity converter (same-format pass-through) for proxy
// tests where conversion isn't the subject.
type fakeConverter struct{}

func (fakeConverter) NeedsConversion(in, out domain.Format) bool { return in != out }
func (fakeConverter) ConvertRequest(_ context.Context, b []byte, _, _ domain.Format) ([]byte, error) {
	return b, nil
}
func (fakeConverter) ConvertResponse(_ context.Context, b []byte, _, _ domain.Format) ([]byte, error) {
	return b, nil
}
func (fakeConverter) ConvertStream(_ context.Context, r io.Reader, w io.Writer, _, _ domain.Format) (ports.Usage, error) {
	_, err := io.Copy(w, r)
	return ports.Usage{}, err
}

// buildTestProxy wires a Proxy with the given fakes.
func buildTestProxy(t *testing.T, issued *domain.IssuedKey, provider *domain.Provider,
	keys []domain.UpstreamKey, client *fakeUpstreamClient, logRepo *fakeLogRepo) *Proxy {
	t.Helper()
	return NewProxy(ProxyDeps{
		Issued:    &fakeIssuedRepo{k: issued},
		Providers: &fakeProviderRepo{p: provider},
		Upstreams: &fakeUpstreamRepo{keys: keys},
		Secrets:   fakeSecretStore{},
		Client:    client,
		Converter: fakeConverter{},
		Limiter:   NewRollingLimiter(logRepo),
		Selector:  NewFailoverSelector(nil),
		Cache:     nil,
		Logs:      logRepo,
	})
}

// ---- tests ----

// TestProxy_Handle_UnauthorizedOnBadToken — missing/unknown token → ErrUnauthorized.
func TestProxy_Handle_UnauthorizedOnBadToken(t *testing.T) {
	logRepo := &fakeLogRepo{}
	p := buildTestProxy(t, nil, nil, nil, &fakeUpstreamClient{}, logRepo)
	_, err := p.Handle(context.Background(), ProxyRequest{Token: "bad", Path: "/v1/chat/completions"})
	if !errors.Is(err, domain.ErrUnauthorized) {
		t.Fatalf("want ErrUnauthorized, got %v", err)
	}
}

// TestProxy_Handle_NoUpstreamKey — valid key, no usable upstream → ErrNoUpstreamKey.
func TestProxy_Handle_NoUpstreamKey(t *testing.T) {
	issued := &domain.IssuedKey{ID: "k", Status: domain.StatusActive}
	provider := &domain.Provider{ID: "p", Status: domain.StatusActive}
	logRepo := &fakeLogRepo{}
	p := buildTestProxy(t, issued, provider, nil, &fakeUpstreamClient{}, logRepo)
	_, err := p.Handle(context.Background(), ProxyRequest{
		Token: "test-key", Path: "/v1/chat/completions", Method: "POST", Body: []byte(`{"model":"m"}`),
	})
	if !errors.Is(err, domain.ErrNoUpstreamKey) {
		t.Fatalf("want ErrNoUpstreamKey, got %v", err)
	}
}

// TestProxy_Handle_SuccessAndFailover — first key fails transiently, second succeeds.
func TestProxy_Handle_FailoverOnTransient(t *testing.T) {
	issued := &domain.IssuedKey{ID: "k", Status: domain.StatusActive}
	provider := &domain.Provider{ID: "p", Status: domain.StatusActive}
	keys := []domain.UpstreamKey{
		{ID: "k1", ProviderID: "p", Format: domain.FormatOpenAI, BaseURL: "http://k1", Status: domain.StatusActive, Priority: 0},
		{ID: "k2", ProviderID: "p", Format: domain.FormatOpenAI, BaseURL: "http://k2", Status: domain.StatusActive, Priority: 1},
	}
	// First call transient, subsequent success.
	client := &fakeUpstreamClient{respBody: []byte(`{"ok":true}`), err: domain.ErrUpstreamUnavailable, failFirst: true}
	logRepo := &fakeLogRepo{}
	p := buildTestProxy(t, issued, provider, keys, client, logRepo)
	res, err := p.Handle(context.Background(), ProxyRequest{
		Token: "test-key", Path: "/v1/chat/completions", Method: "POST", InFormat: domain.FormatOpenAI,
		Body: []byte(`{"model":"m","messages":[]}`),
	})
	if err != nil {
		t.Fatalf("expected success after failover, got %v", err)
	}
	if res.OnDone != nil {
		res.OnDone(nil)
	}
	if client.calledN < 2 {
		t.Errorf("expected failover to try a 2nd key, calls=%d", client.calledN)
	}
}

// TestProxy_Handle_NoFailoverOnClientError — a 400-class upstream error must NOT
// try the next key (audit blocker #1 regression guard).
func TestProxy_Handle_NoFailoverOnClientError(t *testing.T) {
	issued := &domain.IssuedKey{ID: "k", Status: domain.StatusActive}
	provider := &domain.Provider{ID: "p", Status: domain.StatusActive}
	keys := []domain.UpstreamKey{
		{ID: "k1", Format: domain.FormatOpenAI, BaseURL: "http://k1", Status: domain.StatusActive, Priority: 0},
		{ID: "k2", Format: domain.FormatOpenAI, BaseURL: "http://k2", Status: domain.StatusActive, Priority: 1},
	}
	client := &fakeUpstreamClient{err: domain.ErrUpstreamClientError}
	logRepo := &fakeLogRepo{}
	p := buildTestProxy(t, issued, provider, keys, client, logRepo)
	_, err := p.Handle(context.Background(), ProxyRequest{
		Token: "test-key", Path: "/v1/chat/completions", Method: "POST", InFormat: domain.FormatOpenAI,
		Body: []byte(`{"model":"m","messages":[]}`),
	})
	if !errors.Is(err, domain.ErrUpstreamClientError) {
		t.Fatalf("want ErrUpstreamClientError surfaced, got %v", err)
	}
	// Only ONE key should have been attempted (the second must NOT be retried).
	client.mu.Lock()
	defer client.mu.Unlock()
	if len(client.called) > 1 {
		t.Errorf("client error must not failover; called %d keys: %v", len(client.called), client.called)
	}
}

// TestProxy_Handle_LimitExceededBeforeUpstream — issued limit blocks the call
// before the upstream is touched (§4.4). Regression guard for blocker #2/#3.
// Cap = 1 request/5h, pre-existing usage already = 1 → the next request trips.
func TestProxy_Handle_LimitExceededBeforeUpstream(t *testing.T) {
	issued := &domain.IssuedKey{
		ID: "k", Status: domain.StatusActive,
		Limits: domain.Limits{Requests: map[domain.LimitWindow]int64{domain.Window5h: 1}},
	}
	provider := &domain.Provider{ID: "p", Status: domain.StatusActive}
	keys := []domain.UpstreamKey{{ID: "k1", Format: domain.FormatOpenAI, Status: domain.StatusActive}}
	client := &fakeUpstreamClient{}
	logRepo := &fakeLogRepo{usage: ports.WindowUsage{Requests: 1}} // already used the cap
	p := buildTestProxy(t, issued, provider, keys, client, logRepo)
	_, err := p.Handle(context.Background(), ProxyRequest{
		Token: "test-key", Path: "/v1/chat/completions", Method: "POST",
		Body: []byte(`{"model":"m","messages":[]}`),
	})
	if !errors.Is(err, domain.ErrLimitExceeded) {
		t.Fatalf("want ErrLimitExceeded, got %v", err)
	}
	if len(client.called) != 0 {
		t.Errorf("upstream must NOT be called when limit exceeded; called %v", client.called)
	}
}

// TestProxy_Handle_ModelsAggregatedFromProvider — GET /v1/models returns the
// merged provider+upstream model list without an upstream call.
func TestProxy_Handle_ModelsAggregatedFromProvider(t *testing.T) {
	issued := &domain.IssuedKey{ID: "k", Status: domain.StatusActive}
	provider := &domain.Provider{ID: "p", Status: domain.StatusActive, GlobalModels: []string{"g1"}}
	keys := []domain.UpstreamKey{{ID: "k1", Status: domain.StatusActive, Models: []string{"g1", "u1"}}}
	client := &fakeUpstreamClient{}
	logRepo := &fakeLogRepo{}
	p := buildTestProxy(t, issued, provider, keys, client, logRepo)
	res, err := p.Handle(context.Background(), ProxyRequest{Token: "test-key", IsModels: true})
	if err != nil {
		t.Fatal(err)
	}
	var list struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if json.Unmarshal(res.Body, &list) != nil {
		t.Fatal("invalid models body")
	}
	got := map[string]bool{}
	for _, m := range list.Data {
		got[m.ID] = true
	}
	if !got["g1"] || !got["u1"] {
		t.Errorf("models should merge global+upstream deduped, got %v", got)
	}
	if len(client.called) != 0 {
		t.Error("/v1/models must not call upstream")
	}
}
