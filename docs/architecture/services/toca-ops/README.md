# toca-ops (Operações)

## Papel

- Operações do dia a dia: maintenance, tickets/chamados, chaves e inspeções/vistorias.

## HTTP (via gateway)

- Prefixos: `/api/v1/maintenance`, `/api/v1/tickets`, `/api/v1/keys`, `/api/v1/inspections`
- Porta: `8087`

## Rotas principais (API)

Fonte: `toca-ops/internal/handlers/router.go`.

- Interno (internal key):
  - Maintenance: `POST /api/v1/ops/internal/properties/:id/maintenance`, `GET /api/v1/ops/internal/properties/:id/maintenance`
  - Tickets: `POST /api/v1/ops/internal/tickets`, `GET /api/v1/ops/internal/tickets`, `POST /api/v1/ops/internal/tickets/:id/comments`, `GET /api/v1/ops/internal/tickets/:id`
  - Inspections: `GET /api/v1/ops/internal/contracts/:id/inspections`
- Maintenance (JWT operador):
  - `POST/GET /api/v1/maintenance`
  - `GET/PATCH/DELETE /api/v1/maintenance/:id`
  - `PATCH /api/v1/maintenance/:id/status`
- Keys (JWT operador):
  - `POST/GET /api/v1/keys`
  - `GET/DELETE /api/v1/keys/:id`
  - `POST /api/v1/keys/:id/checkout`
  - `POST /api/v1/keys/:id/checkin`
- Inspections (JWT operador):
  - `POST/GET /api/v1/inspections`
  - `GET/PATCH/DELETE /api/v1/inspections/:id`
  - Deductions, uploads e geração de laudo como sub-recursos
- Tickets (JWT operador):
  - Endpoints de listagem, detalhe, assign, status e comments em `/api/v1/tickets/*`

## Eventos (RabbitMQ)

- Publica:
  - `maintenance.requested`, `maintenance.completed`
  - `inspection.scheduled`, `inspection.completed`, `inspection.cancelled`
  - `ticket.created`, `ticket.status_changed`, `ticket.comment_added`, `ticket.assigned`
- Consome:
  - `deal.contract.signed` (automação operacional, ex.: agendar vistoria de entrada)

## Dados

- Postgres (schema `ops`), com isolamento por `tenant_id` (RLS quando usando Supabase Postgres).

## Chamadas para outros serviços

- Reage a eventos do `toca-deal` e emite eventos consumidos por `toca-deal`, `toca-financial` e `toca-comm`.

## Código chave

- Router: `internal/handlers/router.go`
- Contratos de eventos: `pkg/events/contracts/maintenance_events.go`, `pkg/events/contracts/inspection_events.go`, `pkg/events/contracts/ticket_events.go`
