# ADR 001 — Storage de RouletteConfig como JSONB blob

- **Status**: Accepted
- **Data**: 2026-05-17 (escrita retroativa; decisão original ~2025-Q4)
- **Owners**: toca-crm

## Contexto

A configuração da Roleta de Leads (`lead.RouletteConfig`) tem ~15 campos
top-level, vários deles slices ou maps aninhados:

- `ActiveBrokers []string`
- `StageBrokerPools map[string][]string` (variável: tantos stages quanto o
  tenant configurar)
- `StageLastIndex map[string]int` (paralelo ao anterior)
- `SLATiers []SLATier` (cada um com `Actions struct{...}`)
- `Escalation []EscalationStep`
- `NotifChannels []NotifChannel`
- `QualFields []QualField` (form builder dinâmico do tenant)
- `QualRouting *QualRoutingConfig`
- `GlobalBusinessHours *broker.BusinessHours`

O schema **muda com frequência** conforme features avançam (recentes: cap
de escalations, qual_routing, stage broker pools). Cada tenant tem 1
linha — não é uma tabela com milhões de registros.

## Decisão

Persistir o config completo como **um único JSONB** na tabela
`crm.roulette_configs(tenant_id PK, metadata JSONB, updated_at, deleted_at)`.

Mutar via `jsonb_set` quando precisamos de atomic update field-level (o
caso do CAS pointer — ver ADR-002), ou substituir o blob inteiro quando
o admin salva config via PUT `/leads/roulette`.

## Alternativas consideradas

1. **Tabela normalizada por entidade** (roulette_configs, sla_tiers,
   escalation_steps, qual_fields, etc — relacionadas por tenant_id).
   - **Pros**: queries SQL típicas (JOIN, WHERE), constraints declarativos
     (FK, CHECK, UNIQUE), migrations claras por feature.
   - **Cons**: 5-7 tabelas + FKs por feature de roleta; cada mudança no
     domínio (adicionar `MaxEscalations`, `AutoPauseCooldownMin`)
     requer migration coordenada; volume baixíssimo (1 linha/tenant)
     não justifica overhead operacional.
2. **JSON files versionados em disco/objeto storage**.
   - **Pros**: imutável, fácil de copiar/restaurar.
   - **Cons**: incompatível com semântica transacional do CAS pointer
     (precisamos de UPDATE atômico no mesmo registro).

## Consequências

### Aceitas

- Adicionar um campo é só: `cfg.NewField = ...` em Go + `MapToDTO`. Sem
  migration de banco.
- Backup/restore é trivial (`pg_dump` da tabela inteira).
- Query externa (BI, debugging) precisa de `metadata->'key'` em vez de
  SQL natural. Documentado em `docs/domain/leads-roulette.md`.

### Pagas

- **Sem constraints declarativos**. Não há `CHECK` que valide enum de
  `DistMethod`. Validação acontece em Go no service. Risco: bug no
  service grava lixo no JSONB e quebra leitura subsequente. Mitigação:
  testes unit do helper de validação + serialização Go forçada.
- **Sem versionamento**. Não há tabela `roulette_config_versions` para
  rollback; o audit trail (`crm.roulette_audit`) registra os diffs e
  permite reconstruir snapshot histórico, mas isso é replay manual.
  Endereçado parcialmente pela Wave P3 (versionamento dedicado) se a
  necessidade aparecer.
- **CAS pointer requer SQL inline** (`jsonb_set` + `WHERE COALESCE((expr)::int, -1) = $expected`),
  documentado em ADR-002. Não é tão claro quanto `UPDATE table SET pointer = $new WHERE pointer = $expected`.
- **Schema implícito**. Novos contribuidores precisam ler `domain/lead/roulette.go`
  para descobrir o shape — `\\d crm.roulette_configs` no psql só mostra `metadata jsonb`.

## Quando reconsiderar

Se a config crescer para multi-megabyte por tenant (≫ 100 stages,
≫ 50 SLA tiers), o read/write do blob inteiro fica caro. O ponto de
inflexão prático observado em outros projetos é ~1MB por linha JSONB
antes da serialização Go dominar a latência de PUT.

Sintomas para acompanhar:
- p99 do PUT `/leads/roulette` acima de 500ms (hoje, com a config típica
  do Imobiliária Exemplo, fica abaixo de 80ms).
- Conflito de write entre admins em produção (já endereçado por WS de
  config_updated na Wave P1.6, mas dois admins escrevendo o blob inteiro
  em paralelo perdem campos do outro).

Se isso acontecer, migrar para tabela normalizada por
entidade-chave-de-conflito (SLATiers, StageBrokerPools) — não tudo de
uma vez.
