# Contributing to Suitee

Thanks for helping improve Suitee. This repository is a monorepo with Go services, a React/Vite panel and a Next.js/Payload public-site layer.

## Before you start

- Read the [architecture overview](docs/architecture/system-overview.md).
- For local setup, follow [docs/development/run-local.md](docs/development/run-local.md).
- Check existing issues and pull requests to avoid duplicate work.
- For a security issue, **do not open a public issue**; follow [SECURITY.md](SECURITY.md).
- Read the [Git flow and branch protection policy](docs/development/git-flow.md). `main` and `develop` are protected; direct pushes are disabled.

## Development workflow

1. Fork the repository and create a branch from `develop` (or from `main` only for a hotfix):

   ```bash
   git checkout develop
   git pull --ff-only
   git checkout -b feat/short-description
   ```

2. Keep changes focused. A pull request should solve one problem and include tests when behavior changes.
3. Run the relevant checks before opening the PR:

   ```bash
   make check
   make test-go MODULE=<module>
   cd toka && pnpm lint:types && pnpm test
   ```

4. Use [Conventional Commits](https://www.conventionalcommits.org/):

   ```text
   feat: add ... 
   fix: correct ...
   docs: explain ...
   test: cover ...
   refactor: simplify ...
   chore: update ...
   ```

5. Open a pull request against `develop` (or `main` for `release/*`/`hotfix/*`) using the template. Describe the problem, the approach, the tests run and any migration or operational impact. A CODEOWNER approval and green CI are required.

## Code expectations

- Respect service boundaries. A service must not read or write another service's schema.
- Every tenant-scoped query must carry tenant context and respect RLS.
- Never commit credentials, `.env` files, customer data, coverage output, generated graphs or build artifacts.
- Prefer typed clients/events over ad-hoc cross-service calls.
- Add or update documentation when changing a public route, event or operational procedure.
- Keep UI copy and accessibility consistent with the existing design system.

## Database changes

Migrations live in `supabase/migrations/`. A migration needs an explicit tenant/RLS review and must be reversible or accompanied by a rollback plan. Do not edit an already-applied migration.

## Review and licensing

By contributing, you agree that your contribution is licensed under the repository's [AGPL-3.0 license](LICENSE). Do not copy code, images or data that you do not have the right to redistribute.
