package httptrans

import (
	"encoding/hex"
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"github.com/arbuz/ai-arbuz-provider-api/internal/domain"
	"github.com/arbuz/ai-arbuz-provider-api/internal/ports"
)

func hexEncode(b []byte) string { return hex.EncodeToString(b) }

// --- Logs (§4.11) ---

func (s *Server) listLogs(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	f := ports.LogFilter{
		IssuedKeyID: q.Get("issued_key_id"),
		ProviderID:  q.Get("provider_id"),
		Model:       q.Get("model"),
	}
	if v := q.Get("success"); v != "" {
		b := v == "1" || v == "true"
		f.Success = &b
	}
	if v := q.Get("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			f.Limit = n
		}
	}
	if v := q.Get("offset"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			f.Offset = n
		}
	}
	if v := q.Get("from"); v != "" {
		if t, err := time.Parse(time.RFC3339, v); err == nil {
			f.From = t
		}
	}
	if v := q.Get("to"); v != "" {
		if t, err := time.Parse(time.RFC3339, v); err == nil {
			f.To = t
		}
	}
	list, err := s.logs.List(r.Context(), f)
	writeList(w, list, err)
}

func (s *Server) getLog(w http.ResponseWriter, r *http.Request) {
	l, err := s.logs.Get(r.Context(), r.PathValue("id"))
	writeOne(w, l, err)
}

// --- Stats (§4.11 dashboard) ---

func parseStatsQuery(r *http.Request) ports.StatsQuery {
	q := ports.StatsQuery{BucketSeconds: 3600}
	if v := r.URL.Query().Get("from"); v != "" {
		if t, err := time.Parse(time.RFC3339, v); err == nil {
			q.From = t
		}
	}
	if v := r.URL.Query().Get("to"); v != "" {
		if t, err := time.Parse(time.RFC3339, v); err == nil {
			q.To = t
		}
	}
	if v := r.URL.Query().Get("bucket_seconds"); v != "" {
		if n, err := strconv.ParseInt(v, 10, 64); err == nil {
			q.BucketSeconds = n
		}
	}
	if q.From.IsZero() {
		q.From = time.Now().UTC().Add(-24 * time.Hour)
	}
	if q.To.IsZero() {
		q.To = time.Now().UTC()
	}
	return q
}

func (s *Server) statsSummary(w http.ResponseWriter, r *http.Request) {
	if s.stats == nil {
		writeError(w, http.StatusServiceUnavailable, "stats disabled")
		return
	}
	sum, err := s.stats.Summary(r.Context(), parseStatsQuery(r))
	writeOne(w, &sum, err)
}

func (s *Server) statsSeries(w http.ResponseWriter, r *http.Request) {
	if s.stats == nil {
		writeError(w, http.StatusServiceUnavailable, "stats disabled")
		return
	}
	pts, err := s.stats.Series(r.Context(), parseStatsQuery(r))
	writeList(w, pts, err)
}

func (s *Server) statsBreakdown(w http.ResponseWriter, r *http.Request) {
	if s.stats == nil {
		writeError(w, http.StatusServiceUnavailable, "stats disabled")
		return
	}
	dim := r.URL.Query().Get("dimension")
	if dim == "" {
		dim = "model"
	}
	buckets, err := s.stats.Breakdown(r.Context(), parseStatsQuery(r), dim)
	writeList(w, buckets, err)
}

// --- Model auto-search (§4.9) ---

type modelSearchBody struct {
	BaseURL string `json:"base_url"`
	Secret  string `json:"secret"`
	Format  string `json:"format"`
}

func (s *Server) searchModels(w http.ResponseWriter, r *http.Request) {
	var b modelSearchBody
	if err := json.NewDecoder(r.Body).Decode(&b); err != nil {
		writeError(w, http.StatusBadRequest, "invalid body")
		return
	}
	if b.BaseURL == "" || b.Secret == "" {
		writeError(w, http.StatusBadRequest, "base_url and secret required")
		return
	}
	format := domain.FormatOpenAI
	if b.Format == string(domain.FormatAnthropic) {
		format = domain.FormatAnthropic
	}
	if s.modelSearch == nil {
		writeError(w, http.StatusServiceUnavailable, "model search disabled")
		return
	}
	models, err := s.modelSearch.ListModels(r.Context(), b.BaseURL, b.Secret, format)
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"models": models})
}
