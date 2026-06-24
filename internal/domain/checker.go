package domain

// CheckerRun — one API-checker execution result (§4.10).
// JSON tags are snake_case to match the SPA's expectations (the web checker
// reads base_url / started_at / results / http_code / latency_ms / status).
type CheckerRun struct {
	ID         ID              `json:"id"`
	UpstreamID ID              `json:"upstream_id"` // optional: against a saved upstream key
	BaseURL    string          `json:"base_url"`
	SecretTail string          `json:"secret_tail"` // masked tail of the key used
	StartedAt  Time            `json:"started_at"`
	Results    []CheckerResult `json:"results"`
}

// CheckerResult — outcome of a single probe type.
type CheckerResult struct {
	Kind      CheckerProbe `json:"kind"` // models | chat | embeddings | ping
	Status    Status       `json:"status"` // active=passed, disabled=failed
	HTTPCode  int          `json:"http_code"`
	LatencyMs int64        `json:"latency_ms"`
	Error     string       `json:"error,omitempty"`
}

// CheckerProbe enumerates probe kinds the checker can run (§4.10).
type CheckerProbe string

const (
	ProbePing       CheckerProbe = "ping"
	ProbeModels     CheckerProbe = "models"
	ProbeChat       CheckerProbe = "chat"
	ProbeEmbeddings CheckerProbe = "embeddings"
)
