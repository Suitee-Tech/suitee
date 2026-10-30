#!/usr/bin/env bash
set -euo pipefail

repo_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd -P)"
cli="$repo_dir/scripts/deployment/supabase-cli/node_modules/.bin/supabase"
[[ -x "$cli" ]] || { printf 'Install the pinned CLI with npm ci --prefix scripts/deployment/supabase-cli\n' >&2; exit 2; }
qa_dir="$(mktemp -d "${TMPDIR:-/tmp}/toca-review-migrations.XXXXXX")"
qa_project="toca-review-qa-financial-$$"
output_file="${FINANCIAL_INTEGRATION_OUTPUT:-$repo_dir/financial-postgres-tests.jsonl}"
qa_compose=()

cleanup() {
  local status=$?
  trap - EXIT INT TERM
  set +e
  if [[ -n "${qa_compose[*]-}" ]]; then
    if (( status != 0 )); then "${qa_compose[@]}" logs --no-color --tail=80 >&2; fi
    "${qa_compose[@]}" down --volumes --rmi local --remove-orphans >/dev/null 2>&1
  fi
  "$cli" stop --workdir "$qa_dir" --project-id "$qa_project" --no-backup >/dev/null 2>&1
  rm -rf "$qa_dir"
  exit "$status"
}
trap cleanup EXIT
trap 'exit 130' INT
trap 'exit 143' TERM

"$cli" init --workdir "$qa_dir" --yes
python3 - "$qa_dir/supabase/config.toml" "$qa_project" <<'PY'
import re, sys
from pathlib import Path
p = Path(sys.argv[1])
s = p.read_text()
s = re.sub(r'^project_id = .*$', f'project_id = "{sys.argv[2]}"', s, flags=re.M)
s = re.sub(r'\b543(\d\d)\b', r'554\1', s)
s = re.sub(r'(\[db.seed\][\s\S]*?enabled = )true', r'\1false', s, count=1)
p.write_text(s)
PY
cp -R "$repo_dir/supabase/migrations" "$qa_dir/supabase/migrations"

printf '[financial-integration] starting disposable Supabase database with repository migrations\n'
"$cli" start --workdir "$qa_dir" --exclude gotrue,realtime,storage-api,imgproxy,kong,mailpit,postgrest,postgres-meta,studio,edge-runtime,logflare,vector,supavisor

container="supabase_db_$qa_project"
expected_migrations="$(find "$qa_dir/supabase/migrations" -name '*.sql' -type f | wc -l | tr -d ' ')"
actual_migrations="$(docker exec "$container" psql -U postgres -d postgres -tAc 'SELECT count(*) FROM supabase_migrations.schema_migrations')"
[[ "$expected_migrations" == "$actual_migrations" ]] || {
  printf '[financial-integration] incomplete migration history: expected %s, received %s\n' "$expected_migrations" "$actual_migrations" >&2
  exit 1
}

export TOCA_TEST_DATABASE_URI='postgres://postgres:postgres@127.0.0.1:55422/postgres?sslmode=disable'
export DATABASE_URI_SERVICE_TEST="$TOCA_TEST_DATABASE_URI"
cd "$repo_dir"
go test -count=1 -json ./toca-financial/internal/services ./toca-financial/internal/repositories \
  -run '^(TestManualSaleFinancialJourneyPostgresClosesCashAndLiabilities|TestManualRentFinancialJourneyPostgresPreservesFundingAndReceiptComposition|TestManagedBillPostgresTenantScopeCASAndTransactionReuse)$' | tee "$output_file"
python3 - "$output_file" <<'PY'
import json, sys
events = [json.loads(line) for line in open(sys.argv[1])]
required = {'TestManualSaleFinancialJourneyPostgresClosesCashAndLiabilities',
            'TestManualRentFinancialJourneyPostgresPreservesFundingAndReceiptComposition',
            'TestManagedBillPostgresTenantScopeCASAndTransactionReuse'}
assert not any(e.get('Action') in ('skip', 'fail') for e in events), 'mandatory financial tests skipped or failed'
passed = {e.get('Test') for e in events if e.get('Action') == 'pass'}
assert required <= passed, f'mandatory tests did not execute: {required-passed}'
print('[financial-integration] PASS mandatory migrated PostgreSQL journeys')
PY

docker exec -i "$container" psql -U postgres -d postgres -v ON_ERROR_STOP=1 < "$repo_dir/scripts/test-stack-fixture/financial-rls.sql"

export FINANCIAL_QA_NETWORK="$(docker inspect --format '{{range $name, $network := .NetworkSettings.Networks}}{{$name}}{{end}}' "$container")"
export FINANCIAL_QA_DATABASE_URI="postgres://postgres:postgres@$container:5432/postgres?sslmode=disable"
qa_compose=(docker compose --project-name "$qa_project" --project-directory "$repo_dir" -f "$repo_dir/scripts/test-stack-fixture/financial.compose.yml")
"${qa_compose[@]}" config --quiet
for service in test-auth toca-deal toca-financial; do "${qa_compose[@]}" build "$service"; done
docker exec -i "$container" psql -U postgres -d postgres -v ON_ERROR_STOP=1 < "$repo_dir/scripts/test-stack-fixture/financial-seed.sql"
"${qa_compose[@]}" up --detach --no-build --wait --wait-timeout 360
"${qa_compose[@]}" exec --no-TTY test-auth /fixture-server financial
"${qa_compose[@]}" restart toca-deal toca-financial
"${qa_compose[@]}" up --detach --no-build --wait --wait-timeout 360
"${qa_compose[@]}" exec --no-TTY test-auth /fixture-server verify-financial-restart
