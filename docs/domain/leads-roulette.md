# Roleta de Leads (CRM) — configuração e fluxo

## Visão geral (ponta a ponta)

1. Frontend cria lead via `POST /api/v1/leads` (sem `assigned_broker_id`).
2. O toca-crm publica o evento `lead.created`.
3. O toca-crm consome `lead.created` e, se o lead ainda estiver sem corretor, aplica **round-robin** (roleta).
4. Ao atribuir, o toca-crm publica `lead.assigned` (para notificações/tempo real).

## Endpoints

- `GET /api/v1/leads/roulette` — retorna a configuração atual
- `PUT /api/v1/leads/roulette` — atualiza a configuração
- `POST /api/v1/leads/roulette/simulate` — simula a distribuição para N leads (sem criar leads)
- `GET /api/v1/leads/roulette/history` — auditoria: lista decisões de distribuição

## Campos principais da configuração

- `enabled` (bool): liga/desliga a roleta
- `active_brokers` ([]string): pool padrão (fallback)
- `stage_broker_pools` (map[string][]string): pool por etapa da esteira (por **status do lead**)
  - Ex.: `"NEW"`, `"QUALIFIED"`, `"PROPOSAL"`, etc.
  - Se um status não estiver presente (ou estiver vazio), cai no `active_brokers`.
- `autoCapLimit` (bool): respeita capacidade do corretor (ver `max_active_leads` no Broker)

## Exemplo (cURL) — habilitar roleta e configurar pools por etapa

```bash
curl -X PUT "http://localhost:8080/api/v1/leads/roulette" \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "enabled": true,
    "active_brokers": ["broker-1","broker-2","broker-3"],
    "stage_broker_pools": {
      "NEW": ["broker-1","broker-2","broker-3"],
      "QUALIFIED": ["broker-2","broker-3"]
    }
  }'
```

## Exemplo (cURL) — simular distribuição com N leads

Dry-run (não altera o ponteiro):

```bash
curl -X POST "http://localhost:8080/api/v1/leads/roulette/simulate" \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{ "count": 10, "stage": "NEW", "persist": false }'
```

## Auditoria (histórico)

```bash
curl "http://localhost:8080/api/v1/leads/roulette/history?limit=50&offset=0" \
  -H "Authorization: Bearer $TOKEN"
```

Persistente (avança o ponteiro como se leads reais tivessem chegado):

```bash
curl -X POST "http://localhost:8080/api/v1/leads/roulette/simulate" \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{ "count": 10, "stage": "NEW", "persist": true }'
```
