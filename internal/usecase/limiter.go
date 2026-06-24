package usecase

import (
	"context"
	"fmt"
	"time"

	"github.com/arbuz/ai-arbuz-provider-api/internal/domain"
	"github.com/arbuz/ai-arbuz-provider-api/internal/ports"
)

// FailMode controls limiter behavior when the usage read itself errors.
//
// We DEFAULT to fail-CLOSED: a stats/DB hiccup must NOT silently disable the
// quota — that would risk overspending the owner's real upstream money
// (AGENTS.md: "Not lazy about: error handling that prevents data loss").
// Set ARBUZ_LIMIT_FAIL=open for a permissive mode (e.g. noisy dev DBs).
type FailMode string

const (
	FailClosed FailMode = "closed"
	FailOpen   FailMode = "open"
)

// RollingLimiter enforces issued-key limits over rolling windows (§4.3, §2).
//
// Two INDEPENDENT cap dimensions, never mixed:
//   - tokens   (per window 5h / 24h / 30d)
//   - requests (per window 5h / 24h / 30d)
//
// Consumption is read from the log repo (UsageSum per window). Issued limits
// are checked BEFORE any upstream call; upstream limits are NOT this type's
// concern — those are a failover signal handled by the selector.
//
// ponytail: ceiling — each Check runs one SUM query per configured window.
// O(windows) DB hits per request (~3). Growth path = bucketed counters /
// Redis so the limiter doesn't touch the logs at all.
type RollingLimiter struct {
	logs ports.LogRepo
	now  func() time.Time
	fail FailMode
}

func NewRollingLimiter(logs ports.LogRepo) *RollingLimiter {
	return &RollingLimiter{logs: logs, now: time.Now, fail: FailClosed}
}

// WithFailMode sets the failure mode (open/closed). Builder-style; used by DI.
func (l *RollingLimiter) WithFailMode(m FailMode) *RollingLimiter {
	if m == FailOpen {
		l.fail = FailOpen
	}
	return l
}

// Check returns a domain.ErrLimitExceeded-wrapped error if adding (1 request,
// projectedTokens) would breach any configured cap. projectedTokens is a
// pre-call estimate; actual usage is recorded post-call via the log repo.
//
// On a usage-read error: fail-CLOSED returns ErrLimitExceeded (default),
// fail-OPEN skips that window. See FailMode doc for the rationale.
func (l *RollingLimiter) Check(ctx context.Context, key *domain.IssuedKey, projectedTokens int64) error {
	if key == nil {
		return nil
	}
	now := l.now().UTC()
	for _, w := range domain.AllWindows() {
		dur := time.Duration(domain.WindowDuration(w)) * time.Second
		since := now.Add(-dur)
		usage, err := l.logs.UsageSum(ctx, key.ID, since)
		if err != nil {
			if l.fail == FailClosed {
				// Refuse rather than risk unaccounted-for usage.
				return fmt.Errorf("%w: usage check unavailable (%s)", domain.ErrLimitExceeded, w)
			}
			// fail-open: skip this window, keep checking others.
			continue
		}
		if cap, ok := key.Limits.Tokens[w]; ok && cap > 0 {
			if usage.Tokens+projectedTokens > cap {
				return fmt.Errorf("%w: tokens %s", domain.ErrLimitExceeded, w)
			}
		}
		if cap, ok := key.Limits.Requests[w]; ok && cap > 0 {
			if usage.Requests+1 > cap {
				return fmt.Errorf("%w: requests %s", domain.ErrLimitExceeded, w)
			}
		}
	}
	return nil
}

// Record is a no-op: consumption is persisted by the proxy via logs.Insert,
// which is the source of truth for UsageSum. Kept to satisfy ports.Limiter.
func (l *RollingLimiter) Record(_ context.Context, _ *domain.RequestLog) error { return nil }
