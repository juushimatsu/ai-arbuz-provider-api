package usecase

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"strings"
	"time"

	"github.com/arbuz/ai-arbuz-provider-api/internal/domain"
	"github.com/arbuz/ai-arbuz-provider-api/internal/ports"
)

// IssuedService generates and manages the router's own keys (§4.3).
type IssuedService struct {
	repo   ports.IssuedRepo
	prefix string
}

func NewIssuedService(repo ports.IssuedRepo, prefix string) *IssuedService {
	if prefix == "" {
		prefix = "sk-arbuz"
	}
	return &IssuedService{repo: repo, prefix: prefix}
}

// CreateInput describes a key to issue.
type IssuedInput struct {
	Name       string
	ProviderID domain.ID
	Limits     domain.Limits
	ValidDays  int // 0 = no expiry
}

// Create issues a new key and returns the FULL plaintext token (shown once).
func (s *IssuedService) Create(ctx context.Context, in IssuedInput) (*domain.IssuedKey, string, error) {
	in.Name = strings.TrimSpace(in.Name)
	if in.Name == "" {
		return nil, "", errors.New("name required")
	}
	if in.ProviderID == "" {
		return nil, "", errors.New("provider_id required")
	}
	token := s.generateToken()
	k := &domain.IssuedKey{
		ProviderID: in.ProviderID, Name: in.Name, Prefix: s.prefix,
		Token: token, TokenHash: hashToken(token),
		Limits: in.Limits, ValidDays: in.ValidDays, Status: domain.StatusActive,
	}
	if err := s.repo.Create(ctx, k); err != nil {
		return nil, "", err
	}
	return k, token, nil
}

// Update changes limits / status / expiry (revocation = Status=disabled).
func (s *IssuedService) Update(ctx context.Context, id domain.ID, name string, limits domain.Limits, validDays int, status domain.Status) error {
	k, err := s.repo.Get(ctx, id)
	if err != nil {
		return err
	}
	if name = strings.TrimSpace(name); name != "" {
		k.Name = name
	}
	k.Limits = limits
	// Recompute expiry only when validDays changes meaningfully.
	if validDays > 0 {
		k.ValidDays = validDays
		k.ExpiresAt = k.CreatedAt.Add(time.Duration(validDays) * 24 * time.Hour)
	}
	if status != "" {
		k.Status = status
		if status == domain.StatusDisabled && k.RevokedAt.IsZero() {
			k.RevokedAt = time.Now().UTC()
		}
	}
	return s.repo.Update(ctx, k)
}

// Revoke disables a key (sets status + revoked_at) WITHOUT touching limits.
// Bugfix: previously delegated to Update with an empty Limits{}, which wiped the
// key's configured limits on revocation.
func (s *IssuedService) Revoke(ctx context.Context, id domain.ID) error {
	k, err := s.repo.Get(ctx, id)
	if err != nil {
		return err
	}
	if k.Status == domain.StatusDisabled {
		return nil
	}
	k.Status = domain.StatusDisabled
	if k.RevokedAt.IsZero() {
		k.RevokedAt = time.Now().UTC()
	}
	return s.repo.Update(ctx, k)
}

// Pause temporarily disables an issued key without revoking it. A paused key
// stops serving requests (proxy returns 403 "key is paused") until resumed.
func (s *IssuedService) Pause(ctx context.Context, id domain.ID) error {
	k, err := s.repo.Get(ctx, id)
	if err != nil {
		return err
	}
	if k.Status == domain.StatusDisabled {
		return nil // revoked keys cannot be paused
	}
	if k.Status == domain.StatusPaused {
		return nil
	}
	k.Status = domain.StatusPaused
	return s.repo.Update(ctx, k)
}

// Resume re-activates a paused key. No-op if the key is not paused (revoked
// keys stay revoked).
func (s *IssuedService) Resume(ctx context.Context, id domain.ID) error {
	k, err := s.repo.Get(ctx, id)
	if err != nil {
		return err
	}
	if k.Status != domain.StatusPaused {
		return nil
	}
	k.Status = domain.StatusActive
	return s.repo.Update(ctx, k)
}

func (s *IssuedService) Delete(ctx context.Context, id domain.ID) error {
	return s.repo.Delete(ctx, id)
}

func (s *IssuedService) Get(ctx context.Context, id domain.ID) (*domain.IssuedKey, error) {
	return s.repo.Get(ctx, id)
}

func (s *IssuedService) List(ctx context.Context) ([]domain.IssuedKey, error) {
	return s.repo.List(ctx)
}

func (s *IssuedService) ListByProvider(ctx context.Context, providerID domain.ID) ([]domain.IssuedKey, error) {
	return s.repo.ListByProvider(ctx, providerID)
}

// generateToken returns "<prefix>-<48 hex chars>" (24 bytes of entropy).
// ponytail: ceiling — 24 random bytes (~192 bits) is ample for an API key;
// growth path = longer + checksum if brute-force ever becomes plausible.
func (s *IssuedService) generateToken() string {
	b := make([]byte, 24)
	if _, err := rand.Read(b); err != nil {
		panic("crypto/rand failed: " + err.Error())
	}
	return s.prefix + "-" + hex.EncodeToString(b)
}

// hashToken is the stored lookup key (sha256). Looked up by full token on each
// request: constant-time within SQL UNIQUE index, no plaintext persisted.
func hashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

// FindByToken resolves an issued key from a raw token; used by the proxy.
func (s *IssuedService) FindByToken(ctx context.Context, token string) (*domain.IssuedKey, error) {
	token = strings.TrimSpace(token)
	if token == "" {
		return nil, domain.ErrUnauthorized
	}
	return s.repo.GetByTokenHash(ctx, hashToken(token))
}