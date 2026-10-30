SHELL := /bin/bash
.SHELLFLAGS := -eu -o pipefail -c
.DEFAULT_GOAL := help

# Canonical inputs. ENV remains a compatibility alias for ENVIRONMENT, and the
# old lowercase `service=` form remains accepted for restart-service callers.
ifeq ($(origin ENVIRONMENT), undefined)
  ifneq ($(strip $(ENV)),)
    ENVIRONMENT := $(ENV)
  else
    ENVIRONMENT := local
  endif
endif

SERVICE ?=
ifneq ($(strip $(service)),)
  ifeq ($(strip $(SERVICE)),)
    SERVICE := $(service)
  endif
endif

ENV_FILE ?=
IMAGE_TAG ?= $(shell git rev-parse --verify HEAD 2>/dev/null)
WAIT_TIMEOUT ?= 300
TAIL ?= 200
FOLLOW ?= 1
CONFIRM ?=
MODULE ?=
SUPABASE_LOCAL ?= 1
# Sites das imobiliárias (ADR 0006). SITES_PANEL=0 sobe só o site público.
SITES_SLUG ?= exemplo
SITES_PANEL ?= 1
SITES_HOSTS_APPLY ?= 0

STACK = ENVIRONMENT="$(ENVIRONMENT)" \
	SERVICE="$(SERVICE)" \
	ENV_FILE="$(ENV_FILE)" \
	IMAGE_TAG="$(IMAGE_TAG)" \
	WAIT_TIMEOUT="$(WAIT_TIMEOUT)" \
	TAIL="$(TAIL)" \
	FOLLOW="$(FOLLOW)" \
	CONFIRM="$(CONFIRM)" \
	SUPABASE_LOCAL="$(SUPABASE_LOCAL)" \
	bash scripts/stack.sh

SUPABASE_HOME ?= $(CURDIR)/.supabase-home
SUPABASE = HOME=$(SUPABASE_HOME) SUPABASE_DISABLE_TELEMETRY=1 supabase
GO_MODULES := pkg toca-auth toca-core toca-crm toca-deal toca-comm toca-financial toca-signature toca-ops toca-calendar toca-portal toca-sites toca-mcp toca-ai gateway
COVERAGE_BASELINES ?= scripts/coverage-baselines.tsv
TOOLS_BIN ?= $(CURDIR)/.tools/bin
GOLANGCI_LINT_VERSION ?= v2.12.2
GOLANGCI_LINT_PACKAGE ?= github.com/golangci/golangci-lint/v2/cmd/golangci-lint
GOLANGCI_LINT_DIR := $(TOOLS_BIN)/golangci-lint-$(GOLANGCI_LINT_VERSION)
GOLANGCI_LINT := $(GOLANGCI_LINT_DIR)/golangci-lint

.PHONY: \
	help tools doctor config build up down status ps logs restart reset clean \
	check check-compose check-shell check-rls check-function-search-path ci \
	test test-go test-backend lint-go lint-backend test-stack test-e2e test-e2e-isolated \
	frontend frontend-install frontend-lint frontend-test frontend-build frontend-check \
	frontend-playwright-install frontend-e2e-ci \
	run stop dev-up dev-down local-run local-stop local-logs restart-service run-frontend \
	build-experimental run-experimental \
	supabase-local-start supabase-local-stop supabase-local-status supabase-migration-up \
	sites-check sites-up sites-public-up sites-down sites-status sites-logs \
	sites-smoke sites-url sites-hosts sites-migrate sites-reset sites-mail \
	swagger-all test-coverage test-coverage-gate

## Discoverability -----------------------------------------------------------

help:
	@bash scripts/stack.sh help
	@printf '\nAlvos Make adicionais:\n'
	@printf '  check              check-shell + check-compose + check-rls + check-function-search-path + check-route-guards + check-permission-catalog + check-tenant-writes + check-internal-tenant\n'
	@printf '  test               backend + frontend unitário\n'
	@printf '  ci                 checks + coverage + frontend-check\n'
	@printf '  frontend-check     install + lint + testes + build do frontend\n'
	@printf '  frontend-e2e-ci    Playwright determinístico do CI\n'
	@printf '  test-stack         stack Docker efêmera e smoke de readiness\n'
	@printf '  test-go MODULE=x   testes e coverage de um módulo Go\n'
	@printf '  lint-go MODULE=x   golangci-lint de um módulo Go\n'
	@printf '  lint-backend       golangci-lint em todos os módulos Go\n'
	@printf '  tools              instala ferramentas de CI pinadas em .tools/bin\n'
	@printf '\nExemplos:\n'
	@printf '  make up\n'
	@printf '  make logs SERVICE=toca-financial FOLLOW=0\n'
	@printf '  make build ENV=test SERVICE=gateway\n'
	@printf '  make up SUPABASE_LOCAL=0  # usa banco já configurado, sem Supabase CLI\n'
	@printf '  make sites-up            # Toka + toca-sites + toca-web local\n'
	@printf '  make sites-public-up     # só site público (sem painel Toka)\n'
	@printf '  make sites-check         # .env, Docker, portas e host local\n'
	@printf '  make sites-smoke         # health do Go/Next + host público\n'
	@printf '  supabase-migration-up    # aplica migrations pendentes no banco local\n'
	@printf '  sites-mail               # abre o Mailpit (convites/OTP local)\n'
	@printf '  make reset CONFIRM=reset\n'

## Canonical stack interface -------------------------------------------------

doctor:
	@$(STACK) doctor

config:
	@$(STACK) config

build:
	@$(STACK) build

# A successful up always corresponds to the exact images built immediately
# before it. stack.sh deliberately keeps --no-build in the runtime phase.
up: build
	@if [[ "$(ENVIRONMENT)" == "local" && "$(SUPABASE_LOCAL)" != "0" ]]; then \
		$(MAKE) --no-print-directory supabase-local-start; \
	fi
	@$(STACK) up

down:
	@$(STACK) down
	@if [[ "$(ENVIRONMENT)" == "local" && "$(SUPABASE_LOCAL)" != "0" ]]; then \
		$(MAKE) --no-print-directory supabase-local-stop; \
	fi

status:
	@$(STACK) status

ps:
	@$(STACK) ps

logs:
	@$(STACK) logs

restart:
	@$(STACK) restart

reset:
	@$(STACK) reset

# Historical `clean` used to delete volumes without confirmation. It is now a
# safe alias and only proceeds with the same explicit reset acknowledgement.
clean: reset

## Structural checks ---------------------------------------------------------

check-shell:
	@bash scripts/stack.sh check-shell

check-compose:
	@bash scripts/stack.sh check-compose

# Every multi-tenant table declared in supabase/migrations must have RLS and at
# least one policy. Intentional exceptions live in the migration allowlist.
check-rls:
	cd scripts/check-migrations-rls && GOWORK=off go run . -dir ../../supabase/migrations

# Lint `function_search_path_mutable` do Supabase como invariante de build: toda
# função criada por nós precisa de search_path fixo (ver scripts/check-function-search-path).
# É a mesma classe de falha do check-rls: uma migração nova introduz o problema e
# ninguém percebe até o advisor reclamar — ou até alguém explorar o vetor.
check-function-search-path:
	cd scripts/check-function-search-path && GOWORK=off go test ./... && GOWORK=off go run . -dir ../../supabase/migrations

# Invariantes de borda/autorização da revisão de permissões de 2026-09-19:
# a borda não publica /internal (P0/C10), as superfícies de operador rejeitam o
# papel `cliente` e os grupos do portal continuam exigindo-o (P0/A11).
check-route-guards:
	cd scripts/check-route-guards && GOWORK=off go run . -root ../..

# O catálogo TS de permissões é GERADO de pkg/auth/permission_catalog.go (achado
# A6): drift entre backend e frontend escalava privilégios ao salvar a tela.
check-permission-catalog:
	cd pkg && GOWORK=off go run ./cmd/gen-permission-catalog -check

# Item 36 da revisão de permissões: toda escrita em agregado multi-tenant carrega
# o predicado de tenant na própria sentença (`AND tenant_id = $n`), e não só um
# GetByID escopado antes da escrita.
check-tenant-writes:
	cd scripts/check-tenant-scoped-writes && GOWORK=off go run . -root ../..

# Toda chamada cliente a uma rota `/internal/` precisa carregar o tenant: desde o
# achado A3 a rota interna resolve o tenant pela credencial e recusa (400) sem
# ele. O defeito já aconteceu — a tela Financeiro do portal do cliente virava 403
# porque a chamada de cobranças saía sem `tenant_id`.
check-internal-tenant:
	cd scripts/check-internal-tenant-param && GOWORK=off go run . -root ../..

# Serviço que inicializa JWKS precisa da configuração do Supabase no stack local.
# O `toca-mcp` não tinha e o painel de IA anunciava "servidor desconectado"
# (incidente de 2026-09-19) — o sintoma aparecia longe da causa.
check-cors-headers:
	@cd scripts/check-cors-headers && GOWORK=off go run . -root ../..

check-frontend-env:
	@cd scripts/check-frontend-env && GOWORK=off go run . -root ../..

check-internal-caller-key:
	@cd scripts/check-internal-caller-key && GOWORK=off go run . -root ../..

check-jwks-env:
	cd scripts/check-jwks-service-env && GOWORK=off go run . -root ../..

# Permissão do catálogo sem ponto de aplicação é promessa falsa: o admin revoga
# na tela de Permissões, a tela confirma e nada muda (achado A7 da revisão de
# 2026-09-19, remedido no N3 da reauditoria de 2026-09-20). A allowlist de
# permissões de UI/navegação vive no próprio verificador, com o motivo.
check-permission-enforcement:
	cd scripts/check-permission-enforcement && GOWORK=off go run . -root ../..

# Serviço que checa permissão sem o endereço do core nega TUDO para quem não é
# owner/admin (o middleware nega quando o provider falha). Foi o que aconteceu
# com a agenda do corretor em 2026-09-21: `toca-calendar` (e mais quatro
# serviços) sem `CORE_SERVICE_URL` no compose.
check-core-url:
	cd scripts/check-core-service-url && GOWORK=off go run . -root ../..

check: check-shell check-compose check-rls check-route-guards check-permission-catalog check-permission-enforcement check-tenant-writes check-internal-tenant check-jwks-env check-internal-caller-key check-cors-headers check-core-url check-frontend-env check-virus-scan check-help-center

# P1-13: o antivírus do funil de uploads é fail-closed no código — sem o
# clamd no compose e o CLAMAV_URL no toca-deal, TODO upload seria recusado.
# Este gate garante que o wiring do ambiente local existe.
check-virus-scan:
	@bash scripts/check-virus-scan.sh

# Central de Ajuda: sincronia manifest × tópicos, links internos e imagens da
# fonte única (toka/src/help/docs) consumida pelo /ajuda e pelo toca-mcp.
check-help-center:
	@node scripts/docs/check-help-center.mjs

## Go tests and lint ----------------------------------------------------------

tools: $(GOLANGCI_LINT)

$(GOLANGCI_LINT):
	@mkdir -p "$(GOLANGCI_LINT_DIR)"
	GOBIN="$(GOLANGCI_LINT_DIR)" go install $(GOLANGCI_LINT_PACKAGE)@$(GOLANGCI_LINT_VERSION)

test-go:
	@MODULE="$(MODULE)" bash scripts/stack.sh test-go

test-backend:
	@bash scripts/stack.sh test-backend

# Kept per-module so CI can provision golangci-lint once and fan out safely.
lint-go: $(GOLANGCI_LINT)
	@PATH="$(GOLANGCI_LINT_DIR):$$PATH" MODULE="$(MODULE)" \
		GOLANGCI_TIMEOUT="$${GOLANGCI_TIMEOUT:-5m}" bash scripts/stack.sh lint-go

lint-backend:
	@set -eu; \
	for mod in $(GO_MODULES); do \
		$(MAKE) --no-print-directory lint-go MODULE="$$mod"; \
	done

## Frontend ------------------------------------------------------------------

frontend-install:
	cd toka && pnpm install --frozen-lockfile

frontend-lint: frontend-install
	cd toka && pnpm lint

frontend-test: frontend-install
	cd toka && pnpm test

frontend-build: frontend-install
	cd toka && pnpm build:ci

frontend-e2e-ci: frontend-install
	cd toka && pnpm test:e2e:ci

frontend-check: frontend-lint frontend-test frontend-build

frontend-playwright-install: frontend-install
	cd toka && pnpm exec playwright install --with-deps chromium

# Blocking HMR server retained for local frontend-only development.
frontend: frontend-install
	cd toka && pnpm dev

test-stack:
	bash scripts/test-stack.sh

# The deterministic Playwright suite is the supported isolated E2E gate.
test-e2e: frontend-e2e-ci

# Compatibility target: retain both the isolated stack smoke and the
# deterministic Playwright journeys, so old callers do not silently lose UI
# coverage.
test-e2e-isolated: test-stack frontend-e2e-ci

test: test-backend frontend-test

ci: check test-coverage-gate lint-backend frontend-check

## Sites local stack (ADR 0006) ---------------------------------------------

# Interface única para a vitrine local. Lê WEB_PORT/SITES_PORT/
# PUBLIC_SITE_HOST_SUFFIX do .env; o padrão do toca-web é 3002 para não colidir
# com o Toka (3000) nem com o Grafana (3001).
SITES_STACK = SITES_SLUG="$(SITES_SLUG)" \
	SITES_PANEL="$(SITES_PANEL)" \
	SITES_HOSTS_APPLY="$(SITES_HOSTS_APPLY)" \
	ENV_FILE="$(ENV_FILE)" \
	SUPABASE_LOCAL="$(SUPABASE_LOCAL)" \
	bash scripts/local/sites.sh

sites-check:
	@$(SITES_STACK) check

# Stack completa (Toka + gateway + toca-sites + toca-web). Use SITES_PANEL=0
# para subir apenas o site público e a cadeia core/crm/rabbit.
sites-up:
	@$(SITES_STACK) up

sites-public-up:
	@$(MAKE) --no-print-directory sites-up SITES_PANEL=0

sites-down:
	@$(SITES_STACK) down

sites-status:
	@$(SITES_STACK) status

sites-logs:
	@$(SITES_STACK) logs

sites-smoke:
	@$(SITES_STACK) smoke

sites-url:
	@$(SITES_STACK) urls

sites-hosts:
	@$(SITES_STACK) hosts

sites-migrate:
	@$(SITES_STACK) migrate

sites-mail:
	@printf 'Mailpit local (convites/OTP): http://localhost:54324\n'
	@(command -v open >/dev/null 2>&1 && open http://localhost:54324) || \
		(command -v xdg-open >/dev/null 2>&1 && xdg-open http://localhost:54324) || true

sites-reset:
	@[[ "$(CONFIRM)" == "reset" ]] || { \
		printf 'sites-reset remove containers e volumes da stack local. Reexecute com CONFIRM=reset.\n' >&2; \
		exit 2; \
	}
	@$(MAKE) --no-print-directory reset ENVIRONMENT=local CONFIRM=reset

## Backward-compatible local aliases ----------------------------------------

run:
	@$(MAKE) --no-print-directory up ENVIRONMENT=local SERVICE="$(SERVICE)"

stop:
	@$(MAKE) --no-print-directory down ENVIRONMENT=local

dev-up:
	@$(MAKE) --no-print-directory up ENVIRONMENT=local SERVICE="$(SERVICE)"

dev-down:
	@$(MAKE) --no-print-directory down ENVIRONMENT=local

local-run:
	@$(MAKE) --no-print-directory up ENVIRONMENT=local SERVICE="$(SERVICE)"

local-stop:
	@$(MAKE) --no-print-directory down ENVIRONMENT=local

local-logs:
	@$(MAKE) --no-print-directory logs ENVIRONMENT=local SERVICE="$(SERVICE)" TAIL="$(TAIL)" FOLLOW="$(FOLLOW)"

restart-service:
	@$(MAKE) --no-print-directory restart ENVIRONMENT=local SERVICE="$(SERVICE)"

run-frontend:
	@$(MAKE) --no-print-directory up ENVIRONMENT=local SERVICE=toka-frontend

# The AI chain is part of the supported stack; these names remain aliases.
build-experimental:
	@$(MAKE) --no-print-directory build ENVIRONMENT=local

run-experimental:
	@$(MAKE) --no-print-directory up ENVIRONMENT=local

## Supabase CLI helpers -------------------------------------------------------

supabase-local-start:
	@printf 'Starting local Supabase stack...\n'
	@mkdir -p "$(SUPABASE_HOME)"
	$(SUPABASE) start

supabase-local-stop:
	@printf 'Stopping local Supabase stack...\n'
	@mkdir -p "$(SUPABASE_HOME)"
	$(SUPABASE) stop

supabase-local-status:
	@printf 'Supabase local status:\n'
	@mkdir -p "$(SUPABASE_HOME)"
	$(SUPABASE) status

# Aplica as migrations pendentes de supabase/migrations no banco local já
# iniciado (o `supabase start` só aplica na primeira inicialização do volume).
supabase-migration-up:
	@printf 'Aplicando migrations locais do Supabase...\n'
	@mkdir -p "$(SUPABASE_HOME)"
	$(SUPABASE) migration up

## Documentation and coverage ------------------------------------------------

swagger-all:
	@printf 'Generating Swagger docs for all services...\n'
	@cd toca-auth && make swagger
	@cd toca-comm && make swagger
	@cd toca-core && make swagger
	@cd toca-crm && make swagger
	@cd toca-deal && make swagger
	@cd toca-financial && make swagger
	@cd toca-ops && make swagger
	@cd toca-calendar && make swagger
	@printf 'Done!\n'

test-coverage:
	@set -eu; : > coverage-summary.txt; \
	for mod in $(GO_MODULES); do \
		printf '==> %s\n' "$$mod"; \
		(cd "$$mod" && GOWORK=off go test -coverprofile=cover.out -covermode=atomic ./...); \
		pct=$$(cd "$$mod" && GOWORK=off go tool cover -func=cover.out | awk '/^total:/ { gsub("%", "", $$3); print $$3 }'); \
		test -n "$$pct"; \
		printf '%s %s\n' "$$mod" "$$pct" >> coverage-summary.txt; \
	done
	@printf '\nPer-module coverage:\n'
	@cat coverage-summary.txt

test-coverage-gate: test-coverage
	@set -eu; \
	while read -r mod actual; do \
		bash scripts/check-coverage-baseline.sh "$$mod" "$$actual" "$(COVERAGE_BASELINES)"; \
	done < coverage-summary.txt
