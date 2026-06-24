package domain

// Provider — logical group of one or more upstream keys (§2, §4.2).
// JSON tags are snake_case to match the SPA (it reads id / name / strategy /
// global_models / fallback_models / status). Without them Go emits PascalCase
// and the UI shows blank provider names and empty dropdowns.
type Provider struct {
	ID             ID              `json:"id"`
	Name           string          `json:"name"`
	Strategy       RoutingStrategy `json:"strategy"` // default: failover
	GlobalModels   []string        `json:"global_models"`   // applied to keys using global models
	FallbackModels []string        `json:"fallback_models"` // used when requested model unavailable
	Status         Status          `json:"status"`
	CreatedAt      Time            `json:"created_at"`
	UpdatedAt      Time            `json:"updated_at"`
}
