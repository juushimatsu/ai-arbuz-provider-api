package usecase

import (
	"bufio"
	"encoding/json"
	"io"
	"strings"
	"sync"

	"github.com/arbuz/ai-arbuz-provider-api/internal/domain"
	"github.com/arbuz/ai-arbuz-provider-api/internal/ports"
)

// usageCapture is a concurrency-safe holder for the final token usage parsed
// out of a streaming response. Both the converter goroutine (cross-format) and
// the usageParsingReader (same-format pass-through) write into it; OnDone reads
// it at EOF so streaming output tokens are accounted (blocker #4).
type usageCapture struct {
	mu    sync.Mutex
	usage ports.Usage
}

func (u *usageCapture) set(v ports.Usage) {
	u.mu.Lock()
	defer u.mu.Unlock()
	// Keep the latest non-empty reading (the terminal usage chunk wins over the
	// header-time estimate). Merge so a chunk that only carries output tokens
	// doesn't wipe the input-token count already known.
	if v.PromptTokens != 0 {
		u.usage.PromptTokens = v.PromptTokens
	}
	if v.CompletionTokens != 0 {
		u.usage.CompletionTokens = v.CompletionTokens
	}
}

func (u *usageCapture) get() ports.Usage {
	u.mu.Lock()
	defer u.mu.Unlock()
	return u.usage
}

// usageParsingReader wraps an SSE byte stream and extracts the final usage
// object as it flows, without buffering the whole body (§6 streaming).
// ponytail: ceiling — line-based scan; reads each "data:" payload's top-level
// "usage". Both OpenAI (usage on the final chunk) and Anthropic
// (message_delta.usage) carry a usage object, so one scanner serves both.
type usageParsingReader struct {
	r       *bufio.Reader
	closer  io.Closer
	format  domain.Format
	ucap    *usageCapture
}

func newUsageParsingReader(r io.ReadCloser, format domain.Format, ucap *usageCapture) *usageParsingReader {
	return &usageParsingReader{
		r:      bufio.NewReader(r),
		closer: r,
		format: format,
		ucap:   ucap,
	}
}

func (u *usageParsingReader) Read(p []byte) (int, error) {
	n, err := u.r.Read(p)
	if n > 0 {
		// Scan the buffer slice we just produced for a usage object. We re-scan
		// on every Read rather than tracking leftovers; usage appears once near
		// the end, so the cost is negligible.
		u.scanForUsage(p[:n])
	}
	return n, err
}

func (u *usageParsingReader) Close() error { return u.closer.Close() }

// scanForUsage looks for the JSON substring `"usage":{...}` in a chunk and, if
// found and parseable, records it. Best-effort: partial splits across Read
// boundaries are tolerated because the final usage chunk is emitted whole by
// both providers.
func (u *usageParsingReader) scanForUsage(b []byte) {
	s := string(b)
	idx := strings.Index(s, `"usage"`)
	if idx < 0 {
		return
	}
	rest := s[idx:]
	colon := strings.Index(rest, ":")
	if colon < 0 {
		return
	}
	obj, ok := extractJSONObject(rest[colon+1:])
	if !ok {
		return
	}
	var usage map[string]json.Number
	if json.Unmarshal([]byte(obj), &usage) != nil {
		return
	}
	u.ucap.set(ports.Usage{
		PromptTokens:     numField(usage, "prompt_tokens", "input_tokens"),
		CompletionTokens: numField(usage, "completion_tokens", "output_tokens"),
	})
}

// numField returns the int64 value of the first present numeric key.
func numField(m map[string]json.Number, names ...string) int64 {
	for _, n := range names {
		if v, ok := m[n]; ok {
			if i, err := v.Int64(); err == nil {
				return i
			}
		}
	}
	return 0
}

// extractJSONObject reads the first balanced {...} object starting at s[0]
// (skipping leading whitespace). Returns the object substring inclusive.
// ponytail: ceiling — does not understand strings containing braces inside
// JSON string values; usage objects never contain those, so it's safe here.
func extractJSONObject(s string) (string, bool) {
	i := 0
	for i < len(s) && (s[i] == ' ' || s[i] == '\t' || s[i] == '\n' || s[i] == '\r') {
		i++
	}
	if i >= len(s) || s[i] != '{' {
		return "", false
	}
	depth := 0
	for j := i; j < len(s); j++ {
		switch s[j] {
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				return s[i : j+1], true
			}
		}
	}
	return "", false
}
