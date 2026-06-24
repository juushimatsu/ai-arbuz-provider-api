package httptrans

import (
	"encoding/json"
	"net/http"

	"github.com/arbuz/ai-arbuz-provider-api/internal/domain"
)

// prompt-rule admin CRUD (§4.7).

type promptRuleBody struct {
	Name   string `json:"name"`
	Kind   string `json:"kind"`
	Value  string `json:"value"`
	Param  string `json:"param"`
	Status string `json:"status"`
}

func (s *Server) listPromptRules(w http.ResponseWriter, r *http.Request) {
	list, err := s.promptRules.List(r.Context())
	writeList(w, list, err)
}

func (s *Server) createPromptRule(w http.ResponseWriter, r *http.Request) {
	var b promptRuleBody
	if err := json.NewDecoder(r.Body).Decode(&b); err != nil {
		writeError(w, http.StatusBadRequest, "invalid body")
		return
	}
	rule := &domain.PromptRule{
		Name: b.Name, Kind: b.Kind, Value: b.Value, Param: b.Param, Status: domain.Status(b.Status),
	}
	if err := s.promptRules.Create(r.Context(), rule); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, rule)
}

func (s *Server) updatePromptRule(w http.ResponseWriter, r *http.Request) {
	var b promptRuleBody
	if err := json.NewDecoder(r.Body).Decode(&b); err != nil {
		writeError(w, http.StatusBadRequest, "invalid body")
		return
	}
	rule := &domain.PromptRule{
		ID: r.PathValue("id"), Name: b.Name, Kind: b.Kind, Value: b.Value,
		Param: b.Param, Status: domain.Status(b.Status),
	}
	if err := s.promptRules.Update(r.Context(), rule); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, rule)
}

func (s *Server) deletePromptRule(w http.ResponseWriter, r *http.Request) {
	if err := s.promptRules.Delete(r.Context(), r.PathValue("id")); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}
