# toca-web

Public real-estate storefronts for the Toka platform. Each agency gets a branded
site on `{marca}.imoveis.suitee.com.br`, backed by Payload CMS 3 (PostgreSQL) and
served by Next.js 16. The Go service [`toca-sites`](../toca-sites) owns the
orchestration and projects published stock into Payload; Payload is a projection,
not the source of truth.

## Architecture

```
Browser ── Host: marca.imoveis... ──► Next.js (this app)
                                         │
                        middleware.ts    │ rewrite to /site/{marca}/...
                                         ▼
                              React Server Components
                                         │
                           Payload Local API (server-side)
                                         ▼
                              Payload CMS / Postgres schema "payload"

Browser ── view beacon / interest form ──► toca-sites (Go, :8094)
                                            │
                                            └─ CRM / events / sites.* tables

toca-sites ── X-Internal-Key + X-Tenant-Id ──► Payload REST (/api/*) ──► Postgres
toca-sites ── POST /api/revalidate ──────────► Next.js tenant tag/path invalidation
```

- **No agency uses Payload Admin.** Agencies publish and manage content in Toka.
  Payload Admin is restricted to Suitee super-admins.
- **No tenant from the query string, cookies or request body on the public site.**
  Tenant resolution is exclusively the validated request `Host`.
- **No build-time database access.** Every public route is dynamic so
  `next build` succeeds without a live Postgres; `/api/health` never touches
  Payload or the database.

## Routes

Public routes, rewritten per Host without changing the visible URL:

| Visible URL | Internal route |
| --- | --- |
| `/` | `/site/[tenant]` |
| `/imoveis` | `/site/[tenant]/imoveis` |
| `/imoveis/[seoSlug]` | `/site/[tenant]/imoveis/[seoSlug]` |
| `/sobre` | `/site/[tenant]/sobre` |
| `/contato` | `/site/[tenant]/contato` |
| `/sitemap.xml` | `/site/[tenant]/sitemap.xml` |
| `/robots.txt` | `/site/[tenant]/robots.txt` |

Operational routes:

| Route | Behaviour |
| --- | --- |
| `GET /api/health` | `200 {status:"ok",service:"toca-web"}`; no Payload/DB access. |
| `POST /api/revalidate` | Internal ISR invalidation; `X-Internal-Key` required. |
| `POST /api/site-proxy/views` | Same-origin fallback to `toca-sites` when `NEXT_PUBLIC_SITES_API_URL` is unset. |
| `POST /api/site-proxy/leads` | Same-origin fallback to `toca-sites` when `NEXT_PUBLIC_SITES_API_URL` is unset. |
| `/admin` | Payload Admin (super-admin only). |
| `/api/*` | Payload REST/GraphQL, protected by collection access rules. |

Middleware excludes `/api`, `/admin`, `/_next`, `/media`, statics, and returns a
404 for direct `/site/*` URLs. Apex/marketing hosts and unknown paths render a
404; there is no default tenant fallback.

## Host resolution

Production: `{marca}.imoveis.suitee.com.br`
Local: `{marca}.imoveis.localhost`

Rules enforced before any database lookup:

- `PUBLIC_SITE_HOST_SUFFIX` matches the exact host suffix; nested subdomains are
  rejected.
- `{marca}` is 3–48 characters, lowercase `a-z`, `0-9` and `-`, with no leading
  or trailing hyphen.
- Reserved brands: `www`, `app`, `api`, `admin`, `portal`, `sites`, `mail`,
  `cdn`, `imoveis`, `suitee`.
- The Host must be `{marca}.{suffix}`; `www.{suffix}` and the apex are not tenant
  hosts.
- Once the slug is valid, the tenant layout queries Payload for
  `slug == {marca}` **and** `enabled == true`. Missing or disabled tenants return
  a 404.

The rewrite is implemented in [`src/middleware.ts`](src/middleware.ts). Next.js 16
deprecates the `middleware` filename in favour of `proxy`; this app still uses
`middleware.ts` intentionally to match the Sites plan.

**Reverse proxy contract:** Traefik (or another edge) must preserve the public
`Host` header. The app does not fall back to `X-Forwarded-Host`.

### Local domains

Modern browsers resolve `*.localhost` to loopback automatically. Usually this is
enough:

```
http://imobiliaria-exemplo.imoveis.localhost:3000
```

If the browser or OS does not resolve wildcard localhost, add one entry per brand:

```bash
# macOS/Linux
echo "127.0.0.1 imobiliaria-exemplo.imoveis.localhost" | sudo tee -a /etc/hosts
```

For server-side checks without DNS changes:

```bash
curl -H 'Host: imobiliaria-exemplo.imoveis.localhost' http://127.0.0.1:3000/
```

## First run (local)

Prerequisites: Node.js 22+ (the repo uses Node 24/26), pnpm 10, and a PostgreSQL
database (local Postgres or the shared Supabase Postgres).

```bash
cd toca-web
cp .env.example .env
# Set DATABASE_URI, PAYLOAD_SECRET (>= 32 chars) and INTERNAL_API_KEY.
pnpm install

# Option A (recommended): apply the checked-in migration
#   set PAYLOAD_DB_PUSH=false in .env for this path
pnpm migrate

# Option B: let Payload create the "payload" schema through schema push.
#   .env.example ships with PAYLOAD_DB_PUSH=true for local development.

pnpm dev
```

Open `http://localhost:3000/api/health` to confirm the process is alive. Then
open `http://localhost:3000/admin` and create the first super-admin. The first
user bootstrap is only allowed while the `users` collection is empty; afterwards
only an existing super-admin can create users.

The local `payload` schema is isolated from the Supabase product schemas. The
initial migration explicitly runs `CREATE SCHEMA IF NOT EXISTS "payload"` before
creating tables, so `pnpm payload migrate` works against a brand-new database
that does not yet contain that schema.

Production deploys must run migrations as a release step (not on every web boot):

```bash
DATABASE_URI=... PAYLOAD_SECRET=... pnpm payload migrate
```

No Payload migration is run automatically by the web container. If the schema is
missing, `/api/health` still works; public/admin pages fail until migrations (or
dev schema push) have run.

### Production-style local build

```bash
DATABASE_URI=postgresql://... \
PAYLOAD_SECRET=... \
PAYLOAD_PUBLIC_SERVER_URL=http://localhost:3000 \
PUBLIC_SITE_HOST_SUFFIX=imoveis.localhost \
NEXT_PUBLIC_SITE_HOST_SUFFIX=imoveis.localhost \
pnpm build
```

`next build` only evaluates the Payload config; it does not query Postgres. Use
`pnpm dev` for local work with `output: "standalone"` (Next warns about `next
start` with standalone output). The production artifact is run as
`node .next/standalone/server.js` after `.next/static` and `public` are copied —
exactly what the Dockerfile does.

## Environment

| Variable | Required | Purpose |
| --- | --- | --- |
| `DATABASE_URI` | yes | Shared Supabase/Postgres connection; Payload uses schema `payload`. |
| `PAYLOAD_SECRET` | yes | Payload auth/JWT secret, minimum 32 characters. |
| `PAYLOAD_PUBLIC_SERVER_URL` | yes | Admin/API origin used by Payload; the public storefront uses tenant Hosts instead. |
| `PUBLIC_SITE_HOST_SUFFIX` | yes | `imoveis.suitee.com.br` in production, `imoveis.localhost` locally. |
| `NEXT_PUBLIC_SITE_HOST_SUFFIX` | Docker build | Build-time fallback for the middleware bundle; keep equal to the suffix. |
| `PORTAL_PUBLIC_URL` | no | Link "Área do cliente" no header/rodapé do site (ex.: `https://app.example.com/portal`). |
| `INTERNAL_API_KEY` | yes | `X-Internal-Key` for Go writes and `/api/revalidate`. |
| `SITES_INTERNAL_API_URL` | recommended | Server-to-server Go URL used by the fallback proxy (`http://toca-sites:8094`). |
| `NEXT_PUBLIC_SITES_API_URL` | recommended | Browser URL for Go views/leads; baked into the client at build. |
| `PUBLIC_SITE_SCHEME` | no | Override canonical scheme (`http`/`https`); defaults to `http` for localhost and `https` elsewhere. |
| `PAYLOAD_DB_PUSH` | no | `true` for local schema push. Keep `false` in production and run migrations. |
| `MEDIA_DIR` | no | Local upload directory, default `./media`; Docker uses `/app/media`. |
| `S3_BUCKET` | prod | Enables S3-compatible media storage when all S3 values are present. |
| `S3_REGION` | prod | S3 region; use `auto` for R2. |
| `S3_ENDPOINT` | no | Custom endpoint for R2/MinIO/Supabase Storage. |
| `S3_ACCESS_KEY_ID` | prod | S3 access key. |
| `S3_SECRET_ACCESS_KEY` | prod | S3 secret key. |
| `S3_FORCE_PATH_STYLE` | no | `true` for MinIO-style path addressing. |
| `S3_PREFIX` | no | Object prefix, default `media`. |

`REVALIDATE_SECRET` is not used: the revalidation contract reuses
`INTERNAL_API_KEY` because toca-sites already sends `X-Internal-Key`.

## Collections

All content collections have a tenant relationship added by
`@payloadcms/plugin-multi-tenant`. The company UUID stays in
`tenants.suiteeTenantId`; the Payload doc id is internal and is what Go stores.

### `tenants`

`suiteeTenantId` (UUID, unique), `slug` (unique, host brand), `name` (trade name),
`enabled`, `domain` (reserved for future custom domains), `domains[]`,
`branding` (`logoUrl`, uploaded `logo`, `coverUrl`, uploaded `cover`,
`primaryColor`, `accentColor`, `theme`, `tagline`), and `contact` (email, phone,
WhatsApp, address, city, state).

### `pages`

`pageKey` (`home` | `about` | `contact`, unique per tenant), `title`,
`subtitle`, `hero` (optional Lexical), `body` (plain text, the Toka/Toca writer),
`bodyRich` (optional Lexical for future CMS editing), `status`
(`draft` | `published`), `publishedAt`, and `seo` (`metaTitle`,
`metaDescription`, `ogImage`, `noindex`).

### `listings`

Projection of toca-core properties: `propertyId` (UUID), `seoSlug`, `title`,
`description` (plain text), `listingType`, `propertyType`, prices, `address`
(with `showExactAddress`), `features`, `amenities` (string list), `images`
(external Storage URLs), `status`, `publishedAt`, `sourceUpdatedAt`.
`(tenant, propertyId)` and `(tenant, seoSlug)` are unique.

### `media`

Payload uploads. Local disk in development, S3-compatible storage in production.
The S3 plugin is always registered with `alwaysInsertFields` so the database
schema does not depend on whether S3 is configured. Branding image URLs sent by
Go are preferred when present.

### `users`

Super-admins only, with a `role=super-admin` field. No agency accounts. The
custom `internal-service` auth strategy lets Payload process machine writes but
is denied from the users collection and from the admin UI.

## Internal API contract (Go → toca-web)

Base: `PAYLOAD_API_URL` (for example `http://toca-web:3000`).
Every internal request sends:

```
X-Internal-Key: <INTERNAL_API_KEY>
X-Tenant-Id: <company UUID>   # same value as tenants.suiteeTenantId
```

The client is an accepted Payload identity only when both headers are valid.
`X-Tenant-Id` is the **company UUID**, never the Payload tenant doc id. The
tenant relationship on every write is forced server-side from this header; a
body cannot write into another tenant. Unauthorized writes receive Payload's
403 response (`{"errors":[{"message":"You are not allowed to perform this action."}]}`).
Internal reads and updates are scoped to that company UUID.

### Tenant upsert

```http
GET /api/tenants?where[suiteeTenantId][equals]=<company-uuid>&limit=1&depth=0
```

Then `POST /api/tenants` or `PATCH /api/tenants/:payloadId`:

```json
{
  "suiteeTenantId": "<company-uuid>",
  "slug": "imobiliaria-exemplo",
  "name": "Imobiliária Exemplo",
  "enabled": true,
  "domains": [{ "domain": "imobiliaria-exemplo.imoveis.suitee.com.br" }],
  "branding": {
    "logoUrl": "",
    "coverUrl": "",
    "primaryColor": "#0f172a",
    "accentColor": "#f59e0b",
    "theme": "light",
    "tagline": ""
  }
}
```

Response is the standard Payload create/update shape, e.g.
`{"doc":{"id":1,...},"message":"..."}`.

### Listing upsert

Use the locally stored Payload doc id when available:

```http
PATCH /api/listings/:payloadId
```

Fallback lookup:

```http
GET /api/listings?where[propertyId][equals]=<property-uuid>&where[tenant.suiteeTenantId][equals]=<company-uuid>&limit=1&depth=0
```

Then `POST /api/listings` if none exists. Accepted body (exact field names):

```json
{
  "tenant": "<payload tenant doc id>",
  "propertyId": "<property-uuid>",
  "seoSlug": "apartamento-3-quartos-centro",
  "title": "Apartamento 3 quartos no Centro",
  "description": "texto simples\n\ncom parágrafos",
  "listingType": "sale",
  "propertyType": "apartment",
  "salePrice": 350000,
  "rentPrice": 0,
  "condoFee": 450,
  "propertyTax": 1200,
  "address": {
    "street": "Rua ...",
    "number": "100",
    "complement": "Apto 801",
    "neighborhood": "Centro",
    "city": "Fortaleza",
    "state": "CE",
    "postalCode": "60000-000",
    "showExactAddress": false
  },
  "features": {
    "bedrooms": 3,
    "bathrooms": 2,
    "suites": 1,
    "parkingSpaces": 2
  },
  "amenities": ["Piscina", "Academia"],
  "images": [{ "url": "https://...", "alt": "...", "order": 0 }],
  "status": "published",
  "publishedAt": "2026-10-26T12:00:00Z",
  "sourceUpdatedAt": "2026-10-26T12:00:00Z"
}
```

Optional extras for future syncs: `features.areaTotal`, `features.areaBuilt`,
`address.showExactAddress` and `images[].caption`. Unpublish is:

```http
PATCH /api/listings/:payloadId
{ "status": "unpublished" }
```

Only `status=published` is served publicly, indexed or included in the sitemap.

### Page upsert

```http
GET /api/pages?where[tenant.suiteeTenantId][equals]=<company-uuid>&where[pageKey][equals]=home&limit=1&depth=0
```

Then `POST /api/pages` or `PATCH /api/pages/:payloadId`:

```json
{
  "tenant": "<payload tenant doc id>",
  "pageKey": "home",
  "title": "...",
  "subtitle": "...",
  "body": "texto simples",
  "status": "published"
}
```

`body` is a plain textarea rendered as paragraphs. The optional `bodyRich`
Lexical field is not used unless `body` is empty.

### Revalidation

```http
POST /api/revalidate
Content-Type: application/json
X-Internal-Key: <INTERNAL_API_KEY>

{ "tenantSlug": "imobiliaria-exemplo", "paths": ["/", "/imoveis", "/imoveis/apartamento-3-quartos-centro"] }
```

Response: `200 {"revalidated":true}`. Missing/bad key: `401
{"error":"unauthorized"}`. Malformed body/path: `400`. `X-Tenant-Id` is not
required; if present it must be a valid UUID. Invalid slug shape/path returns
400 and no cache is touched. Revalidation invalidates the tenant tag (all
tenant data caches) and the supplied public/internal paths.

## Public API surface

### Storefront data

Server components never call Payload REST; they use Payload's Local API with
explicit `tenant` + `status=published` constraints. Tenant data is cached with
next/cache `unstable_cache` under a per-tenant tag; full pages are dynamic because
the response varies by Host.

Public Payload REST reads remain safe:

```http
GET /api/listings?where[status][equals]=published&limit=10
Host: imobiliaria-exemplo.imoveis.suitee.com.br
```

- Access is ANDed with `tenant = <tenant resolved from Host>` and
  `status = published`.
- Query parameters can narrow results but cannot widen across tenants.
- Apex or unknown hosts receive 403 from Payload.
- Internal callers can read drafts, scoped to `X-Tenant-Id`.
- On a valid tenant Host, anonymous callers may read that tenant's public
  branding/contact projection (`GET /api/tenants`). This is what makes Payload's
  query-path validation accept nested filters such as
  `where[tenant.slug][equals]=...`. The company UUID (`suiteeTenantId`) is hidden
  by field access; arbitrary tenant enumeration is still impossible.

### Views and leads (browser → toca-sites)

If `NEXT_PUBLIC_SITES_API_URL` is set, the browser calls Go directly. Otherwise
the client falls back to the same-origin `/api/site-proxy/{views|leads}` routes,
which forward server-side using `SITES_INTERNAL_API_URL`.

Both include `X-Site-Host: <hostname without port>` and a JSON body:

**Views** — `POST /api/v1/sites/public/views`

```json
{
  "property_id": "uuid",
  "site_slug": "imobiliaria-exemplo",
  "path": "/imoveis/apartamento-3-quartos-centro",
  "referrer": "https://...",
  "session_id": "anonymous-session-id",
  "utm_source": "...",
  "utm_medium": "...",
  "utm_campaign": "..."
}
```

**Leads** — `POST /api/v1/sites/public/leads`

```json
{
  "property_id": "uuid",
  "site_slug": "imobiliaria-exemplo",
  "name": "Nome",
  "email": "email@exemplo.com",
  "phone": "(85) 99999-9999",
  "message": "...",
  "accepted_contact": true,
  "website": ""
}
```

`accepted_contact` is required by the UI (LGPD consent) and `website` is a
honeypot. Go must allow CORS for `https://*.imoveis.suitee.com.br` and local
tenant origins when direct browser calls are enabled (custom header triggers a
preflight). The view beacon is `keepalive` and fails silently.

## Access control and security model

| Caller | Read `pages`/`listings` | Write pages/listings/tenants | Users / Admin |
| --- | --- | --- | --- |
| Anonymous on tenant Host | published pages/listings + that Host's public tenant projection | no | no |
| Anonymous on apex/unknown Host | no | no | no |
| Valid internal key + company UUID | own tenant, all statuses | own tenant only | no |
| Super-admin session | all tenants | all tenants | yes |

Key properties:

- Public tenant selection never comes from a query string, cookie, localStorage
  or request body.
- Tenant write scope is enforced twice: access predicates and beforeValidate
  hooks that overwrite the relationship from `X-Tenant-Id`.
- Cross-tenant updates by internal callers fail even when they know a document id.
- `X-Internal-Key` comparison is constant-time. Missing `INTERNAL_API_KEY` fails
  closed.
- Internal credentials never grant admin UI access or users-collection access.
- Media files are public brand/listing assets; uploads are restricted to
  internal/super-admin.
- Payload Admin is not the agency panel. Agencies use Toka; only super-admins
  can authenticate into `/admin`. Use `PAYLOAD_PUBLIC_SERVER_URL` as the admin
  origin because Payload's CSRF allowlist is pinned to it.
- Payload's CSRF/CORS are pinned to `PAYLOAD_PUBLIC_SERVER_URL`, which should be
  the admin/API origin. The public storefront reads Payload through the Local API
  and does not need browser CORS to Payload.

## SEO

- Listing `<title>`: `{título do imóvel} | {nome da imobiliária}`.
- `og:site_name` and JSON-LD `seller` are the agency; Suitee never appears as the
  page brand.
- Canonical URLs use the request Host, with the scheme inferred (`https` except
  `*.localhost`) or set via `PUBLIC_SITE_SCHEME`.
- `RealEstateListing` JSON-LD includes `Offer`, agency seller, address, images,
  features and price.
- Draft/unpublished listings and pages are not publicly readable, not in the
  sitemap, and detail pages return 404.
- Sitemap/robots are generated per tenant host and exclude `/admin`, `/api`,
  `/site`.
- Page `seo.noindex` is honored; a draft page falls back to non-CMS copy rather
  than leaking draft content.

## Caching / ISR

Full-route ISR would be unsafe because Next's route cache is not keyed by Host.
Therefore storefront pages are rendered dynamically, while all Payload reads are
cached with `unstable_cache` under a `site:{slug}` tag (5-minute TTL). Go's
on-demand revalidation calls `/api/revalidate` to invalidate that tenant's data
immediately and also revalidates the affected public/internal paths. Tenant
lookups are deduped per request with React `cache`.

## Media storage

- **Development:** files go to `MEDIA_DIR` (default `./media`, gitignored).
- **Production:** set all S3 env vars to enable
  `@payloadcms/storage-s3` for the `media` collection. Bucket needs public read
  access for site assets (or configure a public custom domain/policy). S3/R2/
  MinIO/Supabase Storage S3 endpoints are supported via `S3_ENDPOINT` and
  `S3_FORCE_PATH_STYLE`.
- The S3 plugin inserts its fields even when disabled (`alwaysInsertFields`), so
  migrations are consistent across environments. Switching storage backends
  does not require an extra schema migration after the initial one.
- The public site prefers the external `branding.logoUrl`/`coverUrl` and listing
  `images[].url` sent by Go. Uploaded Payload media is a fallback.

## Docker Compose

No compose da raiz o `toca-web` é publicado em **`http://localhost:3002`**
(`WEB_PORT`, porque 3000 é o Toka e 3001 o Grafana). O `PAYLOAD_PUBLIC_SERVER_URL`
local acompanha essa porta. Para rodar o app fora do Docker (`pnpm dev`), a porta
continua 3000.

O compose da raiz sobe o serviço one-shot `toca-web-migrate` (target `migrator`
do Dockerfile) **antes** do `toca-web`: ele roda `payload migrate` com o schema
versionado e o `toca-web` depende de `service_completed_successfully`. A imagem
de runtime é Next standalone e não carrega o CLI/drizzle-kit, por isso a
migração não acontece nela. `PAYLOAD_DB_PUSH` fica `false` nos containers; o
`.env.example` só o liga para rodar o app fora do Docker em desenvolvimento.

## Docker

Build context: `./toca-web` (or `toca-web/` from the repo root). Port 3000.
Non-root `nextjs` user, Next standalone output.

```bash
cd toca-web

docker build \
  --build-arg NEXT_PUBLIC_SITES_API_URL=https://api.example.com \
  --build-arg NEXT_PUBLIC_SITE_HOST_SUFFIX=imoveis.suitee.com.br \
  --build-arg PUBLIC_SITE_HOST_SUFFIX=imoveis.suitee.com.br \
  -t toca-web .

docker run --rm -p 3000:3000 \
  -e DATABASE_URI='postgresql://...' \
  -e PAYLOAD_SECRET='...' \
  -e PAYLOAD_PUBLIC_SERVER_URL='https://web-internal...' \
  -e PUBLIC_SITE_HOST_SUFFIX='imoveis.suitee.com.br' \
  -e INTERNAL_API_KEY='...' \
  -e SITES_INTERNAL_API_URL='http://toca-sites:8094' \
  -e NEXT_PUBLIC_SITES_API_URL='https://api.example.com' \
  -v toca-web-media:/app/media \
  toca-web
```

- `NEXT_PUBLIC_*` values are build args and baked into the client; changing them
  requires rebuilding. Runtime env alone does not update them.
- `PUBLIC_SITE_HOST_SUFFIX` is also accepted as a build arg so the middleware
  bundle is deterministic; keep the runtime value identical.
- `/api/health` is the healthcheck and requires no DB. The image healthcheck
  uses it.
- Create the `payload` schema before serving traffic:
  `pnpm payload migrate` from the release job/checkout. The web image is not a
  migration runner and will not auto-push in production.
- A named volume for `/app/media` is only needed when S3 is disabled.

## Trade-offs / phase 1 scope

- Custom agency domains (`www.imobiliaria...`) are not resolved yet; the
  `tenants.domain` field is reserved. The commercial host stays
  `{marca}.imoveis.suitee.com.br`.
- Full-route ISR is intentionally not used due to Host-based cache keying;
  data-level caching + on-demand tenant invalidation provides the same freshness
  guarantee without cross-tenant leakage.
- Payload Admin remains an internal emergency/backoffice tool, not an agency CMS.
- The sitemap caps at 20,000 published listings per tenant. Paginate/extend if a
  tenant exceeds that.

## Verification commands

```bash
cd toca-web
pnpm install
pnpm typecheck

DATABASE_URI='postgresql://postgres:postgres@localhost:5432/toca' \
PAYLOAD_SECRET='dev-secret-dev-secret-dev-secret-123456' \
PAYLOAD_PUBLIC_SERVER_URL='http://localhost:3000' \
PUBLIC_SITE_HOST_SUFFIX='imoveis.localhost' \
pnpm build
```

Both commands pass without a live database. The build output includes dynamic
public routes and `/api/health`.
