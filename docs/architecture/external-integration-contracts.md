# Contratos das integrações externas

Estado verificado em **17/07/2026**. Os testes desta página são sandboxes
locais e determinísticos: usam `httptest.Server`, tokens fictícios e nenhuma
rede ou credencial real.

## Matriz atual

| Integração | Implementação | Contrato automatizado | Timeout | Circuit breaker |
|---|---|---|---|---|
| Clicksign | `toca-signature/internal/clicksign/client.go` | JSON:API, autenticação, criação, consulta, notificações e erros em `client_test.go` | 10 s no `pkg/clients.HTTPClient` | Abre após 5 falhas consecutivas; recuperação após 30 s |
| Meta WhatsApp | `pkg/clients/whatsapp_business_client.go` | texto, template, autenticação, payloads, erros Meta, rate limit, timeout e circuito | `WHATSAPP_TIMEOUT_SECONDS`, padrão 10 s | Abre após 5 falhas consecutivas; recuperação após 30 s |
| Zenvia SMS | `pkg/clients/sms_zenvia_client.go` | autenticação, payload, normalização, erros, rate limit, timeout e circuito | `SMS_ZENVIA_TIMEOUT_SECONDS`, padrão 10 s | Abre após 5 falhas consecutivas; recuperação após 30 s |
| Resend | `toca-comm/internal/service/email_service.go` | e-mail HTML, template, autenticação, erro estruturado, timeout e circuito | 10 s explícitos | `pkg/clients.HTTPClient`; uma tentativa por POST para evitar entrega duplicada |
| DeepSeek | `toca-ai/internal/services/deepseek_client.go` | chat, tools, JSON mode, SSE, usage e timeout | `AI_REQUEST_TIMEOUT_SECONDS`, padrão 120 s | `pkg/clients.HTTPClient`; abre após 5 falhas consecutivas |
| NFS-e | `toca-financial/internal/services/fiscal_service.go` | Somente persistência local; não existe contrato externo para testar | Não aplicável | Não aplicável |
| Pagamentos | `toca-financial/internal/services/*` | Baixa, PIX/TED e comprovantes são manuais; não existe gateway externo ativo | Não aplicável | Não aplicável |

O transporte compartilhado tem contratos próprios em
`pkg/clients/http_client_contract_test.go`, incluindo o limite de tempo e a
rejeição local de chamadas quando o circuito está aberto.

## Como validar

```bash
(cd pkg && GOWORK=off go test ./clients)
(cd toca-signature && GOWORK=off go test ./internal/clicksign)
(cd toca-comm && GOWORK=off go test ./internal/service)
(cd toca-ai && GOWORK=off go test ./internal/services)
```

## Trabalho restante para concluir P1-05

1. Escolher e implementar um provedor de NFS-e. A documentação funcional
   recomenda comprar uma integração (por exemplo, NFE.io/eNotas/Migrate) ou
   implementar adapters municipais ABRASF; hoje nenhum deles existe no código.
2. Decidir se pagamentos continuarão exclusivamente manuais ou selecionar um
   gateway. Só depois dessa decisão é possível definir idempotência, webhook,
   conciliação e contrato de sandbox reais.
3. Quando cada provedor for escolhido, adicionar seu fake/fixture de contrato,
   timeout, circuit breaker e casos de webhook antes de habilitá-lo em release.

P1-05 permanece aberto enquanto NFS-e e pagamentos não tiverem provedores
reais. Marcar contratos locais desses fluxos como “sandbox externo” esconderia
um gap operacional e de compliance.
