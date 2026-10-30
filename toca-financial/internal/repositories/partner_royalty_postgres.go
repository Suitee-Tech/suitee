package repositories

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/oginoino/toca/pkg/database"
	"github.com/oginoino/toca/toca-financial/internal/domain/partner_royalty"
)

// partnerRoyaltyPostgresRepository persists franchise royalty payables.
// financial.partner_royalties keeps the money fields as first-class numeric
// columns and also stores the full entity in metadata jsonb — same hybrid
// pattern as the rest of financial.*.
type partnerRoyaltyPostgresRepository struct {
	db *database.PostgresDB
}

func NewPartnerRoyaltyPostgresRepository(db *database.PostgresDB) *partnerRoyaltyPostgresRepository {
	return &partnerRoyaltyPostgresRepository{db: db}
}

func (r *partnerRoyaltyPostgresRepository) Create(ctx context.Context, pr *partner_royalty.PartnerRoyalty) error {
	if pr.CreatedAt.IsZero() {
		pr.CreatedAt = time.Now()
	}
	pr.UpdatedAt = time.Now()
	metadata, err := json.Marshal(pr)
	if err != nil {
		return fmt.Errorf("encode partner royalty metadata: %w", err)
	}
	if pr.SourceEventID != "" {
		query := `
			INSERT INTO financial.partner_royalties
				(id, tenant_id, partnership_id, deal_id, contract_id, payer_tenant, payee_tenant,
				 base_amount, percentage, amount, status, metadata, updated_at)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, NOW())
			ON CONFLICT (tenant_id, ((metadata->>'source_event_id')))
			WHERE deleted_at IS NULL AND (metadata->>'source_event_id') IS NOT NULL
			DO NOTHING
			RETURNING created_at
		`
		row := r.db.QueryRow(ctx, query,
			pr.ID, pr.TenantID, pr.PartnershipID, pr.DealID, pr.ContractID,
			pr.PayerTenant, pr.PayeeTenant, pr.BaseAmount, pr.Percentage, pr.Amount,
			string(pr.Status), string(metadata),
		)
		if err := row.Scan(&pr.CreatedAt); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return partner_royalty.ErrPartnerRoyaltyAlreadyExists
			}
			return err
		}
		return nil
	}
	query := `
		INSERT INTO financial.partner_royalties
			(id, tenant_id, partnership_id, deal_id, contract_id, payer_tenant, payee_tenant,
			 base_amount, percentage, amount, status, metadata, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, NOW())
		RETURNING created_at
	`
	return r.db.QueryRow(ctx, query,
		pr.ID, pr.TenantID, pr.PartnershipID, pr.DealID, pr.ContractID,
		pr.PayerTenant, pr.PayeeTenant, pr.BaseAmount, pr.Percentage, pr.Amount,
		string(pr.Status), string(metadata),
	).Scan(&pr.CreatedAt)
}

func (r *partnerRoyaltyPostgresRepository) GetByID(ctx context.Context, tenantID, id string) (*partner_royalty.PartnerRoyalty, error) {
	var metadata []byte
	var persistedStatus string
	var persistedUpdatedAt time.Time
	err := r.db.QueryRow(ctx,
		"SELECT metadata, status, updated_at FROM financial.partner_royalties WHERE id = $1 AND tenant_id = $2 AND deleted_at IS NULL",
		id, tenantID,
	).Scan(&metadata, &persistedStatus, &persistedUpdatedAt)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("partner royalty not found: %w", err)
	}
	var pr partner_royalty.PartnerRoyalty
	if err := json.Unmarshal(metadata, &pr); err != nil {
		return nil, fmt.Errorf("decode partner royalty %s: %w", id, err)
	}
	pr.Status = partner_royalty.Status(persistedStatus)
	pr.UpdatedAt = persistedUpdatedAt
	return &pr, nil
}

func (r *partnerRoyaltyPostgresRepository) GetBySourceEventID(ctx context.Context, tenantID, sourceEventID string) (*partner_royalty.PartnerRoyalty, error) {
	if tenantID == "" || sourceEventID == "" {
		return nil, nil
	}
	var metadata []byte
	var persistedStatus string
	var persistedUpdatedAt time.Time
	err := r.db.QueryRow(ctx,
		"SELECT metadata, status, updated_at FROM financial.partner_royalties WHERE tenant_id = $1 AND (metadata->>'source_event_id') = $2 AND deleted_at IS NULL ORDER BY created_at DESC LIMIT 1",
		tenantID, sourceEventID,
	).Scan(&metadata, &persistedStatus, &persistedUpdatedAt)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("partner royalty lookup by source_event_id failed: %w", err)
	}
	var pr partner_royalty.PartnerRoyalty
	if err := json.Unmarshal(metadata, &pr); err != nil {
		return nil, fmt.Errorf("decode partner royalty for source_event_id %s: %w", sourceEventID, err)
	}
	pr.Status = partner_royalty.Status(persistedStatus)
	pr.UpdatedAt = persistedUpdatedAt
	return &pr, nil
}

// UpdateIfStatus keeps the first-class status column and JSON aggregate in
// lockstep and rejects both lifecycle and same-status revision races.
func (r *partnerRoyaltyPostgresRepository) UpdateIfStatus(ctx context.Context, pr *partner_royalty.PartnerRoyalty, expected partner_royalty.Status, expectedUpdatedAt time.Time) error {
	if pr == nil {
		return errors.New("partner royalty is required")
	}
	pr.UpdatedAt = time.Now().UTC().Truncate(time.Microsecond)
	metadata, err := json.Marshal(pr)
	if err != nil {
		return fmt.Errorf("encode partner royalty %s for transition: %w", pr.ID, err)
	}
	result, err := r.db.Exec(ctx, `
		UPDATE financial.partner_royalties
		SET status = $1, metadata = $2, updated_at = $6
		WHERE id = $3
		  AND tenant_id = $4
		  AND deleted_at IS NULL
		  AND status = $5
		  AND updated_at = $7
	`, string(pr.Status), string(metadata), pr.ID, pr.TenantID, string(expected), pr.UpdatedAt, expectedUpdatedAt)
	if err != nil {
		return err
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rows != 1 {
		return partner_royalty.ErrConcurrentTransition
	}
	return nil
}

func (r *partnerRoyaltyPostgresRepository) List(ctx context.Context, tenantID string, limit, offset int64) ([]*partner_royalty.PartnerRoyalty, int64, error) {
	if limit <= 0 {
		limit = 10
	}
	if offset < 0 {
		offset = 0
	}

	var total int64
	if err := r.db.QueryRow(ctx,
		"SELECT COUNT(1) FROM financial.partner_royalties WHERE tenant_id = $1 AND deleted_at IS NULL",
		tenantID,
	).Scan(&total); err != nil {
		return nil, 0, err
	}

	rows, err := r.db.Query(ctx, `
		SELECT metadata
		FROM financial.partner_royalties
		WHERE tenant_id = $1 AND deleted_at IS NULL
		ORDER BY created_at DESC
		LIMIT $2 OFFSET $3
	`, tenantID, limit, offset)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	out := make([]*partner_royalty.PartnerRoyalty, 0)
	for rows.Next() {
		var metadata []byte
		if err := rows.Scan(&metadata); err != nil {
			return nil, 0, err
		}
		var pr partner_royalty.PartnerRoyalty
		if err := json.Unmarshal(metadata, &pr); err != nil {
			return nil, 0, err
		}
		out = append(out, &pr)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, err
	}
	return out, total, nil
}

// ListPendingByPayerAndContract returns only the pending royalties belonging to
// the paying tenant and exact signed contract. Deal IDs are not globally unique
// enough to be the sole payout lookup key.
func (r *partnerRoyaltyPostgresRepository) ListPendingByPayerAndContract(ctx context.Context, payerTenant, dealID, contractID string) ([]map[string]any, error) {
	if payerTenant == "" || dealID == "" || contractID == "" {
		return nil, nil
	}
	rows, err := r.db.Query(ctx, `
		SELECT id, payee_tenant, percentage, amount, contract_id, status, COALESCE(metadata->>'kind','royalty')
		FROM financial.partner_royalties
		WHERE payer_tenant = $1
		  AND deal_id = $2
		  AND contract_id = $3
		  AND status = $4
		  AND deleted_at IS NULL
		ORDER BY created_at ASC
	`, payerTenant, dealID, contractID, string(partner_royalty.StatusPending))
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]map[string]any, 0)
	for rows.Next() {
		var id, payee, contractID, status, kind string
		var pct, amount float64
		if err := rows.Scan(&id, &payee, &pct, &amount, &contractID, &status, &kind); err != nil {
			return nil, err
		}
		out = append(out, map[string]any{
			"kind":         kind,
			"id":           id,
			"payee_tenant": payee,
			"percentage":   pct,
			"amount":       amount,
			"contract_id":  contractID,
			"status":       status,
		})
	}
	return out, rows.Err()
}
