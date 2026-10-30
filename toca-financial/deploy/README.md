# toca-financial deploy overrides

Suba a stack usando o compose da raiz como base e este arquivo como override.

Exemplo (na raiz do repositório):

```bash
docker compose --env-file .env -f docker-compose.yml -f toca-financial/deploy/docker-compose.yml up -d --build toca-financial
```

Health check:

```bash
curl -sS http://localhost:8083/health
```

Métricas:

```bash
curl -sS http://localhost:8083/metrics
```
