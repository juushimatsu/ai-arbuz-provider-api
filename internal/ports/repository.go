// Package ports defines narrow outbound interfaces (ISP/DIP).
// Use-cases depend only on these; adapters implement them.
package ports

import (
	"context"

	"github.com/arbuz/ai-arbuz-provider-api/internal/domain"
)

// Repositories — persistence ports, one per aggregate.

type ProviderRepo interface {
	Create(ctx context.Context, p *domain.Provider) error
	Update(ctx context.Context, p *domain.Provider) error
	Delete(ctx context.Context, id domain.ID) error
	Get(ctx context.Context, id domain.ID) (*domain.Provider, error)
	List(ctx context.Context) ([]domain.Provider, error)
}

type UpstreamRepo interface {
	Create(ctx context.Context, k *domain.UpstreamKey) error
	Update(ctx context.Context, k *domain.UpstreamKey) error
	Delete(ctx context.Context, id domain.ID) error
	Get(ctx context.Context, id domain.ID) (*domain.UpstreamKey, error)
	List(ctx context.Context) ([]domain.UpstreamKey, error)
	ListByProvider(ctx context.Context, providerID domain.ID) ([]domain.UpstreamKey, error)
	// SetHealth persists cooldown / failure counters.
	SetHealth(ctx context.Context, id domain.ID, h domain.UpstreamHealth) error
}

type IssuedRepo interface {
	Create(ctx context.Context, k *domain.IssuedKey) error
	Update(ctx context.Context, k *domain.IssuedKey) error
	Delete(ctx context.Context, id domain.ID) error
	Get(ctx context.Context, id domain.ID) (*domain.IssuedKey, error)
	GetByTokenHash(ctx context.Context, hash string) (*domain.IssuedKey, error)
	List(ctx context.Context) ([]domain.IssuedKey, error)
	ListByProvider(ctx context.Context, providerID domain.ID) ([]domain.IssuedKey, error)
	MarkUsed(ctx context.Context, id domain.ID, at domain.Time) error
}

type LogRepo interface {
	Insert(ctx context.Context, l *domain.RequestLog) error
	Get(ctx context.Context, id domain.ID) (*domain.RequestLog, error)
	// List returns logs filtered by IssuedKeyID (empty = all), with limit.
	List(ctx context.Context, filter LogFilter) ([]domain.RequestLog, error)
	// UsageSum returns token & request counts since `since` for an issued key.
	// Used by the limiter for rolling-window checks.
	UsageSum(ctx context.Context, issuedKeyID domain.ID, since domain.Time) (WindowUsage, error)
}

// LogFilter — query parameters for log listing.
type LogFilter struct {
	IssuedKeyID domain.ID
	ProviderID  domain.ID
	Success     *bool
	Model       string
	Limit       int
	Offset      int
	From        domain.Time
	To          domain.Time
}

// WindowUsage — aggregated consumption over a rolling window (tokens + requests).
type WindowUsage struct {
	Tokens   int64
	Requests int64
}

type UserRepo interface {
	Get(ctx context.Context) (*domain.User, error)         // single user
	Update(ctx context.Context, u *domain.User) error
	// Seed inserts the initial admin user if none exists; no-op otherwise.
	Seed(ctx context.Context, login, passwordHash string) error
}

type SessionRepo interface {
	Create(ctx context.Context, s *domain.Session) error
	Get(ctx context.Context, token string) (*domain.Session, error)
	Delete(ctx context.Context, token string) error
}

type MCPRepo interface {
	Create(ctx context.Context, m *domain.MCPServer) error
	Update(ctx context.Context, m *domain.MCPServer) error
	Delete(ctx context.Context, id domain.ID) error
	Get(ctx context.Context, id domain.ID) (*domain.MCPServer, error)
	List(ctx context.Context) ([]domain.MCPServer, error)
}

type CheckerRepo interface {
	Insert(ctx context.Context, r *domain.CheckerRun) error
	Get(ctx context.Context, id domain.ID) (*domain.CheckerRun, error)
	List(ctx context.Context, limit int) ([]domain.CheckerRun, error)
}

// PromptRuleRepo persists prompt-transformation rules (§4.7).
// ModelPrefRepo persists per-provider "model -> preferred upstream key"
// mappings. Rows are removed automatically (ON DELETE CASCADE) when either the
// provider or the referenced upstream key is deleted.
type ModelPrefRepo interface {
	// ListByProvider returns all preferences for a provider.
	ListByProvider(ctx context.Context, providerID domain.ID) ([]domain.ModelPreference, error)
	// Set upserts the preferred key for (provider, model).
	Set(ctx context.Context, pref domain.ModelPreference) error
	// Delete removes the preference for (provider, model).
	Delete(ctx context.Context, providerID domain.ID, model string) error
}

type PromptRuleRepo interface {
	Create(ctx context.Context, r *domain.PromptRule) error
	Update(ctx context.Context, r *domain.PromptRule) error
	Delete(ctx context.Context, id domain.ID) error
	Get(ctx context.Context, id domain.ID) (*domain.PromptRule, error)
	List(ctx context.Context) ([]domain.PromptRule, error)
}