package converter

import (
	"encoding/json"
	"testing"
)

// audit #11: multiple Anthropic tool_result blocks must each become a separate
// OpenAI "tool" message (previously all but the first were dropped).
func TestConvertAnthropicMessageMultipleToolResults(t *testing.T) {
	content, _ := json.Marshal([]map[string]any{
		{"type": "tool_result", "tool_use_id": "a", "content": "ra"},
		{"type": "tool_result", "tool_use_id": "b", "content": "rb"},
	})
	msgs := convertAnthropicMessage(anthropicMessage{Role: "user", Content: content})
	if len(msgs) != 2 {
		t.Fatalf("want 2 tool messages, got %d", len(msgs))
	}
	if msgs[0]["tool_call_id"] != "a" || msgs[1]["tool_call_id"] != "b" {
		t.Fatalf("tool_call_id mismatch: %+v", msgs)
	}
}
