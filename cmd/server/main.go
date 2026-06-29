// Command server is the Ai Arbuz Provider Api entrypoint: loads config, wires
// the DI graph (domain → ports → adapters → transport), seeds the admin user,
// and serves HTTP (client traffic on /v1/*, panel API on /api/*).
package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/arbuz/ai-arbuz-provider-api/internal/adapter/cache"
	"github.com/arbuz/ai-arbuz-provider-api/internal/adapter/converter"
	"github.com/arbuz/ai-arbuz-provider-api/internal/adapter/crypto"
	mcpbridge "github.com/arbuz/ai-arbuz-provider-api/internal/adapter/mcp"
	"github.com/arbuz/ai-arbuz-provider-api/internal/adapter/sqlite"
	"github.com/arbuz/ai-arbuz-provider-api/internal/adapter/upstream"
	"github.com/arbuz/ai-arbuz-provider-api/internal/config"
	"github.com/arbuz/ai-arbuz-provider-api/internal/inspect"
	httptrans "github.com/arbuz/ai-arbuz-provider-api/internal/transport/http"
	"github.com/arbuz/ai-arbuz-provider-api/internal/usecase"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		slog.Error("config load failed", "err", err)
		os.Exit(1)
	}
	logger := newLogger(cfg.LogLevel)

	// Ensure the data directory exists (SQLite file lives there).
	if dir := filepath.Dir(cfg.DBPath); dir != "" {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			logger.Error("create data dir failed", "err", err)
			os.Exit(1)
		}
	}

	db, err := sqlite.Open(cfg.DBPath)
	if err != nil {
		logger.Error("open db failed", "err", err)
		os.Exit(1)
	}
	defer db.Close()

	// --- repositories (adapters) ---
	providerRepo := sqlite.NewProviderRepo(db)
	upstreamRepo := sqlite.NewUpstreamRepo(db)
	issuedRepo := sqlite.NewIssuedRepo(db)
	logRepo := sqlite.NewLogRepo(db)
	userRepo := sqlite.NewUserRepo(db)
	sessionRepo := sqlite.NewSessionRepo(db)
	mcpRepo := sqlite.NewMCPRepo(db)
	checkerRepo := sqlite.NewCheckerRepo(db)
	promptRuleRepo := sqlite.NewPromptRuleRepo(db)
	modelPrefRepo := sqlite.NewModelPrefRepo(db)

	// --- infrastructure adapters ---
	secretStore, err := crypto.NewAESGCM(cfg.MasterKey)
	if err != nil {
		logger.Error("init crypto failed", "err", err)
		os.Exit(1)
	}
	hasher := crypto.BcryptHasher{}
	upstreamClient := upstream.New(upstream.DefaultTimeouts())
	memCache := cache.NewMemory(cfg.CacheEnabled, cfg.CacheTTL)
	defer memCache.Close()

	// --- use cases ---
	auth := usecase.NewAuth(userRepo, sessionRepo, hasher)
	if err := auth.SeedAdmin(context.Background(), cfg.AdminLogin, cfg.AdminPassword); err != nil {
		logger.Error("seed admin failed", "err", err)
		os.Exit(1)
	}
	providers := usecase.NewProviderService(providerRepo)
	upstreams := usecase.NewUpstreamService(upstreamRepo, secretStore)
	issued := usecase.NewIssuedService(issuedRepo, cfg.IssuedKeyPrefix)
	logs := usecase.NewLogService(logRepo)

	limiter := usecase.NewRollingLimiter(logRepo).WithFailMode(usecase.FailMode(cfg.LimitFail))
	promptRules := usecase.NewPromptRuleService(promptRuleRepo)
	proxy := usecase.NewProxy(usecase.ProxyDeps{
		Issued: issuedRepo, Providers: providerRepo, Upstreams: upstreamRepo,
		Secrets: secretStore, Client: upstreamClient,
		Converter: converter.New(logger),
		Limiter: limiter,
		Selector: usecase.NewFailoverSelector(upstreamRepo),
		Cache: memCache, Logs: logRepo, LogPayload: cfg.LogPayload,
		ModelPrefs: modelPrefRepo,
	})
	// Load active prompt-transformation rules once at boot (§4.7).
	if active, err := promptRules.Active(context.Background()); err == nil {
		proxy.SetPromptRules(active)
	} else {
		logger.Warn("load prompt rules failed", "err", err)
	}

	// Response guard: inspect upstream replies for malicious-provider injection
	// (tool-call / shell-command smuggling). Default mode = block (config).
	if cfg.GuardMode != "off" {
		if guard, err := inspect.Default(); err == nil {
			proxy.SetGuard(guard, cfg.GuardMode)
			logger.Info("response guard enabled", "mode", cfg.GuardMode, "rules", guard.RuleCount())
		} else {
			logger.Warn("load response guard failed", "err", err)
		}
	}

	// --- transport ---
	srv := httptrans.NewServer(httptrans.Deps{
		Proxy: proxy, Auth: auth, Providers: providers, Upstreams: upstreams,
		Issued: issued, Logs: logs, Stats: sqlite.NewStats(db),
		MCP: mcpbridge.NewBridge(), MCPRepo: mcpRepo,
		Checker: upstreamClient, CheckerRepo: checkerRepo,
		ModelSearch: upstreamClient, PromptRules: promptRules,
		ModelPrefs: modelPrefRepo,
		Secrets: secretStore, Logger: logger,
		StaticDir: "./web/dist", MaxBodyBytes: cfg.MaxBodyBytes,
	})

	httpServer := &http.Server{
		Addr:              cfg.Listen,
		Handler:           srv.Wrap(srv),
		ReadHeaderTimeout: 10 * time.Second,
		// ReadTimeout bounds the time to read the full request (headers+body), so a
		// slow client cannot hold a connection open indefinitely (audit #13). Body
		// size is already capped by MaxBytesReader; this caps the time dimension.
		ReadTimeout: 60 * time.Second,
		// IdleTimeout reaps idle keep-alive connections (audit #13).
		IdleTimeout: 120 * time.Second,
		// WriteTimeout left 0 so SSE streams aren't cut; per-req context caps calls.
	}

	go func() {
		logger.Info("listening", "addr", cfg.Listen, "public_url", cfg.PublicURL)
		if err := httpServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			logger.Error("server failed", "err", err)
			os.Exit(1)
		}
	}()

	// Graceful shutdown on SIGINT/SIGTERM.
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	<-stop
	logger.Info("shutting down")
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_ = httpServer.Shutdown(ctx)
}

func newLogger(level string) *slog.Logger {
	var lv slog.Level
	switch level {
	case "debug":
		lv = slog.LevelDebug
	case "warn":
		lv = slog.LevelWarn
	case "error":
		lv = slog.LevelError
	default:
		lv = slog.LevelInfo
	}
	return slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: lv}))
}