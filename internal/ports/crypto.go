package ports

import "context"

// SecretStore encrypts/decrypts upstream keys at rest (§7).
type SecretStore interface {
	Encrypt(ctx context.Context, plaintext []byte) ([]byte, error)
	Decrypt(ctx context.Context, ciphertext []byte) ([]byte, error)
}

// PasswordHasher — admin password hashing/verification (§7).
type PasswordHasher interface {
	Hash(password string) (string, error)
	Verify(hash, password string) error
}
