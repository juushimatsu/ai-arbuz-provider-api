// Package httptrans is the delivery/transport layer (§3.2): HTTP handlers for
// the OpenAI/Anthropic-compatible client traffic AND the admin panel API.
package httptrans

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/arbuz/ai-arbuz-provider-api/internal/adapter/mcp"
	"github.com/arbuz/ai-arbuz-provider-api/internal/domain"
	"github.com/arbuz/ai-arbuz-provider-api/internal/ports"
	"github.com/arbuz/ai-arbuz-provider-api/internal/usecase"
)

// ctxKey is unexported to avoid key collisions across packages.
type ctxKey int

const (
	keyReqID ctxKey = iota
	keyAdminUserID
)

// Server wires all HTTP routes and holds service references.
type Server struct {
	proxy      *usecase.Proxy
	auth       *usecase.Auth
	providers  *usecase.ProviderService
	upstreams  *usecase.UpstreamService
	issued     *usecase.IssuedService
	logs       *usecase.LogService
	stats      ports.Stats
	mcp        ports.MCPBridge
	mcpRepo     ports.MCPRepo
	mcpWrapper  *usecase.MCPWrapper
	mcpServer   *mcp.Server // the router's OWN MCP server (§4.8)
	checker     ports.UpstreamClient
	checkerRepo ports.CheckerRepo
	modelSearch ports.UpstreamClient
	promptRules *usecase.PromptRuleService
	secrets     ports.SecretStore
	assets      *StaticAssets
	maxBody     int64 // inbound body cap, applied via readBody (§6 hardening)
	log         *slog.Logger
	mux         *http.ServeMux
}

// Deps bundles everything the server needs (DI).
type Deps struct {
	Proxy       *usecase.Proxy
	Auth        *usecase.Auth
	Providers   *usecase.ProviderService
	Upstreams   *usecase.UpstreamService
	Issued      *usecase.IssuedService
	Logs        *usecase.LogService
	Stats       ports.Stats
	MCP         ports.MCPBridge
	MCPRepo     ports.MCPRepo
	Checker     ports.UpstreamClient
	CheckerRepo ports.CheckerRepo
	ModelSearch ports.UpstreamClient
	PromptRules *usecase.PromptRuleService
	Secrets     ports.SecretStore
	Logger      *slog.Logger
	// StaticDir points at the built SPA (web/dist). Empty → SPA not served by Go.
	StaticDir   string
	// MaxBodyBytes caps inbound request bodies (client + admin). 0 = 16MiB.
	MaxBodyBytes int64
}

func NewServer(d Deps) *Server {
	maxBody := d.MaxBodyBytes
	if maxBody <= 0 {
		maxBody = 16 * 1024 * 1024
	}
	s := &Server{
		proxy: d.Proxy, auth: d.Auth, providers: d.Providers, upstreams: d.Upstreams,
		issued: d.Issued, logs: d.Logs, stats: d.Stats, mcp: d.MCP, mcpRepo: d.MCPRepo,
		mcpWrapper: usecase.NewMCPWrapper(d.MCP, d.MCPRepo),
		checker: d.Checker, checkerRepo: d.CheckerRepo, modelSearch: d.ModelSearch,
		promptRules: d.PromptRules,
		secrets: d.Secrets, log: d.Logger, assets: NewStaticAssets(d.StaticDir),
		maxBody: maxBody, mux: http.NewServeMux(),
	}
	s.routes()
	return s
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) { s.mux.ServeHTTP(w, r) }

// routes registers every endpoint. Go 1.22+ ServeMux supports method+path.
func (s *Server) routes() {
	// --- client traffic (OpenAI/Anthropic-compatible), auth by issued key ---
	s.mux.HandleFunc("POST /v1/chat/completions", s.clientKeyAuth(s.handleChat))
	s.mux.HandleFunc("POST /v1/messages", s.clientKeyAuth(s.handleMessages))
	s.mux.HandleFunc("GET /v1/models", s.clientKeyAuth(s.handleModels))
	s.mux.HandleFunc("POST /v1/embeddings", s.clientKeyAuth(s.handleEmbeddings))

	// --- admin panel API, auth by session cookie ---
	s.mux.HandleFunc("POST /api/auth/login", s.handleLogin)
	s.mux.HandleFunc("POST /api/auth/logout", s.adminAuth(s.handleLogout))
	s.mux.HandleFunc("GET /api/auth/me", s.adminAuth(s.handleMe))
	s.mux.HandleFunc("POST /api/auth/credentials", s.adminAuth(s.handleChangeCredentials))

	s.mux.HandleFunc("GET /api/providers", s.adminAuth(s.listProviders))
	s.mux.HandleFunc("POST /api/providers", s.adminAuth(s.createProvider))
	s.mux.HandleFunc("GET /api/providers/{id}", s.adminAuth(s.getProvider))
	s.mux.HandleFunc("PUT /api/providers/{id}", s.adminAuth(s.updateProvider))
	s.mux.HandleFunc("DELETE /api/providers/{id}", s.adminAuth(s.deleteProvider))

	s.mux.HandleFunc("GET /api/upstreams", s.adminAuth(s.listUpstreams))
	s.mux.HandleFunc("POST /api/upstreams", s.adminAuth(s.createUpstream))
	s.mux.HandleFunc("GET /api/upstreams/{id}", s.adminAuth(s.getUpstream))
	s.mux.HandleFunc("PUT /api/upstreams/{id}", s.adminAuth(s.updateUpstream))
	s.mux.HandleFunc("DELETE /api/upstreams/{id}", s.adminAuth(s.deleteUpstream))

	s.mux.HandleFunc("GET /api/issued", s.adminAuth(s.listIssued))
	s.mux.HandleFunc("POST /api/issued", s.adminAuth(s.createIssued))
	s.mux.HandleFunc("GET /api/issued/{id}", s.adminAuth(s.getIssued))
	s.mux.HandleFunc("PUT /api/issued/{id}", s.adminAuth(s.updateIssued))
	s.mux.HandleFunc("DELETE /api/issued/{id}", s.adminAuth(s.deleteIssued))
	s.mux.HandleFunc("POST /api/issued/{id}/revoke", s.adminAuth(s.revokeIssued))

	s.mux.HandleFunc("GET /api/logs", s.adminAuth(s.listLogs))
	s.mux.HandleFunc("GET /api/logs/{id}", s.adminAuth(s.getLog))

	s.mux.HandleFunc("GET /api/stats/summary", s.adminAuth(s.statsSummary))
	s.mux.HandleFunc("GET /api/stats/series", s.adminAuth(s.statsSeries))
	s.mux.HandleFunc("GET /api/stats/breakdown", s.adminAuth(s.statsBreakdown))

	s.mux.HandleFunc("POST /api/models/search", s.adminAuth(s.searchModels))
	s.mux.HandleFunc("POST /api/checker/run", s.adminAuth(s.runChecker))
	s.mux.HandleFunc("GET /api/checker/runs", s.adminAuth(s.listCheckerRuns))
	s.mux.HandleFunc("GET /api/checker/runs/{id}", s.adminAuth(s.getCheckerRun))

	s.mux.HandleFunc("GET /api/prompt-rules", s.adminAuth(s.listPromptRules))
	s.mux.HandleFunc("POST /api/prompt-rules", s.adminAuth(s.createPromptRule))
	s.mux.HandleFunc("PUT /api/prompt-rules/{id}", s.adminAuth(s.updatePromptRule))
	s.mux.HandleFunc("DELETE /api/prompt-rules/{id}", s.adminAuth(s.deletePromptRule))

	s.mux.HandleFunc("GET /api/mcp", s.adminAuth(s.listMCP))
	s.mux.HandleFunc("POST /api/mcp", s.adminAuth(s.createMCP))
	s.mux.HandleFunc("PUT /api/mcp/{id}", s.adminAuth(s.updateMCP))
	s.mux.HandleFunc("DELETE /api/mcp/{id}", s.adminAuth(s.deleteMCP))
	s.mux.HandleFunc("POST /api/mcp/{id}/tools", s.adminAuth(s.discoverMCPTools))
	s.mux.HandleFunc("POST /api/mcp/{id}/call", s.adminAuth(s.callMCPTool))
	// MCP → REST/OpenAI wrapper (§4.8): expose an MCP server as OpenAI models/chat.
	s.mux.HandleFunc("GET /api/mcp/{id}/models", s.adminAuth(s.mcpWrapperModels))
	s.mux.HandleFunc("POST /api/mcp/{id}/chat", s.adminAuth(s.mcpWrapperChat))

	s.mux.HandleFunc("GET /api/health", s.health)

	// MCP SERVER (§4.8): the router publishes its own tools at /mcp.
	// Auth-gated by admin session — these are administrative introspection tools.
	s.buildMCPServer()
	s.mux.HandleFunc("POST /mcp", s.adminAuth(s.mcpServer.ServeHTTP))

	// SPA catch-all: everything not /api, /v1, or a static asset file is served
	// as the SPA (history-mode fallback). Registered last so specific routes win.
	if s.assets != nil {
		spa := s.assets.Handler()
		s.mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
			// ServeMux "/" matches everything; re-guard API/v1 just in case.
			if isClientPath(r.URL.Path) {
				http.NotFound(w, r)
				return
			}
			spa.ServeHTTP(w, r)
		})
	}
}

// --- common middleware ---

// requestID + structured access log, applied to all routes via Wrap.
func (s *Server) Wrap(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rw := &statusRecorder{ResponseWriter: w, status: 200}
		h.ServeHTTP(rw, r)
		s.log.Info("http",
			"method", r.Method, "path", r.URL.Path, "status", rw.status,
			"dur_ms", time.Since(start).Milliseconds(), "ip", r.RemoteAddr)
	})
}

type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (r *statusRecorder) WriteHeader(code int) {
	r.status = code
	r.ResponseWriter.WriteHeader(code)
}

// adminAuth guards panel API routes with a session cookie.
func (s *Server) adminAuth(h http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		cookie, err := r.Cookie("arbuz_session")
		if err != nil || cookie.Value == "" {
			writeError(w, http.StatusUnauthorized, "unauthorized")
			return
		}
		uid, err := s.auth.VerifySession(r.Context(), cookie.Value)
		if err != nil {
			writeError(w, http.StatusUnauthorized, "unauthorized")
			return
		}
		ctx := context.WithValue(r.Context(), keyAdminUserID, uid)
		h.ServeHTTP(w, r.WithContext(ctx))
	}
}

// clientKeyAuth extracts the issued key (Authorization: Bearer OR x-api-key)
// and runs the proxy handler. Format is detected from the path.
func (s *Server) clientKeyAuth(h http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		token := clientToken(r)
		if token == "" {
			writeOpenAIError(w, http.StatusUnauthorized, "invalid_api_key",
				"Missing API key. Provide Authorization: Bearer <key> or x-api-key.")
			return
		}
		inFmt := domain.DetectFormatByPath(r.URL.Path)
		_ = inFmt // forwarded into the proxy request below
		req := usecase.ProxyRequest{
			Token: token, InFormat: inFmt, Path: r.URL.Path, Method: r.Method,
			Header: r.Header,
			IsModels: r.URL.Path == "/v1/models",
		}
		ctx := context.WithValue(r.Context(), proxyReqKey{}, req)
		h.ServeHTTP(w, r.WithContext(ctx))
	}
}

type proxyReqKey struct{}

// proxyReqFromCtx pulls the pre-built ProxyRequest built by clientKeyAuth.
func proxyReqFromCtx(r *http.Request) usecase.ProxyRequest {
	if v, ok := r.Context().Value(proxyReqKey{}).(usecase.ProxyRequest); ok {
		return v
	}
	return usecase.ProxyRequest{}
}

// clientToken accepts "Bearer <key>", raw Authorization, or x-api-key header.
func clientToken(r *http.Request) string {
	if k := r.Header.Get("x-api-key"); k != "" {
		return strings.TrimSpace(k)
	}
	auth := r.Header.Get("Authorization")
	if auth == "" {
		return ""
	}
	if strings.HasPrefix(auth, "Bearer ") {
		return strings.TrimSpace(strings.TrimPrefix(auth, "Bearer "))
	}
	return strings.TrimSpace(auth)
}

// --- JSON response helpers ---

// readBody reads the request body with a size cap (§6 hardening). Returns the
// bytes and an error if the body exceeded maxBody or couldn't be read.
func (s *Server) readBody(w http.ResponseWriter, r *http.Request) ([]byte, bool) {
	b, err := io.ReadAll(http.MaxBytesReader(w, r.Body, s.maxBody))
	if err != nil {
		writeError(w, http.StatusRequestEntityTooLarge, "request body too large")
		return nil, false
	}
	return b, true
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

// writeOpenAIError mirrors the OpenAI error envelope so SDKs parse it.
func writeOpenAIError(w http.ResponseWriter, status int, code, message string) {
	writeJSON(w, status, map[string]any{
		"error": map[string]any{
			"message": message,
			"type":    "invalid_request_error",
			"code":    code,
		},
	})
}

// mapDomainError translates a domain error into an HTTP status + client message.
// Order matters: more specific sentinels first.
func mapDomainError(err error) (int, string) {
	switch {
	case errors.Is(err, domain.ErrUnauthorized):
		return http.StatusUnauthorized, "Unauthorized"
	case errors.Is(err, domain.ErrForbidden):
		return http.StatusForbidden, "Forbidden"
	case errors.Is(err, domain.ErrKeyRevoked):
		return http.StatusUnauthorized, "Key revoked"
	case errors.Is(err, domain.ErrKeyExpired):
		return http.StatusUnauthorized, "Key expired"
	case errors.Is(err, domain.ErrLimitExceeded):
		return http.StatusTooManyRequests, "Rate limit / quota exceeded"
	case errors.Is(err, domain.ErrValidation):
		return http.StatusBadRequest, err.Error()
	case errors.Is(err, domain.ErrUpstreamClientError):
		// Upstream rejected the request itself (400/404/413…) — surface as 400
		// so the client fixes its request instead of retrying blindly.
		return http.StatusBadRequest, err.Error()
	case errors.Is(err, domain.ErrNoUpstreamKey):
		return http.StatusServiceUnavailable, "No upstream available"
	case errors.Is(err, domain.ErrUpstreamUnavailable), errors.Is(err, domain.ErrUpstreamAuth):
		return http.StatusBadGateway, "Upstream error"
	default:
		return http.StatusBadGateway, "Upstream error"
	}
}
