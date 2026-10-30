package service

import (
	"context"
	"fmt"
	"os"

	"github.com/oginoino/toca/pkg/database"
	"github.com/oginoino/toca/pkg/gdpr"
)

// LeadAnonymizer implements gdpr.Anonymizer for crm.leads. Replaces the
// PII fields of every lead row pointing to the given account_id with
// the deterministic hash from gdpr.HashIdentifier — keeping the row
// (and therefore the deal/financial joins) intact while removing any
// way to identify the person.
//
// Fields anonymized (when populated):
//   - metadata->>'Name'
//   - metadata->>'Email'
//   - metadata->>'Phone'
//
// Fields preserved:
//   - id, tenant_id, status, AccountID (used as foreign key)
//   - history entries (operator audit trail)
//
// Idempotent: re-running for the same accountID is a no-op once the
// fields already start with "anon:".
type LeadAnonymizer struct {
	db *database.PostgresDB
}

func NewLeadAnonymizer(db *database.PostgresDB) *LeadAnonymizer {
	return &LeadAnonymizer{db: db}
}

func (a *LeadAnonymizer) Domain() string { return "crm.lead" }

func (a *LeadAnonymizer) Anonymize(ctx context.Context, accountID string) (int, error) {
	if a == nil || a.db == nil || a.db.ServicePool == nil {
		return 0, nil
	}
	if accountID == "" {
		return 0, fmt.Errorf("account_id required")
	}

	// Two-step so the deterministic hash from gdpr.HashIdentifier (used
	// elsewhere — auth.accounts, deal.parties) lines up with what we
	// write here. Computing the hash in Postgres would produce a
	// different output (different prefix/truncation), breaking any
	// cross-table correlation built on the anonymized identifier.
	const selectQuery = `
		SELECT id::text, tenant_id::text,
		       COALESCE(metadata->>'Name','')  AS name,
		       COALESCE(metadata->>'Email','') AS email,
		       COALESCE(metadata->>'Phone','') AS phone
		  FROM crm.leads
		 WHERE metadata->>'AccountID' = $1
		   AND deleted_at IS NULL
	`
	rows, err := a.db.ServicePool.Query(ctx, selectQuery, accountID)
	if err != nil {
		return 0, fmt.Errorf("scan crm.leads: %w", err)
	}
	defer rows.Close()

	type target struct {
		id, tenantID, name, email, phone string
	}
	var pending []target
	for rows.Next() {
		var t target
		if err := rows.Scan(&t.id, &t.tenantID, &t.name, &t.email, &t.phone); err != nil {
			return 0, fmt.Errorf("scan row: %w", err)
		}
		pending = append(pending, t)
	}
	if err := rows.Err(); err != nil {
		return 0, fmt.Errorf("rows iter: %w", err)
	}

	const updateQuery = `
		UPDATE crm.leads
		   SET metadata = metadata
		                  || jsonb_build_object('Name', $2::text, 'Email', $3::text, 'Phone', $4::text),
		       updated_at = NOW()
		 WHERE id = $1::uuid
	`
	touched := 0
	for _, t := range pending {
		newName := maybeHash(t.name)
		newEmail := maybeHash(t.email)
		newPhone := maybeHash(t.phone)

		// Skip when everything's already anonymized (idempotent re-run).
		if newName == t.name && newEmail == t.email && newPhone == t.phone {
			continue
		}
		if _, err := a.db.ServicePool.Exec(ctx, updateQuery, t.id, newName, newEmail, newPhone); err != nil {
			return touched, fmt.Errorf("update crm.leads %s: %w", t.id, err)
		}
		touched++
	}
	return touched, nil
}

// maybeHash returns the deterministic anon hash for non-empty values
// that haven't been anonymized yet. Empty input and already-anon values
// are returned as-is so idempotent re-runs don't re-hash.
func maybeHash(v string) string {
	if v == "" || gdpr.IsAnonymized(v) {
		return v
	}
	return gdpr.HashIdentifier(v)
}

var _ = os.Getenv // env consumed transitively by gdpr.HashIdentifier
