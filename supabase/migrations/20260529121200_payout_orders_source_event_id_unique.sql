CREATE UNIQUE INDEX IF NOT EXISTS uq_finance_payout_orders_source_event_id
    ON financial.payout_orders (tenant_id, ((metadata->>'source_event_id')))
    WHERE deleted_at IS NULL
      AND (metadata->>'source_event_id') IS NOT NULL
      AND COALESCE((metadata->>'status'), '') <> 'CANCELLED';

