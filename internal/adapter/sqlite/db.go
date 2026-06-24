// Package sqlite implements all ports.*Repo interfaces over a single SQLite
// database file via modernc.org/sqlite (pure-Go, CGO-free for easy Docker).
package sqlite

import (
	"database/sql"
	"fmt"
	"os"
	"runtime"
	"strconv"
	"time"

	_ "modernc.org/sqlite" // driver registration
)

// maxOpenConns decides the SQLite connection-pool ceiling.
//
// History: this was hard-coded to runtime.NumCPU()*4. On small VPSes that
// report many vCPUs but have little RAM, that opened dozens of modernc.org
// connections — each one allocates memory-mapped WAL/shm structures — and the
// very first db.Ping() crashed the process with
//   "ping sqlite: unable to open database file: out of memory (14)"
// in a restart loop. We now default to a small, memory-safe pool and allow an
// explicit override via ARBUZ_DB_MAX_CONNS for hosts that have spare RAM.
func maxOpenConns() int {
	if v := os.Getenv("ARBUZ_DB_MAX_CONNS"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			return n
		}
	}
	n := runtime.NumCPU()
	if n > 4 {
		n = 4 // cap: SQLite has a single writer; more readers rarely help here
	}
	if n < 2 {
		n = 2
	}
	return n
}

// Open opens (or creates) the database at path and runs migrations.
// ponytail: ceiling — single writer concurrency is SQLite's model; the driver
// serializes writes. Growth path = Postgres (§10.4) by swapping this adapter.
func Open(path string) (*sql.DB, error) {
	// _pragma busy_timeout: avoid "database is locked" under concurrency.
	// _pragma foreign_keys: enforce FK integrity.
	// _pragma journal_mode=WAL: better concurrency + durability.
	// _pragma mmap_size(0): disable memory-mapped I/O. modernc.org/sqlite's
	//   mmap path is the usual source of the spurious "out of memory (14)"
	//   (SQLITE_CANTOPEN) failures on low-RAM / containerized hosts; plain
	//   read()/write() is slightly slower but rock-solid.
	dsn := "file:" + path + "?_pragma=busy_timeout(5000)&_pragma=foreign_keys(1)&_pragma=journal_mode(WAL)&_pragma=mmap_size(0)"
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}
	// WAL allows many concurrent readers alongside a single writer. We keep a
	// small, memory-safe pool (see maxOpenConns); write contention is handled
	// by busy_timeout(5000) above (the driver retries "database is locked").
	// ponytail: fixed-size pool; growth path = split read/write pools or Postgres.
	moc := maxOpenConns()
	db.SetMaxOpenConns(moc)
	idle := moc / 2
	if idle < 1 {
		idle = 1
	}
	db.SetMaxIdleConns(idle)
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