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

verify_dir="$(mktemp -d "${TMPDIR:-/tmp}/ao-forge-goal-verify.XXXXXX")"
trap 'rm -rf "$verify_dir"' EXIT

goal_runs=()
while IFS= read -r goal_run; do
  goal_runs+=("$goal_run")
done < <(find examples -type f -name '*.goal-run.json' | sort)

update_audits=()
while IFS= read -r update_audit; do
  update_audits+=("$update_audit")
done < <(find examples -type f -name '*.goal-run-update-audit.json' | sort)

invalid_goal_runs=()
while IFS= read -r invalid_goal_run; do
  invalid_goal_runs+=("$invalid_goal_run")
done < <(find examples -type f -name '*.goal-run.invalid.json' | sort)

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
  verify_json="$verify_dir/$(basename "$goal_run").evidence-verify.json"
  "$forge_bin" goal evidence verify --goal-run "$goal_run" --json > "$verify_json"
  "$forge_bin" contract validate \
    --schema docs/contracts/goal-run-evidence-verify-v0.1.schema.json \
    --document "$verify_json"
done

retained_evidence_count="$(
  python3 - "${goal_runs[@]}" <<'PY'
import json
import pathlib
import sys

count = 0
for goal_run in sys.argv[1:]:
    data = json.loads(pathlib.Path(goal_run).read_text())
    for evidence in data.get("last_iteration", {}).get("evidence", []):
        if evidence.get("path", "").startswith("docs/evidence/goals/"):
            count += 1
print(count)
PY
)"
if [[ "$retained_evidence_count" -eq 0 ]]; then
  echo "no retained GoalRun evidence fixture found under docs/evidence/goals/" >&2
  exit 1
fi

for update_audit in "${update_audits[@]}"; do
  "$forge_bin" contract validate \
    --schema docs/contracts/goal-run-update-audit-v0.1.schema.json \
    --document "$update_audit"
done

for invalid_goal_run in "${invalid_goal_runs[@]}"; do
  "$forge_bin" goal validate --goal-run "$invalid_goal_run"
  if "$forge_bin" goal evidence verify --goal-run "$invalid_goal_run"; then
    echo "expected stale GoalRun evidence fixture to fail: $invalid_goal_run" >&2
    exit 1
  fi
  verify_json="$verify_dir/$(basename "$invalid_goal_run").evidence-verify.json"
  if "$forge_bin" goal evidence verify --goal-run "$invalid_goal_run" --json > "$verify_json"; then
    echo "expected stale GoalRun evidence JSON fixture to fail: $invalid_goal_run" >&2
    exit 1
  fi
  "$forge_bin" contract validate \
    --schema docs/contracts/goal-run-evidence-verify-v0.1.schema.json \
    --document "$verify_json"
done

echo "goal_run_fixtures_validated=${#goal_runs[@]}"
echo "goal_run_update_audits_validated=${#update_audits[@]}"
echo "goal_run_invalid_fixtures_rejected=${#invalid_goal_runs[@]}"
echo "goal_run_retained_evidence_fixtures=${retained_evidence_count}"
