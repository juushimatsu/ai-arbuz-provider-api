package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"time"

	"github.com/arbuz/ai-arbuz-provider-api/internal/domain"
)

type MCPRepo struct{ db *sql.DB }

func NewMCPRepo(db *sql.DB) *MCPRepo { return &MCPRepo{db: db} }

func encodeTools(t []domain.MCPTool) string {
	if t == nil {
		return "[]"
	}
	b, _ := json.Marshal(t)
	return string(b)
}

func decodeTools(s string) []domain.MCPTool {
	if s == "" {
		return nil
	}
	var t []domain.MCPTool
	_ = json.Unmarshal([]byte(s), &t)
	return t
}

func (r *MCPRepo) Create(ctx context.Context, m *domain.MCPServer) error {
	now := time.Now().UTC()
	m.ID = domain.NewID()
	m.CreatedAt = now
	m.UpdatedAt = now
	_, err := r.db.ExecContext(ctx, `
INSERT INTO mcp_servers(id, name, kind, transport, address, tools, status, created_at, updated_at)
VALUES(?,?,?,?,?,?,?,?,?)`,
		m.ID, m.Name, string(m.Kind), m.Transport, m.Address, encodeTools(m.Tools),
		string(m.Status), encodeTime(now), encodeTime(now))
	return err
}

func (r *MCPRepo) Update(ctx context.Context, m *domain.MCPServer) error {
	m.UpdatedAt = time.Now().UTC()
	_, err := r.db.ExecContext(ctx, `
UPDATE mcp_servers SET name=?, kind=?, transport=?, address=?, tools=?, status=?, updated_at=? WHERE id=?`,
		m.Name, string(m.Kind), m.Transport, m.Address, encodeTools(m.Tools),
		string(m.Status), encodeTime(m.UpdatedAt), m.ID)
	return err
}

func (r *MCPRepo) Delete(ctx context.Context, id domain.ID) error {
	_, err := r.db.ExecContext(ctx, `DELETE FROM mcp_servers WHERE id=?`, id)
	return err
}

func (r *MCPRepo) Get(ctx context.Context, id domain.ID) (*domain.MCPServer, error) {
	row := r.db.QueryRowContext(ctx, selectMCP+` WHERE id=?`, id)
	return scanMCP(row)
}

func (r *MCPRepo) List(ctx context.Context) ([]domain.MCPServer, error) {
	rows, err := r.db.QueryContext(ctx, selectMCP+` ORDER BY created_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []domain.MCPServer
	for rows.Next() {
		m, err := scanMCP(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *m)
	}
	return out, rows.Err()
}

const selectMCP = `SELECT id, name, kind, transport, address, tools, status, created_at, updated_at FROM mcp_servers`

func scanMCP(s scanner) (*domain.MCPServer, error) {
	var m domain.MCPServer
	var kind, status, tools, created, updated string
	if err := s.Scan(&m.ID, &m.Name, &kind, &m.Transport, &m.Address, &tools, &status, &created, &updated); err != nil {
		if err == sql.ErrNoRows {
			return nil, domain.ErrNotFound
		}
		return nil, err
	}
	m.Kind = domain.MCPKind(kind)
	m.Tools = decodeTools(tools)
	m.Status = domain.Status(status)
	m.CreatedAt = decodeTime(created)
	m.UpdatedAt = decodeTime(updated)
	return &m, nil
}
