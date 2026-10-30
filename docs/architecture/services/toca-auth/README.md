# toca-auth (IAM / Identidade)

## Papel

- Identidade e acesso: usuários, contas/empresas, RBAC, convites, login, refresh, OTP e contexto do tenant.
- Integra com Supabase Auth para emissão/validação de tokens e gestão de usuários.

## HTTP (via gateway)

- Prefixos: `/api/v1/auth`, `/api/v1/companies`, `/api/v1/settings`, `/api/v1/lgpd`, `/api/v1/audit-trail`
- Porta: `8081`

## Rotas principais (API)

Fonte: `toca-auth/internal/api/v1/routes/routes.go`.

- Auth público:
  - `POST /api/v1/auth/register`
  - `POST /api/v1/auth/login`
  - `POST /api/v1/auth/refresh`
  - `POST /api/v1/auth/otp/request`
  - `POST /api/v1/auth/otp/verify`
- Auth (JWT):
  - `GET /api/v1/auth/me`
  - `PATCH /api/v1/auth/me`
  - `POST /api/v1/auth/switch-context`
  - `POST /api/v1/auth/logout`
- Interno (internal key):
  - `POST /api/v1/auth/internal/accounts/ensure-client`
  - `GET /api/v1/auth/internal/companies/:id/members`
- Empresas:
  - `GET/POST /api/v1/companies`
  - `GET/PATCH/DELETE /api/v1/companies/:id`
  - `GET/POST /api/v1/companies/:id/members`
- LGPD:
  - `GET /api/v1/lgpd/me/data`
  - `DELETE /api/v1/lgpd/me/account`
- Audit trail:
  - `GET /api/v1/audit-trail`

## Eventos (RabbitMQ)

- Publica (consumido principalmente por `toca-comm`):
  - `auth.user.registered`
  - `auth.password_reset.requested`
  - `auth.invitation.created`
  - `auth.otp.requested`
  - `auth.client.invited`

## Integrações externas

- Supabase Auth:
  - API REST `/auth/v1/*` (admin/users, token, otp, verify, generate_link, logout).
  - Env: `SUPABASE_URL`, `SUPABASE_ANON_KEY`, `SUPABASE_SERVICE_ROLE_KEY`.

## Dados

- Postgres (schema `auth`), com isolamento por `tenant_id` (RLS quando usando Supabase Postgres).

## Chamadas para outros serviços

- Internamente, depende do serviço de comunicações (`toca-comm`) para envio de e-mails/notificações em fluxos de convite/OTP (via eventos e/ou endpoints internos).

## Código chave

- Rotas: `internal/api/v1/routes/routes.go`
- Supabase Auth client: `internal/infrastructure/supabase/auth_client.go`
- Serviço principal: `internal/service/auth_service.go`
