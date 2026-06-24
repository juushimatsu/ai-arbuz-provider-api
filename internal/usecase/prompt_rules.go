package usecase

import (
	"encoding/json"
	"strings"

	"github.com/arbuz/ai-arbuz-provider-api/internal/domain"
)

// The PromptRule type lives in domain (prompt_rule.go). This file holds the
// pure transformation logic that consumes domain.PromptRule slices.

// ApplyPromptRules mutates a JSON request body in place according to rules.
// Format-agnostic: rewrites the generic "messages" array and top-level fields
// that both OpenAI and Anthropic carry. Returns the (possibly new) body.
//
// Best-effort: if the body isn't a JSON object we recognize, it's returned as-is.
// Only active rules are applied (disabled rules are skipped).
func ApplyPromptRules(body []byte, rules []domain.PromptRule, format domain.Format) []byte {
	if len(rules) == 0 {
		return body
	}
	var v map[string]any
	if json.Unmarshal(body, &v) != nil {
		return body
	}
	for _, r := range rules {
		if r.Status != domain.StatusActive {
			continue
		}
		switch r.Kind {
		case "prepend_system", "append_system":
			sys := systemMessage(format, r.Value)
			msgs, _ := v["messages"].([]any)
			if r.Kind == "prepend_system" {
				v["messages"] = append([]any{sys}, msgs...)
			} else {
				v["messages"] = append(msgs, sys)
			}
		case "replace_model":
			if r.Value != "" {
				v["model"] = r.Value
			}
		case "inject_param":
			if r.Param == "" {
				continue
			}
			// Try to decode Value as JSON; if it fails, store as a string.
			var val any
			if json.Unmarshal([]byte(r.Value), &val) != nil {
				val = r.Value
			}
			v[r.Param] = val
		}
	}
	out, err := json.Marshal(v)
	if err != nil {
		return body
	}
	return out
}

// systemMessage builds a system-role message. Both OpenAI and the converter
// (oa2ant) accept an OpenAI-shaped system message in the array; oa2ant hoists
// it to Anthropic's top-level `system`.
func systemMessage(_ domain.Format, text string) any {
	return map[string]any{"role": "system", "content": strings.TrimSpace(text)}
}
