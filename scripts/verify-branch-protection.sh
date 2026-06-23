#!/usr/bin/env bash
set -euo pipefail

repository="${AO_FORGE_GITHUB_REPOSITORY:-uesugitorachiyo/ao-forge}"
branch="${AO_FORGE_BRANCH_PROTECTION_BRANCH:-main}"
out="${AO_FORGE_BRANCH_PROTECTION_AUDIT:-}"
mode="${AO_FORGE_BRANCH_PROTECTION_MODE:-full}"

tmpdir="$(mktemp -d)"
trap 'rm -rf "$tmpdir"' EXIT

if [[ "$mode" == "full" ]]; then
  gh api "repos/${repository}/branches/${branch}/protection" >"${tmpdir}/protection.json"
  gh api "repos/${repository}/rulesets" >"${tmpdir}/rulesets.json"
elif [[ "$mode" == "limited" ]]; then
  gh api "repos/${repository}/branches/${branch}" >"${tmpdir}/branch.json"
else
  echo "unsupported AO_FORGE_BRANCH_PROTECTION_MODE: ${mode}" >&2
  exit 2
fi

python3 - "$repository" "$branch" "$mode" "$tmpdir" "$out" <<'PY'
import datetime
import json
import pathlib
import sys

repository, branch, mode, tmpdir, out = sys.argv[1:]
tmpdir = pathlib.Path(tmpdir)

required_checks = [
    "Go ubuntu-latest",
    "Go macos-26",
    "Go windows-latest",
    "License policy",
    "Workflow lint",
    "GoalRun fixture smoke",
    "Production readiness audit",
    "Release preview dry-run audit",
]

rulesets_checked = False
rulesets_count = 0

if mode == "full":
    protection = json.loads((tmpdir / "protection.json").read_text())
    rulesets = json.loads((tmpdir / "rulesets.json").read_text())
    observed_checks = protection.get("required_status_checks", {}).get("contexts") or []
    checks = {
        "branch_protection_api_available": True,
        "required_status_checks_strict": protection.get("required_status_checks", {}).get("strict") is True,
        "required_status_checks_complete": False,
        "required_pull_request_reviews_enabled": isinstance(protection.get("required_pull_request_reviews"), dict),
        "dismiss_stale_reviews_enabled": protection.get("required_pull_request_reviews", {}).get("dismiss_stale_reviews") is True,
        "enforce_admins_enabled": protection.get("enforce_admins", {}).get("enabled") is True,
        "required_linear_history_enabled": protection.get("required_linear_history", {}).get("enabled") is True,
        "force_pushes_disabled": protection.get("allow_force_pushes", {}).get("enabled") is False,
        "deletions_disabled": protection.get("allow_deletions", {}).get("enabled") is False,
    }
    rulesets_checked = True
    rulesets_count = len(rulesets)
else:
    branch_info = json.loads((tmpdir / "branch.json").read_text())
    protection = branch_info.get("protection") or {}
    required_status_checks = protection.get("required_status_checks") or {}
    observed_checks = required_status_checks.get("contexts") or []
    checks = {
        "branch_metadata_api_available": True,
        "branch_protected": branch_info.get("protected") is True,
        "required_status_checks_complete": False,
        "required_status_checks_enforced_for_everyone": required_status_checks.get("enforcement_level") == "everyone",
    }

observed_check_set = set(observed_checks)
missing_checks = [check for check in required_checks if check not in observed_check_set]
checks["required_status_checks_complete"] = not missing_checks

errors = []
for name, passed in checks.items():
    if not passed:
        errors.append(name)
if missing_checks:
    errors.append(f"missing required status checks: {', '.join(missing_checks)}")

audit = {
    "schema_version": "ao.forge.branch-protection-audit.v0.1",
    "status": "passed" if not errors else "blocked",
    "checked_at": datetime.datetime.now(datetime.timezone.utc).replace(microsecond=0).isoformat().replace("+00:00", "Z"),
    "repository": repository,
    "branch": branch,
    "mode": mode,
    "required_checks": required_checks,
    "observed_checks": observed_checks,
    "checks": checks,
    "rulesets_checked": rulesets_checked,
    "rulesets_count": rulesets_count,
    "errors": errors,
}

rendered = json.dumps(audit, indent=2, sort_keys=True) + "\n"
if out:
    pathlib.Path(out).write_text(rendered)
else:
    sys.stdout.write(rendered)

if audit["status"] != "passed":
    sys.exit(1)
PY
