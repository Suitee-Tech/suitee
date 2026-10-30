# toca-ai (IA)

## Papel

- Chat, conversas e sugestões de IA.
- Integra com LLM (DeepSeek) e com o `toca-mcp` para tool-calls/automação.

## HTTP (via gateway)

- Prefixos: `/api/v1/ai/*`
- Porta: `8092`

## Rotas principais (API)

Fonte: `toca-ai/internal/handlers/router.go`.

- `GET /health`
- `GET /metrics`
- IA:
  - `GET /api/v1/ai/tools`
  - `POST /api/v1/ai/chat`
  - `GET /api/v1/ai/suggestions`
  - `GET /api/v1/ai/conversations`
  - `GET /api/v1/ai/conversations/:id`
  - `DELETE /api/v1/ai/conversations/:id`

## Eventos (RabbitMQ)

- Não há publishers/consumers diretos no serviço.

## Integrações externas

- DeepSeek:
  - Client HTTP para `/chat/completions` (modo normal e streaming).
  - Env: `DEEPSEEK_API_KEY`, `DEEPSEEK_BASE_URL`, `DEEPSEEK_MODEL`.
  - Modelo padrão: `deepseek-v4-flash` em modo não pensante.

## Dados

- Postgres (schema `ai`), com isolamento por `tenant_id` (RLS quando usando Supabase Postgres).

## Chamadas para outros serviços

- Integra com `toca-mcp` (tool-calls) e pode chamar APIs internas via gateway quando aplicável.

## Código chave

- Router: `internal/handlers/router.go`
- App wiring: `internal/app/run.go`
- DeepSeek client: `internal/services/deepseek_client.go`
