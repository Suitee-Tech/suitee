-- Notificações transacionais por e-mail (beta) — cooldown por categoria.
--
-- O ledger comm.sent_externals nasceu para idempotência (PK event_id,
-- channel, recipient_id). O cooldown do beta precisa diferenciar categorias:
-- "1 e-mail por destinatário por CATEGORIA a cada N minutos" — sem isso, um
-- e-mail de status de negócio suprimiria um e-mail de proposta para o mesmo
-- cliente dentro da janela.
--
-- Coluna é aditiva e opcional: dispatches legados (whatsapp/sms, parcerias)
-- gravam NULL/'' e continuam funcionando; o gate de cooldown só consulta
-- linhas com categoria preenchida.
--
-- A PK não muda: idempotência continua por (event_id, channel, recipient_id).

ALTER TABLE comm.sent_externals
    ADD COLUMN IF NOT EXISTS category TEXT;

CREATE INDEX IF NOT EXISTS idx_comm_sent_externals_tenant_channel_category_sent
    ON comm.sent_externals (tenant_id, channel, category, sent_at DESC);
