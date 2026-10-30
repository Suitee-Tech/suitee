# toca-portal (Portal / Integrações)

## Papel

- Endpoints públicos para feed e webhooks de integrações de portais.
- Gestão de integrações, sincronizações e estatísticas de portal.

## HTTP (via gateway)

- Prefixos: `/api/v1/portals`, `/api/v1/integrations`, `/api/v1/portal-stats`
- Porta: `8089`

## Rotas principais (API)

Fonte: `toca-portal/internal/handlers/router.go`.

- Público (sem JWT):
  - `GET /api/v1/portals/feed/:token`
  - `POST /api/v1/portals/webhook/:integrationID`
- Integrações (JWT):
  - `POST/GET /api/v1/integrations`
  - `GET/PATCH/DELETE /api/v1/integrations/:id`
  - Sync: `POST /api/v1/integrations/:id/sync`, `GET /api/v1/integrations/:id/syncs`
  - Stats por integração: `GET /api/v1/integrations/:id/stats/leads`
- Portal stats (JWT):
  - `GET /api/v1/portal-stats/dashboard`
  - `GET /api/v1/portal-stats/portal-leads`

## Eventos (RabbitMQ)

- Publica (quando aplicável):
  - `portal.lead_received` (consumido por `toca-comm` para notificação/triagem)
- Consome:
  - Não há subscribers diretos no serviço.

## Dados

- Postgres (schema `portal`), com isolamento por `tenant_id` (RLS quando usando Supabase Postgres).

## Chamadas para outros serviços

- Emite eventos consumidos pelo `toca-comm`.

## Código chave

- Router: `internal/handlers/router.go`
