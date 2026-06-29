package domain

import "time"

// Format identifies an API wire format for auto-detection and conversion.
type Format string

const (
	FormatOpenAI    Format = "openai"    // OpenAI / OpenAI-compatible
	FormatAnthropic Format = "anthropic" // native Anthropic
)

// Status is the common enable/disable flag.
type Status string

const (
	StatusActive   Status = "active"
	StatusDisabled Status = "disabled"
	// StatusPaused — issued key temporarily disabled by the admin; can be
	// resumed. Distinct from StatusDisabled (revocation, terminal).
	StatusPaused Status = "paused"
)

// RoutingStrategy selects how keys are picked inside a Provider.
type RoutingStrategy string

const (
	StrategyFailover   RoutingStrategy = "failover"    // ordered by Priority, switch on error
	StrategyRoundRobin RoutingStrategy = "round_robin" // распределяет запросы по ключам, switch on error
)

// LimitWindow is one of the rolling windows required by §4.3.
type LimitWindow string

const (
	Window5h  LimitWindow = "5h"
	Window24h LimitWindow = "24h"
	Window30d LimitWindow = "30d"
)

// ID is the persistent identifier type (string, for portability across stores).
type ID = string

// Time helpers — wrap time.Time so domain stays testable.
type Time = time.Time