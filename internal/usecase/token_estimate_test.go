package usecase

import (
	"io"
	"strings"
	"testing"
)

func TestEstTokens(t *testing.T) {
	cases := map[string]int64{"": 0, "abcd": 1, "abcde": 2, "12345678": 2}
	for in, want := range cases {
		if got := estTokens(in); got != want {
			t.Fatalf("estTokens(%q)=%d want %d", in, got, want)
		}
	}
}

func TestRequestText(t *testing.T) {
	body := []byte(`{"messages":[{"role":"user","content":"hello"},{"role":"assistant","content":[{"type":"text","text":"world"}]}],"system":"sys"}`)
	got := requestText(body)
	for _, want := range []string{"hello", "world", "sys"} {
		if !strings.Contains(got, want) {
			t.Fatalf("requestText missing %q in %q", want, got)
		}
	}
}

func TestResponseText(t *testing.T) {
	if got := responseText([]byte(`{"choices":[{"message":{"content":"hi there"}}]}`)); got != "hi there" {
		t.Fatalf("openai responseText=%q", got)
	}
	if got := responseText([]byte(`{"content":[{"type":"text","text":"anth"}]}`)); got != "anth" {
		t.Fatalf("anthropic responseText=%q", got)
	}
}

func TestRespCharCounter(t *testing.T) {
	sse := `data: {"choices":[{"delta":{"content":"abcd"}}]}` + "\n\n" +
		`data: {"delta":{"text":"ef"}}` + "\n\n" +
		"data: [DONE]\n\n"
	ucap := &usageCapture{}
	rc := newRespCharCounter(strings.NewReader(sse), ucap)
	buf := make([]byte, 7)
	for {
		_, err := rc.Read(buf)
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
	}
	if got := ucap.chars(); got != 6 {
		t.Fatalf("respChars=%d want 6", got)
	}
}
