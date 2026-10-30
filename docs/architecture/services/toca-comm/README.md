# toca-comm (Comunicação)

## Papel

- Mensagens, canais, notificações in-app e integrações de e-mail.
- Consome eventos de domínio para notificar usuários e manter timelines/canais atualizados.

## HTTP (via gateway)

- Prefixos: `/api/v1/messages`, `/api/v1/channels`, `/api/v1/templates`, `/api/v1/notifications`, `/api/v1/ws`, `/api/v1/webhooks/*`
- Porta: `8086`

## Rotas principais (API)

Fonte: `toca-comm/internal/api/v1/routes/routes.go`.

- Webhooks (público, sem JWT):
  - `GET/POST /api/v1/webhooks/whatsapp`
  - `POST /api/v1/webhooks/sms`
- Notificações (internal key):
  - `POST /api/v1/notifications/send`
- Comm internal (internal key):
  - `POST /api/v1/comm/internal/channels/ensure`
  - `GET /api/v1/comm/internal/channels`
  - `POST /api/v1/comm/internal/notifications/:accountId`
- Notificações in-app (JWT):
  - `GET /api/v1/notifications/inapp`
  - `GET /api/v1/notifications/inapp/count`
  - `POST /api/v1/notifications/inapp/:id/read`
- WebSocket (JWT):
  - `GET /api/v1/ws`
- Channels/Messages (JWT):
  - `GET/POST /api/v1/channels`
  - `GET/PATCH /api/v1/channels/:id`
  - `POST /api/v1/channels/:id/messages`
  - Reações, pin/mute, participants e endpoints correlatos em `/api/v1/channels/*`

## Eventos (RabbitMQ)

- Consome:
  - Eventos de deal/contract/proposal (esteira), lead/broker (CRM), property/partnership (Core), tickets/appointments/maintenance/inspection (Ops/Calendar), financeiro (invoice/rental/charge), além de eventos de Auth/Portal.
  - Vitrine (ADR 0006): `site.lead.received` (broadcast WS `site_lead_received`) e `site.listing.published`/`unpublished` (broadcast `site_listing_*`). Sem essas filas o exchange não tem rota e o outbox do `toca-sites` fica em `NO_ROUTE`.
  - Lista completa e roteamento de handlers: `toca-comm/internal/service/event_consumer.go`.
- Publica (quando aplicável):
  - Eventos internos de coordenação/escalação (ex.: supervisor required) originados do consumer.

## Integrações externas

- E-mail transacional:
  - Em produção, Resend.
  - Em local, quando `SMTP_HOST` está definido, o `toca-comm` envia pelo
    Mailpit do Supabase (`host.docker.internal:54325`); a caixa fica em
    http://localhost:54324. Isso cobre convite do portal e OTP sem depender de
    domínio verificado no Resend.
- Resend (e-mail):
  - Envio de e-mails transacionais e templates.
  - Env: `RESEND_API_KEY` e `RESEND_FROM_ADDRESS`.

## Dados

- Postgres (schema `comm`), com isolamento por `tenant_id` (RLS quando usando Supabase Postgres).

## Chamadas para outros serviços

- Recebe comandos internos (`/comm/internal/*`) para garantir canais e criar notificações a partir de outros serviços.

## Engines de fundo e notificações por e-mail

- `ChatReminderEngine` (`internal/service/chat_reminder_engine.go`): lembrete de
  mensagem não lida após 30 min — idempotência ancorada no grupo
  `chat_reminder:{canal}:{participante}:{última não lida}` (um lembrete por
  grupo; mensagem nova forma grupo novo). Lock advisory + presença/mute/opt-out.
- `EmailNotifier` com gates (flag global → categoria → opt-out → presença →
  cooldown → dedup em `comm.sent_externals`) e categorias F0–F6:
  `lead_assigned`, `deal_updates`, `proposals`, `appointments`,
  `recommendations`, `chat_reminders`.
- Consumidor `comm_proposal_followup_email` (`proposal.followup_due`): e-mail ao
  cliente com cópia ao corretor; dedup pela chave lógica do payload (o MessageId
  do outbox é novo a cada publicação, então a chave deriva de
  `DeriveNotificationID(proposta, "proposal_followup", estágio)`).

## Código chave

- Rotas: `internal/api/v1/routes/routes.go`
- Event consumer: `internal/service/event_consumer.go`
- Email service: `internal/service/email_service.go`
