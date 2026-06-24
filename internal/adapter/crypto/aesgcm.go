// Package crypto implements ports.SecretStore (AES-256-GCM) and
// ports.PasswordHasher (bcrypt). Stdlib + golang.org/x/crypto only.
package crypto

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/subtle"
	"errors"
	"fmt"

	"golang.org/x/crypto/bcrypt"
)

// AESGCM is a ports.SecretStore using AES-256-GCM with a random nonce per record.
// ponytail: ceiling — the master key is a single process-wide key (no rotation,
// no envelope encryption); growth path = key versioning + KEK envelope.
type AESGCM struct {
	aead cipher.AEAD
}

// NewAESGCM builds a store from a 32-byte master key.
func NewAESGCM(masterKey []byte) (*AESGCM, error) {
	if len(masterKey) != 32 {
		return nil, fmt.Errorf("master key must be 32 bytes, got %d", len(masterKey))
	}
	block, err := aes.NewCipher(masterKey)
	if err != nil {
		return nil, err
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	return &AESGCM{aead: aead}, nil
}

// Encrypt produces nonce||ciphertext||tag.
func (s *AESGCM) Encrypt(_ context.Context, plaintext []byte) ([]byte, error) {
	nonce := make([]byte, s.aead.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return nil, err
	}
	return s.aead.Seal(nonce, nonce, plaintext, nil), nil
}

// Decrypt reverses Encrypt.
func (s *AESGCM) Decrypt(_ context.Context, ciphertext []byte) ([]byte, error) {
	ns := s.aead.NonceSize()
	if len(ciphertext) < ns+s.aead.Overhead() {
		return nil, errors.New("ciphertext too short")
	}
	nonce, ct := ciphertext[:ns], ciphertext[ns:]
	return s.aead.Open(nil, nonce, ct, nil)
}

// BcryptHasher implements ports.PasswordHasher.
type BcryptHasher struct{}

// Hash returns a bcrypt hash. Cost 12 balances security vs. latency on a VPS.
func (BcryptHasher) Hash(password string) (string, error) {
	if len(password) < 8 {
		return "", errors.New("password must be at least 8 characters")
	}
	b, err := bcrypt.GenerateFromPassword([]byte(password), 12)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

// Verify is constant-time (bcrypt compares via subtle — wrapped to be explicit
// at this trust boundary).
func (BcryptHasher) Verify(hash, password string) error {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(password))
}

// CompareHash is a constant-time string compare helper (generic secret matching).
func CompareHash(x, y string) int {
	return subtle.ConstantTimeCompare([]byte(x), []byte(y))
}
