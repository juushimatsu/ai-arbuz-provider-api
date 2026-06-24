package ports

import (
	"context"
	"io"

	"github.com/arbuz/ai-arbuz-provider-api/internal/domain"
)

// UpstreamRequest is a normalized call to an upstream provider.
type UpstreamRequest struct {
	Method  string
	Path    string // e.g. "/v1/chat/completions"
	Format  domain.Format
	Body    []byte
	Stream  bool
	Headers map[string]string
}

// UpstreamResponse is a normalized upstream reply.
// For non-stream responses, Body holds the full JSON and Stream=nil.
// For streaming, Stream is an io.ReadCloser of the SSE byte stream.
type UpstreamResponse struct {
	StatusCode int
	Body       []byte
	Stream     io.ReadCloser
	// Usage parsed from the response (final chunk for streams).
	PromptTokens     int64
	CompletionTokens int64
}

// UpstreamClient calls a third-party provider with a decrypted key.
// Implementations: OpenAI-compatible, native Anthropic.
type UpstreamClient interface {
	// Do executes one upstream request. Caller closes Stream if non-nil.
	Do(ctx context.Context, baseURL, secret string, req UpstreamRequest) (*UpstreamResponse, error)
	// ListModels calls GET {baseURL}/v1/models (OpenAI-compat) or the Anthropic equivalent.
	ListModels(ctx context.Context, baseURL, secret string, format domain.Format) ([]string, error)
}
