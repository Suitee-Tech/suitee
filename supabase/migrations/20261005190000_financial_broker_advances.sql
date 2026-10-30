-- financial.broker_advances — adiantamentos da imobiliária ao corretor.
--
-- A imobiliária adianta dinheiro ao corretor (antes da comissão existir ou ser
-- paga) e depois recupera o valor abatendo nas comissões seguintes. Sem isso o
-- adiantamento só existia no razão (comentário do relatório financeiro) ou na
-- cabeça de quem operou: não havia saldo, não havia o que abater e o desconto na
-- comissão não tinha contrapartida.
--
-- O adiantamento é um ATIVO (valor a receber do corretor): a concessão debita
-- "contas a receber" e credita caixa; o abatimento na comissão credita o mesmo
-- ativo, fechando o ciclo. Não é tributo, então NÃO gera obrigação a recolher.
--
-- Domain: toca-financial/internal/domain/broker_advance/broker_advance.go
-- Service: toca-financial/internal/services/broker_advance_service.go

CREATE TABLE IF NOT EXISTS financial.broker_advances (
    id          uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id   text NOT NULL,
    metadata    jsonb NOT NULL DEFAULT '{}'::jsonb,
    created_at  timestamptz NOT NULL DEFAULT now(),
    updated_at  timestamptz NOT NULL DEFAULT now(),
    deleted_at  timestamptz
);

CREATE INDEX IF NOT EXISTS idx_finance_broker_advances_tenant
    ON financial.broker_advances (tenant_id)
    WHERE deleted_at IS NULL;

CREATE INDEX IF NOT EXISTS idx_finance_broker_advances_broker
    ON financial.broker_advances (tenant_id, ((metadata->>'broker_id')), ((metadata->>'status')))
    WHERE deleted_at IS NULL AND (metadata->>'broker_id') IS NOT NULL;

DROP POLICY IF EXISTS tenant_isolation_select ON financial.broker_advances;
CREATE POLICY tenant_isolation_select ON financial.broker_advances
    FOR SELECT TO authenticated
    USING (tenant_id = (auth.jwt() -> 'app_metadata' ->> 'tenant_id'));

DROP POLICY IF EXISTS tenant_isolation_modify ON financial.broker_advances;
CREATE POLICY tenant_isolation_modify ON financial.broker_advances
    FOR ALL TO authenticated
    USING (tenant_id = (auth.jwt() -> 'app_metadata' ->> 'tenant_id'))
    WITH CHECK (tenant_id = (auth.jwt() -> 'app_metadata' ->> 'tenant_id'));

ALTER TABLE financial.broker_advances ENABLE ROW LEVEL SECURITY;
