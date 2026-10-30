# Tabelas de identidade sem `tenant_id` — justificativa por tabela (P2-10)

As oito tabelas abaixo são multi-tenant **sem coluna `tenant_id`** — o
escopo vem de `account_id`/`company_id` ou de nenhuma identidade de
usuário (plumbing de eventos). Elas **já têm RLS habilitado** no banco
vivo (verificado em 03/10/2026) e os papéis `anon`/`authenticated` **não
têm grants** sobre elas (único acesso via `toca_app`/`toca_service`).

Estado verificado no beta:

| Tabela | RLS | Policies | Modo |
|---|---|---|---|
| `toca_auth.accounts` | ✅ | 4 | política vinculada ao JWT |
| `toca_auth.credentials` | ✅ | 1 | política vinculada ao JWT |
| `toca_auth.invitations` | ✅ | 1 | política vinculada ao JWT |
| `toca_auth.groups` | ✅ | 1 | política vinculada ao JWT |
| `toca_auth.password_resets` | ✅ | 0 | **deny-by-default** |
| `toca_auth.user_preferences` | ✅ | 1 | política vinculada ao JWT |
| `core.partnerships` | ✅ | 1 | política vinculada ao JWT |
| `events.processed_events` | ✅ | 0 | **deny-by-default** |

## Justificativa por tabela

| Tabela | Por que sem `tenant_id` | Quem acessa | Por que a política é esta |
|---|---|---|---|
| `toca_auth.accounts` | Conta é anterior/independente do vínculo de empresa — um usuário pode pertencer a várias | `toca-auth` pelo pool de aplicação (fluxos de login/me) e pelo pool de serviço | `id = sub do JWT`: o usuário lê a própria conta; o serviço usa BYPASSRLS |
| `toca_auth.credentials` | Credencial de login pertence à CONTA, não à empresa | `toca-auth` (login/registro) pelo pool de aplicação e de serviço | `account_id` do dono; login sem JWT usa o pool de serviço |
| `toca_auth.user_preferences` | Preferências são da conta (tema/idioma seguem o usuário entre empresas) | `toca-auth` (GET/PUT /me/preferences) pelo pool de aplicação | `account_id` do dono |
| `toca_auth.invitations` | Convite liga e-mail → empresa; o alvo ainda não é membro (não tem tenant) | `toca-auth` (criar/aceitar convite) — aceite via token, listagem via JWT de admin | `company_id = tenant do JWT` para listar; aceite usa o pool de serviço (token próprio) |
| `toca_auth.groups` | Grupo de permissões pertence à empresa, mas o escopo vem de `company_id` | `toca-auth` (catálogo de grupos) pelo pool de aplicação | `company_id = tenant do JWT` |
| `core.partnerships` | Parceria liga DOIS tenants (`requester`/`target`) — não há um único `tenant_id` | `toca-core` (rotas de parceria) pelo pool de aplicação e de serviço | `requester_id = tenant do JWT OR target_id = tenant do JWT`: cada lado enxerga a parceria |
| `toca_auth.password_resets` | Token de reset é público por natureza (e-mail) e não pertence a tenant nenhum | Ninguém pelo pool de aplicação: o fluxo de token local foi removido no P1/A14 | **deny-by-default**: sem política, o pool de aplicação não lê nada; só o pool de serviço |
| `events.processed_events` | Dedup de entrega de eventos do RabbitMQ — identidade é `(consumer, message_id)` | Só o relay (`pkg/events`) pelo **pool de serviço** | **deny-by-default**: nenhum papel de aplicação deve ler/escrever; BYPASSRLS para o serviço |

## Não fazer

- **Não criar `tenant_id`** nessas tabelas: quebraria o modelo de conta
  (pré-empresa) e de parceria (dois lados).
- **Não habilitar RLS "por cima" sem política** onde o pool de aplicação
  lê (accounts/credentials/…) — o login e o /me parariam de ver dados.
- O checker `check-migrations-rls` modela exatamente esta tabela
  (`identityTables` em `scripts/check-migrations-rls/main.go`): RLS
  obrigatório para as oito e política obrigatória onde `policyRequired` —
  qualquer tabela nova nessa classe entra ali.

## Vigilância

- Grants: revalidar periodicamente que `anon`/`authenticated` continuam
  sem privilégios (query de `role_table_grants` abaixo).
- `password_resets` e `processed_events` devem permanecer **zero
  policies** — uma policy nova nessas duas é sinal de que o fluxo mudou
  de dono e a decisão precisa ser revisada.

```sql
SELECT table_schema, table_name, grantee, privilege_type
FROM information_schema.role_table_grants
WHERE table_schema IN ('toca_auth','core','events')
  AND grantee IN ('anon','authenticated');
-- Esperado: 0 linhas.
```
