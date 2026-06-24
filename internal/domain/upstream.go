package domain

// UpstreamKey — a third-party provider key added by the owner (§4.1).
// The secret is stored encrypted at rest; the domain holds the plaintext only
// in-memory when actually proxying.
type UpstreamKey struct {
	ID             ID
	ProviderID     ID
	Name           string
	BaseURL        string
	Format         Format // openai | anthropic
	SecretEnc      []byte // ciphertext (AES-GCM), never plaintext
	Models         []string
	UseGlobalModels bool   // when true, provider.GlobalModels apply
	Priority       int    // failover order (lower = earlier)
	Status         Status
	// UpstreamLimits are the limits imposed by the third party (§2).
	// Accounted SEPARATELY from issued-key limits; never mixed.
	UpstreamLimits Limits
	Health         UpstreamHealth
	CreatedAt      Time
	UpdatedAt      Time
}

// UpstreamHealth tracks recent health of an upstream key for failover decisions.
type UpstreamHealth struct {
	// FailuresUntil cooldown: consecutive failures. Reset on success.
	ConsecutiveFailures int
	// CooldownUntil: when set, key is skipped until this time.
	CooldownUntil Time
}

// Usable reports whether a key is currently selectable (active + not cooling down).
func (k UpstreamKey) Usable(now Time) bool {
	if k.Status != StatusActive {
		return false
	}
	if !k.Health.CooldownUntil.IsZero() && now.Before(k.Health.CooldownUntil) {
		return false
	}
	return true
}

// EffectiveModels returns the models a key actually serves.
func (k UpstreamKey) EffectiveModels(global []string) []string {
	if k.UseGlobalModels || len(k.Models) == 0 {
		return global
	}
	return k.Models
}
