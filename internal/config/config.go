// Package config loads runtime configuration from the environment (12-factor).
// One struct, one loader, no magic.
package config

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

// Config is the whole runtime configuration surface.
type Config struct {
	MasterKey        []byte
	DBPath           string
	Listen           string
	PublicURL        string
	AdminLogin       string
	AdminPassword    string
	IssuedKeyPrefix  string
	LogLevel         string
	CacheEnabled     bool
	CacheTTL         time.Duration
	// LimitFail controls limiter behavior when usage reads error: "closed"
	// (default, refuse — protect upstream money) or "open" (permissive).
	LimitFail        string
	// LogPayload enables full request/response body capture in request logs.
	// Secrets are masked before storage (§4.7). Default off (privacy).
	LogPayload       bool
	// MaxBodyBytes caps inbound request body size (defense against huge payloads).
	MaxBodyBytes     int64
	// GuardMode controls inspection of upstream RESPONSES for malicious-provider
	// injection (tool-call / shell-command smuggling): "block" (default, refuse
	// the response), "alert" (forward but log), or "off". Security default = block.
	GuardMode        string
}

// Load reads env vars and returns a populated Config or an error.
// MasterKey is mandatory and validated for length (AES-256 needs 32 bytes).
func Load() (Config, error) {
	c := Config{
		DBPath:          envStr("ARBUZ_DB_PATH", "./data/arbuz.db"),
		Listen:          envStr("ARBUZ_LISTEN", ":8080"),
		PublicURL:       strings.TrimRight(envStr("ARBUZ_PUBLIC_URL", "http://localhost:8080"), "/"),
		AdminLogin:      envStr("ARBUZ_ADMIN_LOGIN", "admin"),
		AdminPassword:   strings.TrimSpace(os.Getenv("ARBUZ_ADMIN_PASSWORD")),
		IssuedKeyPrefix: envStr("ARBUZ_ISSUED_KEY_PREFIX", "sk-arbuz"),
		LogLevel:        strings.ToLower(envStr("ARBUZ_LOG_LEVEL", "info")),
		CacheEnabled:    envBool("ARBUZ_CACHE_ENABLED", true),
		CacheTTL:        time.Duration(envInt("ARBUZ_CACHE_TTL_SECONDS", 300)) * time.Second,
		LimitFail:       strings.ToLower(envStr("ARBUZ_LIMIT_FAIL", "closed")),
		LogPayload:      envBool("ARBUZ_LOG_PAYLOAD", false),
		MaxBodyBytes:    int64(envInt("ARBUZ_MAX_BODY_BYTES", 16*1024*1024)),
		GuardMode:       strings.ToLower(envStr("ARBUZ_GUARD_MODE", "block")),
	}

	key, err := loadMasterKey(os.Getenv("ARBUZ_MASTER_KEY"))
	if err != nil {
		return Config{}, fmt.Errorf("master key: %w", err)
	}
	c.MasterKey = key

	// Security: never seed the admin with a missing or weak/placeholder password.
	// Previously this defaulted silently to "changeme", which shipped a known
	// credential on any direct-binary run. Require an explicit, non-trivial value.
	if err := validateAdminPassword(c.AdminPassword); err != nil {
		return Config{}, err
	}
	return c, nil
}

// validateAdminPassword rejects empty, too-short, or well-known placeholder
// passwords. ponytail: a simple denylist + length floor; growth path = configurable
// policy / zxcvbn-style strength scoring.
func validateAdminPassword(p string) error {
	if p == "" {
		return errors.New("ARBUZ_ADMIN_PASSWORD is required (no default; set a strong value)")
	}
	if len(p) < 8 {
		return errors.New("ARBUZ_ADMIN_PASSWORD too short (min 8 characters)")
	}
	switch strings.ToLower(p) {
	case "changeme", "password", "admin", "secret", "12345678":
		return errors.New("ARBUZ_ADMIN_PASSWORD is a well-known placeholder; choose a unique value")
	}
	return nil
}

// loadMasterKey accepts raw 32 bytes, 64 hex chars, or 44 base64 chars.
// Empty input is rejected (security: never run without encryption).
func loadMasterKey(s string) ([]byte, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil, errors.New("ARBUZ_MASTER_KEY is required (openssl rand -hex 32)")
	}
	// ponytail: we accept hex (common) and base64, plus raw 32 bytes.
	if len(s) == 64 {
		if b, err := hex.DecodeString(s); err == nil && len(b) == 32 {
			return b, nil
		}
	}
	if b, err := base64.StdEncoding.DecodeString(s); err == nil && len(b) == 32 {
		return b, nil
	}
	if len(s) == 32 {
		return []byte(s), nil
	}
	return nil, fmt.Errorf("must be 32 bytes, 64 hex chars, or 44 base64 chars (got %d)", len(s))
}

func envStr(key, def string) string {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return v
	}
	return def
}

// envBool parses a boolean env var. Accepts strconv.ParseBool forms (1/0/true/false)
// plus the friendly "enabled"/"disabled" and "on"/"off" used in .env.example.
// On any unrecognized value it falls back to def.
func envBool(key string, def bool) bool {
	v := strings.ToLower(strings.TrimSpace(os.Getenv(key)))
	if v == "" {
		return def
	}
	switch v {
	case "enabled", "on", "yes":
		return true
	case "disabled", "off", "no":
		return false
	}
	b, err := strconv.ParseBool(v)
	if err != nil {
		return def
	}
	return b
}

func envInt(key string, def int) int {
	v := strings.TrimSpace(os.Getenv(key))
	if v == "" {
		return def
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return def
	}
	return n
}

// GenerateMasterKey is a helper exposed for tooling/scripts: returns 64 hex chars.
func GenerateMasterKey() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}