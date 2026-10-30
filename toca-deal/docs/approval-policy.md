# Política de aprovação por tenant

O gate de negócios de alto valor é configurado no `toca-auth`, em
`toca_auth.tenant_settings.metadata.approval_policy`. O contrato v1 fixa os
status `ANALYSIS` e `CONTRACT` e a permissão `deals:approve`; cada tenant pode
alterar `enabled` e `threshold_brl`.

A tabela mantém RLS habilitada e políticas JWT por `app_metadata.tenant_id`;
as leituras e gravações internas usam exclusivamente `DATABASE_URI_SERVICE`.

Há duas versões no payload:

- `schema_version`: versão do formato JSON, atualmente `1`;
- `revision`: revisão monotônica do tenant, incrementada atomicamente a cada
  atualização.

## APIs

- `GET /api/v1/settings/approval-policy`: consulta autenticada, exige
  `settings:manage_general`;
- `PUT /api/v1/settings/approval-policy`: atualiza `enabled` e
  `threshold_brl`, exige `settings:manage_general`;
- `GET /api/v1/auth/internal/tenants/:tenant_id/approval-policy`: contrato
  interno usado pelo `toca-deal`, protegido por `X-Internal-Key`.

Exemplo de atualização:

```json
{
  "enabled": true,
  "threshold_brl": 750000
}
```

## Cache e falha

O `toca-deal` mantém cache fresco por 5 minutos e aceita o último valor válido
por mais 1 hora durante indisponibilidade do `toca-auth`. A chamada remota tem
timeout de 800 ms. Depois da janela stale, usa explicitamente
`TOCA_DEAL_APPROVAL_THRESHOLD_BRL` (R$ 500 mil por padrão), preservando o
comportamento histórico. Os três tempos são configuráveis por
`APPROVAL_POLICY_CACHE_TTL_SECONDS`, `APPROVAL_POLICY_STALE_TTL_SECONDS` e
`APPROVAL_POLICY_FETCH_TIMEOUT_MS`.

Alterações da política geram `tenant.approval_policy_updated`. Quando alguém
com `deals:approve` atravessa o gate, o `toca-deal` grava
`deal.approval_override` com revisão, threshold e origem da resolução
(`remote`, `cache`, `stale_cache` ou fallback).
