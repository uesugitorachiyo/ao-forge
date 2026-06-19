# AO Forge Release Threat Model

This document maps likely release-path attacks to the controls AO Forge already
enforces or documents. It is scoped to public-preview release readiness for the
AO Forge repository and its local release preview workflow.

## Scope

In scope:

- local `forge release-preview` rehearsals;
- artifact checksum generation and inspection;
- GitHub Actions release preview dry-runs;
- operator review before live release mutation;
- public documentation needed for a fresh maintainer to reason about release
  risk.

Out of scope for this document:

- GitHub account compromise;
- package registry compromise after an artifact has left AO Forge;
- private operator workspaces, local secrets, or unpublished handoff notes;
- AO2, AO Covenant, or ao2-control-plane internals beyond the evidence they
  provide to AO Forge.

## Trust Boundaries

- AO Forge release preview is local-first and non-mutating.
- GitHub Actions release preview runs with `contents: read`.
- Live release mutation requires explicit operator confirmation and release
  preview evidence.
- Release artifacts are treated as untrusted inputs until checksummed and
  included in release preview evidence.
- Public docs must not require private local paths, secrets, or excluded
  handoff material.

## Release Attack Map

| Attack | Example failure mode | Current control | Operator evidence | Remaining gap |
| --- | --- | --- | --- | --- |
| Tag spoofing | A release tag already points at a different commit. | `forge release-preview` resolves the tag and checks whether it is available or already points to HEAD. | `release-preview-audit.json`, `release-preview-inspect.txt`, and `release-preview-inspect.json` show `tag`, `head_commit`, and release-tag check status. | A future signed-tag policy can make tag identity stronger. |
| Artifact tampering | An artifact changes between build and release review. | `forge artifact checksums` emits a stable SHA-256 manifest, `forge release-preview` records artifact size, checksum, status, and provenance, `release-artifact-inventory.v0.1.example.json` defines the expected public artifact set, `release-attestation-plan.v0.1.example.json` defines the signed evidence plan, `release-evidence-bundle.json` binds the release tag, commit, workflow run, rehearsal run, checksums, preview evidence, inventory, attestation plan, and attestation readbacks, and `Release Attestation` produces and verifies GitHub Artifact Attestations before uploading evidence. | `checksums.txt`, release preview artifact details, the release artifact inventory contract, the release attestation plan contract, `release-evidence-bundle.json`, GitHub Artifact Attestation for `release-evidence-bundle.json`, and `dist/*.attestation.json`. | A future signed-tag policy can make tag identity stronger across repository mirrors. |
| Credential exposure | Release preview accidentally depends on write credentials or stores tokens in output. | The release preview workflow uses `contents: read`; preview commands do not require network access and do not store credentials. | Workflow YAML, audit fields `mutates_releases=false` and `network_required=false`. | Secret scanning remains required before public pushes. |
| Workflow permission escalation | CI preview gains write permissions or live release flags. | Release preview workflow permissions are read-only, and tests forbid `contents: write`, `GITHUB_TOKEN:`, `--live`, and `--confirm-release` in the preview workflow. The `Release Publish` workflow is manual-only, draft-only, and requires explicit confirmation plus the `production-release` environment before using `contents: write`. | `TestReleasePreviewWorkflowPublishesDryRunAuditArtifacts`, `TestReleasePublishWorkflowCreatesDraftReleaseOnlyAfterEvidenceGates`, and GitHub Actions logs. | Branch protection should require CI and Release Preview before merge; see the [Branch Protection Runbook](../release/BRANCH-PROTECTION.md). |
| Stale release evidence | A live release uses an old audit from another commit or workspace. | Live release validation checks the passed audit, local-only preview status, workspace binding, tag, and HEAD commit before mutation. `Release Publish` also requires a successful `Release Rehearsal` run ID and regenerates release preview, checksums, verified attestations, and an attested release evidence bundle in the publish job. | Factory packet policy decisions, release preview audit fields, `release-rehearsal-evidence`, `release-publish-evidence`, and `release-evidence-bundle.attestation.json`. | A future signed-tag policy can make tag identity stronger across repository mirrors. |
| Post-release drift | A published release is missing assets, points at the wrong tag target, or has unverifiable evidence. | `Release Verify` runs read-only post-release verification on published releases or manual tag dispatch. It checks release metadata, expected assets, checksum verification, release preview inspect status, archive attestations, release evidence bundle validation, bundle attestation verification, and a host-compatible binary smoke test. | `Release Verify` workflow logs and step summary `post_release_verification=passed`. | A future rollback/yank workflow can turn verification failures into guided correction steps. |
| Bad release remains discoverable | A release is known-bad but stays presented as a normal public release. | `Release Rollback` is manual-only, environment-gated, requires `confirm_rollback=true`, records `release-rollback-audit.json`, appends a public correction note for mutating actions, and can mark the release as prerelease or draft without deleting tags, assets, or evidence. | `Release Rollback` workflow logs, `release-rollback-audit` artifact, and the release correction note. | A future signed-tag policy can make corrective tags stronger across repository mirrors. |
| Dirty workspace release | Local uncommitted changes are released without review. | The dry-run script refuses dirty worktrees, and `forge release-preview` records clean-worktree status. | Script stderr on refusal or audit check `clean-worktree=passed`. | Release packaging should continue to run from clean, reproducible build steps. |
| Malformed audit injection | A hand-written audit omits checks or weakens schema shape. | Audit loading validates schema version, status, checks, artifacts, next actions, and release invariants before use. | CLI validation errors and tests for malformed release preview audits. | Formal schema validation for inspect JSON can further harden downstream consumers. |

## Operator Rules

- Run release preview before any confirmed live release path.
- For the first public release, follow
  [First Public Release Checklist](../release/FIRST-PUBLIC-RELEASE.md).
- Review both human and machine-readable inspect output:
  - `forge release-preview inspect --audit release-preview-audit.json`
  - `forge release-preview inspect --audit release-preview-audit.json --json`
- Treat `forge release-preview inspect --json` as the automation-facing summary
  mode for release preview evidence.
- Generate checksum manifests with `forge artifact checksums`; do not rely on
  platform-specific shell checksum commands for the public release path.
- Do not run live release mutation when the preview audit status is `blocked`.
- Do not commit ad hoc local release audits unless paths and metadata are safe
  for the public repository.
- Do not commit ad hoc local release audits from private workspaces, excluded
  folders, or machines containing sensitive local paths.

## Public-Preview Readiness

AO Forge has enough controls for public-preview release rehearsal:

- read-only CI release preview;
- non-mutating audit generation;
- artifact checksum evidence;
- JSON inspect output for automation;
- tests that prevent accidental live release flags in preview workflow;
- secret scanning in the recommended release-readiness gate.

It is not yet full production-stable release infrastructure. The next hardening
steps are a signed-tag policy for stronger tag identity and promotion criteria
for declaring releases production-stable.
