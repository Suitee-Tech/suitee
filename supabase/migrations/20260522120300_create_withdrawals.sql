-- Agency withdrawals ("saque da imobiliária") — each maps to one Abacate Pay
-- payout. JSONB metadata mirrors financial.charges / financial.payments.
--
-- See the domain in toca-financial/internal/domain/withdrawal for the struct
-- shape.

CREATE TABLE IF NOT EXISTS financial.withdrawals (
    id          uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id   text NOT NULL,
    metadata    jsonb NOT NULL DEFAULT '{}'::jsonb,
    created_at  timestamptz NOT NULL DEFAULT now(),
    updated_at  timestamptz NOT NULL DEFAULT now(),
    deleted_at  timestamptz
);

CREATE INDEX IF NOT EXISTS idx_finance_withdrawals_tenant
    ON financial.withdrawals (tenant_id)
    WHERE deleted_at IS NULL;

CREATE INDEX IF NOT EXISTS idx_finance_withdrawals_tenant_status
    ON financial.withdrawals (tenant_id, ((metadata->>'status')))
    WHERE deleted_at IS NULL;

-- The payout webhook resolves a withdrawal by the external id it echoes back.
CREATE INDEX IF NOT EXISTS idx_finance_withdrawals_external_id
    ON financial.withdrawals ((metadata->>'external_id'))
    WHERE deleted_at IS NULL;
