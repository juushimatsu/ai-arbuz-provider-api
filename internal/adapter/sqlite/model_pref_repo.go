package sqlite

import (
	"context"
	"database/sql"

	"github.com/arbuz/ai-arbuz-provider-api/internal/domain"
)

// ModelPrefRepo stores per-provider "model -> preferred upstream key" mappings.
// FK ON DELETE CASCADE keeps it consistent: deleting a provider or an upstream
// key removes the dependent rows automatically.
type ModelPrefRepo struct{ db *sql.DB }

func NewModelPrefRepo(db *sql.DB) *ModelPrefRepo { return &ModelPrefRepo{db: db} }

func (r *ModelPrefRepo) ListByProvider(ctx context.Context, providerID domain.ID) ([]domain.ModelPreference, error) {
	rows, err := r.db.QueryContext(ctx, `
SELECT provider_id, model, upstream_key_id
FROM model_preferences WHERE provider_id=? ORDER BY model`, providerID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []domain.ModelPreference
	for rows.Next() {
		var p domain.ModelPreference
		if err := rows.Scan(&p.ProviderID, &p.Model, &p.UpstreamKeyID); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

func (r *ModelPrefRepo) Set(ctx context.Context, pref domain.ModelPreference) error {
	_, err := r.db.ExecContext(ctx, `
INSERT INTO model_preferences(provider_id, model, upstream_key_id)
VALUES(?,?,?)
ON CONFLICT(provider_id, model) DO UPDATE SET upstream_key_id=excluded.upstream_key_id`,
		pref.ProviderID, pref.Model, pref.UpstreamKeyID)
	return err
}

func (r *ModelPrefRepo) Delete(ctx context.Context, providerID domain.ID, model string) error {
	_, err := r.db.ExecContext(ctx, `
DELETE FROM model_preferences WHERE provider_id=? AND model=?`, providerID, model)
	return err
}
