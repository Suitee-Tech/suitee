-- Charge-backed owner transfers are idempotent by the paid charge, not by
-- calendar month. SALE plans can legitimately have two installments in the
-- same month (for example 01/Jan and 31/Jan); monthly uniqueness collapsed
-- the second transfer and underpaid the owner.

BEGIN;

CREATE UNIQUE INDEX IF NOT EXISTS uq_finance_transfers_source_charge
    ON financial.transfers (
        tenant_id,
        ((metadata->>'source_charge_id'))
    )
    WHERE deleted_at IS NULL
      AND NULLIF(metadata->>'source_charge_id', '') IS NOT NULL;

-- Preserve the old contract/month identity only for legacy and operator-made
-- transfers that do not carry a source charge. Automatic transfers created by
-- paid charges are allowed to coexist within the same reference month.
DROP INDEX IF EXISTS financial.uq_finance_transfers_monthly;

CREATE UNIQUE INDEX IF NOT EXISTS uq_finance_transfers_monthly_legacy
    ON financial.transfers (
        tenant_id,
        ((metadata->>'contract_id')),
        ((metadata->>'reference_month'))
    )
    WHERE deleted_at IS NULL
      AND NULLIF(metadata->>'source_charge_id', '') IS NULL
      AND NULLIF(metadata->>'contract_id', '') IS NOT NULL
      AND NULLIF(metadata->>'reference_month', '') IS NOT NULL;

COMMENT ON INDEX financial.uq_finance_transfers_source_charge IS
    'One automatic owner transfer per canonical paid charge; supports multiple SALE installments in one month.';

COMMENT ON INDEX financial.uq_finance_transfers_monthly_legacy IS
    'Monthly idempotency for legacy/manual transfers without source_charge_id.';

COMMIT;
