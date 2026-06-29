package usecase

import (
	"context"
	"testing"

	"github.com/arbuz/ai-arbuz-provider-api/internal/domain"
)

type fakePrefRepo struct{ prefs []domain.ModelPreference }

func (f fakePrefRepo) ListByProvider(_ context.Context, _ domain.ID) ([]domain.ModelPreference, error) {
	return f.prefs, nil
}
func (f fakePrefRepo) Set(_ context.Context, _ domain.ModelPreference) error    { return nil }
func (f fakePrefRepo) Delete(_ context.Context, _ domain.ID, _ string) error    { return nil }

func TestBubblePreferred(t *testing.T) {
	keys := []domain.UpstreamKey{{ID: "k1"}, {ID: "k2"}, {ID: "k3"}}

	// Preferred k3 -> bubbles to front, rest keep order.
	p := &Proxy{modelPrefs: fakePrefRepo{prefs: []domain.ModelPreference{{Model: "m", UpstreamKeyID: "k3"}}}}
	out := p.bubblePreferred(context.Background(), "prov", "m", keys)
	if out[0].ID != "k3" || out[1].ID != "k1" || out[2].ID != "k2" {
		t.Fatalf("expected k3,k1,k2 got %v", ids(out))
	}

	// Preferred key not usable (absent from ordered) -> unchanged (failover).
	p2 := &Proxy{modelPrefs: fakePrefRepo{prefs: []domain.ModelPreference{{Model: "m", UpstreamKeyID: "gone"}}}}
	out2 := p2.bubblePreferred(context.Background(), "prov", "m", keys)
	if out2[0].ID != "k1" {
		t.Fatalf("expected unchanged k1 first got %v", ids(out2))
	}

	// No preference for this model -> unchanged.
	out3 := p.bubblePreferred(context.Background(), "prov", "other", keys)
	if out3[0].ID != "k1" {
		t.Fatalf("expected unchanged got %v", ids(out3))
	}

	// Nil repo -> unchanged.
	p4 := &Proxy{}
	if got := p4.bubblePreferred(context.Background(), "prov", "m", keys); got[0].ID != "k1" {
		t.Fatalf("nil repo should be no-op")
	}
}

func ids(ks []domain.UpstreamKey) []string {
	out := make([]string, len(ks))
	for i, k := range ks {
		out[i] = k.ID
	}
	return out
}
