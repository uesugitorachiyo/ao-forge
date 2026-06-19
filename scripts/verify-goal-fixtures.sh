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

invalid_path_goal_runs=()
while IFS= read -r invalid_path_goal_run; do
  invalid_path_goal_runs+=("$invalid_path_goal_run")
done < <(find examples -type f -name '*.goal-run.path-invalid.json' | sort)

if [[ "${#goal_runs[@]}" -eq 0 ]]; then
  echo "no GoalRun fixtures found under examples/" >&2
  exit 1
fi

if [[ "${#update_audits[@]}" -eq 0 ]]; then
  echo "no GoalRun update-audit fixtures found under examples/" >&2
  exit 1
fi

lint_goal_run_evidence_paths() {
  python3 - "$@" <<'PY'
import json
import pathlib
import re
import sys

errors = []

def evidence_items(document):
    data = json.loads(pathlib.Path(document).read_text())
    if document.endswith(".goal-run-update-audit.json"):
        return data.get("evidence", [])
    return data.get("last_iteration", {}).get("evidence", [])

def rejected_reason(path):
    normalized = path.replace("\\", "/")
    if normalized.startswith(("/", "//")):
        return "an absolute path"
    if re.match(r"^[A-Za-z]:/", normalized):
        return "an absolute path"
    if normalized.startswith(("~/", "$HOME/", "${HOME}/")):
        return "a home-directory path"
    parts = [part for part in normalized.split("/") if part not in ("", ".")]
    if parts and parts[0] == "..":
        return "a parent traversal path"
    if any(part in {"tmp", ".tmp", "temp"} for part in parts):
        return "a temporary path"
    return ""

for document in sys.argv[1:]:
    for index, evidence in enumerate(evidence_items(document)):
        path = evidence.get("path", "")
        reason = rejected_reason(path)
        if reason:
            errors.append(f"{document}: evidence[{index}].path {path!r} uses {reason}")

if errors:
    for error in errors:
        print(error, file=sys.stderr)
    sys.exit(1)
PY
}

for goal_run in "${goal_runs[@]}"; do
  "$forge_bin" goal validate --goal-run "$goal_run"
  lint_goal_run_evidence_paths "$goal_run"
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
  lint_goal_run_evidence_paths "$update_audit"
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

for invalid_path_goal_run in "${invalid_path_goal_runs[@]}"; do
  "$forge_bin" goal validate --goal-run "$invalid_path_goal_run"
  if lint_goal_run_evidence_paths "$invalid_path_goal_run"; then
    echo "expected GoalRun evidence path policy fixture to fail: $invalid_path_goal_run" >&2
    exit 1
  fi
done

echo "goal_run_fixtures_validated=${#goal_runs[@]}"
echo "goal_run_update_audits_validated=${#update_audits[@]}"
echo "goal_run_invalid_fixtures_rejected=${#invalid_goal_runs[@]}"
echo "goal_run_invalid_path_fixtures_rejected=${#invalid_path_goal_runs[@]}"
echo "goal_run_retained_evidence_fixtures=${retained_evidence_count}"
