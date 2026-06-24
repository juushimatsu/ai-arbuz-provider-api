package httptrans

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"

	"github.com/arbuz/ai-arbuz-provider-api/internal/domain"
	"github.com/arbuz/ai-arbuz-provider-api/internal/usecase"
)

// handleChat — POST /v1/chat/completions (OpenAI-compatible).
func (s *Server) handleChat(w http.ResponseWriter, r *http.Request) {
	body, ok := s.readBody(w, r)
	if !ok {
		return
	}
	req := proxyReqFromCtx(r)
	req.InFormat = domain.FormatOpenAI
	req.Path = "/v1/chat/completions"
	req.Method = http.MethodPost
	req.Body = body
	req.Stream = bodyHasStream(body)
	s.runProxy(w, r, req)
}

// handleMessages — POST /v1/messages (Anthropic-compatible).
func (s *Server) handleMessages(w http.ResponseWriter, r *http.Request) {
	body, ok := s.readBody(w, r)
	if !ok {
		return
	}
	req := proxyReqFromCtx(r)
	req.InFormat = domain.FormatAnthropic
	req.Path = "/v1/messages"
	req.Method = http.MethodPost
	req.Body = body
	req.Stream = bodyHasStream(body)
	s.runProxy(w, r, req)
}

// handleModels — GET /v1/models. Served from provider config (no upstream call).
func (s *Server) handleModels(w http.ResponseWriter, r *http.Request) {
	req := proxyReqFromCtx(r)
	req.InFormat = domain.FormatOpenAI
	req.IsModels = true
	s.runProxy(w, r, req)
}

// handleEmbeddings — POST /v1/embeddings (Phase 7 full; Phase 1 passes through).
func (s *Server) handleEmbeddings(w http.ResponseWriter, r *http.Request) {
	body, ok := s.readBody(w, r)
	if !ok {
		return
	}
	req := proxyReqFromCtx(r)
	req.InFormat = domain.FormatOpenAI
	req.Path = "/v1/embeddings"
	req.Method = http.MethodPost
	req.Body = body
	s.runProxy(w, r, req)
}

// runProxy invokes the Proxy and streams/writes the result back to the client.
func (s *Server) runProxy(w http.ResponseWriter, r *http.Request, req usecase.ProxyRequest) {
	res, err := s.proxy.Handle(r.Context(), req)
	if err != nil {
		status, msg := mapDomainError(err)
		if req.InFormat == domain.FormatAnthropic {
			writeAnthropicError(w, status, msg)
		} else {
			writeOpenAIError(w, status, "api_error", msg)
		}
		return
	}

	// Copy upstream-native headers we want to forward.
	if res.Header != nil {
		for k, v := range res.Header {
			if isPassthroughHeader(k) {
				w.Header()[k] = v
			}
		}
	}

	// Streaming: flush each chunk from the upstream body to the client.
	if res.Stream != nil {
		defer res.Stream.Close()
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")
		w.WriteHeader(res.StatusCode)
		flusher, _ := w.(http.Flusher)
		buf := make([]byte, 4096)
		var streamErr error
		for {
			n, rerr := res.Stream.Read(buf)
			if n > 0 {
				res.MarkTTFB()
				_, _ = w.Write(buf[:n])
				if flusher != nil {
					flusher.Flush()
				}
			}
			if rerr == io.EOF {
				break
			}
			if rerr != nil {
				// Non-EOF error mid-stream: report to the post-completion hook
				// so the request is logged as a failure (blocker #4).
				streamErr = rerr
				break
			}
		}
		if res.OnDone != nil {
			res.OnDone(streamErr)
		}
		return
	}

	// Non-streaming: write the body in the client's incoming format.
	ct := "application/json"
	if res.Header != nil && len(res.Header.Get("Content-Type")) > 0 {
		ct = res.Header.Get("Content-Type")
	}
	w.Header().Set("Content-Type", ct)
	w.WriteHeader(res.StatusCode)
	res.MarkTTFB()
	_, _ = w.Write(res.Body)
	if res.OnDone != nil {
		res.OnDone(nil)
	}
}

// bodyHasStream detects {"stream":true} without a full decode (best effort).
func bodyHasStream(body []byte) bool {
	var v struct {
		Stream bool `json:"stream"`
	}
	if err := json.Unmarshal(body, &v); err != nil {
		return false
	}
	return v.Stream
}

// isPassthroughHeader allows through harmless upstream headers only.
// ponytail: allowlist, not blocklist — safer default at this trust boundary.
func isPassthroughHeader(h string) bool {
	switch strings.ToLower(h) {
	case "content-type", "x-request-id", "openai-organization", "anthropic-ratelimit-requests-limit":
		return true
	}
	return false
}

// writeAnthropicError mirrors the Anthropic error envelope.
func writeAnthropicError(w http.ResponseWriter, status int, message string) {
	type antErr struct {
		Type  string `json:"type"`
		Error struct {
			Type    string `json:"type"`
			Message string `json:"message"`
		} `json:"error"`
	}
	var e antErr
	e.Type = "error"
	e.Error.Type = "api_error"
	e.Error.Message = message
	writeJSON(w, status, e)
}
