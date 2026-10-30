-- Wave 1 (1.6) — Optimistic locking interino via updated_seq.
--
-- Contexto: o ETag atual de crm.leads usa updated_at em RFC3339Nano.
-- Em escrita concorrente sub-nanosegundo, dois UPDATEs podem produzir o
-- mesmo timestamp e ambos serem aceitos. Adicionamos uma sequence column
-- monótona que é incrementada em cada UPDATE, garantindo unicidade do
-- token de versão. A coluna `version` definitiva chega na Wave 2 (2.5);
-- esta migração é aditiva, com default 0 para registros existentes.
ALTER TABLE crm.leads
  ADD COLUMN IF NOT EXISTS updated_seq BIGINT NOT NULL DEFAULT 0;

-- Backfill: garante que registros existentes tenham seq=1 para que o primeiro
-- PATCH não colida com o default.
UPDATE crm.leads SET updated_seq = 1 WHERE updated_seq = 0;
