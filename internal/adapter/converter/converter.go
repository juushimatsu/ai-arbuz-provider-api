// converter.go — the ports.Converter implementation tying request, response,
// vision and stream transforms together. Replaces the Phase-1 Identity stub.
package converter

import (
	"context"
	"io"
	"log/slog"

	"github.com/arbuz/ai-arbuz-provider-api/internal/domain"
	"github.com/arbuz/ai-arbuz-provider-api/internal/ports"
)

// Converter implements ports.Converter for OpenAI ↔ Anthropic (§4.6).
type Converter struct {
	log *slog.Logger
}

// New builds the production converter. The logger is optional (nil = discard).
func New(log *slog.Logger) *Converter {
	if log == nil {
		log = slog.New(slog.NewTextHandler(io.Discard, nil))
	}
	return &Converter{log: log}
}

func (c *Converter) NeedsConversion(in, out domain.Format) bool { return in != out }

func (c *Converter) ConvertRequest(_ context.Context, body []byte, in, out domain.Format) ([]byte, error) {
	if in == out {
		return body, nil
	}
	switch {
	case in == domain.FormatOpenAI && out == domain.FormatAnthropic:
		return OpenAIToAnthropic(body)
	case in == domain.FormatAnthropic && out == domain.FormatOpenAI:
		return AnthropicToOpenAI(body)
	}
	return body, nil
}

func (c *Converter) ConvertResponse(_ context.Context, body []byte, out, in domain.Format) ([]byte, error) {
	if in == out {
		return body, nil
	}
	// NOTE: params are (out, in) = (upstream format, client format). We convert
	// FROM `out` TO `in`.
	switch {
	case out == domain.FormatAnthropic && in == domain.FormatOpenAI:
		return AnthropicResponseToOpenAI(body)
	case out == domain.FormatOpenAI && in == domain.FormatAnthropic:
		return OpenAIResponseToAnthropic(body)
	}
	return body, nil
}

func (c *Converter) ConvertStream(_ context.Context, r io.Reader, w io.Writer, out, in domain.Format) (ports.Usage, error) {
	if in == out {
		// Same format: pass bytes through untouched (caller already does this,
		// but keep the contract honest).
		_, err := io.Copy(w, r)
		return ports.Usage{}, err
	}
	switch {
	case out == domain.FormatAnthropic && in == domain.FormatOpenAI:
		return anthropicStreamToOpenAI(r, w)
	case out == domain.FormatOpenAI && in == domain.FormatAnthropic:
		return openAIStreamToAnthropic(r, w)
	}
	_, err := io.Copy(w, r)
	return ports.Usage{}, err
}
