-- financial.sale_billing_plan_drafts — billing plan choice gravada pelo
-- operador na esteira ANTES da assinatura do contrato. Quando o
-- contract.signed chega no NewContractSignedEventHandler, o branch SALE
-- consome este draft (lê + marca consumed_at), materializa o
-- SaleBillingPlan real, e o draft fica histórico. Sem draft o branch SALE
-- cai no fallback SINGLE (à vista) com warning visual no frontend.
--
-- Idempotente em (tenant_id, deal_id) via partial unique index com
-- consumed_at IS NULL — re-upsert antes do signed substitui a escolha;
-- após o signed o draft fica fora do índice e um novo deal pode ser
-- aberto sem colisão.
--
-- Domain: toca-financial/internal/domain/sale_billing_plan_draft/draft.go
-- Repository: toca-financial/internal/repository/sale_billing_plan_draft_postgres.go
-- Consumer: toca-financial/internal/service/sale_billing_plan_service.go
--           (CreateFromDraft chamado pelo event_handler.go branch SALE)

CREATE TABLE IF NOT EXISTS financial.sale_billing_plan_drafts (
    id           uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id    text NOT NULL,
    metadata     jsonb NOT NULL DEFAULT '{}'::jsonb,
    consumed_at  timestamptz,
    created_at   timestamptz NOT NULL DEFAULT now(),
    updated_at   timestamptz NOT NULL DEFAULT now()
);

-- Shape stored under metadata:
--   {
--     "deal_id":                    "<uuid>",
--     "mode":                       "SINGLE" | "INSTALLMENTS",
--     "total_amount_cents":         30000000,
--     "down_payment_cents":         5000000,
--     "installment_amount_cents":   2500000,
--     "installment_count":          10,
--     "first_due_date":             "2026-06-05T00:00:00Z",
--     "recurrence_interval_days":   30,
--     "composite": {
--       "saldo_amount_cents":       5000000,
--       "saldo_due_date":           "2026-12-05T00:00:00Z",
--       "saldo_description":        "Saldo financiamento bancário",
--       "dacao_amount_cents":       10000000,
--       "dacao_description":        "Imóvel em dação",
--       "troca_amount_cents":       0,
--       "troca_description":        ""
--     },
--     "created_by":                 "<account_id>"
--   }

CREATE INDEX IF NOT EXISTS idx_finance_sbp_draft_tenant
    ON financial.sale_billing_plan_drafts (tenant_id)
    WHERE consumed_at IS NULL;

-- Idempotência: 1 draft ativo por deal. Re-upsert substitui em vez de
-- duplicar; pós-consumo o draft sai do índice e o slot fica livre.
CREATE UNIQUE INDEX IF NOT EXISTS uq_finance_sbp_draft_deal
    ON financial.sale_billing_plan_drafts (tenant_id, ((metadata->>'deal_id')))
    WHERE consumed_at IS NULL AND (metadata->>'deal_id') IS NOT NULL;
