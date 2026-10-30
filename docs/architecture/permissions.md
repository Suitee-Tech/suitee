# Permissões (RBAC)

## Visão geral

O controle de acesso da Suitee usa RBAC baseado em:

- **Functional role** do usuário (ex.: `admin`, `gestor`, `corretor`, `operacoes`, `financeiro`, `marketing`, `auditor`, `parceiro`, `cliente`)
- **Permissions** no formato `recurso:acao` (ex.: `leads:read`, `properties:write`)

O catálogo canônico está em [permission_catalog.go](../../pkg/auth/permission_catalog.go).

## JWT claims

O middleware lê o JWT do Supabase e extrai os campos de `app_metadata`:

- `tenant_id`
- `functional_role`
- `permission` (mantido para compatibilidade)

Implementação: [middleware.go](../../pkg/auth/middleware.go).

## Como proteger rotas (Go)

As rotas devem exigir permission por endpoint (definida no catálogo).

- Exemplo (conceito): `RequirePermission(permissionRepo, "leads:read")`
- O serviço obtém a permission do request via contexto preenchido pelo middleware.

## Como evoluir o catálogo

- Adicionar novas entries em [permission_catalog.go](../../pkg/auth/permission_catalog.go)
- Manter IDs estáveis (strings usadas nas rotas e no frontend)
