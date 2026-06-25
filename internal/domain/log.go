package domain

// RequestLog — per-request telemetry (§4.11, §8). The source of truth for stats.
type RequestLog struct {
	ID               ID     `json:"id"`
	IssuedKeyID      ID     `json:"issued_key_id"`
	ProviderID       ID     `json:"provider_id"`
	UpstreamKeyID    ID     `json:"upstream_key_id"`
	Model            string `json:"model"`
	InFormat         Format `json:"in_format"`  // incoming wire format
	OutFormat        Format `json:"out_format"` // upstream wire format (after conversion)
	PromptTokens     int64  `json:"prompt_tokens"`
	CompletionTokens int64  `json:"completion_tokens"`
	TotalTokens      int64  `json:"total_tokens"`
	Success          bool   `json:"success"`
	ErrorCode        string `json:"error_code"`
	LatencyTTFBMs    int64  `json:"latency_ttfb_ms"` // time to first byte
	TotalMs          int64  `json:"total_ms"`
	Timestamp        Time   `json:"timestamp"`
	Streamed         bool   `json:"streamed"`
	// Payload is optional (4.7 payload logging). Stored masked.
	Payload          *PayloadSnapshot `json:"payload,omitempty"`
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