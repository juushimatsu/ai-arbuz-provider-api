package sqlite

import (
	"context"
	"database/sql"
	"time"

	"github.com/arbuz/ai-arbuz-provider-api/internal/domain"
)

// UserRepo — single admin user. Stored in row with fixed id="user".
type UserRepo struct{ db *sql.DB }

func NewUserRepo(db *sql.DB) *UserRepo { return &UserRepo{db: db} }

func (r *UserRepo) Get(ctx context.Context) (*domain.User, error) {
	row := r.db.QueryRowContext(ctx, `SELECT id, login, password_hash, created_at, updated_at FROM users LIMIT 1`)
	var u domain.User
	var login, pw, created, updated string
	if err := row.Scan(&u.ID, &login, &pw, &created, &updated); err != nil {
		if err == sql.ErrNoRows {
			return nil, domain.ErrNotFound
		}
		return nil, err
	}
	u.Login = login
	u.PasswordHash = pw
	u.CreatedAt = decodeTime(created)
	u.UpdatedAt = decodeTime(updated)
	return &u, nil
}

func (r *UserRepo) Update(ctx context.Context, u *domain.User) error {
	now := time.Now().UTC()
	u.UpdatedAt = now
	_, err := r.db.ExecContext(ctx,
		`UPDATE users SET login=?, password_hash=?, updated_at=? WHERE id=?`,
		u.Login, u.PasswordHash, encodeTime(now), u.ID)
	return err
}

// Seed inserts the initial admin user if none exists. Called once at startup.
func (r *UserRepo) Seed(ctx context.Context, login, passwordHash string) error {
	var existing string
	err := r.db.QueryRowContext(ctx, `SELECT id FROM users LIMIT 1`).Scan(&existing)
	if err == nil {
		return nil // already seeded
	}
	if err != sql.ErrNoRows {
		return err
	}
	now := time.Now().UTC()
	u := domain.User{ID: domain.NewID(), Login: login, PasswordHash: passwordHash, CreatedAt: now, UpdatedAt: now}
	_, err = r.db.ExecContext(ctx,
		`INSERT INTO users(id, login, password_hash, created_at, updated_at) VALUES(?,?,?,?,?)`,
		u.ID, u.Login, u.PasswordHash, encodeTime(now), encodeTime(now))
	return err
}

// SessionRepo — admin session tokens.
type SessionRepo struct{ db *sql.DB }

func NewSessionRepo(db *sql.DB) *SessionRepo { return &SessionRepo{db: db} }

func (r *SessionRepo) Create(ctx context.Context, s *domain.Session) error {
	_, err := r.db.ExecContext(ctx,
		`INSERT INTO sessions(token, user_id, created_at, expires_at) VALUES(?,?,?,?)`,
		s.Token, s.UserID, encodeTime(s.CreatedAt), encodeTime(s.ExpiresAt))
	return err
}

func (r *SessionRepo) Get(ctx context.Context, token string) (*domain.Session, error) {
	row := r.db.QueryRowContext(ctx,
		`SELECT token, user_id, created_at, expires_at FROM sessions WHERE token=?`, token)
	var s domain.Session
	var uid, created, expires string
	if err := row.Scan(&s.Token, &uid, &created, &expires); err != nil {
		if err == sql.ErrNoRows {
			return nil, domain.ErrNotFound
		}
		return nil, err
	}
	s.UserID = uid
	s.CreatedAt = decodeTime(created)
	s.ExpiresAt = decodeTime(expires)
	return &s, nil
}

func (r *SessionRepo) Delete(ctx context.Context, token string) error {
	_, err := r.db.ExecContext(ctx, `DELETE FROM sessions WHERE token=?`, token)
	return err
}
