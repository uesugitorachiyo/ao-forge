#!/usr/bin/env python3
import datetime
import json
import os
import pathlib
import subprocess
import sys
import tempfile


REQUIRED_CHECKS = [
    "Go ubuntu-latest",
    "Go macos-26",
    "Go windows-latest",
    "License policy",
    "Workflow lint",
    "GoalRun fixture smoke",
    "Production readiness audit",
    "Release preview dry-run audit",
]


def gh_json(path: str, out_path: pathlib.Path):
    with out_path.open("w", encoding="utf-8") as handle:
        subprocess.run(["gh", "api", path], stdout=handle, check=True)
    return json.loads(out_path.read_text(encoding="utf-8"))


def build_audit(repository: str, branch: str, mode: str, tmpdir: pathlib.Path) -> dict:
    rulesets_checked = False
    rulesets_count = 0
    if mode == "full":
        protection = gh_json(f"repos/{repository}/branches/{branch}/protection", tmpdir / "protection.json")
        rulesets = gh_json(f"repos/{repository}/rulesets", tmpdir / "rulesets.json")
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
    elif mode == "limited":
        branch_info = gh_json(f"repos/{repository}/branches/{branch}", tmpdir / "branch.json")
        protection = branch_info.get("protection") or {}
        required_status_checks = protection.get("required_status_checks") or {}
        observed_checks = required_status_checks.get("contexts") or []
        checks = {
            "branch_metadata_api_available": True,
            "branch_protected": branch_info.get("protected") is True,
            "required_status_checks_complete": False,
            "required_status_checks_enforced_for_everyone": required_status_checks.get("enforcement_level") == "everyone",
        }
    else:
        raise ValueError(f"unsupported AO_FORGE_BRANCH_PROTECTION_MODE: {mode}")

    missing_checks = [check for check in REQUIRED_CHECKS if check not in set(observed_checks)]
    checks["required_status_checks_complete"] = not missing_checks

    errors = [name for name, passed in checks.items() if not passed]
    if missing_checks:
        errors.append(f"missing required status checks: {', '.join(missing_checks)}")

    return {
        "schema_version": "ao.forge.branch-protection-audit.v0.1",
        "status": "passed" if not errors else "blocked",
        "checked_at": datetime.datetime.now(datetime.timezone.utc).replace(microsecond=0).isoformat().replace("+00:00", "Z"),
        "repository": repository,
        "branch": branch,
        "mode": mode,
        "required_checks": REQUIRED_CHECKS,
        "observed_checks": observed_checks,
        "checks": checks,
        "rulesets_checked": rulesets_checked,
        "rulesets_count": rulesets_count,
        "errors": errors,
    }


def main() -> int:
    repository = os.environ.get("AO_FORGE_GITHUB_REPOSITORY", "uesugitorachiyo/ao-forge")
    branch = os.environ.get("AO_FORGE_BRANCH_PROTECTION_BRANCH", "main")
    out = os.environ.get("AO_FORGE_BRANCH_PROTECTION_AUDIT", "")
    mode = os.environ.get("AO_FORGE_BRANCH_PROTECTION_MODE", "full")

    with tempfile.TemporaryDirectory() as tmp:
        try:
            audit = build_audit(repository, branch, mode, pathlib.Path(tmp))
        except ValueError as exc:
            print(str(exc), file=sys.stderr)
            return 2

    rendered = json.dumps(audit, indent=2, sort_keys=True) + "\n"
    if out:
        pathlib.Path(out).write_text(rendered, encoding="utf-8")
    else:
        sys.stdout.write(rendered)
    return 0 if audit["status"] == "passed" else 1


if __name__ == "__main__":
    raise SystemExit(main())
