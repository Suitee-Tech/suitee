# ADR 004 — Default de 3 escalations antes de marcar LOST

- **Status**: Accepted
- **Data**: 2026-05-17
- **Owners**: toca-crm

## Contexto

`triggerEscalation` em `lead_sla_service.go` itera por `cfg.Escalation`
em loop: a cada violação de SLA escala para o próximo step. Pré-Wave-1.2
o loop usava `step := cfg.Escalation[l.EscalationLevel % len(cfg.Escalation)]`
sem cap — um lead poderia escalar indefinidamente, ciclando pelos
mesmos passos.

Operacionalmente, escalar um lead 10x sem resolver virou ruído (notif
para gerente, supervisor, urgent badge) sem progresso. Uma operadora
relatou que leads abandonados ficavam gerando ping diário até alguém
manualmente marcar LOST.

## Decisão

Adicionar `MaxEscalations int` no `RouletteConfig` (opcional, default
via constante de domínio):

```go
const DefaultMaxEscalations = 3
```

Em `triggerEscalation`:

```go
maxEscalations := cfg.MaxEscalations
if maxEscalations <= 0 {
    maxEscalations = lead.DefaultMaxEscalations
}
if l.EscalationLevel >= maxEscalations {
    l.Status = lead.LeadStatusLost
    // audit + metric "lost_capped"
    return
}
```

Após `MaxEscalations` violações, o lead vira LOST automaticamente — sai
da fila do SLA worker (que só varre `PENDING_ACCEPTANCE`) e do plantão
operacional.

## Alternativas consideradas

1. **Sem cap** (comportamento original).
   - **Cons**: descrito no contexto. Leads zumbis pingando para sempre.
2. **Cap fixo via `const`, não configurável**.
   - **Pros**: simplifica.
   - **Cons**: tenant com SLA agressivo pode querer 5 ou 7 retries
     antes de declarar perda; tenant com SLA permissivo pode querer 1.
3. **Marcar como `URGENT` em vez de `LOST`**.
   - **Cons**: outro status, outras consequências; não resolve o
     ping-loop, só adiciona ruído de cor.
4. **Atribuir a um broker "fallback" (supervisor)** em vez de marcar LOST.
   - **Pros**: lead não morre.
   - **Cons**: orquestração com responsabilidade ambígua; mistura
     "estratégia da última chance" com a Roleta. Pode ser implementado
     futuramente como um passo extra em `cfg.Escalation` (action =
     `assign_to_supervisor`) sem mudar o cap.

## Consequências

### Aceitas

- Loop infinito eliminado.
- Default sensato (3) cobre o caso de "tentei 3 vezes em ~15min" sem
  precisar de configuração explícita.
- Audit registra ambos os events: `lead.escalated` (com `level`,
  `previous_level`, `step_id`) e `lead.escalation_capped` (com `level`
  final e `max`).
- Métrica `roulette_sla_actions_total{action="lost_capped"}` permite
  alertar quando o cap está sendo atingido com frequência (sinal de
  configuração de SLA mal calibrada).

### Pagas

- **Default mágico**. Admin que nunca configurou `MaxEscalations` pode
  se surpreender com leads virando LOST automaticamente. Endereçado por
  alerta no contract test (`TestTriggerEscalation_UsesDefaultWhenCapZero`)
  e por documentação em `docs/domain/leads-roulette.md` (futuro).
- **Cap se aplica a todos os tenants igualmente** até alguém configurar.
  Migração para defaults por-tenant pode acontecer via seed na
  criação do tenant — fora do escopo desta ADR.

## Quando reconsiderar

- Métrica `roulette_sla_actions_total{action="lost_capped"}` acima de
  5% das atribuições por tenant — sinal que o default está perdendo
  leads que deveriam ser perseguidos mais agressivamente. Subir o
  default para 5 ou pedir ao tenant que configure.
- Surgir necessidade de "branching" na escalation (ex: depois de N
  steps, tentar outro broker; depois de M, ir pro supervisor). Isso
  expande além de um simples cap — provavelmente nasce ADR-005.
