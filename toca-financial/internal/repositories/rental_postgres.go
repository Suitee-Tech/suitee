package repositories

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/jmoiron/sqlx"
	"github.com/oginoino/toca/pkg/database"
	"github.com/oginoino/toca/toca-financial/internal/domain/rental"
)

type rentalPostgresRepository struct {
	db *database.PostgresDB
}

// NewRentalPostgresRepository persists Rental aggregates in
// `financial.rentals` (id, tenant_id, metadata, created_at, updated_at,
// deleted_at). Mirrors the JSONB-metadata pattern used across the module —
// see rental_charge_config_postgres.go for the same shape.
func NewRentalPostgresRepository(db *database.PostgresDB) rental.Repository {
	return &rentalPostgresRepository{db: db}
}

func (r *rentalPostgresRepository) Create(ctx context.Context, ren *rental.Rental) error {
	metadata, _ := json.Marshal(ren)
	query := `INSERT INTO financial.rentals (id, tenant_id, metadata) VALUES ($1, $2, $3) RETURNING created_at`
	return r.db.QueryRow(ctx, query, ren.ID, ren.TenantID, string(metadata)).Scan(&ren.CreatedAt)
}

// CreateInTx writes the rental using the caller-supplied transaction so
// the INSERT and a subsequent EnqueueOutboxInTx land or roll back
// together. See RentalService.CreateFromDeal for the pattern.
func (r *rentalPostgresRepository) CreateInTx(ctx context.Context, tx *sqlx.Tx, ren *rental.Rental) error {
	if tx == nil {
		return fmt.Errorf("transaction is nil; use Create for non-tx callers")
	}
	metadata, _ := json.Marshal(ren)
	query := `INSERT INTO financial.rentals (id, tenant_id, metadata) VALUES ($1, $2, $3) RETURNING created_at`
	return tx.QueryRowContext(ctx, query, ren.ID, ren.TenantID, string(metadata)).Scan(&ren.CreatedAt)
}

func (r *rentalPostgresRepository) GetByID(ctx context.Context, tenantID, id string) (*rental.Rental, error) {
	var metadata []byte
	err := r.db.QueryRow(ctx, `
		SELECT metadata FROM financial.rentals
		WHERE id = $1 AND tenant_id = $2 AND deleted_at IS NULL
	`, id, tenantID).Scan(&metadata)
	if err != nil {
		return nil, fmt.Errorf("rental not found: %w", err)
	}
	var ren rental.Rental
	if err := json.Unmarshal(metadata, &ren); err != nil {
		return nil, err
	}
	return &ren, nil
}

func (r *rentalPostgresRepository) GetByDealID(ctx context.Context, tenantID, dealID string) (*rental.Rental, error) {
	return r.lookupOne(ctx, `(metadata->>'deal_id') = $2`, tenantID, dealID)
}

func (r *rentalPostgresRepository) GetByContractID(ctx context.Context, tenantID, contractID string) (*rental.Rental, error) {
	return r.lookupOne(ctx, `(metadata->>'contract_id') = $2`, tenantID, contractID)
}

func (r *rentalPostgresRepository) lookupOne(ctx context.Context, predicate, tenantID, id string) (*rental.Rental, error) {
	var metadata []byte
	q := fmt.Sprintf(`
		SELECT metadata FROM financial.rentals
		WHERE tenant_id = $1 AND deleted_at IS NULL AND %s
		ORDER BY created_at DESC LIMIT 1
	`, predicate)
	if err := r.db.QueryRow(ctx, q, tenantID, id).Scan(&metadata); err != nil {
		return nil, fmt.Errorf("rental not found: %w", err)
	}
	var ren rental.Rental
	if err := json.Unmarshal(metadata, &ren); err != nil {
		return nil, err
	}
	return &ren, nil
}

func (r *rentalPostgresRepository) ListByLeadID(ctx context.Context, tenantID, leadID string, role rental.LeadRole) ([]*rental.Rental, error) {
	field := "tenant_lead_id"
	if role == rental.LeadRoleOwner {
		field = "owner_lead_id"
	}
	q := fmt.Sprintf(`
		SELECT metadata FROM financial.rentals
		WHERE tenant_id = $1 AND deleted_at IS NULL AND (metadata->>'%s') = $2
		ORDER BY created_at DESC
	`, field)
	return r.listMany(ctx, q, tenantID, leadID)
}

func (r *rentalPostgresRepository) ListByPropertyID(ctx context.Context, tenantID, propertyID string) ([]*rental.Rental, error) {
	q := `
		SELECT metadata FROM financial.rentals
		WHERE tenant_id = $1 AND deleted_at IS NULL AND (metadata->>'property_id') = $2
		ORDER BY created_at DESC
	`
	return r.listMany(ctx, q, tenantID, propertyID)
}

// ListForLeadIDs uses ANY($2) with a string-array parameter so a single
// query handles "every rental where this user is tenant or owner". Returns
// rentals deduplicated since the same lead could theoretically be both
// roles on different rentals — the OR clause handles both at once.
func (r *rentalPostgresRepository) ListForLeadIDs(ctx context.Context, tenantID string, leadIDs []string) ([]*rental.Rental, error) {
	if len(leadIDs) == 0 {
		return []*rental.Rental{}, nil
	}
	rows, err := r.db.Query(ctx, `
		SELECT metadata FROM financial.rentals
		WHERE tenant_id = $1
		  AND deleted_at IS NULL
		  AND (
		    (metadata->>'tenant_lead_id') = ANY($2)
		    OR (metadata->>'owner_lead_id') = ANY($2)
		  )
		ORDER BY created_at DESC
	`, tenantID, leadIDs)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*rental.Rental
	for rows.Next() {
		var metadata []byte
		if err := rows.Scan(&metadata); err != nil {
			return nil, err
		}
		var ren rental.Rental
		if err := json.Unmarshal(metadata, &ren); err != nil {
			return nil, err
		}
		out = append(out, &ren)
	}
	return out, rows.Err()
}

func (r *rentalPostgresRepository) listMany(ctx context.Context, query, tenantID, secondArg string) ([]*rental.Rental, error) {
	rows, err := r.db.Query(ctx, query, tenantID, secondArg)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*rental.Rental
	for rows.Next() {
		var metadata []byte
		if err := rows.Scan(&metadata); err != nil {
			return nil, err
		}
		var ren rental.Rental
		if err := json.Unmarshal(metadata, &ren); err != nil {
			return nil, err
		}
		out = append(out, &ren)
	}
	return out, rows.Err()
}

func (r *rentalPostgresRepository) Update(ctx context.Context, ren *rental.Rental) error {
	metadata, _ := json.Marshal(ren)
	_, err := r.db.Exec(ctx, `
		UPDATE financial.rentals SET metadata = $1, updated_at = NOW()
		WHERE id = $2 AND tenant_id = $3 AND deleted_at IS NULL
	`, string(metadata), ren.ID, ren.TenantID)
	return err
}
