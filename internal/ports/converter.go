package ports

import (
	"context"
	"io"

	"github.com/arbuz/ai-arbuz-provider-api/internal/domain"
)

// Converter transforms API payloads between OpenAI and Anthropic wire formats.
// Isolated module per §3.1 and §4.6.
type Converter interface {
	// NeedsConversion reports whether in→out requires actual conversion
	// (same-format = pass-through, no work).
	NeedsConversion(in, out domain.Format) bool

	// ConvertRequest converts a non-streaming request body in→out.
	ConvertRequest(ctx context.Context, body []byte, in, out domain.Format) ([]byte, error)
	// ConvertResponse converts a non-streaming response body out→in.
	ConvertResponse(ctx context.Context, body []byte, out, in domain.Format) ([]byte, error)

	// ConvertStream converts an SSE byte stream out→in, writing to w.
	// The final usage chunk is parsed and returned for accounting.
	ConvertStream(ctx context.Context, r io.Reader, w io.Writer, out, in domain.Format) (Usage, error)
}

// Usage is the token tally extracted from a converted stream/response.
type Usage struct {
	PromptTokens     int64
	CompletionTokens int64
}
