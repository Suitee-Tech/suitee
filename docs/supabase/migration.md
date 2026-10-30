# Integração e migrations Supabase

Esta é a referência operacional para Supabase Auth, Postgres e Storage. O
snapshot de produção verificado fica em `status.md`; as regras
para criar migrations ficam em
[`supabase/migrations/GUIDELINES.md`](../../supabase/migrations/GUIDELINES.md).

## Visão geral

- **Auth:** validação de JWT via JWKS do Supabase (RS256/ES256); o middleware
  rejeita tokens HS256.
- **Banco:** Supabase Postgres com JSONB e isolamento multi-tenant por RLS.
- **Storage:** Supabase Storage quando `STORAGE_PROVIDER=supabase`.
- **Schema versionado:** [`supabase/migrations/`](../../supabase/migrations/).

## Variáveis de ambiente

A lista completa e comentada está em [`.env.example`](../../.env.example). As
variáveis principais são:

```env
SUPABASE_URL=https://<project-ref>.supabase.co
SUPABASE_PUBLIC_URL=https://<project-ref>.supabase.co
SUPABASE_JWKS_URL=${SUPABASE_URL}/auth/v1/.well-known/jwks.json
SUPABASE_ANON_KEY=
SUPABASE_SERVICE_ROLE_KEY=

DATABASE_URI=
SUPABASE_DB_URL=
DATABASE_URI_SERVICE=

STORAGE_PROVIDER=supabase
```

Regras importantes:

- `SUPABASE_SERVICE_ROLE_KEY` existe apenas no backend e nunca deve ser exposta
  no bundle do frontend.
- `SUPABASE_URL` é o endpoint usado pelos serviços. `SUPABASE_PUBLIC_URL` é a
  origem entregue ao navegador em imagens e links assinados; em beta/produção
  ela deve ser HTTPS e publicamente resolvível. Em desenvolvimento Docker, use
  `host.docker.internal` apenas em `SUPABASE_URL` e `http://localhost:54321` em
  `SUPABASE_PUBLIC_URL`.
- `DATABASE_URI` é a conexão principal exigida pelo Compose de produção.
- `SUPABASE_DB_URL` é o alias explícito usado pelos serviços integrados ao
  Supabase; em produção ele deve apontar para o mesmo banco tenant-scoped de
  `DATABASE_URI`.
- `DATABASE_URI_SERVICE` é reservada a rotinas internas que precisam do papel
  de serviço/`BYPASSRLS`; em release ela deve ser explícita e distinta da URI
  tenant-scoped.
- Use conexão direta ou Pooler em modo **SESSION**, ambos na porta 5432. Não use
  transaction pooling na porta 6543 para fluxos que dependem de estado de
  sessão como `request.jwt.claims`.

## Estado atual de produção

No snapshot remoto verificado em 18 de julho de 2026:

- 57 versões constavam no ledger remoto; a árvore local agora contém 61
  migrations, incluindo `20260721190000_deployment_migration_checksums.sql`;
  o rollout das versões posteriores ao snapshot em beta/produção ainda faz
  parte de P0-06;
- o checker local aprovou 62 tabelas multi-tenant, com 2 exceções allowlisted e
  1 tabela removida;
- as 67 tabelas físicas que possuem `tenant_id` estão com RLS habilitado;
- as duas funções anteriormente apontadas pelo advisor estão com
  `search_path = ''`;
- a colisão histórica `20260522120000` foi reconciliada por uma migration
  forward-only, sem reescrever o ledger.

Detalhes, exceções e limitações de evidência sobre proteção contra senhas
vazadas estão em `status.md`.

## Aplicar migrations com segurança

Nunca reaplique todos os arquivos com um loop de `psql -f`. Esse replay cego
ignora o ledger e pode agravar ou recriar divergências como a colisão histórica
de `20260522120000`. Também não edite, renomeie ou remova uma migration que já
chegou a qualquer ambiente compartilhado; crie sempre uma migration
forward-only.

Antes de acessar beta ou produção, valide o conjunto versionado:

```bash
make check-rls
```

O caminho normal de release é o deploy integrado. A partir de uma árvore
totalmente limpa, ele exporta o commit com `git archive`, publica o snapshot
remoto write-once `.deploy/sources/<sha>` e usa a SHA completa do `HEAD` como
aprovação explícita:

```bash
make deploy-beta
# Se houver pendencias, revise o dry-run e repita com aprovacao:
MIGRATION_APPROVAL_SHA="$(git rev-parse HEAD)" make deploy-beta

make deploy-prod
# Se houver pendencias, revise o dry-run e repita com aprovacao:
MIGRATION_APPROVAL_SHA="$(git rev-parse HEAD)" make deploy-prod
```

Exercite primeiro em beta, revise a evidência e gere uma nova aprovação para
produção. `MIGRATION_APPROVAL_SHA` não é segredo: ela vincula a decisão do
operador ao código e ao manifesto daquela SHA. A aprovação é capturada da
sessão antes da leitura do arquivo de ambiente; uma chave persistida no dotenv
não concede permissão. Quando o preflight encontra versões pendentes sem
aprovação, ele persiste o dry-run e para antes do build; no novo comando,
`preflight` e `apply` exigem o valor e o recusam se não corresponder à
tag/`HEAD`. Sem pendências, a variável pode ficar vazia.

No início da execução remota, `.env.<ambiente>` é copiado uma única vez para um
temporário `0600`. Esse snapshot é passado ao Compose, a todas as fases e ao
rollback automático; ele é tratado como dotenv literal e nunca executado com
`source`. Rotação concorrente
só vale no deploy seguinte, e `$()`, crases ou outras construções shell não são
avaliadas.

O gate integrado executa três fases:

| Fase | Momento | Garantia |
|---|---|---|
| `preflight` | antes de qualquer build | gera o manifesto por versão/SHA-256, exige histórico forward-only, compara versões e checksums remotos e executa `supabase db push --dry-run` |
| `apply` | somente depois que todas as imagens foram construídas | sob advisory lock, aplica apenas versões pendentes aprovadas e registra checksums write-once |
| `verify` | depois de readiness/smoke e antes de `promote` | sob o mesmo advisory lock, exige igualdade exata de versões/checksums e dry-run vazio; produz a evidência vinculada ao apply |

Antes de passar a URI ao runtime, o gate garante uma imagem local construída
por `supabase-cli.Dockerfile` com Supabase CLI 2.107.0. O container de execução
recebe somente a URI necessária e o projeto Supabase read-only; não executa
`npx` nem instala dependências enquanto o segredo está disponível.

O pipeline chama o gate com o estado isolado por ambiente:

```bash
MIGRATION_STATE_DIR=.deploy/<ambiente>/migrations \
MIGRATION_DIR=supabase/migrations \
bash scripts/deployment/migration-gate.sh \
  <preflight|apply|verify> <env-file> <sha>
```

Essa interface interna é útil para auditoria e diagnóstico, mas não deve ser
usada para contornar `make deploy-*`, os locks ou a aprovação. A fase interna
`hold-lock` é reservada ao orquestrador: ela mantém uma conexão com o advisory
lock PostgreSQL `(1937006964, 20260721)` do apply até a promoção. O
manifesto SHA-256 identifica o conteúdo SQL que acompanhou a release; o ledger
`supabase_migrations.schema_migrations` é confrontado por versão e o ledger
`ops.deployment_migration_checksums` por conteúdo, nome, proveniência e release.
Uma versão aplicada sem arquivo/checksum local, uma alteração de conteúdo ou
uma pendência inesperada interrompe o fluxo.

Em `.deploy/<ambiente>/migrations/<sha>/`, o gate preserva `manifest.tsv`
(release, versão, SHA-256 e nome), `database-target.sha256` (vínculo sem a URI),
`ledger-before.txt`, `checksum-status.txt`, `checksums-before.tsv`, `pending.txt`,
`preflight.sha256`, `ledger-after.txt`, `checksums-after.tsv`, `apply.sha256` e
`verified.sha256`. `database-target.sha256` deriva de usuário, host, porta e
database, excluindo URI e senha para permitir rotação de credencial no mesmo
alvo. Antes do push, `apply` grava `apply-intent.sha256` com o digest do
preflight. Se o processo cair depois de avançar parte do ledger, uma repetição
exige manifesto intacto e aceita somente um prefixo ordenado das pendências
write-once; registra os checksums desse prefixo, reaplica as versões restantes e
só então conclui a evidência. Ledger completo é reconciliado sem replay de SQL.
Qualquer outro drift falha fechado. Enquanto o advisory lock está vivo,
`db-lock.owner` registra seu PID.

`verify` repete as leituras antes e depois do dry-run e só grava
`verified.sha256` com o digest do apply. `release.sh promote` exige que essa
evidência pertença à tag pendente, seja um SHA-256 válido e coincida byte a byte
com o `apply.sha256` irmão.

### Ledger write-once de checksums

`20260721190000_deployment_migration_checksums.sql` cria
`ops.deployment_migration_checksums` com `version` como chave primária e guarda
`sha256`, nome, proveniência, release e horário. `PUBLIC`, `anon` e
`authenticated` não têm acesso; `service_role` recebe somente `SELECT` e
`INSERT`, nunca `UPDATE`/`DELETE`.

No primeiro rollout, a ausência da tabela só é aceita se a migration bootstrap
estiver local e pendente. Ela semeia os checksums históricos revisados; depois do
push, o gate valida o baseline e insere também o checksum da própria versão
`20260721190000`. Nos deploys seguintes, as versões aplicadas e os checksums
precisam ter cobertura exata. Migration aplicada é imutável; correção entra em
nova versão forward-only.

Há três exclusões externas ao `gate.lock` de cada fase: `pipeline.lock` protege
upload/publicação da árvore, `deploy.lock` protege a execução do ambiente e o
advisory lock protege o trecho mutável do banco até a promoção. Assim, beta e
produção também são serializados se apontarem para o mesmo Postgres.

A validação local cobre 25 cenários unitários do gate — inclusive retomada
idempotente de um lote parcialmente aplicado, rotação de senha e metadata de
checksum —, 5 de integração do orquestrador e 6 do wrapper SSH (11 de integração
no total):

```bash
make test-migration-gate
make test-deployment-pipeline
make test-deploy-wrapper
```

Esses testes não fecham P0-06: ainda é necessária evidência do fluxo completo em
beta e, depois, em produção.

Para diagnóstico manual, com `DATABASE_URI_SERVICE` carregada de uma fonte
segura, o mesmo dry-run pode ser reproduzido com:

```bash
supabase db push --db-url "$DATABASE_URI_SERVICE" --dry-run
```

Não faça um `db push --yes` manual para “destravar” uma release: isso separa a
mudança de schema da SHA, do manifesto e da evidência do deploy. Em incidente,
capture preflight/ledger, interrompa a promoção e corrija por uma nova migration
forward-only. Finalize consultando ledger e advisors pelo Supabase CLI/MCP e
executando uma verificação funcional somente leitura. Para alterações de RLS,
valide ainda o comportamento com um JWT de tenant em ambiente isolado antes de
produção.

### Relação com o deploy da Hostinger

O caminho direto integra o snapshot de código e o schema sob a mesma SHA.
`make deploy-beta` e `make deploy-prod` executam preflight/ledgers antes do
build, aplicam migrations somente depois de todas as imagens serem construídas e
mantêm o advisory lock até verificar o schema e promover a release.

Quando uma release depender de migration:

1. crie apenas uma nova migration forward-only e compatível com a release anterior;
2. execute o deploy aprovado em beta e preserve a evidência das três fases;
3. confirme comportamento, ledger e advisors no ambiente beta;
4. gere nova aprovação da mesma SHA e execute o deploy de produção;
5. confirme verify, smoke público e promoção.

Rollback restaura **somente imagens**. O trap só age quando `pending-sha` ainda
é a SHA desta execução; falha anterior à própria pendência não reverte outra
release. Ele não apaga versões nem checksums e não executa SQL reverso. Por isso,
o schema promovido deve continuar aceitando a release anterior. Se verify,
readiness ou smoke falhar depois do `apply`, mantenha o schema avançado, reverta
as imagens e corrija o banco apenas com outra migration forward-only. Consulte o
guia Hostinger e o
runbook de rollback.

## Colisão histórica de migrations

Produção preserva duas entradas legadas chamadas `partner_royalties` nas
versões `20260522120000` e `20260522120001`. O Git preserva a primeira versão
como a antiga migration de webhook Abacate. As linhagens convergem depois da
remoção dos artefatos Abacate e são verificadas por
`20260718150751_reconcile_20260522_history_collision.sql`.

Esse checkpoint deve continuar no histórico. Não use comandos de repair para
“alinhar nomes”, não apague entradas remotas e não renomeie os arquivos antigos.

## Desenvolvimento local

O modo preferencial para reproduzir todo o histórico é a stack Supabase CLI:

```bash
make supabase-local-start
make supabase-local-status
```

O perfil opcional `local-db` do Docker Compose é apenas um bootstrap de
compatibilidade. Ele monta `scripts/postgres/init.sql` e um subconjunto de três
migrations antigas; portanto não oferece paridade com as 57 versões do snapshot
de produção nem com a 58ª migration local pendente. Use a stack Supabase CLI
para validar uma alteração de schema.

## Storage

- Buckets, policies e convenções de path:
  [`storage_buckets.md`](./storage_buckets.md).
- Salve objetos com prefixo de tenant:
  `{tenant_id}/{dominio}/{uuid}.{ext}`.
- Mudanças de bucket ou policy também precisam ser versionadas em uma nova
  migration e verificadas com o mesmo procedimento de dry-run/push.
