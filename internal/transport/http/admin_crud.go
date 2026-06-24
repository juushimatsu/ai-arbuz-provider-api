package httptrans

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/arbuz/ai-arbuz-provider-api/internal/domain"
	"github.com/arbuz/ai-arbuz-provider-api/internal/usecase"
)

// --- Providers (§4.2) ---

type providerBody struct {
	Name           string   `json:"name"`
	Strategy       string   `json:"strategy"`
	GlobalModels   []string `json:"global_models"`
	FallbackModels []string `json:"fallback_models"`
	Status         string   `json:"status"`
}

func (s *Server) listProviders(w http.ResponseWriter, r *http.Request) {
	list, err := s.providers.List(r.Context())
	writeList(w, list, err)
}

func (s *Server) createProvider(w http.ResponseWriter, r *http.Request) {
	var b providerBody
	if err := json.NewDecoder(r.Body).Decode(&b); err != nil {
		writeError(w, http.StatusBadRequest, "invalid body")
		return
	}
	p := &domain.Provider{
		Name: b.Name, Strategy: domain.RoutingStrategy(b.Strategy),
		GlobalModels: b.GlobalModels, FallbackModels: b.FallbackModels, Status: domain.Status(b.Status),
	}
	if err := s.providers.Create(r.Context(), p); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, p)
}

func (s *Server) getProvider(w http.ResponseWriter, r *http.Request) {
	p, err := s.providers.Get(r.Context(), r.PathValue("id"))
	writeOne(w, p, err)
}

func (s *Server) updateProvider(w http.ResponseWriter, r *http.Request) {
	var b providerBody
	if err := json.NewDecoder(r.Body).Decode(&b); err != nil {
		writeError(w, http.StatusBadRequest, "invalid body")
		return
	}
	p, err := s.providers.Get(r.Context(), r.PathValue("id"))
	if err != nil {
		writeError(w, http.StatusNotFound, "not found")
		return
	}
	p.Name = b.Name
	if b.Strategy != "" {
		p.Strategy = domain.RoutingStrategy(b.Strategy)
	}
	p.GlobalModels = b.GlobalModels
	p.FallbackModels = b.FallbackModels
	if b.Status != "" {
		p.Status = domain.Status(b.Status)
	}
	if err := s.providers.Update(r.Context(), p); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, p)
}

func (s *Server) deleteProvider(w http.ResponseWriter, r *http.Request) {
	if err := s.providers.Delete(r.Context(), r.PathValue("id")); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}

// --- Upstreams (§4.1) ---

type upstreamBody struct {
	Name            string            `json:"name"`
	ProviderID      string            `json:"provider_id"`
	BaseURL         string            `json:"base_url"`
	Format          string            `json:"format"`
	PlaintextSecret string            `json:"secret"`
	Models          []string          `json:"models"`
	UseGlobalModels bool              `json:"use_global_models"`
	Priority        int               `json:"priority"`
	Status          string            `json:"status"`
	UpstreamLimits  limitsBody        `json:"upstream_limits"`
}

type limitsBody struct {
	Tokens   map[string]int64 `json:"tokens"`
	Requests map[string]int64 `json:"requests"`
}

func toDomainLimits(b limitsBody) domain.Limits {
	l := domain.NewLimits()
	for k, v := range b.Tokens {
		l.Tokens[domain.LimitWindow(k)] = v
	}
	for k, v := range b.Requests {
		l.Requests[domain.LimitWindow(k)] = v
	}
	return l
}

func fromDomainLimits(l domain.Limits) limitsBody {
	out := limitsBody{Tokens: map[string]int64{}, Requests: map[string]int64{}}
	for k, v := range l.Tokens {
		out.Tokens[string(k)] = v
	}
	for k, v := range l.Requests {
		out.Requests[string(k)] = v
	}
	return out
}

func (s *Server) listUpstreams(w http.ResponseWriter, r *http.Request) {
	if pid := r.URL.Query().Get("provider_id"); pid != "" {
		list, err := s.upstreams.ListByProvider(r.Context(), pid)
		writeList(w, maskedUpstreams(list), err)
		return
	}
	list, err := s.upstreams.List(r.Context())
	writeList(w, maskedUpstreams(list), err)
}

// maskedUpstreams strips the encrypted secret blob before sending to the UI.
// Secrets are revealed explicitly via a dedicated endpoint if ever needed.
func maskedUpstreams(list []domain.UpstreamKey) []map[string]any {
	out := make([]map[string]any, 0, len(list))
	for _, k := range list {
		m := map[string]any{
			"id": k.ID, "provider_id": k.ProviderID, "name": k.Name,
			"base_url": k.BaseURL, "format": k.Format, "models": k.Models,
			"use_global_models": k.UseGlobalModels, "priority": k.Priority,
			"status": k.Status, "secret_tail": tail(k.SecretEnc),
			"upstream_limits": fromDomainLimits(k.UpstreamLimits),
			"cooldown_until": k.Health.CooldownUntil,
			"consecutive_failures": k.Health.ConsecutiveFailures,
			"created_at": k.CreatedAt, "updated_at": k.UpdatedAt,
		}
		out = append(out, m)
	}
	return out
}

func (s *Server) createUpstream(w http.ResponseWriter, r *http.Request) {
	var b upstreamBody
	if err := json.NewDecoder(r.Body).Decode(&b); err != nil {
		writeError(w, http.StatusBadRequest, "invalid body")
		return
	}
	in := usecase.UpstreamInput{
		Name: b.Name, ProviderID: b.ProviderID, BaseURL: b.BaseURL,
		Format: domain.Format(b.Format), PlaintextSecret: b.PlaintextSecret,
		Models: b.Models, UseGlobalModels: b.UseGlobalModels, Priority: b.Priority,
		Status: domain.Status(b.Status), UpstreamLimits: toDomainLimits(b.UpstreamLimits),
	}
	k, err := s.upstreams.Create(r.Context(), in)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"id": k.ID, "name": k.Name, "secret_tail": tail(k.SecretEnc)})
}

func (s *Server) getUpstream(w http.ResponseWriter, r *http.Request) {
	k, err := s.upstreams.Get(r.Context(), r.PathValue("id"))
	if err != nil {
		writeError(w, http.StatusNotFound, "not found")
		return
	}
	writeJSON(w, http.StatusOK, maskedUpstreams([]domain.UpstreamKey{*k})[0])
}

func (s *Server) updateUpstream(w http.ResponseWriter, r *http.Request) {
	var b upstreamBody
	if err := json.NewDecoder(r.Body).Decode(&b); err != nil {
		writeError(w, http.StatusBadRequest, "invalid body")
		return
	}
	in := usecase.UpstreamInput{
		Name: b.Name, ProviderID: b.ProviderID, BaseURL: b.BaseURL,
		Format: domain.Format(b.Format), PlaintextSecret: b.PlaintextSecret,
		Models: b.Models, UseGlobalModels: b.UseGlobalModels, Priority: b.Priority,
		Status: domain.Status(b.Status), UpstreamLimits: toDomainLimits(b.UpstreamLimits),
	}
	if err := s.upstreams.Update(r.Context(), r.PathValue("id"), in); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "updated"})
}

func (s *Server) deleteUpstream(w http.ResponseWriter, r *http.Request) {
	if err := s.upstreams.Delete(r.Context(), r.PathValue("id")); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}

// --- Issued keys (§4.3) ---

type issuedBody struct {
	Name      string     `json:"name"`
	ProviderID string    `json:"provider_id"`
	Limits    limitsBody `json:"limits"`
	ValidDays int        `json:"valid_days"`
	Status    string     `json:"status"`
}

func (s *Server) listIssued(w http.ResponseWriter, r *http.Request) {
	if pid := r.URL.Query().Get("provider_id"); pid != "" {
		list, err := s.issued.ListByProvider(r.Context(), pid)
		writeList(w, list, err)
		return
	}
	list, err := s.issued.List(r.Context())
	writeList(w, list, err)
}

func (s *Server) createIssued(w http.ResponseWriter, r *http.Request) {
	var b issuedBody
	if err := json.NewDecoder(r.Body).Decode(&b); err != nil {
		writeError(w, http.StatusBadRequest, "invalid body")
		return
	}
	in := usecase.IssuedInput{
		Name: b.Name, ProviderID: b.ProviderID,
		Limits: toDomainLimits(b.Limits), ValidDays: b.ValidDays,
	}
	k, token, err := s.issued.Create(r.Context(), in)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	// token is shown ONCE here; never retrievable again.
	writeJSON(w, http.StatusCreated, map[string]any{
		"id": k.ID, "name": k.Name, "token": token, "token_shown_once": true,
		"created_at": k.CreatedAt, "expires_at": k.ExpiresAt,
	})
}

func (s *Server) getIssued(w http.ResponseWriter, r *http.Request) {
	k, err := s.issued.Get(r.Context(), r.PathValue("id"))
	writeOne(w, k, err)
}

func (s *Server) updateIssued(w http.ResponseWriter, r *http.Request) {
	var b issuedBody
	if err := json.NewDecoder(r.Body).Decode(&b); err != nil {
		writeError(w, http.StatusBadRequest, "invalid body")
		return
	}
	if err := s.issued.Update(r.Context(), r.PathValue("id"), b.Name, toDomainLimits(b.Limits), b.ValidDays, domain.Status(b.Status)); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "updated"})
}

func (s *Server) revokeIssued(w http.ResponseWriter, r *http.Request) {
	if err := s.issued.Revoke(r.Context(), r.PathValue("id")); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "revoked"})
}

func (s *Server) deleteIssued(w http.ResponseWriter, r *http.Request) {
	if err := s.issued.Delete(r.Context(), r.PathValue("id")); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}

// --- response helpers ---

func writeList[T any](w http.ResponseWriter, list []T, err error) {
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if list == nil {
		list = []T{}
	}
	writeJSON(w, http.StatusOK, list)
}

func writeOne[T any](w http.ResponseWriter, v *T, err error) {
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			writeError(w, http.StatusNotFound, "not found")
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, v)
}

// tail returns a masked tail of a secret (last 4 chars of the decoded value,
// or of the ciphertext hex if we don't want to decrypt). ponytail: we mask the
// ciphertext length here; the UI only needs to distinguish keys.
func tail(secret []byte) string {
	if len(secret) == 0 {
		return ""
	}
	h := hexEncode(secret)
	if len(h) <= 4 {
		return "…" + h
	}
	return "…" + h[len(h)-4:]
}
