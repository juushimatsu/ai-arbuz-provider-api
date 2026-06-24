package domain

// RequestLog — per-request telemetry (§4.11, §8). The source of truth for stats.
type RequestLog struct {
	ID             ID
	IssuedKeyID    ID
	ProviderID     ID
	UpstreamKeyID  ID
	Model          string
	InFormat       Format // incoming wire format
	OutFormat      Format // upstream wire format (after conversion)
	PromptTokens   int64
	CompletionTokens int64
	TotalTokens    int64
	Success        bool
	ErrorCode      string
	LatencyTTFBMs  int64 // time to first byte
	TotalMs        int64
	Timestamp      Time
	Streamed       bool
	// Payload is optional (§4.7 payload logging). Stored masked.
	Payload        *PayloadSnapshot
}

// PayloadSnapshot is the optional full request/response capture, secrets masked.
type PayloadSnapshot struct {
	RequestBody  string `json:"request_body,omitempty"`
	ResponseBody string `json:"response_body,omitempty"`
}

// Tokens records usage from an upstream response into the log.
func (r *RequestLog) AddTokens(prompt, completion int64) {
	r.PromptTokens += prompt
	r.CompletionTokens += completion
	r.TotalTokens = r.PromptTokens + r.CompletionTokens
}
