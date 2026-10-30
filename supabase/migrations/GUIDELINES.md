# Migration Guidelines

These rules apply to every new SQL file added to `supabase/migrations/`.
Their goal is to make tenant isolation a build-time concern rather than
a "we'll remember during review" concern. CI enforces the parts that can
be enforced mechanically (see `scripts/check-migrations-rls`); the rest
is on us.

## File naming

- Format: `YYYYMMDDHHMMSS_short_descricao.sql` (UTC).
- `descricao` is `snake_case` and starts with a verb when possible:
  `create_…`, `add_…`, `drop_…`, `backfill_…`, `rls_…`.
- One concern per migration. If you find yourself writing "_and_", split.
- Do not edit migrations that have already shipped — add a new one. The
  legacy `scripts/postgres/*.sql` files in this repo are frozen.

## Naming inside SQL

- All identifiers are `snake_case`.
- The tenant scoping column is always called **`tenant_id`** — never
  `account_id`, `customer_account_id`, `org_id`, `workspace_id`.
  *Mixing names was the original sin: `pkg/clients/calendar_client.go`
  used to pass `CustomerAccountID` where a tenant was expected. Use
  `pkg/domain.TenantID` on the Go side.*
- Column types:
  - `tenant_id` → `UUID NOT NULL` (preferred). A handful of legacy
    financial tables use `text`; new tables must use `UUID`.
  - Timestamps → `TIMESTAMPTZ`, default `NOW()`, with `deleted_at` for
    soft deletion where applicable.

## Row Level Security (mandatory for multi-tenant tables)

A table is "multi-tenant" if it has a `tenant_id` column. Every
multi-tenant table **must** declare RLS in the same migration that
creates it:

```sql
ALTER TABLE my_schema.my_table ENABLE ROW LEVEL SECURITY;

DROP POLICY IF EXISTS tenant_isolation ON my_schema.my_table;
CREATE POLICY tenant_isolation ON my_schema.my_table
    FOR ALL TO authenticated USING (
        tenant_id = NULLIF(
            (SELECT auth.jwt()) -> 'app_metadata' ->> 'tenant_id',
            ''
        )::uuid
    ) WITH CHECK (
        tenant_id = NULLIF(
            (SELECT auth.jwt()) -> 'app_metadata' ->> 'tenant_id',
            ''
        )::uuid
    );
```

Notes:

- The tenant claim lives inside `app_metadata` — that's the path
  every other policy in this repo uses. Do **not** use
  `auth.jwt() ->> 'tenant_id'` (top-level); it returns NULL on a
  Supabase-issued JWT.
- Wrap the JWT helper in `(SELECT auth.jwt())` so Postgres evaluates it once
  per statement instead of once per row. Avoid overlapping `FOR SELECT` and
  `FOR ALL` policies; use one `FOR ALL` policy only when full CRUD is intended,
  or separate non-overlapping action policies for least privilege.
- `DROP POLICY IF EXISTS` first so migrations are idempotent on reruns
  (Supabase migrations sometimes get replayed after a recover).
- If the table is service-only (writes through a service-role pool
  bypassing RLS) but reads might ever go through the Supabase REST
  layer, you **still** want RLS — it is defense in depth. See
  `20260514120000_rls_signature_webhook_events.sql` for an example
  retrofit.
- If you need a per-tenant index, add it alongside the RLS block:
  `CREATE INDEX IF NOT EXISTS idx_<schema>_<table>_tenant ON <schema>.<table> (tenant_id);`
- Use `to_regprocedure('auth.jwt()')` when checking whether the Supabase
  helper exists. `to_regproc('auth.jwt()')` is invalid for a signature and
  returns `NULL`, silently skipping guarded policy blocks.
- Dynamic schema-wide retrofit blocks cover only tables that exist when that
  migration runs. They do not satisfy RLS for tables created by later files;
  the CI checker preserves migration order and enforces this.
- Legacy `tenant_id text` tables must compare against the text claim directly.
  Do not cast the claim to UUID unless the column itself is UUID.

## Exceptions to RLS

Add the table to `supabase/migrations/.no-rls-allowlist.txt`, one per
line, with a `#` comment describing **why** it's allowlisted. The CI
check honors this file. Each new line should be reviewed by someone
outside the immediate author's team — this is the one knob that lets
data leak.

Acceptable reasons for allowlisting:

- The table is the tenant directory itself (e.g. `auth.tenants`) — RLS on
  it would create a chicken-and-egg during login. **Identidade do produto
  (`toca_auth.*`) não entra mais aqui**: desde o P2/M1 as tabelas de
  identidade têm RLS ligado e política vinculada ao JWT quando o pool de
  aplicação as consulta (ver `scripts/check-migrations-rls`, lista
  `identityTables`). Leituras pré-autenticação (login por e-mail, convite
  por token) não têm claim e devem passar pelo pool de serviço — o
  `pkg/database` reporta `database.rls_backstop_disabled` no boot enquanto
  o role de aplicação ainda bypassar RLS.
- The table is intentionally cross-tenant audit/observability and is
  only read by platform-admin tooling under a service role (e.g.
  `audit.*`). The service role must be documented and the table must
  not be exposed to the user-facing JWT path.
- The table has no per-tenant ownership concept (e.g.
  `events.outbox` where rows are routed by event metadata, not
  user identity).

Never allowlist a table just because RLS is "extra work for this PR".
If the table has `tenant_id`, RLS goes in the same migration.

## Foreign keys to tenants

We do **not** currently enforce `tenant_id REFERENCES tenants(id)`
because the `tenants` table lives in the auth schema and the FK creates
deployment ordering pain. Treat tenant_id as a logical FK — RLS plus
application-side checks are the real isolation boundary.

## Application role and the RLS backstop

RLS only protects the services if the **application** pool connects with a role
that does *not* have `BYPASSRLS`/`SUPERUSER`. `pkg/database` opens two pools:

- `DATABASE_URI` — application/tenant-scoped pool, sets `request.jwt.claims`
  per request. This is the one RLS is supposed to filter.
- `DATABASE_URI_SERVICE` — cross-tenant pool. It **must** bypass RLS
  (verified at boot, otherwise `ErrServiceRoleRequired`).

Boot reports the state: `database.rls_backstop_active` when the application
pool is subject to RLS, `database.rls_backstop_disabled` (ERROR in release)
while it bypasses. `REQUIRE_RLS_BACKSTOP=true` turns that into a boot failure.
When the application role is switched to a non-bypass one, pre-auth lookups
(login by e-mail, invitation by token, password reset) and cross-tenant workers
must move to the service pool — they have no JWT to scope a policy.

## What CI checks

`scripts/check-migrations-rls` (also exposed as `make check-rls` /
`go run ./scripts/check-migrations-rls/...`) runs on every PR that
touches `supabase/migrations/**`. Besides every table with a `tenant_id`
column, it enforces the identity class (`identityTables` in the checker):
`toca_auth.{accounts,credentials,user_preferences,invitations,groups}`,
`core.partnerships` (RLS + policy) and
`toca_auth.password_resets`/`events.processed_events` (RLS only,
deny-by-default). It will fail if:

1. A `CREATE TABLE` declares `tenant_id` but the same migration (or any
   later migration referencing the table by `schema.table` name) never
   does `ENABLE ROW LEVEL SECURITY`.
2. RLS is enabled but no `CREATE POLICY` exists for the table.
3. A table is in the allowlist but no longer has a `tenant_id` column
   (allowlist entries should be deleted when the table is dropped).

The check does **not** validate the policy expression — review still
matters. It also does not catch typos like `tenant_id = auth.jwt()…`
without the `::uuid` cast; copy the snippet above instead.

## Local workflow

```sh
# Lint the migration set:
GOWORK=off go run ./scripts/check-migrations-rls

# Apply locally (against docker-compose.local-db.yml):
supabase db reset            # or:  psql -f supabase/migrations/<file>.sql
```

If you need to test a destructive migration, copy `local` to a scratch
schema first.
