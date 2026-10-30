-- Manual sale receipts release an owner transfer just like canonical Charge
-- payments, but their identity is the Transaction posting. Keep both sources
-- explicit and exclude them from the legacy monthly uniqueness rule.

BEGIN;

CREATE UNIQUE INDEX IF NOT EXISTS uq_finance_transfers_source_transaction
    ON financial.transfers (
        tenant_id,
        ((metadata->>'source_transaction_id'))
    )
    WHERE deleted_at IS NULL
      AND NULLIF(metadata->>'source_transaction_id', '') IS NOT NULL;

DROP INDEX IF EXISTS financial.uq_finance_transfers_monthly_legacy;

CREATE UNIQUE INDEX IF NOT EXISTS uq_finance_transfers_monthly_legacy
    ON financial.transfers (
        tenant_id,
        ((metadata->>'contract_id')),
        ((metadata->>'reference_month'))
    )
    WHERE deleted_at IS NULL
      AND NULLIF(metadata->>'source_charge_id', '') IS NULL
      AND NULLIF(metadata->>'source_transaction_id', '') IS NULL
      AND NULLIF(metadata->>'contract_id', '') IS NOT NULL
      AND NULLIF(metadata->>'reference_month', '') IS NOT NULL;

COMMENT ON INDEX financial.uq_finance_transfers_source_transaction IS
    'One automatic owner transfer per settled manual receipt transaction.';

COMMENT ON INDEX financial.uq_finance_transfers_monthly_legacy IS
    'Monthly idempotency only for transfers without a canonical charge or transaction source.';

-- Receipts posted before the economic waterfall fields existed can be
-- reconstructed from the immutable operation and its one-to-one Commission
-- mirrors. Backfilling here makes the obligation visible before the operator
-- settles the existing row; malformed or ambiguous operations are skipped and
-- remain fail-closed in the settlement service.
WITH commission_summary AS (
    SELECT
        receipt.id,
        receipt.tenant_id,
        ROUND((receipt.metadata->>'value')::numeric * 100)::bigint AS gross_cents,
        ROUND(SUM((aggregate.metadata->>'amount')::numeric) * 100)::bigint AS broker_cents,
        SUM((aggregate.metadata->>'percentage')::numeric) AS broker_percentage,
        MIN(aggregate.metadata->>'deal_id') AS deal_id,
        MAX(aggregate.metadata->>'deal_id') AS max_deal_id,
        COUNT(*) AS resolved_commissions,
        COUNT(aggregate.metadata->>'deal_id') AS resolved_deals
    FROM financial.transactions AS receipt
    JOIN financial.transactions AS ledger
      ON ledger.tenant_id = receipt.tenant_id
     AND ledger.deleted_at IS NULL
     AND (ledger.metadata->>'operation_id') = (receipt.metadata->>'operation_id')
     AND (ledger.metadata->>'type') = 'comissao'
    JOIN financial.commissions AS aggregate
      ON aggregate.tenant_id = receipt.tenant_id
     AND aggregate.deleted_at IS NULL
     AND (aggregate.metadata->>'ledger_transaction_id') = ledger.id::text
    WHERE receipt.deleted_at IS NULL
      AND (receipt.metadata->>'type') = 'receita'
      AND LOWER(COALESCE(receipt.metadata->>'description', '')) LIKE '%venda%'
      AND NULLIF(receipt.metadata->>'batch_fingerprint', '') IS NOT NULL
      AND NULLIF(receipt.metadata->>'operation_id', '') IS NOT NULL
      AND receipt.metadata->'economic_breakdown' IS NULL
      AND NULLIF(receipt.metadata->>'value', '') IS NOT NULL
    GROUP BY receipt.id, receipt.tenant_id, receipt.metadata
    HAVING COUNT(*) = COALESCE((receipt.metadata->>'commission_count')::integer, 0)
       AND COUNT(*) > 0
       AND COUNT(aggregate.metadata->>'deal_id') = COUNT(*)
       AND MIN(aggregate.metadata->>'deal_id') = MAX(aggregate.metadata->>'deal_id')
       AND SUM((aggregate.metadata->>'percentage')::numeric) > 0
       AND SUM((aggregate.metadata->>'percentage')::numeric) <= 100.01
), waterfall AS (
    SELECT
        summary.*,
        ROUND(summary.broker_cents * 100::numeric / summary.broker_percentage)::bigint AS commission_pool_cents
    FROM commission_summary AS summary
), valid_waterfall AS (
    SELECT *
    FROM waterfall
    WHERE commission_pool_cents >= broker_cents
      AND commission_pool_cents <= gross_cents
      AND NULLIF(deal_id, '') IS NOT NULL
)
UPDATE financial.transactions AS receipt
SET metadata = jsonb_set(
        jsonb_set(
            jsonb_set(receipt.metadata, '{deal_id}', to_jsonb(valid.deal_id), true),
            '{economic_breakdown}',
            jsonb_build_object(
                'flow', 'sale',
                'gross_cents', valid.gross_cents,
                'commission_pool_cents', valid.commission_pool_cents,
                'commission_payable_cents', valid.broker_cents,
                'partner_payable_cents', 0,
                'agency_retained_cents', valid.commission_pool_cents - valid.broker_cents,
                'owner_payable_cents', valid.gross_cents - valid.commission_pool_cents,
                'discount_cents', 0
            ),
            true
        ),
        '{updated_at}',
        to_jsonb(transaction_timestamp()),
        true
    ),
    updated_at = transaction_timestamp()
FROM valid_waterfall AS valid
WHERE receipt.id = valid.id
  AND receipt.tenant_id = valid.tenant_id
  AND receipt.deleted_at IS NULL
  AND receipt.metadata->'economic_breakdown' IS NULL;

COMMIT;
