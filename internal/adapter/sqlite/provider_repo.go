package sqlite

import (
	"context"
	"database/sql"
	"time"

	"github.com/arbuz/ai-arbuz-provider-api/internal/domain"
)

type ProviderRepo struct{ db *sql.DB }

func NewProviderRepo(db *sql.DB) *ProviderRepo { return &ProviderRepo{db: db} }

func (r *ProviderRepo) Create(ctx context.Context, p *domain.Provider) error {
	now := time.Now().UTC()
	p.ID = domain.NewID()
	p.CreatedAt = now
	p.UpdatedAt = now
	_, err := r.db.ExecContext(ctx, `
INSERT INTO providers(id, name, strategy, global_models, fallback_models, status, created_at, updated_at)
VALUES(?,?,?,?,?,?,?,?)`,
		p.ID, p.Name, string(p.Strategy), encodeStrings(p.GlobalModels),
		encodeStrings(p.FallbackModels), string(p.Status),
		encodeTime(now), encodeTime(now))
	return err
}

func (r *ProviderRepo) Update(ctx context.Context, p *domain.Provider) error {
	p.UpdatedAt = time.Now().UTC()
	_, err := r.db.ExecContext(ctx, `
UPDATE providers SET name=?, strategy=?, global_models=?, fallback_models=?, status=?, updated_at=? WHERE id=?`,
		p.Name, string(p.Strategy), encodeStrings(p.GlobalModels),
		encodeStrings(p.FallbackModels), string(p.Status), encodeTime(p.UpdatedAt), p.ID)
	return err
}

func (r *ProviderRepo) Delete(ctx context.Context, id domain.ID) error {
	_, err := r.db.ExecContext(ctx, `DELETE FROM providers WHERE id=?`, id)
	return err
}

func (r *ProviderRepo) Get(ctx context.Context, id domain.ID) (*domain.Provider, error) {
	row := r.db.QueryRowContext(ctx, `
SELECT id, name, strategy, global_models, fallback_models, status, created_at, updated_at
FROM providers WHERE id=?`, id)
	p, err := scanProvider(row)
	if err != nil {
		return nil, err
	}
	return p, nil
}

func (r *ProviderRepo) List(ctx context.Context) ([]domain.Provider, error) {
	rows, err := r.db.QueryContext(ctx, `
SELECT id, name, strategy, global_models, fallback_models, status, created_at, updated_at
FROM providers ORDER BY created_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []domain.Provider
	for rows.Next() {
		p, err := scanProvider(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *p)
	}
	return out, rows.Err()
}

// scanner abstracts *Row and *Rows Scan (both implement Scan).
type scanner interface {
	Scan(dest ...any) error
}

func scanProvider(s scanner) (*domain.Provider, error) {
	var p domain.Provider
	var strategy, status, gmodels, fmodels, created, updated string
	if err := s.Scan(&p.ID, &p.Name, &strategy, &gmodels, &fmodels, &status, &created, &updated); err != nil {
		if err == sql.ErrNoRows {
			return nil, domain.ErrNotFound
		}
		return nil, err
	}
	p.Strategy = domain.RoutingStrategy(strategy)
	p.GlobalModels = decodeStrings(gmodels)
	p.FallbackModels = decodeStrings(fmodels)
	p.Status = domain.Status(status)
	p.CreatedAt = decodeTime(created)
	p.UpdatedAt = decodeTime(updated)
	return &p, nil
}
