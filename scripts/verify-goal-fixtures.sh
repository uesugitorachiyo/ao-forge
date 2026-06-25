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

invalid_path_update_audits=()
while IFS= read -r invalid_path_update_audit; do
  invalid_path_update_audits+=("$invalid_path_update_audit")
done < <(find examples -type f -name '*.goal-run-update-audit.path-invalid.json' | sort)

retained_evidence_artifacts=()
while IFS= read -r retained_evidence_artifact; do
  retained_evidence_artifacts+=("$retained_evidence_artifact")
done < <(find docs/evidence/goals -type f -name '*.json' ! -name '*readiness-audit.json' | sort)

retained_readiness_audits=()
while IFS= read -r retained_readiness_audit; do
  retained_readiness_audits+=("$retained_readiness_audit")
done < <(find docs/evidence/goals -type f -name '*readiness-audit.json' | sort)

invalid_retained_evidence_artifacts=()
while IFS= read -r invalid_retained_evidence_artifact; do
  invalid_retained_evidence_artifacts+=("$invalid_retained_evidence_artifact")
done < <(find examples -type f -name '*.goal-run-retained-evidence.invalid.json' | sort)

invalid_readiness_audits=()
while IFS= read -r invalid_readiness_audit; do
  invalid_readiness_audits+=("$invalid_readiness_audit")
done < <(find examples -type f -name '*.goal-run-readiness-audit.invalid.json' | sort)

invalid_readiness_provenance_audits=()
while IFS= read -r invalid_readiness_provenance_audit; do
  invalid_readiness_provenance_audits+=("$invalid_readiness_provenance_audit")
done < <(find examples -type f -name '*.goal-run-readiness-audit.provenance-invalid.json' | sort)

verify_readiness_audit_provenance() {
  local readiness_audit="$1"
  python3 - "$readiness_audit" <<'PY'
import hashlib
import json
import pathlib
import sys

audit_path = pathlib.Path(sys.argv[1])
audit = json.loads(audit_path.read_text())
provenance = audit.get("provenance", {})
goal_run = provenance.get("goal_run", {})
goal_path = pathlib.Path(goal_run.get("path", ""))
if not goal_path.is_file():
    raise SystemExit(f"readiness audit provenance goal_run is not readable: {goal_path}")
actual_goal_sha = hashlib.sha256(goal_path.read_bytes()).hexdigest()
if goal_run.get("sha256") != actual_goal_sha:
    raise SystemExit(
        f"readiness audit provenance goal_run sha256 mismatch: {goal_path}: "
        f"expected {goal_run.get('sha256')}, got {actual_goal_sha}"
    )

evidence_by_path = {
    item.get("path"): item
    for item in provenance.get("evidence", [])
}
for evidence in audit.get("evidence_verify", {}).get("evidence", []):
    evidence_path = pathlib.Path(evidence.get("path", ""))
    if evidence.get("status") != "passed":
        continue
    if not evidence_path.is_file():
        raise SystemExit(f"readiness audit provenance evidence is not readable: {evidence_path}")
    provenance_evidence = evidence_by_path.get(str(evidence_path))
    if provenance_evidence is None:
        raise SystemExit(f"readiness audit provenance missing evidence: {evidence_path}")
    actual_evidence_sha = hashlib.sha256(evidence_path.read_bytes()).hexdigest()
    expected_sha = provenance_evidence.get("sha256")
    if expected_sha != actual_evidence_sha:
        raise SystemExit(
            f"readiness audit provenance evidence sha256 mismatch: {evidence_path}: "
            f"expected {expected_sha}, got {actual_evidence_sha}"
        )
    if evidence.get("actual_sha256") != actual_evidence_sha:
        raise SystemExit(
            f"readiness audit evidence_verify sha256 mismatch: {evidence_path}: "
            f"expected {evidence.get('actual_sha256')}, got {actual_evidence_sha}"
        )
PY
}

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
  "$forge_bin" goal readiness --goal-run "$goal_run" --now 2026-06-19T18:00:00Z
  readiness_json="$verify_dir/$(basename "$goal_run").readiness-audit.json"
  "$forge_bin" goal readiness --goal-run "$goal_run" --now 2026-06-19T18:00:00Z --json > "$readiness_json"
  "$forge_bin" contract validate \
    --schema docs/contracts/goal-run-readiness-audit-v0.1.schema.json \
    --document "$readiness_json"
  pulse_readiness_json="$verify_dir/$(basename "$goal_run").ao2-pulse-readiness-audit.json"
  AO_FORGE_BIN="$forge_bin" scripts/ao2-pulse-goal-readiness.sh \
    --goal-run "$goal_run" \
    --now 2026-06-19T18:00:00Z \
    --out "$pulse_readiness_json"
  "$forge_bin" contract validate \
    --schema docs/contracts/goal-run-readiness-audit-v0.1.schema.json \
    --document "$pulse_readiness_json"
  "$forge_bin" goal evidence lint --goal-run "$goal_run"
  lint_json="$verify_dir/$(basename "$goal_run").evidence-lint.json"
  "$forge_bin" goal evidence lint --goal-run "$goal_run" --json > "$lint_json"
  "$forge_bin" contract validate \
    --schema docs/contracts/goal-run-evidence-lint-v0.1.schema.json \
    --document "$lint_json"
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

if [[ "${#retained_evidence_artifacts[@]}" -eq 0 ]]; then
  echo "no retained GoalRun evidence artifacts found under docs/evidence/goals/" >&2
  exit 1
fi

if [[ "${#retained_readiness_audits[@]}" -eq 0 ]]; then
  echo "no retained GoalRun readiness audit found under docs/evidence/goals/" >&2
  exit 1
fi

if [[ "${#invalid_retained_evidence_artifacts[@]}" -eq 0 ]]; then
  echo "no invalid retained GoalRun evidence artifact fixtures found under examples/" >&2
  exit 1
fi

if [[ "${#invalid_readiness_audits[@]}" -eq 0 ]]; then
  echo "no invalid GoalRun readiness audit fixtures found under examples/" >&2
  exit 1
fi

if [[ "${#invalid_readiness_provenance_audits[@]}" -eq 0 ]]; then
  echo "no invalid GoalRun readiness provenance fixtures found under examples/" >&2
  exit 1
fi

for retained_evidence_artifact in "${retained_evidence_artifacts[@]}"; do
  "$forge_bin" contract validate \
    --schema docs/contracts/goal-run-retained-evidence-v0.1.schema.json \
    --document "$retained_evidence_artifact"
  "$forge_bin" goal evidence retention \
    --artifact "$retained_evidence_artifact" \
    --now 2026-06-19T18:00:00Z
  retention_json="$verify_dir/$(basename "$retained_evidence_artifact").retention-audit.json"
  "$forge_bin" goal evidence retention \
    --artifact "$retained_evidence_artifact" \
    --now 2026-06-19T18:00:00Z \
    --json > "$retention_json"
  "$forge_bin" contract validate \
    --schema docs/contracts/goal-run-retained-evidence-audit-v0.1.schema.json \
    --document "$retention_json"
done

cleanup_json="$verify_dir/goal-run-retained-evidence-cleanup.json"
"$forge_bin" goal evidence cleanup \
  --dry-run \
  --now 2026-06-19T18:00:00Z \
  --json > "$cleanup_json"
"$forge_bin" contract validate \
  --schema docs/contracts/goal-run-retained-evidence-cleanup-v0.1.schema.json \
  --document "$cleanup_json"
python3 - "$cleanup_json" <<'PY'
import json
import pathlib
import sys

summary = json.loads(pathlib.Path(sys.argv[1]).read_text())
expected = {
    "status": "passed",
    "mode": "dry-run",
    "artifacts_scanned": 6,
    "eligible_artifacts": 1,
    "protected_artifacts": 5,
    "failed_artifacts": 0,
    "public_provenance_excluded": 2,
    "active_goal_excluded": 3,
}
for key, value in expected.items():
    if summary.get(key) != value:
        raise SystemExit(f"retained evidence cleanup {key} drifted: expected {value!r}, got {summary.get(key)!r}")
eligible = [
    audit for audit in summary.get("retention_audits", [])
    if audit.get("cleanup_review_status") == "eligible_after_review"
]
if len(eligible) != 1:
    raise SystemExit(f"expected exactly one cleanup-review-eligible artifact, got {len(eligible)}")
public_provenance = [
    audit for audit in summary.get("retention_audits", [])
    if audit.get("cleanup_review_status") == "not_eligible_public_provenance"
]
if len(public_provenance) != 2:
    raise SystemExit(f"expected exactly two public provenance exclusions, got {len(public_provenance)}")
classes = {audit.get("retention_class") for audit in public_provenance}
if classes != {"release_provenance", "promotion_provenance"}:
    raise SystemExit(f"public provenance cleanup exclusions drifted: {sorted(classes)}")
PY

for retained_readiness_audit in "${retained_readiness_audits[@]}"; do
  "$forge_bin" contract validate \
    --schema docs/contracts/goal-run-readiness-audit-v0.1.schema.json \
    --document "$retained_readiness_audit"
  verify_readiness_audit_provenance "$retained_readiness_audit"
done

for invalid_retained_evidence_artifact in "${invalid_retained_evidence_artifacts[@]}"; do
  if "$forge_bin" contract validate \
    --schema docs/contracts/goal-run-retained-evidence-v0.1.schema.json \
    --document "$invalid_retained_evidence_artifact"; then
    echo "expected retained GoalRun evidence artifact fixture to fail: $invalid_retained_evidence_artifact" >&2
    exit 1
  fi
done

for invalid_readiness_audit in "${invalid_readiness_audits[@]}"; do
  if "$forge_bin" contract validate \
    --schema docs/contracts/goal-run-readiness-audit-v0.1.schema.json \
    --document "$invalid_readiness_audit"; then
    echo "expected GoalRun readiness audit fixture to fail: $invalid_readiness_audit" >&2
    exit 1
  fi
done

for invalid_readiness_provenance_audit in "${invalid_readiness_provenance_audits[@]}"; do
  "$forge_bin" contract validate \
    --schema docs/contracts/goal-run-readiness-audit-v0.1.schema.json \
    --document "$invalid_readiness_provenance_audit"
  provenance_error="$verify_dir/$(basename "$invalid_readiness_provenance_audit").provenance-error.txt"
  if verify_readiness_audit_provenance "$invalid_readiness_provenance_audit" 2> "$provenance_error"; then
    echo "expected GoalRun readiness provenance fixture to fail: $invalid_readiness_provenance_audit" >&2
    exit 1
  fi
  if ! grep -q "readiness audit provenance .* sha256 mismatch" "$provenance_error"; then
    echo "expected GoalRun readiness provenance fixture to fail with a sha256 mismatch: $invalid_readiness_provenance_audit" >&2
    cat "$provenance_error" >&2
    exit 1
  fi
done

for update_audit in "${update_audits[@]}"; do
  "$forge_bin" goal evidence lint --update-audit "$update_audit"
  lint_json="$verify_dir/$(basename "$update_audit").evidence-lint.json"
  "$forge_bin" goal evidence lint --update-audit "$update_audit" --json > "$lint_json"
  "$forge_bin" contract validate \
    --schema docs/contracts/goal-run-evidence-lint-v0.1.schema.json \
    --document "$lint_json"
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
  pulse_readiness_json="$verify_dir/$(basename "$invalid_goal_run").ao2-pulse-readiness-audit.json"
  if AO_FORGE_BIN="$forge_bin" scripts/ao2-pulse-goal-readiness.sh \
    --goal-run "$invalid_goal_run" \
    --now 2026-06-19T18:00:00Z \
    --out "$pulse_readiness_json"; then
    echo "expected AO2 Pulse readiness entrypoint to fail: $invalid_goal_run" >&2
    exit 1
  fi
  "$forge_bin" contract validate \
    --schema docs/contracts/goal-run-readiness-audit-v0.1.schema.json \
    --document "$pulse_readiness_json"
  python3 - "$pulse_readiness_json" "$invalid_goal_run" <<'PY'
import json
import pathlib
import sys

audit_path, goal_run = sys.argv[1:]
audit = json.loads(pathlib.Path(audit_path).read_text())
checks = {check["check_id"]: check for check in audit.get("checks", [])}
if audit.get("goal_run") != goal_run:
    raise SystemExit(f"readiness audit goal_run mismatch: {audit.get('goal_run')} != {goal_run}")
if audit.get("status") != "failed":
    raise SystemExit(f"readiness audit should be failed: {audit.get('status')}")
if audit.get("evidence_verify", {}).get("status") != "failed":
    raise SystemExit("readiness audit did not preserve failed evidence_verify result")
if checks.get("evidence_verify", {}).get("status") != "failed":
    raise SystemExit("readiness audit did not preserve failed evidence_verify check")
if not audit.get("errors"):
    raise SystemExit("readiness audit did not preserve failure errors")
PY
  blocked_candidate="$verify_dir/$(basename "$invalid_goal_run").advanced.goal-run.json"
  if "$forge_bin" goal update \
    --goal-run "$invalid_goal_run" \
    --out "$blocked_candidate" \
    --phase verification; then
    echo "expected stale GoalRun update to fail after readiness failure: $invalid_goal_run" >&2
    exit 1
  fi
  if [[ -e "$blocked_candidate" ]]; then
    echo "stale GoalRun update wrote candidate after readiness failure: $blocked_candidate" >&2
    exit 1
  fi
done

for invalid_path_goal_run in "${invalid_path_goal_runs[@]}"; do
  "$forge_bin" goal validate --goal-run "$invalid_path_goal_run"
  if "$forge_bin" goal evidence lint --goal-run "$invalid_path_goal_run"; then
    echo "expected GoalRun evidence path policy fixture to fail: $invalid_path_goal_run" >&2
    exit 1
  fi
  lint_json="$verify_dir/$(basename "$invalid_path_goal_run").evidence-lint.json"
  if "$forge_bin" goal evidence lint --goal-run "$invalid_path_goal_run" --json > "$lint_json"; then
    echo "expected GoalRun evidence path policy JSON fixture to fail: $invalid_path_goal_run" >&2
    exit 1
  fi
  "$forge_bin" contract validate \
    --schema docs/contracts/goal-run-evidence-lint-v0.1.schema.json \
    --document "$lint_json"
done

for invalid_path_update_audit in "${invalid_path_update_audits[@]}"; do
  "$forge_bin" contract validate \
    --schema docs/contracts/goal-run-update-audit-v0.1.schema.json \
    --document "$invalid_path_update_audit"
  if "$forge_bin" goal evidence lint --update-audit "$invalid_path_update_audit"; then
    echo "expected GoalRun update-audit evidence path policy fixture to fail: $invalid_path_update_audit" >&2
    exit 1
  fi
  lint_json="$verify_dir/$(basename "$invalid_path_update_audit").evidence-lint.json"
  if "$forge_bin" goal evidence lint --update-audit "$invalid_path_update_audit" --json > "$lint_json"; then
    echo "expected GoalRun update-audit evidence path policy JSON fixture to fail: $invalid_path_update_audit" >&2
    exit 1
  fi
  "$forge_bin" contract validate \
    --schema docs/contracts/goal-run-evidence-lint-v0.1.schema.json \
    --document "$lint_json"
done

echo "goal_run_fixtures_validated=${#goal_runs[@]}"
echo "goal_run_readiness_audits_validated=${#goal_runs[@]}"
echo "ao2_pulse_readiness_entrypoints_validated=${#goal_runs[@]}"
echo "goal_run_update_audits_validated=${#update_audits[@]}"
echo "goal_run_invalid_fixtures_rejected=${#invalid_goal_runs[@]}"
echo "goal_run_invalid_path_fixtures_rejected=${#invalid_path_goal_runs[@]}"
echo "goal_run_update_audit_invalid_path_fixtures_rejected=${#invalid_path_update_audits[@]}"
echo "goal_run_retained_evidence_fixtures=${retained_evidence_count}"
echo "goal_run_retained_evidence_artifacts_validated=${#retained_evidence_artifacts[@]}"
echo "goal_run_retained_evidence_audits_validated=${#retained_evidence_artifacts[@]}"
echo "goal_run_retained_evidence_cleanup_dry_run_validated=1"
echo "goal_run_retained_readiness_audits_validated=${#retained_readiness_audits[@]}"
echo "goal_run_retained_readiness_provenance_verified=${#retained_readiness_audits[@]}"
echo "goal_run_retained_evidence_invalid_fixtures_rejected=${#invalid_retained_evidence_artifacts[@]}"
echo "goal_run_readiness_audit_invalid_fixtures_rejected=${#invalid_readiness_audits[@]}"
echo "goal_run_readiness_provenance_invalid_fixtures_rejected=${#invalid_readiness_provenance_audits[@]}"
echo "ao2_pulse_failed_readiness_audits_preserved=${#invalid_goal_runs[@]}"
