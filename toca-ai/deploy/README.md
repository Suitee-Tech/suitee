## Executar com override

Na raiz do monorepo:

```bash
docker compose up -d
docker compose -f docker-compose.yml -f toca-ai/deploy/docker-compose.yml up -d toca-ai
```

## Endpoints

```bash
curl -s http://localhost:8092/health
curl -s http://localhost:8092/metrics | head
```

