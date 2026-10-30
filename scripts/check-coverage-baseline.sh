#!/usr/bin/env bash
set -euo pipefail

module=${1:?usage: check-coverage-baseline.sh MODULE ACTUAL_PERCENT [BASELINES_FILE]}
actual=${2:?usage: check-coverage-baseline.sh MODULE ACTUAL_PERCENT [BASELINES_FILE]}
baselines_file=${3:-scripts/coverage-baselines.tsv}

if [[ ! -f "$baselines_file" ]]; then
  echo "coverage baselines not found: $baselines_file" >&2
  exit 2
fi

read -r minimum next_target < <(
  awk -v module="$module" '$1 == module { print $2, $3; found = 1; exit } END { if (!found) exit 1 }' "$baselines_file"
) || {
  echo "coverage baseline missing for module: $module" >&2
  exit 2
}

if ! [[ "$actual" =~ ^[0-9]+([.][0-9]+)?$ ]]; then
  echo "invalid coverage for $module: $actual" >&2
  exit 2
fi

awk -v module="$module" -v actual="$actual" -v minimum="$minimum" -v target="$next_target" 'BEGIN {
  printf "Coverage %-18s actual=%5.1f%% minimum=%5.1f%% next_target=%5.1f%%\n", module, actual, minimum, target
  if (actual + 0.0001 < minimum) {
    printf "FAIL: %s regressed below its module baseline\n", module > "/dev/stderr"
    exit 1
  }
}'
