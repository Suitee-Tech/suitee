# Serviços (Microsserviços)

Índice de documentação por serviço. Cada pasta contém o papel do serviço, suas rotas HTTP (via gateway), chamadas internas relevantes, eventos RabbitMQ (publica/consome) e integrações externas.

## Índice

- [gateway](./gateway/README.md)
- [toca-auth](./toca-auth/README.md)
- [toca-core](./toca-core/README.md)
- [toca-crm](./toca-crm/README.md)
- [toca-deal](./toca-deal/README.md)
- [toca-financial](./toca-financial/README.md)
- [toca-comm](./toca-comm/README.md)
- [toca-ops](./toca-ops/README.md)
- [toca-calendar](./toca-calendar/README.md)
- [toca-portal](./toca-portal/README.md)
- [toca-signature](./toca-signature/README.md)
- [toca-sites](./toca-sites/README.md)
- `toca-web` — Payload 3 + Next.js do site público; documentação em [`toca-web/README.md`](../../../toca-web/README.md).
- [toca-ai](./toca-ai/README.md)
- [toca-mcp](./toca-mcp/README.md)
- `toca-ocr` — serviço interno FastAPI/Docling em `:5001`, consumido pelo `toca-ai`; documentação em [`toca-ocr/README.md`](../../../toca-ocr/README.md). Integra os Composes local/produção, healthcheck e build direto da Hostinger.
