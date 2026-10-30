# Migrations Supabase neste repositório

As mudanças versionadas de schema, RLS e Storage ficam em:

- [`supabase/migrations/`](./migrations/)

Esse diretório é a fonte da verdade do Git. O ledger remoto do Supabase é a
fonte da verdade sobre versões aplicadas. Alterações manuais no banco podem
causar drift, por isso o estado remoto deve ser confrontado com o repositório;
não presuma que todo SQL existente em produção veio desta pasta.

Regras para novas migrations: [`migrations/GUIDELINES.md`](./migrations/GUIDELINES.md).
Status produtivo detalhado: [`docs/supabase/status.md`](../docs/supabase/status.md).

## Snapshot verificado

No snapshot remoto verificado em 18 de julho de 2026:

- 57 versões constam no ledger remoto; a árvore local contém uma 58ª migration,
  `20260721190000_deployment_migration_checksums.sql`, ainda pendente do rollout
  controlado de P0-06;
- o checker aprovou 60 tabelas multi-tenant, 2 allowlisted e 1 removida;
- produção possui 75 tabelas de aplicação, das quais 67 têm `tenant_id` e todas
  as 67 estão com RLS habilitado;
- 8 tabelas sem `tenant_id` permanecem sem RLS, mas sem grants de schema ou
  tabela para `anon` e `authenticated`;
- os únicos advisors de segurança restantes são 2 achados `INFO` para
  `events.outbox_events` e `events.dlq_events`, protegidas por RLS default-deny,
  sem policies de usuário e sem grants para os papéis da Data API;
- as funções `crm.prevent_lead_delete_with_active_deal()`,
  `exports.set_jobs_updated_at()` e `audit.ensure_event_partitions(integer)`
  estão com `search_path = ''`.

O advisor não apresenta atualmente o achado de proteção contra senhas vazadas,
mas isso não confirma diretamente a configuração Have I Been Pwned do Auth
hospedado. A confirmação no Dashboard (`Auth > Providers > Email`) continua
pendente; `supabase/config.toml` descreve o ambiente local, não produção.

## Pré-requisitos

- Supabase CLI instalada: <https://supabase.com/docs/guides/cli>
- Sessão autenticada com `supabase login`, quando usar projeto vinculado
- Projeto Supabase já criado
- `DATABASE_URI_SERVICE` obtida de fonte segura para operações remotas por URI

Nunca registre a URI, a senha do banco ou chaves Supabase em commits, logs,
issues ou exemplos reais.

## Vincular o projeto uma única vez

A partir da raiz do repositório:

```bash
supabase link --project-ref <project-ref>
```

O link local não substitui a revisão do destino antes de um push. Confirme que o
projeto/URI corresponde ao ambiente pretendido.

## Validar e aplicar em ambiente remoto

Primeiro execute o checker versionado:

```bash
make check-rls
```

O fluxo normal de beta/produção é integrado ao deploy direto. Ele exige árvore
limpa, tag igual ao `HEAD` e aprovação explícita da SHA quando existem versões
pendentes. O commit é exportado com `git archive` e publicado na VPS como
`.deploy/sources/<sha>` write-once; o worktree não é a fonte do deploy:

```bash
make deploy-beta
# Se houver pendencias, revise o dry-run e repita com aprovacao:
MIGRATION_APPROVAL_SHA="$(git rev-parse HEAD)" make deploy-beta

make deploy-prod
# Se houver pendencias, revise o dry-run e repita com aprovacao:
MIGRATION_APPROVAL_SHA="$(git rev-parse HEAD)" make deploy-prod
```

O pipeline adquire locks separados para upload (`pipeline.lock`) e execução do
ambiente (`deploy.lock`). Ele copia `.env.<ambiente>` uma vez para um temporário
`0600`; Compose, todas as fases e o rollback automático usam esse mesmo dotenv
como dados literais, sem `source` ou avaliação de shell. Em seguida gera o manifesto por versão/SHA-256, compara o
ledger de versões e `ops.deployment_migration_checksums`, e executa dry-run antes
do build. Se houver pendências sem aprovação, persiste a evidência e para.

Só depois que todas as imagens terminam de construir o orquestrador adquire o
advisory lock PostgreSQL compartilhado, grava `apply-intent.sha256`, aplica as
versões aprovadas e registra checksums write-once. Se houver crash no meio de um
lote, a repetição aceita somente o prefixo ordenado das pendências aprovadas,
registra seus checksums e continua pelas versões restantes; se o ledger já está
completo, reconcilia sem replay. O lock permanece até readiness, smoke, verify e
promoção.
As evidências ficam em `.deploy/<ambiente>/migrations/<sha>/`, incluindo os
ledgers antes/depois e `apply.sha256`/`verified.sha256`; promoção exige que os
dois digests coincidam e pertençam à mesma tag pendente.

Para diagnóstico somente, reproduza o dry-run contra a mesma URI usada pelo
pipeline:

```bash
supabase db push --db-url "$DATABASE_URI_SERVICE" --dry-run
```

Não execute `db push --yes` manualmente para contornar o gate. O no-op acontece
porque as versões já estão no ledger, não porque seja seguro reexecutar
arbitrariamente todo o SQL. Migrations devem continuar idempotentes quando
possível, mas o ledger é a barreira operacional contra replay.

Após o push, consulte migrations e advisors com Supabase CLI/MCP e valide o
comportamento afetado. Não aplique automaticamente sugestões genéricas de RLS
às oito tabelas sem `tenant_id`: os grants e as políticas precisam ser definidos
de acordo com o modelo de acesso de cada uma.

## Deploy integrado de aplicação e schema

O pipeline direto da Hostinger integra as migrations à release da aplicação,
mas preserva fronteiras explícitas: preflight antes do build, apply depois do
build e verify antes da promoção. `20260721190000` cria
`ops.deployment_migration_checksums`, semeia o baseline histórico e concede ao
papel de serviço somente `SELECT`/`INSERT`; o gate registra o checksum da própria
bootstrap e de cada versão nova. Alteração de conteúdo já aplicado bloqueia o
deploy.

Rollback continua restrito às imagens, nunca remove entradas dos ledgers nem
executa SQL reverso. O trap só reverte se `pending-sha` ainda for a SHA desta
execução, evitando afetar outra release quando a falha ocorreu antes da própria
pendência. Portanto toda migration deve ser forward-only e backward-compatible
com pelo menos a release anterior.

A validação local possui 25 cenários unitários do gate — inclusive retomada de
lote parcial, rotação de senha e metadata de checksum —, 5 de integração do
orquestrador e 6 do wrapper SSH (11 de integração no total):

```bash
make test-migration-gate
make test-deployment-pipeline
make test-deploy-wrapper
```

A integração está validada localmente, mas só será considerada operacionalmente
concluída depois de evidência do fluxo completo em beta e produção.

Detalhes: [`docs/supabase/migration.md`](../docs/supabase/migration.md) e
[`docs/deployment/hostinger.md`](../docs/deployment/hostinger.md).

## Colisão histórica `20260522120000`

O ledger de produção registra `20260522120000` e `20260522120001` com o nome
`partner_royalties`. No Git, `20260522120000` permanece como a migration antiga
do webhook Abacate e `20260522120001` como `partner_royalties`.

Não renomeie, apague, repare ou reescreva essas versões. A migration forward-only
`20260718150751_reconcile_20260522_history_collision.sql` está aplicada e valida
que as duas linhagens convergiram para o mesmo schema final.

## Banco local

Para reproduzir o histórico completo, prefira a stack Supabase CLI:

```bash
make supabase-local-start
make supabase-local-status
```

### Roles de runtime no banco local

`supabase/roles.sql` cria `toca_app` e `toca_service` (sem login) antes das
migrations em `supabase start`. Em beta/produção essas roles continuam sendo
criadas à mão, com senha (o gate de deploy não usa `--include-roles`); no banco
local elas existem só para que migrations que fazem `REVOKE` nelas não falhem
com `role "toca_app" does not exist`. Se o seu banco local foi criado antes
desse arquivo, recrie-o (`supabase stop --no-backup && supabase start`) ou crie
as duas roles manualmente antes de aplicar as migrations pendentes.

O perfil `local-db` de `docker-compose.yml` é um bootstrap de compatibilidade e
monta somente:

- `scripts/postgres/init.sql`
- `20260421120000_finalize_jsonb_rls.sql`
- `20260425120000_create_ai_tables.sql`
- `20260426120000_create_ai_tool_invocations.sql`

Esse perfil não representa as 57 versões do snapshot de produção nem a 58ª
migration local pendente e não deve ser usado como prova de paridade de schema.
