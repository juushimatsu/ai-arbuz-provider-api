// Package usecase contains application services (§3.2). Each service orchestrates
// domain + ports; it owns no infrastructure.
package usecase

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"time"

	"github.com/arbuz/ai-arbuz-provider-api/internal/domain"
	"github.com/arbuz/ai-arbuz-provider-api/internal/ports"
)

// Auth handles admin login, sessions and credential changes (§4.12).
type Auth struct {
	users    ports.UserRepo
	sessions ports.SessionRepo
	hasher   ports.PasswordHasher
	sessionTTL time.Duration
}

func NewAuth(users ports.UserRepo, sessions ports.SessionRepo, hasher ports.PasswordHasher) *Auth {
	return &Auth{users: users, sessions: sessions, hasher: hasher, sessionTTL: 7 * 24 * time.Hour}
}

// Login validates credentials and creates a session token. Constant-time compare.
func (a *Auth) Login(ctx context.Context, login, password string) (string, error) {
	u, err := a.users.Get(ctx)
	if err != nil {
		return "", err
	}
	// ponytail: timing — bcrypt compare is constant-time; we also avoid
	// short-circuiting on the login mismatch by always doing the hash compare.
	loginOK := subtle.ConstantTimeCompare([]byte(u.Login), []byte(login)) == 1
	if err := a.hasher.Verify(u.PasswordHash, password); err != nil {
		return "", domain.ErrUnauthorized
	}
	if !loginOK {
		return "", domain.ErrUnauthorized
	}
	return a.createSession(ctx, u.ID)
}

func (a *Auth) createSession(ctx context.Context, userID domain.ID) (string, error) {
	tok := randomToken(32)
	now := time.Now().UTC()
	s := &domain.Session{
		Token: tok, UserID: userID,
		CreatedAt: now, ExpiresAt: now.Add(a.sessionTTL),
	}
	if err := a.sessions.Create(ctx, s); err != nil {
		return "", err
	}
	return tok, nil
}

// User returns the single admin user (for the /me endpoint).
func (a *Auth) User(ctx context.Context) (*domain.User, error) {
	return a.users.Get(ctx)
}

// VerifySession returns the user id for a live session token, or ErrUnauthorized.
func (a *Auth) VerifySession(ctx context.Context, token string) (domain.ID, error) {
	if token == "" {
		return "", domain.ErrUnauthorized
	}
	s, err := a.sessions.Get(ctx, token)
	if err != nil || s == nil {
		return "", domain.ErrUnauthorized
	}
	if time.Now().UTC().After(s.ExpiresAt) {
		_ = a.sessions.Delete(ctx, token)
		return "", domain.ErrUnauthorized
	}
	return s.UserID, nil
}

// Logout revokes a session.
func (a *Auth) Logout(ctx context.Context, token string) error {
	return a.sessions.Delete(ctx, token)
}

// ChangeCredentials updates login and/or password. password=="" keeps the old one.
// ChangeCredentials updates login and/or password AFTER verifying the current
// password (§4.12 security: credential changes must be re-authenticated, not
// just gated by a live session — a hijacked session shouldn't let an attacker
// silently rotate credentials). Pass newPassword="" to change only the login.
func (a *Auth) ChangeCredentials(ctx context.Context, currentPassword, newLogin, newPassword string) error {
	u, err := a.users.Get(ctx)
	if err != nil {
		return err
	}
	// Re-authenticate: the caller MUST prove knowledge of the current password.
	if err := a.hasher.Verify(u.PasswordHash, currentPassword); err != nil {
		return domain.ErrUnauthorized
	}
	if newLogin != "" {
		if len(newLogin) < 3 {
			return errors.New("login must be at least 3 characters")
		}
		u.Login = newLogin
	}
	if newPassword != "" {
		hash, err := a.hasher.Hash(newPassword)
		if err != nil {
			return err
		}
		u.PasswordHash = hash
	}
	return a.users.Update(ctx, u)
}

// SeedAdmin inserts the initial admin if none exists. Called once at startup.
func (a *Auth) SeedAdmin(ctx context.Context, login, password string) error {
	hash, err := a.hasher.Hash(password)
	if err != nil {
		return err
	}
	return a.users.Seed(ctx, login, hash)
}

// randomToken returns n bytes as hex (2n chars).
func randomToken(n int) string {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		panic("crypto/rand failed: " + err.Error())
	}
	return hex.EncodeToString(b)
}
