package sqlite

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/arbuz/ai-arbuz-provider-api/internal/domain"
)

// newTestDB opens a fresh in-memory-ish SQLite file for one test and returns
// all the repos wired to it. ponytail: one file per test (t.TempDir) — no shared
// state, no fixtures framework.
func newTestDB(t *testing.T) (*providerRepoCtx, context.Context) {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "test.db")
	db, err := Open(dbPath)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return &providerRepoCtx{
		db:        db,
		providers: NewProviderRepo(db),
		upstreams: NewUpstreamRepo(db),
		issued:    NewIssuedRepo(db),
		logs:      NewLogRepo(db),
	}, context.Background()
}

type providerRepoCtx struct {
	db        any
	providers *ProviderRepo
	upstreams *UpstreamRepo
	issued    *IssuedRepo
	logs      *LogRepo
}

// Round-trip the core aggregates through their repos to catch schema/mapping
// regressions (audit §3.4). Each subtest is independent and uses its own DB.
func TestRepos_ProviderRoundTrip(t *testing.T) {
	c, ctx := newTestDB(t)
	p := &domain.Provider{Name: "p1", Strategy: domain.StrategyFailover, GlobalModels: []string{"m1", "m2"}, Status: domain.StatusActive}
	if err := c.providers.Create(ctx, p); err != nil {
		t.Fatal(err)
	}
	got, err := c.providers.Get(ctx, p.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Name != "p1" || len(got.GlobalModels) != 2 || got.GlobalModels[0] != "m1" {
		t.Errorf("round-trip mismatch: %+v", got)
	}
	list, _ := c.providers.List(ctx)
	if len(list) != 1 {
		t.Errorf("List want 1, got %d", len(list))
	}
}

func TestRepos_UpstreamRoundTrip(t *testing.T) {
	c, ctx := newTestDB(t)
	// need a provider first (FK)
	p := &domain.Provider{Name: "p", Status: domain.StatusActive}
	_ = c.providers.Create(ctx, p)

	k := &domain.UpstreamKey{
		ProviderID: p.ID, Name: "k", BaseURL: "https://x", Format: domain.FormatOpenAI,
		SecretEnc: []byte("encbytes"), Models: []string{"m"}, Priority: 2,
		Status: domain.StatusActive,
		UpstreamLimits: domain.Limits{
			Tokens:   map[domain.LimitWindow]int64{domain.Window24h: 1000},
			Requests: map[domain.LimitWindow]int64{domain.Window5h: 50},
		},
	}
	if err := c.upstreams.Create(ctx, k); err != nil {
		t.Fatal(err)
	}
	got, err := c.upstreams.Get(ctx, k.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Name != "k" || got.Format != domain.FormatOpenAI || string(got.SecretEnc) != "encbytes" {
		t.Errorf("round-trip mismatch: %+v", got)
	}
	if got.UpstreamLimits.Tokens[domain.Window24h] != 1000 || got.UpstreamLimits.Requests[domain.Window5h] != 50 {
		t.Errorf("upstream limits lost: %+v", got.UpstreamLimits)
	}
	byProv, _ := c.upstreams.ListByProvider(ctx, p.ID)
	if len(byProv) != 1 {
		t.Errorf("ListByProvider want 1, got %d", len(byProv))
	}
}

func TestRepos_IssuedRoundTripAndTokenHash(t *testing.T) {
	c, ctx := newTestDB(t)
	p := &domain.Provider{Name: "p", Status: domain.StatusActive}
	_ = c.providers.Create(ctx, p)

	k := &domain.IssuedKey{
		ProviderID: p.ID, Name: "issued", Prefix: "sk-arbuz", TokenHash: "abc123",
		Limits: domain.Limits{Tokens: map[domain.LimitWindow]int64{domain.Window30d: 100000}},
		ValidDays: 14, Status: domain.StatusActive,
	}
	if err := c.issued.Create(ctx, k); err != nil {
		t.Fatal(err)
	}
	// Token-hash lookup (the hot path used by the proxy).
	got, err := c.issued.GetByTokenHash(ctx, "abc123")
	if err != nil {
		t.Fatalf("GetByTokenHash: %v", err)
	}
	if got.Name != "issued" || got.Limits.Tokens[domain.Window30d] != 100000 {
		t.Errorf("round-trip mismatch: %+v", got)
	}
	if got.ExpiresAt.IsZero() {
		t.Error("ExpiresAt should be computed from ValidDays")
	}
}

func TestRepos_LogInsertAndUsageSum(t *testing.T) {
	c, ctx := newTestDB(t)
	// UsageSum sums total_tokens over a window for an issued key (limiter input).
	now := time.Now().UTC()
	logs := []*domain.RequestLog{
		{IssuedKeyID: "k1", ProviderID: "p", InFormat: "openai", OutFormat: "openai", Success: true, TotalTokens: 100, Timestamp: now},
		{IssuedKeyID: "k1", ProviderID: "p", InFormat: "openai", OutFormat: "openai", Success: true, TotalTokens: 50, Timestamp: now},
		// a failed log must NOT count toward usage (UsageSum filters success=1).
		{IssuedKeyID: "k1", ProviderID: "p", InFormat: "openai", OutFormat: "openai", Success: false, TotalTokens: 999, Timestamp: now},
	}
	for _, l := range logs {
		if err := c.logs.Insert(ctx, l); err != nil {
			t.Fatal(err)
		}
	}
	usage, err := c.logs.UsageSum(ctx, "k1", now.Add(-time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if usage.Tokens != 150 {
		t.Errorf("UsageSum tokens = %d, want 150 (failed excluded)", usage.Tokens)
	}
	if usage.Requests != 2 {
		t.Errorf("UsageSum requests = %d, want 2", usage.Requests)
	}
}
