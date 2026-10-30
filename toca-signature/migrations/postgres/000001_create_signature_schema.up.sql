-- Mirror of supabase/migrations/20260510120400_create_signature_schema.sql.
-- Kept here so the migration is colocated with the service for tools that
-- run service-local migrations (e.g. `migrate -path migrations/postgres ...`).
-- When the supabase migration runs in dev/prod against the central database,
-- this file is a no-op (IF NOT EXISTS guards everywhere).

CREATE SCHEMA IF NOT EXISTS signature;

CREATE TABLE IF NOT EXISTS signature.envelopes (
    id                    UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id             UUID NOT NULL,
    contract_id           UUID NOT NULL,
    deal_id               UUID NULL,
    external_envelope_id  TEXT NULL,
    environment           TEXT NOT NULL DEFAULT 'sandbox',
    status                TEXT NOT NULL DEFAULT 'PENDING',
    metadata              JSONB NOT NULL DEFAULT '{}'::jsonb,
    last_error            TEXT NULL,
    created_at            TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at            TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    finished_at           TIMESTAMPTZ NULL,
    deleted_at            TIMESTAMPTZ NULL
);

CREATE UNIQUE INDEX IF NOT EXISTS uq_signature_envelopes_contract_active
    ON signature.envelopes (contract_id)
    WHERE deleted_at IS NULL
      AND status NOT IN ('CANCELED','EXPIRED','REFUSED','ERROR');

CREATE INDEX IF NOT EXISTS idx_signature_envelopes_tenant
    ON signature.envelopes (tenant_id)
    WHERE deleted_at IS NULL;

CREATE INDEX IF NOT EXISTS idx_signature_envelopes_external
    ON signature.envelopes (external_envelope_id)
    WHERE external_envelope_id IS NOT NULL AND deleted_at IS NULL;

CREATE INDEX IF NOT EXISTS idx_signature_envelopes_status
    ON signature.envelopes (tenant_id, status)
    WHERE deleted_at IS NULL;

CREATE INDEX IF NOT EXISTS idx_signature_envelopes_deal
    ON signature.envelopes (tenant_id, deal_id)
    WHERE deal_id IS NOT NULL AND deleted_at IS NULL;

CREATE TABLE IF NOT EXISTS signature.webhook_events (
    id                    UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    external_envelope_id  TEXT NOT NULL,
    event_type            TEXT NOT NULL,
    delivery_id           TEXT NULL,
    raw_body              JSONB NOT NULL,
    signature_header      TEXT NULL,
    source_ip             TEXT NULL,
    processed             BOOLEAN NOT NULL DEFAULT FALSE,
    process_error         TEXT NULL,
    received_at           TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_signature_webhook_events_envelope
    ON signature.webhook_events (external_envelope_id);

CREATE INDEX IF NOT EXISTS idx_signature_webhook_events_received
    ON signature.webhook_events (received_at);
