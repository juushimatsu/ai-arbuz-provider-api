package domain

import (
	"testing"
	"time"
)

// Self-checks for domain primitives (AGENTS.md). Pure functions, no deps.

func TestNewID_UniqueAndHex(t *testing.T) {
	a := NewID()
	b := NewID()
	if a == b {
		t.Fatal("two ids collided — entropy source broken")
	}
	if len(a) != 32 {
		t.Fatalf("id length = %d, want 32 hex chars", len(a))
	}
	for _, c := range a {
		isHex := (c >= '0' && c <= '9') || (c >= 'a' && c <= 'f')
		if !isHex {
			t.Fatalf("id contains non-hex char %q", c)
		}
	}
}

func TestDetectFormatByPath(t *testing.T) {
	cases := map[string]Format{
		"/v1/chat/completions": FormatOpenAI,
		"/v1/messages":         FormatAnthropic,
		"/v1/messages?beta=true": FormatAnthropic,
		"/v1/models":           FormatOpenAI,
		"/v1/embeddings":       FormatOpenAI,
		"/something/else":      FormatOpenAI, // default
	}
	for path, want := range cases {
		if got := DetectFormatByPath(path); got != want {
			t.Errorf("DetectFormatByPath(%q) = %v, want %v", path, got, want)
		}
	}
}

func TestWindowDuration(t *testing.T) {
	if d := WindowDuration(Window5h); d != 5*3600 {
		t.Errorf("5h = %d, want %d", d, 5*3600)
	}
	if d := WindowDuration(Window24h); d != 24*3600 {
		t.Errorf("24h = %d, want %d", d, 24*3600)
	}
	if d := WindowDuration(Window30d); d != 30*24*3600 {
		t.Errorf("30d = %d, want %d", d, 30*24*3600)
	}
}

func TestLimits_HasAnyCap(t *testing.T) {
	l := NewLimits()
	if l.HasAnyCap() {
		t.Error("empty limits should have no cap")
	}
	l.Tokens[Window24h] = 1000
	if !l.HasAnyCap() {
		t.Error("limits with token cap should report HasAnyCap")
	}
	l2 := NewLimits()
	l2.Requests[Window5h] = 10
	if !l2.HasAnyCap() {
		t.Error("limits with request cap should report HasAnyCap")
	}
}

func TestIssuedKey_IsActive(t *testing.T) {
	now := time.Date(2026, 6, 19, 12, 0, 0, 0, time.UTC)
	k := IssuedKey{Status: StatusActive}
	if !k.IsActive(now) {
		t.Error("active key with no expiry should be active")
	}
	k.ExpiresAt = now.Add(time.Hour)
	if !k.IsActive(now) {
		t.Error("key before expiry should be active")
	}
	k.ExpiresAt = now.Add(-time.Hour)
	if k.IsActive(now) {
		t.Error("expired key should not be active")
	}
	k.ExpiresAt = time.Time{} // reset
	k.Status = StatusDisabled
	if k.IsActive(now) {
		t.Error("disabled key should not be active")
	}
}

func TestUpstreamKey_UsableAndEffectiveModels(t *testing.T) {
	now := time.Now().UTC()
	k := UpstreamKey{Status: StatusActive}
	if !k.Usable(now) {
		t.Error("active key with no cooldown should be usable")
	}
	k.Health.CooldownUntil = now.Add(time.Hour)
	if k.Usable(now) {
		t.Error("key in cooldown should not be usable")
	}
	k.Health.CooldownUntil = time.Time{}
	k.Status = StatusDisabled
	if k.Usable(now) {
		t.Error("disabled key should not be usable")
	}

	// EffectiveModels: use global when flag set OR own list empty.
	k = UpstreamKey{UseGlobalModels: true, Models: []string{"a"}}
	global := []string{"g1", "g2"}
	got := k.EffectiveModels(global)
	if len(got) != 2 || got[0] != "g1" {
		t.Errorf("UseGlobalModels should return global list, got %v", got)
	}
	k.UseGlobalModels = false
	got = k.EffectiveModels(global)
	if len(got) != 1 || got[0] != "a" {
		t.Errorf("should return own list, got %v", got)
	}
	// Empty own list falls back to global.
	k.Models = nil
	got = k.EffectiveModels(global)
	if len(got) != 2 {
		t.Errorf("empty own list should fall back to global, got %v", got)
	}
}
