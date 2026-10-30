-- O ledger de migrations (ops.deployment_migration_checksums) deve ser
-- inacessível às roles de runtime. A migration 20260721190000 revogou de
-- PUBLIC/anon/authenticated/service_role, mas as ALTER DEFAULT PRIVILEGES do
-- schema ops concedem a toca_app/toca_service acesso a toda tabela nova, então
-- o ledger herdou SELECT/INSERT/UPDATE/DELETE dessas roles. Revoga agora, para
-- o gate validar que a role de serviço não tem privilégio no ledger.

REVOKE ALL ON TABLE ops.deployment_migration_checksums
    FROM PUBLIC, anon, authenticated, service_role, toca_app, toca_service;

REVOKE ALL ON FUNCTION ops.reject_deployment_migration_checksum_mutation()
    FROM PUBLIC, anon, authenticated, service_role, toca_app, toca_service;
