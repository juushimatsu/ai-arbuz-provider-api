// Package cache implements ports.Cache as a bounded in-memory TTL store.
// Stdlib only. ponytail: ceiling — single-process; not shared across instances.
// Growth path = Redis adapter implementing the same ports.Cache interface.
package cache

import (
	"context"
	"sync"
	"time"
)

type entry struct {
	value []byte
	exp   time.Time
}

// defaultMaxEntries bounds memory so a flood of unique request bodies cannot
// grow the cache without limit (audit #4 DoS). ponytail: fixed cap + approx
// oldest-expiry eviction; growth path = true LRU or a Redis adapter.
const defaultMaxEntries = 10000

// Memory is a ports.Cache with TTL eviction on read + a periodic sweeper.
type Memory struct {
	mu      sync.RWMutex
	ttl     time.Duration
	enabled    bool
	maxEntries int
	items      map[string]entry
	stop       chan struct{}
}

// NewMemory builds a cache. ttl<=0 with enabled=false disables caching.
func NewMemory(enabled bool, ttl time.Duration) *Memory {
	m := &Memory{
		ttl:        ttl,
		enabled:    enabled,
		maxEntries: defaultMaxEntries,
		items:      make(map[string]entry),
		stop:       make(chan struct{}),
	}
	if enabled && ttl > 0 {
		go m.sweep()
	}
	return m
}

func (m *Memory) Get(_ context.Context, key string) ([]byte, bool) {
	if !m.enabled {
		return nil, false
	}
	m.mu.RLock()
	e, ok := m.items[key]
	m.mu.RUnlock()
	if !ok {
		return nil, false
	}
	if !e.exp.IsZero() && time.Now().After(e.exp) {
		m.mu.Lock()
		delete(m.items, key)
		m.mu.Unlock()
		return nil, false
	}
	return e.value, true
}

func (m *Memory) Set(_ context.Context, key string, value []byte) {
	if !m.enabled || m.ttl <= 0 {
		return
	}
	// copy to avoid aliasing caller's slice
	cp := make([]byte, len(value))
	copy(cp, value)
	m.mu.Lock()
	if _, exists := m.items[key]; !exists && len(m.items) >= m.maxEntries {
		m.evictLocked()
	}
	m.items[key] = entry{value: cp, exp: time.Now().Add(m.ttl)}
	m.mu.Unlock()
}

// ponytail: ceiling — sweep walks the whole map every ttl/2; O(n) periodic.
// Fine for a single-node self-hosted router; growth path = LRU + size cap.
func (m *Memory) sweep() {
	interval := m.ttl / 2
	if interval <= 0 {
		interval = time.Minute
	}
	t := time.NewTicker(interval)
	defer t.Stop()
	for {
		select {
		case <-m.stop:
			return
		case now := <-t.C:
			m.mu.Lock()
			for k, e := range m.items {
				if !e.exp.IsZero() && now.After(e.exp) {
					delete(m.items, k)
				}
			}
			m.mu.Unlock()
		}
	}
}

// evictLocked frees room when the cache is full: drop any expired entries
// first, and if still at capacity remove the entry with the earliest expiry
// (approximate oldest). Caller must hold m.mu.
func (m *Memory) evictLocked() {
	now := time.Now()
	for k, e := range m.items {
		if !e.exp.IsZero() && now.After(e.exp) {
			delete(m.items, k)
		}
	}
	if len(m.items) < m.maxEntries {
		return
	}
	var oldestKey string
	var oldestExp time.Time
	for k, e := range m.items {
		if oldestKey == "" || e.exp.Before(oldestExp) {
			oldestKey, oldestExp = k, e.exp
		}
	}
	if oldestKey != "" {
		delete(m.items, oldestKey)
	}
}

// Close stops the sweeper goroutine.
func (m *Memory) Close() { close(m.stop) }