package domain

// IssuedKey — the router's own key issued to a client on top of a Provider (§4.3).
// Its limits are INDEPENDENT of upstream limits.
// JSON tags are snake_case to match the SPA. TokenHash is never exposed.
type IssuedKey struct {
	ID         ID     `json:"id"`
	ProviderID ID     `json:"provider_id"`
	Name       string `json:"name"`
	Prefix     string `json:"prefix"` // e.g. "sk-arbuz"
	// Token is the full secret, shown ONCE at creation. Stored as a lookup hash.
	Token      string `json:"token,omitempty"`
	TokenHash  string `json:"-"` // sha256(prefix + token), never serialized
	Limits     Limits `json:"limits"`     // owner-defined caps (issued limits)
	ValidDays  int    `json:"valid_days"` // 0 = no expiry; else valid for N days from CreatedAt
	Status     Status `json:"status"`     // active | disabled; revocation = Status=disabled
	CreatedAt  Time   `json:"created_at"`
	ExpiresAt  Time   `json:"expires_at"` // zero value = never expires
	RevokedAt  Time   `json:"revoked_at"`
	LastUsedAt Time   `json:"last_used_at"`
}

// IsActive reports whether the key can serve a request right now.
func (k IssuedKey) IsActive(now Time) bool {
	if k.Status != StatusActive {
		return false
	}
	if !k.ExpiresAt.IsZero() && now.After(k.ExpiresAt) {
		return false
	}
	return true
}
