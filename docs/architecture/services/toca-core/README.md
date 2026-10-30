# toca-core (Core / Imóveis)

## Papel

- Catálogo de imóveis (Property), captação e parcerias.
- Fonte de verdade do domínio de imóveis; outros serviços consomem por projeção/eventos ou chamadas internas.

## HTTP (via gateway)

- Prefixos: `/api/v1/properties`, `/api/v1/captacao`, `/api/v1/partnerships`, `/api/v1/company-settings`, `/api/v1/me/permissions`, `/api/v1/health`
- Porta: `8082`

## Rotas principais (API)

Fonte: `toca-core/internal/handlers/router.go`.

- Permissions:
  - `GET /api/v1/me/permissions`
- Properties:
  - `POST/GET /api/v1/properties`
  - `GET/PATCH/DELETE /api/v1/properties/:id`
  - Sub-recursos: `media`, `owners`, `capturing-brokers`, `interested-leads`, `active-deal`
- Captação:
  - `POST /api/v1/captacao`
- Partnerships:
  - `POST/GET /api/v1/partnerships`
  - `GET/PATCH/DELETE /api/v1/partnerships/:id`
  - `GET /api/v1/partnerships/shared/properties`
  - `GET /api/v1/partnerships/:id/shared-portfolio`
- Company settings:
  - `GET/PATCH /api/v1/company-settings/appearance`
  - `GET/PATCH /api/v1/company-settings/permissions`
  - `GET/PATCH /api/v1/company-settings/notifications`
  - `GET/POST/DELETE /api/v1/company-settings/webhooks`
  - `GET/POST/DELETE /api/v1/company-settings/api-keys`

## Rotas internas (serviço-a-serviço, `X-Internal-Key`)

Não passam pelo gateway (`RejectInternalPaths`). O tenant é resolvido pela
credencial + `tenant_id` explícito.

- `GET /api/v1/core/internal/properties/:id?tenant_id=` — leitura do agregado
  (usada por toca-financial/DIMOB e toca-sites).
- `GET /api/v1/core/internal/properties?tenant_id=&limit=&offset=` — listagem
  paginada do estoque; usada pelo toca-sites para o backfill da vitrine no
  primeiro enable.
- `GET /api/v1/core/internal/tenant-settings` — fuso do tenant.

## Eventos (RabbitMQ)

- Publica:
  - `property.captured`
  - `property.capturing_broker_added`
  - `property.updated`
  - `partnership.invited`
  - `partnership.accepted`
- Consome:
  - `deal.status_changed` (atualiza projeções do imóvel, ex.: `Property.ActiveDealID`)

## Dados

- Postgres (schema `core`), com isolamento por `tenant_id` (RLS quando usando Supabase Postgres).

## Chamadas para outros serviços

- Reage a eventos de `toca-deal` para sincronizar estado do imóvel.

## Código chave

- Router: `internal/handlers/router.go`
- Contratos de eventos: `pkg/events/contracts/captacao_events.go`, `pkg/events/contracts/partnership_events.go`
