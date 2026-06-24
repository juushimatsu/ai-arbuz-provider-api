// converter_responses.go — non-streaming response conversion (both directions).
package converter

import (
	"encoding/json"
	"fmt"
)

// --- Anthropic response → OpenAI ---

type anthropicResponse struct {
	ID      string `json:"id"`
	Model   string `json:"model"`
	Role    string `json:"role"`
	Content []struct {
		Type     string          `json:"type"`
		Text     string          `json:"text,omitempty"`
		Thinking string          `json:"thinking,omitempty"`
		ID       string          `json:"id,omitempty"`
		Name     string          `json:"name,omitempty"`
		Input    json.RawMessage `json:"input,omitempty"`
	} `json:"content"`
	StopReason string `json:"stop_reason"`
	Usage      struct {
		InputTokens  int `json:"input_tokens"`
		OutputTokens int `json:"output_tokens"`
	} `json:"usage"`
}

// AnthropicResponseToOpenAI converts a /v1/messages response to chat/completions.
func AnthropicResponseToOpenAI(body []byte) ([]byte, error) {
	var r anthropicResponse
	if err := json.Unmarshal(body, &r); err != nil {
		return nil, fmt.Errorf("anthropic response: %w", err)
	}
	var text string
	var toolCalls []map[string]any
	for i, c := range r.Content {
		switch c.Type {
		case "text":
			text += c.Text
		case "thinking":
			// Anthropic reasoning (§4.6). OpenAI chat has no thinking field;
			// surface as a prefixed block so reasoning isn't silently lost.
			// ponytail: ceiling — growth path = o1-style reasoning_summary.
			if c.Thinking != "" {
				text += "[thinking] " + c.Thinking + "\n"
			}
		case "tool_use":
			args := "{}"
			if len(c.Input) > 0 {
				args = string(c.Input)
			}
			toolCalls = append(toolCalls, map[string]any{
				"id":   c.ID,
				"type": "function",
				"function": map[string]any{
					"name":      c.Name,
					"arguments": args,
				},
			})
		}
		_ = i
	}
	choice := map[string]any{
		"index": 0,
		"message": map[string]any{
			"role": "assistant",
		},
		"finish_reason": anthropicStopToOpenAI(r.StopReason),
	}
	msg := choice["message"].(map[string]any)
	if len(toolCalls) > 0 {
		msg["tool_calls"] = toolCalls
		msg["content"] = nil
	} else {
		msg["content"] = text
	}
	out := map[string]any{
		"id":      r.ID,
		"object":  "chat.completion",
		"model":   r.Model,
		"choices": []any{choice},
		"usage": map[string]any{
			"prompt_tokens":     r.Usage.InputTokens,
			"completion_tokens": r.Usage.OutputTokens,
			"total_tokens":      r.Usage.InputTokens + r.Usage.OutputTokens,
		},
	}
	return json.Marshal(out)
}

func anthropicStopToOpenAI(reason string) string {
	switch reason {
	case "end_turn":
		return "stop"
	case "max_tokens":
		return "length"
	case "tool_use", "stop_sequence":
		return "tool_calls"
	}
	return "stop"
}

// --- OpenAI response → Anthropic ---

type openaiResponse struct {
	ID      string `json:"id"`
	Model   string `json:"model"`
	Choices []struct {
		Message struct {
			Role     string `json:"role"`
			Content  string `json:"content"`
			ToolCalls []struct {
				ID   string `json:"id"`
				Type string `json:"type"`
				Function struct {
					Name      string `json:"name"`
					Arguments string `json:"arguments"`
				} `json:"function"`
			} `json:"tool_calls"`
		} `json:"message"`
		FinishReason string `json:"finish_reason"`
	} `json:"choices"`
	Usage struct {
		PromptTokens     int `json:"prompt_tokens"`
		CompletionTokens int `json:"completion_tokens"`
	} `json:"usage"`
}

// OpenAIResponseToAnthropic converts a chat/completions response to /v1/messages.
func OpenAIResponseToAnthropic(body []byte) ([]byte, error) {
	var r openaiResponse
	if err := json.Unmarshal(body, &r); err != nil {
		return nil, fmt.Errorf("openai response: %w", err)
	}
	var content []map[string]any
	stopReason := "end_turn"
	if len(r.Choices) > 0 {
		ch := r.Choices[0]
		if ch.Message.Content != "" {
			content = append(content, map[string]any{"type": "text", "text": ch.Message.Content})
		}
		for _, tc := range ch.Message.ToolCalls {
			args := decodeJSONArg(tc.Function.Arguments)
			content = append(content, map[string]any{
				"type": "tool_use", "id": tc.ID, "name": tc.Function.Name, "input": args,
			})
		}
		stopReason = openAIFinishToAnthropic(ch.FinishReason, len(ch.Message.ToolCalls) > 0)
	}
	out := map[string]any{
		"id": r.ID, "type": "message", "role": "assistant",
		"model": r.Model, "content": content, "stop_reason": stopReason,
		"usage": map[string]any{
			"input_tokens":  r.Usage.PromptTokens,
			"output_tokens": r.Usage.CompletionTokens,
		},
	}
	return json.Marshal(out)
}

func openAIFinishToAnthropic(reason string, hasTools bool) string {
	switch reason {
	case "stop":
		if hasTools {
			return "tool_use"
		}
		return "end_turn"
	case "length":
		return "max_tokens"
	case "tool_calls", "function_call":
		return "tool_use"
	}
	return "end_turn"
}
