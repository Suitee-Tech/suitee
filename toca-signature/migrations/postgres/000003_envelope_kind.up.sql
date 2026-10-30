-- #436 — distrato. Um contrato terminado precisa de um SEGUNDO envelope de
-- assinatura (o distrato), além do envelope original do contrato. O índice
-- único parcial atual permite só um envelope ativo por contract_id, o que
-- bloquearia o distrato. Adiciona a coluna `kind` e passa o índice a
-- considerá-la: um contract_id pode ter um envelope ativo de cada kind.

ALTER TABLE signature.envelopes
    ADD COLUMN IF NOT EXISTS kind TEXT NOT NULL DEFAULT 'contract';

DROP INDEX IF EXISTS signature.uq_signature_envelopes_contract_active;

CREATE UNIQUE INDEX IF NOT EXISTS uq_signature_envelopes_contract_kind_active
    ON signature.envelopes (contract_id, kind)
    WHERE deleted_at IS NULL
      AND status NOT IN ('CANCELED','EXPIRED','REFUSED','ERROR');
