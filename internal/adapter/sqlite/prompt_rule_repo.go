package sqlite

import (
	"context"
	"database/sql"
	"time"

	"github.com/arbuz/ai-arbuz-provider-api/internal/domain"
)

// PromptRuleRepo implements ports.PromptRuleRepo (§4.7).
type PromptRuleRepo struct{ db *sql.DB }

func NewPromptRuleRepo(db *sql.DB) *PromptRuleRepo { return &PromptRuleRepo{db: db} }

func (r *PromptRuleRepo) Create(ctx context.Context, rule *domain.PromptRule) error {
	now := time.Now().UTC()
	rule.ID = domain.NewID()
	rule.CreatedAt = now
	rule.UpdatedAt = now
	if rule.Status == "" {
		rule.Status = domain.StatusActive
	}
	_, err := r.db.ExecContext(ctx, `
INSERT INTO prompt_rules(id, name, kind, value, param, status, created_at, updated_at)
VALUES(?,?,?,?,?,?,?,?)`,
		rule.ID, rule.Name, rule.Kind, rule.Value, rule.Param,
		string(rule.Status), encodeTime(now), encodeTime(now))
	return err
}

func (r *PromptRuleRepo) Update(ctx context.Context, rule *domain.PromptRule) error {
	rule.UpdatedAt = time.Now().UTC()
	_, err := r.db.ExecContext(ctx, `
UPDATE prompt_rules SET name=?, kind=?, value=?, param=?, status=?, updated_at=? WHERE id=?`,
		rule.Name, rule.Kind, rule.Value, rule.Param,
		string(rule.Status), encodeTime(rule.UpdatedAt), rule.ID)
	return err
}

func (r *PromptRuleRepo) Delete(ctx context.Context, id domain.ID) error {
	_, err := r.db.ExecContext(ctx, `DELETE FROM prompt_rules WHERE id=?`, id)
	return err
}

func (r *PromptRuleRepo) Get(ctx context.Context, id domain.ID) (*domain.PromptRule, error) {
	row := r.db.QueryRowContext(ctx,
		`SELECT id, name, kind, value, param, status, created_at, updated_at FROM prompt_rules WHERE id=?`, id)
	return scanPromptRule(row)
}

func (r *PromptRuleRepo) List(ctx context.Context) ([]domain.PromptRule, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT id, name, kind, value, param, status, created_at, updated_at FROM prompt_rules ORDER BY created_at ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []domain.PromptRule
	for rows.Next() {
		rule, err := scanPromptRule(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *rule)
	}
	return out, rows.Err()
}

func scanPromptRule(s scanner) (*domain.PromptRule, error) {
	var rule domain.PromptRule
	var status, created, updated string
	if err := s.Scan(&rule.ID, &rule.Name, &rule.Kind, &rule.Value, &rule.Param,
		&status, &created, &updated); err != nil {
		if err == sql.ErrNoRows {
			return nil, domain.ErrNotFound
		}
		return nil, err
	}
	rule.Status = domain.Status(status)
	rule.CreatedAt = decodeTime(created)
	rule.UpdatedAt = decodeTime(updated)
	return &rule, nil
}
