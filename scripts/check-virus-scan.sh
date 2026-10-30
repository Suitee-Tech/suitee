#!/usr/bin/env bash
# check-virus-scan (P1-13): o funil de uploads é fail-closed no código —
# sem clamd no compose de produção e CLAMAV_URL no toca-deal, TODO upload
# seria recusado em runtime. Este gate trava o wiring no lugar.
set -euo pipefail

repo_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd -P)"
failures=0

fail() {
  printf '[check-virus-scan] %s\n' "$1" >&2
  failures=$((failures + 1))
}

# 1. O serviço clamav precisa existir no compose de produção com a porta 3310.
if ! grep -qE '^\s+clamav:\s*$' "$repo_dir/docker-compose.prod.yml"; then
  fail "docker-compose.prod.yml não declara o serviço clamav"
fi
if ! grep -q 'clamav/clamav:stable@sha256:' "$repo_dir/docker-compose.prod.yml"; then
  fail "clamav no compose de produção não está pinado por digest (P1-16)"
fi
if ! awk '/^  [a-z0-9-]+:\s*$/ { inblk=0 } /^  clamav:\s*$/ { inblk=1 } inblk' "$repo_dir/docker-compose.prod.yml" | grep -q '3310'; then
  fail "serviço clamav não expõe a porta 3310 no compose de produção"
fi

# 2. O bloco do toca-deal precisa declarar CLAMAV_URL apontando para o clamd.
if ! awk '/^  [a-z0-9-]+:\s*$/ { inblk=0 } /^  toca-deal:\s*$/ { inblk=1 } inblk' "$repo_dir/docker-compose.prod.yml" | grep -q 'CLAMAV_URL=clamav:3310'; then
  fail "bloco do toca-deal no compose de produção não declara CLAMAV_URL=clamav:3310"
fi

# 3. O scanner é injetado no router do toca-deal.
if ! grep -q 'virusscan.NewClamAVScanner' "$repo_dir/toca-deal/internal/api/v1/routes/routes.go"; then
  fail "routes.go do toca-deal não injeta o clamd scanner"
fi

# 4. O Upload do DocumentService recusa scanner ausente (fail-closed).
if ! grep -q 'virus scanner not configured for uploads' "$repo_dir/toca-deal/internal/services/document_service.go"; then
  fail "DocumentService.Upload não é fail-closed sem scanner"
fi

if (( failures > 0 )); then
  printf '[check-virus-scan] %d violação(ões) — uploads seriam recusados em runtime\n' "$failures" >&2
  exit 1
fi

printf '[check-virus-scan] clamd no compose + CLAMAV_URL + fail-closed no upload\n'
