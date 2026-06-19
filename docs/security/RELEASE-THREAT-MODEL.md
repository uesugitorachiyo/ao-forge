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
| Tag spoofing | A release tag already points at a different commit, is replaced with a lightweight tag, or is signed by an unapproved key. | `forge release-preview` resolves the tag and checks whether it is available or already points to HEAD. `Release Publish` requires an existing signed annotated tag that resolves to the publish commit, imports public keys from `docs/release/signers/`, and checks the signing fingerprint against `RELEASE-SIGNERS.json`. `Release Verify` applies the same signer policy by default. | `release-preview-audit.json`, `release-preview-inspect.txt`, `Release Publish` logs with `signed_release_tag_verified`, `release_tag_signer`, and `Release Verify` logs with `release_tag_identity_verified`. | Keep signer policy evidence attached to every promoted public release. |
| Artifact tampering | An artifact changes between build and release review or a published archive cannot be installed by users. | `forge artifact checksums` emits a stable SHA-256 manifest, `forge release-preview` records artifact size, checksum, status, and provenance, `release-artifact-inventory.v0.1.example.json` defines the expected public artifact set, `release-attestation-plan.v0.1.example.json` defines the signed evidence plan, `release-evidence-bundle.json` binds the release tag, commit, workflow run, rehearsal run, checksums, preview evidence, inventory, attestation plan, and attestation readbacks, `Release Attestation` produces and verifies GitHub Artifact Attestations before uploading evidence, and `Release Install Verify` downloads public assets from GitHub, verifies checksums, extracts archives, and runs a Linux install smoke test. | `checksums.txt`, release preview artifact details, the release artifact inventory contract, the release attestation plan contract, `release-evidence-bundle.json`, GitHub Artifact Attestation for `release-evidence-bundle.json`, `dist/*.attestation.json`, and `release-install-verify-audit.json`. | Keep evidence-bundle validation mandatory for promoted public releases. |
| Credential exposure | Release preview accidentally depends on write credentials or stores tokens in output. | The release preview workflow uses `contents: read`; preview commands do not require network access and do not store credentials. | Workflow YAML, audit fields `mutates_releases=false` and `network_required=false`. | Secret scanning remains required before public pushes. |
| Workflow permission escalation | CI preview gains write permissions or live release flags. | Release preview workflow permissions are read-only, and tests forbid `contents: write`, `GITHUB_TOKEN:`, `--live`, and `--confirm-release` in the preview workflow. CI runs `actionlint` against `.github/workflows` to catch invalid GitHub Actions contexts before merge. Branch protection requires CI, Workflow lint, and Release Preview before merge, includes admins, requires linear history, and blocks force pushes and deletions. The `Release Publish` workflow is manual-only, draft-only, and requires explicit confirmation plus the `production-release` environment before using `contents: write`. `Release Rollback` keeps audit-only evidence read-only and gates mutating rollback actions behind the `production-release` environment before using `contents: write`. | `TestReleasePreviewWorkflowPublishesDryRunAuditArtifacts`, `TestReleasePublishWorkflowCreatesDraftReleaseOnlyAfterEvidenceGates`, `TestReleaseRollbackWorkflowGuardsReleaseYankActions`, `TestBranchProtectionRunbookDocumentsRequiredChecks`, [Branch Protection Runbook](../release/BRANCH-PROTECTION.md), [Branch Protection Evidence](../release/BRANCH-PROTECTION-EVIDENCE.md), and GitHub Actions logs. | Keep branch-protection evidence current when workflow job names or required checks change. |
| Stale release evidence | A live release uses an old audit from another commit or workspace. | Live release validation checks the passed audit, local-only preview status, workspace binding, tag, and HEAD commit before mutation. `Release Publish` also requires a successful `Release Rehearsal` run ID, an existing signed annotated tag for the publish commit, and regenerated release preview, checksums, verified attestations, and an attested release evidence bundle in the publish job. `Production Promotion` requires successful Release Verify audit evidence bound to the promoted tag and commit with default signed-tag and evidence-bundle controls, contract-valid Release Install Verify audit evidence bound to the promoted tag and commit, and Release Rollback audit-only evidence with normalized unchanged-field comparison plus a soak window before production-stable language is allowed. Scheduled `Release Verify` reruns strict post-release checks for promoted releases. | Factory packet policy decisions, release preview audit fields, `release-rehearsal-evidence`, `release-publish-evidence`, signed tag verification logs, `release-evidence-bundle.attestation.json`, `release-verify-audit.json`, `release-install-verify-audit.json`, `release-rollback-audit.json`, `production-promotion-audit.json`, and scheduled Release Verify logs. | Expand scheduled drift verification when more promoted releases exist. |
| Post-release drift | A published release is missing assets, points at the wrong tag target, or has unverifiable evidence. | `Release Verify` runs read-only post-release verification on published releases, manual tag dispatch, and a weekly schedule for promoted releases. It checks release metadata, expected assets, checksum verification, release preview inspect status, archive attestations, release evidence bundle validation, bundle attestation verification, and a host-compatible binary smoke test, then uploads contract-valid `release-verify-audit.json`. | `Release Verify` workflow logs, step summary `post_release_verification=passed`, and `release-verify-audit.json`. | Turn scheduled verification failures into guided public correction steps. |
| Bad release remains discoverable | A release is known-bad but stays presented as a normal public release. | `Release Rollback` is manual-only, requires `confirm_rollback=true`, records read-only `release-rollback-audit.json` evidence for `audit-only` including normalized before/after release comparison, appends a public correction note for mutating actions, and can mark the release as prerelease or draft only after `production-release` environment approval. It must not delete tags, assets, or evidence. | `Release Rollback` workflow logs, `release-rollback-audit` artifact, normalized mutation comparison, and the release correction note. | Corrective releases must use a new signed annotated tag rather than replacing original assets. |
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

AO Forge now has a proven solo-operator production-promotion chain for v0.1.2,
live branch-protection evidence for `main`, and scheduled post-release drift
verification for the promoted release. It is not yet broad multi-user
production infrastructure. The next hardening step is guided public correction
handling for failed drift checks.
