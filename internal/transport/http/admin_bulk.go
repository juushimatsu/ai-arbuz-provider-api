package httptrans

import (
	"encoding/json"
	"net/http"
	"sort"

	"github.com/arbuz/ai-arbuz-provider-api/internal/domain"
)

// checkAllUpstream runs the selected probe(s) against every upstream key (or
// every key of one provider when provider_id is set) and returns per-key
// results. probes is any subset of {"models","chat"}; defaults to both.
func (s *Server) checkAllUpstream(w http.ResponseWriter, r *http.Request) {
	var b struct {
		ProviderID string                 `json:"provider_id"`
		Probes     []domain.CheckerProbe  `json:"probes"`
	}
	_ = json.NewDecoder(r.Body).Decode(&b)
	if len(b.Probes) == 0 {
		b.Probes = []domain.CheckerProbe{domain.ProbeModels, domain.ProbeChat}
	}

	keys, err := s.upstreams.List(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	type keyResult struct {
		ID      string                 `json:"id"`
		Name    string                 `json:"name"`
		Results []domain.CheckerResult `json:"results"`
	}
	out := make([]keyResult, 0, len(keys))
	for _, k := range keys {
		if b.ProviderID != "" && string(k.ProviderID) != b.ProviderID {
			continue
		}
		kr := keyResult{ID: string(k.ID), Name: k.Name}
		secretBytes, err := s.secrets.Decrypt(r.Context(), k.SecretEnc)
		if err != nil {
			kr.Results = append(kr.Results, domain.CheckerResult{Kind: domain.CheckerProbe("decrypt"), Status: domain.StatusDisabled, Error: err.Error()})
			out = append(out, kr)
			continue
		}
		secret := string(secretBytes)
		for _, probe := range b.Probes {
			kr.Results = append(kr.Results, s.runProbe(r.Context(), k.BaseURL, secret, k.Format, probe))
		}
		out = append(out, kr)
	}
	writeJSON(w, http.StatusOK, map[string]any{"keys": out})
}

// searchProviderModels aggregates the model lists reported by every upstream
// key belonging to a provider (deduped, sorted). Mirrors the per-key
// auto-search but at the provider level so GLOBAL_MODELS can be filled in.
func (s *Server) searchProviderModels(w http.ResponseWriter, r *http.Request) {
	keys, err := s.upstreams.ListByProvider(r.Context(), r.PathValue("id"))
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	seen := map[string]bool{}
	models := []string{}
	var lastErr string
	for _, k := range keys {
		secretBytes, err := s.secrets.Decrypt(r.Context(), k.SecretEnc)
		if err != nil {
			lastErr = err.Error()
			continue
		}
		ms, err := s.modelSearch.ListModels(r.Context(), k.BaseURL, string(secretBytes), k.Format)
		if err != nil {
			lastErr = err.Error()
			continue
		}
		for _, m := range ms {
			if !seen[m] {
				seen[m] = true
				models = append(models, m)
			}
		}
	}
	sort.Strings(models)
	resp := map[string]any{"models": models}
	if len(models) == 0 && lastErr != "" {
		resp["error"] = lastErr
	}
	writeJSON(w, http.StatusOK, resp)
}
