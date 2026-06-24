package sqlite

import (
	"context"
	"database/sql"
	"time"

	"github.com/arbuz/ai-arbuz-provider-api/internal/domain"
)

// UpstreamRepo stores upstream keys with their secrets ALREADY encrypted
// (encryption happens in the use-case layer; the repo is storage-only).
type UpstreamRepo struct{ db *sql.DB }

func NewUpstreamRepo(db *sql.DB) *UpstreamRepo { return &UpstreamRepo{db: db} }

func (r *UpstreamRepo) Create(ctx context.Context, k *domain.UpstreamKey) error {
	now := time.Now().UTC()
	k.ID = domain.NewID()
	k.CreatedAt = now
	k.UpdatedAt = now
	_, err := r.db.ExecContext(ctx, `
INSERT INTO upstream_keys(id, provider_id, name, base_url, format, secret_enc, models, use_global_models,
  priority, status, lim_tokens, lim_requests, consec_failures, cooldown_until, created_at, updated_at)
VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		k.ID, k.ProviderID, k.Name, k.BaseURL, string(k.Format), []byte(k.SecretEnc),
		encodeStrings(k.Models), boolToInt(k.UseGlobalModels), k.Priority, string(k.Status),
		encodeMap(k.UpstreamLimits.Tokens), encodeMap(k.UpstreamLimits.Requests),
		k.Health.ConsecutiveFailures, encodeTime(k.Health.CooldownUntil),
		encodeTime(now), encodeTime(now))
	return err
}

func (r *UpstreamRepo) Update(ctx context.Context, k *domain.UpstreamKey) error {
	k.UpdatedAt = time.Now().UTC()
	_, err := r.db.ExecContext(ctx, `
UPDATE upstream_keys SET name=?, base_url=?, format=?, secret_enc=?, models=?, use_global_models=?,
  priority=?, status=?, lim_tokens=?, lim_requests=?, updated_at=? WHERE id=?`,
		k.Name, k.BaseURL, string(k.Format), []byte(k.SecretEnc),
		encodeStrings(k.Models), boolToInt(k.UseGlobalModels), k.Priority, string(k.Status),
		encodeMap(k.UpstreamLimits.Tokens), encodeMap(k.UpstreamLimits.Requests),
		encodeTime(k.UpdatedAt), k.ID)
	return err
}

func (r *UpstreamRepo) Delete(ctx context.Context, id domain.ID) error {
	_, err := r.db.ExecContext(ctx, `DELETE FROM upstream_keys WHERE id=?`, id)
	return err
}

func (r *UpstreamRepo) Get(ctx context.Context, id domain.ID) (*domain.UpstreamKey, error) {
	row := r.db.QueryRowContext(ctx, selectUpstream+` WHERE id=?`, id)
	return scanUpstream(row)
}

func (r *UpstreamRepo) List(ctx context.Context) ([]domain.UpstreamKey, error) {
	rows, err := r.db.QueryContext(ctx, selectUpstream+` ORDER BY priority ASC, created_at DESC`)
	if err != nil {
		return nil, err
	}
	return collectUpstreams(rows)
}

func (r *UpstreamRepo) ListByProvider(ctx context.Context, providerID domain.ID) ([]domain.UpstreamKey, error) {
	rows, err := r.db.QueryContext(ctx, selectUpstream+` WHERE provider_id=? ORDER BY priority ASC`, providerID)
	if err != nil {
		return nil, err
	}
	return collectUpstreams(rows)
}

func (r *UpstreamRepo) SetHealth(ctx context.Context, id domain.ID, h domain.UpstreamHealth) error {
	_, err := r.db.ExecContext(ctx, `UPDATE upstream_keys SET consec_failures=?, cooldown_until=? WHERE id=?`,
		h.ConsecutiveFailures, encodeTime(h.CooldownUntil), id)
	return err
}

const selectUpstream = `SELECT id, provider_id, name, base_url, format, secret_enc, models, use_global_models,
  priority, status, lim_tokens, lim_requests, consec_failures, cooldown_until, created_at, updated_at
FROM upstream_keys`

func collectUpstreams(rows *sql.Rows) ([]domain.UpstreamKey, error) {
	defer rows.Close()
	var out []domain.UpstreamKey
	for rows.Next() {
		k, err := scanUpstream(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *k)
	}
	return out, rows.Err()
}

func scanUpstream(s scanner) (*domain.UpstreamKey, error) {
	var k domain.UpstreamKey
	var providerID, format, status string
	var secretEnc []byte
	var models, limTokens, limRequests string
	var useGlobal int
	var cooldown, created, updated string
	if err := s.Scan(&k.ID, &providerID, &k.Name, &k.BaseURL, &format, &secretEnc, &models,
		&useGlobal, &k.Priority, &status, &limTokens, &limRequests,
		&k.Health.ConsecutiveFailures, &cooldown, &created, &updated); err != nil {
		if err == sql.ErrNoRows {
			return nil, domain.ErrNotFound
		}
		return nil, err
	}
	k.ProviderID = providerID
	k.Format = domain.Format(format)
	k.SecretEnc = secretEnc
	k.Models = decodeStrings(models)
	k.UseGlobalModels = useGlobal == 1
	k.Status = domain.Status(status)
	k.UpstreamLimits = domain.Limits{Tokens: decodeMap(limTokens), Requests: decodeMap(limRequests)}
	k.Health.CooldownUntil = decodeTime(cooldown)
	k.CreatedAt = decodeTime(created)
	k.UpdatedAt = decodeTime(updated)
	return &k, nil
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}
