#!/usr/bin/env bash
set -euo pipefail

repo_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd -P)"
env_file="${TEST_STACK_ENV_FILE:-$repo_dir/.env.test.example}"
wait_timeout="${TEST_STACK_WAIT_TIMEOUT_SECONDS:-360}"
gateway_url="${TEST_STACK_GATEWAY_URL:-http://127.0.0.1:18080}"
frontend_url="${TEST_STACK_FRONTEND_URL:-http://127.0.0.1:13000}"
test_project="${TEST_STACK_PROJECT_NAME:-suitee-test-$$}"
export COMPOSE_PARALLEL_LIMIT="${COMPOSE_PARALLEL_LIMIT:-1}"
# Authentication is isolated too: never consult the developer's Supabase.
export SUPABASE_URL="http://test-auth:9000"
export SUPABASE_JWKS_URL="http://test-auth:9000/auth/v1/.well-known/jwks.json"

[[ -f "$env_file" ]] || {
  printf '[test-stack] arquivo de ambiente ausente: %s\n' "$env_file" >&2
  exit 2
}
[[ "$wait_timeout" =~ ^[1-9][0-9]*$ ]] || {
  printf '[test-stack] TEST_STACK_WAIT_TIMEOUT_SECONDS deve ser inteiro positivo\n' >&2
  exit 2
}

compose=(
  docker compose
  --project-name "$test_project"
  --project-directory "$repo_dir"
  --env-file "$env_file"
  -f "$repo_dir/docker-compose.yml"
  -f "$repo_dir/docker-compose.test.yml"
  --profile local-db
  --profile frontend
)
infra_services=(
  test-auth
  postgres
  redis
  rabbitmq
  minio
  gotenberg
  jaeger
)

print_failure_diagnostics() {
  local container_id
  local container_state
  local health_state
  local service_name
  local failing_services=()

  printf '[test-stack] falha; estado final dos containers:\n' >&2
  "${compose[@]}" ps --all >&2

  while IFS= read -r container_id; do
    [[ -n "$container_id" ]] || continue
    container_state="$(docker inspect --format '{{.State.Status}}' "$container_id" 2>/dev/null || true)"
    health_state="$(docker inspect --format '{{if .State.Health}}{{.State.Health.Status}}{{end}}' "$container_id" 2>/dev/null || true)"
    if [[ "$container_state" == "running" && "$health_state" != "unhealthy" ]]; then
      continue
    fi
    service_name="$(docker inspect --format '{{index .Config.Labels "com.docker.compose.service"}}' "$container_id" 2>/dev/null || true)"
    [[ -n "$service_name" ]] || continue
    failing_services[${#failing_services[@]}]="$service_name"
  done < <("${compose[@]}" ps --all --quiet)

  if (( ${#failing_services[@]} > 0 )); then
    printf '[test-stack] logs dos serviços que não ficaram prontos: %s\n' \
      "${failing_services[*]}" >&2
    "${compose[@]}" logs --no-color --tail=160 "${failing_services[@]}" >&2
  else
    printf '[test-stack] nenhum container específico identificado; exibindo logs recentes\n' >&2
    "${compose[@]}" logs --no-color --tail=40 >&2
  fi
}

cleanup() {
  local status=$?
  trap - EXIT INT TERM
  set +e
  if (( status != 0 )); then
    print_failure_diagnostics
  fi
  "${compose[@]}" down --volumes --rmi local --remove-orphans >/dev/null 2>&1
  exit "$status"
}
trap cleanup EXIT
trap 'exit 130' INT
trap 'exit 143' TERM

probe() {
  local name="$1"
  local url="$2"
  local attempts="${3:-12}"
  local delay="${4:-2}"
  local attempt

  for ((attempt = 1; attempt <= attempts; attempt++)); do
    if curl --fail --silent --show-error \
      --connect-timeout 3 --max-time 10 "$url" >/dev/null; then
      printf '[test-stack] OK %s %s\n' "$name" "$url"
      return 0
    fi
    if (( attempt < attempts )); then
      sleep "$delay"
    fi
  done

  printf '[test-stack] endpoint indisponivel: %s (%s)\n' "$name" "$url" >&2
  return 1
}

printf '[test-stack] validando configuracao Compose\n'
"${compose[@]}" config --quiet

printf '[test-stack] resolvendo imagens da aplicacao\n'
build_plan="$("${compose[@]}" build --print --with-dependencies \
  gateway toka-frontend)"
build_targets=()
while IFS= read -r service; do
  [[ -n "$service" ]] || continue
  build_targets[${#build_targets[@]}]="$service"
done < <(
  printf '%s\n' "$build_plan" |
    sed -n '/^  "target": {$/,/^  }$/s/^    "\([^"]*\)": {$/\1/p'
)
(( ${#build_targets[@]} > 0 )) || {
  printf '[test-stack] nenhuma imagem propria foi resolvida\n' >&2
  exit 2
}

printf '[test-stack] construindo %d imagem(ns) sequencialmente\n' \
  "${#build_targets[@]}"
for service in "${build_targets[@]}"; do
  printf '[test-stack] build %s\n' "$service"
  "${compose[@]}" build "$service"
done

printf '[test-stack] iniciando infraestrutura (projeto=%s)\n' "$test_project"
"${compose[@]}" up --detach --no-build --wait --wait-timeout "$wait_timeout" \
  "${infra_services[@]}"

printf '[test-stack] iniciando aplicacao e aguardando readiness (timeout=%ss)\n' \
  "$wait_timeout"
"${compose[@]}" up --detach --no-build --wait --wait-timeout "$wait_timeout" \
  gateway toka-frontend

printf '[test-stack] containers prontos\n'
"${compose[@]}" ps

probe gateway "${gateway_url%/}/health"
probe core-via-gateway "${gateway_url%/}/api/v1/health"
probe frontend "${frontend_url%/}/"
probe gateway-via-frontend "${frontend_url%/}/api/v1/health"

printf '[test-stack] preparando fixture no banco descartavel\n'
"${compose[@]}" exec --no-TTY postgres psql -U postgres -d toca_test \
  -v ON_ERROR_STOP=1 < "$repo_dir/scripts/test-stack-fixture/seed.sql"

printf '[test-stack] executando jornada com servicos reais\n'
"${compose[@]}" exec --no-TTY test-auth /fixture-server run
appointments_count="$("${compose[@]}" exec --no-TTY postgres psql -U postgres -d toca_test -tAc \
  "SELECT count(*) FROM calendar.appointments WHERE metadata->>'lead_id' = '11111111-1111-4111-8111-111111111103'")"
[[ "$appointments_count" == "1" ]] || {
  printf '[test-stack] retry criou compromissos duplicados: %s\n' "$appointments_count" >&2
  exit 1
}

# P1-08: as jornadas full-stack rodam no MESMO ambiente efêmero (Auth fixture,
# Postgres com migrations, RabbitMQ, Storage e seed idempotente) — sem o
# projeto hospedado. deal-pipeline percorre negócio→proposta→aceite→análise→
# contrato; roulette-pipeline valida config→simulação determinística→histórico.
printf '[test-stack] jornada deal-pipeline (esteira)\n'
"${compose[@]}" exec --no-TTY test-auth /fixture-server deal-pipeline
printf '[test-stack] jornada roulette-pipeline (roleta de leads)\n'
"${compose[@]}" exec --no-TTY test-auth /fixture-server roulette-pipeline

printf '[test-stack] reiniciando CRM e calendario para verificar persistencia\n'
"${compose[@]}" restart toca-crm toca-calendar
"${compose[@]}" up --detach --no-build --wait --wait-timeout "$wait_timeout" toca-crm toca-calendar
"${compose[@]}" exec --no-TTY test-auth /fixture-server verify-restart

printf '[test-stack] readiness e jornada de visitas concluidas\n'
