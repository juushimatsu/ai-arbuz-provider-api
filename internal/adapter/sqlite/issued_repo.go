package sqlite

import (
	"context"
	"database/sql"
	"time"

	"github.com/arbuz/ai-arbuz-provider-api/internal/domain"
)

type IssuedRepo struct{ db *sql.DB }

func NewIssuedRepo(db *sql.DB) *IssuedRepo { return &IssuedRepo{db: db} }

func (r *IssuedRepo) Create(ctx context.Context, k *domain.IssuedKey) error {
	now := time.Now().UTC()
	k.ID = domain.NewID()
	k.CreatedAt = now
	if k.ValidDays > 0 {
		k.ExpiresAt = now.Add(time.Duration(k.ValidDays) * 24 * time.Hour)
	}
	_, err := r.db.ExecContext(ctx, `
INSERT INTO issued_keys(id, provider_id, name, prefix, token_hash, valid_days, lim_tokens, lim_requests,
  status, created_at, expires_at, revoked_at, last_used_at)
VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		k.ID, k.ProviderID, k.Name, k.Prefix, k.TokenHash, k.ValidDays,
		encodeMap(k.Limits.Tokens), encodeMap(k.Limits.Requests),
		string(k.Status), encodeTime(now), encodeTime(k.ExpiresAt), encodeTime(k.RevokedAt), "")
	return err
}

func (r *IssuedRepo) Update(ctx context.Context, k *domain.IssuedKey) error {
	_, err := r.db.ExecContext(ctx, `
UPDATE issued_keys SET name=?, lim_tokens=?, lim_requests=?, status=?, expires_at=?, revoked_at=?
WHERE id=?`,
		k.Name, encodeMap(k.Limits.Tokens), encodeMap(k.Limits.Requests),
		string(k.Status), encodeTime(k.ExpiresAt), encodeTime(k.RevokedAt), k.ID)
	return err
}

func (r *IssuedRepo) Delete(ctx context.Context, id domain.ID) error {
	_, err := r.db.ExecContext(ctx, `DELETE FROM issued_keys WHERE id=?`, id)
	return err
}

func (r *IssuedRepo) Get(ctx context.Context, id domain.ID) (*domain.IssuedKey, error) {
	row := r.db.QueryRowContext(ctx, selectIssued+` WHERE id=?`, id)
	return scanIssued(row)
}

func (r *IssuedRepo) GetByTokenHash(ctx context.Context, hash string) (*domain.IssuedKey, error) {
	row := r.db.QueryRowContext(ctx, selectIssued+` WHERE token_hash=?`, hash)
	return scanIssued(row)
}

func (r *IssuedRepo) List(ctx context.Context) ([]domain.IssuedKey, error) {
	rows, err := r.db.QueryContext(ctx, selectIssued+` ORDER BY created_at DESC`)
	if err != nil {
		return nil, err
	}
	return collectIssued(rows)
}

func (r *IssuedRepo) ListByProvider(ctx context.Context, providerID domain.ID) ([]domain.IssuedKey, error) {
	rows, err := r.db.QueryContext(ctx, selectIssued+` WHERE provider_id=? ORDER BY created_at DESC`, providerID)
	if err != nil {
		return nil, err
	}
	return collectIssued(rows)
}

func (r *IssuedRepo) MarkUsed(ctx context.Context, id domain.ID, at domain.Time) error {
	_, err := r.db.ExecContext(ctx, `UPDATE issued_keys SET last_used_at=? WHERE id=?`, encodeTime(at), id)
	return err
}

const selectIssued = `SELECT id, provider_id, name, prefix, token_hash, valid_days, lim_tokens, lim_requests,
  status, created_at, expires_at, revoked_at, last_used_at FROM issued_keys`

func collectIssued(rows *sql.Rows) ([]domain.IssuedKey, error) {
	defer rows.Close()
	var out []domain.IssuedKey
	for rows.Next() {
		k, err := scanIssued(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *k)
	}
	return out, rows.Err()
}

func scanIssued(s scanner) (*domain.IssuedKey, error) {
	var k domain.IssuedKey
	var providerID, status, limTokens, limRequests string
	var created, expires, revoked, lastUsed string
	if err := s.Scan(&k.ID, &providerID, &k.Name, &k.Prefix, &k.TokenHash, &k.ValidDays,
		&limTokens, &limRequests, &status, &created, &expires, &revoked, &lastUsed); err != nil {
		if err == sql.ErrNoRows {
			return nil, domain.ErrNotFound
		}
		return nil, err
	}
	k.ProviderID = providerID
	k.Limits = domain.Limits{Tokens: decodeMap(limTokens), Requests: decodeMap(limRequests)}
	k.Status = domain.Status(status)
	k.CreatedAt = decodeTime(created)
	k.ExpiresAt = decodeTime(expires)
	k.RevokedAt = decodeTime(revoked)
	k.LastUsedAt = decodeTime(lastUsed)
	return &k, nil
}
