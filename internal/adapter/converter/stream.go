// converter_stream.go — SSE stream conversion in both directions (§4.6).
//
// Streaming is event-by-event: parse each SSE `data: {...}` line, map the chunk
// to the target format, emit a new `data:` line. The final usage chunk and the
// terminal [DONE]/message_stop markers are handled so the client gets a clean
// stream end plus token accounting for the proxy.
package converter

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/arbuz/ai-arbuz-provider-api/internal/domain"
	"github.com/arbuz/ai-arbuz-provider-api/internal/ports"
)

// sseScanner yields the JSON payload of each `data: ` line, plus the sentinel
// "[DONE]" unchanged. ponytail: minimal SSE parser — no multi-line event fields
// or retry handling; OpenAI/Anthropic both use single-line `data:` payloads.
func scanSSE(r io.Reader, emit func(data string)) error {
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 64*1024), 8*1024*1024)
	for sc.Scan() {
		line := sc.Text()
		if line == "" {
			continue
		}
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		payload := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		emit(payload)
	}
	return sc.Err()
}

// writeSSE emits one `data: <json>\n\n` frame (and flushes if possible).
func writeSSE(w io.Writer, payload string) {
	fmt.Fprintf(w, "data: %s\n\n", payload)
}

// ConvertStreamOut→In routes to the right direction.
// Implemented by the Converter type (converter.go) — these are the building blocks.

// anthropicStreamToOpenAI converts Anthropic message-stream events to OpenAI
// chat.completion.chunk frames. Returns usage parsed from message_delta.
func anthropicStreamToOpenAI(r io.Reader, w io.Writer) (ports.Usage, error) {
	var usage ports.Usage
	var idx int
	err := scanSSE(r, func(payload string) {
		if payload == "[DONE]" {
			writeSSE(w, "[DONE]")
			return
		}
		var ev struct {
			Type  string          `json:"type"`
			Delta json.RawMessage `json:"delta"`
			Message json.RawMessage `json:"message"`
			Usage  *struct {
				InputTokens  int `json:"input_tokens"`
				OutputTokens int `json:"output_tokens"`
			} `json:"usage"`
		}
		if json.Unmarshal([]byte(payload), &ev) != nil {
			return
		}
		switch ev.Type {
		case "message_start":
			// Initial chunk with role; extract input usage if present.
			if ev.Message != nil {
				var msg struct {
					Usage *struct {
						InputTokens int `json:"input_tokens"`
					} `json:"usage"`
				}
				_ = json.Unmarshal(ev.Message, &msg)
				if msg.Usage != nil {
					usage.PromptTokens = int64(msg.Usage.InputTokens)
				}
			}
			writeSSE(w, mustJSONStr(openAIChunk("", "assistant", nil, &idx)))
		case "content_block_start":
			// Could be text or tool_use; we emit on deltas.
		case "content_block_delta":
			handleAnthropicDelta(ev.Delta, w, &idx, &usage)
		case "message_delta":
			if ev.Usage != nil {
				usage.CompletionTokens = int64(ev.Usage.OutputTokens)
			}
		case "message_stop":
			// OpenAI expects [DONE] as the terminator.
		}
	})
	if err == nil {
		// Ensure terminator if upstream forgot [DONE].
		writeSSE(w, "[DONE]")
	}
	return usage, err
}

func handleAnthropicDelta(delta json.RawMessage, w io.Writer, idx *int, usage *ports.Usage) {
	var d struct {
		Type string `json:"type"`
		Text string `json:"text"`
		// thinking deltas (§4.6)
		Thinking string `json:"thinking"`
		// tool_use deltas
		ID    string          `json:"id"`
		Name  string          `json:"name"`
		Input json.RawMessage `json:"partial_json"`
		Index *int            `json:"index"`
	}
	if json.Unmarshal(delta, &d) != nil {
		return
	}
	switch d.Type {
	case "text_delta":
		writeSSE(w, mustJSONStr(openAIChunk(d.Text, "", nil, idx)))
	case "thinking_delta":
		// Anthropic reasoning stream (§4.6). OpenAI has no thinking delta; emit
		// as a text chunk prefixed so the content is preserved.
		// ponytail: ceiling — growth path = o1-style reasoning deltas.
		if d.Thinking != "" {
			writeSSE(w, mustJSONStr(openAIChunk("[thinking] "+d.Thinking, "", nil, idx)))
		}
	case "input_json_delta":
		// Tool argument streaming. OpenAI represents it as a tool_calls delta.
		writeSSE(w, mustJSONStr(openAIChunkToolArgs(d.Input, d.Index, idx)))
	case "tool_use":
		// New tool use: emit function name in tool_calls delta.
		writeSSE(w, mustJSONStr(openAIChunkToolBegin(d.ID, d.Name, d.Index, idx)))
	}
}

// openAIStreamToAnthropic converts OpenAI chat.completion.chunk frames to
// Anthropic message-stream events. Returns usage parsed from the final chunk.
func openAIStreamToAnthropic(r io.Reader, w io.Writer) (ports.Usage, error) {
	var usage ports.Usage
	msgStarted := false
	blockStarted := false
	toolStarted := false
	var blockIdx int
	err := scanSSE(r, func(payload string) {
		if payload == "[DONE]" {
			if blockStarted {
				writeSSE(w, mustJSONStr(map[string]any{"type": "content_block_stop", "index": blockIdx}))
			}
			if toolStarted {
				writeSSE(w, mustJSONStr(map[string]any{"type": "content_block_stop", "index": blockIdx + 1}))
			}
			writeSSE(w, mustJSONStr(map[string]any{"type": "message_delta", "delta": map[string]any{"stop_reason": "end_turn"}, "usage": map[string]any{"output_tokens": usage.CompletionTokens}}))
			writeSSE(w, mustJSONStr(map[string]any{"type": "message_stop"}))
			return
		}
		var chunk struct {
			Choices []struct {
				Delta struct {
					Role      string `json:"role"`
					Content   string `json:"content"`
					ToolCalls []struct {
						Index    int    `json:"index"`
						ID       string `json:"id"`
						Type     string `json:"type"`
						Function struct {
							Name      string `json:"name"`
							Arguments string `json:"arguments"`
						} `json:"function"`
					} `json:"tool_calls"`
				} `json:"delta"`
				FinishReason *string `json:"finish_reason"`
			} `json:"choices"`
			Usage *struct {
				PromptTokens     int `json:"prompt_tokens"`
				CompletionTokens int `json:"completion_tokens"`
			} `json:"usage"`
		}
		if json.Unmarshal([]byte(payload), &chunk) != nil {
			return
		}
		if chunk.Usage != nil {
			usage.PromptTokens = int64(chunk.Usage.PromptTokens)
			usage.CompletionTokens = int64(chunk.Usage.CompletionTokens)
		}
		if !msgStarted {
			writeSSE(w, mustJSONStr(map[string]any{
				"type": "message_start",
				"message": map[string]any{
					"id": "msg_stream", "type": "message", "role": "assistant",
					"content": []any{}, "model": "", "stop_reason": nil,
					"usage": map[string]any{"input_tokens": 0, "output_tokens": 0},
				},
			}))
			msgStarted = true
		}
		for _, ch := range chunk.Choices {
			if ch.Delta.Content != "" {
				if !blockStarted {
					writeSSE(w, mustJSONStr(map[string]any{"type": "content_block_start", "index": blockIdx, "content_block": map[string]any{"type": "text", "text": ""}}))
					blockStarted = true
				}
				writeSSE(w, mustJSONStr(map[string]any{"type": "content_block_delta", "index": blockIdx, "delta": map[string]any{"type": "text_delta", "text": ch.Delta.Content}}))
			}
			for _, tc := range ch.Delta.ToolCalls {
				if !toolStarted {
					if blockStarted {
						writeSSE(w, mustJSONStr(map[string]any{"type": "content_block_stop", "index": blockIdx}))
					}
					writeSSE(w, mustJSONStr(map[string]any{"type": "content_block_start", "index": blockIdx + 1, "content_block": map[string]any{"type": "tool_use", "id": tc.ID, "name": tc.Function.Name, "input": map[string]any{}}}))
					toolStarted = true
				}
				if tc.Function.Arguments != "" {
					writeSSE(w, mustJSONStr(map[string]any{"type": "content_block_delta", "index": blockIdx + 1, "delta": map[string]any{"type": "input_json_delta", "partial_json": tc.Function.Arguments}}))
				}
			}
		}
	})
	return usage, err
}

// openAIChunk builds an OpenAI chat.completion.chunk with a text delta.
func openAIChunk(text, role string, _ any, idx *int) map[string]any {
	delta := map[string]any{}
	if role != "" {
		delta["role"] = role
	}
	if text != "" {
		delta["content"] = text
	}
	return map[string]any{
		"id": "chatcmpl-stream", "object": "chat.completion.chunk",
		"choices": []map[string]any{{
			"index": *idx, "delta": delta, "finish_reason": nil,
		}},
	}
}

func openAIChunkToolArgs(input json.RawMessage, blockIdx *int, idx *int) map[string]any {
	args := ""
	if len(input) > 0 {
		args = string(input)
	}
	return map[string]any{
		"id": "chatcmpl-stream", "object": "chat.completion.chunk",
		"choices": []map[string]any{{
			"index": *idx,
			"delta": map[string]any{"tool_calls": []map[string]any{{
				"index": deref(blockIdx), "function": map[string]any{"arguments": args},
			}}},
			"finish_reason": nil,
		}},
	}
}

func openAIChunkToolBegin(id, name string, blockIdx *int, idx *int) map[string]any {
	return map[string]any{
		"id": "chatcmpl-stream", "object": "chat.completion.chunk",
		"choices": []map[string]any{{
			"index": *idx,
			"delta": map[string]any{"tool_calls": []map[string]any{{
				"index": deref(blockIdx), "id": id, "type": "function",
				"function": map[string]any{"name": name, "arguments": ""},
			}}},
			"finish_reason": nil,
		}},
	}
}

func deref(p *int) int {
	if p == nil {
		return 0
	}
	return *p
}

// mustJSONStr marshals v to a JSON string (panics only on impossible inputs).
func mustJSONStr(v any) string {
	b, err := json.Marshal(v)
	if err != nil {
		return "{}"
	}
	return string(b)
}

// Ensure domain import is referenced for the public Converter wiring below.
var _ = domain.FormatOpenAI
