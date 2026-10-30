-- Add events.dlq_events.replayed_at.
--
-- The DLQ replay path (pkg/events/dlq_consumer.go: `SET replayed_at = NOW()`)
-- and the DLQ-pending metric (pkg/events/outbox.go:
-- `SELECT COUNT(*) FROM events.dlq_events WHERE replayed_at IS NULL`, run on
-- every drainer cycle ~2s per service) both reference this column — but it was
-- never added to events.dlq_events (created in
-- 20231026000000_init_jsonb_pilot.sql). Its absence makes every drainer cycle
-- fail with `column "replayed_at" does not exist`, flooding the Postgres logs
-- (~20x/sec across services) and silently breaking DLQ replay.
ALTER TABLE events.dlq_events ADD COLUMN IF NOT EXISTS replayed_at TIMESTAMPTZ NULL;

-- Partial index backs the hot DLQ-pending COUNT (WHERE replayed_at IS NULL).
CREATE INDEX IF NOT EXISTS idx_events_dlq_events_replayed_at
  ON events.dlq_events (replayed_at)
  WHERE replayed_at IS NULL;
