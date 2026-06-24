package usecase

import (
	"context"
	"errors"
	"time"

	"github.com/arbuz/ai-arbuz-provider-api/internal/domain"
	"github.com/arbuz/ai-arbuz-provider-api/internal/ports"
)

// FailoverSelector implements ports.KeySelector (§4.4).
//
// Selection: active keys, not in cooldown, in ascending Priority order.
// A model filter narrows to keys whose effective model list includes the
// requested model (or has an empty list = "accepts anything").
//
// Health escalation on failure: consecutive-failure counter; on threshold,
// the key is sidelined for a cooldown window. On success the counter resets.
// ponytail: ceiling — fixed cooldown ladder; no circuit-breaker half-open
// probing. Growth path = a real breaker with probe traffic.
type FailoverSelector struct {
	repo             ports.UpstreamRepo
	cooldown         time.Duration
	failThreshold    int
	fallbackStrategy fallbackStrategy
	now              func() time.Time
}

type fallbackStrategy int

const (
	noFallback fallbackStrategy = iota
)

// NewFailoverSelector builds a selector backed by a health repo.
// nil repo is allowed for tests (health becomes in-memory only).
func NewFailoverSelector(repo ports.UpstreamRepo) *FailoverSelector {
	return &FailoverSelector{
		repo: repo, cooldown: 60 * time.Second, failThreshold: 3,
		now: time.Now,
	}
}

func (s *FailoverSelector) Select(_ context.Context, provider *domain.Provider, keys []domain.UpstreamKey, model string, now domain.Time) ([]domain.UpstreamKey, error) {
	out := make([]domain.UpstreamKey, 0, len(keys))
	for _, k := range keys {
		if !k.Usable(now) {
			continue
		}
		// Model filter: skip if the key has an explicit list that excludes it.
		if model != "" {
			effective := k.EffectiveModels(provider.GlobalModels)
			if len(effective) > 0 && !contains(effective, model) {
				// ponytail: try fallback models before giving up — done by the
				// proxy which knows provider.FallbackModels; here we just skip.
				continue
			}
		}
		out = append(out, k)
	}
	return out, nil
}

func (s *FailoverSelector) OnFailure(ctx context.Context, key *domain.UpstreamKey, reason error) error {
	if key == nil {
		return nil
	}
	key.Health.ConsecutiveFailures++
	// Escalate cooldown on repeated failures (transient errors matter more).
	if errors.Is(reason, domain.ErrUpstreamUnavailable) && key.Health.ConsecutiveFailures >= s.failThreshold {
		key.Health.CooldownUntil = s.now().UTC().Add(s.cooldown)
		key.Health.ConsecutiveFailures = 0
	}
	if s.repo != nil {
		return s.repo.SetHealth(ctx, key.ID, key.Health)
	}
	return nil
}

func (s *FailoverSelector) OnSuccess(ctx context.Context, key *domain.UpstreamKey) error {
	if key == nil {
		return nil
	}
	key.Health.ConsecutiveFailures = 0
	key.Health.CooldownUntil = time.Time{}
	if s.repo != nil {
		return s.repo.SetHealth(ctx, key.ID, key.Health)
	}
	return nil
}

func contains(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}
