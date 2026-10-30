-- Fix: audit events sem contexto de tenant.
--
-- eventos como auth.login_success / auth.register_success ocorrem ANTES de o
-- usuário ter um tenant ativo (a seleção de tenant é posterior via
-- switch-context). Nesses casos o pkg/audit.Record passa tenant_id NULL, e a
-- coluna era NOT NULL, fazendo o INSERT falhar (audit.write_failed —
-- "null value in column tenant_id of relation events_yyyy_mm"). O helper já
-- foi desenhado para aceitar tenant ausente (comentário em auth_service.go:
-- "a tabela tem coluna tenant_id default null"), então a coluna deve ser
-- nullable. A RLS de leitura (tenant_isolation_select) continua excluindo as
-- linhas de tenant NULL das queries por tenant (correcto: eventos de sistema
-- não pertencem a um tenant).

ALTER TABLE audit.events ALTER COLUMN tenant_id DROP NOT NULL;

COMMENT ON COLUMN audit.events.tenant_id IS
    'Tenant dono do evento. NULL para eventos de sistema sem contexto de tenant (ex. auth.login_success/register_success), que ocorrem antes da seleção de tenant.';
