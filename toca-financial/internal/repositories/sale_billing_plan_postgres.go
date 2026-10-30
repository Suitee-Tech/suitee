package repositories

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/oginoino/toca/pkg/database"
	"github.com/oginoino/toca/toca-financial/internal/domain/sale_billing_plan"
)

type saleBillingPlanPostgresRepository struct {
	db *database.PostgresDB
}

func NewSaleBillingPlanPostgresRepository(db *database.PostgresDB) sale_billing_plan.Repository {
	return &saleBillingPlanPostgresRepository{db: db}
}

func (r *saleBillingPlanPostgresRepository) Create(ctx context.Context, plan *sale_billing_plan.SaleBillingPlan) (*sale_billing_plan.SaleBillingPlan, error) {
	if plan == nil {
		return nil, errors.New("sale billing plan is required")
	}
	plan.CreatedAt = time.Now()
	metadata, err := json.Marshal(plan)
	if err != nil {
		return nil, fmt.Errorf("encode sale billing plan: %w", err)
	}

	// A SALE contract has exactly one active billing plan. The database is the
	// serialization point: concurrent contract.signed deliveries may construct
	// different transient IDs, but only one row wins this partial unique key.
	query := `INSERT INTO financial.sale_billing_plan (id, tenant_id, metadata)
		VALUES ($1, $2, $3)
		ON CONFLICT (
			tenant_id,
			(NULLIF(metadata->>'contract_id', ''))
		)
		WHERE deleted_at IS NULL
		  AND NULLIF(metadata->>'contract_id', '') IS NOT NULL
		DO NOTHING
		RETURNING created_at`
	if err := r.db.QueryRow(ctx, query, plan.ID, plan.TenantID, string(metadata)).Scan(&plan.CreatedAt); err == nil {
		return plan, nil
	} else if !errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("create sale billing plan: %w", err)
	}

	// ON CONFLICT waits for the winning transaction. Reading in a subsequent
	// statement gives this transaction a fresh READ COMMITTED snapshot and
	// recovers the canonical row instead of leaking the losing transient ID.
	canonical, err := r.GetByContractID(ctx, plan.TenantID, plan.ContractID)
	if err != nil {
		return nil, fmt.Errorf("recover canonical sale billing plan: %w", err)
	}
	if canonical == nil {
		return nil, errors.New("canonical sale billing plan missing after contract conflict")
	}
	return canonical, nil
}

func (r *saleBillingPlanPostgresRepository) GetByID(ctx context.Context, tenantID, id string) (*sale_billing_plan.SaleBillingPlan, error) {
	var metadata []byte
	err := r.db.QueryRow(ctx,
		"SELECT metadata FROM financial.sale_billing_plan WHERE id = $1 AND tenant_id = $2 AND deleted_at IS NULL",
		id, tenantID,
	).Scan(&metadata)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("sale_billing_plan not found: %w", err)
	}
	var p sale_billing_plan.SaleBillingPlan
	if err := json.Unmarshal(metadata, &p); err != nil {
		return nil, err
	}
	return &p, nil
}

func (r *saleBillingPlanPostgresRepository) GetByContractID(ctx context.Context, tenantID, contractID string) (*sale_billing_plan.SaleBillingPlan, error) {
	var metadata []byte
	err := r.db.QueryRow(ctx, `
		SELECT metadata
		FROM financial.sale_billing_plan
		WHERE tenant_id = $1
		  AND deleted_at IS NULL
		  AND NULLIF(metadata->>'contract_id', '') = $2
		ORDER BY created_at ASC, id ASC
		LIMIT 1
	`, tenantID, contractID).Scan(&metadata)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("sale_billing_plan lookup failed: %w", err)
	}
	var p sale_billing_plan.SaleBillingPlan
	if err := json.Unmarshal(metadata, &p); err != nil {
		return nil, err
	}
	return &p, nil
}

func (r *saleBillingPlanPostgresRepository) Update(ctx context.Context, plan *sale_billing_plan.SaleBillingPlan) error {
	plan.UpdatedAt = time.Now()
	metadata, err := json.Marshal(plan)
	if err != nil {
		return fmt.Errorf("encode sale billing plan: %w", err)
	}
	result, err := r.db.Exec(ctx,
		"UPDATE financial.sale_billing_plan SET metadata = $1, updated_at = NOW() WHERE id = $2 AND tenant_id = $3 AND deleted_at IS NULL",
		string(metadata), plan.ID, plan.TenantID,
	)
	if err != nil {
		return err
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rows != 1 {
		return fmt.Errorf("sale billing plan not found")
	}
	return nil
}

func (r *saleBillingPlanPostgresRepository) Delete(ctx context.Context, tenantID, id string) error {
	_, err := r.db.Exec(ctx,
		"UPDATE financial.sale_billing_plan SET deleted_at = NOW() WHERE id = $1 AND tenant_id = $2 AND deleted_at IS NULL",
		id, tenantID,
	)
	return err
}
