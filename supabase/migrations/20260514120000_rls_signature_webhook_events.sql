-- Hardening Onda 1 (P0) — Aplica RLS em signature.webhook_events.
--
-- Contexto: a tabela era service-only, sem coluna tenant_id e sem RLS, com a
-- justificativa de que webhooks da Clicksign chegam sem JWT. Defesa em
-- profundidade: se um caminho de leitura via Supabase REST surgir no futuro
-- (intencional ou não), webhooks de outros tenants não vazam. raw_body contém
-- PII de assinantes (nome, email, telefone).
--
-- Estratégia:
--   1. Adicionar coluna tenant_id (NULL inicialmente).
--   2. Backfill via JOIN com signature.envelopes (mesmo external_envelope_id).
--   3. Habilitar RLS com policy padrão do schema (igual a signature.envelopes).
--   4. O writer continua usando o service pool (BYPASSRLS), agora preenchendo
--      tenant_id via subquery — ver toca-signature/internal/repository/envelope_postgres.go.
--
-- Linhas remanescentes com tenant_id IS NULL ficam invisíveis para usuários JWT
-- e acessíveis apenas via service pool — comportamento intencional para webhooks
-- recebidos antes do envelope correspondente ser persistido (race rara) ou
-- spoof de external_envelope_id desconhecido.

ALTER TABLE signature.webhook_events
    ADD COLUMN IF NOT EXISTS tenant_id UUID;

UPDATE signature.webhook_events we
   SET tenant_id = e.tenant_id
  FROM signature.envelopes e
 WHERE we.external_envelope_id = e.external_envelope_id
   AND we.tenant_id IS NULL
   AND e.tenant_id IS NOT NULL;

CREATE INDEX IF NOT EXISTS idx_signature_webhook_events_tenant
    ON signature.webhook_events (tenant_id)
    WHERE tenant_id IS NOT NULL;

ALTER TABLE signature.webhook_events ENABLE ROW LEVEL SECURITY;

DROP POLICY IF EXISTS tenant_isolation_select ON signature.webhook_events;
CREATE POLICY tenant_isolation_select ON signature.webhook_events
    FOR SELECT USING (
        tenant_id IS NOT NULL
        AND tenant_id = ((auth.jwt() -> 'app_metadata' ->> 'tenant_id')::uuid)
    );

DROP POLICY IF EXISTS tenant_isolation_modify ON signature.webhook_events;
CREATE POLICY tenant_isolation_modify ON signature.webhook_events
    FOR ALL USING (
        tenant_id IS NOT NULL
        AND tenant_id = ((auth.jwt() -> 'app_metadata' ->> 'tenant_id')::uuid)
    ) WITH CHECK (
        tenant_id IS NOT NULL
        AND tenant_id = ((auth.jwt() -> 'app_metadata' ->> 'tenant_id')::uuid)
    );
