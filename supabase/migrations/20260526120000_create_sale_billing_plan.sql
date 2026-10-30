-- financial.sale_billing_plan — billing plan for sale contracts. Holds the
-- payment mode (SINGLE | INSTALLMENTS), the installment count + first due
-- date + amount-per-installment, plus the Abacate Pay product backing the
-- pre-generated charges.
--
-- For RENT contracts there is already financial.rental_charge_configs; this
-- is its sibling for SALE contracts. Charges are pre-generated at
-- contract.signed time (see SaleBillingPlanService.CreateAndGenerateCharges)
-- with deterministic source_event_id ("sale.{plan_id}.installment.{i}") for
-- idempotency, leveraging the partial unique index established by migration
-- 20260518120000.
--
-- Domain: toca-financial/internal/domain/sale_billing_plan/sale_billing_plan.go
-- Repository: toca-financial/internal/repository/sale_billing_plan_postgres.go

CREATE TABLE IF NOT EXISTS financial.sale_billing_plan (
    id          uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id   text NOT NULL,
    metadata    jsonb NOT NULL DEFAULT '{}'::jsonb,
    created_at  timestamptz NOT NULL DEFAULT now(),
    updated_at  timestamptz NOT NULL DEFAULT now(),
    deleted_at  timestamptz
);

CREATE INDEX IF NOT EXISTS idx_finance_sale_billing_plan_tenant
    ON financial.sale_billing_plan (tenant_id)
    WHERE deleted_at IS NULL;

CREATE INDEX IF NOT EXISTS idx_finance_sale_billing_plan_contract
    ON financial.sale_billing_plan (tenant_id, ((metadata->>'contract_id')))
    WHERE deleted_at IS NULL AND (metadata->>'contract_id') IS NOT NULL;

CREATE INDEX IF NOT EXISTS idx_finance_sale_billing_plan_deal
    ON financial.sale_billing_plan (tenant_id, ((metadata->>'deal_id')))
    WHERE deleted_at IS NULL AND (metadata->>'deal_id') IS NOT NULL;
