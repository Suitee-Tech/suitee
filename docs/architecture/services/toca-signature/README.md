# toca-signature (Assinatura)

## Papel

- Integração com provedor de assinatura (Clicksign).
- Gestão de envelopes/signers e consolidação de status de assinatura.
- Geração/transformação de documento para assinatura (HTML → PDF via Gotenberg).

## HTTP (via gateway)

- Prefixos: `/api/v1/signature` e `/webhooks/clicksign`
- Porta: `8093`

## Rotas principais (API)

Fonte: `toca-signature/internal/api/v1/routes/routes.go`.

- Webhook público (sem JWT):
  - `POST /webhooks/clicksign`
- Interno (internal key):
  - `GET /api/v1/signature/internal/contracts/:id/envelope`
  - `POST /api/v1/signature/internal/contracts/:id/envelope`
  - `POST /api/v1/signature/internal/contracts/:id/envelope/refresh-signed-document-url`
- API (JWT + permission `contracts:*`):
  - `GET /api/v1/signature/config`
  - `GET /api/v1/signature/envelopes`
  - `GET /api/v1/signature/envelopes/:id`
  - `GET /api/v1/signature/contracts/:id/envelopes`
  - `POST /api/v1/signature/envelopes/:id/cancel`
  - `POST /api/v1/signature/envelopes/:id/resend`
  - `POST /api/v1/signature/envelopes/:id/reconcile`
  - Signers: `POST/PATCH /api/v1/signature/envelopes/:id/signers/*`

## Eventos (RabbitMQ)

- Consome:
  - `signature.requested` (disparado pelo `toca-deal` quando um contrato entra em fluxo de assinatura)
- Publica:
  - `signature.signer_viewed`
  - `signature.signer_signed`
  - `signature.envelope_started`
  - `signature.envelope_finished`
  - `signature.refused`
  - `signature.expired`

## Integrações externas

- Clicksign:
  - Client HTTP para `/api/v3/*` e webhook para eventos de assinatura.
  - O webhook aplica allowlist de IP e valida `Content-Hmac`.
  - **Cancelamento**: a v3 não cancela o envelope (`PATCH /envelopes/{id}` aceita
    só `draft|running`; `DELETE` só vale para rascunho). Cancela-se **por
    documento** — `PATCH /envelopes/{id}/documents/{doc}` com
    `{"status":"canceled"}` — e é isso que leva o envelope a `canceled` e
    invalida o link (`CancelEnvelope` em `internal/clicksign/client.go`).
  - As notificações de assinatura são disparadas por
    `POST /envelopes/{id}/notifications` depois da ativação; com grupos de
    ordem, a Clicksign notifica só o grupo da vez (o grupo seguinte entra
    quando o anterior conclui).
  - O webhook é chamador de sistema (sem JWT): a escrita do envelope vai pelo
    pool de serviço e o contexto recebe o `tenant_id` resolvido da própria
    linha — sem isso a RLS esconde a linha e o webhook morre em "envelope not
    found" sem publicar nada (incidente 14 da revisão de permissões).

## Dados

- Postgres (schema `signature`), com isolamento por `tenant_id` (RLS quando usando Supabase Postgres).

## Chamadas para outros serviços

- Consumidor de eventos do `toca-deal`; emite eventos consumidos por `toca-deal` e `toca-comm`.

## Código chave

- Rotas: `internal/api/v1/routes/routes.go`
- Webhook: `internal/api/v1/handlers/webhook_handler.go`
- Clicksign client: `internal/clicksign/client.go`
- Contratos de eventos: `pkg/events/contracts/signature_events.go`
