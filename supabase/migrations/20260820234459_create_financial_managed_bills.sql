-- Persist managed owner-paid obligations as a relational lifecycle instead of
-- extending the JSONB UtilityAccount/Charge/Transfer projections. A managed
-- bill starts only after owner-payable funds have been reserved, so every row
-- has one canonical transfer, charge and charge-item source.

BEGIN;

SET LOCAL lock_timeout = '5s';
SET LOCAL statement_timeout = '60s';

-- Composite foreign keys prevent a UUID from another tenant from being linked
-- even when an application bug already knows that UUID. transfers already has
-- the equivalent key from 20260810123000_transfer_deduction_claims.sql.
CREATE UNIQUE INDEX IF NOT EXISTS uq_finance_charges_tenant_id_id
    ON financial.charges (tenant_id, id);

CREATE UNIQUE INDEX IF NOT EXISTS uq_finance_transfers_tenant_id_id
    ON financial.transfers (tenant_id, id);

CREATE TABLE IF NOT EXISTS financial.managed_bills (
    id                  UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id           UUID        NOT NULL,
    source_transfer_id  UUID        NOT NULL,
    source_charge_id    UUID        NOT NULL,
    charge_item_id      UUID        NOT NULL,
    source_key          TEXT        NOT NULL CHECK (btrim(source_key) <> ''),
    contract_id         TEXT        NOT NULL CHECK (btrim(contract_id) <> ''),
    property_id         TEXT        NOT NULL CHECK (btrim(property_id) <> ''),
    owner_id            TEXT        NOT NULL CHECK (btrim(owner_id) <> ''),
    component           TEXT        NOT NULL CHECK (btrim(component) <> ''),
    creditor            TEXT        NOT NULL CHECK (btrim(creditor) <> ''),
    description         TEXT        NOT NULL CHECK (btrim(description) <> ''),
    amount_cents        BIGINT      NOT NULL CHECK (amount_cents > 0),
    currency            TEXT        NOT NULL DEFAULT 'BRL' CHECK (currency = 'BRL'),
    due_date            DATE        NOT NULL,
    competence_month    TEXT        NOT NULL CHECK (
        competence_month ~ '^[0-9]{4}-(0[1-9]|1[0-2])$'
    ),
    status              TEXT        NOT NULL DEFAULT 'RESERVED' CHECK (
        status IN ('RESERVED', 'APPROVED', 'PAID', 'RECONCILED', 'CANCELLED')
    ),
    version             BIGINT      NOT NULL DEFAULT 1 CHECK (version > 0),
    evidence            JSONB       NOT NULL DEFAULT '[]'::jsonb CHECK (
        jsonb_typeof(evidence) = 'array'
    ),
    reserved_by         TEXT        NOT NULL CHECK (btrim(reserved_by) <> ''),
    approved_by         TEXT,
    paid_by             TEXT,
    reconciled_by       TEXT,
    cancelled_by        TEXT,
    reserved_at         TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    approved_at         TIMESTAMPTZ,
    paid_at             TIMESTAMPTZ,
    reconciled_at       TIMESTAMPTZ,
    cancelled_at        TIMESTAMPTZ,
    payment_reference   TEXT,
    payment_journal_source_id TEXT,
    reconciliation_reference TEXT,
    cancel_reason       TEXT,
    history             JSONB       NOT NULL DEFAULT '[]'::jsonb CHECK (
        jsonb_typeof(history) = 'array'
    ),
    created_at          TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    CONSTRAINT uq_finance_managed_bills_tenant_id_id
        UNIQUE (tenant_id, id),
    CONSTRAINT fk_managed_bill_tenant_transfer
        FOREIGN KEY (tenant_id, source_transfer_id)
        REFERENCES financial.transfers (tenant_id, id)
        ON DELETE RESTRICT,
    CONSTRAINT fk_managed_bill_tenant_charge
        FOREIGN KEY (tenant_id, source_charge_id)
        REFERENCES financial.charges (tenant_id, id)
        ON DELETE RESTRICT,
    CONSTRAINT ck_managed_bill_actor_timestamps CHECK (
        (approved_at IS NULL OR NULLIF(btrim(approved_by), '') IS NOT NULL)
        AND (paid_at IS NULL OR NULLIF(btrim(paid_by), '') IS NOT NULL)
        AND (reconciled_at IS NULL OR NULLIF(btrim(reconciled_by), '') IS NOT NULL)
        AND (cancelled_at IS NULL OR NULLIF(btrim(cancelled_by), '') IS NOT NULL)
    ),
    CONSTRAINT ck_managed_bill_status_milestones CHECK (
        (status NOT IN ('APPROVED', 'PAID', 'RECONCILED') OR approved_at IS NOT NULL)
        AND (status NOT IN ('PAID', 'RECONCILED') OR paid_at IS NOT NULL)
        AND (status <> 'RECONCILED' OR reconciled_at IS NOT NULL)
        AND (status NOT IN ('PAID', 'RECONCILED') OR NULLIF(btrim(payment_reference), '') IS NOT NULL)
        AND (status NOT IN ('PAID', 'RECONCILED') OR NULLIF(btrim(payment_journal_source_id), '') IS NOT NULL)
        AND (status <> 'RECONCILED' OR NULLIF(btrim(reconciliation_reference), '') IS NOT NULL)
        AND (
            status <> 'CANCELLED'
            OR (
                cancelled_at IS NOT NULL
                AND NULLIF(btrim(cancel_reason), '') IS NOT NULL
            )
        )
    ),
    CONSTRAINT ck_managed_bill_timestamp_order CHECK (
        updated_at >= created_at
        AND reserved_at >= created_at
        AND (approved_at IS NULL OR approved_at >= reserved_at)
        AND (paid_at IS NULL OR paid_at >= COALESCE(approved_at, reserved_at))
        AND (reconciled_at IS NULL OR reconciled_at >= paid_at)
        AND (cancelled_at IS NULL OR cancelled_at >= reserved_at)
    )
);

CREATE UNIQUE INDEX IF NOT EXISTS uq_finance_managed_bills_source_key
    ON financial.managed_bills (tenant_id, source_key);

CREATE UNIQUE INDEX IF NOT EXISTS uq_finance_managed_bills_transfer_item
    ON financial.managed_bills (tenant_id, source_transfer_id, charge_item_id);

CREATE INDEX IF NOT EXISTS idx_finance_managed_bills_tenant_status_due
    ON financial.managed_bills (tenant_id, status, due_date, id);

CREATE INDEX IF NOT EXISTS idx_finance_managed_bills_tenant_contract_competence
    ON financial.managed_bills (tenant_id, contract_id, competence_month, created_at DESC);

CREATE INDEX IF NOT EXISTS idx_finance_managed_bills_active_reservations
    ON financial.managed_bills (tenant_id, source_transfer_id)
    WHERE status IN ('RESERVED', 'APPROVED');

-- Even authenticated clients with an INSERT policy may only create the first
-- canonical lifecycle revision. Later states must be reached through guarded
-- updates, never by inserting a pre-paid or pre-reconciled row.
CREATE OR REPLACE FUNCTION financial.guard_managed_bill_insert()
RETURNS TRIGGER
LANGUAGE plpgsql
AS $managed_bill_insert_guard$
BEGIN
    IF NEW.status <> 'RESERVED' OR NEW.version <> 1 THEN
        RAISE EXCEPTION 'managed bill must be inserted as RESERVED version 1'
            USING ERRCODE = '23514';
    END IF;

    IF NEW.approved_by IS NOT NULL OR NEW.approved_at IS NOT NULL
       OR NEW.paid_by IS NOT NULL OR NEW.paid_at IS NOT NULL
       OR NEW.payment_reference IS NOT NULL OR NEW.payment_journal_source_id IS NOT NULL
       OR NEW.reconciled_by IS NOT NULL OR NEW.reconciled_at IS NOT NULL
       OR NEW.reconciliation_reference IS NOT NULL
       OR NEW.cancelled_by IS NOT NULL OR NEW.cancelled_at IS NOT NULL
       OR NEW.cancel_reason IS NOT NULL THEN
        RAISE EXCEPTION 'managed bill lifecycle milestones must be empty on insert'
            USING ERRCODE = '23514';
    END IF;

    IF jsonb_array_length(NEW.history) <> 1
       OR NEW.history -> 0 ->> 'action' IS DISTINCT FROM 'reserved'
       OR NULLIF(NEW.history -> 0 ->> 'version', '')::BIGINT IS DISTINCT FROM 1
       OR NEW.history -> 0 ->> 'actor' IS DISTINCT FROM NEW.reserved_by
       OR NULLIF(NEW.history -> 0 ->> 'at', '')::TIMESTAMPTZ IS DISTINCT FROM NEW.reserved_at THEN
        RAISE EXCEPTION 'managed bill initial history is invalid'
            USING ERRCODE = '23514';
    END IF;

    RETURN NEW;
END
$managed_bill_insert_guard$;

DROP TRIGGER IF EXISTS trg_guard_managed_bill_insert
    ON financial.managed_bills;
CREATE TRIGGER trg_guard_managed_bill_insert
    BEFORE INSERT ON financial.managed_bills
    FOR EACH ROW
    EXECUTE FUNCTION financial.guard_managed_bill_insert();

-- Protect the idempotency/source identity and economic snapshot, and require
-- every update to use the explicit bigint revision. Status changes are
-- constrained to the lifecycle; same-status updates remain available only for
-- append-only evidence/history enrichment.
CREATE OR REPLACE FUNCTION financial.guard_managed_bill_update()
RETURNS TRIGGER
LANGUAGE plpgsql
AS $managed_bill_guard$
BEGIN
    IF NEW.id IS DISTINCT FROM OLD.id
       OR NEW.tenant_id IS DISTINCT FROM OLD.tenant_id
       OR NEW.source_transfer_id IS DISTINCT FROM OLD.source_transfer_id
       OR NEW.source_charge_id IS DISTINCT FROM OLD.source_charge_id
       OR NEW.charge_item_id IS DISTINCT FROM OLD.charge_item_id
       OR NEW.source_key IS DISTINCT FROM OLD.source_key THEN
        RAISE EXCEPTION 'managed bill source identity is immutable'
            USING ERRCODE = '23514';
    END IF;

    IF NEW.contract_id IS DISTINCT FROM OLD.contract_id
       OR NEW.property_id IS DISTINCT FROM OLD.property_id
       OR NEW.owner_id IS DISTINCT FROM OLD.owner_id
       OR NEW.component IS DISTINCT FROM OLD.component
       OR NEW.creditor IS DISTINCT FROM OLD.creditor
       OR NEW.description IS DISTINCT FROM OLD.description
       OR NEW.amount_cents IS DISTINCT FROM OLD.amount_cents
       OR NEW.currency IS DISTINCT FROM OLD.currency
       OR NEW.due_date IS DISTINCT FROM OLD.due_date
       OR NEW.competence_month IS DISTINCT FROM OLD.competence_month
       OR NEW.reserved_by IS DISTINCT FROM OLD.reserved_by
       OR NEW.reserved_at IS DISTINCT FROM OLD.reserved_at
       OR NEW.created_at IS DISTINCT FROM OLD.created_at THEN
        RAISE EXCEPTION 'managed bill economic attributes are immutable'
            USING ERRCODE = '23514';
    END IF;

    IF NEW.version IS DISTINCT FROM OLD.version + 1 THEN
        RAISE EXCEPTION 'managed bill version must increment by exactly one'
            USING ERRCODE = '23514';
    END IF;

    IF NEW.status IS DISTINCT FROM OLD.status
       AND NOT (
            (OLD.status = 'RESERVED' AND NEW.status IN ('APPROVED', 'CANCELLED'))
            OR (OLD.status = 'APPROVED' AND NEW.status IN ('PAID', 'CANCELLED'))
            OR (OLD.status = 'PAID' AND NEW.status = 'RECONCILED')
       ) THEN
        RAISE EXCEPTION 'invalid managed bill transition: % -> %', OLD.status, NEW.status
            USING ERRCODE = '23514';
    END IF;

    IF (NEW.approved_by IS DISTINCT FROM OLD.approved_by
        OR NEW.approved_at IS DISTINCT FROM OLD.approved_at)
       AND NOT (OLD.status = 'RESERVED' AND NEW.status = 'APPROVED') THEN
        RAISE EXCEPTION 'managed bill approval milestone is write-once'
            USING ERRCODE = '23514';
    END IF;

    IF (NEW.paid_by IS DISTINCT FROM OLD.paid_by
        OR NEW.paid_at IS DISTINCT FROM OLD.paid_at
        OR NEW.payment_reference IS DISTINCT FROM OLD.payment_reference
        OR NEW.payment_journal_source_id IS DISTINCT FROM OLD.payment_journal_source_id)
       AND NOT (OLD.status = 'APPROVED' AND NEW.status = 'PAID') THEN
        RAISE EXCEPTION 'managed bill payment milestone is write-once'
            USING ERRCODE = '23514';
    END IF;

    IF (NEW.reconciled_by IS DISTINCT FROM OLD.reconciled_by
        OR NEW.reconciled_at IS DISTINCT FROM OLD.reconciled_at
        OR NEW.reconciliation_reference IS DISTINCT FROM OLD.reconciliation_reference)
       AND NOT (OLD.status = 'PAID' AND NEW.status = 'RECONCILED') THEN
        RAISE EXCEPTION 'managed bill reconciliation milestone is write-once'
            USING ERRCODE = '23514';
    END IF;

    IF (NEW.cancelled_by IS DISTINCT FROM OLD.cancelled_by
        OR NEW.cancelled_at IS DISTINCT FROM OLD.cancelled_at
        OR NEW.cancel_reason IS DISTINCT FROM OLD.cancel_reason)
       AND NOT (
            OLD.status IN ('RESERVED', 'APPROVED')
            AND NEW.status = 'CANCELLED'
       ) THEN
        RAISE EXCEPTION 'managed bill lifecycle milestones are write-once'
            USING ERRCODE = '23514';
    END IF;

    IF NEW.updated_at < OLD.updated_at THEN
        RAISE EXCEPTION 'managed bill updated_at must be monotonic'
            USING ERRCODE = '23514';
    END IF;

    IF jsonb_array_length(NEW.history) <> jsonb_array_length(OLD.history) + 1 THEN
        RAISE EXCEPTION 'managed bill history must append exactly one entry'
            USING ERRCODE = '23514';
    END IF;

    IF EXISTS (
        SELECT 1
        FROM jsonb_array_elements(OLD.history) WITH ORDINALITY AS prior(entry, ordinal)
        WHERE NEW.history -> ((prior.ordinal - 1)::INTEGER) IS DISTINCT FROM prior.entry
    ) THEN
        RAISE EXCEPTION 'managed bill history entries are immutable'
            USING ERRCODE = '23514';
    END IF;

    IF NULLIF(NEW.history -> -1 ->> 'version', '')::BIGINT IS DISTINCT FROM NEW.version THEN
        RAISE EXCEPTION 'managed bill history version must match row version'
            USING ERRCODE = '23514';
    END IF;

    IF jsonb_array_length(NEW.evidence) < jsonb_array_length(OLD.evidence) THEN
        RAISE EXCEPTION 'managed bill evidence is append-only'
            USING ERRCODE = '23514';
    END IF;

    IF EXISTS (
        SELECT 1
        FROM jsonb_array_elements(OLD.evidence) WITH ORDINALITY AS prior(entry, ordinal)
        WHERE NEW.evidence -> ((prior.ordinal - 1)::INTEGER) IS DISTINCT FROM prior.entry
    ) THEN
        RAISE EXCEPTION 'managed bill evidence entries are immutable'
            USING ERRCODE = '23514';
    END IF;

    RETURN NEW;
END
$managed_bill_guard$;

DROP TRIGGER IF EXISTS trg_guard_managed_bill_update
    ON financial.managed_bills;
CREATE TRIGGER trg_guard_managed_bill_update
    BEFORE UPDATE ON financial.managed_bills
    FOR EACH ROW
    EXECUTE FUNCTION financial.guard_managed_bill_update();

ALTER TABLE financial.managed_bills ENABLE ROW LEVEL SECURITY;

DROP POLICY IF EXISTS tenant_isolation_select ON financial.managed_bills;
CREATE POLICY tenant_isolation_select ON financial.managed_bills
    FOR SELECT TO authenticated
    USING (
        tenant_id = NULLIF(
            (SELECT auth.jwt()) -> 'app_metadata' ->> 'tenant_id',
            ''
        )::UUID
    );

DROP POLICY IF EXISTS tenant_isolation_insert ON financial.managed_bills;
CREATE POLICY tenant_isolation_insert ON financial.managed_bills
    FOR INSERT TO authenticated
    WITH CHECK (
        tenant_id = NULLIF(
            (SELECT auth.jwt()) -> 'app_metadata' ->> 'tenant_id',
            ''
        )::UUID
    );

DROP POLICY IF EXISTS tenant_isolation_update ON financial.managed_bills;
CREATE POLICY tenant_isolation_update ON financial.managed_bills
    FOR UPDATE TO authenticated
    USING (
        tenant_id = NULLIF(
            (SELECT auth.jwt()) -> 'app_metadata' ->> 'tenant_id',
            ''
        )::UUID
    )
    WITH CHECK (
        tenant_id = NULLIF(
            (SELECT auth.jwt()) -> 'app_metadata' ->> 'tenant_id',
            ''
        )::UUID
    );

-- Lifecycle rows are cancelled, never deleted. The service-role connection can
-- still remove exact test fixtures and perform controlled disaster recovery.
REVOKE DELETE ON financial.managed_bills FROM authenticated;

COMMIT;
