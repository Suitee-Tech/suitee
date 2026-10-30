-- Hardening Onda 2 (P1.7) — Adiciona controle de backoff e estado FAILED
-- ao outbox para que mensagens não fiquem retentando indefinidamente.
--
-- Antes desta migration:
--   - O drainer (pkg/events/outbox.go) reintroduzia eventos PENDING a cada
--     2 segundos com `attempts++` sem teto. Um evento com falha durável
--     (payload malformado, evento removido do schema, etc.) gerava log spam
--     infinito e atrapalhava o throughput do drainer com retries inúteis.
--   - Sem `next_attempt_at`, o drainer reprocessava o evento imediatamente
--     no ciclo seguinte (2s), sem backoff — em incidentes longos isso
--     martelava o broker e o Postgres sem deixar tempo de recuperação.
--
-- Esta migration adiciona:
--   - `next_attempt_at TIMESTAMPTZ NULL` — quando preenchido, indica que o
--     drainer deve ignorar a linha até NOW() >= next_attempt_at.
--   - status='FAILED' passa a ser um estado terminal usado pelo drainer
--     quando `attempts >= MaxOutboxAttempts` (definido em
--     pkg/events/outbox.go).
--   - Índice parcial em next_attempt_at para que a query do drainer use
--     index scan ao invés de filter.
--
-- Linhas legacy com next_attempt_at = NULL continuam elegíveis no primeiro
-- ciclo (comportamento anterior preservado).

ALTER TABLE events.outbox_events
    ADD COLUMN IF NOT EXISTS next_attempt_at TIMESTAMPTZ NULL;

CREATE INDEX IF NOT EXISTS idx_outbox_events_next_attempt
    ON events.outbox_events (next_attempt_at)
    WHERE status = 'PENDING';
