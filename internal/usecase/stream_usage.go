package usecase

import (
	"bufio"
	"bytes"
	"encoding/json"
	"io"
	"strings"
	"sync"
	"unicode/utf8"

	"github.com/arbuz/ai-arbuz-provider-api/internal/domain"
	"github.com/arbuz/ai-arbuz-provider-api/internal/ports"
)

// usageCapture is a concurrency-safe holder for the final token usage parsed
// out of a streaming response. Both the converter goroutine (cross-format) and
// the usageParsingReader (same-format pass-through) write into it; OnDone reads
// it at EOF so streaming output tokens are accounted (blocker #4).
type usageCapture struct {
	mu        sync.Mutex
	usage     ports.Usage
	respChars int64 // accumulated assistant text length, for fallback estimation
}

// addChars accumulates the rune-length of assistant text seen on the stream.
// Used only when the upstream never reports real usage.
func (u *usageCapture) addChars(n int) {
	u.mu.Lock()
	u.respChars += int64(n)
	u.mu.Unlock()
}

// chars returns the accumulated assistant text length.
func (u *usageCapture) chars() int64 {
	u.mu.Lock()
	defer u.mu.Unlock()
	return u.respChars
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

// respCharCounter wraps the final response stream (after any format conversion
// and guard wrapping) and counts the rune-length of assistant text emitted in
// SSE chunks. Handles both OpenAI (choices[].delta.content) and Anthropic
// (delta.text) shapes, so a single wrapper covers same-format and cross-format
// paths. It is byte-transparent: it never alters what the client receives.
//
// This feeds the fallback token estimate used when the upstream never reports
// real usage (mirrors the Electron reference, which estimates char/4).
type respCharCounter struct {
	r    io.Reader
	ucap *usageCapture
	buf  []byte
}

func newRespCharCounter(r io.Reader, ucap *usageCapture) *respCharCounter {
	return &respCharCounter{r: r, ucap: ucap}
}

func (c *respCharCounter) Read(p []byte) (int, error) {
	n, err := c.r.Read(p)
	if n > 0 {
		c.scan(p[:n])
	}
	return n, err
}

// Close forwards to the underlying reader if it is a Closer.
func (c *respCharCounter) Close() error {
	if cl, ok := c.r.(io.Closer); ok {
		return cl.Close()
	}
	return nil
}

func (c *respCharCounter) scan(b []byte) {
	c.buf = append(c.buf, b...)
	for {
		i := bytes.IndexByte(c.buf, '\n')
		if i < 0 {
			// Guard against unbounded growth if a newline never arrives.
			if len(c.buf) > 1<<20 {
				c.buf = c.buf[:0]
			}
			return
		}
		line := c.buf[:i]
		c.buf = append([]byte(nil), c.buf[i+1:]...)
		c.handleLine(line)
	}
}

func (c *respCharCounter) handleLine(line []byte) {
	line = bytes.TrimSpace(line)
	if !bytes.HasPrefix(line, []byte("data:")) {
		return
	}
	payload := bytes.TrimSpace(line[len("data:"):])
	if len(payload) == 0 || bytes.Equal(payload, []byte("[DONE]")) {
		return
	}
	var chunk struct {
		Choices []struct {
			Delta struct {
				Content string `json:"content"`
			} `json:"delta"`
		} `json:"choices"`
		Delta struct {
			Text string `json:"text"`
		} `json:"delta"`
	}
	if json.Unmarshal(payload, &chunk) != nil {
		return
	}
	n := 0
	for _, ch := range chunk.Choices {
		n += utf8.RuneCountInString(ch.Delta.Content)
	}
	n += utf8.RuneCountInString(chunk.Delta.Text)
	if n > 0 {
		c.ucap.addChars(n)
	}
}

// estTokens mirrors the reference heuristic: ceil(runeLen/4).
func estTokens(s string) int64 {
	n := utf8.RuneCountInString(s)
	return int64((n + 3) / 4)
}

// requestText extracts human-readable text from a chat/messages request body
// for fallback prompt-token estimation. Handles OpenAI (messages[].content as
// string or content-parts), Anthropic (system + messages), and raw prompt.
func requestText(body []byte) string {
	var m struct {
		Messages []struct {
			Content json.RawMessage `json:"content"`
		} `json:"messages"`
		System json.RawMessage `json:"system"`
		Prompt string `json:"prompt"`
	}
	if json.Unmarshal(body, &m) != nil {
		return string(body)
	}
	var b strings.Builder
	add := func(raw json.RawMessage) {
		if len(raw) == 0 {
			return
		}
		var s string
		if json.Unmarshal(raw, &s) == nil {
			b.WriteString(s)
			b.WriteByte(' ')
			return
		}
		var parts []struct {
			Text string `json:"text"`
		}
		if json.Unmarshal(raw, &parts) == nil {
			for _, p := range parts {
				b.WriteString(p.Text)
				b.WriteByte(' ')
			}
		}
	}
	for _, msg := range m.Messages {
		add(msg.Content)
	}
	add(m.System)
	b.WriteString(m.Prompt)
	if b.Len() == 0 {
		return string(body)
	}
	return b.String()
}

// responseText extracts assistant text from a non-streamed response body for
// fallback completion-token estimation. Handles OpenAI (choices[].message
// .content) and Anthropic (content[].text).
func responseText(body []byte) string {
	var m struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
		Content []struct {
			Text string `json:"text"`
		} `json:"content"`
	}
	if json.Unmarshal(body, &m) != nil {
		return ""
	}
	var b strings.Builder
	for _, ch := range m.Choices {
		b.WriteString(ch.Message.Content)
	}
	for _, c := range m.Content {
		b.WriteString(c.Text)
	}
	return b.String()
}
