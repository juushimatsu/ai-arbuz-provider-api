package httptrans

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/arbuz/ai-arbuz-provider-api/internal/adapter/mcp"
	"github.com/arbuz/ai-arbuz-provider-api/internal/ports"
	"github.com/arbuz/ai-arbuz-provider-api/internal/usecase"
)

// buildMCPServer constructs the router's own MCP server (§4.8) and registers
// the built-in introspection tools. Handlers adapt the existing use-cases.
func (s *Server) buildMCPServer() {
	srv := mcp.NewServer()
	mcp.RegisterBuiltins(srv, mcp.BuiltinDeps{
		ListProviders: s.mcpListProviders,
		ListIssued:    s.mcpListIssued,
		ListLogs:      s.mcpListLogs,
		RunChat:       s.mcpRunChat,
	})
	s.mcpServer = srv
}

// limitFromArgs pulls {"limit":N} from MCP args (default 50, capped at 200).
func limitFromArgs(args json.RawMessage) int {
	var v struct {
		Limit int `json:"limit"`
	}
	_ = json.Unmarshal(args, &v)
	if v.Limit <= 0 {
		return 50
	}
	if v.Limit > 200 {
		return 200
	}
	return v.Limit
}

func (s *Server) mcpListProviders(ctx context.Context, _ json.RawMessage) (json.RawMessage, error) {
	list, err := s.providers.List(ctx)
	if err != nil {
		return nil, err
	}
	// ponytail: surface a compact, safe shape (no internal timestamps noise).
	out := make([]map[string]any, 0, len(list))
	for _, p := range list {
		out = append(out, map[string]any{
			"id": p.ID, "name": p.Name, "strategy": p.Strategy,
			"global_models": p.GlobalModels, "status": p.Status,
		})
	}
	return json.Marshal(out)
}

func (s *Server) mcpListIssued(ctx context.Context, args json.RawMessage) (json.RawMessage, error) {
	list, err := s.issued.List(ctx)
	if err != nil {
		return nil, err
	}
	limit := limitFromArgs(args)
	if limit > len(list) {
		limit = len(list)
	}
	out := make([]map[string]any, 0, limit)
	for _, k := range list[:limit] {
		out = append(out, map[string]any{
			"id": k.ID, "name": k.Name, "provider_id": k.ProviderID,
			"status": k.Status, "expires_at": k.ExpiresAt,
			// Tokens are NEVER exposed (security §7).
		})
	}
	return json.Marshal(out)
}

func (s *Server) mcpListLogs(ctx context.Context, args json.RawMessage) (json.RawMessage, error) {
	limit := limitFromArgs(args)
	list, err := s.logs.List(ctx, ports.LogFilter{Limit: limit})
	if err != nil {
		return nil, err
	}
	out := make([]map[string]any, 0, len(list))
	for _, l := range list {
		out = append(out, map[string]any{
			"id": l.ID, "model": l.Model, "success": l.Success,
			"total_tokens": l.TotalTokens, "latency_ttfb_ms": l.LatencyTTFBMs,
			"timestamp": l.Timestamp,
		})
	}
	return json.Marshal(out)
}

// mcpRunChat bills a chat completion to an issued key by reusing the proxy.
// args: {"issued_key","model","message"}.
func (s *Server) mcpRunChat(ctx context.Context, args json.RawMessage) (json.RawMessage, error) {
	var p struct {
		IssuedKey string `json:"issued_key"`
		Model     string `json:"model"`
		Message   string `json:"message"`
	}
	if err := json.Unmarshal(args, &p); err != nil {
		return nil, err
	}
	if p.IssuedKey == "" || p.Model == "" || p.Message == "" {
		return nil, errMCPBadArgs
	}
	body, _ := json.Marshal(map[string]any{
		"model": p.Model,
		"messages": []map[string]string{{"role": "user", "content": p.Message}},
	})
	res, err := s.proxy.Handle(ctx, usecase.ProxyRequest{
		Token: p.IssuedKey, InFormat: "openai", Path: "/v1/chat/completions",
		Method: "POST", Body: body,
	})
	if err != nil {
		return nil, err
	}
	return res.Body, nil
}

var errMCPBadArgs = mcpBadArgsError{}

type mcpBadArgsError struct{}

func (mcpBadArgsError) Error() string { return "issued_key, model and message are required" }

// --- MCP → REST/OpenAI wrapper handlers (§4.8) ---

// mcpWrapperModels — GET /api/mcp/{id}/models: list an MCP server's tools as
// OpenAI model entries.
func (s *Server) mcpWrapperModels(w http.ResponseWriter, r *http.Request) {
	if s.mcpWrapper == nil {
		writeError(w, http.StatusServiceUnavailable, "mcp wrapper disabled")
		return
	}
	body, err := s.mcpWrapper.ListModelsAsOpenAI(r.Context(), r.PathValue("id"))
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(body)
}

// mcpWrapperChat — POST /api/mcp/{id}/chat: invoke an MCP tool via an
// OpenAI-shaped chat-completions body and return an OpenAI chat completion.
func (s *Server) mcpWrapperChat(w http.ResponseWriter, r *http.Request) {
	if s.mcpWrapper == nil {
		writeError(w, http.StatusServiceUnavailable, "mcp wrapper disabled")
		return
	}
	body, ok := s.readBody(w, r)
	if !ok {
		return
	}
	out, err := s.mcpWrapper.InvokeChat(r.Context(), r.PathValue("id"), body)
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(out)
}
