package usecase

import (
	"encoding/json"
	"testing"
)

// Verifies ensureStreamUsage injects include_usage without clobbering fields.
func TestEnsureStreamUsage(t *testing.T) {
	cases := []string{
		`{"model":"gpt-5.5","stream":true,"messages":[{"role":"user","content":"hi"}]}`,
		`{"model":"gpt-5.5","stream":true,"stream_options":{"foo":1}}`,
		`{"model":"gpt-5.5","stream":true,"stream_options":{"include_usage":false}}`,
	}
	for _, in := range cases {
		out := ensureStreamUsage([]byte(in))
		var m map[string]json.RawMessage
		if err := json.Unmarshal(out, &m); err != nil {
			t.Fatalf("invalid json out: %v (%s)", err, out)
		}
		var so map[string]json.RawMessage
		if err := json.Unmarshal(m["stream_options"], &so); err != nil {
			t.Fatalf("no stream_options: %v (%s)", err, out)
		}
		if string(so["include_usage"]) != "true" {
			t.Fatalf("include_usage not true: %s", out)
		}
		// model must survive untouched
		if string(m["model"]) != `"gpt-5.5"` {
			t.Fatalf("model field altered: %s", out)
		}
	}
	// malformed body returns unchanged
	bad := []byte("not json")
	if string(ensureStreamUsage(bad)) != "not json" {
		t.Fatal("malformed body should be returned unchanged")
	}
}
