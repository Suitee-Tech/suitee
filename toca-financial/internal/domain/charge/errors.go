package charge

import "errors"

// Sentinel errors for the charge domain. Handlers compare against these via
// errors.Is so the HTTP status mapping survives any rewording of the error
// message at the service or repository layer.
var (
	// ErrNotFound is returned by service lookups when no charge matches the
	// (tenantID, id) tuple. The HTTP layer maps it to 404.
	ErrNotFound = errors.New("charge not found")

	// ErrUnauthorized is returned when the caller cannot act on a charge they
	// do not own (kept for forward compatibility; tenant isolation is
	// currently enforced at the repository layer, which collapses to
	// ErrNotFound). The HTTP layer maps it to 403.
	ErrUnauthorized = errors.New("unauthorized")

	// ErrInvalidState is wrapped by service operations that are rejected
	// because the charge's lifecycle status forbids them (e.g. paying a
	// CANCELLED charge, notifying a PAID one). The HTTP layer maps it to
	// 400/422 depending on caller convention.
	ErrInvalidState = errors.New("charge in invalid state for this operation")

	// ErrConcurrentModification is returned when another payment changed the
	// same charge between read and write. The service retries this conflict so
	// independently arriving payments cannot overwrite one another.
	ErrConcurrentModification = errors.New("charge was modified concurrently")
)
