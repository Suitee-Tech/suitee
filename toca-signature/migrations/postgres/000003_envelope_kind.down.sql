-- Reverte #436: volta ao índice único por contract_id (um envelope ativo por
-- contrato) e remove a coluna kind. Pressupõe que não há contratos com
-- envelope de contrato E de distrato ativos ao mesmo tempo — caso haja, o
-- CREATE UNIQUE INDEX abaixo falha (esperado: resolver os duplicados antes).

DROP INDEX IF EXISTS signature.uq_signature_envelopes_contract_kind_active;

CREATE UNIQUE INDEX IF NOT EXISTS uq_signature_envelopes_contract_active
    ON signature.envelopes (contract_id)
    WHERE deleted_at IS NULL
      AND status NOT IN ('CANCELED','EXPIRED','REFUSED','ERROR');

ALTER TABLE signature.envelopes DROP COLUMN IF EXISTS kind;
