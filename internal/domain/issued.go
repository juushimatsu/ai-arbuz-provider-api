package domain

// IssuedKey — the router's own key issued to a client on top of a Provider (§4.3).
// Its limits are INDEPENDENT of upstream limits.
type IssuedKey struct {
	ID          ID
	ProviderID  ID
	Name        string
	Prefix      string // e.g. "sk-arbuz"
	// Token is the full secret, shown ONCE at creation. Stored as a lookup hash.
	Token       string
	TokenHash   string // sha256(prefix + token), constant-time looked up
	Limits      Limits // owner-defined caps (issued limits)
	ValidDays   int    // 0 = no expiry; else valid for N days from CreatedAt
	Status      Status // active | disabled; revocation = Status=disabled
	CreatedAt   Time
	ExpiresAt   Time // zero value = never expires
	RevokedAt   Time
	LastUsedAt  Time
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
