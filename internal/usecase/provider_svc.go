package usecase

import (
	"context"
	"errors"
	"strings"

	"github.com/arbuz/ai-arbuz-provider-api/internal/domain"
	"github.com/arbuz/ai-arbuz-provider-api/internal/ports"
)

// ProviderService — CRUD for providers (§4.2).
type ProviderService struct {
	repo ports.ProviderRepo
}

func NewProviderService(repo ports.ProviderRepo) *ProviderService {
	return &ProviderService{repo: repo}
}

func (s *ProviderService) Create(ctx context.Context, p *domain.Provider) error {
	p.Name = strings.TrimSpace(p.Name)
	if p.Name == "" {
		return errors.New("name required")
	}
	if p.Strategy == "" {
		p.Strategy = domain.StrategyFailover
	}
	if p.Status == "" {
		p.Status = domain.StatusActive
	}
	return s.repo.Create(ctx, p)
}

func (s *ProviderService) Update(ctx context.Context, p *domain.Provider) error {
	if p.ID == "" {
		return domain.ErrNotFound
	}
	return s.repo.Update(ctx, p)
}

func (s *ProviderService) Delete(ctx context.Context, id domain.ID) error {
	return s.repo.Delete(ctx, id)
}

func (s *ProviderService) Get(ctx context.Context, id domain.ID) (*domain.Provider, error) {
	return s.repo.Get(ctx, id)
}

func (s *ProviderService) List(ctx context.Context) ([]domain.Provider, error) {
	return s.repo.List(ctx)
}
