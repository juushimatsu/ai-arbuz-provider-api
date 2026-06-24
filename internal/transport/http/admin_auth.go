package httptrans

import (
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/arbuz/ai-arbuz-provider-api/internal/domain"
)

const sessionCookieName = "arbuz_session"
const sessionMaxAge = 7 * 24 * 3600 // 7 days, matches Auth.sessionTTL

type loginRequest struct {
	Login    string `json:"login"`
	Password string `json:"password"`
}

func (s *Server) handleLogin(w http.ResponseWriter, r *http.Request) {
	var in loginRequest
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeError(w, http.StatusBadRequest, "invalid body")
		return
	}
	tok, err := s.auth.Login(r.Context(), in.Login, in.Password)
	if err != nil {
		writeError(w, http.StatusUnauthorized, "invalid credentials")
		return
	}
	http.SetCookie(w, &http.Cookie{
		Name: sessionCookieName, Value: tok, Path: "/", HttpOnly: true,
		SameSite: http.SameSiteLaxMode, MaxAge: sessionMaxAge, Secure: isHTTPS(r),
	})
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// isHTTPS reports whether the request is HTTPS, either directly (r.TLS) or via
// a trusted reverse proxy setting X-Forwarded-Proto=https. Nginx fronts this
// service in prod (DEPLOYMENT.md), so r.TLS is always nil there — we must
// honour the forwarded header or the session cookie is never marked Secure.
func isHTTPS(r *http.Request) bool {
	if r.TLS != nil {
		return true
	}
	// ponytail: we trust X-Forwarded-Proto only because Nginx terminates TLS and
	// sets it; if you expose the Go listener directly, this still falls back to
	// r.TLS correctly.
	return r.Header.Get("X-Forwarded-Proto") == "https"
}

func (s *Server) handleLogout(w http.ResponseWriter, r *http.Request) {
	if c, err := r.Cookie(sessionCookieName); err == nil {
		_ = s.auth.Logout(r.Context(), c.Value)
	}
	http.SetCookie(w, &http.Cookie{Name: sessionCookieName, Value: "", Path: "/", MaxAge: -1})
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *Server) handleMe(w http.ResponseWriter, r *http.Request) {
	u, err := s.auth.User(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "no user")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"login": u.Login})
}

type credentialsRequest struct {
	CurrentPassword string `json:"current_password"`
	Login           string `json:"login"`
	NewPassword     string `json:"new_password"`
}

func (s *Server) handleChangeCredentials(w http.ResponseWriter, r *http.Request) {
	var in credentialsRequest
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeError(w, http.StatusBadRequest, "invalid body")
		return
	}
	if in.CurrentPassword == "" {
		writeError(w, http.StatusBadRequest, "current_password required")
		return
	}
	err := s.auth.ChangeCredentials(r.Context(), in.CurrentPassword, in.Login, in.NewPassword)
	switch {
	case err == nil:
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	case errors.Is(err, domain.ErrUnauthorized):
		// Wrong current password → 401 so the UI shows a clear error.
		writeError(w, http.StatusUnauthorized, "current password incorrect")
	default:
		writeError(w, http.StatusBadRequest, err.Error())
	}
}

func (s *Server) health(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"status": "ok", "time": time.Now().UTC().Format(time.RFC3339),
		"guard": s.proxy.GuardMode(),
	})
}