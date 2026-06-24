package usecase

import (
	"context"

	"github.com/arbuz/ai-arbuz-provider-api/internal/domain"
	"github.com/arbuz/ai-arbuz-provider-api/internal/ports"
)

// LogService writes request telemetry and serves log queries.
type LogService struct {
	repo ports.LogRepo
}

func NewLogService(repo ports.LogRepo) *LogService { return &LogService{repo: repo} }

func (s *LogService) Record(ctx context.Context, l *domain.RequestLog) error {
	return s.repo.Insert(ctx, l)
}

func (s *LogService) Get(ctx context.Context, id domain.ID) (*domain.RequestLog, error) {
	return s.repo.Get(ctx, id)
}

func (s *LogService) List(ctx context.Context, f ports.LogFilter) ([]domain.RequestLog, error) {
	return s.repo.List(ctx, f)
}
