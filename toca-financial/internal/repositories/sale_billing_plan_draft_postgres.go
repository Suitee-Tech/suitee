package repositories

import (
	"context"

	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/jackc/pgx/v5"
	"time"

	"github.com/oginoino/toca/pkg/database"
	"github.com/oginoino/toca/toca-financial/internal/domain/sale_billing_plan_draft"
)

type saleBillingPlanDraftPostgresRepository struct {
	db *database.PostgresDB
}

func NewSaleBillingPlanDraftPostgresRepository(db *database.PostgresDB) sale_billing_plan_draft.Repository {
	return &saleBillingPlanDraftPostgresRepository{db: db}
}

// Upsert substitui o draft ativo se houver um para o mesmo (tenant,
// deal); caso contrário insere novo. Reusa o índice partial
// uq_finance_sbp_draft_deal (consumed_at IS NULL) como chave de
// conflito — ON CONFLICT DO UPDATE bumpa metadata + updated_at sem
// criar row duplicada nem abrir uma janela SELECT/INSERT concorrente.
func (r *saleBillingPlanDraftPostgresRepository) Upsert(ctx context.Context, d *sale_billing_plan_draft.Draft) error {
	d.UpdatedAt = time.Now()
	metadata, err := json.Marshal(d)
	if err != nil {
		return fmt.Errorf("sale_billing_plan_draft marshal failed: %w", err)
	}
	// PostgreSQL can infer an expression-based partial unique index when the
	// conflict target repeats both the indexed expression and its predicate.
	// Preserve the winning row id inside metadata as well as in the table key.
	return r.db.QueryRow(ctx, `
		INSERT INTO financial.sale_billing_plan_drafts AS existing_draft
			(id, tenant_id, metadata)
		VALUES ($1, $2, $3)
		ON CONFLICT (tenant_id, ((metadata->>'deal_id')))
		WHERE consumed_at IS NULL
		  AND (metadata->>'deal_id') IS NOT NULL
		DO UPDATE SET
			metadata = jsonb_set(EXCLUDED.metadata, '{id}', to_jsonb(existing_draft.id::text), true),
			updated_at = NOW()
		RETURNING id, created_at
	`, d.ID, d.TenantID, string(metadata)).Scan(&d.ID, &d.CreatedAt)
}

// GetActiveByDealForService atende as rotas INTERNAS, em que o chamador é um
// serviço (chave interna) e não há claims de JWT no contexto. A leitura no pool
// do app depende dessas claims: a linha existe, mas fica invisível, e o plano
// configurado era reportado como ausente — o contrato de venda era recusado com
// 422 mesmo com a forma de recebimento preenchida. O tenant continua vindo da
// credencial e entra na própria sentença.
func (r *saleBillingPlanDraftPostgresRepository) GetActiveByDealForService(ctx context.Context, tenantID, dealID string) (*sale_billing_plan_draft.Draft, error) {
	var metadata []byte
	err := r.db.QueryServiceRow(ctx, `
		SELECT metadata
		FROM financial.sale_billing_plan_drafts
		WHERE tenant_id = $1
		  AND (metadata->>'deal_id') = $2
		  AND consumed_at IS NULL
		LIMIT 1
	`, tenantID, dealID).Scan(&metadata)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("sale_billing_plan_draft lookup failed: %w", err)
	}
	var d sale_billing_plan_draft.Draft
	if err := json.Unmarshal(metadata, &d); err != nil {
		return nil, err
	}
	return &d, nil
}

func (r *saleBillingPlanDraftPostgresRepository) GetActiveByDeal(ctx context.Context, tenantID, dealID string) (*sale_billing_plan_draft.Draft, error) {
	var metadata []byte
	err := r.db.QueryRow(ctx, `
		SELECT metadata
		FROM financial.sale_billing_plan_drafts
		WHERE tenant_id = $1
		  AND (metadata->>'deal_id') = $2
		  AND consumed_at IS NULL
		LIMIT 1
	`, tenantID, dealID).Scan(&metadata)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("sale_billing_plan_draft lookup failed: %w", err)
	}
	var d sale_billing_plan_draft.Draft
	if err := json.Unmarshal(metadata, &d); err != nil {
		return nil, err
	}
	return &d, nil
}

func (r *saleBillingPlanDraftPostgresRepository) MarkConsumed(ctx context.Context, tenantID, dealID string) error {
	_, err := r.db.Exec(ctx, `
		UPDATE financial.sale_billing_plan_drafts
		SET consumed_at = NOW(), updated_at = NOW()
		WHERE tenant_id = $1
		  AND (metadata->>'deal_id') = $2
		  AND consumed_at IS NULL
	`, tenantID, dealID)
	return err
}
