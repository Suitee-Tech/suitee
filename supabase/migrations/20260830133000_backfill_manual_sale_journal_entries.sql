-- Backfill the double-entry ledger for manual SALE receipts introduced before
-- the receipt/commission/owner-transfer journal hooks existed.
--
-- The three postings close one economic waterfall:
--   1. receipt: Dr cash / Cr owner + commissions + partners + agency revenue;
--   2. broker payment: Dr commission_payable / Cr cash + withholdings;
--   3. owner transfer: Dr owner_payable / Cr cash.
--
-- Candidates are validated before any durable write. Existing source IDs are
-- accepted only when their complete economic payload matches the expected
-- posting, so this migration cannot conceal a divergent historical journal.

BEGIN;

CREATE TEMP TABLE manual_sale_receipt_journal_candidates ON COMMIT DROP AS
SELECT
    receipt.id AS receipt_id,
    receipt.tenant_id,
    receipt.metadata->>'contract_id' AS contract_id,
    receipt.metadata->>'operation_id' AS operation_id,
    deal.metadata->>'owner_id' AS owner_id,
    CASE
        WHEN COALESCE(receipt.metadata->>'payment_date', '') ~ '^\d{4}-\d{2}-\d{2}T' THEN
            (receipt.metadata->>'payment_date')::timestamptz
        WHEN COALESCE(receipt.metadata->>'date', '') ~ '^\d{4}-\d{2}-\d{2}' THEN
            (receipt.metadata->>'date')::timestamptz
    END AS paid_at,
    CASE WHEN receipt.metadata#>>'{economic_breakdown,gross_cents}' ~ '^[0-9]+$'
        THEN (receipt.metadata#>>'{economic_breakdown,gross_cents}')::bigint END AS gross_cents,
    CASE WHEN receipt.metadata#>>'{economic_breakdown,discount_cents}' ~ '^[0-9]+$'
        THEN (receipt.metadata#>>'{economic_breakdown,discount_cents}')::bigint END AS discount_cents,
    CASE WHEN receipt.metadata#>>'{economic_breakdown,owner_payable_cents}' ~ '^[0-9]+$'
        THEN (receipt.metadata#>>'{economic_breakdown,owner_payable_cents}')::bigint END AS owner_payable_cents,
    CASE WHEN receipt.metadata#>>'{economic_breakdown,commission_payable_cents}' ~ '^[0-9]+$'
        THEN (receipt.metadata#>>'{economic_breakdown,commission_payable_cents}')::bigint END AS commission_payable_cents,
    CASE WHEN receipt.metadata#>>'{economic_breakdown,partner_payable_cents}' ~ '^[0-9]+$'
        THEN (receipt.metadata#>>'{economic_breakdown,partner_payable_cents}')::bigint END AS partner_payable_cents,
    CASE WHEN receipt.metadata#>>'{economic_breakdown,agency_retained_cents}' ~ '^[0-9]+$'
        THEN (receipt.metadata#>>'{economic_breakdown,agency_retained_cents}')::bigint END AS agency_retained_cents,
    uuid_generate_v5(
        'd7b8e6e6-7e16-5d3b-9c5b-1f87a5e8cd6e'::uuid,
        'manual_sale_receipt.payment|' || receipt.tenant_id::text || '|' || receipt.id::text
    )::text AS source_event_id
FROM financial.transactions AS receipt
JOIN deal.deals AS deal
  ON deal.id::text = receipt.metadata->>'deal_id'
 AND deal.tenant_id = receipt.tenant_id
 AND deal.deleted_at IS NULL
WHERE receipt.deleted_at IS NULL
  AND receipt.metadata->>'type' = 'receita'
  AND UPPER(COALESCE(receipt.metadata->>'status', '')) = 'PAID'
  AND receipt.metadata#>>'{economic_breakdown,flow}' = 'sale';

DO $$
BEGIN
    IF EXISTS (
        SELECT 1
        FROM manual_sale_receipt_journal_candidates
        WHERE NULLIF(BTRIM(contract_id), '') IS NULL
           OR NULLIF(BTRIM(operation_id), '') IS NULL
           OR NULLIF(BTRIM(owner_id), '') IS NULL
           OR paid_at IS NULL
           OR gross_cents IS NULL OR gross_cents <= 0
           OR discount_cents IS NULL OR discount_cents < 0
           OR owner_payable_cents IS NULL OR owner_payable_cents < 0
           OR commission_payable_cents IS NULL OR commission_payable_cents < 0
           OR partner_payable_cents IS NULL OR partner_payable_cents < 0
           OR agency_retained_cents IS NULL OR agency_retained_cents < 0
           OR gross_cents - discount_cents <> owner_payable_cents + commission_payable_cents + partner_payable_cents + agency_retained_cents
    ) THEN
        RAISE EXCEPTION 'manual SALE receipt journal backfill found an incomplete or unbalanced waterfall';
    END IF;
    IF EXISTS (
        SELECT operation_id
        FROM manual_sale_receipt_journal_candidates
        GROUP BY tenant_id, operation_id
        HAVING COUNT(*) <> 1
    ) THEN
        RAISE EXCEPTION 'manual SALE journal backfill found multiple receipts for one operation';
    END IF;
END $$;

CREATE TEMP TABLE expected_manual_sale_journals (
    tenant_id uuid NOT NULL,
    contract_id text NOT NULL,
    operation_id text NOT NULL,
    source_event_id text NOT NULL,
    entry_date timestamptz NOT NULL,
    description text NOT NULL,
    legs jsonb NOT NULL
) ON COMMIT DROP;

INSERT INTO expected_manual_sale_journals
SELECT
    candidate.tenant_id,
    candidate.contract_id,
    candidate.receipt_id::text,
    candidate.source_event_id,
    candidate.paid_at,
    'Recebimento manual da venda ' || candidate.receipt_id::text,
    jsonb_build_array(jsonb_build_object(
        'account', 'cash', 'direction', 'debit',
        'amount_cents', candidate.gross_cents - candidate.discount_cents
    ))
    || CASE WHEN candidate.owner_payable_cents > 0 THEN jsonb_build_array(jsonb_build_object(
        'account', 'owner_payable', 'direction', 'credit',
        'amount_cents', candidate.owner_payable_cents,
        'beneficiary_party_id', candidate.owner_id
    )) ELSE '[]'::jsonb END
    || CASE WHEN candidate.commission_payable_cents > 0 THEN jsonb_build_array(jsonb_build_object(
        'account', 'commission_payable', 'direction', 'credit',
        'amount_cents', candidate.commission_payable_cents
    )) ELSE '[]'::jsonb END
    || CASE WHEN candidate.partner_payable_cents > 0 THEN jsonb_build_array(jsonb_build_object(
        'account', 'partner_payable', 'direction', 'credit',
        'amount_cents', candidate.partner_payable_cents
    )) ELSE '[]'::jsonb END
    || CASE WHEN candidate.agency_retained_cents > 0 THEN jsonb_build_array(jsonb_build_object(
        'account', 'revenue', 'direction', 'credit',
        'amount_cents', candidate.agency_retained_cents,
        'beneficiary_party_id', candidate.tenant_id::text
    )) ELSE '[]'::jsonb END
FROM manual_sale_receipt_journal_candidates AS candidate;

CREATE TEMP TABLE manual_sale_commission_payment_candidates ON COMMIT DROP AS
SELECT
    comm.id AS commission_id,
    comm.tenant_id,
    comm.metadata->>'contract_id' AS contract_id,
    comm.metadata->>'broker_id' AS broker_id,
    receipt.receipt_id,
    CASE WHEN COALESCE(comm.metadata->>'paid_at', '') ~ '^\d{4}-\d{2}-\d{2}T'
        THEN (comm.metadata->>'paid_at')::timestamptz END AS paid_at,
    CASE WHEN COALESCE(NULLIF(comm.metadata->>'gross_amount', ''), comm.metadata->>'amount', '') ~ '^[0-9]+(\.[0-9]+)?$'
        THEN ROUND(COALESCE(NULLIF(comm.metadata->>'gross_amount', ''), comm.metadata->>'amount')::numeric * 100)::bigint END AS gross_cents,
    CASE WHEN COALESCE(NULLIF(comm.metadata->>'net_amount', ''), comm.metadata->>'amount', '') ~ '^[0-9]+(\.[0-9]+)?$'
        THEN ROUND(COALESCE(NULLIF(comm.metadata->>'net_amount', ''), comm.metadata->>'amount')::numeric * 100)::bigint END AS net_cents,
    COALESCE(deductions.tax_cents, 0)::bigint AS tax_cents,
    COALESCE(deductions.other_cents, 0)::bigint AS other_cents,
    COALESCE(deductions.invalid, false) AS invalid_deduction,
    uuid_generate_v5(
        'd7b8e6e6-7e16-5d3b-9c5b-1f87a5e8cd6e'::uuid,
        'manual_commission.payment|' || comm.tenant_id::text || '|' || comm.id::text
    )::text AS source_event_id
FROM financial.commissions AS comm
JOIN financial.transactions AS ledger
  ON ledger.id::text = comm.metadata->>'ledger_transaction_id'
 AND ledger.tenant_id = comm.tenant_id
 AND ledger.deleted_at IS NULL
JOIN manual_sale_receipt_journal_candidates AS receipt
  ON receipt.tenant_id = ledger.tenant_id
 AND receipt.operation_id = ledger.metadata->>'operation_id'
LEFT JOIN LATERAL (
    SELECT
        SUM(CASE WHEN item->>'kind' IN ('IRRF', 'INSS', 'ISS')
                AND COALESCE(item->>'amount', '') ~ '^[0-9]+(\.[0-9]+)?$'
            THEN ROUND((item->>'amount')::numeric * 100) ELSE 0 END) AS tax_cents,
        SUM(CASE WHEN item->>'kind' = 'OUTROS'
                AND COALESCE(item->>'amount', '') ~ '^[0-9]+(\.[0-9]+)?$'
            THEN ROUND((item->>'amount')::numeric * 100) ELSE 0 END) AS other_cents,
        BOOL_OR(
            item->>'kind' NOT IN ('IRRF', 'INSS', 'ISS', 'OUTROS')
            OR COALESCE(item->>'amount', '') !~ '^[0-9]+(\.[0-9]+)?$'
        ) AS invalid
    FROM jsonb_array_elements(
        CASE WHEN jsonb_typeof(comm.metadata->'deductions') = 'array'
            THEN comm.metadata->'deductions' ELSE '[]'::jsonb END
    ) AS item
) AS deductions ON TRUE
WHERE comm.deleted_at IS NULL
  AND UPPER(COALESCE(comm.metadata->>'status', '')) = 'PAID';

DO $$
BEGIN
    IF EXISTS (
        SELECT 1
        FROM manual_sale_commission_payment_candidates
        WHERE NULLIF(BTRIM(contract_id), '') IS NULL
           OR NULLIF(BTRIM(broker_id), '') IS NULL
           OR paid_at IS NULL
           OR gross_cents IS NULL OR gross_cents <= 0
           OR net_cents IS NULL OR net_cents < 0
           OR tax_cents < 0 OR other_cents < 0
           OR invalid_deduction
           OR gross_cents <> net_cents + tax_cents + other_cents
    ) THEN
        RAISE EXCEPTION 'manual commission journal backfill found an invalid payment composition';
    END IF;
END $$;

INSERT INTO expected_manual_sale_journals
SELECT
    candidate.tenant_id,
    candidate.contract_id,
    candidate.commission_id::text,
    candidate.source_event_id,
    candidate.paid_at,
    'Pagamento da comissão ' || candidate.commission_id::text,
    jsonb_build_array(jsonb_build_object(
        'account', 'commission_payable', 'direction', 'debit',
        'amount_cents', candidate.gross_cents,
        'beneficiary_party_id', candidate.broker_id
    ))
    || CASE WHEN candidate.net_cents > 0 THEN jsonb_build_array(jsonb_build_object(
        'account', 'cash', 'direction', 'credit',
        'amount_cents', candidate.net_cents,
        'beneficiary_party_id', candidate.broker_id
    )) ELSE '[]'::jsonb END
    || CASE WHEN candidate.tax_cents > 0 THEN jsonb_build_array(jsonb_build_object(
        'account', 'tax_withheld', 'direction', 'credit',
        'amount_cents', candidate.tax_cents
    )) ELSE '[]'::jsonb END
    || CASE WHEN candidate.other_cents > 0 THEN jsonb_build_array(jsonb_build_object(
        'account', 'other_payable', 'direction', 'credit',
        'amount_cents', candidate.other_cents
    )) ELSE '[]'::jsonb END
FROM manual_sale_commission_payment_candidates AS candidate;

CREATE TEMP TABLE manual_sale_transfer_settlement_candidates ON COMMIT DROP AS
SELECT
    transfer.id AS transfer_id,
    transfer.tenant_id,
    transfer.metadata->>'contract_id' AS contract_id,
    transfer.metadata->>'owner_id' AS owner_id,
    CASE WHEN COALESCE(transfer.metadata->>'processed_at', '') ~ '^\d{4}-\d{2}-\d{2}T'
        THEN (transfer.metadata->>'processed_at')::timestamptz END AS processed_at,
    CASE WHEN COALESCE(transfer.metadata->>'net_amount', '') ~ '^[0-9]+(\.[0-9]+)?$'
        THEN ROUND((transfer.metadata->>'net_amount')::numeric * 100)::bigint END AS amount_cents,
    CASE WHEN transfer.metadata#>>'{owner_balance,total_cents}' ~ '^[0-9]+$'
        THEN (transfer.metadata#>>'{owner_balance,total_cents}')::bigint END AS total_cents,
    CASE WHEN transfer.metadata#>>'{owner_balance,reserved_cents}' ~ '^[0-9]+$'
        THEN (transfer.metadata#>>'{owner_balance,reserved_cents}')::bigint END AS reserved_cents,
    CASE WHEN transfer.metadata#>>'{owner_balance,blocked_cents}' ~ '^[0-9]+$'
        THEN (transfer.metadata#>>'{owner_balance,blocked_cents}')::bigint END AS blocked_cents,
    COALESCE(
        NULLIF(BTRIM(transfer.metadata->>'settlement_journal_source_id'), ''),
        uuid_generate_v5(
            'd7b8e6e6-7e16-5d3b-9c5b-1f87a5e8cd6e'::uuid,
            'owner_transfer.settlement|' || transfer.tenant_id::text || '|' || transfer.id::text
        )::text
    ) AS source_event_id
FROM financial.transfers AS transfer
JOIN manual_sale_receipt_journal_candidates AS receipt
  ON receipt.tenant_id = transfer.tenant_id
 AND receipt.receipt_id::text = transfer.metadata->>'source_transaction_id'
WHERE transfer.deleted_at IS NULL
  AND UPPER(COALESCE(transfer.metadata->>'status', '')) = 'PROCESSED'
  AND COALESCE(transfer.metadata->>'settlement_type', transfer.metadata#>>'{economic_breakdown,flow}') = 'sale';

DO $$
BEGIN
    IF EXISTS (
        SELECT 1
        FROM manual_sale_transfer_settlement_candidates
        WHERE NULLIF(BTRIM(contract_id), '') IS NULL
           OR NULLIF(BTRIM(owner_id), '') IS NULL
           OR processed_at IS NULL
           OR amount_cents IS NULL OR amount_cents <= 0
           OR total_cents IS NULL OR reserved_cents IS NULL OR blocked_cents IS NULL
           OR amount_cents <> total_cents - reserved_cents - blocked_cents
    ) THEN
        RAISE EXCEPTION 'manual SALE transfer journal backfill found invalid owner-balance coverage';
    END IF;
END $$;

UPDATE financial.transfers AS transfer
SET metadata = jsonb_set(transfer.metadata, '{settlement_journal_source_id}', to_jsonb(candidate.source_event_id), true)
FROM manual_sale_transfer_settlement_candidates AS candidate
WHERE transfer.id = candidate.transfer_id
  AND transfer.tenant_id = candidate.tenant_id
  AND NULLIF(BTRIM(transfer.metadata->>'settlement_journal_source_id'), '') IS NULL;

INSERT INTO expected_manual_sale_journals
SELECT
    candidate.tenant_id,
    candidate.contract_id,
    candidate.transfer_id::text,
    candidate.source_event_id,
    candidate.processed_at,
    'Repasse ao proprietário ' || candidate.transfer_id::text,
    jsonb_build_array(
        jsonb_build_object(
            'account', 'owner_payable', 'direction', 'debit',
            'amount_cents', candidate.amount_cents,
            'transfer_id', candidate.transfer_id::text,
            'beneficiary_party_id', candidate.owner_id
        ),
        jsonb_build_object(
            'account', 'cash', 'direction', 'credit',
            'amount_cents', candidate.amount_cents,
            'transfer_id', candidate.transfer_id::text,
            'beneficiary_party_id', candidate.owner_id
        )
    )
FROM manual_sale_transfer_settlement_candidates AS candidate;

WITH rows_to_insert AS (
    SELECT expected.*, gen_random_uuid() AS journal_id
    FROM expected_manual_sale_journals AS expected
)
INSERT INTO financial.journal_entries (id, tenant_id, metadata, created_at, updated_at)
SELECT
    expected.journal_id,
    expected.tenant_id,
    jsonb_build_object(
        'id', expected.journal_id::text,
        'tenant_id', expected.tenant_id::text,
        'contract_id', expected.contract_id,
        'operation_id', expected.operation_id,
        'source_event_id', expected.source_event_id,
        'date', expected.entry_date,
        'description', expected.description,
        'legs', expected.legs,
        'status', 'POSTED',
        'created_at', transaction_timestamp(),
        'updated_at', transaction_timestamp()
    ),
    transaction_timestamp(),
    transaction_timestamp()
FROM rows_to_insert AS expected
ON CONFLICT DO NOTHING;

DO $$
BEGIN
    IF EXISTS (
        SELECT 1
        FROM expected_manual_sale_journals AS expected
        LEFT JOIN financial.journal_entries AS persisted
          ON persisted.tenant_id = expected.tenant_id
         AND persisted.deleted_at IS NULL
         AND persisted.metadata->>'source_event_id' = expected.source_event_id
        WHERE persisted.id IS NULL
           OR persisted.metadata->>'contract_id' IS DISTINCT FROM expected.contract_id
           OR persisted.metadata->>'operation_id' IS DISTINCT FROM expected.operation_id
           OR persisted.metadata->>'description' IS DISTINCT FROM expected.description
           OR persisted.metadata->>'status' IS DISTINCT FROM 'POSTED'
           OR (persisted.metadata->>'date')::timestamptz IS DISTINCT FROM expected.entry_date
           OR persisted.metadata->'legs' IS DISTINCT FROM expected.legs
    ) THEN
        RAISE EXCEPTION 'manual SALE journal backfill found a divergent canonical source_event_id';
    END IF;
END $$;

COMMIT;
