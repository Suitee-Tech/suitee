-- Phase 6 (end-to-end financial) — per-tenant defaults that feed:
--   * delinquency_service (late_fee_pct / monthly_interest_pct fallback chain
--     contract → tenant → global)
--   * rental_charge_config_service (default_due_day when caller omits)
--   * charge_service manual creation (pre-fill multa/juros from tenant)
--   * event_handler contract.signed (default commission_pct_rent / _sale when
--     the contract leaves commission_pct blank)
--   * payout_order workflow (payout_proof_required toggles the "anexar
--     comprovante obrigatório" guard on POST /payout-orders/:id/mark-paid)
--   * abacate checkout (abacate_methods_default — array de "PIX","CARD")
--
-- We store the lot in metadata JSONB to stay aligned com o restante do
-- schema financial.*. Defaults applied at runtime (settings_service.Get
-- devolve um Settings populado quando a row é ausente — nada de seed.)
--
-- Domain: toca-financial/internal/domain/tenant_financial_settings/settings.go
-- Service: toca-financial/internal/service/tenant_financial_settings_service.go

CREATE TABLE IF NOT EXISTS financial.tenant_financial_settings (
    id          uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id   text NOT NULL UNIQUE,
    metadata    jsonb NOT NULL DEFAULT '{}'::jsonb,
    created_at  timestamptz NOT NULL DEFAULT now(),
    updated_at  timestamptz NOT NULL DEFAULT now(),
    deleted_at  timestamptz
);

CREATE INDEX IF NOT EXISTS idx_finance_tenant_financial_settings_tenant
    ON financial.tenant_financial_settings (tenant_id)
    WHERE deleted_at IS NULL;

-- Shape stored under metadata:
--   {
--     "default_late_fee_pct":          2.0,
--     "default_monthly_interest_pct":  1.0,
--     "default_due_day":               10,
--     "default_commission_pct_rent":   100.0,
--     "default_commission_pct_sale":   6.0,
--     "default_currency":              "BRL",
--     "payout_proof_required":         true,
--     "abacate_methods_default":       ["PIX","CARD"],
--     "updated_by":                    "<account_id>",
--     "updated_at":                    "<rfc3339>"
--   }
