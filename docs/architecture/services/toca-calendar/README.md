# toca-calendar (Agenda)

## Papel

- Agendamentos e compromissos (`appointments`).

## HTTP (via gateway)

- Prefixos: `/api/v1/appointments`
- Porta: `8088`

## Rotas principais (API)

Fonte: `toca-calendar/internal/handlers/router.go`.

- Interno (internal key):
  - `POST /api/v1/calendar/internal/appointments`
  - `GET /api/v1/calendar/internal/deals/:id/appointments`
- Appointments (JWT):
  - `POST /api/v1/appointments`
  - `GET /api/v1/appointments`
  - `GET /api/v1/appointments/:id`
  - `PUT /api/v1/appointments/:id`
  - `DELETE /api/v1/appointments/:id`

## Eventos (RabbitMQ)

- Publica:
  - `appointment.updated`
  - `appointment.cancelled`
- Consome:
  - Não há subscribers diretos no serviço (opera principalmente por HTTP e/ou por orquestração via `toca-deal`).

## Dados

- Postgres (schema `calendar`), com isolamento por `tenant_id` (RLS quando usando Supabase Postgres).

## Chamadas para outros serviços

- Recebe requisições do `toca-deal` (portal) e emite eventos consumidos por `toca-comm`.

## Código chave

- Router: `internal/handlers/router.go`
- Contratos de eventos: `pkg/events/contracts/calendar_events.go`
