# Microsserviços — Papéis e Relacionamentos

**Revisado em 18/07/2026.** O mapa representa implementação observada; planos futuros ficam em `architecture/plans/` e no TODO consolidado.

Este documento descreve cada microsserviço do monorepo Suitee, seu papel na arquitetura e como ele se relaciona com os demais via HTTP (Gateway e clients internos) e eventos (RabbitMQ).

👉 Documentação detalhada por serviço: [services/README.md](./services/README.md)

## Convenções

- **HTTP público**: Frontend → `toca-gateway` → serviço (rota `/api/v1/*`).
- **HTTP interno**: serviço → serviço via clients tipados em `pkg/clients` (quando é necessário resposta síncrona).
- **Eventos**: serviço publica/consome via RabbitMQ (`pkg/events/contracts`).
- **Confiabilidade de eventos**: outbox transacional, deduplicação por consumidor, retry/backoff e DLQ ficam em `pkg/events`.
- **Dados**: Postgres (frequentemente Supabase Postgres) com schemas por domínio e RLS (tenant-scoped) quando aplicável.
- **Auth**: tokens JWT validados via JWKS do Supabase Auth (`SUPABASE_JWKS_URL`).

## Serviço interno de OCR

`toca-ocr` não recebe rota pública no Gateway. Ele expõe `:5001` apenas na rede Docker e é chamado pelo `toca-ai` via `OCR_SERVICE_URL`. A imagem usa FastAPI/Docling e cache de modelos. O serviço integra os Composes local e de produção, os healthchecks e o pipeline Hostinger; ele é uma das 15 imagens próprias construídas em cada release direta.

## Mapa rápido (porta + rotas no gateway)

Fonte: mapeamento de handlers do gateway em `gateway/internal/handlers/*.go`.

| Serviço | Porta | Prefixos no Gateway (exemplos) |
|---|---:|---|
| `gateway` | 8080 | `/api/v1/*` (reverse proxy) |
| `toca-auth` | 8081 | `/api/v1/auth`, `/api/v1/companies`, `/api/v1/settings` |
| `toca-core` | 8082 | `/api/v1/properties`, `/api/v1/captacao`, `/api/v1/partnerships` |
| `toca-financial` | 8083 | `/api/v1/charges`, `/api/v1/invoices`, `/api/v1/rentals`, `/api/v1/payments` |
| `toca-crm` | 8084 | `/api/v1/leads`, `/api/v1/brokers`, `/api/v1/customers` |
| `toca-deal` | 8085 | `/api/v1/deals`, `/api/v1/contracts`, `/api/v1/portal` |
| `toca-comm` | 8086 | `/api/v1/messages`, `/api/v1/channels`, `/api/v1/notifications`, `/api/v1/ws` |
| `toca-ops` | 8087 | `/api/v1/maintenance`, `/api/v1/tickets`, `/api/v1/inspections` |
| `toca-calendar` | 8088 | `/api/v1/appointments` |
| `toca-portal` | 8089 | `/api/v1/portals`, `/api/v1/integrations` |
| `toca-mcp` | 8090 | exposto via Gateway conforme necessidade de IA |
| `toca-ai` | 8092 | `/api/v1/ai/chat`, `/api/v1/ai/conversations` |
| `toca-signature` | 8093 | `/api/v1/signature` |
| `toca-sites` | 8094 | `/api/v1/sites`, `/api/v1/sites/public/*` |
| `toca-web` | 3000 | fora do gateway: `{marca}.imoveis.suitee.com.br` (Traefik → Next) |

## Papéis e relacionamentos (por serviço)

### `gateway` (API Gateway)

- **Papel**: ponto único de entrada HTTP; aplica CORS, roteamento por prefixo e políticas básicas.
- **Relacionamentos**:
  - **HTTP**: encaminha para todos os serviços `toca-*`.
  - **Auth**: valida JWT (ou delega aos serviços) e usa JWKS do Supabase Auth.
  - **Client Portal**: possui regra de roteamento especial para `/api/v1/client-portal/*` (divide entre Deal e Financial).

### `toca-auth` (IAM / Identidade)

- **Papel**: identidade e controle de acesso (usuários, contas/empresas, RBAC, convites, fluxos de login/OTP).
- **Relacionamentos**:
  - **Supabase Auth**: usa API REST (admin + flows de token/OTP) e injeta `tenant_id` em `app_metadata`.
  - **HTTP**: consumido por frontend via gateway para login e administração.
  - **Eventos**: publica eventos de ciclo de vida de usuário/convite/OTP consumidos pelo `toca-comm` para notificações.

### `toca-core` (Core / Imóveis)

- **Papel**: catálogo de imóveis e dados centrais (propriedades, captação, parcerias).
- **Relacionamentos**:
  - **Eventos**: publica eventos de propriedade/captação; consome `deal.status_changed` para atualizar projeções relacionadas a imóvel.
  - **HTTP**: consumido pelo frontend (operador) via gateway.

### `toca-crm` (CRM / Leads)

- **Papel**: leads, brokers, roleta, qualificação e distribuição.
- **Relacionamentos**:
  - **Eventos (publica)**: ciclo de vida de lead (`lead.created`, `lead.assigned`, `lead.status_changed`, `lead.converted`) e broker (`broker.updated`).
  - **Eventos (consome)**: integrações com conversão/assinatura e eventos de propriedade para automações (ex.: roleta e conversão para cliente).
  - **HTTP**: consumido pelo frontend (operador) via gateway.

### `toca-deal` (Esteira / Contratos) — Orchestrator/Saga

- **Papel**: orquestrador dos fluxos de propostas/contratos e side-effects entre domínios (financeiro, comunicação, agenda, assinatura).
- **Relacionamentos**:
  - **HTTP**: expõe esteira (`deals`, `contracts`, `documents`, `portal`).
  - **Eventos (publica)**: `deal.created`, `deal.status_changed`, `proposal.sent`, `proposal.responded`, eventos de ticket/appointment do portal, `signature.requested` e eventos relacionados a signers.
  - **Eventos (consome)**: eventos de assinatura vindos de `toca-signature`, além de mudanças em lead/property e eventos operacionais (inspeção/manutenção) para consolidar estado do deal.
  - **Dependências**:
    - **Síncronas (HTTP interno)**: pode chamar serviços donos quando precisa resposta imediata.
    - **Assíncronas (eventos)**: padrão preferido para acionar `toca-comm`, `toca-financial`, `toca-calendar`, `toca-ops`, `toca-signature`.

### `toca-financial` (Financeiro)

- **Papel**: rentals, charges, invoices, pagamentos, repasses, comissões e inadimplência.
- **Relacionamentos**:
  - **Eventos (consome)**: eventos do contrato/deal para criar `Rental`, cobrar e orquestrar financeiro; eventos de manutenção/inspeção para ajustes/deduções.
  - **Eventos (publica)**: `invoice.paid`, `rental.created`, `rental.charge_overdue`, `charge.overdue`, `collection.escalated`.
  - **Integração de pagamento**: rotas e variáveis de ambiente existem; implementação de gateway específico pode estar parcial/legada (ver `toca-financial/docs/swagger.*` e `.env.example`).
  - **Storage**: upload/gestão de comprovantes via provider (`STORAGE_PROVIDER`), com suporte a Supabase Storage.

### `toca-comm` (Comunicação)

- **Papel**: mensagens, canais, notificações e integrações de e-mail.
- **Relacionamentos**:
  - **Eventos (consome)**: grande parte dos eventos de domínio para disparar notificações e atualizar canais.
  - **Eventos (publica)**: eventos internos de coordenação (ex.: escalonamentos) quando aplicável.
  - **Resend**: envio de e-mails transacionais (templates, convites, OTP, notificações).
  - **WebSocket**: endpoint `/api/v1/ws` via gateway para atualizações em tempo real.

### `toca-ops` (Operações)

- **Papel**: manutenção, tickets/chamados, chaves e inspeções/vistorias.
- **Relacionamentos**:
  - **Eventos (publica)**: eventos de maintenance/inspection/ticket.
  - **Eventos (consome)**: `deal.contract.signed` para acionar automações operacionais (ex.: agenda de vistoria).
  - **HTTP**: consumido pelo frontend operador via gateway.

### `toca-calendar` (Agenda)

- **Papel**: agendamentos e gestão de compromissos.
- **Relacionamentos**:
  - **Eventos (publica)**: `appointment.updated`, `appointment.cancelled`.
  - **Eventos (consome)**: pode operar indiretamente via `toca-deal` (fluxos de portal) conforme arquitetura do produto.

### `toca-portal` (Integrações / Portal)

- **Papel**: integrações e funções específicas da experiência portal (e/ou integrações externas, conforme evolução do módulo).
- **Relacionamentos**:
  - **Eventos**: publica eventos específicos do portal consumidos por `toca-comm` quando aplicável.
  - **HTTP**: endpoints de integrações e estatísticas via gateway.

### `toca-signature` (Assinatura)

- **Papel**: geração de documentos para assinatura, integração com provedor de assinatura e consolidação de status de envelopes/signers.
- **Relacionamentos**:
  - **Clicksign**: client HTTP + webhook público (via gateway) para atualizar estado.
  - **Eventos (consome)**: `signature.requested` disparado pelo `toca-deal`.
  - **Eventos (publica)**: eventos de evolução da assinatura (signer viewed/signed, envelope finished/refused/expired).
  - **PDF**: geração/transformação de HTML → PDF via Gotenberg (infra interna).

### `toca-sites` (Site de imóveis / vitrine)

- **Papel**: registro da vitrine por tenant, projeção do imóvel no Payload,
  ingestão de pageviews/leads do site público e métricas do painel.
- **Relacionamentos**:
  - **HTTP (interno)**: lê `GET /api/v1/core/internal/properties/:id` no
    `toca-core`; cria lead em `POST /api/v1/leads` do `toca-crm`; escreve
    listings/páginas e revalida o ISR no `toca-web`.
  - **Eventos (consome)**: `property.updated`, `property.captured` (filas
    `toca-sites.*`).
  - **Eventos (publica)**: `site.listing.published`,
    `site.listing.unpublished`, `site.lead.received` (outbox).
  - **Banco**: schema `sites.*` com RLS por `tenant_id`.
  - **Host público**: resolve tenant só pelo Host (`{marca}.imoveis...`).
  - **Docs**: [toca-sites/README.md](./services/toca-sites/README.md).

### `toca-web` (Payload CMS + Next SSR)

- **Papel**: renderizar o site público multi-tenant com SEO/ISR. Payload é
  **projeção** (ADR 0006), nunca source of truth de imóvel/lead.
- **Relacionamentos**:
  - **Go**: recebe listings/páginas via API interna (`X-Internal-Key` +
    `X-Tenant-Id`) e `POST /api/revalidate` do `toca-sites`.
  - **Postgres**: schema `payload` (mesmo banco da plataforma).
  - **Browser**: beacon de view e formulário de lead chamam o `toca-sites`
    (rotas públicas), não o Payload.
  - **Acesso**: admin do Payload restrito a super-admin/rede interna; a
    imobiliária opera em `/site` no Toka.

### `toca-ai` (IA)

- **Papel**: conversas, sugestões e automações de IA.
- **Relacionamentos**:
  - **DeepSeek**: client HTTP para completions e streaming.
  - **MCP**: integra com `toca-mcp` para tool-calls e automações.
  - **HTTP**: endpoints `/api/v1/ai/*` via gateway.

### `toca-mcp` (MCP Server)

- **Papel**: servidor MCP para conectar a IA com ferramentas internas/externas, com governança e autorização.
- **Relacionamentos**:
  - **IA**: recebe tool-calls de `toca-ai`.
  - **Gateway**: pode chamar APIs internas, dependendo do toolset habilitado.

## Referências

- Diagramas: [arch.mermaid](./diagrams/arch.mermaid) e [arch-simpl.mermaid](./diagrams/arch-simpl.mermaid)
- Mapa de eventos: [integracao-eventos.md](./integracao-eventos.md)
