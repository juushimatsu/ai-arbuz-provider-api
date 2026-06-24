// Package upstream implements ports.UpstreamClient for OpenAI-compatible and
// native Anthropic providers. Streaming is pass-through (no full buffering, §6).
package upstream

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/arbuz/ai-arbuz-provider-api/internal/domain"
	"github.com/arbuz/ai-arbuz-provider-api/internal/ports"
)

// Client is a ports.UpstreamClient backed by net/http.
// ponytail: ceiling — one shared http.Transport; no per-key pooling knobs.
// Growth path = tunable transport per upstream (e.g. custom timeouts/MTLS).
type Client struct {
	http    *http.Client
	timeouts Timeouts
}

// Timeouts — separate TTFB vs overall so streaming isn't cut by a tight read timeout.
type Timeouts struct {
	Dial        time.Duration // connection establishment
	Header      time.Duration // time to first response header (TTFB upstream)
	StreamIdle  time.Duration // max idle between SSE chunks
}

// DefaultTimeouts — sensible VPS defaults.
func DefaultTimeouts() Timeouts {
	return Timeouts{Dial: 10 * time.Second, Header: 60 * time.Second, StreamIdle: 120 * time.Second}
}

// New builds a Client. timeouts zero values fall back to defaults.
func New(t Timeouts) *Client {
	if t.Dial == 0 {
		t = DefaultTimeouts()
	}
	dialer := &net.Dialer{Timeout: t.Dial}
	tr := &http.Transport{
		MaxIdleConnsPerHost:   64,
		IdleConnTimeout:       90 * time.Second,
		DialContext:           dialer.DialContext,
		ResponseHeaderTimeout: t.Header, // §6: cap upstream TTFB (time to first response header)
	}
	return &Client{
		http: &http.Client{
			Transport: tr,
			// No Client.Timeout — that would cut SSE streams mid-flight. Per-request
			// deadlines come from the caller's context; ResponseHeaderTimeout covers TTFB.
		},
		timeouts: t,
	}
}

// Do executes one upstream request. For Stream requests the caller owns Stream.
func (c *Client) Do(ctx context.Context, baseURL, secret string, req ports.UpstreamRequest) (*ports.UpstreamResponse, error) {
	url := joinURL(baseURL, req.Path)
	httpReq, err := http.NewRequestWithContext(ctx, req.Method, url, bytes.NewReader(req.Body))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	applyAuth(httpReq.Header, req.Format, secret)
	for k, v := range req.Headers {
		httpReq.Header.Set(k, v)
	}

	resp, err := c.http.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", domain.ErrUpstreamUnavailable, err)
	}

	// Non-2xx → drain body, map to domain error for failover decisions.
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		resp.Body.Close()
		return nil, classifyHTTP(resp.StatusCode, body)
	}

	out := &ports.UpstreamResponse{StatusCode: resp.StatusCode}
	// Stream: hand the body reader to the caller; do not buffer (§6).
	// Wrap it so an idle upstream (no SSE chunk for StreamIdle) surfaces as a
	// transient error instead of hanging the client forever.
	if req.Stream && strings.Contains(resp.Header.Get("Content-Type"), "event-stream") {
		out.Stream = &idleTimeoutReadCloser{
			r:           resp.Body,
			idleTimeout: c.timeouts.StreamIdle,
		}
		return out, nil
	}
	// Non-stream: read full body, then close.
	body, err := io.ReadAll(io.LimitReader(resp.Body, 64*1024*1024))
	resp.Body.Close()
	if err != nil {
		return nil, fmt.Errorf("%w: read body: %v", domain.ErrUpstreamUnavailable, err)
	}
	out.Body = body
	out.PromptTokens, out.CompletionTokens = extractUsage(req.Format, body)
	return out, nil
}

// ListModels calls GET {baseURL}/v1/models (OpenAI-compat) or the Anthropic form.
func (c *Client) ListModels(ctx context.Context, baseURL, secret string, format domain.Format) ([]string, error) {
	path := "/v1/models"
	req := ports.UpstreamRequest{Method: http.MethodGet, Path: path, Format: format}
	resp, err := c.Do(ctx, baseURL, secret, req)
	if err != nil {
		return nil, err
	}
	return parseModels(format, resp.Body)
}

// parseModels extracts id list from both OpenAI {"data":[{"id":...}]} and
// Anthropic {"data":[{"id":...}]} shapes (Anthropic v1/models is OpenAI-shaped).
func parseModels(_ domain.Format, body []byte) ([]string, error) {
	var shape struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &shape); err != nil {
		return nil, fmt.Errorf("parse models: %w", err)
	}
	out := make([]string, 0, len(shape.Data))
	for _, m := range shape.Data {
		if m.ID != "" {
			out = append(out, m.ID)
		}
	}
	return out, nil
}

// joinURL joins a base URL and a path that may or may not start with "/".
func joinURL(base, path string) string {
	if path == "" {
		return strings.TrimRight(base, "/")
	}
	if strings.HasPrefix(path, "http://") || strings.HasPrefix(path, "https://") {
		return path
	}
	base = strings.TrimRight(base, "/")
	path = "/" + strings.TrimLeft(path, "/")
	// Avoid duplicating an API version prefix when the configured base_url
	// already includes it, e.g. base ".../v1" + path "/v1/chat/completions"
	// must not become ".../v1/v1/chat/completions" (upstream 404).
	for _, seg := range []string{"/v1beta", "/v1"} {
		if strings.HasSuffix(base, seg) && (path == seg || strings.HasPrefix(path, seg+"/")) {
			path = strings.TrimPrefix(path, seg)
			break
		}
	}
	return base + path
}

// applyAuth sets the right auth header per format.
// OpenAI: Authorization: Bearer <key>. Anthropic: x-api-key: <key> + version.
func applyAuth(h http.Header, format domain.Format, secret string) {
	switch format {
	case domain.FormatAnthropic:
		h.Set("x-api-key", secret)
		if h.Get("anthropic-version") == "" {
			h.Set("anthropic-version", "2023-06-01")
		}
	default:
		h.Set("Authorization", "Bearer "+secret)
	}
}

// classifyHTTP maps a non-2xx upstream response to one of three domain errors.
// This is the failover/retry decision point — getting it wrong wastes upstream
// quota (§4.4) and breaks SDK backoff.
//
//   - 429 / 5xx          → ErrUpstreamUnavailable  (transient → failover + retry)
//   - 401 / 403          → ErrUpstreamAuth         (bad KEY → failover, no retry)
//   - 400 / 404 / 4xx…   → ErrUpstreamClientError  (bad REQUEST → surface, no failover)
//
// ponytail: ceiling — 408/425/451/460+ are mapped by code-range heuristics.
// Growth path = inspect upstream error JSON `type`/`code` for finer calls.
func classifyHTTP(code int, body []byte) error {
	switch {
	case code == http.StatusTooManyRequests, code >= 500:
		return fmt.Errorf("%w: http %d: %s", domain.ErrUpstreamUnavailable, code, snippet(body))
	case code == http.StatusUnauthorized, code == http.StatusForbidden:
		return fmt.Errorf("%w: http %d: %s", domain.ErrUpstreamAuth, code, snippet(body))
	case code >= 400 && code < 500:
		return fmt.Errorf("%w: http %d: %s", domain.ErrUpstreamClientError, code, snippet(body))
	default:
		return fmt.Errorf("%w: http %d: %s", domain.ErrUpstreamUnavailable, code, snippet(body))
	}
}

func snippet(b []byte) string {
	s := strings.TrimSpace(string(b))
	if len(s) > 256 {
		return s[:256] + "…"
	}
	return s
}

// idleTimeoutReadCloser wraps an upstream SSE body so a stalled stream (no data
// for idleTimeout) fails the read instead of blocking forever (§6 reliability).
// ponytail: ceiling — idle is per-Read, not whole-stream; a slow but steady
// stream of chunks will not trip it. Growth path = context-aware deadline reset.
type idleTimeoutReadCloser struct {
	r           io.ReadCloser
	idleTimeout time.Duration
}

func (i *idleTimeoutReadCloser) Read(p []byte) (int, error) {
	if i.idleTimeout <= 0 {
		return i.r.Read(p)
	}
	type readResult struct {
		n   int
		err error
	}
	ch := make(chan readResult, 1)
	go func() {
		n, err := i.r.Read(p)
		ch <- readResult{n, err}
	}()
	select {
	case res := <-ch:
		return res.n, res.err
	case <-time.After(i.idleTimeout):
		// Best-effort close so the goroutine's read doesn't leak forever.
		_ = i.r.Close()
		return 0, fmt.Errorf("%w: stream idle for %s", domain.ErrUpstreamUnavailable, i.idleTimeout)
	}
}

func (i *idleTimeoutReadCloser) Close() error { return i.r.Close() }