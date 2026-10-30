-- financial.collection_agreements — acordo formalizado com o devedor (Fase 2, G2).
--
-- Antes, "negociar" era gravar `negotiated_amount`/`negotiated_date` no metadata
-- do caso: nenhuma parcela existia, nenhuma cobrança era baixada e o acordo não
-- tinha como ser cumprido, quebrado ou cobrado — o campo era escrito e nunca
-- lido por ninguém. Aqui o acordo passa a ser entidade própria, com o plano de
-- parcelas materializado em cobranças de verdade (financial.charges).
--
-- Por que tabela e não mais um campo no JSONB do caso: o acordo tem ciclo de
-- vida próprio (ACTIVE → FULFILLED | BROKEN | CANCELLED), histórico (quem criou,
-- quem aprovou, por que quebrou) e é consultado por parcela. Empilhar isso no
-- metadata do caso repetiria o problema que o diagnóstico descreveu — estado
-- escondido em JSONB sem índice, sem constraint e sem leitura.
--
-- A dívida NÃO é do acordo: quem deve é a cobrança. O acordo guarda
-- `original_charge_ids` (o que foi substituído) e `installment_charge_ids` (as
-- parcelas criadas) para rastreabilidade — o total do caso continua vindo da
-- soma das cobranças abertas.
--
-- Domain: toca-financial/internal/domain/collection/agreement.go
-- Repo:   toca-financial/internal/repositories/collection_agreement_postgres.go

CREATE TABLE IF NOT EXISTS financial.collection_agreements (
    id          uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id   text NOT NULL,
    metadata    jsonb NOT NULL DEFAULT '{}'::jsonb,
    created_at  timestamptz NOT NULL DEFAULT now(),
    updated_at  timestamptz NOT NULL DEFAULT now(),
    deleted_at  timestamptz
);

CREATE INDEX IF NOT EXISTS idx_financial_collection_agreements_tenant
    ON financial.collection_agreements (tenant_id)
    WHERE deleted_at IS NULL;

-- Busca do acordo pelo caso: é o caminho de leitura da tela (um acordo por caso).
CREATE INDEX IF NOT EXISTS idx_financial_collection_agreements_collection
    ON financial.collection_agreements (tenant_id, ((metadata->>'collection_id')))
    WHERE deleted_at IS NULL;

CREATE INDEX IF NOT EXISTS idx_financial_collection_agreements_status
    ON financial.collection_agreements (tenant_id, ((metadata->>'status')))
    WHERE deleted_at IS NULL;

-- Um acordo por caso: o caso não pode ter dois acordos vigentes. O índice é
-- parcial porque acordos BROKEN/CANCELLED/FULFILLED são histórico e podem
-- coexistir ao longo do tempo.
CREATE UNIQUE INDEX IF NOT EXISTS uq_financial_collection_agreements_active
    ON financial.collection_agreements (tenant_id, ((metadata->>'collection_id')))
    WHERE deleted_at IS NULL AND (metadata->>'status') IN ('DRAFT', 'ACTIVE');

DROP POLICY IF EXISTS tenant_isolation_select ON financial.collection_agreements;
CREATE POLICY tenant_isolation_select ON financial.collection_agreements
    FOR SELECT TO authenticated
    USING (tenant_id = (auth.jwt() -> 'app_metadata' ->> 'tenant_id'));

DROP POLICY IF EXISTS tenant_isolation_modify ON financial.collection_agreements;
CREATE POLICY tenant_isolation_modify ON financial.collection_agreements
    FOR ALL TO authenticated
    USING (tenant_id = (auth.jwt() -> 'app_metadata' ->> 'tenant_id'))
    WITH CHECK (tenant_id = (auth.jwt() -> 'app_metadata' ->> 'tenant_id'));

ALTER TABLE financial.collection_agreements ENABLE ROW LEVEL SECURITY;
