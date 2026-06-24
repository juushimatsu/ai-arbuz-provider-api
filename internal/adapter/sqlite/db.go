// Package sqlite implements all ports.*Repo interfaces over a single SQLite
// database file via modernc.org/sqlite (pure-Go, CGO-free for easy Docker).
package sqlite

import (
	"database/sql"
	"fmt"
	"runtime"
	"time"

	_ "modernc.org/sqlite" // driver registration
)

// Open opens (or creates) the database at path and runs migrations.
// ponytail: ceiling — single writer concurrency is SQLite's model; the driver
// serializes writes. Growth path = Postgres (§10.4) by swapping this adapter.
func Open(path string) (*sql.DB, error) {
	// _pragma busy_timeout: avoid "database is locked" under concurrency.
	// _pragma foreign_keys: enforce FK integrity.
	// _pragma journal_mode=WAL: better concurrency + durability.
	dsn := "file:" + path + "?_pragma=busy_timeout(5000)&_pragma=foreign_keys(1)&_pragma=journal_mode(WAL)"
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}
	// WAL allows many concurrent readers alongside a single writer. Capping the
	// pool at 1 (the previous behavior) serialized reads too and negated WAL,
	// becoming a throughput bottleneck under proxy load. Allow a small pool of
	// connections for concurrent reads; write contention is handled by
	// busy_timeout(5000) above (the driver retries "database is locked").
	// ponytail: fixed-size pool; growth path = split read/write pools or Postgres.
	db.SetMaxOpenConns(runtime.NumCPU() * 4)
	db.SetMaxIdleConns(runtime.NumCPU())
	db.SetConnMaxIdleTime(5 * time.Minute)
	if err := db.Ping(); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("ping sqlite: %w", err)
	}
	if err := migrate(db); err != nil {
		_ = db.Close()
		return nil, err
	}
	return db, nil
}