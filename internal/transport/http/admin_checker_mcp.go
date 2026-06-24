package httptrans

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/arbuz/ai-arbuz-provider-api/internal/domain"
	"github.com/arbuz/ai-arbuz-provider-api/internal/ports"
)

// --- API Checker (§4.10) ---

type checkerBody struct {
	BaseURL string `json:"base_url"`
	Secret  string `json:"secret"`
	Format  string `json:"format"`
	Probes  []string `json:"probes"` // subset of ping|models|chat|embeddings
}

func (s *Server) runChecker(w http.ResponseWriter, r *http.Request) {
	var b checkerBody
	if err := json.NewDecoder(r.Body).Decode(&b); err != nil {
		writeError(w, http.StatusBadRequest, "invalid body")
		return
	}
	if b.BaseURL == "" || b.Secret == "" {
		writeError(w, http.StatusBadRequest, "base_url and secret required")
		return
	}
	if s.checker == nil {
		writeError(w, http.StatusServiceUnavailable, "checker disabled")
		return
	}
	format := domain.FormatOpenAI
	if b.Format == string(domain.FormatAnthropic) {
		format = domain.FormatAnthropic
	}
	if len(b.Probes) == 0 {
		b.Probes = []string{"ping", "models", "chat", "embeddings"}
	}

	run := &domain.CheckerRun{
		BaseURL: b.BaseURL, SecretTail: tailString(b.Secret), StartedAt: time.Now().UTC(),
	}
	for _, p := range b.Probes {
		run.Results = append(run.Results, s.runProbe(r.Context(), b.BaseURL, b.Secret, format, domain.CheckerProbe(p)))
	}
	if err := s.checkerRepo.Insert(r.Context(), run); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, run)
}

func (s *Server) runProbe(ctx context.Context, baseURL, secret string, format domain.Format, probe domain.CheckerProbe) domain.CheckerResult {
	start := time.Now()
	res := domain.CheckerResult{Kind: probe}
	switch probe {
	case domain.ProbePing:
		resp, err := s.checker.Do(ctx, baseURL, secret, ports.UpstreamRequest{
			Method: http.MethodGet, Path: "/v1/models", Format: format,
		})
		res.LatencyMs = time.Since(start).Milliseconds()
		if err != nil {
			res.Status = domain.StatusDisabled
			res.Error = err.Error()
			return res
		}
		res.HTTPCode = resp.StatusCode
		res.Status = statusFromCode(resp.StatusCode)
	case domain.ProbeModels:
		_, err := s.checker.ListModels(ctx, baseURL, secret, format)
		res.LatencyMs = time.Since(start).Milliseconds()
		if err != nil {
			res.Status = domain.StatusDisabled
			res.Error = err.Error()
			return res
		}
		res.Status = domain.StatusActive
	case domain.ProbeChat:
		body := []byte(`{"model":"gpt-3.5-turbo","messages":[{"role":"user","content":"ping"}],"max_tokens":1}`)
		resp, err := s.checker.Do(ctx, baseURL, secret, ports.UpstreamRequest{
			Method: http.MethodPost, Path: "/v1/chat/completions", Format: format, Body: body,
		})
		res.LatencyMs = time.Since(start).Milliseconds()
		if err != nil {
			res.Status = domain.StatusDisabled
			res.Error = err.Error()
			return res
		}
		res.HTTPCode = resp.StatusCode
		res.Status = statusFromCode(resp.StatusCode)
	case domain.ProbeEmbeddings:
		body := []byte(`{"model":"text-embedding-ada-002","input":"ping"}`)
		resp, err := s.checker.Do(ctx, baseURL, secret, ports.UpstreamRequest{
			Method: http.MethodPost, Path: "/v1/embeddings", Format: format, Body: body,
		})
		res.LatencyMs = time.Since(start).Milliseconds()
		if err != nil {
			res.Status = domain.StatusDisabled
			res.Error = err.Error()
			return res
		}
		res.HTTPCode = resp.StatusCode
		res.Status = statusFromCode(resp.StatusCode)
	default:
		res.Status = domain.StatusDisabled
		res.Error = fmt.Sprintf("unknown probe: %s", probe)
	}
	return res
}

func statusFromCode(code int) domain.Status {
	if code >= 200 && code < 300 {
		return domain.StatusActive
	}
	return domain.StatusDisabled
}

func (s *Server) listCheckerRuns(w http.ResponseWriter, r *http.Request) {
	limit := 50
	runs, err := s.checkerRepo.List(r.Context(), limit)
	writeList(w, runs, err)
}

func (s *Server) getCheckerRun(w http.ResponseWriter, r *http.Request) {
	run, err := s.checkerRepo.Get(r.Context(), r.PathValue("id"))
	writeOne(w, run, err)
}

// --- MCP servers (§4.8) ---

type mcpBody struct {
	Name      string `json:"name"`
	Kind      string `json:"kind"`
	Transport string `json:"transport"`
	Address   string `json:"address"`
	Status    string `json:"status"`
}

func (s *Server) listMCP(w http.ResponseWriter, r *http.Request) {
	list, err := s.mcpRepo.List(r.Context())
	writeList(w, list, err)
}

func (s *Server) createMCP(w http.ResponseWriter, r *http.Request) {
	var b mcpBody
	if err := json.NewDecoder(r.Body).Decode(&b); err != nil {
		writeError(w, http.StatusBadRequest, "invalid body")
		return
	}
	m := &domain.MCPServer{
		Name: b.Name, Kind: domain.MCPKind(b.Kind), Transport: b.Transport,
		Address: b.Address, Status: domain.Status(b.Status),
	}
	if err := s.mcpRepo.Create(r.Context(), m); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, m)
}

func (s *Server) updateMCP(w http.ResponseWriter, r *http.Request) {
	var b mcpBody
	if err := json.NewDecoder(r.Body).Decode(&b); err != nil {
		writeError(w, http.StatusBadRequest, "invalid body")
		return
	}
	m, err := s.mcpRepo.Get(r.Context(), r.PathValue("id"))
	if err != nil {
		writeError(w, http.StatusNotFound, "not found")
		return
	}
	m.Name = b.Name
	if b.Kind != "" {
		m.Kind = domain.MCPKind(b.Kind)
	}
	if b.Transport != "" {
		m.Transport = b.Transport
	}
	m.Address = b.Address
	if b.Status != "" {
		m.Status = domain.Status(b.Status)
	}
	if err := s.mcpRepo.Update(r.Context(), m); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, m)
}

func (s *Server) deleteMCP(w http.ResponseWriter, r *http.Request) {
	if err := s.mcpRepo.Delete(r.Context(), r.PathValue("id")); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}

// tailString masks a plaintext secret to its last 4 chars (for log display).
func tailString(s string) string {
	if len(s) <= 4 {
		return "****"
	}
	return "…" + s[len(s)-4:]
}

// --- MCP live actions (§4.8) ---

// discoverMCPTools connects to a saved MCP server, lists its tools, and
// persists the discovered tool list onto the MCPServer record.
func (s *Server) discoverMCPTools(w http.ResponseWriter, r *http.Request) {
	if s.mcp == nil {
		writeError(w, http.StatusServiceUnavailable, "mcp disabled")
		return
	}
	m, err := s.mcpRepo.Get(r.Context(), r.PathValue("id"))
	if err != nil {
		writeError(w, http.StatusNotFound, "not found")
		return
	}
	tools, err := s.mcp.ListTools(r.Context(), ports.MCPEndpoint{Transport: m.Transport, Address: m.Address})
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	m.Tools = make([]domain.MCPTool, 0, len(tools))
	for _, t := range tools {
		m.Tools = append(m.Tools, domain.MCPTool{Name: t.Name, Description: t.Description, InputSchema: string(t.InputSchema)})
	}
	_ = s.mcpRepo.Update(r.Context(), m)
	writeJSON(w, http.StatusOK, map[string]any{"tools": m.Tools})
}

// callMCPTool invokes one tool on a saved MCP server.
type mcpCallBody struct {
	Name      string          `json:"name"`
	Arguments json.RawMessage `json:"arguments"`
}

func (s *Server) callMCPTool(w http.ResponseWriter, r *http.Request) {
	if s.mcp == nil {
		writeError(w, http.StatusServiceUnavailable, "mcp disabled")
		return
	}
	m, err := s.mcpRepo.Get(r.Context(), r.PathValue("id"))
	if err != nil {
		writeError(w, http.StatusNotFound, "not found")
		return
	}
	var b mcpCallBody
	if err := json.NewDecoder(r.Body).Decode(&b); err != nil {
		writeError(w, http.StatusBadRequest, "invalid body")
		return
	}
	if b.Name == "" {
		writeError(w, http.StatusBadRequest, "name required")
		return
	}
	res, err := s.mcp.CallTool(r.Context(), ports.MCPEndpoint{Transport: m.Transport, Address: m.Address}, b.Name, b.Arguments)
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, res)
}
