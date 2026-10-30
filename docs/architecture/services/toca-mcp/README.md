# toca-mcp (MCP Server)

## Papel

- Servidor MCP para conectar agentes/IA a ferramentas e chamadas internas com governança.
- Exposição por SSE para sessões e endpoint de mensagens.

## HTTP

- Porta: `8090`
- Endpoints principais:
  - `GET /health`
  - `GET /mcp/sse`
  - `POST /mcp/messages`

## Eventos (RabbitMQ)

- Não há publishers/consumers diretos no serviço.

## Chamadas para outros serviços

- Pode chamar o gateway e serviços internos dependendo das ferramentas habilitadas no MCP.

## Código chave

- Servidor e handlers: `toca-mcp/main.go`
