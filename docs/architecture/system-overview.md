# Documentação de Arquitetura da Suitee Platform

**Estado verificado em 18/07/2026.** Fontes autoritativas: `docker-compose.yml`, `docker-compose.prod.yml`, rotas dos serviços, contratos em `pkg/events/contracts` e diagnóstico consolidado.

Esta documentação descreve a arquitetura atualizada da plataforma **Suitee**, um SaaS B2B voltado para a gestão imobiliária end-to-end (CRM, Vendas, Locação, Financeiro e Portal do Cliente).

## 1. Visão Geral do Sistema
A plataforma adota um modelo de arquitetura em **Microsserviços**, permitindo escalabilidade horizontal, isolamento de domínios e facilidade de manutenção. A aplicação cliente é uma **Single Page Application (SPA)** hospedada separadamente, que se comunica com os microsserviços exclusivamente através de um **API Gateway**.

---

## 2. Stack Tecnológico

### Frontend (Aplicação `toka`)
- **Framework:** React 18+ (via Vite)
- **Linguagem:** TypeScript
- **Estilização:** Tailwind CSS (v4) + Variáveis CSS (Whitelabel/Dark Mode)
- **Componentes de UI:** Shadcn UI, Radix UI, Lucide Icons, Recharts
- **Gerenciamento de Estado & Requisições:** Context API, Axios, React Router

### Backend (Microsserviços)
- **Linguagem:** Go (Golang) 1.25+
- **Framework Web:** Gin Web Framework
- **Banco de Dados:** Postgres com JSONB + RLS (quando usando Supabase Postgres)
- **Mensageria/Eventos:** RabbitMQ (Comunicação assíncrona entre serviços)
- **Cache & Sessão:** Redis
- **Autenticação Gerenciada:** Supabase Auth (JWT/JWKS + Admin)
- **Storage (Arquivos):** Supabase Storage (opcional) ou MinIO (dev/local)
- **Formatos:** JSON (HTTP) e JSONB (Postgres)

### Infraestrutura & Deploy
- **Orquestração:** Docker & Docker Compose
- **Ambiente Principal:** Hostinger VPS (Alpine Linux 3.22.1 no snapshot de 18/07/2026)
- **Validação automatizada:** workflows do GitHub Actions versionados para testes, coverage, E2E, RLS e documentação; execuções recentes terminam em `startup_failure` antes dos jobs, com causa ainda não confirmada
- **Deploy operacional:** pipeline direto por SSH/rsync, build local na VPS Hostinger, smoke e promoção/rollback por SHA

---

## 3. Mapa de Microsserviços

O ecossistema backend é fragmentado em serviços especialistas e um Gateway, divididos por contexto de negócio (Domain-Driven Design).

👉 Referência detalhada de papéis e relacionamentos: **[Microsserviços (Papéis e Relacionamentos)](./microservices.md)**

| Serviço | Porta Interna | Responsabilidade Principal |
|---------|---------------|----------------------------|
| **Gateway** | `:8080` | Proxy Reverso, Rate Limiting, Roteamento, Validação de CORS e Centralização de Logs. |
| **toca-auth** | `:8081` | Autenticação (JWT, Refresh Tokens), Gestão de Usuários, Tenants (Whitelabel), Configurações e RBAC. |
| **toca-core** | `:8082` | Catálogo de Imóveis (Estoque), Integrações de portais, Empresas parceiras. |
| **toca-financial** | `:8083` | Gestão financeira, faturamento, repasses, comissões, boletos, PIX e histórico de transações. |
| **toca-crm** | `:8084` | Gestão de Leads, Funil de Vendas (Kanban), Roleta de Distribuição, Histórico de interações. |
| **toca-deal** | `:8085` | Orquestrador (Saga) de Propostas/Contratos e eventos de downstream (financeiro, comunicação, agenda, assinatura). |
| **toca-comm** | `:8086` | Comunicações omnicanal, Mensagens internas, Notificações, Disparos de E-mail (Resend) e SMS. |
| **toca-ops** | `:8087` | Operações de manutenção, Tickets de reparo, Laudos de Vistoria (Entrada/Saída). |
| **toca-calendar**| `:8088` | Agendamento de visitas, Compromissos, Gestão de Agenda e Integração com calendários externos. |
| **toca-portal** | `:8089` | Backend específico para a visão do "Portal do Cliente" (Inquilinos/Proprietários). |
| **toca-mcp** | `:8090` | Model Context Protocol (Integrações com IA, automações e agentes). |
| **toca-ai** | `:8092` | Serviço de IA (LLM, contextos, automações) conectado ao MCP e ao Gateway. |
| **toca-signature** | `:8093` | Assinatura digital (integração Clicksign) e geração de PDF (Gotenberg). |
| **toca-ocr** | `:5001` (somente rede interna) | Extração de PDF/imagem via FastAPI + Docling, consumida sob demanda pelo `toca-ai`; incluída nos Composes e no build/deploy de produção. |

---

## 4. Padrões de Comunicação e Fluxo de Dados

### 4.1. Comunicação Síncrona (HTTP/gRPC)
- Todas as chamadas do cliente web batem no **Gateway** (`/api/v1/*`).
- O Gateway repassa a requisição ao serviço correspondente baseado na rota (Ex: `/api/v1/leads` -> `toca-crm`).
- **Autenticação:** O Gateway (ou os middlewares dos serviços) validam o JWT no header `Authorization`. Tokens de renovação trafegam em cookies `HttpOnly`.
- Quando necessário, um serviço pode chamar outro de forma síncrona via HTTP interno (clientes tipados em `pkg/clients`), mas o padrão preferido para integrações entre domínios é eventos.

### 4.2. Comunicação Assíncrona (Eventos)
- Quando um serviço precisa notificar outro sem travar a requisição, ele publica um evento no **RabbitMQ**.
  - *Exemplo:* quando um "Contrato é Assinado" no `toca-deal` (orquestrador), ele publica `contract.signed`. O `toca-financial` consome para gerar cobranças e o `toca-comm` consome para notificar o cliente.

O barramento compartilhado implementa outbox transacional, retry/backoff, deduplicação por consumidor e DLQ com rotas administrativas. O catálogo detalhado está em [Integração por eventos](./integracao-eventos.md).

### 4.3. Upload de Arquivos (Storage)
- O upload de imagens, avatares e PDFs vai do Frontend (Multipart/Form-Data) pelo Gateway até o serviço de destino (Ex: `toca-auth` para avatares, `toca-deal` para documentos).
- O serviço valida a integridade/tipo do arquivo e o envia ao provider configurado (`STORAGE_PROVIDER=supabase|s3|local`), persistindo apenas a referência no Postgres (JSONB).

---

## 5. Arquitetura de Banco de Dados

- **Fonte de verdade:** Postgres. Cada domínio usa schemas (ex.: `auth`, `core`, `crm`, `deal`, `financial`) com tabelas padronizadas em JSONB.
- **Multitenancy (Multi-Inquilino):** o isolamento é aplicado via RLS (Row Level Security) quando usando Supabase Postgres, e reforçado pelo middleware (claim `app_metadata.tenant_id`) que propaga `tenant_id` no contexto da requisição.

---

## 6. Fluxo de Deploy (VPS Hostinger)

O caminho operacional não depende do GitHub Actions. Na máquina do operador,
`make deploy-prod` executa o pipeline direto documentado em
`deployment/hostinger.md`:

1. valida a worktree e resolve a SHA imutável da release;
2. sincroniza o repositório por `rsync`, preservando segredos e estado de deploy na VPS;
3. constrói sequencialmente as 15 imagens próprias, incluindo frontend, MCP, IA e OCR;
4. aplica a stack com a mesma tag de SHA e aguarda readiness/healthchecks;
5. executa smoke público em API e frontend;
6. promove a SHA para `current` ou tenta rollback para a última release saudável.

Esse pipeline publica a aplicação, mas **não aplica migrations**. Toda mudança
de schema/RLS precisa ser aplicada e validada separadamente no Supabase antes
de ser tratada como concluída.

O Nginx/Traefik atua como ingress na frente da VPS, expondo
`app.example.com` para o frontend e `api.example.com` para o Gateway. O
workflow `.github/workflows/deploy-hostinger.yml` permanece como alternativa
legada, mas não é o caminho ativo enquanto o Actions não inicia jobs.

---

## 7. Auditoria de Consistência e Confiabilidade

Para o snapshot histórico que originou o hardening de projeções,
idempotência, eventos e RLS, acompanhado de um adendo com o estado atual,
consulte:

👉 **Auditoria de Consistência e Confiabilidade**

## 8. Estado operacional conhecido

- Compose local válido com 24 serviços/infra, incluindo Postgres opcional, Redis, RabbitMQ, MinIO, imgproxy, Gotenberg, Jaeger, Prometheus, Loki, Promtail e Grafana.
- Compose de produção usa Traefik e inclui MCP, IA e OCR; o pipeline direto constrói 15 imagens próprias por SHA na VPS.
- Os workflows de PR contêm gates de Go, frontend, RLS, documentação, Prometheus, smoke, coverage e Playwright determinístico. As execuções recentes terminam em `startup_failure` antes dos jobs, com causa ainda não confirmada; portanto validação local não deve ser chamada de "CI verde".
- O build Vite de produção passa o budget de 500 kB gzip por chunk; typecheck, fronteiras de features e 497 testes Vitest também passam.
- O checker do repositório cobre 61 migrations, 62 tabelas multi-tenant, duas exceções service-only e uma tabela removida. A inspeção anterior do banco de produção encontrou 67 tabelas com `tenant_id`, todas com RLS; outras oito tabelas sem `tenant_id`/RLS não concediam acesso a `anon` ou `authenticated`.

---

## 9. Diretrizes para Novos Desenvolvimentos

- **Frontend:** Ao criar novos componentes visuais, sempre use o `button.tsx` e padrões Shadcn. As cores primárias (`bg-primary`, `text-primary-600`) são processadas em runtime pelo `ThemeContext.tsx` para se adaptar ao Whitelabel do tenant atual. Não injete "cores fixas" (ex: `bg-green-600`) se o elemento for parte do Whitelabel.
- **Backend (Novas Rotas):** 
  1. Declare o endpoint no serviço apropriado (`internal/api/routes/routes.go`).
  2. Implemente o manipulador (`handlers`), passando a lógica complexa para a camada de `service`.
  3. Insira as regras de acesso via Middleware de Permissão (`auth.RequirePermission(permissionRepo, "recurso:acao")`).
  4. Certifique-se de rotear no `gateway/main.go` para que o frontend consiga alcançar o endpoint.
