-- PR 1 — Clicksign integration. Creates the signature schema with the
-- envelope aggregate (one Clicksign envelope per Contract) and an audit
-- trail of raw webhook deliveries.
--
-- See toca-signature/internal/domain/envelope/envelope.go for the JSONB
-- shape persisted in `metadata`. The handful of denormalised columns
-- (status, external_envelope_id, contract_id, deal_id, environment) exist
-- only to support the indexes below — everything else lives in JSON.

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

-- Only one *active* envelope per contract. Cancellation/refusal/error/expired
-- envelopes don't block creating a new attempt.
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

-- Audit trail for raw webhook deliveries. NOT used for idempotency
-- (events.processed_events handles that downstream), but kept for debugging
-- and replay. No RLS — written by the webhook handler over the service pool
-- because Clicksign delivers without a JWT.
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

-- Tenant isolation policies. Mirrors the loop pattern from
-- 20260421120000_finalize_jsonb_rls.sql, but applied just to signature.envelopes
-- (signature.webhook_events is service-only).
ALTER TABLE signature.envelopes ENABLE ROW LEVEL SECURITY;

DROP POLICY IF EXISTS tenant_isolation_select ON signature.envelopes;
CREATE POLICY tenant_isolation_select ON signature.envelopes
    FOR SELECT USING (tenant_id = ((auth.jwt() -> 'app_metadata' ->> 'tenant_id')::uuid));

DROP POLICY IF EXISTS tenant_isolation_modify ON signature.envelopes;
CREATE POLICY tenant_isolation_modify ON signature.envelopes
    FOR ALL USING (tenant_id = ((auth.jwt() -> 'app_metadata' ->> 'tenant_id')::uuid))
    WITH CHECK (tenant_id = ((auth.jwt() -> 'app_metadata' ->> 'tenant_id')::uuid));
