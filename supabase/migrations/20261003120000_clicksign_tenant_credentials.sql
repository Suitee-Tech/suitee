-- P1-14 — Credenciais Clicksign por tenant, rotacionáveis.
--
-- Antes todas as chamadas ao Clicksign usavam credenciais GLOBAIS de
-- ambiente (CLICKSIGN_ACCESS_TOKEN etc.), compartilhadas por todos os
-- tenants. Esta migração cria o armazenamento por tenant:
--
--   - signature.clicksign_credentials: um registro por tenant, com o
--     access_token e o webhook_secret CRIPTOGRAFADOS em repouso (AES-256-GCM
--     com chave de aplicação — nunca texto puro), revision monotônico para
--     rotação;
--   - signature.clicksign_credential_rotations: trilha append-only de
--     rotações (quem, quando, revisão).
--
-- O provedor global por ambiente continua como FALLBACK para tenants sem
-- linha (compatibilidade com o MVP); tenants COM linha usam a credencial
-- própria. Ver toca-signature/internal/domain/credential/db_provider.go.

CREATE TABLE IF NOT EXISTS signature.clicksign_credentials (
    tenant_id                 UUID PRIMARY KEY,
    base_url                  TEXT NOT NULL,
    access_token_encrypted    TEXT NOT NULL,
    webhook_secret_encrypted  TEXT NULL,
    environment               TEXT NOT NULL DEFAULT 'sandbox',
    auth_scheme               TEXT NOT NULL DEFAULT 'raw',
    revision                  INTEGER NOT NULL DEFAULT 1,
    created_at                TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at                TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    deleted_at                TIMESTAMPTZ NULL
);

CREATE TABLE IF NOT EXISTS signature.clicksign_credential_rotations (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id   UUID NOT NULL,
    revision    INTEGER NOT NULL,
    rotated_by  TEXT NOT NULL,
    note        TEXT NULL,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_signature_clicksign_credential_rotations_tenant
    ON signature.clicksign_credential_rotations (tenant_id, created_at DESC);

-- Isolamento por tenant — mesmo padrão de signature.envelopes. A leitura/escrita
-- do serviço usa o pool de serviço (BYPASSRLS) com o tenant resolvido da
-- credencial; o RLS protege o pool de aplicação.
ALTER TABLE signature.clicksign_credentials ENABLE ROW LEVEL SECURITY;

DROP POLICY IF EXISTS tenant_isolation_select ON signature.clicksign_credentials;
CREATE POLICY tenant_isolation_select ON signature.clicksign_credentials
    FOR SELECT USING (tenant_id = ((auth.jwt() -> 'app_metadata' ->> 'tenant_id')::uuid));

DROP POLICY IF EXISTS tenant_isolation_modify ON signature.clicksign_credentials;
CREATE POLICY tenant_isolation_modify ON signature.clicksign_credentials
    FOR ALL USING (tenant_id = ((auth.jwt() -> 'app_metadata' ->> 'tenant_id')::uuid))
    WITH CHECK (tenant_id = ((auth.jwt() -> 'app_metadata' ->> 'tenant_id')::uuid));

ALTER TABLE signature.clicksign_credential_rotations ENABLE ROW LEVEL SECURITY;

DROP POLICY IF EXISTS tenant_isolation_select ON signature.clicksign_credential_rotations;
CREATE POLICY tenant_isolation_select ON signature.clicksign_credential_rotations
    FOR SELECT USING (tenant_id = ((auth.jwt() -> 'app_metadata' ->> 'tenant_id')::uuid));

DROP POLICY IF EXISTS tenant_isolation_modify ON signature.clicksign_credential_rotations;
CREATE POLICY tenant_isolation_modify ON signature.clicksign_credential_rotations
    FOR ALL USING (tenant_id = ((auth.jwt() -> 'app_metadata' ->> 'tenant_id')::uuid))
    WITH CHECK (tenant_id = ((auth.jwt() -> 'app_metadata' ->> 'tenant_id')::uuid));
