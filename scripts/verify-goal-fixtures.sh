#!/usr/bin/env bash
set -euo pipefail

root="$(git rev-parse --show-toplevel)"
cd "$root"

if [[ -n "${AO_FORGE_BIN:-}" ]]; then
  forge_bin="$AO_FORGE_BIN"
else
  forge_bin="${RUNNER_TEMP:-/tmp}/ao-forge-goal-fixtures/forge"
  mkdir -p "$(dirname "$forge_bin")"
  go build -o "$forge_bin" ./cmd/forge
fi

if [[ ! -x "$forge_bin" ]]; then
  echo "forge binary is not executable: $forge_bin" >&2
  exit 1
fi

goal_runs=()
while IFS= read -r goal_run; do
  goal_runs+=("$goal_run")
done < <(find examples -type f -name '*.goal-run.json' | sort)

update_audits=()
while IFS= read -r update_audit; do
  update_audits+=("$update_audit")
done < <(find examples -type f -name '*.goal-run-update-audit.json' | sort)

if [[ "${#goal_runs[@]}" -eq 0 ]]; then
  echo "no GoalRun fixtures found under examples/" >&2
  exit 1
fi

if [[ "${#update_audits[@]}" -eq 0 ]]; then
  echo "no GoalRun update-audit fixtures found under examples/" >&2
  exit 1
fi

for goal_run in "${goal_runs[@]}"; do
  "$forge_bin" goal validate --goal-run "$goal_run"
  "$forge_bin" goal evidence verify --goal-run "$goal_run"
done

for update_audit in "${update_audits[@]}"; do
  "$forge_bin" contract validate \
    --schema docs/contracts/goal-run-update-audit-v0.1.schema.json \
    --document "$update_audit"
done

echo "goal_run_fixtures_validated=${#goal_runs[@]}"
echo "goal_run_update_audits_validated=${#update_audits[@]}"
