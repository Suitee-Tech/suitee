# Suitee

**Open-source real estate management platform.** CRM, property portfolio, deal pipeline, contracts, electronic signature, billing, client portal, public property sites and an AI assistant — built as a Go microservices monorepo with a React panel and a Next.js/Payload public site layer.

[![License: AGPL v3](https://img.shields.io/badge/License-AGPL%20v3-blue.svg)](LICENSE)

Official site: **[suitee.com.br](https://suitee.com.br)**

> The product documentation is primarily written in Brazilian Portuguese. Code, APIs and this README are in English.

## Why this repository exists

A real estate transaction crosses many domains: a lead becomes a visit, a visit becomes a proposal, a proposal becomes a contract, and a contract produces signatures, billing schedules, client-portal updates and a public listing. Suitee keeps those steps in one coherent platform while keeping each domain behind an explicit boundary.

The repository is a monorepo, not a monolith: every service is independently containerized and communicates through HTTP or events.

## Architecture at a glance

```text
Browser / Client portal / Public site
                  │
               Traefik
                  │
               Gateway
       ┌──────────┼───────────────┐
     Auth        Core            CRM
       │          │                │
     Deal ───── Financial ───── Calendar
       │          │                │
  Signature    Comm / Portal     Sites
       └──────────┴────────────────┘
             PostgreSQL + RabbitMQ
```

- **Gateway** — authentication context, CORS, routing, entitlements and edge protections.
- **Domain services** — Go/Gin services with their own schemas and permissions.
- **Frontend** — React + Vite panel (`toka`).
- **Public sites** — Next.js + Payload (`toca-web`), with per-tenant hosts and automatic TLS.
- **AI** — `toca-ai` orchestrates an MCP tool surface (`toca-mcp`) with permission-aware reads and human-confirmed writes.
- **Events** — RabbitMQ with transactional outbox, retries and dead-letter queues.
- **Data** — PostgreSQL (Supabase-compatible), JSONB aggregates and row-level security for tenant isolation.

## Repository layout

```text
gateway/            API gateway and edge policies
toca-auth/          authentication, permissions, tenants
toca-core/          properties, companies and shared domain
toca-crm/           leads, roulette and client relationships
toca-deal/          deal pipeline, proposals and contracts
toca-financial/     billing, commissions, transfers and reconciliation
toca-comm/          email, notifications, messaging and WebSockets
toca-calendar/      appointments and visits
toca-ops/           maintenance, inspections and tickets
toca-portal/        client and owner portal
toca-signature/     Clicksign integration and signed PDFs
toca-sites/         public-site configuration, projection and metrics
toca-ai/            AI chat, prompts and tool orchestration
toca-mcp/           Model Context Protocol tools and help corpus
toca-ocr/           document/image extraction worker
toca-web/           Next.js + Payload public property sites
toka/               React/Vite operator panel
pkg/                shared Go packages
supabase/           database migrations and RLS policies
docs/               architecture, domain and engineering documentation
scripts/            local and CI tooling
```

## Getting started

### Prerequisites

- Docker and Docker Compose
- Go 1.25+
- Node.js 24 and pnpm 10
- Supabase CLI for optional local services

### Run the local stack

```bash
git clone https://github.com/Suitee-Tech/suitee.git
cd suitee
cp .env.example .env
make doctor ENV=local
make up ENV=local
```

The operator panel is available at `http://localhost:3000`. For frontend-only development:

```bash
cd toka
pnpm install --frozen-lockfile
pnpm dev
```

> Never commit your `.env` or service credentials. The repository ships only with `.env.example` placeholders.

## Tests

```bash
make test-go MODULE=pkg    # a Go module
cd toka && pnpm lint:types && pnpm test
```

See [docs/development/testing.md](docs/development/testing.md) and [CONTRIBUTING.md](CONTRIBUTING.md).

## Documentation

Start with [docs/README.md](docs/README.md). Recommended paths:

- [System overview](docs/architecture/system-overview.md)
- [Services](docs/architecture/services/README.md)
- [Local development](docs/development/run-local.md)
- [Testing](docs/development/testing.md)
- [Git flow](docs/development/git-flow.md)
- [Database](docs/database/README.md)

The public documentation covers architecture, domain rules, local development and testing. Operational runbooks, production access details, incident reports and customer-specific material are intentionally kept private.

## Security

Please do not open a public issue for a security vulnerability. Follow [SECURITY.md](SECURITY.md) to report it privately.

Multi-tenancy is enforced in layers: signed tenant context, permission middleware and PostgreSQL row-level security. Service credentials never belong in the browser.

## Contributing

Contributions are welcome. Read [CONTRIBUTING.md](CONTRIBUTING.md), the [Git flow and branch protection policy](docs/development/git-flow.md) and [CODE_OF_CONDUCT.md](CODE_OF_CONDUCT.md) before opening a pull request.

`main` and `develop` are protected: contributors land changes through reviewed pull requests with green CI.

## License

Suitee is licensed under the **GNU Affero General Public License v3.0**. See [LICENSE](LICENSE).

If you run a modified version as a network service, the AGPL requires you to make the corresponding source available to users of that service.
