# ADR 002 — Optimistic locking via CAS para pointer da Roleta

- **Status**: Accepted
- **Data**: 2026-05-17 (escrita retroativa; decisão original ~2025-Q4)
- **Owners**: toca-crm
- **Relacionados**: ADR-001 (config como JSONB)

## Contexto

O pointer round-robin da Roleta (`StageLastIndex[stage]`, ou `LastIndex`
para o fallback global) precisa avançar atomicamente: dois leads
chegando simultaneamente não podem ambos pegar o mesmo broker.

Como a config inteira vive em um JSONB blob por tenant (ADR-001), não
podemos usar `SELECT ... FOR UPDATE` na linha sem serializar todo o
read/write da config (inclusive admins editando `ActiveBrokers`, que é
um cenário paralelo independente).

Cenário típico de contenção: pico de leads vindos de campanhas, ~10
leads/s/tenant. Cada lead aciona um `AssignByRoulette` que precisa ler
config, escolher broker, avançar pointer.

## Decisão

Implementar **compare-and-swap (CAS) otimista** no pointer via SQL:

```sql
UPDATE crm.roulette_configs
SET metadata = jsonb_set(metadata, '{StageLastIndex,STAGE}', to_jsonb($1::int), true),
    updated_at = NOW()
WHERE tenant_id = $2
  AND COALESCE((metadata#>>'{StageLastIndex,STAGE}')::int, -1) = $3
  AND deleted_at IS NULL
```

(Quando `stage` é vazio, usa `{LastIndex}` em vez do path aninhado.)

`SetIndexCAS` retorna `true` quando `RowsAffected > 0`. O caller
(`AssignByRoulette` em `lead_roulette_service.go`) entra em um loop de
até **10 tentativas**, recarregando a config a cada iteração:

```go
for attempts := 0; attempts < 10; attempts++ {
    if attempts > 0 {
        cfg, _ = s.rouletteRepo.GetConfig(ctx, l.TenantID)
    }
    chosenID, idx, m, _ := s.pickBroker(ctx, ..., cfg, l, ...)
    expected := currentIndex(cfg, stage)
    ok, _ := s.rouletteRepo.SetIndexCAS(ctx, l.TenantID, stage, expected, idx)
    if ok { brokerID = chosenID; break }
}
```

## Alternativas consideradas

1. **`SELECT ... FOR UPDATE` na linha de config**.
   - **Pros**: semântica linearizable trivial.
   - **Cons**: serializa toda escrita na config, inclusive admins
     editando `ActiveBrokers`. Em pico, lock waits matam o p99 de
     `POST /leads`. Inaceitável.
2. **Advisory lock Postgres** (`pg_advisory_xact_lock(tenant_hash)`).
   - **Pros**: granularidade por tenant sem tocar a row.
   - **Cons**: lock implícito (não aparece no `\dl`), dificulta
     debugging; ainda serializa N requests por tenant.
3. **Mover pointer para tabela própria normalizada** (`roulette_pointers`).
   - **Pros**: lock fino, schema explícito.
   - **Cons**: viola decisão ADR-001 sem ganho prático mensurável; CAS
     já cobre o caso.
4. **Redis/etcd para pointer**.
   - **Pros**: latência sub-millisecond.
   - **Cons**: adiciona dependência operacional. Hoje a Roleta opera
     bem dentro do envelope Postgres existente.

## Consequências

### Aceitas

- Sem locks bloqueantes — admins editando config não serializam
  atribuições. Escritas concorrentes ao mesmo pointer são detectadas e
  re-tentadas.
- Cobertura testada pelo `TestRoulette_ConcurrentAssignmentsPreserveOrder`
  (Wave P2.10): 50 goroutines, 5 brokers, perfeitamente balanceadas
  dentro da tolerância de ±3.
- `RowsAffected = 0` permite distinguir conflito CAS de erro real.

### Pagas

- **Sob alta contenção**, o limite de 10 tentativas pode ser exaurido —
  registrado como `lead.unassigned` com `reason=cas_exhausted` no audit
  e em `roulette_assignments_total{outcome="cas_exhausted"}` (Wave P1.5).
  Hoje o limiar prático de saturação é ~20 leads/s/tenant — acima disso
  considerar backoff exponencial ou queue intermediária.
- **Sem fairness garantida**. Em caso de retries, o request que entra
  primeiro pode perder pra um que entrou depois. Para Roleta isso é OK
  (a fairness é a longo prazo, garantida pelo round-robin); para outros
  domínios pode não ser aceitável.
- **`jsonb_set` no path aninhado falha silenciosamente** se a chave não
  existir e `create_missing=false`. Mitigado por sempre usar
  `create_missing=true` e pré-popular `StageLastIndex[stage]=-1` no
  `UpdateConfig` (linha 168 de `lead_roulette_service.go`).

## Quando reconsiderar

- Métrica `roulette_assignments_total{outcome="cas_exhausted"}` atinge
  taxa > 1% sustentada por uma semana.
- Throughput cresce além de 50 leads/s/tenant.
- Surge requisito de fairness estrita (primeiro a chegar é primeiro a
  ser servido) — neste caso uma fila persistente substitui CAS.
