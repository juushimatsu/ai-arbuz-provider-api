package usecase

import (
	"context"
	"errors"
	"strings"

	"github.com/arbuz/ai-arbuz-provider-api/internal/domain"
	"github.com/arbuz/ai-arbuz-provider-api/internal/ports"
)

// PromptRuleService — CRUD for prompt-transformation rules (§4.7).
type PromptRuleService struct {
	repo ports.PromptRuleRepo
}

func NewPromptRuleService(repo ports.PromptRuleRepo) *PromptRuleService {
	return &PromptRuleService{repo: repo}
}

var validKinds = map[string]bool{
	"prepend_system": true,
	"append_system":  true,
	"replace_model":  true,
	"inject_param":   true,
}

func (s *PromptRuleService) Create(ctx context.Context, r *domain.PromptRule) error {
	if err := validateRule(r); err != nil {
		return err
	}
	return s.repo.Create(ctx, r)
}

func (s *PromptRuleService) Update(ctx context.Context, r *domain.PromptRule) error {
	if r.ID == "" {
		return domain.ErrNotFound
	}
	if err := validateRule(r); err != nil {
		return err
	}
	return s.repo.Update(ctx, r)
}

func (s *PromptRuleService) Delete(ctx context.Context, id domain.ID) error { return s.repo.Delete(ctx, id) }
func (s *PromptRuleService) Get(ctx context.Context, id domain.ID) (*domain.PromptRule, error) {
	return s.repo.Get(ctx, id)
}
func (s *PromptRuleService) List(ctx context.Context) ([]domain.PromptRule, error) {
	return s.repo.List(ctx)
}

// Active returns the currently-enabled rules for the proxy to apply.
func (s *PromptRuleService) Active(ctx context.Context) ([]domain.PromptRule, error) {
	all, err := s.repo.List(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]domain.PromptRule, 0, len(all))
	for _, r := range all {
		if r.Status == domain.StatusActive {
			out = append(out, r)
		}
	}
	return out, nil
}

func validateRule(r *domain.PromptRule) error {
	r.Name = strings.TrimSpace(r.Name)
	if r.Name == "" {
		return errors.New("name required")
	}
	if !validKinds[r.Kind] {
		return errors.New("kind must be prepend_system|append_system|replace_model|inject_param")
	}
	if (r.Kind == "prepend_system" || r.Kind == "append_system" || r.Kind == "replace_model") && r.Value == "" {
		return errors.New("value required for this kind")
	}
	if r.Kind == "inject_param" && r.Param == "" {
		return errors.New("param required for inject_param")
	}
	return nil
}
