# AO Forge Branch Protection Runbook

This runbook documents the recommended GitHub branch protection settings for
the public AO Forge repository. It is intended for maintainers configuring
protection on `main`. For first public release operations, also follow
`FIRST-PUBLIC-RELEASE.md`. The latest live verification evidence is recorded in
`BRANCH-PROTECTION-EVIDENCE.md`.

## Required Settings

Configure a ruleset or branch protection rule for `main` with these controls:

- Require a pull request before merging.
- Dismiss stale pull request approvals when new commits are pushed.
- Require status checks to pass before merging.
- Require branches to be up to date before merging when GitHub offers that
  option for the ruleset.
- Require linear history.
- Restrict force pushes.
- Do not allow deletions.
- Do not bypass the required checks for public releases.

## Required Checks

Require these status checks before merge:

- `Go ubuntu-latest`
- `Go macos-26`
- `Go windows-latest`
- `License policy`
- `Workflow lint`
- `GoalRun fixture smoke`
- `Production readiness audit`
- `Release preview dry-run audit`

The macOS Go check is pinned to an explicit image label so GitHub's moving
`macos-latest` alias cannot silently change the required status context.

The Go checks, License policy check, Workflow lint check, GoalRun fixture smoke
check, and Production readiness audit check come from
`.github/workflows/ci.yml`. License policy verifies the canonical Apache-2.0
root license, NOTICE, and package metadata before merge. Workflow lint runs
`actionlint` against
`.github/workflows/*.yml` so GitHub Actions context and expression errors are
caught before merge. GoalRun fixture smoke validates checked-in GoalRun
fixtures, update-audit fixtures, evidence verifier JSON, and stale-evidence
failure behavior before merge. Optional external shell and Python linters are
disabled for this gate so local and hosted results stay deterministic.
Production readiness audit runs `forge production-readiness audit`, validates
the JSON output against its published schema, runs
`forge goal evidence cleanup --dry-run --json`, validates that cleanup dry-run
against `docs/contracts/goal-run-retained-evidence-cleanup-v0.1.schema.json`,
validates the checked-in release-candidate handoff, and uploads both
machine-readable artifacts before merge. The release preview check comes from
`.github/workflows/release-preview.yml` and verifies the non-mutating release
rehearsal, human inspect output, JSON inspect output, and uploaded release
evidence artifacts.

Release Preview also validates generated audit and inspect JSON against their published schemas before uploading artifacts.

## Maintainer Flow

1. Work on a feature branch.
2. Open a pull request into `main`.
3. Wait for all required checks to pass.
4. Review the release preview artifacts when the change touches release,
   workflow, artifact, security, or evidence paths.
5. Merge through GitHub after the required checks pass.
6. Confirm the post-merge `main` CI and Release Preview runs pass.

## Live Verification

After changing branch protection, or after renaming workflow jobs, run:

```sh
scripts/verify-branch-protection.sh
```

The verifier is read-only. It uses `gh api` to inspect live GitHub protection
for `main`, confirms the required check names and protection toggles, and emits
`ao.forge.branch-protection-audit.v0.1` JSON. The
`Production Readiness Ops` workflow runs the same verifier on a daily schedule
and by manual dispatch using read-only repository permissions. Because the
default `GITHUB_TOKEN` cannot read every administrative branch-protection field,
the workflow sets `AO_FORGE_BRANCH_PROTECTION_MODE=limited`; maintainer local
runs default to `full`. Keep `BRANCH-PROTECTION-EVIDENCE.md` current when the
live settings change.

## Local Fallback

Before pushing public changes, run the local release-readiness gate:
`forge contract validate --schema docs/contracts/release-preview-inspect-v0.1.schema.json`

```sh
git diff --check
go run github.com/rhysd/actionlint/cmd/actionlint@v1.7.12 -shellcheck= -pyflakes= .github/workflows/*.yml
go test ./... -count=1
go vet ./...
go build -o /tmp/ao-forge-smoke ./cmd/forge
/tmp/ao-forge-smoke doctor --foundation docs/foundation/foundation-baseline.v0.1.json
/tmp/ao-forge-smoke contract validate --schema docs/contracts/release-preview-audit-v0.1.schema.json --document examples/release-preview/dirty-workspace-blocked.audit.json
/tmp/ao-forge-smoke contract validate --schema docs/contracts/release-preview-inspect-v0.1.schema.json --document examples/release-preview/dirty-workspace-blocked.inspect.expected.json
/tmp/ao-forge-smoke contract validate --schema docs/contracts/release-artifact-inventory-v0.1.schema.json --document examples/release-preview/release-artifact-inventory.v0.1.example.json
/tmp/ao-forge-smoke contract validate --schema docs/contracts/release-attestation-plan-v0.1.schema.json --document examples/release-preview/release-attestation-plan.v0.1.example.json
/tmp/ao-forge-smoke production-readiness audit --json >/tmp/ao-forge-production-readiness-audit.json
/tmp/ao-forge-smoke contract validate --schema docs/contracts/production-readiness-audit-v0.1.schema.json --document /tmp/ao-forge-production-readiness-audit.json
/tmp/ao-forge-smoke release-candidate validate --candidate examples/release-preview/release-candidate.v0.1.example.json
/tmp/ao-forge-smoke artifact verify-checksums --manifest examples/release-preview/checksums.txt
scripts/check-public-repo-policy.sh
gitleaks detect --source . --redact --verbose
gitleaks dir . --redact --verbose
AO_FORGE_RELEASE_PREVIEW_OUT=/tmp/ao-forge-release-preview scripts/release-preview-dry-run.sh
python3 -m json.tool /tmp/ao-forge-release-preview/release-preview-inspect.json
```

The local gate does not replace branch protection. It reduces feedback time
before the required GitHub checks run.

## Updating This Runbook

If workflow job names change, update this runbook in the same pull request as
the workflow change. The public required-check names should remain easy to copy
from this document into GitHub branch protection settings.
