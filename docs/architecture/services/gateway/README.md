# gateway (API Gateway)

## Papel

- Ponto único de entrada HTTP para o frontend e integrações externas.
- Reverse proxy por prefixo (`/api/v1/*`) para os serviços `toca-*`.

## Rotas (no gateway)

Fonte: `gateway/internal/handlers/*.go`.

- Auth: `/api/v1/auth`, `/api/v1/companies`, `/api/v1/settings`, `/api/v1/lgpd`, `/api/v1/audit-trail` → `toca-auth:8081`
- Core: `/api/v1/properties`, `/api/v1/captacao`, `/api/v1/partnerships`, `/api/v1/company-settings`, `/api/v1/me/permissions` → `toca-core:8082`
- Financial: `/api/v1/charges`, `/api/v1/invoices`, `/api/v1/rentals`, `/api/v1/payments`, `/api/v1/webhooks`, etc. → `toca-financial:8083`
- CRM: `/api/v1/leads`, `/api/v1/brokers`, `/api/v1/customers` → `toca-crm:8084`
- Deal: `/api/v1/deals`, `/api/v1/contracts`, `/api/v1/contract-templates`, `/api/v1/documents`, `/api/v1/portal` → `toca-deal:8085`
- Comm: `/api/v1/messages`, `/api/v1/channels`, `/api/v1/notifications`, `/api/v1/ws` → `toca-comm:8086`
- Ops: `/api/v1/maintenance`, `/api/v1/tickets`, `/api/v1/keys`, `/api/v1/inspections` → `toca-ops:8087`
- Calendar: `/api/v1/appointments` → `toca-calendar:8088`
- Portal: `/api/v1/portals`, `/api/v1/integrations`, `/api/v1/portal-stats` → `toca-portal:8089`
- AI: `/api/v1/ai/*` → `toca-ai:8092`
- Signature: `/api/v1/signature` e `/webhooks/clicksign` → `toca-signature:8093`

## Regras especiais

- `/api/v1/client-portal/*path` é roteado condicionalmente:
  - se `path` começar com `/rentals`, `/transfers`, `/charges`, `/informes` ou `/owner-statements` → `toca-financial:8083`
  - caso contrário → `toca-deal:8085`

## Chamadas internas

- O gateway chama os serviços por DNS de container (`http://toca-*:PORTA`) na rede Docker.

## Código chave

- Configuração de destinos/ports: `gateway/internal/config/config.go`
- Registro de rotas: `gateway/cmd/gateway/main.go`
- Handlers por serviço: `gateway/internal/handlers/*.go`
