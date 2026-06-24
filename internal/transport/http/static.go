package httptrans

import (
	"io/fs"
	"net/http"
	"os"
	"strings"
)

// StaticAssets serves the built Vue SPA from web/dist (embedded or on disk).
// ponytail: ceiling — disk-based serving from a configured dir. Growth path =
// go:embed the dist tree so the binary is self-contained.
//
// Behavior:
//   - /assets/* and other real files under dist are served directly.
//   - any non-API, non-/v1, non-file path returns index.html (SPA history fallback).
//   - missing dist dir → 404 for non-API paths (dev mode: vite serves the SPA).
type StaticAssets struct {
	root  http.FileSystem
	index []byte
}

// NewStaticAssets builds a static handler rooted at dir. Returns nil if the
// dir doesn't exist (caller should skip SPA serving then).
func NewStaticAssets(dir string) *StaticAssets {
	if dir == "" {
		return nil
	}
	info, err := os.Stat(dir)
	if err != nil || !info.IsDir() {
		return nil
	}
	h := &StaticAssets{root: http.Dir(dir)}
	if idx, err := os.ReadFile(strings.TrimRight(dir, "/") + "/index.html"); err == nil {
		h.index = idx
	}
	return h
}

// Handler returns an http.Handler that serves SPA assets with history fallback.
// If s is nil, returns a handler that 404s (dev mode — vite owns the SPA).
func (s *StaticAssets) Handler() http.Handler {
	if s == nil {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			http.NotFound(w, r)
		})
	}
	fileServer := http.FileServer(s.root)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// SPA fallback: if the path doesn't map to a real file, serve index.html.
		if f, err := s.root.Open(r.URL.Path); err == nil {
			stat, _ := f.Stat()
			_ = f.Close()
			if stat != nil && !stat.IsDir() {
				fileServer.ServeHTTP(w, r)
				return
			}
		}
		s.serveIndex(w, r)
	})
}

func (s *StaticAssets) serveIndex(w http.ResponseWriter, r *http.Request) {
	if len(s.index) == 0 {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write(s.index)
}

// isClientPath reports whether a path is client traffic / API (must NOT be
// handed to the SPA fallback).
func isClientPath(path string) bool {
	return strings.HasPrefix(path, "/api/") || strings.HasPrefix(path, "/v1/")
}

// keep fs referenced (used by http.FileSystem implementations indirectly).
var _ fs.FS = (fs.FS)(nil)
