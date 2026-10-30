-- Notificações transacionais por e-mail (beta) — preferências por usuário.
--
-- Contexto: docs/features/comunicacao/notificacoes-email-beta.md. O canal
-- de e-mail (Resend) passa a cobrir fluxos transacionais (lead atribuído,
-- atualizações de negócio, propostas, agenda, recomendações, lembretes de
-- chat). Cada usuário pode desligar categorias específicas; a ausência de
-- linha significa "ligado" (default).
--
-- user_id é TEXT (não UUID): o espaço de identidades de destinatário é misto
-- — corretor (crm.brokers.id), operador/conta (toca_auth.accounts.id) e, no
-- futuro, clientes do portal (account id). O mesmo id nunca pertence a dois
-- tenants no fluxo de e-mail, mas a PK carrega tenant_id mesmo assim para
-- alinhar com a política RLS padrão do schema.
--
-- RLS: writers usam o service pool (BYPASSRLS) — mesma filosofia de
-- comm.sent_externals; leituras humanas só veem o próprio tenant.

CREATE TABLE IF NOT EXISTS comm.notification_prefs (
    user_id    TEXT        NOT NULL,
    tenant_id  UUID        NOT NULL,
    category   TEXT        NOT NULL CHECK (category IN (
        'lead_assigned',
        'deal_updates',
        'proposals',
        'appointments',
        'recommendations',
        'chat_reminders'
    )),
    enabled    BOOLEAN     NOT NULL DEFAULT TRUE,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (user_id, tenant_id, category)
);

CREATE INDEX IF NOT EXISTS idx_comm_notification_prefs_tenant_category
    ON comm.notification_prefs (tenant_id, category);

ALTER TABLE comm.notification_prefs ENABLE ROW LEVEL SECURITY;

DROP POLICY IF EXISTS tenant_isolation_select ON comm.notification_prefs;
CREATE POLICY tenant_isolation_select ON comm.notification_prefs
    FOR SELECT USING (
        tenant_id = ((auth.jwt() -> 'app_metadata' ->> 'tenant_id')::uuid)
    );

DROP POLICY IF EXISTS tenant_isolation_modify ON comm.notification_prefs;
CREATE POLICY tenant_isolation_modify ON comm.notification_prefs
    FOR ALL USING (
        tenant_id = ((auth.jwt() -> 'app_metadata' ->> 'tenant_id')::uuid)
    ) WITH CHECK (
        tenant_id = ((auth.jwt() -> 'app_metadata' ->> 'tenant_id')::uuid)
    );
