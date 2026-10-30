# toca-portal deploy overrides

Suba a stack usando o compose da raiz como base e este arquivo como override.

Exemplo (na raiz do repositório):

```bash
docker compose --env-file .env -f docker-compose.yml -f toca-portal/deploy/docker-compose.yml up -d --build toca-portal
```

Health check:

```bash
curl -sS http://localhost:8089/health
```

Métricas:

```bash
curl -sS http://localhost:8089/metrics
```
