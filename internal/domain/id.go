package domain

import (
	"crypto/rand"
	"encoding/hex"
)

// NewID returns a 16-byte random hex id (32 chars). Stdlib-only UUID substitute.
// ponytail: ceiling — not a standards-compliant UUID (no version bits); fine for
// internal opaque ids. Growth path = github.com/google/uuid if interop needed.
func NewID() ID {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		// rand.Read failing means the system PRNG is broken; panic is correct
		// at this trust boundary (we cannot generate ids safely).
		panic("crypto/rand failed: " + err.Error())
	}
	return hex.EncodeToString(b)
}
