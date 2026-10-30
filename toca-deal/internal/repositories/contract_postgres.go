package repositories

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
	"github.com/oginoino/toca/pkg/database"
	"github.com/oginoino/toca/toca-deal/internal/domain/contract"
)

type contractPostgresRepository struct {
	db *database.PostgresDB
}

func NewContractPostgresRepository(db *database.PostgresDB) contract.Repository {
	return &contractPostgresRepository{db: db}
}

func (r *contractPostgresRepository) Create(ctx context.Context, c *contract.Contract) error {
	if c.ID == "" {
		c.ID = uuid.NewString()
	}
	c.CreatedAt = time.Now()
	metadata, _ := json.Marshal(c)
	query := `INSERT INTO deal.contracts (id, tenant_id, deal_id, metadata) VALUES ($1, $2, $3, $4) RETURNING created_at`
	return r.db.QueryRowContext(ctx, query, c.ID, c.TenantID, c.DealID, string(metadata)).Scan(&c.CreatedAt)
}

func (r *contractPostgresRepository) GetByID(ctx context.Context, tenantID, id string) (*contract.Contract, error) {
	var metadata []byte
	err := r.db.QueryRowContext(ctx, "SELECT metadata FROM deal.contracts WHERE id = $1 AND tenant_id = $2", id, tenantID).Scan(&metadata)
	if err != nil {
		return nil, fmt.Errorf("contract not found: %w", err)
	}
	var c contract.Contract
	_ = json.Unmarshal(metadata, &c)
	return &c, nil
}

func (r *contractPostgresRepository) GetByDealID(ctx context.Context, tenantID, dealID string) ([]*contract.Contract, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT metadata
		FROM deal.contracts
		WHERE tenant_id = $1 AND deleted_at IS NULL AND deal_id = $2
		ORDER BY created_at DESC
	`, tenantID, dealID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]*contract.Contract, 0)
	for rows.Next() {
		var metadata []byte
		if err := rows.Scan(&metadata); err != nil {
			return nil, err
		}
		var c contract.Contract
		if err := json.Unmarshal(metadata, &c); err != nil {
			return nil, err
		}
		out = append(out, &c)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

func (r *contractPostgresRepository) Update(ctx context.Context, c *contract.Contract) error {
	c.UpdatedAt = time.Now()
	metadata, _ := json.Marshal(c)
	// B12: predicado de tenant como defesa em profundidade.
	res, err := r.db.ExecContext(ctx, "UPDATE deal.contracts SET metadata = $1 WHERE id = $2 AND tenant_id = $3", string(metadata), c.ID, c.TenantID)
	if err != nil {
		return err
	}
	if n, rowsErr := res.RowsAffected(); rowsErr == nil && n == 0 {
		return fmt.Errorf("contract not updated: id=%s tenant=%s not found", c.ID, c.TenantID)
	}
	return nil
}

// Compare the revision timestamp so a regeneration and a send cannot both
// overwrite the same draft. Tenant and soft-delete filters are mandatory.
func (r *contractPostgresRepository) UpdateIfUnchanged(ctx context.Context, c *contract.Contract, expectedUpdatedAt time.Time) error {
	c.UpdatedAt = time.Now()
	metadata, err := json.Marshal(c)
	if err != nil {
		return err
	}
	result, err := r.db.ExecContext(ctx, `UPDATE deal.contracts SET metadata = $1
		WHERE id = $2 AND tenant_id = $3 AND deleted_at IS NULL
		AND COALESCE(metadata->>'updated_at', '0001-01-01T00:00:00Z') = $4`,
		string(metadata), c.ID, c.TenantID, expectedUpdatedAt.Format(time.RFC3339Nano))
	if err != nil {
		return err
	}
	n, err := result.RowsAffected()
	if err == nil && n != 1 {
		return contract.ErrConcurrentModification
	}
	return err
}

// UpdateIfUnchangedInTx keeps signature transitions and their outbox atomic.
func (r *contractPostgresRepository) UpdateIfUnchangedInTx(ctx context.Context, tx *sqlx.Tx, c *contract.Contract, expected time.Time) error {
	c.UpdatedAt = time.Now()
	metadata, err := json.Marshal(c)
	if err != nil {
		return err
	}
	result, err := tx.ExecContext(ctx, `UPDATE deal.contracts SET metadata=$1
 WHERE id=$2 AND tenant_id=$3 AND deleted_at IS NULL
 AND COALESCE(metadata->>'updated_at', '0001-01-01T00:00:00Z')=$4`, string(metadata), c.ID, c.TenantID, expected.Format(time.RFC3339Nano))
	if err != nil {
		return err
	}
	n, err := result.RowsAffected()
	if err == nil && n != 1 {
		return contract.ErrConcurrentModification
	}
	return err
}

// UpdateInTx mirrors Update but executes against the caller-supplied
// transaction, so the contract row update can be batched with an
// EnqueueOutboxInTx call in the same RunInTx callback (audit P0.1).
func (r *contractPostgresRepository) UpdateInTx(ctx context.Context, tx *sqlx.Tx, c *contract.Contract) error {
	if tx == nil {
		return fmt.Errorf("transaction is nil; use Update for non-tx callers")
	}
	c.UpdatedAt = time.Now()
	metadata, _ := json.Marshal(c)
	res, err := tx.ExecContext(ctx, "UPDATE deal.contracts SET metadata = $1 WHERE id = $2 AND tenant_id = $3", string(metadata), c.ID, c.TenantID)
	if err != nil {
		return err
	}
	if n, rowsErr := res.RowsAffected(); rowsErr == nil && n == 0 {
		return fmt.Errorf("contract not updated in tx: id=%s tenant=%s not found", c.ID, c.TenantID)
	}
	return nil
}

func (r *contractPostgresRepository) Delete(ctx context.Context, id, tenantID string) error {
	_, err := r.db.ExecContext(ctx, "UPDATE deal.contracts SET deleted_at = NOW() WHERE id = $1 AND tenant_id = $2 AND deleted_at IS NULL", id, tenantID)
	return err
}

// ListByPropertyID returns every non-deleted contract whose metadata
// references the given PropertyID. Uses the service pool so the sweep
// covers all tenants in one query — the property.updated subscriber needs
// to find contracts wherever they live since the event originates from
// toca-core, which has no per-tenant scoping at this stage.
func (r *contractPostgresRepository) ListByPropertyID(ctx context.Context, propertyID string) ([]*contract.Contract, error) {
	rows, err := r.db.QueryServiceRows(ctx, `
		SELECT metadata
		FROM deal.contracts
		WHERE deleted_at IS NULL
		  AND (metadata->>'property_id') = $1
		ORDER BY created_at DESC
	`, propertyID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]*contract.Contract, 0)
	for rows.Next() {
		var metadata []byte
		if err := rows.Scan(&metadata); err != nil {
			return nil, err
		}
		var c contract.Contract
		if err := json.Unmarshal(metadata, &c); err != nil {
			return nil, err
		}
		out = append(out, &c)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

// ListAutoAdjustDue returns every active contract with auto_adjust=true and
// next_adjustment <= today. Cross-tenant by design — the auto-readjustment
// scheduler runs in main.go without a tenant context. Uses the service pool
// (BYPASSRLS) for the same reason as ListByPropertyID.
func (r *contractPostgresRepository) ListAutoAdjustDue(ctx context.Context, today time.Time) ([]*contract.Contract, error) {
	dateStr := today.Format("2006-01-02")
	rows, err := r.db.QueryServiceRows(ctx, `
		SELECT metadata
		FROM deal.contracts
		WHERE deleted_at IS NULL
		  AND (metadata->>'auto_adjust')::boolean = true
		  AND (metadata->>'next_adjustment') IS NOT NULL
		  AND (metadata->>'next_adjustment') <> ''
		  AND (metadata->>'next_adjustment') <= $1
		  AND (metadata->>'status') IN ('SIGNED','ACTIVE')
	`, dateStr)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]*contract.Contract, 0)
	for rows.Next() {
		var metadata []byte
		if err := rows.Scan(&metadata); err != nil {
			return nil, err
		}
		var c contract.Contract
		if err := json.Unmarshal(metadata, &c); err != nil {
			return nil, err
		}
		out = append(out, &c)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

// ListMissingCRMSnapshot returns every non-deleted contract whose denormalized
// CRM snapshot is missing at least one of client/property_id/customer_id/value.
// Runs on the service pool so the backfill cmd can sweep every tenant in one
// pass; when tenantID is non-empty, narrows to that tenant only.
func (r *contractPostgresRepository) ListMissingCRMSnapshot(ctx context.Context, tenantID string) ([]*contract.Contract, error) {
	args := []any{}
	query := `
		SELECT metadata
		FROM deal.contracts
		WHERE deleted_at IS NULL
		  AND (
		    COALESCE(metadata->>'client', '') = ''
		    OR COALESCE(metadata->>'property_id', '') = ''
		    OR COALESCE(metadata->>'customer_id', '') = ''
		    OR COALESCE((metadata->>'value')::float, 0) = 0
		  )
	`
	if tenantID != "" {
		query += " AND tenant_id = $1"
		args = append(args, tenantID)
	}
	query += " ORDER BY created_at DESC"

	rows, err := r.db.QueryServiceRows(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]*contract.Contract, 0)
	for rows.Next() {
		var metadata []byte
		if err := rows.Scan(&metadata); err != nil {
			return nil, err
		}
		var c contract.Contract
		if err := json.Unmarshal(metadata, &c); err != nil {
			return nil, err
		}
		out = append(out, &c)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

func (r *contractPostgresRepository) List(ctx context.Context, tenantID string, status *contract.Status) ([]*contract.Contract, error) {
	args := []any{tenantID}
	query := `
		SELECT metadata
		FROM deal.contracts
		WHERE tenant_id = $1 AND deleted_at IS NULL
	`
	if status != nil && *status != "" {
		query += " AND (metadata->>'status') = $2"
		args = append(args, string(*status))
	}
	query += " ORDER BY created_at DESC"

	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]*contract.Contract, 0)
	for rows.Next() {
		var metadata []byte
		if err := rows.Scan(&metadata); err != nil {
			return nil, err
		}
		var c contract.Contract
		if err := json.Unmarshal(metadata, &c); err != nil {
			return nil, err
		}
		out = append(out, &c)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}
