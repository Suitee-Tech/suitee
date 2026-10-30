# Architecture Decision Records (ADRs)

Cada ADR documenta uma decisão arquitetural significativa: contexto que
forçou a escolha, alternativas consideradas, decisão tomada, e
consequências (positivas e negativas) aceitas em troca.

Formato: [MADR](https://adr.github.io/madr/) leve. Cada ADR é um arquivo
numerado sequencialmente. Aceitas mudam de status mas não de número.

## Status possíveis

- **Proposed** — em discussão; ainda não implementada.
- **Accepted** — implementada e em produção.
- **Deprecated** — substituída por outra ADR (referencia o número).
- **Superseded by ADR-XXXX** — vez fez sentido, hoje uma outra decisão prevalece.

## Quando escrever uma ADR

- Trade-off técnico não trivial (ex: jsonb_set vs tabela normalizada).
- Restrição ou invariante que terceiros precisam respeitar.
- Decisão cuja motivação se perde rapidamente (ex: "por que 1 minuto?").

## Índice

- [001 — Storage de RouletteConfig como JSONB blob](0001-roulette-config-jsonb.md)
- [002 — Optimistic locking via CAS para pointer de roleta](0002-roulette-pointer-cas.md)
- [003 — SLA worker in-process via ticker vs cron externo](0003-sla-worker-in-process-ticker.md)
- [004 — Default de 3 escalations antes de marcar LOST](0004-escalation-cap-default.md)
- [005 — Modularização do frontend e estratégia de estado](0005-frontend-modularization-state-management.md)
- [006 — Site público: Payload como projeção, Go como orquestração](0006-sites-payload-projection.md)
