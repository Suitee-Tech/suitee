# toca-deal (Esteira / Contratos) — Orchestrator/Saga

## Papel

- Orquestra propostas, deals, contratos, templates e documentos.
- Emite eventos de downstream para financeiro/comunicação/agenda/assinatura e consolida o estado a partir de callbacks e eventos consumidos.

## HTTP (via gateway)

- Prefixos: `/api/v1/deals`, `/api/v1/contracts`, `/api/v1/contract-templates`, `/api/v1/documents`, `/api/v1/portal`, `/api/v1/client-portal/*`
- Porta: `8085`

## Rotas principais (API)

Fonte: `toca-deal/internal/api/v1/routes/routes.go`.

- Interno (internal key):
  - `GET /api/v1/deal/internal/deals/:id`
  - `GET /api/v1/deal/internal/contracts/:id`
  - `GET /api/v1/deal/internal/documents/:id/content`
- Portal legado:
  - `GET /api/v1/portal/:token`
  - `GET /api/v1/portal/:token/checklist`
  - `PATCH /api/v1/portal/:token/checklist/:itemId/upload`
- Client portal (JWT role `cliente`):
  - `GET /api/v1/client-portal/deals/mine`
  - `GET /api/v1/client-portal/deals/:id`
  - `GET /api/v1/client-portal/deals/:id/property` (imóvel DO negócio)
  - `GET /api/v1/client-portal/deals/:id/properties` (sugestões ao cliente)
  - `GET /api/v1/client-portal/deals/:id/recommendations`
  - `GET /api/v1/client-portal/deals/:id/checklist` (+ `:itemId/download` e uploads)
  - `GET /api/v1/client-portal/proposals/mine`
  - `POST /api/v1/client-portal/proposals/:id/respond`
  - `POST /api/v1/client-portal/uploads/presign`
  - `POST /api/v1/client-portal/appointments/request`
  - `POST /api/v1/client-portal/maintenance/request`
  - `POST /api/v1/client-portal/tickets`
- Deals:
  - `POST/GET /api/v1/deals`
  - `GET/PATCH/DELETE /api/v1/deals/:id`
  - Propostas, signers, parties, checklist e auditoria como sub-recursos de deal
- Contracts:
  - `POST/GET /api/v1/contracts`
  - `GET/PATCH/DELETE /api/v1/contracts/:id`
  - Ações: `send-for-signature`, `terminate`, `readjust`, `amend`
- Templates:
  - CRUD em `/api/v1/contract-templates` e `/api/v1/checklist-templates`
- Documents:
  - CRUD, upload/download, versionamento e listagem por entidade em `/api/v1/documents`

## Projeção de imóvel no portal do cliente

Fonte: `internal/domain/portal/property_summary.go`, `internal/repositories/cross_postgres.go` (`propertySummaryColumns`, `propertySummaryQuery`).

- Uma única projeção (`PropertySummary`) alimenta busca, recomendações e o imóvel do negócio: id, título, bairro/cidade/UF, preço pela finalidade (`price` + `listing_type`), quartos, suítes, banheiros, vagas, área total/útil, comodidades, descrição e galeria (`images[]` com legenda). O detalhe do portal não lê mais nomes de campo da ops (`media_images`, `thumbnail_url`, `sale_price`) — era isso que rendia placeholder e "Preço sob consulta".
- **Privacidade como valor pretendido, não como CSS**: `portal.MaskPropertyPrivacy` zera `Address` (rua, número, CEP) nas leituras de VITRINE — `Search` (sugestões) e `ListRecommendations`. O cliente navega por bairro/cidade/UF; o endereço completo só sai no endpoint do imóvel do próprio negócio (`GetMyDealProperty`), onde ele já é parte e a visita está sendo combinada. A máscara fica no backend para a lista de sugestões não carregar o endereço até o navegador.
- Cobertura: `internal/domain/portal/property_summary_test.go` (máscara zera só o endereço) e `toka/e2e/portal-deal-tracking.spec.ts` (galeria/contador/características/comodidades/descrição/preço e o endereço mascarado na vitrine).

## Eventos (RabbitMQ)

- Publica (principais):
  - `deal.created`
  - `deal.status_changed`
  - `proposal.sent`
  - `proposal.responded`
  - `proposal.followup_due` — régua de follow-up de proposta (F6): publicada pelo
    motor com event_id estável por grupo (proposta + estágio)
  - `signer.invited`, `signer.signed`
  - `signature.requested` (aciona assinatura)
  - Eventos de portal: `ticket.*`, `client.ticket.created`, `client.appointment.requested`, `checklist.solicited`
- Consome (principais):
  - Eventos de assinatura (`signature.*`) vindos de `toca-signature`
  - `lead.updated` e `property.updated` para manter agregados coerentes
  - Eventos operacionais (inspeção/manutenção) para refletir na esteira
  - Eventos financeiros de cobrança/escalonamento quando aplicável (ex.: `collection.escalated`)

## Engines de fundo

- `ProposalFollowUpEngine` (`internal/services/proposal_followup_engine.go`):
  varre negócios com proposta aguardando resposta (SENT/COUNTER_OFFER) e publica
  `proposal.followup_due` — cadência 3/7/7 dias (até 3 estágios, idade máxima 30
  dias), configurável por env (`PROPOSAL_FOLLOWUP_*`). Idempotência por grupo
  (proposta + estágio): o mark no agregado acontece depois do publish e o dedup
  do consumidor colapsa republicações. Leitura cross-tenant pelo pool de serviço.
- Reconciliação de signatários e reajuste anual de locação (loops existentes).

## Chamadas para outros serviços

- Assíncrono (padrão):
  - Publica eventos consumidos por `toca-comm`, `toca-financial`, `toca-calendar`, `toca-ops`, `toca-signature`.
- Síncrono (quando necessário):
  - Pode chamar serviços donos via HTTP interno (clients tipados em `pkg/clients`) para leituras/validações.

## Integrações externas

- Assinatura é delegada ao `toca-signature` (Clicksign + webhooks).

## Dados

- Postgres (schema `deal`), com isolamento por `tenant_id` (RLS quando usando Supabase Postgres).

## Código chave

- Rotas: `internal/api/v1/routes/routes.go`
- Contratos de eventos: `pkg/events/contracts/deal_events.go`, `pkg/events/contracts/signature_events.go`, `pkg/events/contracts/proposal_events.go`
