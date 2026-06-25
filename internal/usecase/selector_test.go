package usecase

import (
	"context"
	"testing"
	"time"

	"github.com/arbuz/ai-arbuz-provider-api/internal/domain"
)

// TestSelector_RoundRobin_RotatesAndKeepsFailoverOrder verifies the round_robin
// strategy starts each request on the next key (load distribution) while still
// returning the remaining keys as ordered fallbacks (switch-on-no-response).
func TestSelector_RoundRobin_RotatesAndKeepsFailoverOrder(t *testing.T) {
	s := NewFailoverSelector(nil)
	now := time.Now()
	provider := &domain.Provider{ID: "p", Status: domain.StatusActive, Strategy: domain.StrategyRoundRobin}
	keys := []domain.UpstreamKey{
		{ID: "k1", Status: domain.StatusActive, Priority: 0},
		{ID: "k2", Status: domain.StatusActive, Priority: 1},
		{ID: "k3", Status: domain.StatusActive, Priority: 2},
	}
	wantFirst := []string{"k1", "k2", "k3", "k1"}
	for i, want := range wantFirst {
		out, err := s.Select(context.Background(), provider, keys, "m", now)
		if err != nil {
			t.Fatalf("call %d: %v", i, err)
		}
		if len(out) != 3 {
			t.Fatalf("call %d: want 3 keys, got %d", i, len(out))
		}
		if out[0].ID != want {
			t.Errorf("call %d: want first key %s, got %s", i, want, out[0].ID)
		}
		// All three keys must still be present (fallbacks preserved).
		seen := map[string]bool{out[0].ID: true, out[1].ID: true, out[2].ID: true}
		if !seen["k1"] || !seen["k2"] || !seen["k3"] {
			t.Errorf("call %d: lost a fallback key: %v", i, out)
		}
	}
}

// TestSelector_Failover_StableOrder verifies the default failover strategy does
// NOT rotate: every call returns keys in the same priority order.
func TestSelector_Failover_StableOrder(t *testing.T) {
	s := NewFailoverSelector(nil)
	now := time.Now()
	provider := &domain.Provider{ID: "p", Status: domain.StatusActive, Strategy: domain.StrategyFailover}
	keys := []domain.UpstreamKey{
		{ID: "k1", Status: domain.StatusActive, Priority: 0},
		{ID: "k2", Status: domain.StatusActive, Priority: 1},
	}
	for i := 0; i < 3; i++ {
		out, err := s.Select(context.Background(), provider, keys, "m", now)
		if err != nil {
			t.Fatal(err)
		}
		if out[0].ID != "k1" {
			t.Errorf("call %d: failover must stay on k1 first, got %s", i, out[0].ID)
		}
	}
}
