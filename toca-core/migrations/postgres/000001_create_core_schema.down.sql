-- Reverter RLS Policies
DROP POLICY IF EXISTS "tenant_isolation_delete_properties" ON core.properties;
DROP POLICY IF EXISTS "tenant_isolation_update_properties" ON core.properties;
DROP POLICY IF EXISTS "tenant_isolation_insert_properties" ON core.properties;
DROP POLICY IF EXISTS "tenant_isolation_select_properties" ON core.properties;

-- Reverter Trigger
DROP TRIGGER IF EXISTS set_timestamp_core_properties ON core.properties;

-- Reverter Tabelas e Índices
DROP TABLE IF EXISTS core.properties CASCADE;

-- Reverter Função (somente se não for mais necessária)
DROP FUNCTION IF EXISTS trigger_set_timestamp();

-- Reverter Schema
DROP SCHEMA IF EXISTS core CASCADE;
