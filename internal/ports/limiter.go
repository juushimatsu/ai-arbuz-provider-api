package ports

import (
	"context"

	"github.com/arbuz/ai-arbuz-provider-api/internal/domain"
)

// Limiter enforces ISSUED-key limits (§2). Checks happen BEFORE upstream call.
// Upstream limits are handled separately by the Selector (failover signal).
type Limiter interface {
	// Check returns nil if the request (1 request, projected tokens) is within
	// all configured rolling windows for the issued key; ErrLimitExceeded otherwise.
	Check(ctx context.Context, key *domain.IssuedKey, projectedTokens int64) error
	// Record commits actual usage after a completed request.
	Record(ctx context.Context, log *domain.RequestLog) error
}

// KeySelector picks the next usable upstream key within a provider (§4.4).
type KeySelector interface {
	// Select returns keys in failover order; the caller tries them in sequence.
	Select(ctx context.Context, provider *domain.Provider, keys []domain.UpstreamKey, model string, now domain.Time) ([]domain.UpstreamKey, error)
	// OnFailure updates health/cooldown after an upstream failure.
	OnFailure(ctx context.Context, key *domain.UpstreamKey, reason error) error
	// OnSuccess resets failure counters after a success.
	OnSuccess(ctx context.Context, key *domain.UpstreamKey) error
}

// Cache — exact-match response cache (§4.7). Interface leaves room for Redis.
type Cache interface {
	Get(ctx context.Context, key string) ([]byte, bool)
	Set(ctx context.Context, key string, value []byte)
}

// Stats serves aggregated statistics for the dashboard (§4.11).
type Stats interface {
	Summary(ctx context.Context, q StatsQuery) (StatsSummary, error)
	Series(ctx context.Context, q StatsQuery) ([]StatsPoint, error)
	Breakdown(ctx context.Context, q StatsQuery, dimension string) ([]StatsBucket, error)
}

type StatsQuery struct {
	From domain.Time
	To   domain.Time
	BucketSeconds int64 // for Series granularity
}

type StatsSummary struct {
	TotalRequests    int64   `json:"total_requests"`
	SuccessCount     int64   `json:"success_count"`
	ErrorCount       int64   `json:"error_count"`
	ErrorRate        float64 `json:"error_rate"`
	PromptTokens     int64   `json:"prompt_tokens"`
	CompletionTokens int64   `json:"completion_tokens"`
	AvgTTFBMs        float64 `json:"avg_ttfb_ms"`
}

type StatsPoint struct {
	TS     domain.Time `json:"ts"`
	Count  int64       `json:"count"`
	Errors int64       `json:"errors"`
	Tokens int64       `json:"tokens"`
	TTFBMs float64     `json:"ttfb_ms"`
}

type StatsBucket struct {
	Label  string `json:"label"`
	Count  int64  `json:"count"`
	Tokens int64  `json:"tokens"`
}