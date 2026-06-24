// converter_request_reverse.go — Anthropic → OpenAI request/response conversion.
package converter

import (
	"encoding/json"
	"fmt"
	"strings"
)

// --- Anthropic → OpenAI ---

type anthropicRequest struct {
	Model       string                `json:"model"`
	Messages    []anthropicMessage    `json:"messages"`
	System      json.RawMessage       `json:"system"`
	Tools       []anthropicTool       `json:"tools,omitempty"`
	ToolChoice  json.RawMessage       `json:"tool_choice,omitempty"`
	Stream      bool                  `json:"stream"`
	MaxTokens   int                   `json:"max_tokens"`
	Temperature *float64              `json:"temperature,omitempty"`
	TopP        *float64              `json:"top_p,omitempty"`
}

type anthropicMessage struct {
	Role    string             `json:"role"`
	Content json.RawMessage    `json:"content"`
}

type anthropicTool struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	InputSchema json.RawMessage `json:"input_schema"`
}

// AnthropicToOpenAI converts an Anthropic /v1/messages body to OpenAI chat/completions.
func AnthropicToOpenAI(body []byte) ([]byte, error) {
	var req anthropicRequest
	if err := json.Unmarshal(body, &req); err != nil {
		return nil, fmt.Errorf("anthropic request: %w", err)
	}
	out := map[string]any{
		"model": req.Model,
	}
	if req.MaxTokens > 0 {
		out["max_tokens"] = req.MaxTokens
	}
	if req.Temperature != nil {
		out["temperature"] = *req.Temperature
	}
	if req.TopP != nil {
		out["top_p"] = *req.TopP
	}

	var msgs []map[string]any
	// Anthropic system → OpenAI system message(s).
	if sysText := anthropicSystemText(req.System); sysText != "" {
		msgs = append(msgs, map[string]any{"role": "system", "content": sysText})
	}
	for _, m := range req.Messages {
		msgs = append(msgs, convertAnthropicMessage(m)...)
	}
	out["messages"] = orSlice(msgs)

	if len(req.Tools) > 0 {
		out["tools"] = convertAnthropicTools(req.Tools)
	}
	if len(req.ToolChoice) > 0 {
		if tc, ok := convertAnthropicToolChoice(req.ToolChoice); ok {
			out["tool_choice"] = tc
		}
	}
	return json.Marshal(out)
}

func convertAnthropicMessage(m anthropicMessage) []map[string]any {
	// Anthropic content is either a string or an array of blocks.
	var s string
	if json.Unmarshal(m.Content, &s) == nil {
		return []map[string]any{{"role": m.Role, "content": s}}
	}
	var blocks []map[string]any
	if json.Unmarshal(m.Content, &blocks) != nil {
		return []map[string]any{{"role": m.Role, "content": ""}}
	}

	// Detect block kinds to decide OpenAI shape.
	var textParts, toolUses, toolResults []map[string]any
	for _, b := range blocks {
		switch b["type"] {
		case "text":
			textParts = append(textParts, b)
		case "tool_use":
			toolUses = append(toolUses, b)
		case "tool_result":
			toolResults = append(toolResults, b)
		case "image":
			// vision: lift to OpenAI image_url part
			textParts = append(textParts, b)
		case "thinking":
			// Anthropic reasoning blocks (§4.6) have no OpenAI equivalent on the
			// request side; surface as a quoted text part so the content isn't
			// silently dropped. ponytail: ceiling — OpenAI "reasoning" models use
			// a different field; growth path = map to o1-style reasoning when
			// the upstream advertises it.
			if t, ok := b["thinking"].(string); ok && t != "" {
				textParts = append(textParts, map[string]any{"type": "text", "text": "[thinking] " + t})
			}
		}
	}

	if len(toolResults) > 0 {
		// Anthropic tool_result → OpenAI "tool" role message.
		// ponytail: ceiling — multiple results collapse into one OpenAI message
		// per result; OpenAI allows only one tool_call_id per message, so emit N.
		// (In practice the last one is used here for simplicity; growth path =
		// emit one OpenAI message per tool_result block.)
		// audit #11: emit one OpenAI "tool" message per tool_result block.
		// OpenAI permits a single tool_call_id per message, so N Anthropic
		// results become N messages (previously all but the first were dropped).
		trMsgs := make([]map[string]any, 0, len(toolResults))
		for _, tr := range toolResults {
			trMsgs = append(trMsgs, map[string]any{
				"role": "tool", "tool_call_id": tr["tool_use_id"],
				"content": anthropicToolResultText(tr),
			})
		}
		return trMsgs
	}

	om := map[string]any{"role": m.Role}
	if len(toolUses) > 0 && m.Role == "assistant" {
		// Assistant tool_use → OpenAI assistant.tool_calls.
		toolCalls := []map[string]any{}
		var text string
		for _, b := range blocks {
			if b["type"] == "text" {
				if t, ok := b["text"].(string); ok {
					text += t
				}
			}
			if b["type"] == "tool_use" {
				args, _ := json.Marshal(b["input"])
				toolCalls = append(toolCalls, map[string]any{
					"id": b["id"], "type": "function",
					"function": map[string]any{"name": b["name"], "arguments": string(args)},
				})
			}
		}
		om["content"] = text
		om["tool_calls"] = toolCalls
		return []map[string]any{om}
	}

	// Default: lift text + image + thinking blocks to OpenAI content parts.
	parts := []map[string]any{}
	for _, b := range blocks {
		switch b["type"] {
		case "text":
			if t, ok := b["text"].(string); ok {
				parts = append(parts, map[string]any{"type": "text", "text": t})
			}
		case "thinking":
			// §4.6: surface Anthropic reasoning as a prefixed text part.
			if t, ok := b["thinking"].(string); ok && t != "" {
				parts = append(parts, map[string]any{"type": "text", "text": "[thinking] " + t})
			}
		case "image":
			if url, ok := anthropicImageToURL(b); ok {
				parts = append(parts, map[string]any{"type": "image_url", "image_url": map[string]any{"url": url}})
			}
		}
	}
	om["content"] = parts
	return []map[string]any{om}
}

func convertAnthropicTools(tools []anthropicTool) []map[string]any {
	out := make([]map[string]any, 0, len(tools))
	for _, t := range tools {
		var params any = map[string]any{}
		if len(t.InputSchema) > 0 {
			_ = json.Unmarshal(t.InputSchema, &params)
		}
		out = append(out, map[string]any{
			"type": "function",
			"function": map[string]any{
				"name": t.Name, "description": t.Description, "parameters": params,
			},
		})
	}
	return out
}

func convertAnthropicToolChoice(raw json.RawMessage) (any, bool) {
	var obj struct {
		Type string `json:"type"`
		Name string `json:"name"`
	}
	if json.Unmarshal(raw, &obj) == nil {
		switch obj.Type {
		case "auto":
			return "auto", true
		case "none":
			return "none", true
		case "tool":
			return map[string]any{"type": "function", "function": map[string]any{"name": obj.Name}}, true
		}
	}
	return nil, false
}

// anthropicSystemText extracts a system string from Anthropic's system field
// (which can be a plain string or an array of text blocks, optionally with
// cache_control annotations we ignore).
func anthropicSystemText(raw json.RawMessage) string {
	if len(raw) == 0 || string(raw) == "null" {
		return ""
	}
	var s string
	if json.Unmarshal(raw, &s) == nil {
		return s
	}
	var blocks []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	}
	if json.Unmarshal(raw, &blocks) == nil {
		var sb strings.Builder
		for _, b := range blocks {
			sb.WriteString(b.Text)
		}
		return sb.String()
	}
	return ""
}

func anthropicToolResultText(b map[string]any) string {
	if c, ok := b["content"].(string); ok {
		return c
	}
	if arr, ok := b["content"].([]any); ok {
		var sb strings.Builder
		for _, e := range arr {
			if m, ok := e.(map[string]any); ok {
				if t, ok := m["text"].(string); ok {
					sb.WriteString(t)
				}
			}
		}
		return sb.String()
	}
	return ""
}

// anthropicImageToURL rebuilds a data URL from an Anthropic base64 image source.
func anthropicImageToURL(b map[string]any) (string, bool) {
	src, ok := b["source"].(map[string]any)
	if !ok {
		return "", false
	}
	if src["type"] != "base64" {
		return "", false
	}
	media, _ := src["media_type"].(string)
	data, _ := src["data"].(string)
	if media == "" || data == "" {
		return "", false
	}
	return "data:" + media + ";base64," + data, true
}