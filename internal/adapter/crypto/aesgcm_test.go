package crypto

import (
	"bytes"
	"context"
	"testing"
)

// Self-checks for the crypto adapter (AGENTS.md: ONE runnable check per
// non-trivial logic). AES-GCM round-trip + wrong-key rejection + bcrypt.
func TestAESGCM_RoundTrip(t *testing.T) {
	key := bytes.Repeat([]byte{0x01}, 32) // 32-byte key
	s, err := NewAESGCM(key)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	plaintext := []byte("sk-upstream-secret-12345")
	ct, err := s.Encrypt(ctx, plaintext)
	if err != nil {
		t.Fatal(err)
	}
	// Ciphertext must differ from plaintext (it's actually encrypted).
	if bytes.Equal(ct, plaintext) {
		t.Fatal("ciphertext equals plaintext — not encrypted")
	}
	// Nonce is random → two encryptions of the same value differ.
	ct2, _ := s.Encrypt(ctx, plaintext)
	if bytes.Equal(ct, ct2) {
		t.Fatal("two encryptions produced identical ciphertext — nonce not random")
	}
	// Round-trip.
	pt, err := s.Decrypt(ctx, ct)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(pt, plaintext) {
		t.Fatalf("round-trip mismatch: got %q want %q", pt, plaintext)
	}
}

func TestAESGCM_WrongKeyRejected(t *testing.T) {
	s1, _ := NewAESGCM(bytes.Repeat([]byte{1}, 32))
	s2, _ := NewAESGCM(bytes.Repeat([]byte{2}, 32))
	ctx := context.Background()
	ct, _ := s1.Encrypt(ctx, []byte("secret"))
	if _, err := s2.Decrypt(ctx, ct); err == nil {
		t.Fatal("decrypt with wrong key should fail")
	}
}

func TestAESGCM_RejectsBadKeyLength(t *testing.T) {
	if _, err := NewAESGCM(bytes.Repeat([]byte{1}, 16)); err == nil {
		t.Fatal("16-byte key should be rejected (need 32)")
	}
}

func TestBcrypt_RoundTrip(t *testing.T) {
	h := BcryptHasher{}
	hash, err := h.Hash("correct horse battery")
	if err != nil {
		t.Fatal(err)
	}
	if err := h.Verify(hash, "correct horse battery"); err != nil {
		t.Fatalf("verify correct password failed: %v", err)
	}
	if err := h.Verify(hash, "wrong"); err == nil {
		t.Fatal("verify wrong password should fail")
	}
	// Short password rejected at the trust boundary.
	if _, err := h.Hash("short"); err == nil {
		t.Fatal("short password should be rejected")
	}
}
