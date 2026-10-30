-- financial.tax_obligations — tributos retidos a recolher.
--
-- O plano de contas já tem a conta "Tributos retidos a recolher" (2.1.05) e a
-- liquidação da comissão já credita essa conta quando há desconto (IRRF, INSS,
-- ISS). O que faltava era a VIDA do passivo depois disso: nada dizia quanto foi
-- retido, em que competência, até quando deve ser recolhido e quando foi pago —
-- o valor simplesmente ficava parado na conta contábil.
--
-- Esta tabela materializa a obrigação por (tenant, tipo, competência):
--
--   * cada item aponta para a comissão e o desconto que o originou (derivado da
--     comissão, nunca digitado à mão — recomputável e idempotente);
--   * vencimento por tipo (IRRF/INSS: dia 20 do mês seguinte; ISS: dia 10), com
--     ajuste para dia útil;
--   * a baixa (recolhimento) exige comprovante e gera o lançamento contábil
--     (débito em 2.1.05 / crédito em caixa), fechando o ciclo que a liquidação da
--     comissão abriu.
--
-- Domain: toca-financial/internal/domain/tax_obligation/tax_obligation.go
-- Service: toca-financial/internal/services/tax_obligation_service.go

CREATE TABLE IF NOT EXISTS financial.tax_obligations (
    id          uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id   text NOT NULL,
    metadata    jsonb NOT NULL DEFAULT '{}'::jsonb,
    created_at  timestamptz NOT NULL DEFAULT now(),
    updated_at  timestamptz NOT NULL DEFAULT now(),
    deleted_at  timestamptz
);

CREATE INDEX IF NOT EXISTS idx_finance_tax_obligations_tenant
    ON financial.tax_obligations (tenant_id)
    WHERE deleted_at IS NULL;

-- Uma obrigação aberta por (tipo, competência): o acúmulo do mês é a própria
-- linha, e é isso que o índice garante para duas retenções do mesmo imposto
-- caírem juntas.
CREATE UNIQUE INDEX IF NOT EXISTS uq_finance_tax_obligations_kind_competence
    ON financial.tax_obligations (tenant_id,
                                  ((metadata->>'kind')),
                                  ((metadata->>'competence')))
    WHERE deleted_at IS NULL
      AND (metadata->>'kind') IS NOT NULL
      AND (metadata->>'competence') IS NOT NULL
      AND COALESCE((metadata->>'status'), '') <> 'CANCELLED';

CREATE INDEX IF NOT EXISTS idx_finance_tax_obligations_due
    ON financial.tax_obligations (tenant_id,
                                  ((metadata->>'status')),
                                  ((metadata->>'due_date')))
    WHERE deleted_at IS NULL;

DROP POLICY IF EXISTS tenant_isolation_select ON financial.tax_obligations;
CREATE POLICY tenant_isolation_select ON financial.tax_obligations
    FOR SELECT TO authenticated
    USING (
        tenant_id = (auth.jwt() -> 'app_metadata' ->> 'tenant_id')
    );

DROP POLICY IF EXISTS tenant_isolation_modify ON financial.tax_obligations;
CREATE POLICY tenant_isolation_modify ON financial.tax_obligations
    FOR ALL TO authenticated
    USING (
        tenant_id = (auth.jwt() -> 'app_metadata' ->> 'tenant_id')
    )
    WITH CHECK (
        tenant_id = (auth.jwt() -> 'app_metadata' ->> 'tenant_id')
    );

ALTER TABLE financial.tax_obligations ENABLE ROW LEVEL SECURITY;
