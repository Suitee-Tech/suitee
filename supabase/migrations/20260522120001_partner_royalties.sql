-- financial.partner_royalties — franchise royalties owed when a deal closes
-- on a property captured under a partnership of type "franchise".
--
-- The toca-financial contract.signed pipeline detects a cross-company deal
-- (the closing tenant differs from the property's owner tenant). When an
-- active franchise partnership exists between the two companies, a royalty
-- payable is recorded here for the franchisor.
--
-- Explicit numeric columns (base_amount/percentage/amount) are kept first
-- class — these are money fields queried by finance dashboards — while
-- metadata jsonb absorbs any future shape evolution without a migration,
-- mirroring the rest of the financial.* schema.

CREATE TABLE IF NOT EXISTS financial.partner_royalties (
    id             uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id      text NOT NULL,
    partnership_id text,
    deal_id        text,
    contract_id    text,
    payer_tenant   text,
    payee_tenant   text,
    base_amount    numeric NOT NULL DEFAULT 0,
    percentage     numeric NOT NULL DEFAULT 0,
    amount         numeric NOT NULL DEFAULT 0,
    status         text NOT NULL DEFAULT 'PENDING',
    metadata       jsonb NOT NULL DEFAULT '{}'::jsonb,
    created_at     timestamptz NOT NULL DEFAULT now(),
    updated_at     timestamptz,
    deleted_at     timestamptz
);

CREATE INDEX IF NOT EXISTS idx_finance_partner_royalties_tenant
    ON financial.partner_royalties (tenant_id)
    WHERE deleted_at IS NULL;

CREATE INDEX IF NOT EXISTS idx_finance_partner_royalties_partnership
    ON financial.partner_royalties (partnership_id)
    WHERE deleted_at IS NULL;
