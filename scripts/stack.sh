#!/usr/bin/env bash
set -euo pipefail

# Canonical local/build interface for Suitee. Keep this script compatible with
# macOS' Bash 3.2: indexed arrays are used, but associative arrays/mapfile are
# intentionally avoided.

script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd -P)"
repo_dir="$(cd "$script_dir/.." && pwd -P)"
cd "$repo_dir"

die() {
  printf '[stack] ERRO: %s\n' "$*" >&2
  exit 2
}

note() {
  printf '[stack:%s] %s\n' "$environment" "$*"
}

usage() {
  cat <<'USAGE'
Uso: scripts/stack.sh <comando>

Comandos de ambiente:
  doctor         valida ferramentas e configuração do ambiente
  config         valida o Compose real sem imprimir segredos
  build          constrói imagens próprias sequencialmente
  up             sobe local/test com --no-build --wait
  down           encerra local/test sem remover volumes
  status|ps      mostra o estado dos containers local/test
  logs           acompanha logs local/test (FOLLOW=0 desativa -f)
  restart        reinicia local/test; SERVICE limita o serviço
  reset          remove stack e volumes local/test; requer CONFIRM=reset
Checks e testes:
  check-compose  valida estruturalmente os ambientes local e test
  check-shell    executa bash -n em todos os scripts shell
  test-go        executa testes do módulo informado em MODULE
  test-backend   executa testes de todos os módulos Go
  lint-go        executa golangci-lint no módulo informado em MODULE

Variáveis:
  ENVIRONMENT=local|test  (ENV é aceito como alias)
  ENV_FILE=<arquivo>      sobrescreve o dotenv padrão
  SERVICE=<serviço>       limita build/up/status/logs/restart
  IMAGE_TAG=<sha completa>
  WAIT_TIMEOUT=<segundos>           padrão: 300
  TAIL=<linhas|all>                 padrão: 200
  FOLLOW=0                          não acompanha logs continuamente
  SUPABASE_LOCAL=0                  não exige Supabase CLI no doctor local
USAGE
}

command_name="${1:-help}"
environment="${ENVIRONMENT:-${ENV:-local}}"
service="${SERVICE:-}"
image_tag="${IMAGE_TAG:-}"
env_file="${ENV_FILE:-}"
wait_timeout="${WAIT_TIMEOUT:-300}"
tail_lines="${TAIL:-200}"
follow_logs="${FOLLOW:-1}"
confirm_reset="${CONFIRM:-}"
supabase_local="${SUPABASE_LOCAL:-1}"

case "$environment" in
  local|test) ;;
  *) die "ENVIRONMENT deve ser local ou test: $environment" ;;
esac

compose_files=()
compose_args=()

configure_environment() {
  compose_files=()
  compose_args=()

  case "$environment" in
    local)
      if [[ -z "$env_file" ]]; then
        env_file=".env"
      fi
      compose_files=(docker-compose.yml)
      compose_args=(-f docker-compose.yml --profile frontend)
      ;;
    test)
      if [[ -z "$env_file" ]]; then
        if [[ -f .env.test ]]; then
          env_file=".env.test"
        else
          env_file=".env.test.example"
        fi
      fi
      compose_files=(
        docker-compose.yml
        docker-compose.test.yml
      )
      compose_args=(
        -f docker-compose.yml
        -f docker-compose.test.yml
        --profile local-db
        --profile frontend
      )
      ;;
  esac
}

configure_environment

require_command() {
  command -v "$1" >/dev/null 2>&1 || die "comando obrigatório ausente: $1"
}

require_file() {
  [[ -f "$1" && -r "$1" ]] || die "arquivo ausente ou ilegível: $1"
}

report_version() {
  local tool="$1"
  local actual="$2"
  local expected="$3"

  if [[ "$actual" == "$expected" ]]; then
    printf '[doctor] %s=%s\n' "$tool" "$actual"
  else
    printf '[doctor] AVISO: %s=%s; versão de referência=%s\n' \
      "$tool" "$actual" "$expected" >&2
  fi
}

require_local_runtime() {
  case "$environment" in
    local|test) ;;
    *) die "$command_name só pode operar containers em ENVIRONMENT=local ou test" ;;
  esac
}

resolve_image_tag() {
  if [[ -z "$image_tag" ]]; then
    require_command git
    image_tag="$(git rev-parse --verify 'HEAD^{commit}')"
  fi
  case "$image_tag" in
    *[!0-9a-f]*|'') die "IMAGE_TAG deve ser uma SHA Git completa" ;;
  esac
  case "${#image_tag}" in
    40|64) ;;
    *) die "IMAGE_TAG deve ter 40 ou 64 caracteres hexadecimais" ;;
  esac
}

run_compose() {
  IMAGE_TAG="$image_tag" docker compose \
    --env-file "$env_file" \
    "${compose_args[@]}" \
    "$@"
}

validate_compose() {
  require_command docker
  require_file "$env_file"
  run_compose config --quiet
}

service_exists() {
  local wanted="$1"
  local candidate
  while IFS= read -r candidate; do
    if [[ "$candidate" == "$wanted" ]]; then
      return 0
    fi
  done < <(run_compose config --services)
  return 1
}

validate_optional_service() {
  if [[ -n "$service" ]]; then
    service_exists "$service" || die "serviço não existe no ambiente $environment: $service"
  fi
}

is_local_owned_service() {
  case "$1" in
    gateway|toka-frontend|toca-*) return 0 ;;
    *) return 1 ;;
  esac
}

build_with_dependencies_sequentially() {
  local wanted="$1"
  local build_plan
  local build_target
  local build_count=0
  local build_targets=()

  build_plan="$(run_compose build --print --with-dependencies "$wanted")" ||
    die "não foi possível resolver dependências de build para $wanted"
  while IFS= read -r build_target; do
    [[ -n "$build_target" ]] || continue
    build_targets[${#build_targets[@]}]="$build_target"
  done < <(
    printf '%s\n' "$build_plan" |
      sed -n '/^  "target": {$/,/^  }$/s/^    "\([^"]*\)": {$/\1/p'
  )

  for build_target in "${build_targets[@]}"; do
    build_count=$((build_count + 1))
    note "build sequencial: $build_target"
    run_compose build "$build_target"
  done

  (( build_count > 0 )) ||
    die "nenhuma imagem própria foi resolvida para $wanted"
}

build_images() {
  local candidate
  local candidate_image
  local selected=0

  validate_compose
  validate_optional_service

  if [[ -n "$service" ]]; then
    case "$environment" in
      local|test)
        if ! is_local_owned_service "$service"; then
          note "baixando imagem externa de $service"
          run_compose pull "$service"
          return
        fi
        ;;
    esac

    note "build de $service e das dependências próprias"
    build_with_dependencies_sequentially "$service"
    note "build direcionado concluído"
    return
  fi

  while IFS= read -r candidate; do
    [[ -n "$candidate" ]] || continue

    case "$environment" in
      local|test)
        is_local_owned_service "$candidate" || continue
        ;;
    esac

    selected=$((selected + 1))
    note "build sequencial: $candidate"
    run_compose build "$candidate"
  done < <(run_compose config --services)

  (( selected > 0 )) || die "nenhum serviço próprio selecionado para build"
  note "$selected imagem(ns) própria(s) construída(s)"
}

up_stack() {
  require_local_runtime
  case "$wait_timeout" in
    *[!0-9]*|'') die "WAIT_TIMEOUT deve ser um inteiro positivo" ;;
  esac
  (( wait_timeout > 0 )) || die "WAIT_TIMEOUT deve ser maior que zero"

  validate_compose
  validate_optional_service
  note "subindo containers e aguardando readiness por até ${wait_timeout}s"
  if [[ -n "$service" ]]; then
    run_compose up --detach --no-build --wait --wait-timeout "$wait_timeout" "$service"
  else
    run_compose up --detach --no-build --wait --wait-timeout "$wait_timeout"
  fi
  run_compose ps
}

down_stack() {
  require_local_runtime
  validate_compose
  [[ -z "$service" ]] || die "SERVICE não se aplica a down; use restart ou docker compose stop"
  note "encerrando containers sem remover volumes"
  run_compose down --remove-orphans
}

show_status() {
  require_local_runtime
  validate_compose
  validate_optional_service
  if [[ -n "$service" ]]; then
    run_compose ps "$service"
  else
    run_compose ps
  fi
}

show_logs() {
  require_local_runtime
  validate_compose
  validate_optional_service
  case "$tail_lines" in
    all) ;;
    *[!0-9]*|'') die "TAIL deve ser um inteiro não negativo ou all" ;;
  esac

  if [[ "$follow_logs" == "0" || "$follow_logs" == "false" ]]; then
    if [[ -n "$service" ]]; then
      run_compose logs --tail "$tail_lines" "$service"
    else
      run_compose logs --tail "$tail_lines"
    fi
  else
    if [[ -n "$service" ]]; then
      run_compose logs --follow --tail "$tail_lines" "$service"
    else
      run_compose logs --follow --tail "$tail_lines"
    fi
  fi
}

restart_stack() {
  require_local_runtime
  validate_compose
  validate_optional_service
  if [[ -n "$service" ]]; then
    note "reiniciando $service"
    run_compose restart "$service"
  else
    note "reiniciando todos os serviços"
    run_compose restart
  fi
}

reset_stack() {
  require_local_runtime
  [[ "$confirm_reset" == "reset" ]] || {
    die "reset remove volumes; confirme explicitamente com CONFIRM=reset"
  }
  validate_compose
  [[ -z "$service" ]] || die "SERVICE não se aplica a reset"
  note "removendo containers, órfãos e volumes"
  run_compose down --volumes --remove-orphans
}

doctor() {
  local actual_version

  require_command bash
  require_command docker
  require_command git
  require_command make
  docker compose version >/dev/null
  docker info >/dev/null 2>&1 || die "Docker daemon não está acessível"

  if [[ "$environment" == "local" && "$supabase_local" != "0" ]]; then
    require_command supabase
    require_file supabase/config.toml
  fi

  printf '[doctor] ambiente=%s\n' "$environment"
  printf '[doctor] env_file=%s\n' "$env_file"
  printf '[doctor] %s\n' "$(docker compose version)"
  printf '[doctor] %s\n' "$(git --version)"

  if command -v go >/dev/null 2>&1; then
    actual_version="$(go env GOVERSION)"
    report_version go "$actual_version" "go1.25.2"
  fi
  if command -v node >/dev/null 2>&1; then
    actual_version="$(node --version)"
    report_version node "$actual_version" "v24.22.1"
  fi
  if command -v pnpm >/dev/null 2>&1; then
    actual_version="$(pnpm --version)"
    report_version pnpm "$actual_version" "10.15.0"
  fi
  if [[ "$environment" == "local" && "$supabase_local" != "0" ]]; then
    mkdir -p "$repo_dir/.supabase-home"
    actual_version="$(HOME="$repo_dir/.supabase-home" SUPABASE_DISABLE_TELEMETRY=1 supabase --version)"
    report_version supabase "$actual_version" "2.107.0"
  fi

  if [[ -f "$env_file" ]]; then
    validate_compose
    has_build_env=true
    printf '[doctor] Compose válido para build/config\n'
    if grep -Eq '^[A-Za-z_][A-Za-z0-9_]*=.*\.\.\.' "$env_file"; then
      printf '[doctor] AVISO: %s ainda contém valores com reticências/placeholders\n' "$env_file" >&2
    fi
  elif [[ "$environment" == "local" || "$environment" == "test" ]]; then
    die "arquivo de ambiente obrigatório ausente: $env_file"
  else
    printf '[doctor] AVISO: %s ausente; build local desse ambiente não está pronto\n' "$env_file" >&2
  fi

  printf '[doctor] OK\n'
}

active_temp_env=""
active_temp_vars=""

cleanup_temporary_files() {
  if [[ -n "$active_temp_env" ]]; then
    rm -f "$active_temp_env"
  fi
  if [[ -n "$active_temp_vars" ]]; then
    rm -f "$active_temp_vars"
  fi
}

trap cleanup_temporary_files EXIT
trap 'cleanup_temporary_files; exit 130' HUP INT TERM

synthetic_value() {
  case "$1" in
    IMAGE_TAG) printf '%s' '0000000000000000000000000000000000000000' ;;
    GITHUB_REPOSITORY) printf '%s' 'suitee/local' ;;
    COMPOSE_PROJECT_NAME) printf '%s' "suitee-check-$environment" ;;
    DATABASE_URI|SUPABASE_DB_URL)
      printf '%s' 'postgresql://tenant:check@db.example.test:5432/postgres?sslmode=require'
      ;;
    DATABASE_URI_SERVICE)
      printf '%s' 'postgresql://service:check@db.example.test:5432/postgres?sslmode=require'
      ;;
    MIGRATION_DATABASE_URI)
      printf '%s' 'postgresql://migration:check@db.example.test:5432/postgres?sslmode=require'
      ;;
    OCR_CPU_LIMIT) printf '%s' '1.0' ;;
    OCR_MEMORY_LIMIT) printf '%s' '1g' ;;
    # Limite de memória do clamd (container): o Compose valida o formato do
    # valor, então o placeholder sintético também precisa ser um tamanho.
    CLAMAV_MEMORY_LIMIT) printf '%s' '1g' ;;
    *URL|*_URL|*URI|*_URI)
      printf '%s' 'https://service.example.test'
      ;;
    *PORT|*_PORT) printf '%s' '19000' ;;
    *) printf '%s' 'check-value' ;;
  esac
}

create_synthetic_env() {
  local compose_file
  local variable_name

  active_temp_env="$(mktemp "${TMPDIR:-/tmp}/suitee-compose-check.XXXXXX")"
  active_temp_vars="${active_temp_env}.vars"
  : > "$active_temp_vars"

  for compose_file in "${compose_files[@]}"; do
    require_file "$compose_file"
    grep -Eho '\$\{[A-Za-z_][A-Za-z0-9_]*' "$compose_file" 2>/dev/null || true
  done | sed 's/^${//' | LC_ALL=C sort -u > "$active_temp_vars"

  : > "$active_temp_env"
  while IFS= read -r variable_name; do
    [[ -n "$variable_name" ]] || continue
    printf '%s=%s\n' "$variable_name" "$(synthetic_value "$variable_name")" \
      >> "$active_temp_env"
  done < "$active_temp_vars"
}

check_compose_structure() {
  local checked_environment
  local unpinned_image

  # P1-16: imagem de infraestrutura (não-ghcr) precisa de digest — tags
  # flutuantes (latest, minor) mudam de bits entre ambientes. As imagens
  # ghcr.io/... são buildadas pela release com o próprio freeze de digest.
  while IFS= read -r unpinned_image; do
    [[ -n "$unpinned_image" ]] || continue
    die "imagem de infraestrutura sem digest (P1-16): $unpinned_image"
  done < <(grep -hE '^\s+image:\s+' \
    docker-compose.yml docker-compose.test.yml 2>/dev/null \
    | sed -E 's/^\s*image:\s*//' \
    | grep -vE 'ghcr\.io|@sha256:' || true)

  require_command docker
  for checked_environment in local test; do
    environment="$checked_environment"
    env_file=""
    image_tag="0000000000000000000000000000000000000000"
    configure_environment
    create_synthetic_env
    env_file="$active_temp_env"
    note "validando estrutura Compose"
    GITHUB_REPOSITORY=suitee/local run_compose config --quiet
    rm -f "$active_temp_env" "$active_temp_vars"
    active_temp_env=""
    active_temp_vars=""
  done
  printf '[stack] Compose válido em local e test (imagens de infra com digest)\n'
}

check_shell_syntax() {
  local shell_script
  local script_count=0

  while IFS= read -r shell_script; do
    [[ -n "$shell_script" ]] || continue
    bash -n "$shell_script"
    script_count=$((script_count + 1))
  done < <(find scripts -type f -name '*.sh' -print | LC_ALL=C sort)

  (( script_count > 0 )) || die "nenhum script shell encontrado"
  printf '[stack] sintaxe válida em %d script(s) shell\n' "$script_count"
}

go_modules="pkg toca-auth toca-core toca-crm toca-deal toca-comm toca-financial toca-signature toca-ops toca-calendar toca-portal toca-sites toca-mcp toca-ai gateway"

validate_go_module() {
  local requested="$1"
  local candidate
  for candidate in $go_modules; do
    if [[ "$candidate" == "$requested" ]]; then
      return 0
    fi
  done
  die "MODULE inválido ou ausente: ${requested:-<vazio>}"
}

test_go_module() {
  local requested="${MODULE:-}"
  validate_go_module "$requested"
  require_command go
  printf '[test-go] %s\n' "$requested"
  (cd "$requested" && GOWORK=off go test -coverprofile=cover.out -covermode=atomic ./...)
}

test_all_backends() {
  local candidate
  require_command go
  for candidate in $go_modules; do
    printf '[test-backend] %s\n' "$candidate"
    (cd "$candidate" && GOWORK=off go test -coverprofile=cover.out -covermode=atomic ./...)
  done
}

lint_go_module() {
  local requested="${MODULE:-}"
  validate_go_module "$requested"
  require_command golangci-lint
  printf '[lint-go] %s\n' "$requested"
  (cd "$requested" && GOWORK=off golangci-lint run ./... --timeout="${GOLANGCI_TIMEOUT:-5m}")
}

case "$command_name" in
  help|-h|--help) usage ;;
  doctor) doctor ;;
  config) validate_compose; note "Compose válido" ;;
  build) build_images ;;
  up) up_stack ;;
  down) down_stack ;;
  status|ps) show_status ;;
  logs) show_logs ;;
  restart) restart_stack ;;
  reset) reset_stack ;;
  check-compose) check_compose_structure ;;
  check-shell) check_shell_syntax ;;
  test-go) test_go_module ;;
  test-backend) test_all_backends ;;
  lint-go) lint_go_module ;;
  *) usage >&2; die "comando desconhecido: $command_name" ;;
esac
