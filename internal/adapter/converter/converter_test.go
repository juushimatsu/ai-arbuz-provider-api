package converter

import (
	"encoding/json"
	"strings"
	"testing"
)

// Self-checks for the converter (AGENTS.md: one runnable check per non-trivial
// logic unit). These exercise the request/response round-trips that matter most:
// system prompts, tools, tool_calls, and that we produce structurally-valid JSON
// in both directions. No frameworks, no fixtures.

func assertJSON(t *testing.T, b []byte) map[string]any {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatalf("not valid JSON: %v\n%s", err, b)
	}
	return m
}

func TestOpenAIToAnthropic_SystemAndText(t *testing.T) {
	in := []byte(`{"model":"gpt-4","messages":[{"role":"system","content":"be brief"},{"role":"user","content":"hi"}]}`)
	out, err := OpenAIToAnthropic(in)
	if err != nil {
		t.Fatal(err)
	}
	m := assertJSON(t, out)
	// system is hoisted to an Anthropic content-block array carrying the text
	// plus a cache_control breakpoint on the final block (§4.6 prompt caching).
	sys, _ := m["system"].([]any)
	if len(sys) != 1 {
		t.Fatalf("system should be a 1-element block array, got %v", m["system"])
	}
	block, _ := sys[0].(map[string]any)
	if block["text"] != "be brief" {
		t.Errorf("system text = %v, want 'be brief'", block["text"])
	}
	if cc, _ := block["cache_control"].(map[string]any); cc["type"] != "ephemeral" {
		t.Errorf("system missing cache_control=ephemeral on final block: %v", block["cache_control"])
	}
	msgs, _ := m["messages"].([]any)
	// system message must NOT appear in messages (it's hoisted to `system`).
	if len(msgs) != 1 {
		t.Fatalf("want 1 message (user only), got %d", len(msgs))
	}
	if m["max_tokens"] != float64(4096) {
		t.Errorf("max_tokens default not applied: %v", m["max_tokens"])
	}
}

func TestOpenAIToAnthropic_Tools(t *testing.T) {
	in := []byte(`{"model":"gpt-4","tools":[{"type":"function","function":{"name":"get_weather","description":"d","parameters":{"type":"object"}}}],"messages":[{"role":"user","content":"hi"}]}`)
	out, err := OpenAIToAnthropic(in)
	if err != nil {
		t.Fatal(err)
	}
	m := assertJSON(t, out)
	tools, _ := m["tools"].([]any)
	if len(tools) != 1 {
		t.Fatalf("want 1 tool, got %d", len(tools))
	}
	t0 := tools[0].(map[string]any)
	if t0["name"] != "get_weather" || t0["input_schema"] == nil {
		t.Errorf("tool shape wrong: %v", t0)
	}
}

func TestAnthropicToOpenAI_SystemHoist(t *testing.T) {
	in := []byte(`{"model":"claude","max_tokens":100,"system":"be brief","messages":[{"role":"user","content":[{"type":"text","text":"hi"}]}]}`)
	out, err := AnthropicToOpenAI(in)
	if err != nil {
		t.Fatal(err)
	}
	m := assertJSON(t, out)
	msgs, _ := m["messages"].([]any)
	if len(msgs) != 2 {
		t.Fatalf("want system+user = 2 messages, got %d", len(msgs))
	}
	first := msgs[0].(map[string]any)
	if first["role"] != "system" || first["content"] != "be brief" {
		t.Errorf("system not hoisted: %v", first)
	}
}

func TestAnthropicResponseToOpenAI_TextAndTools(t *testing.T) {
	in := []byte(`{"id":"m1","model":"claude","role":"assistant","content":[{"type":"text","text":"hello"},{"type":"tool_use","id":"t1","name":"get_weather","input":{"q":"sf"}}],"stop_reason":"tool_use","usage":{"input_tokens":5,"output_tokens":7}}`)
	out, err := AnthropicResponseToOpenAI(in)
	if err != nil {
		t.Fatal(err)
	}
	m := assertJSON(t, out)
	if m["object"] != "chat.completion" {
		t.Errorf("object wrong: %v", m["object"])
	}
	choices, _ := m["choices"].([]any)
	ch := choices[0].(map[string]any)
	msg := ch["message"].(map[string]any)
	tcs, _ := msg["tool_calls"].([]any)
	if len(tcs) != 1 {
		t.Fatalf("want 1 tool_call, got %d", len(tcs))
	}
	usage := m["usage"].(map[string]any)
	if usage["total_tokens"] != float64(12) {
		t.Errorf("usage total = %v, want 12", usage["total_tokens"])
	}
}

func TestOpenAIResponseToAnthropic(t *testing.T) {
	in := []byte(`{"id":"c1","model":"gpt-4","choices":[{"message":{"role":"assistant","content":"hi"},"finish_reason":"stop"}],"usage":{"prompt_tokens":3,"completion_tokens":2}}`)
	out, err := OpenAIResponseToAnthropic(in)
	if err != nil {
		t.Fatal(err)
	}
	m := assertJSON(t, out)
	if m["type"] != "message" {
		t.Errorf("type wrong: %v", m["type"])
	}
	content, _ := m["content"].([]any)
	if len(content) != 1 {
		t.Fatalf("want 1 content block, got %d", len(content))
	}
	if m["stop_reason"] != "end_turn" {
		t.Errorf("stop_reason = %v, want end_turn", m["stop_reason"])
	}
}

func TestParseDataURL(t *testing.T) {
	media, data, ok := parseDataURL("data:image/png;base64,aGVsbG8=")
	if !ok || media != "image/png" || data != "aGVsbG8=" {
		t.Errorf("parseDataURL failed: media=%q data=%q ok=%v", media, data, ok)
	}
	if _, _, ok := parseDataURL("https://x/y.png"); ok {
		t.Error("non-data URL should not parse")
	}
}

// §4.6 Anthropic-specifics: thinking blocks (request + response) and prompt
// caching (cache_control on the system array, OpenAI→Anthropic).

func TestAnthropicResponseToOpenAI_ThinkingBlock(t *testing.T) {
	in := []byte(`{"id":"m","model":"claude","role":"assistant","content":[{"type":"thinking","thinking":"let me consider"},{"type":"text","text":"answer"}],"stop_reason":"end_turn","usage":{"input_tokens":1,"output_tokens":2}}`)
	out, err := AnthropicResponseToOpenAI(in)
	if err != nil {
		t.Fatal(err)
	}
	m := assertJSON(t, out)
	ch := m["choices"].([]any)[0].(map[string]any)
	msg := ch["message"].(map[string]any)
	content, _ := msg["content"].(string)
	if !strings.Contains(content, "[thinking] let me consider") || !strings.Contains(content, "answer") {
		t.Errorf("thinking not surfaced in content: %q", content)
	}
}

func TestAnthropicToOpenAI_ThinkingInMessages(t *testing.T) {
	in := []byte(`{"model":"c","max_tokens":10,"messages":[{"role":"assistant","content":[{"type":"thinking","thinking":"reasoning"},{"type":"text","text":"hi"}]}]}`)
	out, err := AnthropicToOpenAI(in)
	if err != nil {
		t.Fatal(err)
	}
	m := assertJSON(t, out)
	msgs, _ := m["messages"].([]any)
	if len(msgs) != 1 {
		t.Fatalf("want 1 message, got %d", len(msgs))
	}
	content := msgs[0].(map[string]any)["content"].([]any)
	joined := ""
	for _, p := range content {
		pm := p.(map[string]any)
		if t, ok := pm["text"].(string); ok {
			joined += t
		}
	}
	if !strings.Contains(joined, "[thinking] reasoning") || !strings.Contains(joined, "hi") {
		t.Errorf("thinking not preserved: %q", joined)
	}
}

func TestOpenAIToAnthropic_PromptCachingSystem(t *testing.T) {
	// Multiple system parts → array; cache_control lands on the LAST block only.
	in := []byte(`{"model":"gpt-4","messages":[{"role":"system","content":"a"},{"role":"system","content":"b"},{"role":"user","content":"hi"}]}`)
	out, err := OpenAIToAnthropic(in)
	if err != nil {
		t.Fatal(err)
	}
	m := assertJSON(t, out)
	sys, _ := m["system"].([]any)
	if len(sys) != 2 {
		t.Fatalf("want 2 system blocks, got %d", len(sys))
	}
	last := sys[1].(map[string]any)
	if cc, _ := last["cache_control"].(map[string]any); cc["type"] != "ephemeral" {
		t.Error("last system block should carry cache_control=ephemeral")
	}
	first := sys[0].(map[string]any)
	if _, ok := first["cache_control"]; ok {
		t.Error("non-last system block must NOT carry cache_control")
	}
}

func TestStreamOpenAIToAnthropic(t *testing.T) {
	// Two OpenAI chunks then [DONE].
	src := "data: " + openAIChunkHelper("hel", "assistant") + "\n\n" +
		"data: " + openAIChunkHelper("lo", "") + "\n\n" +
		"data: [DONE]\n\n"
	var sb strings.Builder
	if _, err := openAIStreamToAnthropic(strings.NewReader(src), &sb); err != nil {
		t.Fatal(err)
	}
	got := sb.String()
	for _, want := range []string{"message_start", "content_block_start", "text_delta", "message_stop"} {
		if !strings.Contains(got, want) {
			t.Errorf("stream missing %q\n%s", want, got)
		}
	}
}

// openAIChunkHelper builds a minimal OpenAI chunk string for tests.
func openAIChunkHelper(content, role string) string {
	c := openAIChunk(content, role, nil, new(int))
	b, _ := json.Marshal(c)
	return string(b)
}

// TestStreamAnthropicToOpenAI — reverse direction: Anthropic message-stream
// events convert to OpenAI chat.completion.chunk frames, with text deltas and
// final usage captured (audit §3.4 missing coverage).
func TestStreamAnthropicToOpenAI(t *testing.T) {
	src := strings.Join([]string{
		`data: {"type":"message_start","message":{"usage":{"input_tokens":8}}}`,
		`data: {"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}`,
		`data: {"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"hel"}}`,
		`data: {"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"lo"}}`,
		`data: {"type":"message_delta","delta":{"stop_reason":"end_turn"},"usage":{"output_tokens":3}}`,
		`data: {"type":"message_stop"}`,
		"", // trailing empty for the join newline
	}, "\n\n")
	var sb strings.Builder
	usage, err := anthropicStreamToOpenAI(strings.NewReader(src), &sb)
	if err != nil {
		t.Fatal(err)
	}
	got := sb.String()
	for _, want := range []string{`"role":"assistant"`, "hel", "lo", "[DONE]"} {
		if !strings.Contains(got, want) {
			t.Errorf("stream missing %q\n%s", want, got)
		}
	}
	// Usage parsed from message_start (input) + message_delta (output).
	if usage.PromptTokens != 8 || usage.CompletionTokens != 3 {
		t.Errorf("usage = %+v, want prompt=8 completion=3", usage)
	}
}

// TestStreamAnthropicToOpenAI_ThinkingDelta — reasoning deltas are surfaced
// as prefixed text chunks (§4.6).
func TestStreamAnthropicToOpenAI_ThinkingDelta(t *testing.T) {
	src := strings.Join([]string{
		`data: {"type":"message_start","message":{"usage":{"input_tokens":1}}}`,
		`data: {"type":"content_block_delta","index":0,"delta":{"type":"thinking_delta","thinking":"reasoning"}}`,
		`data: {"type":"message_stop"}`,
		"",
	}, "\n\n")
	var sb strings.Builder
	if _, err := anthropicStreamToOpenAI(strings.NewReader(src), &sb); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(sb.String(), "[thinking] reasoning") {
		t.Errorf("thinking delta not surfaced:\n%s", sb.String())
	}
}
