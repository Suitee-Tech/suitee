// Package gdpr provides primitives to fulfil LGPD Art. 18 (right to be
// forgotten) requests across the platform.
//
// Strategy: irreversibly replace every PII field in user-linked rows
// with a deterministic hash of the original value, prefixed with
// "anon:". The replacement is deterministic per-user so audit/fiscal
// records can still join on the same key, but the underlying identity
// is unrecoverable (one-way SHA-256 with a server-side pepper).
//
// What this package does:
//   - hashIdentifier: produces the anonymized replacement
//   - Anonymizer: an interface that domain services implement to expose
//     their per-entity anonymization logic, keyed by user/account_id
//   - Registry: services register their anonymizers at boot; the LGPD
//     handler (in toca-auth) iterates the registry when a deletion
//     request is approved
//
// What this package does NOT do:
//   - schedule deletions (caller's job — typically a job dispatched off
//     a "lgpd.deletion_requested" event)
//   - delete fiscal records (those have a separate legal retention
//     obligation; only the *display* fields are anonymized)
package gdpr

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"sync"

	"github.com/oginoino/toca/pkg/logger"
)

const (
	// AnonPrefix marks values produced by HashIdentifier. UI / reports
	// can detect this prefix and render "<anonimizado>" instead of the
	// hash for end-user display.
	AnonPrefix = "anon:"
)

// HashIdentifier returns the deterministic anonymized replacement for
// the given raw value. The pepper from LGPD_ANON_PEPPER (or a stable
// default in dev) prevents rainbow-table attacks against the universe
// of known emails/CPFs.
//
// The output length is fixed (AnonPrefix + 32 hex chars) so it always
// fits in TEXT columns. Deterministic: same input always maps to the
// same output, which keeps audit/billing joins working.
func HashIdentifier(raw string) string {
	pepper := os.Getenv("LGPD_ANON_PEPPER")
	if pepper == "" {
		pepper = "toca-lgpd-default-pepper-rotate-me" // dev fallback only
	}
	sum := sha256.Sum256([]byte(pepper + ":" + raw))
	return AnonPrefix + hex.EncodeToString(sum[:16]) // 32 hex chars
}

// IsAnonymized reports whether the given value has already been
// anonymized. Used by domain services to avoid re-anonymizing on
// idempotent retries (a no-op should remain no-op).
func IsAnonymized(s string) bool {
	return len(s) > len(AnonPrefix) && s[:len(AnonPrefix)] == AnonPrefix
}

// Anonymizer is implemented by every service that owns user-linked
// data. The LGPD orchestrator (toca-auth/.../lgpd) invokes each
// registered anonymizer once per deletion request, passing the
// account_id of the user being forgotten.
//
// Anonymize MUST be idempotent — the same call with the same
// account_id should produce the same final state. Errors should be
// transient (DB unavailable, etc.); permanent failures (account_id
// doesn't exist in this domain) should return nil.
type Anonymizer interface {
	// Domain returns a short identifier ("crm.lead", "deal.party",
	// "auth.account") used for logging + the audit trail.
	Domain() string
	// Anonymize replaces PII fields referencing accountID with the
	// output of HashIdentifier. Returns the number of rows touched +
	// any persistent error.
	Anonymize(ctx context.Context, accountID string) (touched int, err error)
}

var (
	registryMu sync.RWMutex
	registry   = make([]Anonymizer, 0)
)

// Register adds an Anonymizer to the global registry. Safe to call
// from each service's main.go at boot. Duplicate registrations are
// ignored (matched by Domain()).
func Register(a Anonymizer) {
	if a == nil {
		return
	}
	registryMu.Lock()
	defer registryMu.Unlock()
	for _, existing := range registry {
		if existing.Domain() == a.Domain() {
			return
		}
	}
	registry = append(registry, a)
}

// All returns the registered anonymizers in registration order.
// Callers should iterate and call Anonymize on each.
func All() []Anonymizer {
	registryMu.RLock()
	defer registryMu.RUnlock()
	out := make([]Anonymizer, len(registry))
	copy(out, registry)
	return out
}

// Result summarises one anonymization run.
type Result struct {
	Domain      string
	RowsTouched int
	Err         error
}

// AnonymizeAccount runs every registered Anonymizer for the given
// account_id. Returns one Result per domain — partial failures are
// preserved so the caller can retry only what failed.
//
// Audit footprint: writes one log entry per domain (event=lgpd.anonymize)
// with the domain and row count. Combined with the pkg/audit entry
// emitted by the caller, this gives a complete trail of which user
// requested deletion, when, and what was touched.
func AnonymizeAccount(ctx context.Context, accountID string) []Result {
	if accountID == "" {
		return nil
	}
	anonymizers := All()
	out := make([]Result, 0, len(anonymizers))
	for _, a := range anonymizers {
		touched, err := a.Anonymize(ctx, accountID)
		r := Result{Domain: a.Domain(), RowsTouched: touched, Err: err}
		out = append(out, r)
		if err != nil {
			logger.ErrorContext(ctx, "lgpd.anonymize failed",
				"event", "lgpd.anonymize_failed",
				"domain", a.Domain(),
				"account_id", accountID,
				"error", err.Error(),
			)
			continue
		}
		logger.InfoContext(ctx, "lgpd.anonymize ok",
			"event", "lgpd.anonymize",
			"domain", a.Domain(),
			"account_id", accountID,
			"rows_touched", touched,
		)
	}
	return out
}

// SummaryString formats a Result slice for inclusion in admin / audit
// responses. Errors are joined with "; " so the caller can fit the
// summary in a JSON field.
func SummaryString(results []Result) string {
	if len(results) == 0 {
		return "no anonymizers registered"
	}
	out := ""
	for i, r := range results {
		if i > 0 {
			out += "; "
		}
		if r.Err != nil {
			out += fmt.Sprintf("%s=error(%s)", r.Domain, r.Err.Error())
			continue
		}
		out += fmt.Sprintf("%s=%d rows", r.Domain, r.RowsTouched)
	}
	return out
}
