# toca-core deploy overrides

Suba a stack usando o compose da raiz como base e este arquivo como override.

Exemplo (na raiz do repositório):

```bash
docker compose --env-file .env -f docker-compose.yml -f toca-core/deploy/docker-compose.yml up -d --build toca-core
```

Health check:

```bash
curl -sS http://localhost:8082/health
curl -sS http://localhost:8082/api/v1/health
```

Métricas:

```bash
curl -sS http://localhost:8082/metrics
```
