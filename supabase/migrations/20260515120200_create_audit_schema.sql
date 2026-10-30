-- Hardening Onda 4 (P1) — Audit log universal.
--
-- Contexto: hoje só toca-core/partnership tem audit interno. Ações
-- críticas em Deal, Contract, Rental, Lead, Property, User não geram
-- registro forense — auditoria fiscal e compliance LGPD (Art. 37 e 38)
-- exigem 5 anos de trilha de quem alterou o quê.
--
-- Esquema:
--   audit.events — uma linha por mutation auditada. Particionada por mês
--                  para que retenção/expurgo seja DROP PARTITION em vez
--                  de DELETE massivo (Postgres bloqueia a tabela inteira
--                  por horas em DELETE de 5 anos).
--
-- before/after são JSONB com o estado relevante (não sub-graphs inteiros).
-- O helper pkg/audit.Record decide quais campos vão para esses blobs
-- — sempre exclui PII direto (use pkg/pii.MaskForLog primeiro).
--
-- RLS: somente leitura por tenant via JWT; INSERT via service pool.

CREATE SCHEMA IF NOT EXISTS audit;

-- The parent table is partitioned by month using created_at as the key.
-- Each child partition lives in audit.events_yyyy_mm.
CREATE TABLE IF NOT EXISTS audit.events (
    id            UUID NOT NULL DEFAULT gen_random_uuid(),
    tenant_id     UUID NOT NULL,
    actor_id      UUID,
    actor_type    TEXT NOT NULL DEFAULT 'user', -- "user" | "service" | "system"
    action        TEXT NOT NULL,                -- "create" | "update" | "delete" | "soft_delete" | domain-specific
    entity_type   TEXT NOT NULL,                -- "Deal" | "Contract" | "Rental" | "Lead" | "Property" | "User"
    entity_id     TEXT NOT NULL,                -- text to accept UUID / string ids uniformly
    before        JSONB,                        -- pre-mutation state (NULL on creates)
    after         JSONB,                        -- post-mutation state (NULL on deletes)
    metadata      JSONB,                        -- request_id, trace_id, source ip, etc.
    created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (id, created_at)
) PARTITION BY RANGE (created_at);

CREATE INDEX IF NOT EXISTS idx_audit_events_tenant_created
    ON audit.events (tenant_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_audit_events_entity
    ON audit.events (entity_type, entity_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_audit_events_actor
    ON audit.events (actor_id, created_at DESC) WHERE actor_id IS NOT NULL;

-- Bootstrap partition: this month + next month so writes don't fail
-- before the maintenance job (TODO) creates future ones. Replace these
-- with a scheduled function once the operations runbook is in place.
DO $$
DECLARE
    cur_month_start DATE := date_trunc('month', NOW())::DATE;
    next_month_start DATE := (date_trunc('month', NOW()) + INTERVAL '1 month')::DATE;
    next_next_start DATE := (date_trunc('month', NOW()) + INTERVAL '2 months')::DATE;
BEGIN
    EXECUTE format(
        'CREATE TABLE IF NOT EXISTS audit.events_%s PARTITION OF audit.events FOR VALUES FROM (%L) TO (%L)',
        to_char(cur_month_start, 'YYYY_MM'),
        cur_month_start,
        next_month_start
    );
    EXECUTE format(
        'CREATE TABLE IF NOT EXISTS audit.events_%s PARTITION OF audit.events FOR VALUES FROM (%L) TO (%L)',
        to_char(next_month_start, 'YYYY_MM'),
        next_month_start,
        next_next_start
    );
END $$;

ALTER TABLE audit.events ENABLE ROW LEVEL SECURITY;

DROP POLICY IF EXISTS tenant_isolation_select ON audit.events;
CREATE POLICY tenant_isolation_select ON audit.events
    FOR SELECT USING (
        tenant_id = ((auth.jwt() -> 'app_metadata' ->> 'tenant_id')::uuid)
    );

-- No INSERT policy via JWT: writes always go through the service pool so
-- application code can't fake the actor_id. Reads are tenant-scoped.

COMMENT ON TABLE audit.events IS
    'Onda 4 P1 — append-only audit log of mutations across services. Partitioned by month to support 5y retention with cheap drop-partition expurgo. Writes via service pool only; reads tenant-scoped via RLS.';
