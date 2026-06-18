# AO Forge Release Preview

`forge release-preview` rehearses a release without mutating local or remote
release state. It is intended to run before `forge run --live --confirm-release`
or any manual GitHub release operation.

## What It Checks

- workspace path exists and is a git repository;
- worktree is clean;
- HEAD commit resolves;
- `origin` resolves to a GitHub repository;
- release tag is either available for creation or already points to HEAD;
- declared artifacts exist and have SHA-256 checksums;
- the preview itself does not create tags, push refs, publish releases, or
  require network access.

## Example

Generate the checksum manifest with AO Forge before running the preview:

```sh
forge artifact checksums \
  --artifact ./dist/ao-forge_Linux_x86_64.tar.gz \
  --out ./dist/checksums.txt
```

```sh
forge release-preview \
  --workspace . \
  --artifact ./dist/ao-forge_Linux_x86_64.tar.gz \
  --artifact ./dist/checksums.txt \
  --out release-preview-audit.json
```

If `--tag` is omitted, AO Forge resolves it from `AO_FORGE_RELEASE_TAG` or the
workspace `VERSION` file. The command exits `0` when every check passes. It exits
`1` and still writes the audit when release preview checks are blocked.

Inspect the audit summary before approving a confirmed release:

```sh
forge release-preview inspect --audit release-preview-audit.json
```

Automation can consume the same inspect summary as JSON:

```sh
forge release-preview inspect --audit release-preview-audit.json --json
```

GitHub Actions runs the same non-mutating preview path on pull requests and
pushes to `main`. The workflow uploads the machine-readable audit, an inspect
summary, a JSON inspect summary, and artifact checksums for review without using
write permissions or live release flags.

## Tagged Release Rehearsal

The `Release Rehearsal` workflow rehearses tagged release evidence without
publishing a GitHub release, pushing refs, or requiring write permissions. It can
run manually with a tag input before the first public release, and it also runs
on pushed `v*` tags as a final non-mutating evidence check.

The workflow uses `scripts/release-preview-dry-run.sh` with
`AO_FORGE_RELEASE_PREVIEW_TAG` set to the selected tag, validates the audit and
inspect JSON schema versions, reads back the artifact inventory and attestation plan,
confirms the evidence is passed, local-only, and non-mutating, then uploads
`release-rehearsal-evidence`.

Before publishing a real release, review the uploaded evidence artifact and
confirm it matches the intended tag and commit. For the first public release,
follow `FIRST-PUBLIC-RELEASE.md`.

## Audit Contract

The audit uses schema version `ao.forge.release-preview-audit.v0.1`. The formal
schema is published at `docs/contracts/release-preview-audit-v0.1.schema.json`
and includes:

- `status`: `passed` or `blocked`;
- `workspace`, `github_repo`, `tag`, and `head_commit`;
- `mutates_releases: false` and `network_required: false`;
- structured `checks`;
- artifact `path`, `size_bytes`, `sha256`, `status`, and `provenance`;
- `release_notes_preview`;
- `next_actions`.

Committed [release preview fixtures](../../examples/release-preview/) cover
artifact checksum and inspect JSON behavior for maintainers changing this path.
They include `dirty-workspace-blocked.audit.json`, a fail-closed dirty-workspace preview,
plus its expected inspect JSON so blocked release
evidence remains reviewable and regression-tested.

The JSON inspect summary uses schema version
`ao.forge.release-preview-inspect.v0.1`. Its formal schema is published at
`docs/contracts/release-preview-inspect-v0.1.schema.json` and includes the
source audit schema version, pass/fail counts, artifact details, and next
actions.

## Release Artifact Inventory

The expected public artifact inventory uses schema version
`ao.forge.release-artifact-inventory.v0.1`. The formal schema is published at
`docs/contracts/release-artifact-inventory-v0.1.schema.json`, and the public-safe
example inventory lives at
`../../examples/release-preview/release-artifact-inventory.v0.1.example.json`.

Use this inventory as the release checklist for expected archives, platform
coverage, checksum manifest requirements, provenance source, build workflow, and
required attestations. Keep it free of ad hoc local paths, private operator
state, credentials, or unreleased customer data.

```sh
forge contract validate \
  --schema docs/contracts/release-artifact-inventory-v0.1.schema.json \
  --document examples/release-preview/release-artifact-inventory.v0.1.example.json
```

## Release Attestation Plan

The signed evidence plan uses schema version
`ao.forge.release-attestation-plan.v0.1`. The formal schema is published at
`docs/contracts/release-attestation-plan-v0.1.schema.json`, and the public-safe
example lives at
`../../examples/release-preview/release-attestation-plan.v0.1.example.json`.

Use this plan to bind release archives, checksum manifests, release preview
evidence, and artifact inventory to a signer identity. The v0.1 example assumes
GitHub Actions OIDC keyless signing for the eventual release workflow. It does
not store private keys, local absolute paths, or operator secrets.

```sh
forge contract validate \
  --schema docs/contracts/release-attestation-plan-v0.1.schema.json \
  --document examples/release-preview/release-attestation-plan.v0.1.example.json
```

## Operator Rule

Do not run a live confirmed release if the preview audit is `blocked`. Fix the
failed checks, regenerate artifacts if needed, rerun the preview, and only then
proceed to release mutation.

Review the [Release Threat Model](../security/RELEASE-THREAT-MODEL.md) when
changing release workflow permissions, artifact handling, or live release
mutation gates.

Confirmed release mutation through `forge run`, `forge once`, or `forge resume`
requires the passed audit:

```sh
forge run \
  --plan factory-plan.json \
  --gate-result gate-result.json \
  --live \
  --confirm-release \
  --release-preview-audit release-preview-audit.json
```

AO Forge validates that the audit is passed, non-mutating, local-only, and bound
to the same workspace, tag, and HEAD commit before release mutation can proceed.

## Privacy

The preview audit can include local workspace and artifact paths. Keep ad hoc
operator audits out of public commits unless the paths are intentionally public
and safe to disclose.
