package httptrans

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/arbuz/ai-arbuz-provider-api/internal/domain"
)

// listModelPrefs returns the per-provider "model -> preferred upstream key" map.
func (s *Server) listModelPrefs(w http.ResponseWriter, r *http.Request) {
	if s.modelPrefs == nil {
		writeJSON(w, http.StatusOK, []domain.ModelPreference{})
		return
	}
	list, err := s.modelPrefs.ListByProvider(r.Context(), r.PathValue("id"))
	writeList(w, list, err)
}

type modelPrefBody struct {
	Model         string `json:"model"`
	UpstreamKeyID string `json:"upstream_key_id"`
}

// setModelPref upserts the preferred key for a model. Validates that the key
// belongs to the provider and actually serves the model.
func (s *Server) setModelPref(w http.ResponseWriter, r *http.Request) {
	if s.modelPrefs == nil {
		writeError(w, http.StatusServiceUnavailable, "model preferences unavailable")
		return
	}
	providerID := r.PathValue("id")
	var b modelPrefBody
	if err := json.NewDecoder(r.Body).Decode(&b); err != nil || b.Model == "" || b.UpstreamKeyID == "" {
		writeError(w, http.StatusBadRequest, "model and upstream_key_id are required")
		return
	}
	provider, err := s.providers.Get(r.Context(), providerID)
	if err != nil {
		writeError(w, http.StatusNotFound, "provider not found")
		return
	}
	keys, err := s.upstreams.ListByProvider(r.Context(), providerID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	var match *domain.UpstreamKey
	for i := range keys {
		if keys[i].ID == b.UpstreamKeyID {
			match = &keys[i]
			break
		}
	}
	if match == nil {
		writeError(w, http.StatusBadRequest, "upstream key does not belong to this provider")
		return
	}
	if !containsStr(match.EffectiveModels(provider.GlobalModels), b.Model) {
		writeError(w, http.StatusBadRequest, "selected key does not serve this model")
		return
	}
	pref := domain.ModelPreference{ProviderID: providerID, Model: b.Model, UpstreamKeyID: b.UpstreamKeyID}
	if err := s.modelPrefs.Set(r.Context(), pref); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, pref)
}

// deleteModelPref clears the preference for a model (routing reverts to
// priority-based selection across all usable keys).
func (s *Server) deleteModelPref(w http.ResponseWriter, r *http.Request) {
	if s.modelPrefs == nil {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	if err := s.modelPrefs.Delete(r.Context(), r.PathValue("id"), r.PathValue("model")); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// modelAvailability reports, per model, the live keys that actually serve it.
type modelAvailability struct {
	Model           string   `json:"model"`
	Available       bool     `json:"available"`        // at least one usable key serves it
	Keys            []string `json:"keys"`             // usable keys serving the model
	AllKeys         []string `json:"all_keys"`         // every key serving it (incl. unusable)
	PreferredKeyID  string   `json:"preferred_key_id"` // configured preferred key id, if any
}

// providerModels computes REAL model availability from the live upstream keys,
// not just provider.GlobalModels. A model can appear "configured" yet be
// unavailable if no usable key serves it (e.g. after deleting a key).
func (s *Server) providerModels(w http.ResponseWriter, r *http.Request) {
	providerID := r.PathValue("id")
	provider, err := s.providers.Get(r.Context(), providerID)
	if err != nil {
		writeError(w, http.StatusNotFound, "provider not found")
		return
	}
	keys, err := s.upstreams.ListByProvider(r.Context(), providerID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	now := time.Now().UTC()
	// Collect the universe of models: provider globals + each key's explicit list.
	modelsSet := map[string]bool{}
	for _, m := range provider.GlobalModels {
		modelsSet[m] = true
	}
	for _, k := range keys {
		for _, m := range k.EffectiveModels(provider.GlobalModels) {
			modelsSet[m] = true
		}
	}
	prefByModel := map[string]string{}
	if s.modelPrefs != nil {
		if prefs, e := s.modelPrefs.ListByProvider(r.Context(), providerID); e == nil {
			for _, p := range prefs {
				prefByModel[p.Model] = p.UpstreamKeyID
			}
		}
	}
	out := make([]modelAvailability, 0, len(modelsSet))
	for m := range modelsSet {
		row := modelAvailability{Model: m, PreferredKeyID: prefByModel[m], Keys: []string{}, AllKeys: []string{}}
		for _, k := range keys {
			if !containsStr(k.EffectiveModels(provider.GlobalModels), m) {
				continue
			}
			row.AllKeys = append(row.AllKeys, k.ID)
			if k.Usable(now) {
				row.Keys = append(row.Keys, k.ID)
			}
		}
		row.Available = len(row.Keys) > 0
		out = append(out, row)
	}
	writeJSON(w, http.StatusOK, out)
}
func containsStr(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}