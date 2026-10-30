# ADR 003 — SLA worker in-process via ticker (vs cron externo)

- **Status**: Accepted
- **Data**: 2026-05-17 (escrita retroativa; decisão original ~2025-Q4)
- **Owners**: toca-crm

## Contexto

O SLA worker varre leads em status `PENDING_ACCEPTANCE` cuja
`AssignedAt` ultrapassou o `TimeLimitMin` de algum tier, e dispara
ações configuradas: redirect, escalate, broker_paused.

A varredura precisa rodar periodicamente (1 minuto é granularidade
suficiente: SLA tiers menores que 1min seriam ruído operacional).

## Decisão

Rodar o worker **in-process**, no mesmo binário do toca-crm, via
`time.NewTicker(1 * time.Minute)`. Goroutine única iniciada no
`SetupRouter` e vinculada a um `context.Context` cancelado no graceful
shutdown:

```go
func (s *LeadSLAService) RunSLAWorker(ctx context.Context) {
    ticker := time.NewTicker(SLAWorkerInterval)
    defer ticker.Stop()
    for {
        select {
        case <-ctx.Done():
            return
        case <-ticker.C:
            if err := s.CheckSLAViolations(ctx); err != nil && !errors.Is(err, context.Canceled) {
                logger.Error("sla.check_violations failed", "error", err.Error())
            }
        }
    }
}
```

`SLAWorkerInterval` é `var` (não `const`) para testes encurtarem o tick
(ver `lead_sla_service_test.go:TestRunSLAWorker_StopsOnContextCancel`).

## Alternativas consideradas

1. **Cron externo (k8s CronJob, AWS EventBridge, etc) chamando `POST /internal/sla/run`**.
   - **Pros**: scheduler robusto, fácil de pausar via console, métricas
     do scheduler já existem.
   - **Cons**: adiciona endpoint interno (superfície de ataque + perm),
     introduz dependência externa para uma operação que já vive no
     binário. Falha do cron é silenciosa do ponto de vista da app.
2. **Worker dedicado em outro processo/repo** (`toca-crm-worker`).
   - **Pros**: escalar workers separado de API, isolamento de blast
     radius.
   - **Cons**: duplica config e secrets, complica deploy (2 binários),
     overkill para uma varredura periódica de baixa intensidade.
3. **Postgres `pg_cron` extension chamando função SQL**.
   - **Pros**: zero dependência de aplicação.
   - **Cons**: lógica de escalation/redirect tem branches complexos que
     ficariam ilegíveis em PL/pgSQL; pg_cron exige extensão habilitada
     pelo provider (Supabase suporta, mas adiciona vendor lock).

## Consequências

### Aceitas

- Zero infra extra. Sobe junto com o serviço, morre junto.
- Contexto compartilhado: logger, metrics, eventBus, repos — tudo já
  injetado no `LeadSLAService`. Não precisa duplicar wiring.
- Testes podem ajustar `SLAWorkerInterval` e provar tanto que o worker
  tica quanto que ele encerra ao receber ctx.Done.
- Wave P1.5 instrumentou via `metrics.IncRouletteSLAAction(tenantID, action)`
  — operações têm visibilidade sem precisar de painel do scheduler.

### Pagas

- **Em replicas N>1, o worker roda N vezes em paralelo** — cada réplica
  vai processar os mesmos leads. As ações são idempotentes via WHERE
  optimistic (`ReassignBrokerIfCurrent` checa o broker atual), então o
  pior caso é duplo trabalho, não duplo efeito. Mas se a operação
  evoluir para algo não-idempotente (ex: notificação push), precisamos
  de lock distribuído ou eleição de leader.
- **Restart do serviço pausa o SLA worker** por uns segundos. Como o
  ticker é de 1min, no pior caso a primeira varredura pós-restart
  acontece 1min depois do start — aceitável dado o SLA mínimo de 5min
  na config típica.
- **Erro silencioso pré-Wave-P0**. Antes do fix, o `_ = ...` engolia
  qualquer erro de banco/transient. Hoje logamos via logger estruturado
  e o erro aparece no agregador.

## Quando reconsiderar

- Migração para Kubernetes com N réplicas onde a duplicação de trabalho
  vira problema (ex: notificação a usuário final que dispara N vezes).
  Solução: leader election via row lock em uma tabela `worker_leadership`
  ou via cron externo + endpoint interno.
- SLA evoluir para granularidade sub-minuto (improvável — tempo de
  resposta humano dificilmente justifica < 1min).
- Worker passar a fazer trabalho longo (> 30s) — bloqueia o ticker da
  próxima varredura, criando backlog. Hoje cada `CheckSLAViolations`
  termina em ~100ms para um tenant típico.
