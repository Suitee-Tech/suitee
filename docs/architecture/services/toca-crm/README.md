# toca-crm (CRM / Leads)

## Papel

- Leads, brokers, roleta de distribuição, qualificação e conversão de lead → cliente.
- Fonte de verdade dos agregados de CRM.

## HTTP (via gateway)

- Prefixos: `/api/v1/leads`, `/api/v1/brokers`, `/api/v1/customers`, `/api/v1/exports`
- Porta: `8084`

## Rotas principais (API)

Fonte: `toca-crm/internal/api/v1/routes/routes.go`.

- Interno (internal key):
  - `GET /api/v1/crm/internal/leads/:id`
- Leads:
  - `POST /api/v1/leads`
  - `GET /api/v1/leads`
  - `GET/PATCH /api/v1/leads/:id`
  - `POST /api/v1/leads/:id/notes`
  - `POST /api/v1/leads/:id/first-contact`
  - `POST /api/v1/leads/:id/disqualify`
  - `POST /api/v1/leads/:id/convert`
  - `POST /api/v1/leads/:id/invite`
  - Roleta: `GET /leads/roulette/*`, `POST /leads/:id/roulette/accept`, `POST /leads/:id/roulette/reject`
  - Config roleta: `GET/POST /leads/roulette/config`
- Brokers:
  - `POST/GET /api/v1/brokers`
  - `GET/PATCH/DELETE /api/v1/brokers/:id`
  - `PATCH /api/v1/brokers/me/qualification`
- Customers:
  - `GET /api/v1/customers/:id/profile`
  - `GET /api/v1/customers/:id/summary`
  - Agregados: `GET /customers/:id/leads|deals|charges|appointments|tickets`
- Exports:
  - `POST /api/v1/leads/export`
  - `GET /api/v1/exports/mine`
  - `GET /api/v1/exports/:id`

## Eventos (RabbitMQ)

- Publica:
  - `lead.created`
  - `lead.assigned`
  - `lead.escalated`
  - `lead.status_changed`
  - `lead.converted`
  - `lead.updated`
  - `broker.updated`
  - `lead.first_contact_sla_violated`
- Consome:
  - `lead.created` (auto-assign/roleta)
  - `deal.contract.signed` (conversões relacionadas a contrato)
  - `property.captured` (ajustes de owner lead / captação)

## Dados

- Postgres (schema `crm`), com isolamento por `tenant_id` (RLS quando usando Supabase Postgres).

## Chamadas para outros serviços

- Orquestra automações via eventos; quando precisa dados de outros domínios, a preferência é consumir projeções/eventos ou chamar via HTTP interno.

## Código chave

- Rotas: `internal/api/v1/routes/routes.go`
- Contratos de eventos: `pkg/events/contracts/lead_events.go`, `pkg/events/contracts/broker_events.go`
