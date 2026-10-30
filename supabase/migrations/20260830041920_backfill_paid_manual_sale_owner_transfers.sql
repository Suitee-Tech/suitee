-- The preceding migration reconstructs the economic waterfall for legacy
-- manual SALE receipts. Receipts that were already PAID before the settlement
-- hook existed also need their seller obligation materialized: there will be
-- no future PENDING -> PAID transition to create it in application code.
--
-- This migration deliberately fails closed. It only accepts a tenant-scoped
-- SALE deal, complete owner/property/contract references, a cent-precision
-- waterfall that closes, and no existing transaction-backed transfer. An
-- unclaimed transfer for the same contract/month also blocks insertion so a
-- legacy obligation is never duplicated silently.

BEGIN;

WITH raw_candidates AS MATERIALIZED (
    SELECT
        receipt.id AS receipt_id,
        receipt.tenant_id,
        receipt.metadata->>'contract_id' AS contract_id,
        receipt.metadata->>'deal_id' AS deal_id,
        deal.metadata->>'owner_id' AS owner_id,
        deal.metadata->>'property_id' AS property_id,
        CASE
            WHEN COALESCE(receipt.metadata->>'payment_date', '') ~ '^\d{4}-(0[1-9]|1[0-2])-' THEN
                SUBSTRING(receipt.metadata->>'payment_date' FROM 1 FOR 7)
            WHEN COALESCE(receipt.metadata->>'date', '') ~ '^\d{4}-(0[1-9]|1[0-2])-' THEN
                SUBSTRING(receipt.metadata->>'date' FROM 1 FOR 7)
            ELSE TO_CHAR(receipt.created_at, 'YYYY-MM')
        END AS reference_month,
        receipt.metadata#>>'{economic_breakdown,gross_cents}' AS gross_cents_text,
        receipt.metadata#>>'{economic_breakdown,commission_pool_cents}' AS commission_pool_cents_text,
        receipt.metadata#>>'{economic_breakdown,commission_payable_cents}' AS commission_payable_cents_text,
        receipt.metadata#>>'{economic_breakdown,partner_payable_cents}' AS partner_payable_cents_text,
        receipt.metadata#>>'{economic_breakdown,agency_retained_cents}' AS agency_retained_cents_text,
        receipt.metadata#>>'{economic_breakdown,owner_payable_cents}' AS owner_payable_cents_text,
        receipt.metadata#>>'{economic_breakdown,discount_cents}' AS discount_cents_text
    FROM financial.transactions AS receipt
    JOIN deal.deals AS deal
      ON deal.id::text = receipt.metadata->>'deal_id'
     AND deal.tenant_id = receipt.tenant_id
     AND deal.deleted_at IS NULL
    WHERE receipt.deleted_at IS NULL
      AND (receipt.metadata->>'type') = 'receita'
      AND UPPER(COALESCE(receipt.metadata->>'status', '')) = 'PAID'
      AND LOWER(COALESCE(receipt.metadata->>'description', '')) LIKE '%venda%'
      AND (receipt.metadata#>>'{economic_breakdown,flow}') = 'sale'
      AND UPPER(COALESCE(deal.metadata->>'type', '')) = 'SALE'
      AND NULLIF(BTRIM(receipt.metadata->>'contract_id'), '') IS NOT NULL
      AND NULLIF(BTRIM(receipt.metadata->>'deal_id'), '') IS NOT NULL
      AND NULLIF(BTRIM(deal.metadata->>'owner_id'), '') IS NOT NULL
      AND NULLIF(BTRIM(deal.metadata->>'property_id'), '') IS NOT NULL
), normalized_candidates AS MATERIALIZED (
    SELECT
        raw.*,
        CASE WHEN gross_cents_text ~ '^[0-9]+$' THEN gross_cents_text::bigint END AS gross_cents,
        CASE WHEN commission_pool_cents_text ~ '^[0-9]+$' THEN commission_pool_cents_text::bigint END AS commission_pool_cents,
        CASE WHEN commission_payable_cents_text ~ '^[0-9]+$' THEN commission_payable_cents_text::bigint END AS commission_payable_cents,
        CASE WHEN partner_payable_cents_text ~ '^[0-9]+$' THEN partner_payable_cents_text::bigint END AS partner_payable_cents,
        CASE WHEN agency_retained_cents_text ~ '^[0-9]+$' THEN agency_retained_cents_text::bigint END AS agency_retained_cents,
        CASE WHEN owner_payable_cents_text ~ '^[0-9]+$' THEN owner_payable_cents_text::bigint END AS owner_payable_cents,
        CASE WHEN discount_cents_text ~ '^[0-9]+$' THEN discount_cents_text::bigint END AS discount_cents
    FROM raw_candidates AS raw
), valid_candidates AS MATERIALIZED (
    SELECT normalized.*
    FROM normalized_candidates AS normalized
    WHERE gross_cents > 0
      AND commission_pool_cents >= 0
      AND commission_payable_cents >= 0
      AND partner_payable_cents >= 0
      AND agency_retained_cents >= 0
      AND owner_payable_cents >= 0
      AND discount_cents >= 0
      AND commission_pool_cents = commission_payable_cents + partner_payable_cents + agency_retained_cents
      AND gross_cents - discount_cents = owner_payable_cents + commission_pool_cents
      AND NOT EXISTS (
          SELECT 1
          FROM financial.transfers AS existing
          WHERE existing.tenant_id = normalized.tenant_id
            AND existing.deleted_at IS NULL
            AND existing.metadata->>'source_transaction_id' = normalized.receipt_id::text
      )
      AND NOT EXISTS (
          SELECT 1
          FROM financial.transfers AS legacy
          WHERE legacy.tenant_id = normalized.tenant_id
            AND legacy.deleted_at IS NULL
            AND COALESCE(legacy.metadata->>'source_charge_id', '') = ''
            AND COALESCE(legacy.metadata->>'source_transaction_id', '') = ''
            AND legacy.metadata->>'contract_id' = normalized.contract_id
            AND legacy.metadata->>'reference_month' = normalized.reference_month
      )
), transfers_to_insert AS (
    SELECT gen_random_uuid() AS transfer_id, valid.*
    FROM valid_candidates AS valid
)
INSERT INTO financial.transfers (id, tenant_id, metadata, created_at)
SELECT
    candidate.transfer_id,
    candidate.tenant_id,
    jsonb_build_object(
        'id', candidate.transfer_id::text,
        'tenant_id', candidate.tenant_id::text,
        'source_transaction_id', candidate.receipt_id::text,
        'contract_id', candidate.contract_id,
        'owner_id', candidate.owner_id,
        'property_id', candidate.property_id,
        'reference_month', candidate.reference_month,
        'settlement_type', 'sale',
        'gross_rent', candidate.gross_cents::numeric / 100,
        'admin_fee_percent', 0,
        'admin_fee_amount', 0,
        'maintenance_deductions', 0,
        'other_deductions', 0,
        'net_amount', candidate.owner_payable_cents::numeric / 100,
        'owner_balance', jsonb_build_object(
            'total_cents', candidate.owner_payable_cents,
            'reserved_cents', 0,
            'blocked_cents', 0
        ),
        'economic_breakdown', jsonb_build_object(
            'flow', 'sale',
            'gross_cents', candidate.gross_cents,
            'owner_payable_cents', candidate.owner_payable_cents,
            'reserved_cents', 0,
            'available_cents', candidate.owner_payable_cents,
            'agency_revenue_cents', candidate.agency_retained_cents,
            'commission_payable_cents', candidate.commission_payable_cents,
            'partner_payable_cents', candidate.partner_payable_cents,
            'agency_retained_cents', candidate.agency_retained_cents,
            'discount_cents', candidate.discount_cents,
            'deductions_cents', candidate.discount_cents
        ),
        'owner_balance_base_total_cents', candidate.owner_payable_cents,
        'owner_balance_base_reserved_cents', 0,
        'status', 'PENDING',
        'notes', 'Repasse da venda retroativo ao recebimento manual ' || candidate.receipt_id::text,
        'history', jsonb_build_array(jsonb_build_object(
            'at', transaction_timestamp(),
            'action', 'created',
            'actor', 'system',
            'note', 'Obrigação materializada por migração de recebimento legado já pago'
        )),
        'created_at', transaction_timestamp(),
        'updated_at', transaction_timestamp()
    ),
    transaction_timestamp()
FROM transfers_to_insert AS candidate
ON CONFLICT DO NOTHING;

COMMIT;
