# AO Forge Branch Protection Runbook

This runbook documents the recommended GitHub branch protection settings for
the public AO Forge repository. It is intended for maintainers configuring
protection on `main`.

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
- `Go macos-latest`
- `Go windows-latest`
- `Release preview dry-run audit`

The Go checks come from `.github/workflows/ci.yml`. The release preview check
comes from `.github/workflows/release-preview.yml` and verifies the non-mutating
release rehearsal, human inspect output, JSON inspect output, and uploaded
release evidence artifacts.

Release Preview also validates generated audit and inspect JSON against their published schemas before uploading artifacts.

## Maintainer Flow

1. Work on a feature branch.
2. Open a pull request into `main`.
3. Wait for all required checks to pass.
4. Review the release preview artifacts when the change touches release,
   workflow, artifact, security, or evidence paths.
5. Merge through GitHub after the required checks pass.
6. Confirm the post-merge `main` CI and Release Preview runs pass.

## Local Fallback

Before pushing public changes, run the local release-readiness gate:
`forge contract validate --schema docs/contracts/release-preview-inspect-v0.1.schema.json`

```sh
git diff --check
go test ./... -count=1
go vet ./...
go build -o /tmp/ao-forge-smoke ./cmd/forge
/tmp/ao-forge-smoke doctor --foundation docs/foundation/foundation-baseline.v0.1.json
/tmp/ao-forge-smoke contract validate --schema docs/contracts/release-preview-audit-v0.1.schema.json --document examples/release-preview/dirty-workspace-blocked.audit.json
/tmp/ao-forge-smoke contract validate --schema docs/contracts/release-preview-inspect-v0.1.schema.json --document examples/release-preview/dirty-workspace-blocked.inspect.expected.json
/tmp/ao-forge-smoke contract validate --schema docs/contracts/release-artifact-inventory-v0.1.schema.json --document examples/release-preview/release-artifact-inventory.v0.1.example.json
/tmp/ao-forge-smoke contract validate --schema docs/contracts/release-attestation-plan-v0.1.schema.json --document examples/release-preview/release-attestation-plan.v0.1.example.json
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
