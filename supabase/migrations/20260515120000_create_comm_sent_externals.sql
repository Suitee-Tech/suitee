-- Hardening Onda 1 (P1) — Idempotência de disparos externos em toca-comm.
--
-- Contexto: handlers como `lead.escalated -> notify_whatsapp` chamavam
-- DispatchWhatsApp/SMS/Email diretamente, sem checar se aquele evento já
-- havia sido processado. Em redelivery do RabbitMQ (consumer crash, DLQ
-- replay, etc.) o broker/cliente recebia múltiplas mensagens externas.
--
-- Esta tabela funciona como ledger de "primeira-entrega-vence": antes do
-- dispatch real, o handler tenta INSERT ON CONFLICT DO NOTHING. Se inseriu,
-- é a primeira vez — dispara. Se conflitou, já foi enviado antes — skip.
--
-- Chave composta: (event_id, channel, recipient_id) — o mesmo evento pode
-- gerar disparos para canais ou destinatários distintos (broker via SMS +
-- gerente via WhatsApp, p.ex.) mas cada combinação é única.
--
-- RLS: ativada com policy padrão do schema, mas writers usam service pool
-- (BYPASSRLS) para registrar o dispatch — leituras humanas só veem o tenant
-- próprio.

CREATE TABLE IF NOT EXISTS comm.sent_externals (
    event_id     TEXT NOT NULL,
    channel      TEXT NOT NULL CHECK (channel IN ('whatsapp', 'sms', 'email')),
    recipient_id TEXT NOT NULL,
    tenant_id    UUID NOT NULL,
    sent_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (event_id, channel, recipient_id)
);

CREATE INDEX IF NOT EXISTS idx_comm_sent_externals_tenant_sent_at
    ON comm.sent_externals (tenant_id, sent_at DESC);

ALTER TABLE comm.sent_externals ENABLE ROW LEVEL SECURITY;

DROP POLICY IF EXISTS tenant_isolation_select ON comm.sent_externals;
CREATE POLICY tenant_isolation_select ON comm.sent_externals
    FOR SELECT USING (
        tenant_id = ((auth.jwt() -> 'app_metadata' ->> 'tenant_id')::uuid)
    );

DROP POLICY IF EXISTS tenant_isolation_modify ON comm.sent_externals;
CREATE POLICY tenant_isolation_modify ON comm.sent_externals
    FOR ALL USING (
        tenant_id = ((auth.jwt() -> 'app_metadata' ->> 'tenant_id')::uuid)
    ) WITH CHECK (
        tenant_id = ((auth.jwt() -> 'app_metadata' ->> 'tenant_id')::uuid)
    );
