// Package domain is the business core: entities and rules, no DB/HTTP awareness.
// Errors here are sentinel domain errors so use-cases and transport can branch
// without leaking adapter types.
package domain

import "errors"

var (
	// ErrNotFound — entity does not exist.
	ErrNotFound = errors.New("not found")
	// ErrUnauthorized — bad credentials / missing session.
	ErrUnauthorized = errors.New("unauthorized")
	// ErrForbidden — authenticated but not allowed.
	ErrForbidden = errors.New("forbidden")
	// ErrConflict — unique constraint / state conflict.
	ErrConflict = errors.New("conflict")
	// ErrValidation — input failed validation at a trust boundary.
	ErrValidation = errors.New("validation")

	// ErrLimitExceeded — issued-key limit (tokens or requests) exceeded.
	// Returned to client as-is; NOT a failover trigger.
	ErrLimitExceeded = errors.New("limit exceeded")
	// ErrKeyRevoked — issued key was revoked.
	ErrKeyRevoked = errors.New("key revoked")
	// ErrKeyExpired — issued key past its validity window.
	ErrKeyExpired = errors.New("key expired")
	// ErrKeyPaused — issued key was paused by the admin (resumable).
	ErrKeyPaused = errors.New("key paused")
	// ErrNoUpstreamKey — provider has no usable upstream key (all disabled/failed).
	ErrNoUpstreamKey = errors.New("no upstream key available")

	// ErrUpstreamUnavailable — transient: upstream errored / rate-limited / timed out.
	// Triggers failover to the next key AND retry with backoff.
	ErrUpstreamUnavailable = errors.New("upstream unavailable")

	// ErrUpstreamAuth — the upstream key itself is bad/forbidden (401/403).
	// Key-level failure: we DO failover to another key, but we must NOT retry
	// the same key (it won't self-heal) — OnFailure escalates it to cooldown.
	ErrUpstreamAuth = errors.New("upstream auth error")

	// ErrUpstreamClientError — non-transient rejection caused by the REQUEST
	// itself (400/404/413/…). Does NOT trigger failover or retry: another key
	// would just reproduce the same client error. Surfaced to the client.
	ErrUpstreamClientError = errors.New("upstream client error")
)
