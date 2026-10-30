-- Preserve every completed managed-bill payment before reopening the bill for
-- another payment attempt. The active payment/reconciliation columns continue
-- to describe only the current attempt; immutable prior snapshots live here.

BEGIN;

SET LOCAL lock_timeout = '5s';
SET LOCAL statement_timeout = '60s';

ALTER TABLE financial.managed_bills
    ADD COLUMN IF NOT EXISTS payment_attempts JSONB NOT NULL DEFAULT '[]'::jsonb;

ALTER TABLE financial.managed_bills
    DROP CONSTRAINT IF EXISTS ck_managed_bill_payment_attempts_array;
ALTER TABLE financial.managed_bills
    ADD CONSTRAINT ck_managed_bill_payment_attempts_array CHECK (
        jsonb_typeof(payment_attempts) = 'array'
    );

-- A newly reserved obligation cannot claim a historical payment. Replacing the
-- complete function preserves every insert invariant from the table's original
-- migration while adding the payment-attempt invariant.
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

    IF jsonb_array_length(NEW.payment_attempts) <> 0 THEN
        RAISE EXCEPTION 'managed bill payment attempts must be empty on insert'
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

-- Protect the complete source/economic snapshot and permit exactly one new
-- lifecycle edge: PAID/RECONCILED -> APPROVED. That edge is a compensating
-- payment reversal, never an erasure: it must archive the old active payment,
-- clear those active columns and append an auditable payment_reversed action.
CREATE OR REPLACE FUNCTION financial.guard_managed_bill_update()
RETURNS TRIGGER
LANGUAGE plpgsql
AS $managed_bill_guard$
DECLARE
    is_payment_reversal BOOLEAN := (
        OLD.status IN ('PAID', 'RECONCILED') AND NEW.status = 'APPROVED'
    );
    latest_attempt JSONB;
    latest_history JSONB;
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
            OR is_payment_reversal
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
       AND NOT (
            (OLD.status = 'APPROVED' AND NEW.status = 'PAID')
            OR is_payment_reversal
       ) THEN
        RAISE EXCEPTION 'managed bill payment milestone is write-once'
            USING ERRCODE = '23514';
    END IF;

    IF (NEW.reconciled_by IS DISTINCT FROM OLD.reconciled_by
        OR NEW.reconciled_at IS DISTINCT FROM OLD.reconciled_at
        OR NEW.reconciliation_reference IS DISTINCT FROM OLD.reconciliation_reference)
       AND NOT (
            (OLD.status = 'PAID' AND NEW.status = 'RECONCILED')
            OR is_payment_reversal
       ) THEN
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

    latest_history := NEW.history -> -1;
    IF NULLIF(latest_history ->> 'version', '')::BIGINT IS DISTINCT FROM NEW.version THEN
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

    IF is_payment_reversal THEN
        IF NEW.paid_by IS NOT NULL OR NEW.paid_at IS NOT NULL
           OR NEW.payment_reference IS NOT NULL OR NEW.payment_journal_source_id IS NOT NULL
           OR NEW.reconciled_by IS NOT NULL OR NEW.reconciled_at IS NOT NULL
           OR NEW.reconciliation_reference IS NOT NULL THEN
            RAISE EXCEPTION 'managed bill active payment milestones must be cleared on reversal'
                USING ERRCODE = '23514';
        END IF;

        IF jsonb_array_length(NEW.payment_attempts) <> jsonb_array_length(OLD.payment_attempts) + 1 THEN
            RAISE EXCEPTION 'managed bill payment reversal must append exactly one payment attempt'
                USING ERRCODE = '23514';
        END IF;

        IF EXISTS (
            SELECT 1
            FROM jsonb_array_elements(OLD.payment_attempts) WITH ORDINALITY AS prior(entry, ordinal)
            WHERE NEW.payment_attempts -> ((prior.ordinal - 1)::INTEGER) IS DISTINCT FROM prior.entry
        ) THEN
            RAISE EXCEPTION 'managed bill payment attempts are immutable'
                USING ERRCODE = '23514';
        END IF;

        latest_attempt := NEW.payment_attempts -> -1;
        IF NULLIF(BTRIM(latest_attempt ->> 'id'), '') IS NULL
           OR NULLIF(BTRIM(latest_attempt ->> 'reversal_reference'), '') IS NULL
           OR NULLIF(BTRIM(latest_attempt ->> 'reversal_journal_source_id'), '') IS NULL
           OR NULLIF(BTRIM(latest_attempt ->> 'reversed_by'), '') IS NULL
           OR NULLIF(latest_attempt ->> 'reversed_at', '')::TIMESTAMPTZ IS NULL
           OR NULLIF(BTRIM(latest_attempt ->> 'reversal_reason'), '') IS NULL THEN
            RAISE EXCEPTION 'managed bill payment reversal metadata is required'
                USING ERRCODE = '23514';
        END IF;

        IF latest_attempt ->> 'id' IS DISTINCT FROM OLD.payment_journal_source_id THEN
            RAISE EXCEPTION 'managed bill payment attempt id must match the payment journal source'
                USING ERRCODE = '23514';
        END IF;

        IF EXISTS (
            SELECT 1
            FROM jsonb_array_elements(OLD.payment_attempts) AS prior(entry)
            WHERE prior.entry ->> 'id' = latest_attempt ->> 'id'
        ) THEN
            RAISE EXCEPTION 'managed bill payment attempt id must be unique'
                USING ERRCODE = '23514';
        END IF;

        IF EXISTS (
            SELECT 1
            FROM jsonb_array_elements(OLD.payment_attempts) AS prior(entry)
            WHERE prior.entry ->> 'reversal_reference' = latest_attempt ->> 'reversal_reference'
               OR prior.entry ->> 'reversal_journal_source_id' = latest_attempt ->> 'reversal_journal_source_id'
        ) THEN
            RAISE EXCEPTION 'managed bill payment reversal references must be unique'
                USING ERRCODE = '23514';
        END IF;

        IF latest_attempt ->> 'payment_reference' IS DISTINCT FROM OLD.payment_reference
           OR latest_attempt ->> 'payment_journal_source_id' IS DISTINCT FROM OLD.payment_journal_source_id
           OR latest_attempt ->> 'paid_by' IS DISTINCT FROM OLD.paid_by
           OR NULLIF(latest_attempt ->> 'paid_at', '')::TIMESTAMPTZ IS DISTINCT FROM OLD.paid_at
           OR NULLIF(latest_attempt ->> 'reconciled_by', '') IS DISTINCT FROM OLD.reconciled_by
           OR NULLIF(latest_attempt ->> 'reconciled_at', '')::TIMESTAMPTZ IS DISTINCT FROM OLD.reconciled_at
           OR NULLIF(latest_attempt ->> 'reconciliation_reference', '') IS DISTINCT FROM OLD.reconciliation_reference THEN
            RAISE EXCEPTION 'managed bill payment attempt does not match active payment snapshot'
                USING ERRCODE = '23514';
        END IF;

        IF NOT EXISTS (
            SELECT 1
            FROM financial.journal_entries AS original_journal
            JOIN financial.journal_entries AS reversal_journal
              ON reversal_journal.tenant_id = original_journal.tenant_id
             AND reversal_journal.deleted_at IS NULL
             AND reversal_journal.metadata ->> 'source_event_id' = latest_attempt ->> 'reversal_journal_source_id'
             AND reversal_journal.metadata ->> 'reversal_of' = original_journal.id::TEXT
             AND reversal_journal.metadata ->> 'status' = 'POSTED'
            WHERE original_journal.tenant_id = OLD.tenant_id
              AND original_journal.deleted_at IS NULL
              AND original_journal.metadata ->> 'source_event_id' = OLD.payment_journal_source_id
              AND original_journal.metadata ->> 'status' = 'REVERSED'
        ) THEN
            RAISE EXCEPTION 'managed bill payment reversal journal is missing or inconsistent'
                USING ERRCODE = '23514';
        END IF;

        IF latest_history ->> 'action' IS DISTINCT FROM 'payment_reversed'
           OR latest_history ->> 'actor' IS DISTINCT FROM latest_attempt ->> 'reversed_by'
           OR NULLIF(latest_history ->> 'at', '')::TIMESTAMPTZ IS DISTINCT FROM NULLIF(latest_attempt ->> 'reversed_at', '')::TIMESTAMPTZ
           OR NULLIF(latest_history ->> 'reference', '') IS DISTINCT FROM NULLIF(latest_attempt ->> 'reversal_reference', '')
           OR NULLIF(latest_history ->> 'reason', '') IS DISTINCT FROM NULLIF(latest_attempt ->> 'reversal_reason', '')
           OR NULLIF(latest_attempt ->> 'reversed_at', '')::TIMESTAMPTZ IS DISTINCT FROM NEW.updated_at THEN
            RAISE EXCEPTION 'managed bill payment reversal history is inconsistent'
                USING ERRCODE = '23514';
        END IF;
    ELSE
        IF NEW.payment_attempts IS DISTINCT FROM OLD.payment_attempts THEN
            RAISE EXCEPTION 'managed bill payment attempts may change only on payment reversal'
                USING ERRCODE = '23514';
        END IF;
        IF latest_history ->> 'action' = 'payment_reversed' THEN
            RAISE EXCEPTION 'managed bill payment_reversed history requires a payment reversal'
                USING ERRCODE = '23514';
        END IF;
    END IF;

    RETURN NEW;
END
$managed_bill_guard$;

COMMIT;
