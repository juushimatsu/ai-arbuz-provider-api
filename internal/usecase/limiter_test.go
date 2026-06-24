package usecase

import (
	"context"
	"testing"
	"time"

	"github.com/arbuz/ai-arbuz-provider-api/internal/domain"
	"github.com/arbuz/ai-arbuz-provider-api/internal/ports"
)

// fakeLogRepo is a minimal ports.LogRepo for limiter/selector/proxy tests.
type fakeLogRepo struct {
	usage   ports.WindowUsage
	inserts []*domain.RequestLog
}

func (f *fakeLogRepo) Insert(_ context.Context, l *domain.RequestLog) error {
	f.inserts = append(f.inserts, l)
	return nil
}
func (f *fakeLogRepo) Get(context.Context, domain.ID) (*domain.RequestLog, error) {
	return nil, domain.ErrNotFound
}
func (f *fakeLogRepo) List(context.Context, ports.LogFilter) ([]domain.RequestLog, error) {
	return nil, nil
}
func (f *fakeLogRepo) UsageSum(context.Context, domain.ID, domain.Time) (ports.WindowUsage, error) {
	return f.usage, nil
}

// TestRollingLimiter_Boundary — the limit fires exactly at the window boundary.
func TestRollingLimiter_Boundary(t *testing.T) {
	// Pre-existing usage: 99 tokens. Cap: 100. Projected: 1 → OK. Projected: 2 → block.
	repo := &fakeLogRepo{usage: ports.WindowUsage{Tokens: 99, Requests: 0}}
	key := &domain.IssuedKey{
		ID:     "k1",
		Limits: domain.Limits{Tokens: map[domain.LimitWindow]int64{domain.Window24h: 100}},
	}
	l := NewRollingLimiter(repo)
	if err := l.Check(context.Background(), key, 1); err != nil {
		t.Fatalf("within cap should pass, got %v", err)
	}
	if err := l.Check(context.Background(), key, 2); err == nil {
		t.Fatal("over cap should fail")
	}

	// Requests cap boundary.
	repo.usage = ports.WindowUsage{Tokens: 0, Requests: 5}
	key.Limits = domain.Limits{Requests: map[domain.LimitWindow]int64{domain.Window5h: 5}}
	if err := l.Check(context.Background(), key, 0); err == nil {
		t.Fatal("6th request over cap of 5 should fail")
	}

	// No cap configured → always allow.
	repo.usage = ports.WindowUsage{Tokens: 1 << 30, Requests: 1 << 30}
	key.Limits = domain.NewLimits()
	if err := l.Check(context.Background(), key, 9999); err != nil {
		t.Fatalf("no cap should allow, got %v", err)
	}
}

// TestFailoverSelector_CooldownSkip — a key in cooldown is skipped.
func TestFailoverSelector_CooldownSkip(t *testing.T) {
	now := time.Date(2026, 6, 19, 12, 0, 0, 0, time.UTC)
	good := domain.UpstreamKey{ID: "g", Status: domain.StatusActive, Priority: 1}
	cooling := domain.UpstreamKey{
		ID: "c", Status: domain.StatusActive, Priority: 0,
		Health: domain.UpstreamHealth{CooldownUntil: now.Add(30 * time.Second)},
	}
	disabled := domain.UpstreamKey{ID: "d", Status: domain.StatusDisabled, Priority: 2}

	sel := NewFailoverSelector(nil)
	got, err := sel.Select(context.Background(), &domain.Provider{}, []domain.UpstreamKey{cooling, good, disabled}, "", now)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].ID != "g" {
		t.Fatalf("only the good key should be selected, got %+v", got)
	}
}

// TestFailoverSelector_ModelFilter — a key with an explicit model list that
// excludes the requested model is skipped.
func TestFailoverSelector_ModelFilter(t *testing.T) {
	now := time.Now().UTC()
	k1 := domain.UpstreamKey{ID: "1", Status: domain.StatusActive, Models: []string{"gpt-4"}}
	k2 := domain.UpstreamKey{ID: "2", Status: domain.StatusActive, Models: []string{"claude"}}
	sel := NewFailoverSelector(nil)
	got, _ := sel.Select(context.Background(), &domain.Provider{}, []domain.UpstreamKey{k1, k2}, "claude", now)
	if len(got) != 1 || got[0].ID != "2" {
		t.Fatalf("model filter should select only claude key, got %+v", got)
	}
}

// TestRetry_TransientOnly — retry only retries ErrUpstreamUnavailable.
func TestRetry_TransientOnly(t *testing.T) {
	calls := 0
	err := retry(context.Background(), 3, time.Millisecond, func() error {
		calls++
		return domain.ErrUpstreamUnavailable
	})
	if err == nil || calls != 4 {
		t.Fatalf("should retry 4× then return err, calls=%d", calls)
	}
	// Non-transient: returns immediately, 1 call.
	calls = 0
	err = retry(context.Background(), 3, time.Millisecond, func() error {
		calls++
		return domain.ErrKeyRevoked
	})
	if calls != 1 {
		t.Fatalf("non-transient should not retry, calls=%d", calls)
	}
	// Success on 2nd attempt.
	calls = 0
	_ = retry(context.Background(), 3, time.Millisecond, func() error {
		calls++
		if calls < 2 {
			return domain.ErrUpstreamUnavailable
		}
		return nil
	})
	if calls != 2 {
		t.Fatalf("expected 2 calls, got %d", calls)
	}
}