package store

import "errors"

// Sentinel errors returned by store implementations.
// Use errors.Is to check; wrap with %w to add context.
var (
	// ErrNotFound is returned when a lookup finds no matching record.
	ErrNotFound = errors.New("store: not found")

	// ErrAlreadyExists is returned when a unique constraint would be violated
	// (e.g. slug collision, duplicate idempotency key).
	ErrAlreadyExists = errors.New("store: already exists")

	// ErrVersionMismatch is returned when an optimistic-concurrency update
	// observes a stale Version (e.g. on PolicyRecord).
	ErrVersionMismatch = errors.New("store: version mismatch")

	// ErrNoCredential is returned by CredentialStore.ResolveCredential when
	// no active credential matches the filter. Callers may decide whether
	// to fall through to pass-through mode.
	ErrNoCredential = errors.New("store: no credential")
)
