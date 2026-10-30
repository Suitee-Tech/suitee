-- Cross-path idempotency anchor for charge state reconciliation.
--
-- The Abacate Pay billing.paid webhook and the periodic
-- ChargeReconciliationService poll both arrive at MarkPaid, but on different
-- triggers and timescales. The lock in charge_service.MarkPaid covers
-- *simultaneous* concurrency. This column covers *sequential* races:
-- a webhook at T marks the charge paid, the reconcile tick at T+20min
-- looks again and — if propagation lagged or ordering shifted — re-applies
-- MarkPaid, doubling the ledger transaction.
--
-- Both paths now stamp last_reconciled_at at the end of successful
-- processing. The reconcile loop's selector skips any charge whose
-- last_reconciled_at falls inside the recent window (default 60min), so a
-- webhook that arrived since the last tick keeps the polling path away
-- from the row.
--
-- Writer (webhook path): toca-financial/internal/service/charge_checkout.go
--   (ChargeService.MarkPaidByCheckoutID)
-- Writer (polling path): toca-financial/internal/service/charge_reconciliation_service.go
--   (reconcileTenant, end of each charge iteration)
-- Selector: charge_postgres.go (ListPendingNeedsReconcile)

ALTER TABLE financial.charges
    ADD COLUMN IF NOT EXISTS last_reconciled_at TIMESTAMPTZ NULL;

-- Partial index covers the polling selector: PENDING + not recently touched.
-- NULLS FIRST so charges that were never reconciled are picked up first.
CREATE INDEX IF NOT EXISTS idx_finance_charges_pending_needs_reconcile
    ON financial.charges (last_reconciled_at NULLS FIRST)
    WHERE deleted_at IS NULL AND (metadata->>'status') = 'PENDING';
