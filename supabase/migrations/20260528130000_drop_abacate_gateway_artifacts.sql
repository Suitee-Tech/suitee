-- Forward-only teardown of the AbacatePay payment-gateway integration.
--
-- Background: the financial flow no longer uses any payment gateway. Charges
-- are created in PENDING with no checkout and settled MANUALLY via
-- ChargeService.MarkPaid (reconcile-manual). This migration drops the
-- gateway-only schema objects that the removed code maintained:
--
--   - The Abacate Pay checkout lookup indexes on financial.charges. They were
--     created by 20260522120200_charges_abacate_checkout_idx.sql to let the
--     billing.paid webhook resolve a charge by its (possibly superseded)
--     checkout id. With no webhook, they are dead weight.
--   - financial.charges.last_reconciled_at + its partial index. Added by
--     20260527120200_charges_last_reconciled_at.sql as the cross-path
--     idempotency anchor between the billing.paid webhook and the polling
--     ChargeReconciliationService. Both paths are gone. (Dropping the column
--     also drops idx_finance_charges_pending_needs_reconcile, but we drop the
--     index explicitly first to keep the intent clear.)
--   - financial.abacate_webhook_events. Created by
--     20260522120000_create_abacate_webhook_events.sql to persist inbound
--     Abacate Pay webhook deliveries for idempotent replay. No webhook is
--     mounted anymore.
--
-- JSONB metadata keys on financial.charges (abacate_checkout_id,
-- prior_checkout_ids, payment_url, payment_id, etc.) are intentionally left
-- in place — they are inert without the indexes and scrubbing them would be a
-- heavy rewrite of every row for no functional gain.

DROP INDEX IF EXISTS financial.idx_finance_charges_abacate_checkout;
DROP INDEX IF EXISTS financial.idx_finance_charges_prior_checkouts;
DROP INDEX IF EXISTS financial.idx_finance_charges_pending_needs_reconcile;

ALTER TABLE financial.charges
    DROP COLUMN IF EXISTS last_reconciled_at;

DROP TABLE IF EXISTS financial.abacate_webhook_events;
