-- FASE 1 (integração total) — idempotency anchor for cross-service charge
-- creation. Handlers that materialize a charge from an upstream event
-- (maintenance.requested, maintenance.completed diff, inspection.deductions_recorded)
-- store a deterministic source_event_id in the JSONB metadata. The partial
-- unique index lets the writer use INSERT ... ON CONFLICT DO NOTHING so
-- redeliveries of the same broker message collapse to a single charge row.
--
-- Domain: toca-financial/internal/domain/charge/charge.go (Charge.SourceEventID)
-- Writer: toca-financial/internal/service/charge_service.go (CreateWithSourceEventID)
-- ID derivation: pkg/events/DeriveNotificationID

CREATE UNIQUE INDEX IF NOT EXISTS uq_finance_charges_source_event_id
    ON financial.charges (tenant_id, ((metadata->>'source_event_id')))
    WHERE deleted_at IS NULL AND (metadata->>'source_event_id') IS NOT NULL;
