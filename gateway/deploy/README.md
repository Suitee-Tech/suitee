# Gateway deploy overrides

Suba a stack usando o compose da raiz como base e este arquivo como override.

Exemplo (na raiz do repositório):

```bash
docker compose --env-file .env -f docker-compose.yml -f gateway/deploy/docker-compose.yml up -d --build gateway
```

Health check:

```bash
curl -sS http://localhost:8080/health
```

Métricas:

```bash
curl -sS http://localhost:8080/metrics
```
