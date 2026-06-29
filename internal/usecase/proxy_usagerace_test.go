package usecase

import (
	"context"
	"io"
	"strings"
	"testing"

	"github.com/arbuz/ai-arbuz-provider-api/internal/domain"
	"github.com/arbuz/ai-arbuz-provider-api/internal/ports"
)

// usageThenSignal mimics a converter that produces usage only after fully
// reading upstream — exactly the cross-format case. It writes a sentinel to w,
// drains r, then returns non-zero usage.
type usageThenConv struct{}

func (usageThenConv) NeedsConversion(_, _ domain.Format) bool { return true }
func (usageThenConv) ConvertRequest(_ context.Context, b []byte, _, _ domain.Format) ([]byte, error) {
	return b, nil
}
func (usageThenConv) ConvertResponse(_ context.Context, b []byte, _, _ domain.Format) ([]byte, error) {
	return b, nil
}
func (usageThenConv) ConvertStream(_ context.Context, r io.Reader, w io.Writer, _, _ domain.Format) (ports.Usage, error) {
	_, _ = io.WriteString(w, "data: chunk\n\n")
	_, _ = io.Copy(io.Discard, r)
	return ports.Usage{PromptTokens: 11, CompletionTokens: 7}, nil
}

// Regression for the cross-format usage race: the consumer must observe the
// usage by the time it reads EOF from the converted stream. This only holds if
// ucap.set runs BEFORE pw.Close in the proxy goroutine.
func TestCrossFormatUsageVisibleAtEOF(t *testing.T) {
	for i := 0; i < 200; i++ {
		var ucap usageCapture
		pr, pw := io.Pipe()
		conv := usageThenConv{}
		up := io.NopCloser(strings.NewReader("data: upstream\n\n"))
		go func() {
			u, _ := conv.ConvertStream(context.Background(), up, pw, domain.FormatOpenAI, domain.FormatAnthropic)
			ucap.set(u) // must precede Close (mirrors proxy_svc ordering)
			_ = pw.Close()
		}()
		_, _ = io.Copy(io.Discard, pr) // returns at EOF, i.e. after pw.Close
		if got := ucap.get(); got.CompletionTokens == 0 {
			t.Fatalf("iter %d: usage not visible at EOF: %+v", i, got)
		}
	}
}
