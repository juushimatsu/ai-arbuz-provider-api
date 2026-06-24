// converter_request.go — non-streaming request/response conversion between
// OpenAI and Anthropic wire formats (§4.6). Pure JSON transforms, no I/O.
package converter

import (
	"encoding/json"
	"fmt"
)

// --- OpenAI → Anthropic ---

// openaiChatRequest mirrors the fields we read from OpenAI's chat body.
type openaiChatRequest struct {
	Model     string          `json:"model"`
	Messages  []openaiMessage `json:"messages"`
	Tools     []openaiTool    `json:"tools,omitempty"`
	ToolChoice json.RawMessage `json:"tool_choice,omitempty"`
	Stream    bool            `json:"stream"`
	MaxTokens int             `json:"max_tokens,omitempty"`
	Temperature *float64      `json:"temperature,omitempty"`
	TopP      *float64        `json:"top_p,omitempty"`
	Stop      json.RawMessage `json:"stop,omitempty"`
}

type openaiMessage struct {
	Role       string             `json:"role"`
	Content    json.RawMessage    `json:"content"`
	Name       string             `json:"name,omitempty"`
	ToolCalls  []openaiToolCall   `json:"tool_calls,omitempty"`
	ToolCallID string             `json:"tool_call_id,omitempty"`
}

type openaiTool struct {
	Type     string `json:"type"`
	Function struct {
		Name        string          `json:"name"`
		Description string          `json:"description"`
		Parameters  json.RawMessage `json:"parameters"`
	} `json:"function"`
}

type openaiToolCall struct {
	ID       string `json:"id"`
	Type     string `json:"type"`
	Function struct {
		Name      string `json:"name"`
		Arguments string `json:"arguments"`
	} `json:"function"`
}

// OpenAIToAnthropic converts an OpenAI chat/completions body to Anthropic /v1/messages.
func OpenAIToAnthropic(body []byte) ([]byte, error) {
	var req openaiChatRequest
	if err := json.Unmarshal(body, &req); err != nil {
		return nil, fmt.Errorf("openai request: %w", err)
	}
	out := map[string]any{
		"model": req.Model,
		// Anthropic requires max_tokens; OpenAI often omits it. Default generously.
		"max_tokens": orDefault(req.MaxTokens, 4096),
	}
	if req.Temperature != nil {
		out["temperature"] = *req.Temperature
	}
	if req.TopP != nil {
		out["top_p"] = *req.TopP
	}

	// Split out the system message: OpenAI puts it in messages; Anthropic wants
	// a top-level "system" field.
	var systemParts []string
	var msgs []map[string]any
	for _, m := range req.Messages {
		if m.Role == "system" {
			if s := openaiContentText(m.Content); s != "" {
				systemParts = append(systemParts, s)
			}
			continue
		}
		msgs = append(msgs, convertOpenAIMessage(m))
	}
	if len(systemParts) > 0 {
		// Emit system as an Anthropic content-block array with cache_control on
		// the final block (§4.6 prompt caching). Anthropic caches the prefix up
		// to the cache_control breakpoint; this lets repeated system prompts hit
		// the cache automatically. OpenAI has no equivalent, so this is a
		// one-way enhancement on the OpenAI→Anthropic path.
		blocks := make([]map[string]any, 0, len(systemParts))
		for i, s := range systemParts {
			b := map[string]any{"type": "text", "text": s}
			if i == len(systemParts)-1 {
				b["cache_control"] = map[string]any{"type": "ephemeral"}
			}
			blocks = append(blocks, b)
		}
		out["system"] = blocks
	}
	out["messages"] = orSlice(msgs)
	if len(req.Tools) > 0 {
		out["tools"] = convertOpenAITools(req.Tools)
	}
	if len(req.ToolChoice) > 0 {
		if tc, ok := convertOpenAIToolChoice(req.ToolChoice); ok {
			out["tool_choice"] = tc
		}
	}
	return json.Marshal(out)
}

// convertOpenAIMessage maps a single OpenAI message to an Anthropic message.
func convertOpenAIMessage(m openaiMessage) map[string]any {
	role := m.Role
	switch role {
	case "assistant":
		// Assistant turns may contain text + tool_calls (assistant tool use).
		blocks := []map[string]any{}
		if txt := openaiContentText(m.Content); txt != "" {
			blocks = append(blocks, map[string]any{"type": "text", "text": txt})
		}
		for _, tc := range m.ToolCalls {
			args := decodeJSONArg(tc.Function.Arguments)
			blocks = append(blocks, map[string]any{
				"type": "tool_use", "id": tc.ID, "name": tc.Function.Name, "input": args,
			})
		}
		return map[string]any{"role": "assistant", "content": blocks}
	case "tool":
		// OpenAI "tool" role = a tool result; Anthropic wraps it as user/tool_result.
		return map[string]any{
			"role": "user",
			"content": []map[string]any{{
				"type": "tool_result", "tool_use_id": m.ToolCallID,
				"content": openaiContentText(m.Content),
			}},
		}
	default:
		// user / any → keep role, convert content (text + image parts).
		return map[string]any{"role": role, "content": convertOpenAIContent(m.Content)}
	}
}

// convertOpenAIContent turns an OpenAI content (string OR array of parts) into
// Anthropic content blocks. Supports text and image_url (vision, §4.6).
func convertOpenAIContent(raw json.RawMessage) any {
	if len(raw) == 0 || string(raw) == "null" {
		return ""
	}
	// Plain string content.
	var s string
	if json.Unmarshal(raw, &s) == nil {
		return []map[string]any{{"type": "text", "text": s}}
	}
	// Array of parts.
	var parts []struct {
		Type     string `json:"type"`
		Text     string `json:"text"`
		ImageURL *struct {
			URL string `json:"url"`
		} `json:"image_url,omitempty"`
	}
	if err := json.Unmarshal(raw, &parts); err != nil {
		return []map[string]any{{"type": "text", "text": string(raw)}}
	}
	blocks := []map[string]any{}
	for _, p := range parts {
		switch p.Type {
		case "text":
			blocks = append(blocks, map[string]any{"type": "text", "text": p.Text})
		case "image_url":
			if p.ImageURL != nil {
				if media, data, ok := parseDataURL(p.ImageURL.URL); ok {
					blocks = append(blocks, map[string]any{
						"type": "image", "source": map[string]any{
							"type": "base64", "media_type": media, "data": data,
						},
					})
				}
			}
		}
	}
	return blocks
}

// convertOpenAITools maps OpenAI function tools to Anthropic tools.
func convertOpenAITools(tools []openaiTool) []map[string]any {
	out := make([]map[string]any, 0, len(tools))
	for _, t := range tools {
		var params any = map[string]any{}
		if len(t.Function.Parameters) > 0 {
			_ = json.Unmarshal(t.Function.Parameters, &params)
		}
		out = append(out, map[string]any{
			"name": t.Function.Name, "description": t.Function.Description,
			"input_schema": params,
		})
	}
	return out
}

// convertOpenAIToolChoice maps OpenAI tool_choice to Anthropic tool_choice.
func convertOpenAIToolChoice(raw json.RawMessage) (any, bool) {
	var s string
	if json.Unmarshal(raw, &s) == nil {
		switch s {
		case "auto":
			return map[string]any{"type": "auto"}, true
		case "none":
			return map[string]any{"type": "none"}, true
		}
		return nil, false
	}
	var obj struct {
		Function struct {
			Name string `json:"name"`
		} `json:"function"`
	}
	if json.Unmarshal(raw, &obj) == nil && obj.Function.Name != "" {
		return map[string]any{"type": "tool", "name": obj.Function.Name}, true
	}
	return nil, false
}

// --- helpers ---

// openaiContentText extracts the textual part of an OpenAI content field.
func openaiContentText(raw json.RawMessage) string {
	if len(raw) == 0 || string(raw) == "null" {
		return ""
	}
	var s string
	if json.Unmarshal(raw, &s) == nil {
		return s
	}
	var parts []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	}
	if json.Unmarshal(raw, &parts) == nil {
		var sb string
		for _, p := range parts {
			if p.Type == "text" || p.Type == "" {
				sb += p.Text
			}
		}
		return sb
	}
	return ""
}

// decodeJSONArg parses a tool-call arguments string into a value; on failure
// returns the raw string so the upstream still receives valid JSON.
func decodeJSONArg(s string) any {
	var v any
	if json.Unmarshal([]byte(s), &v) == nil {
		return v
	}
	return s
}

func orDefault(v, def int) int {
	if v <= 0 {
		return def
	}
	return v
}

func orSlice[T any](s []T) []T {
	if s == nil {
		return []T{}
	}
	return s
}
