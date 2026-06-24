package sqlite

import (
	"database/sql"
	"fmt"
)

// schema is the single source of truth for the DB shape (§8).
// One file = easier review; migrations are idempotent CREATE statements.
const schema = `
CREATE TABLE IF NOT EXISTS users (
    id            TEXT PRIMARY KEY,
    login         TEXT NOT NULL UNIQUE,
    password_hash TEXT NOT NULL,
    created_at    TEXT NOT NULL,
    updated_at    TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS sessions (
    token      TEXT PRIMARY KEY,
    user_id    TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    created_at TEXT NOT NULL,
    expires_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS providers (
    id              TEXT PRIMARY KEY,
    name            TEXT NOT NULL,
    strategy        TEXT NOT NULL DEFAULT 'failover',
    global_models   TEXT NOT NULL DEFAULT '[]',
    fallback_models TEXT NOT NULL DEFAULT '[]',
    status          TEXT NOT NULL DEFAULT 'active',
    created_at      TEXT NOT NULL,
    updated_at      TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS upstream_keys (
    id                TEXT PRIMARY KEY,
    provider_id       TEXT NOT NULL REFERENCES providers(id) ON DELETE CASCADE,
    name              TEXT NOT NULL,
    base_url          TEXT NOT NULL,
    format            TEXT NOT NULL,
    secret_enc        BLOB NOT NULL,
    models            TEXT NOT NULL DEFAULT '[]',
    use_global_models INTEGER NOT NULL DEFAULT 0,
    priority          INTEGER NOT NULL DEFAULT 0,
    status            TEXT NOT NULL DEFAULT 'active',
    lim_tokens        TEXT NOT NULL DEFAULT '{}',
    lim_requests      TEXT NOT NULL DEFAULT '{}',
    consec_failures   INTEGER NOT NULL DEFAULT 0,
    cooldown_until    TEXT NOT NULL DEFAULT '',
    created_at        TEXT NOT NULL,
    updated_at        TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_upstream_provider ON upstream_keys(provider_id);

CREATE TABLE IF NOT EXISTS issued_keys (
    id           TEXT PRIMARY KEY,
    provider_id  TEXT NOT NULL REFERENCES providers(id) ON DELETE CASCADE,
    name         TEXT NOT NULL,
    prefix       TEXT NOT NULL,
    token_hash   TEXT NOT NULL UNIQUE,
    valid_days   INTEGER NOT NULL DEFAULT 0,
    lim_tokens   TEXT NOT NULL DEFAULT '{}',
    lim_requests TEXT NOT NULL DEFAULT '{}',
    status       TEXT NOT NULL DEFAULT 'active',
    created_at   TEXT NOT NULL,
    expires_at   TEXT NOT NULL DEFAULT '',
    revoked_at   TEXT NOT NULL DEFAULT '',
    last_used_at TEXT NOT NULL DEFAULT ''
);
CREATE INDEX IF NOT EXISTS idx_issued_provider ON issued_keys(provider_id);
CREATE INDEX IF NOT EXISTS idx_issued_token ON issued_keys(token_hash);

CREATE TABLE IF NOT EXISTS request_logs (
    id                TEXT PRIMARY KEY,
    issued_key_id     TEXT NOT NULL,
    provider_id       TEXT NOT NULL,
    upstream_key_id   TEXT NOT NULL DEFAULT '',
    model             TEXT NOT NULL DEFAULT '',
    in_format         TEXT NOT NULL,
    out_format        TEXT NOT NULL,
    prompt_tokens     INTEGER NOT NULL DEFAULT 0,
    completion_tokens INTEGER NOT NULL DEFAULT 0,
    total_tokens      INTEGER NOT NULL DEFAULT 0,
    success           INTEGER NOT NULL DEFAULT 0,
    error_code        TEXT NOT NULL DEFAULT '',
    latency_ttfb_ms   INTEGER NOT NULL DEFAULT 0,
    total_ms          INTEGER NOT NULL DEFAULT 0,
    streamed          INTEGER NOT NULL DEFAULT 0,
    timestamp         TEXT NOT NULL,
    payload_request   TEXT NOT NULL DEFAULT '',
    payload_response  TEXT NOT NULL DEFAULT ''
);
CREATE INDEX IF NOT EXISTS idx_log_key_ts ON request_logs(issued_key_id, timestamp);
CREATE INDEX IF NOT EXISTS idx_log_ts ON request_logs(timestamp);
CREATE INDEX IF NOT EXISTS idx_log_provider ON request_logs(provider_id);

CREATE TABLE IF NOT EXISTS mcp_servers (
    id         TEXT PRIMARY KEY,
    name       TEXT NOT NULL,
    kind       TEXT NOT NULL,
    transport  TEXT NOT NULL,
    address    TEXT NOT NULL,
    tools      TEXT NOT NULL DEFAULT '[]',
    status     TEXT NOT NULL DEFAULT 'active',
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS checker_runs (
    id          TEXT PRIMARY KEY,
    upstream_id TEXT NOT NULL DEFAULT '',
    base_url    TEXT NOT NULL,
    secret_tail TEXT NOT NULL DEFAULT '',
    started_at  TEXT NOT NULL,
    results     TEXT NOT NULL DEFAULT '[]'
);
CREATE INDEX IF NOT EXISTS idx_checker_started ON checker_runs(started_at);

CREATE TABLE IF NOT EXISTS prompt_rules (
    id         TEXT PRIMARY KEY,
    name       TEXT NOT NULL,
    kind       TEXT NOT NULL,
    value      TEXT NOT NULL DEFAULT '',
    param      TEXT NOT NULL DEFAULT '',
    status     TEXT NOT NULL DEFAULT 'active',
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL
);
`

func migrate(db *sql.DB) error {
	if _, err := db.Exec(schema); err != nil {
		return fmt.Errorf("migrate: %w", err)
	}
	return nil
}
