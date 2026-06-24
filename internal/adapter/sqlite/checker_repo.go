package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"time"

	"github.com/arbuz/ai-arbuz-provider-api/internal/domain"
)

type CheckerRepo struct{ db *sql.DB }

func NewCheckerRepo(db *sql.DB) *CheckerRepo { return &CheckerRepo{db: db} }

func encodeResults(r []domain.CheckerResult) string {
	if r == nil {
		return "[]"
	}
	b, _ := json.Marshal(r)
	return string(b)
}

func decodeResults(s string) []domain.CheckerResult {
	if s == "" {
		return nil
	}
	var r []domain.CheckerResult
	_ = json.Unmarshal([]byte(s), &r)
	return r
}

func (r *CheckerRepo) Insert(ctx context.Context, run *domain.CheckerRun) error {
	run.ID = domain.NewID()
	if run.StartedAt.IsZero() {
		run.StartedAt = time.Now().UTC()
	}
	_, err := r.db.ExecContext(ctx, `
INSERT INTO checker_runs(id, upstream_id, base_url, secret_tail, started_at, results)
VALUES(?,?,?,?,?,?)`,
		run.ID, run.UpstreamID, run.BaseURL, run.SecretTail,
		encodeTime(run.StartedAt), encodeResults(run.Results))
	return err
}

func (r *CheckerRepo) Get(ctx context.Context, id domain.ID) (*domain.CheckerRun, error) {
	row := r.db.QueryRowContext(ctx, `SELECT id, upstream_id, base_url, secret_tail, started_at, results FROM checker_runs WHERE id=?`, id)
	return scanChecker(row)
}

func (r *CheckerRepo) List(ctx context.Context, limit int) ([]domain.CheckerRun, error) {
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	rows, err := r.db.QueryContext(ctx, `SELECT id, upstream_id, base_url, secret_tail, started_at, results FROM checker_runs ORDER BY started_at DESC LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []domain.CheckerRun
	for rows.Next() {
		run, err := scanChecker(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *run)
	}
	return out, rows.Err()
}

func scanChecker(s scanner) (*domain.CheckerRun, error) {
	var run domain.CheckerRun
	var started, results string
	if err := s.Scan(&run.ID, &run.UpstreamID, &run.BaseURL, &run.SecretTail, &started, &results); err != nil {
		if err == sql.ErrNoRows {
			return nil, domain.ErrNotFound
		}
		return nil, err
	}
	run.StartedAt = decodeTime(started)
	run.Results = decodeResults(results)
	return &run, nil
}
