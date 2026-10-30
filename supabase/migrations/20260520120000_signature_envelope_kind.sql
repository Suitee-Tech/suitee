-- #436 (distrato) — adiciona a coluna `kind` em signature.envelopes.
--
-- Esta migration existia apenas em toca-signature/migrations/postgres/
-- (000003_envelope_kind) e NÃO foi portada para supabase/migrations/, que é
-- a fonte da verdade do schema aplicada no deploy (deploy-hostinger.yml roda
-- `psql -f` em todos os .sql daqui, em ordem). Sem ela, o INSERT do envelope
-- falha em produção com:
--   column "kind" of relation "envelopes" does not exist (SQLSTATE 42703)
--
-- Um contrato terminado precisa de um SEGUNDO envelope de assinatura (o
-- distrato) além do envelope original do contrato. O índice único parcial
-- antigo (uq_signature_envelopes_contract_active) permitia só um envelope
-- ativo por contract_id, o que bloquearia o distrato — agora o índice passa
-- a considerar também o `kind`: um contract_id pode ter um envelope ativo de
-- cada kind (contract | distrato).
--
-- Idempotente (ADD COLUMN / CREATE INDEX IF NOT EXISTS, DROP IF EXISTS): o
-- loop de deploy reaplica todos os arquivos a cada release sem efeito
-- colateral. Linhas existentes recebem kind='contract' pelo DEFAULT.

ALTER TABLE signature.envelopes
    ADD COLUMN IF NOT EXISTS kind TEXT NOT NULL DEFAULT 'contract';

DROP INDEX IF EXISTS signature.uq_signature_envelopes_contract_active;

CREATE UNIQUE INDEX IF NOT EXISTS uq_signature_envelopes_contract_kind_active
    ON signature.envelopes (contract_id, kind)
    WHERE deleted_at IS NULL
      AND status NOT IN ('CANCELED','EXPIRED','REFUSED','ERROR');
