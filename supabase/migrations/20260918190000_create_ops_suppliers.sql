-- Cadastro de profissionais/fornecedores da imobiliária (eletricista,
-- encanador, vistoriador, chaveiro). É a fonte de verdade para reutilizar o
-- mesmo prestador em chamados de manutenção e vistorias, com características,
-- contato e tipo de serviço — sem redigitar o nome a cada agendamento.
CREATE TABLE IF NOT EXISTS ops.suppliers (
  id UUID PRIMARY KEY,
  tenant_id UUID NOT NULL,
  metadata JSONB NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  deleted_at TIMESTAMPTZ
);

CREATE INDEX IF NOT EXISTS idx_ops_suppliers_tenant_id ON ops.suppliers(tenant_id);

-- Seleção por tipo de serviço e nome ativo é o caso de uso dominante.
CREATE INDEX IF NOT EXISTS idx_ops_suppliers_tenant_service_type
  ON ops.suppliers (tenant_id, (metadata->>'service_type'))
  WHERE deleted_at IS NULL;

CREATE INDEX IF NOT EXISTS idx_ops_suppliers_tenant_name
  ON ops.suppliers (tenant_id, lower(metadata->>'name'))
  WHERE deleted_at IS NULL;

-- Reuso do cadastro: impedir prestador duplicado pelo mesmo contato.
CREATE INDEX IF NOT EXISTS idx_ops_suppliers_tenant_phone
  ON ops.suppliers (tenant_id, (metadata->>'phone'))
  WHERE deleted_at IS NULL AND COALESCE(metadata->>'phone', '') <> '';

-- Row Level Security (mandatory for multi-tenant tables).
ALTER TABLE ops.suppliers ENABLE ROW LEVEL SECURITY;

DROP POLICY IF EXISTS tenant_isolation ON ops.suppliers;
CREATE POLICY tenant_isolation ON ops.suppliers
    FOR ALL TO authenticated USING (
        tenant_id = NULLIF(
            (SELECT auth.jwt()) -> 'app_metadata' ->> 'tenant_id',
            ''
        )::uuid
    ) WITH CHECK (
        tenant_id = NULLIF(
            (SELECT auth.jwt()) -> 'app_metadata' ->> 'tenant_id',
            ''
        )::uuid
    );
