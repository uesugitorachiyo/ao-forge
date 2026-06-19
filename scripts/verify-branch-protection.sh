#!/usr/bin/env bash
set -euo pipefail

repository="${AO_FORGE_GITHUB_REPOSITORY:-uesugitorachiyo/ao-forge}"
branch="${AO_FORGE_BRANCH_PROTECTION_BRANCH:-main}"
out="${AO_FORGE_BRANCH_PROTECTION_AUDIT:-}"

tmpdir="$(mktemp -d)"
trap 'rm -rf "$tmpdir"' EXIT

gh api "repos/${repository}/branches/${branch}/protection" >"${tmpdir}/protection.json"
gh api "repos/${repository}/rulesets" >"${tmpdir}/rulesets.json"

python3 - "$repository" "$branch" "${tmpdir}/protection.json" "${tmpdir}/rulesets.json" "$out" <<'PY'
import datetime
import json
import pathlib
import sys

repository, branch, protection_path, rulesets_path, out = sys.argv[1:]
protection = json.loads(pathlib.Path(protection_path).read_text())
rulesets = json.loads(pathlib.Path(rulesets_path).read_text())

required_checks = [
    "Go ubuntu-latest",
    "Go macos-latest",
    "Go windows-latest",
    "Workflow lint",
    "GoalRun fixture smoke",
    "Release preview dry-run audit",
]
observed_checks = protection.get("required_status_checks", {}).get("contexts") or []
observed_check_set = set(observed_checks)
missing_checks = [check for check in required_checks if check not in observed_check_set]

checks = {
    "branch_protection_api_available": True,
    "required_status_checks_strict": protection.get("required_status_checks", {}).get("strict") is True,
    "required_status_checks_complete": not missing_checks,
    "required_pull_request_reviews_enabled": isinstance(protection.get("required_pull_request_reviews"), dict),
    "dismiss_stale_reviews_enabled": protection.get("required_pull_request_reviews", {}).get("dismiss_stale_reviews") is True,
    "enforce_admins_enabled": protection.get("enforce_admins", {}).get("enabled") is True,
    "required_linear_history_enabled": protection.get("required_linear_history", {}).get("enabled") is True,
    "force_pushes_disabled": protection.get("allow_force_pushes", {}).get("enabled") is False,
    "deletions_disabled": protection.get("allow_deletions", {}).get("enabled") is False,
}

errors = []
for name, passed in checks.items():
    if not passed:
        errors.append(name)
if missing_checks:
    errors.append(f"missing required status checks: {', '.join(missing_checks)}")

audit = {
    "schema_version": "ao.forge.branch-protection-audit.v0.1",
    "status": "passed" if not errors else "blocked",
    "checked_at": datetime.datetime.now(datetime.UTC).replace(microsecond=0).isoformat().replace("+00:00", "Z"),
    "repository": repository,
    "branch": branch,
    "required_checks": required_checks,
    "observed_checks": observed_checks,
    "checks": checks,
    "rulesets_checked": True,
    "rulesets_count": len(rulesets),
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
