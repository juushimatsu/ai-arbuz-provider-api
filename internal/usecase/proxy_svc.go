package usecase

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/arbuz/ai-arbuz-provider-api/internal/domain"
	"github.com/arbuz/ai-arbuz-provider-api/internal/inspect"
	"github.com/arbuz/ai-arbuz-provider-api/internal/ports"
)

// Proxy orchestrates a single client request end-to-end (§4.4, §4.5, §4.6):
// issued-key lookup → limit check → upstream selection → conversion → upstream
// call → response/streaming → logging.
//
// Streaming is pass-through: for SSE the upstream body reader is handed to the
// transport (which flushes chunk-by-chunk); we never buffer the whole reply (§6).
type Proxy struct {
	issued    ports.IssuedRepo
	providers ports.ProviderRepo
	upstreams ports.UpstreamRepo
	secrets   ports.SecretStore
	client    ports.UpstreamClient
	converter ports.Converter
	limiter   ports.Limiter
	selector  ports.KeySelector
	cache     ports.Cache
	logs      ports.LogRepo
	// logPayload enables full masked request/response capture (§4.7).
	logPayload bool
	// promptRules applies pre-configured transformations to request bodies.
	promptRules []domain.PromptRule
	// guard scans upstream RESPONSES for malicious-provider injection (tool-call /
	// shell-command smuggling). guardMode is "block", "alert" or "off".
	guard     *inspect.Engine
	guardMode string
}

type ProxyDeps struct {
	Issued    ports.IssuedRepo
	Providers ports.ProviderRepo
	Upstreams ports.UpstreamRepo
	Secrets   ports.SecretStore
	Client    ports.UpstreamClient
	Converter ports.Converter
	Limiter   ports.Limiter
	Selector  ports.KeySelector
	Cache     ports.Cache
	Logs      ports.LogRepo
	LogPayload bool
}

func NewProxy(d ProxyDeps) *Proxy {
	return &Proxy{
		issued: d.Issued, providers: d.Providers, upstreams: d.Upstreams,
		secrets: d.Secrets, client: d.Client, converter: d.Converter,
		limiter: d.Limiter, selector: d.Selector, cache: d.Cache, logs: d.Logs,
		logPayload: d.LogPayload,
	}
}

// SetPromptRules installs the active prompt-transformation rules (§4.7).
// Thread-safe to call at startup; rules are read without a lock in Handle
// (set-once at boot is the intended usage — see AGENTS.md, no premature locks).
func (p *Proxy) SetPromptRules(rules []domain.PromptRule) { p.promptRules = rules }

// SetGuard installs the response-inspection engine and its reaction mode
// ("block" | "alert" | "off"). A nil engine or "off" disables inspection.
func (p *Proxy) SetGuard(e *inspect.Engine, mode string) { p.guard = e; p.guardMode = mode }

// GuardMode reports the active response-guard mode for diagnostics/UI
// ("block", "alert" or "off").
func (p *Proxy) GuardMode() string {
	if p.guard == nil || p.guardMode == "" {
		return "off"
	}
	return p.guardMode
}

// guardEnabled reports whether response inspection should run.
func (p *Proxy) guardEnabled() bool {
	return p.guard != nil && (p.guardMode == "block" || p.guardMode == "alert")
}

// logFindings records guard hits to the server log (ponytail: stderr only;
// growth path = surface in the Logs UI via a dedicated event).
func (p *Proxy) logFindings(stream bool, fs []inspect.Finding) {
	inspect.SortFindings(fs)
	kind := "buffered"
	if stream {
		kind = "stream"
	}
	for _, f := range fs {
		log.Printf("guard[%s] %s severity=%s rule=%s src=%s: %s", p.guardMode, kind, f.Severity, f.RuleID, f.Source, f.Description)
	}
}

// SetLogPayload toggles masked payload capture at runtime.
func (p *Proxy) SetLogPayload(enabled bool) { p.logPayload = enabled }

// ProxyRequest carries what the transport gathered about one client call.
type ProxyRequest struct {
	Token    string        // issued key (bearer / x-api-key)
	InFormat domain.Format // detected from path
	Path     string        // e.g. "/v1/chat/completions"
	Method   string
	Body     []byte
	Stream   bool
	IsModels bool // GET /v1/models
	Header   http.Header
}

// ProxyResult holds the connection the transport should stream/write.
// Exactly one of (Body, Stream) is populated.
type ProxyResult struct {
	StatusCode int
	Header     http.Header
	Body       []byte        // non-stream response, already in IN format
	Stream     io.ReadCloser // SSE pass-through
	OutFormat  domain.Format
	Usage      ports.Usage
	// OnDone, when set, MUST be invoked by the transport after the response has
	// been fully delivered (non-stream) or the stream reached EOF/error. The
	// transport passes any transport-level stream error; the FINAL usage is read
	// from UCap (populated by the converter / stream parser as it flows).
	//
	// This deferred-completion hook is what lets the proxy account tokens and
	// log success/failure POST-FACTUM for streams (§4.3, §4.11), instead of at
	// header time when output tokens aren't known yet.
	OnDone func(streamErr error)
	// UCap holds the final parsed usage for streams; nil for non-stream.
	UCap *usageCapture
	// ttfbAt is set by the transport (via MarkTTFB) when the FIRST byte is
	// written to the client; TTFB = ttfbAt - start. Lets us log TTFB separately
	// from TotalMs (§4.11 / audit #11) — previously both were identical.
	ttfbMu       sync.Mutex
	ttfbAt       time.Time
	ttfbRecorded bool
	// Bookkeeping for logging, filled by Handle.
	issuedID    domain.ID
	providerID  domain.ID
	upstreamID  domain.ID
	model       string
	inFormat    domain.Format
	outFormat   domain.Format
	streamed    bool
}

// MarkTTFB records the time-to-first-byte moment. Idempotent: only the first
// call wins, so the transport can safely call it before every Write.
func (r *ProxyResult) MarkTTFB() {
	r.ttfbMu.Lock()
	defer r.ttfbMu.Unlock()
	if r.ttfbRecorded {
		return
	}
	r.ttfbAt = time.Now()
	r.ttfbRecorded = true
}

// Handle resolves the request and returns what to send to the client.
// Errors are domain errors; transport maps them to HTTP status.
func (p *Proxy) Handle(ctx context.Context, req ProxyRequest) (*ProxyResult, error) {
	start := time.Now()

	issued, err := p.resolveKey(ctx, req.Token)
	if err != nil {
		return nil, err
	}
	provider, err := p.providers.Get(ctx, issued.ProviderID)
	if err != nil {
		return nil, err
	}
	if provider.Status != domain.StatusActive {
		return nil, domain.ErrForbidden
	}

	// GET /v1/models: answer from provider config, no upstream call (§4.5).
	if req.IsModels {
		res := p.handleModels(ctx, provider)
		p.recordSuccess(ctx, issued, provider, nil, req, res, start)
		return res, nil
	}

	// Apply pre-configured prompt transformations (§4.7) to the incoming body.
	// Done once, in the client's incoming format, before conversion/upstream.
	if len(p.promptRules) > 0 {
		req.Body = ApplyPromptRules(req.Body, p.promptRules, req.InFormat)
	}

	model := extractModel(req.Body)
	projected := estimateTokens(req.Body)

	// Issued-limit check BEFORE any upstream call (§4.4).
	if p.limiter != nil {
		if err := p.limiter.Check(ctx, issued, projected); err != nil {
			return nil, err
		}
	}

	// Exact-match cache for non-streaming GET-equivalent requests (§4.7).
	// Streamed and embedding requests bypass the cache (they're not idempotent
	// in a useful way for this router). ponytail: ceiling — key is sha256(body)
	// + issuedID; no semantic matching. Growth path = embedding cache + semantic.
	if p.cache != nil && !req.Stream && req.Path == "/v1/chat/completions" {
		if cached, ok := p.cache.Get(ctx, cacheKey(issued.ID, req.Body)); ok {
			res := &ProxyResult{
				StatusCode: 200, Header: jsonHeader(), Body: cached,
				OutFormat: req.InFormat, issuedID: issued.ID, providerID: provider.ID,
				model: model, inFormat: req.InFormat, outFormat: req.InFormat,
			}
			// Cache hits must still be logged and accounted (audit #3): otherwise
			// repeated cacheable requests bypass usage limits and never appear in
			// the request log. Usage is parsed from the cached response body.
			res.Usage = extractUsageFromBody(cached, req.InFormat)
			p.recordSuccess(ctx, issued, provider, nil, req, res, start)
			_ = p.issued.MarkUsed(ctx, issued.ID, time.Now().UTC())
			return res, nil
		}
	}

	keys, err := p.upstreams.ListByProvider(ctx, provider.ID)
	if err != nil {
		return nil, err
	}
	ordered, err := p.selector.Select(ctx, provider, keys, model, time.Now().UTC())
	if err != nil {
		return nil, err
	}
	if len(ordered) == 0 {
		// No usable key for the requested model — try fallback models (§4.4),
		// re-running selection per fallback so model-filter uses the fallback.
		for _, fb := range provider.FallbackModels {
			reqFB := req
			reqFB.Body = rewriteModel(req.Body, fb)
			fbOrdered, selErr := p.selector.Select(ctx, provider, keys, fb, time.Now().UTC())
			if selErr != nil || len(fbOrdered) == 0 {
				continue
			}
			res, clientErr := p.tryChain(ctx, issued, provider, fbOrdered, reqFB, fb, start)
			if clientErr != nil {
				p.recordFailure(ctx, issued, provider, req, clientErr, start)
				return nil, clientErr
			}
			if res != nil {
				return res, nil
			}
		}
		p.recordFailure(ctx, issued, provider, req, domain.ErrNoUpstreamKey, start)
		return nil, domain.ErrNoUpstreamKey
	}

	res, clientErr := p.tryChain(ctx, issued, provider, ordered, req, model, start)
	if clientErr != nil {
		// Bad request surfaced by an upstream — do NOT try fallback models.
		p.recordFailure(ctx, issued, provider, req, clientErr, start)
		return nil, clientErr
	}
	if res != nil {
		return res, nil
	}
	// All keys failed transiently/auth — try fallback models with fresh selection.
	for _, fb := range provider.FallbackModels {
		reqFB := req
		reqFB.Body = rewriteModel(req.Body, fb)
		fbOrdered, selErr := p.selector.Select(ctx, provider, keys, fb, time.Now().UTC())
		if selErr != nil || len(fbOrdered) == 0 {
			continue
		}
		res, clientErr := p.tryChain(ctx, issued, provider, fbOrdered, reqFB, fb, start)
		if clientErr != nil {
			p.recordFailure(ctx, issued, provider, req, clientErr, start)
			return nil, clientErr
		}
		if res != nil {
			return res, nil
		}
	}
	p.recordFailure(ctx, issued, provider, req, domain.ErrNoUpstreamKey, start)
	return nil, domain.ErrNoUpstreamKey
}

// tryChain attempts the ordered keys in sequence for one request.
// Returns (result, clientErr) where exactly one is meaningful:
//   - result != nil  → a key succeeded (clientErr == nil)
//   - clientErr != nil → a key returned ErrUpstreamClientError (bad request);
//     the caller must surface this to the client and NOT try fallback models.
//   - both nil → every key failed with a transient/auth error (failover exhausted);
//     the caller may try fallback models or report no-upstream.
func (p *Proxy) tryChain(ctx context.Context, issued *domain.IssuedKey, provider *domain.Provider,
	ordered []domain.UpstreamKey, req ProxyRequest, model string, start time.Time) (*ProxyResult, error) {
	for i := range ordered {
		k := ordered[i]
		// chooseOutFormat per-key: a key whose native format matches the input
		// avoids conversion, so this depends on the key, not the whole chain.
		outFormat := chooseOutFormat(req.InFormat, []domain.UpstreamKey{k})
		res, ferr := p.tryUpstream(ctx, provider, issued, &k, req, outFormat)
		if ferr == nil {
			res.issuedID = issued.ID
			res.providerID = provider.ID
			res.upstreamID = k.ID
			res.model = model
			res.inFormat = req.InFormat
			res.outFormat = outFormat
			res.streamed = req.Stream
			// Defer logging/cache/health until the response is fully delivered.
			// For streams, the converter parses the final usage chunk which the
			// transport reports back via finalUsage — without this, output tokens
			// (and thus token-based limits for streaming clients) were never
			// accounted (blocker #4).
			res.OnDone = p.onRequestDone(issued, provider, &k, req, res, start)
			return res, nil
		}
		// ErrUpstreamClientError = the request itself is bad → stop immediately,
		// surface to client. No failover, no retry, no fallback-model retry.
		if errors.Is(ferr, domain.ErrUpstreamClientError) {
			return nil, ferr
		}
		// ErrUpstreamAuth (bad key) / ErrUpstreamUnavailable (transient) → failover.
		_ = p.selector.OnFailure(ctx, &k, ferr)
	}
	return nil, nil
}

// onRequestDone builds the post-completion hook called by the transport once
// the response body has been fully written (non-stream) or streamed to EOF.
// The final usage is read from res.UCap (populated during streaming) so output
// tokens for streams are accounted accurately (blocker #4).
func (p *Proxy) onRequestDone(issued *domain.IssuedKey, provider *domain.Provider, key *domain.UpstreamKey,
	req ProxyRequest, res *ProxyResult, start time.Time) func(error) {
	called := false
	return func(streamErr error) {
		if called {
			return // idempotent guard against double-invoke
		}
		called = true
		// For streams, UCap carries the usage parsed from the terminal chunk.
		if res.UCap != nil {
			if u := res.UCap.get(); u.PromptTokens != 0 || u.CompletionTokens != 0 {
				res.Usage = u
			}
		}
		if streamErr != nil {
			p.recordFailure(context.Background(), issued, provider, req, streamErr, start)
			return
		}
		// Cache exact-match, non-stream chat responses AFTER successful delivery.
		if p.cache != nil && !req.Stream && req.Path == "/v1/chat/completions" && len(res.Body) > 0 {
			p.cache.Set(context.Background(), cacheKey(issued.ID, req.Body), res.Body)
		}
		p.recordSuccess(context.Background(), issued, provider, key, req, res, start)
		_ = p.issued.MarkUsed(context.Background(), issued.ID, time.Now().UTC())
		_ = p.selector.OnSuccess(context.Background(), key)
	}
}

func (p *Proxy) resolveKey(ctx context.Context, token string) (*domain.IssuedKey, error) {
	token = strings.TrimSpace(token)
	if token == "" {
		return nil, domain.ErrUnauthorized
	}
	k, err := p.issued.GetByTokenHash(ctx, hashToken(token))
	if err != nil {
		return nil, domain.ErrUnauthorized
	}
	if !k.IsActive(time.Now().UTC()) {
		if k.Status != domain.StatusActive {
			return nil, domain.ErrKeyRevoked
		}
		return nil, domain.ErrKeyExpired
	}
	return k, nil
}

// tryUpstream decrypts the secret, converts the body if needed, calls the
// upstream, and converts the response/stream back to the client's IN format.
func (p *Proxy) tryUpstream(ctx context.Context, provider *domain.Provider, issued *domain.IssuedKey,
	key *domain.UpstreamKey, req ProxyRequest, outFormat domain.Format) (*ProxyResult, error) {

	secret, err := p.secrets.Decrypt(ctx, key.SecretEnc)
	if err != nil {
		return nil, fmt.Errorf("%w: decrypt: %v", domain.ErrUpstreamUnavailable, err)
	}

	// Convert request body IN→OUT (no-op when formats match).
	body := req.Body
	if p.converter != nil && p.converter.NeedsConversion(req.InFormat, outFormat) {
		body, err = p.converter.ConvertRequest(ctx, req.Body, req.InFormat, outFormat)
		if err != nil {
			return nil, err
		}
	}

	upReq := ports.UpstreamRequest{
		Method: req.Method, Path: pathForFormat(req.Path, outFormat, req.IsModels),
		Format: outFormat, Body: body, Stream: req.Stream,
	}

	// Retry transient upstream errors with exponential backoff (§4.4).
	// ponytail: ceiling — 2 retries, base 200ms×2^n; no jitter. Growth path =
	// bounded jitter + per-upstream retry budget. Only ErrUpstreamUnavailable
	// (network/5xx/429) is retried; client-class errors surface immediately.
	var up *ports.UpstreamResponse
	err = retry(ctx, 2, 200*time.Millisecond, func() error {
		var derr error
		up, derr = p.client.Do(ctx, key.BaseURL, string(secret), upReq)
		return derr
	})
	if err != nil {
		return nil, err
	}

	res := &ProxyResult{StatusCode: up.StatusCode, Header: jsonHeader(), OutFormat: req.InFormat}
	// ucap accumulates usage parsed from the stream as it flows, so OnDone
	// (fired by the transport at EOF) reports accurate output tokens for streams.
	ucap := &usageCapture{}
	res.UCap = ucap
	res.Usage = ports.Usage{PromptTokens: up.PromptTokens, CompletionTokens: up.CompletionTokens}
	ucap.set(res.Usage)

	if up.Stream != nil {
		// Streaming: wrap the body through the converter if formats differ.
		if p.converter != nil && p.converter.NeedsConversion(outFormat, req.InFormat) {
			// Cross-format: the converter parses the terminal usage chunk.
			pr, pw := io.Pipe()
			go func() {
				u, _ := p.converter.ConvertStream(ctx, up.Stream, pw, outFormat, req.InFormat)
				_ = up.Stream.Close()
				_ = pw.Close()
				ucap.set(u)
			}()
			res.Stream = pr
		} else {
			// Same-format pass-through: parse usage from the SSE stream as it
			// flows so we still account output tokens (blocker #4).
			res.Stream = newUsageParsingReader(up.Stream, outFormat, ucap)
		}
		if p.guardEnabled() {
			res.Stream = inspect.NewStreamGuard(res.Stream, p.guard, p.guardMode == "block", func(fs []inspect.Finding) { p.logFindings(true, fs) })
		}
		return res, nil
	}

	// Non-stream: convert OUT→IN (no-op when matching).
	respBody := up.Body
	if p.converter != nil && p.converter.NeedsConversion(outFormat, req.InFormat) {
		respBody, err = p.converter.ConvertResponse(ctx, up.Body, outFormat, req.InFormat)
		if err != nil {
			return nil, err
		}
	}
	if p.guardEnabled() {
		if fs := inspect.ScanResponse(p.guard, respBody, string(req.InFormat)); len(fs) > 0 {
			p.logFindings(false, fs)
			if p.guardMode == "block" {
				inspect.SortFindings(fs)
				return nil, fmt.Errorf("%w: response blocked by security guard (rule %s, %s)", domain.ErrUpstreamClientError, fs[0].RuleID, fs[0].Severity)
			}
		}
	}
	res.Body = respBody
	res.Usage = ports.Usage{PromptTokens: up.PromptTokens, CompletionTokens: up.CompletionTokens}
	return res, nil
}

// chooseOutFormat prefers an upstream key whose native format matches the
// incoming request (avoids conversion). ponytail: simple first-match; growth
// path = a smarter cost model in the selector.
func chooseOutFormat(in domain.Format, keys []domain.UpstreamKey) domain.Format {
	for _, k := range keys {
		if k.Format == in {
			return in
		}
	}
	if len(keys) > 0 {
		return keys[0].Format
	}
	return in
}

// pathForFormat maps the incoming path to the upstream-native path.
// /v1/chat/completions ↔ /v1/messages across formats.
func pathForFormat(_ string, out domain.Format, isModels bool) string {
	if isModels {
		return "/v1/models"
	}
	if out == domain.FormatAnthropic {
		return "/v1/messages"
	}
	return "/v1/chat/completions"
}

// handleModels aggregates the provider's known model list into an OpenAI-shaped
// /v1/models response. ponytail: ceiling — does not live-fetch upstreams' model
// lists per call (that's the admin "auto-search" job, §4.9); growth path = cache
// those lists and merge here.
func (p *Proxy) handleModels(ctx context.Context, provider *domain.Provider) *ProxyResult {
	keys, _ := p.upstreams.ListByProvider(ctx, provider.ID)
	seen := map[string]struct{}{}
	var data []map[string]string
	add := func(list []string) {
		for _, m := range list {
			if m == "" {
				continue
			}
			if _, ok := seen[m]; ok {
				continue
			}
			seen[m] = struct{}{}
			data = append(data, map[string]string{"id": m, "object": "model"})
		}
	}
	add(provider.GlobalModels)
	for _, k := range keys {
		add(k.EffectiveModels(provider.GlobalModels))
	}
	body, _ := json.Marshal(map[string]any{"object": "list", "data": data})
	return &ProxyResult{StatusCode: 200, Header: jsonHeader(), Body: body, OutFormat: domain.FormatOpenAI}
}

// --- logging helpers ---

func (p *Proxy) recordSuccess(ctx context.Context, issued *domain.IssuedKey, provider *domain.Provider,
	key *domain.UpstreamKey, req ProxyRequest, res *ProxyResult, start time.Time) {
	// TTFB measured at the transport's first Write (res.ttfbAt); TotalMs is the
	// whole request lifetime up to OnDone. These differ meaningfully for streams
	// (§4.11 / audit #11) — previously both were computed at the same instant.
	var ttfb int64
	res.ttfbMu.Lock()
	if !res.ttfbAt.IsZero() {
		ttfb = res.ttfbAt.Sub(start).Milliseconds()
	}
	res.ttfbMu.Unlock()
	log := &domain.RequestLog{
		IssuedKeyID: issued.ID, ProviderID: provider.ID, Model: res.model,
		InFormat: res.inFormat, OutFormat: res.outFormat, Success: true,
		PromptTokens: res.Usage.PromptTokens, CompletionTokens: res.Usage.CompletionTokens,
		TotalTokens: res.Usage.PromptTokens + res.Usage.CompletionTokens,
		LatencyTTFBMs: ttfb,
		TotalMs: time.Since(start).Milliseconds(), Timestamp: time.Now().UTC(),
		Streamed: res.streamed,
	}
	if key != nil {
		log.UpstreamKeyID = key.ID
	}
	if p.logPayload {
		log.Payload = p.buildPayload(req.Body, res.Body)
	}
	_ = p.logs.Insert(ctx, log)
}

func (p *Proxy) recordFailure(ctx context.Context, issued *domain.IssuedKey, provider *domain.Provider,
	req ProxyRequest, cause error, start time.Time) {
	code := "error"
	if cause != nil {
		code = cause.Error()
	}
	log := &domain.RequestLog{
		IssuedKeyID: issued.ID, ProviderID: provider.ID, Model: extractModel(req.Body),
		InFormat: req.InFormat, OutFormat: req.InFormat, Success: false, ErrorCode: code,
		TotalMs: time.Since(start).Milliseconds(), Timestamp: time.Now().UTC(),
	}
	if p.logPayload {
		// On failure we capture only the request body (no upstream response).
		log.Payload = p.buildPayload(req.Body, nil)
	}
	_ = p.logs.Insert(ctx, log)
}

// buildPayload captures the request/response bodies with secrets masked (§4.7).
// Bodies are truncated to keep the DB row bounded.
func (p *Proxy) buildPayload(reqBody, respBody []byte) *domain.PayloadSnapshot {
	const cap = 64 * 1024
	snap := &domain.PayloadSnapshot{}
	if len(reqBody) > 0 {
		snap.RequestBody = truncate(string(maskSecrets(reqBody)), cap)
	}
	if len(respBody) > 0 {
		snap.ResponseBody = truncate(string(maskSecrets(respBody)), cap)
	}
	if snap.RequestBody == "" && snap.ResponseBody == "" {
		return nil
	}
	return snap
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}

// --- request inspection helpers ---

// extractModel pulls "model" from a JSON body without full decode (best effort).
func extractModel(body []byte) string {
	var v struct {
		Model string `json:"model"`
	}
	_ = json.Unmarshal(body, &v)
	return v.Model
}

// extractUsageFromBody parses token usage from a fully-buffered response body
// in the given (client/IN) format. Used to account cache hits (audit #3).
func extractUsageFromBody(body []byte, format domain.Format) ports.Usage {
	var v struct {
		Usage struct {
			PromptTokens     int64 `json:"prompt_tokens"`
			CompletionTokens int64 `json:"completion_tokens"`
			InputTokens      int64 `json:"input_tokens"`
			OutputTokens     int64 `json:"output_tokens"`
		} `json:"usage"`
	}
	if json.Unmarshal(body, &v) != nil {
		return ports.Usage{}
	}
	if format == domain.FormatAnthropic {
		return ports.Usage{PromptTokens: v.Usage.InputTokens, CompletionTokens: v.Usage.OutputTokens}
	}
	return ports.Usage{PromptTokens: v.Usage.PromptTokens, CompletionTokens: v.Usage.CompletionTokens}
}

// estimateTokens is a rough pre-call token estimate for limit checks.
// ponytail: ceiling — naive char/4 heuristic; actual usage is recorded post-call.
// Growth path = a real tokenizer (tiktoken) for tight limit enforcement.
func estimateTokens(body []byte) int64 {
	// Count payload size as a proxy; messages dominate the body.
	return int64(len(body) / 4)
}

// hashToken is defined in issued_svc.go (single source of truth, same package).

func jsonHeader() http.Header {
	h := http.Header{}
	h.Set("Content-Type", "application/json")
	return h
}

// retry calls fn up to maxRetries+1 times, sleeping base×2^n between attempts,
// but only retrying on ErrUpstreamUnavailable (transient). Other errors return
// immediately. Respects context cancellation.
func retry(ctx context.Context, maxRetries int, base time.Duration, fn func() error) error {
	var err error
	for attempt := 0; attempt <= maxRetries; attempt++ {
		err = fn()
		if err == nil {
			return nil
		}
		if !errors.Is(err, domain.ErrUpstreamUnavailable) {
			return err
		}
		if attempt == maxRetries {
			return err
		}
		// ponytail: ceiling — no jitter; bounded to maxRetries. The sleep is
		// skipped if the context is already done.
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(base * time.Duration(1<<uint(attempt))):
		}
	}
	return err
}

// rewriteModel returns a copy of body with the "model" field replaced by `m`.
// Used for fallback-model retry (§4.4). Cheap JSON rewrite via marshal.
func rewriteModel(body []byte, m string) []byte {
	var v map[string]any
	if json.Unmarshal(body, &v) != nil {
		return body
	}
	v["model"] = m
	out, err := json.Marshal(v)
	if err != nil {
		return body
	}
	return out
}

// cacheKey builds a stable exact-match cache key from the issued key + body.
// ponytail: sha256 — collisions are cryptographically negligible; the body is
// already trusted JSON from a validated client.
func cacheKey(issuedID domain.ID, body []byte) string {
	sum := sha256.Sum256(append([]byte(issuedID+":"), body...))
	return hex.EncodeToString(sum[:])
}

// maskSecrets rewrites obvious secret-shaped string values in a JSON payload
// to a masked tail (§4.7 payload logging). Best-effort: scans string values for
// keys named like sk-/Bearer/api_key and masks them.
// ponytail: ceiling — name-based heuristic; may miss custom secret fields.
// Growth path = structured field masking per known schema.
func maskSecrets(body []byte) []byte {
	var v any
	if json.Unmarshal(body, &v) != nil {
		return body
	}
	maskWalk(v)
	out, err := json.Marshal(v)
	if err != nil {
		return body
	}
	return out
}

func maskWalk(v any) {
	switch t := v.(type) {
	case map[string]any:
		for k, val := range t {
			if isSecretKey(k) {
				if s, ok := val.(string); ok {
					t[k] = maskTail(s)
				}
				continue
			}
			maskWalk(val)
		}
	case []any:
		for _, e := range t {
			maskWalk(e)
		}
	}
}

func isSecretKey(k string) bool {
	lk := strings.ToLower(k)
	switch lk {
	case "api_key", "apikey", "secret", "authorization", "token", "key", "password":
		return true
	}
	return false
}

func maskTail(s string) string {
	if len(s) <= 4 {
		return "****"
	}
	return "…" + s[len(s)-4:]
}