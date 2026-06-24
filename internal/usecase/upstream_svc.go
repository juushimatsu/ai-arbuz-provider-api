package usecase

import (
	"context"
	"errors"
	"strings"

	"github.com/arbuz/ai-arbuz-provider-api/internal/domain"
	"github.com/arbuz/ai-arbuz-provider-api/internal/ports"
)

// UpstreamService — CRUD for upstream keys (§4.1). Encrypts secrets at rest.
type UpstreamService struct {
	repo  ports.UpstreamRepo
	store ports.SecretStore
}

func NewUpstreamService(repo ports.UpstreamRepo, store ports.SecretStore) *UpstreamService {
	return &UpstreamService{repo: repo, store: store}
}

// CreateInput carries the plaintext secret for creation; it is encrypted before
// persistence and never stored.
type UpstreamInput struct {
	Name            string
	ProviderID      domain.ID
	BaseURL         string
	Format          domain.Format
	PlaintextSecret string
	Models          []string
	UseGlobalModels bool
	Priority        int
	Status          domain.Status
	UpstreamLimits  domain.Limits
}

func (s *UpstreamService) Create(ctx context.Context, in UpstreamInput) (*domain.UpstreamKey, error) {
	if err := validateUpstream(in); err != nil {
		return nil, err
	}
	enc, err := s.store.Encrypt(ctx, []byte(in.PlaintextSecret))
	if err != nil {
		return nil, err
	}
	k := &domain.UpstreamKey{
		ProviderID: in.ProviderID, Name: in.Name, BaseURL: in.BaseURL, Format: in.Format,
		SecretEnc: enc, Models: in.Models, UseGlobalModels: in.UseGlobalModels,
		Priority: in.Priority, Status: orDefaultStatus(in.Status),
		UpstreamLimits: in.UpstreamLimits,
	}
	if err := s.repo.Create(ctx, k); err != nil {
		return nil, err
	}
	return k, nil
}

// Update re-encrypts the secret if a new plaintext is provided.
func (s *UpstreamService) Update(ctx context.Context, id domain.ID, in UpstreamInput) error {
	if id == "" {
		return domain.ErrNotFound
	}
	if err := validateUpstream(in); err != nil {
		return err
	}
	existing, err := s.repo.Get(ctx, id)
	if err != nil {
		return err
	}
	existing.Name = in.Name
	existing.BaseURL = in.BaseURL
	existing.Format = in.Format
	existing.Models = in.Models
	existing.UseGlobalModels = in.UseGlobalModels
	existing.Priority = in.Priority
	if in.Status != "" {
		existing.Status = in.Status
	}
	existing.UpstreamLimits = in.UpstreamLimits
	if in.PlaintextSecret != "" {
		enc, err := s.store.Encrypt(ctx, []byte(in.PlaintextSecret))
		if err != nil {
			return err
		}
		existing.SecretEnc = enc
	}
	return s.repo.Update(ctx, existing)
}

// Reveal returns the decrypted secret plaintext (admin only; for the UI inspector).
func (s *UpstreamService) Reveal(ctx context.Context, id domain.ID) (string, error) {
	k, err := s.repo.Get(ctx, id)
	if err != nil {
		return "", err
	}
	pt, err := s.store.Decrypt(ctx, k.SecretEnc)
	if err != nil {
		return "", err
	}
	return string(pt), nil
}

func (s *UpstreamService) Delete(ctx context.Context, id domain.ID) error {
	return s.repo.Delete(ctx, id)
}

func (s *UpstreamService) Get(ctx context.Context, id domain.ID) (*domain.UpstreamKey, error) {
	return s.repo.Get(ctx, id)
}

func (s *UpstreamService) List(ctx context.Context) ([]domain.UpstreamKey, error) {
	return s.repo.List(ctx)
}

func (s *UpstreamService) ListByProvider(ctx context.Context, providerID domain.ID) ([]domain.UpstreamKey, error) {
	return s.repo.ListByProvider(ctx, providerID)
}

// SetHealth persists failover/cooldown state (used by the selector).
func (s *UpstreamService) SetHealth(ctx context.Context, id domain.ID, h domain.UpstreamHealth) error {
	return s.repo.SetHealth(ctx, id, h)
}

func validateUpstream(in UpstreamInput) error {
	in.Name = strings.TrimSpace(in.Name)
	in.BaseURL = strings.TrimSpace(in.BaseURL)
	if in.Name == "" {
		return errors.New("name required")
	}
	if in.ProviderID == "" {
		return errors.New("provider_id required")
	}
	if in.BaseURL == "" {
		return errors.New("base_url required")
	}
	if in.Format != domain.FormatOpenAI && in.Format != domain.FormatAnthropic {
		return errors.New("format must be openai or anthropic")
	}
	if in.PlaintextSecret == "" {
		return errors.New("secret required")
	}
	return nil
}

func orDefaultStatus(s domain.Status) domain.Status {
	if s == "" {
		return domain.StatusActive
	}
	return s
}
