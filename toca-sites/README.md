# toca-sites

Serviço Go (`:8094`) da vitrine pública multi-tenant. A documentação de
arquitetura, contrato HTTP, eventos, schema e operação está em
[`docs/architecture/services/toca-sites/README.md`](../docs/architecture/services/toca-sites/README.md).

```bash
go test ./...
go run ./cmd/server
```

Porta padrão: `8094`. Configuração mínima para subir: `DATABASE_URI`,
`SUPABASE_URL`, `INTERNAL_API_KEY`. `PAYLOAD_API_URL` e `RABBITMQ_URI` habilitam
respectivamente a publicação no CMS e o sync por eventos.
