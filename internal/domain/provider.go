package domain

// Provider — logical group of one or more upstream keys (§2, §4.2).
type Provider struct {
	ID              ID
	Name            string
	Strategy        RoutingStrategy // default: failover
	GlobalModels    []string        // applied to keys using global models
	FallbackModels  []string        // used when requested model unavailable
	Status          Status
	CreatedAt       Time
	UpdatedAt       Time
}
