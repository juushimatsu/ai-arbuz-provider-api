package upstream

import (
	"encoding/json"

	"github.com/arbuz/ai-arbuz-provider-api/internal/domain"
)

// extractUsage pulls prompt/completion token counts from a non-streaming
// response body. Both OpenAI and Anthropic carry a "usage" object, but with
// different field names.
func extractUsage(format domain.Format, body []byte) (prompt, completion int64) {
	if len(body) == 0 {
		return 0, 0
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(body, &raw); err != nil {
		return 0, 0
	}
	u, ok := raw["usage"]
	if !ok {
		return 0, 0
	}
	var usage map[string]json.Number
	if err := json.Unmarshal(u, &usage); err != nil {
		return 0, 0
	}
	prompt = num(usage, "prompt_tokens", "input_tokens")
	completion = num(usage, "completion_tokens", "output_tokens")
	return prompt, completion
}

// num returns the first present numeric field by any of the given names.
func num(m map[string]json.Number, names ...string) int64 {
	for _, n := range names {
		if v, ok := m[n]; ok {
			if i, err := v.Int64(); err == nil {
				return i
			}
		}
	}
	return 0
}
