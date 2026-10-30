-- Abacate Pay v2 webhook deliveries. Every inbound webhook is persisted here
-- before processing so retries are idempotent (unique event_id) and failed
-- deliveries can be replayed from raw_body. Mirrors signature.webhook_events.
--
-- tenant_id is nullable: Abacate Pay webhooks carry no tenant context, so it is
-- only filled once the delivery is resolved to a local payment/charge/payout.

CREATE TABLE IF NOT EXISTS financial.abacate_webhook_events (
    id            uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    event_id      text NOT NULL,
    event_type    text NOT NULL,
    api_version   integer,
    dev_mode      boolean NOT NULL DEFAULT false,
    tenant_id     text,
    raw_body      jsonb NOT NULL,
    signature     text,
    source_ip     text,
    processed     boolean NOT NULL DEFAULT false,
    process_error text,
    received_at   timestamptz NOT NULL DEFAULT now(),
    processed_at  timestamptz
);

-- Idempotency: a redelivered webhook collapses onto the existing row.
CREATE UNIQUE INDEX IF NOT EXISTS uq_abacate_webhook_events_event_id
    ON financial.abacate_webhook_events (event_id);

CREATE INDEX IF NOT EXISTS idx_abacate_webhook_events_type
    ON financial.abacate_webhook_events (event_type);

CREATE INDEX IF NOT EXISTS idx_abacate_webhook_events_received
    ON financial.abacate_webhook_events (received_at);

-- Surfaces failed deliveries awaiting replay.
CREATE INDEX IF NOT EXISTS idx_abacate_webhook_events_unprocessed
    ON financial.abacate_webhook_events (received_at)
    WHERE processed = false;
