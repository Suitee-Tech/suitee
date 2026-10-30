-- Criar schema isolado para o domínio core
CREATE SCHEMA IF NOT EXISTS core;

-- Tabela piloto: Properties (multi-tenant com RLS)
CREATE TABLE IF NOT EXISTS core.properties (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL,
    -- Campos básicos (exemplo)
    title VARCHAR(255) NOT NULL,
    price DECIMAL(15,2) NOT NULL DEFAULT 0.0,
    status VARCHAR(50) NOT NULL DEFAULT 'active',
    type VARCHAR(50) NOT NULL DEFAULT 'residential',
    -- Metadados JSON para flexibilidade legada do Mongo
    metadata JSONB DEFAULT '{}'::jsonb,
    
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    deleted_at TIMESTAMPTZ
);

-- Índices principais
CREATE INDEX idx_core_properties_tenant_id ON core.properties(tenant_id);
CREATE INDEX idx_core_properties_status ON core.properties(tenant_id, status);

-- Trigger de Updated At automático
CREATE OR REPLACE FUNCTION trigger_set_timestamp()
RETURNS TRIGGER AS $$
BEGIN
  NEW.updated_at = NOW();
  RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER set_timestamp_core_properties
BEFORE UPDATE ON core.properties
FOR EACH ROW
EXECUTE FUNCTION trigger_set_timestamp();

-- ==========================================
-- ROW LEVEL SECURITY (RLS)
-- ==========================================
ALTER TABLE core.properties ENABLE ROW LEVEL SECURITY;

-- Select (Leitura isolada)
CREATE POLICY "tenant_isolation_select_properties" ON core.properties
FOR SELECT USING (
  tenant_id = ((auth.jwt() -> 'app_metadata' ->> 'tenant_id')::uuid)
);

-- Insert (Valida que o tenant_id sendo inserido pertence ao usuário logado)
CREATE POLICY "tenant_isolation_insert_properties" ON core.properties
FOR INSERT WITH CHECK (
  tenant_id = ((auth.jwt() -> 'app_metadata' ->> 'tenant_id')::uuid)
);

-- Update (Modificação isolada)
CREATE POLICY "tenant_isolation_update_properties" ON core.properties
FOR UPDATE USING (
  tenant_id = ((auth.jwt() -> 'app_metadata' ->> 'tenant_id')::uuid)
) WITH CHECK (
  tenant_id = ((auth.jwt() -> 'app_metadata' ->> 'tenant_id')::uuid)
);

-- Delete (Soft delete preferencialmente via UPDATE, mas mantendo a restrição de hard delete se ocorrer)
CREATE POLICY "tenant_isolation_delete_properties" ON core.properties
FOR DELETE USING (
  tenant_id = ((auth.jwt() -> 'app_metadata' ->> 'tenant_id')::uuid)
);
