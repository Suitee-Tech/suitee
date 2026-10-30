#!/usr/bin/env bash
# Stack local dos Sites das imobiliárias (ADR 0006).
#
# Interface usada pelos alvos `make sites-*`. Mantém o fluxo Docker existente
# (`make up`) e adiciona o que é específico da vitrine: checagem de portas,
# resolução de `{marca}.imoveis.localhost`, smoke de saúde do Go/Next e
# instruções de /etc/hosts.
#
# Compatível com Bash 3.2 (macOS): sem arrays associativos, sem mapfile.
set -euo pipefail

script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd -P)"
repo_dir="$(cd "$script_dir/../.." && pwd -P)"
cd "$repo_dir"

command_name="${1:-help}"
env_file="${ENV_FILE:-.env}"
slug="${SITES_SLUG:-Exemplo}"
panel="${SITES_PANEL:-1}"
apply_hosts="${SITES_HOSTS_APPLY:-0}"
supabase_local="${SUPABASE_LOCAL:-1}"
tail_lines="${TAIL:-200}"

die() {
  printf '[sites] ERRO: %s\n' "$*" >&2
  exit 2
}

note() {
  printf '[sites] %s\n' "$*"
}

dotenv_value() {
  local key="$1"
  local fallback="$2"
  local value=""
  if [[ -f "$env_file" ]]; then
    value="$(grep -E "^${key}=" "$env_file" 2>/dev/null | tail -1 | cut -d= -f2- || true)"
  fi
  # Remove aspas simples/duplas ao redor do valor.
  value="${value%\"}"
  value="${value#\"}"
  value="${value%\'}"
  value="${value#\'}"
  if [[ -z "$value" ]]; then
    printf '%s' "$fallback"
  else
    printf '%s' "$value"
  fi
}

web_port="$(dotenv_value WEB_PORT 3002)"
sites_port="$(dotenv_value SITES_PORT 8094)"
frontend_port="$(dotenv_value FRONTEND_PORT 3000)"
grafana_port="$(dotenv_value GRAFANA_PORT 3001)"
suffix="$(dotenv_value PUBLIC_SITE_HOST_SUFFIX imoveis.localhost)"
host="${slug}.${suffix}"

scheme="http"
public_port="$web_port"
if [[ "$suffix" != *.localhost ]]; then
  scheme="https"
  public_port=""
fi
if [[ -n "$public_port" ]]; then
  public_url="${scheme}://${host}:${public_port}"
  web_url="http://localhost:${web_port}"
else
  public_url="${scheme}://${host}"
  web_url="http://localhost:${web_port}"
fi
admin_url="${web_url}/admin"
panel_url="http://localhost:${frontend_port}/site"
api_url="http://localhost:${sites_port}"

compose() {
  docker compose --env-file "$env_file" -f docker-compose.yml --profile frontend "$@"
}

require_env_file() {
  [[ -f "$env_file" ]] || {
    printf '[sites] ERRO: %s não existe.\n' "$env_file" >&2
    printf '[sites] Crie a configuração local: cp .env.example .env\n' >&2
    exit 2
  }
}

require_docker() {
  command -v docker >/dev/null 2>&1 || die "docker não encontrado no PATH"
  docker info >/dev/null 2>&1 || die "Docker daemon não está rodando (abra o Docker Desktop)"
}

check_ports() {
  local conflict=""
  local payload_public_url
  payload_public_url="$(dotenv_value PAYLOAD_PUBLIC_SERVER_URL "")"
  payload_public_url="${payload_public_url%/}"
  [[ "$web_port" != "$frontend_port" ]] || conflict="FRONTEND_PORT=${frontend_port}"
  [[ "$web_port" != "$grafana_port" ]] || conflict="${conflict:+$conflict, }GRAFANA_PORT=${grafana_port}"
  [[ -z "$conflict" ]] || {
    die "WEB_PORT=${web_port} colide com ${conflict}. Ajuste WEB_PORT no ${env_file} (o padrão é 3002)."
  }
  if [[ -n "$payload_public_url" && "$payload_public_url" != "http://localhost:${web_port}" ]]; then
    die "PAYLOAD_PUBLIC_SERVER_URL=${payload_public_url} não corresponde a WEB_PORT=${web_port}. Ajuste para http://localhost:${web_port}."
  fi
}

host_resolves() {
  python3 - "$host" <<'PY' >/dev/null 2>&1
import socket
import sys

try:
    addresses = socket.getaddrinfo(sys.argv[1], None)
except OSError:
    raise SystemExit(1)
raise SystemExit(0 if addresses else 1)
PY
}

print_hosts_help() {
  if host_resolves; then
    return 0
  fi
  printf '\n'
  note "O navegador precisa resolver ${host}."
  printf '  Adicione ao /etc/hosts (uma vez por marca):\n'
  printf '    sudo sh -c "printf %s >> /etc/hosts"\n' "'\\n127.0.0.1 ${host}\\n'"
  printf '  Ou rode: SITES_HOSTS_APPLY=1 make sites-hosts SITES_SLUG=%s\n' "$slug"
}

apply_hosts_if_requested() {
  if [[ "$apply_hosts" != "1" ]]; then
    return 0
  fi
  if host_resolves; then
    note "${host} já resolve."
    return 0
  fi
  note "adicionando ${host} a /etc/hosts (sudo)"
  sudo sh -c "grep -qE '[[:space:]]${host}([[:space:]]|$)' /etc/hosts || printf '\n127.0.0.1 ${host}\n' >> /etc/hosts"
}

wait_http() {
  local url="$1"
  local label="$2"
  local attempts=40
  local i=0
  while (( i < attempts )); do
    if curl -fsS --max-time 2 "$url" >/dev/null 2>&1; then
      note "${label}: ok (${url})"
      return 0
    fi
    i=$((i + 1))
    sleep 1
  done
  printf '[sites] AVISO: %s não respondeu em %ss: %s\n' "$label" "$attempts" "$url" >&2
  return 1
}

print_urls() {
  printf '\n'
  note "Toka (painel):     ${panel_url}"
  note "Payload Admin:     ${admin_url}"
  note "Site público:      ${public_url}"
  note "toca-sites API:    ${api_url}"
  printf '\n'
  note "Marca/tenant usado no preview: ${slug}"
  print_hosts_help
}

check() {
  require_env_file
  require_docker
  check_ports
  if [[ "$supabase_local" == "1" ]] && ! command -v supabase >/dev/null 2>&1; then
    die "SUPABASE_LOCAL=1 exige a Supabase CLI no PATH (ou use SUPABASE_LOCAL=0 com banco já configurado)"
  fi
  note "env: ${env_file}"
  note "web_port=${web_port} sites_port=${sites_port} host=${host} suffix=${suffix}"
  if host_resolves; then
    note "${host} resolve localmente."
  else
    note "${host} ainda não resolve — o smoke usa --resolve; o navegador precisa de /etc/hosts."
  fi
  note "checagem concluída."
}

up() {
  require_env_file
  require_docker
  check_ports
  apply_hosts_if_requested

  # O `supabase start` só aplica migrations ao inicializar o volume. Um
  # checkout existente precisa do `migration up` para receber o schema sites.*.
  if [[ "$supabase_local" == "1" ]]; then
    note "iniciando Supabase local..."
    make --no-print-directory supabase-local-start
    note "aplicando migrations pendentes do Supabase..."
    make --no-print-directory supabase-migration-up
  fi

  local make_args=(ENVIRONMENT=local SUPABASE_LOCAL=0 ENV_FILE="$env_file")
  if [[ "$panel" == "1" ]]; then
    note "subindo stack local completa (Toka + gateway + toca-sites + toca-web)..."
    make --no-print-directory up "${make_args[@]}"
  else
    note "subindo apenas o site público (toca-web + toca-sites + core/crm/rabbit)..."
    make --no-print-directory up "${make_args[@]}" SERVICE=toca-web
  fi
  print_urls
}

down() {
  require_env_file
  require_docker
  note "derrubando a stack local (sem remover volumes)..."
  make --no-print-directory down ENVIRONMENT=local SUPABASE_LOCAL="$supabase_local" ENV_FILE="$env_file"
}

status() {
  require_env_file
  require_docker
  compose ps toca-web toca-web-migrate toca-sites toka-frontend gateway 2>/dev/null || compose ps
}

logs() {
  require_env_file
  require_docker
  compose logs --follow --tail "$tail_lines" toca-web toca-sites
}

migrate() {
  require_env_file
  require_docker
  note "aplicando migrations versionadas do Payload (schema payload)..."
  compose build toca-web-migrate
  compose run --rm --no-deps toca-web-migrate
}

smoke() {
  require_env_file
  require_docker
  local failures=0

  wait_http "${api_url}/health" "toca-sites /health" || failures=$((failures + 1))
  wait_http "${web_url}/api/health" "toca-web /api/health" || failures=$((failures + 1))

  local public_status
  public_status="$(curl -sS --max-time 5 --resolve "${host}:${web_port}:127.0.0.1" \
    -o /dev/null -w '%{http_code}' "${public_url}/" 2>/dev/null || true)"
  case "$public_status" in
    200)
      note "site público: ok (200 em ${public_url}/)"
      ;;
    404)
      note "site público: host ok, mas nenhum site configurado ainda (404 esperado até criar o tenant em ${panel_url})"
      ;;
    000|"")
      note "site público: FALHOU (sem conexão em ${public_url}/)"
      failures=$((failures + 1))
      ;;
    *)
      note "site público: HTTP ${public_status} em ${public_url}/"
      ;;
  esac

  # O admin do Payload compila/abre o bundle no primeiro hit e pode passar de
  # 5s; tenta algumas vezes em vez de marcar a stack como quebrada.
  local admin_status=""
  local admin_attempt
  for admin_attempt in 1 2 3; do
    admin_status="$(curl -sS --max-time 15 -o /dev/null -w '%{http_code}' "${admin_url}" 2>/dev/null || true)"
    case "$admin_status" in
      200|301|302|307|308) break ;;
    esac
    sleep 2
  done
  case "$admin_status" in
    200|301|302|307|308)
      note "Payload Admin: ok (${admin_status} em ${admin_url})"
      ;;
    *)
      note "Payload Admin: HTTP ${admin_status:-sem resposta} em ${admin_url}"
      if [[ "$admin_status" == "000" || -z "$admin_status" ]]; then
        failures=$((failures + 1))
      fi
      ;;
  esac

  if (( failures > 0 )); then
    die "smoke falhou (${failures} verificação(ões)). Veja: make sites-logs"
  fi
  note "smoke concluído."
}

urls() {
  printf '%s\n' "$public_url"
  printf 'web=%s\n' "$web_url"
  printf 'admin=%s\n' "$admin_url"
  printf 'api=%s\n' "$api_url"
  printf 'panel=%s\n' "$panel_url"
  printf 'mail=http://localhost:54324\n'
}

hosts() {
  if host_resolves; then
    note "${host} já resolve."
    return 0
  fi
  apply_hosts_if_requested
  print_hosts_help
  command -v python3 >/dev/null 2>&1 || note "python3 não encontrado; valide o host manualmente."
}

usage() {
  cat <<'USAGE'
Uso: scripts/local/sites.sh <comando>

Comandos:
  check          valida .env, Docker, portas e resolução de {marca}.imoveis.localhost
  up             sobe a stack local (SITES_PANEL=1) ou só o site público (SITES_PANEL=0)
  down           derruba a stack local sem remover volumes
  status         mostra os containers da vitrine/gateway/frontend
  logs           acompanha logs de toca-web e toca-sites
  smoke          valida /health, /api/health, host público e Payload Admin
  urls           imprime as URLs locais
  hosts          mostra/instala a linha de /etc/hosts para a marca
  migrate        aplica as migrations do Payload (toca-web-migrate)
  help           esta ajuda

Variáveis:
  SITES_SLUG        marca usada no preview (default: Exemplo)
  SITES_PANEL       1 sobe a stack completa; 0 sobe só o site público (default: 1)
  SITES_HOSTS_APPLY 1 tenta adicionar a linha em /etc/hosts via sudo (default: 0)
  ENV_FILE          dotenv local (default: .env)
  SUPABASE_LOCAL    1 usa Supabase CLI no local (default: 1)
USAGE
}

case "$command_name" in
  check) check ;;
  up) up ;;
  down) down ;;
  status) status ;;
  logs) logs ;;
  smoke) smoke ;;
  urls) urls ;;
  hosts) hosts ;;
  migrate) migrate ;;
  help|-h|--help) usage ;;
  *) usage; exit 2 ;;
esac
