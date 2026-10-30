# toca-financial (Financeiro)

## Papel

- Rentals, charges, invoices, transfers, commissions, collections e delinquency.
- Consolida o lado financeiro do ciclo de vida de contratos e locações.

## HTTP (via gateway)

- Prefixos: `/api/v1/charges`, `/api/v1/invoices`, `/api/v1/transactions`, `/api/v1/rentals`, `/api/v1/transfers`, `/api/v1/collections`, `/api/v1/delinquency`, `/api/v1/payments`, `/api/v1/webhooks`, `/api/v1/reports`, `/api/v1/financial-settings`, `/api/v1/proofs`, `/api/v1/owner-statements`, `/api/v1/client-portal/*`
- Porta: `8083`

## Rotas principais (API)

Fonte: `toca-financial/internal/handlers/router.go`.

- Interno (internal key):
  - `GET /api/v1/financial/internal/deals/:id/charges`
- Client portal (JWT role `cliente`):
  - Rentals: `GET /api/v1/client-portal/rentals/mine`, `GET /api/v1/client-portal/rentals/:id`
  - Transfers: `GET /api/v1/client-portal/transfers/mine`, `GET /api/v1/client-portal/transfers/:id`, `GET /api/v1/client-portal/transfers/:id/receipt`
  - Charges: `GET /api/v1/client-portal/charges/mine`, `GET /api/v1/client-portal/charges/:id`, `GET /api/v1/client-portal/charges/:id/receipt`
  - Informes: `GET /api/v1/client-portal/informes/mine`
  - Owner statements: `GET /api/v1/client-portal/owner-statements/mine`
  - Owner statements (admin): `GET /api/v1/owner-statements/:ownerLeadId` (json/pdf)
    — o demonstrativo inclui a **intermediação** descontada do aluguel
    (`intermediation_amount`, `intermediation_commission_funded_amount`,
    `intermediation_outstanding_amount`, este lido do plano congelado do
    contrato), para o repasse fechar com o líquido;
  - Prévia financeira: `GET /financial-preview/deals/:id` aceita
    `start_date`/`end_date` opcionais do plano pré-contrato (400 para datas
    inválidas ou término antes do início);
  - `GET/PUT /financial-settings` inclui `default_lease_term_months` (padrão de
    30 meses da locação, 1 a 120).
- Operação (JWT operador):
  - CRUD e ações para commissions/charges/transfers/invoices/transactions
  - Cobrança recorrente: `rental-charge-configs`
  - Collections/delinquency
  - Fiscal e reports

## Eventos (RabbitMQ)

- Consome:
  - Eventos do ciclo de vida de deal/contrato para criar/agregar financeiro.
  - Eventos operacionais (maintenance/inspection) para ajustes/deduções quando aplicável.
- Publica:
  - `invoice.paid`
  - `rental.created`
  - `rental.charge_overdue`
  - `charge.overdue`
  - `collection.escalated`

## Integrações externas

- Pagamentos:
  - Rotas `/api/v1/payments` e `/api/v1/webhooks` existem no gateway e no swagger, mas a integração com um provedor específico pode estar parcial/legada.
- Storage (comprovantes):
  - Upload/assinatura de URLs via provider configurado (`STORAGE_PROVIDER`), com suporte a Supabase Storage (presigned URLs).

## Dados

- Postgres (schema `financial`), com isolamento por `tenant_id` (RLS quando usando Supabase Postgres).

## Chamadas para outros serviços

- Consome eventos de `toca-deal` (contratos) e publica eventos consumidos por `toca-comm` e por fluxos de esteira.
- Pode expor dados financeiros agregados para o client portal (roteamento especial no gateway).

## Código chave

- Router: `internal/handlers/router.go`
- Contratos de eventos: `pkg/events/contracts/invoice_events.go`, `pkg/events/contracts/rental_events.go`, `pkg/events/contracts/financial_events.go`
