package domain

// CheckerRun — one API-checker execution result (§4.10).
type CheckerRun struct {
	ID         ID
	UpstreamID ID // optional: against a saved upstream key
	BaseURL    string
	SecretTail string // masked tail of the key used
	StartedAt  Time
	Results    []CheckerResult
}

// CheckerResult — outcome of a single probe type.
type CheckerResult struct {
	Kind     CheckerProbe // models | chat | embeddings | ping
	Status   Status       // active=passed, disabled=failed
	HTTPCode int
	LatencyMs int64
	Error    string
}

// CheckerProbe enumerates probe kinds the checker can run (§4.10).
type CheckerProbe string

const (
	ProbePing       CheckerProbe = "ping"
	ProbeModels     CheckerProbe = "models"
	ProbeChat       CheckerProbe = "chat"
	ProbeEmbeddings CheckerProbe = "embeddings"
)
