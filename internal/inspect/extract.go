package inspect

import "encoding/json"

// extract.go pulls the assistant-visible text and tool-call payloads out of a
// buffered (non-stream) provider response so the engine can scan them. Tool-call
// names+arguments are scanned because that is the payload a malicious provider
// uses to make an agent client auto-execute commands.
//
// ponytail: handles the common shapes only (string "content", standard
// tool_calls / tool_use). Ceiling = multimodal content arrays are reduced to
// their text parts. Growth path = richer content walking if ever needed.

// ScanInput is one piece of provider output to inspect, tagged with its origin.
type ScanInput struct {
	Text   string
	Source string
}

// ScanResponse extracts then inspects a buffered response body for the given
// wire format ("openai" or "anthropic"), returning all findings.
func ScanResponse(e *Engine, body []byte, format string) []Finding {
	if e == nil || len(body) == 0 {
		return nil
	}
	var inputs []ScanInput
	switch format {
	case "anthropic":
		inputs = extractAnthropic(body)
	default:
		inputs = extractOpenAI(body)
	}
	var out []Finding
	for _, in := range inputs {
		out = append(out, e.Inspect(in.Text, in.Source)...)
	}
	return out
}

// --- OpenAI chat-completion (non-stream) ---

func extractOpenAI(body []byte) []ScanInput {
	var r struct {
		Choices []struct {
			Message struct {
				Content   json.RawMessage `json:"content"`
				ToolCalls []struct {
					Function struct {
						Name      string `json:"name"`
						Arguments string `json:"arguments"`
					} `json:"function"`
				} `json:"tool_calls"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(body, &r); err != nil {
		return nil
	}
	var out []ScanInput
	for _, c := range r.Choices {
		if t := contentText(c.Message.Content); t != "" {
			out = append(out, ScanInput{Text: t, Source: "assistant text"})
		}
		for _, tc := range c.Message.ToolCalls {
			out = append(out, ScanInput{
				Text:   tc.Function.Name + " " + tc.Function.Arguments,
				Source: "tool call: " + tc.Function.Name,
			})
		}
	}
	return out
}

// --- Anthropic messages (non-stream) ---

func extractAnthropic(body []byte) []ScanInput {
	var r struct {
		Content []struct {
			Type  string          `json:"type"`
			Text  string          `json:"text"`
			Name  string          `json:"name"`
			Input json.RawMessage `json:"input"`
		} `json:"content"`
	}
	if err := json.Unmarshal(body, &r); err != nil {
		return nil
	}
	var out []ScanInput
	for _, b := range r.Content {
		switch b.Type {
		case "text":
			if b.Text != "" {
				out = append(out, ScanInput{Text: b.Text, Source: "assistant text"})
			}
		case "tool_use":
			out = append(out, ScanInput{
				Text:   b.Name + " " + string(b.Input),
				Source: "tool call: " + b.Name,
			})
		}
	}
	return out
}

// contentText reduces an OpenAI "content" field (a JSON string or an array of
// typed parts) to its plain text.
func contentText(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var s string
	if json.Unmarshal(raw, &s) == nil {
		return s
	}
	var parts []struct {
		Text string `json:"text"`
	}
	if json.Unmarshal(raw, &parts) == nil {
		var b string
		for _, p := range parts {
			b += p.Text + " "
		}
		return b
	}
	return ""
}
