-- A monthly contract may now contain more than one charge-backed transfer.
-- Deduction retries therefore need an identity independent of whichever
-- installment is currently first in the month. This immutable claim maps one
-- tenant/reference_id to the canonical transfer selected for that deduction.

BEGIN;

CREATE UNIQUE INDEX IF NOT EXISTS uq_finance_transfers_tenant_id_id
    ON financial.transfers (tenant_id, id);

CREATE TABLE IF NOT EXISTS financial.transfer_deduction_claims (
    tenant_id       UUID        NOT NULL,
    reference_id    TEXT        NOT NULL CHECK (btrim(reference_id) <> ''),
    contract_id     TEXT        NOT NULL,
    reference_month TEXT        NOT NULL,
    transfer_id     UUID        NOT NULL,
    deduction_type  TEXT        NOT NULL,
    amount_cents    BIGINT      NOT NULL CHECK (amount_cents > 0),
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (tenant_id, reference_id),
    CONSTRAINT fk_transfer_deduction_claim_tenant_transfer
        FOREIGN KEY (tenant_id, transfer_id)
        REFERENCES financial.transfers (tenant_id, id)
        ON DELETE RESTRICT
);

CREATE INDEX IF NOT EXISTS idx_transfer_deduction_claims_contract_month
    ON financial.transfer_deduction_claims
       (tenant_id, contract_id, reference_month, created_at);

ALTER TABLE financial.transfer_deduction_claims ENABLE ROW LEVEL SECURITY;

DROP POLICY IF EXISTS tenant_isolation_select
    ON financial.transfer_deduction_claims;
CREATE POLICY tenant_isolation_select
    ON financial.transfer_deduction_claims
    FOR SELECT TO authenticated
    USING (
        tenant_id = NULLIF(
            auth.jwt() -> 'app_metadata' ->> 'tenant_id',
            ''
        )::uuid
    );

DROP POLICY IF EXISTS tenant_isolation_modify
    ON financial.transfer_deduction_claims;
DROP POLICY IF EXISTS tenant_isolation_insert
    ON financial.transfer_deduction_claims;
CREATE POLICY tenant_isolation_insert
    ON financial.transfer_deduction_claims
    FOR INSERT TO authenticated
    WITH CHECK (
        tenant_id = NULLIF(
            auth.jwt() -> 'app_metadata' ->> 'tenant_id',
            ''
        )::uuid
    );

COMMENT ON TABLE financial.transfer_deduction_claims IS
    'Immutable tenant-scoped idempotency claims for owner-transfer deductions.';

COMMIT;
