# AO Forge

AO Forge is the factory brain for the AO stack.

It does not replace AO2, ao2-control-plane, AO Covenant, AO Operator,
agy-swarms, or AO Conductor. It coordinates them into a higher-level agentic
factory:

- AO2 executes governed work and produces replayable evidence.
- ao2-control-plane stores and exposes read-only evidence.
- AO Covenant governs trust, policy, provenance, and release boundaries.
- AO Forge plans factory work, schedules workcells, routes evidence, and emits
  operator-ready go/no-go packets.

## Status

Phase 0 foundation with the Slice 0.2 CLI skeleton in progress. The repository
now includes design, contracts, fixtures, and a small Go CLI for deterministic
planning and packet inspection. Execution remains disabled until the Covenant
gate and AO2 adapter slices land.

## Product Thesis

Most orchestration systems either run agents directly or manage task queues.
AO Forge should do something stricter: turn a product objective into an
evidence-backed production line.

The factory should make every important transition explicit:

```text
objective -> factory brief -> workcell graph -> covenant policy gate
-> AO2 execution -> evidence routing -> control-plane readback
-> operator packet -> next factory decision
```

## First Vertical Slice

The first v0.1 slice is:

```text
factory plan -> task graph -> covenant policy gate -> AO2 execution adapter
-> control-plane readback -> operator packet
```

This slice is intentionally narrow. It proves the factory loop without taking
over provider execution, release publishing, or control-plane approval.

## Design Documents

- [AO Forge v0.1 Design](docs/design/AO-FORGE-V0.1.md)
- [Phase 0 Roadmap](docs/roadmap/PHASE-0.md)
- [Branch Protection Runbook](docs/release/BRANCH-PROTECTION.md)
- [Branch Protection Evidence](docs/release/BRANCH-PROTECTION-EVIDENCE.md)
- [First Public Release Checklist](docs/release/FIRST-PUBLIC-RELEASE.md)
- [v0.1.0 Release Notes Draft](docs/release/V0.1.0-RELEASE-NOTES.md)
- [v0.1.2 Release Notes Draft](docs/release/V0.1.2-RELEASE-NOTES.md)
- [Verified Foundation Baseline](docs/foundation/VERIFIED-BASELINE.md)
- [Foundation Baseline JSON](docs/foundation/foundation-baseline.v0.1.json)
- [Release Threat Model](docs/security/RELEASE-THREAT-MODEL.md)
- [GoalRun Contract](docs/design/GOAL-RUNS.md)
- [AO2 Pulse GoalRun Loop](docs/design/AO2-PULSE-GOAL-RUN-LOOP.md)
- [GoalRun v0.1 Schema](docs/contracts/goal-run-v0.1.schema.json)
- [GoalRun Update Audit v0.1 Schema](docs/contracts/goal-run-update-audit-v0.1.schema.json)
- [GoalRun Evidence Verify v0.1 Schema](docs/contracts/goal-run-evidence-verify-v0.1.schema.json)
- [GoalRun Evidence Lint v0.1 Schema](docs/contracts/goal-run-evidence-lint-v0.1.schema.json)
- [GoalRun Retained Evidence v0.1 Schema](docs/contracts/goal-run-retained-evidence-v0.1.schema.json)
- [GoalRun Retained Evidence Audit v0.1 Schema](docs/contracts/goal-run-retained-evidence-audit-v0.1.schema.json)
- [GoalRun Retained Evidence Cleanup v0.1 Schema](docs/contracts/goal-run-retained-evidence-cleanup-v0.1.schema.json)
- [GoalRun Readiness Audit v0.1 Schema](docs/contracts/goal-run-readiness-audit-v0.1.schema.json)
- [Factory Brief v0.1 Schema](docs/contracts/factory-brief-v0.1.schema.json)
- [Factory Plan v0.1 Schema](docs/contracts/factory-plan-v0.1.schema.json)
- [Factory Packet v0.1 Schema](docs/contracts/factory-packet-v0.1.schema.json)
- [Covenant Decision Fixture v0.1 Schema](docs/contracts/covenant-decision-fixture-v0.1.schema.json)
- [Covenant Gate Result v0.1 Schema](docs/contracts/covenant-gate-result-v0.1.schema.json)
- [Release Preview Audit v0.1 Schema](docs/contracts/release-preview-audit-v0.1.schema.json)
- [Release Preview Inspect v0.1 Schema](docs/contracts/release-preview-inspect-v0.1.schema.json)
- [Release Artifact Inventory v0.1 Schema](docs/contracts/release-artifact-inventory-v0.1.schema.json)
- [Release Attestation Plan v0.1 Schema](docs/contracts/release-attestation-plan-v0.1.schema.json)
- [Release Evidence Bundle v0.1 Schema](docs/contracts/release-evidence-bundle-v0.1.schema.json)
- [Release Verify Audit v0.1 Schema](docs/contracts/release-verify-audit-v0.1.schema.json)
- [Release Install Verify Audit v0.1 Schema](docs/contracts/release-install-verify-audit-v0.1.schema.json)
- [Release Rollback Audit v0.1 Schema](docs/contracts/release-rollback-audit-v0.1.schema.json)
- [Production Promotion Audit v0.1 Schema](docs/contracts/production-promotion-audit-v0.1.schema.json)
- [Production Readiness Audit v0.1 Schema](docs/contracts/production-readiness-audit-v0.1.schema.json)
- [Example GoalRun](examples/goals/ao2-weekend-hardening.goal-run.json)
- [Example GoalRun Update Audit](examples/goals/ao2-weekend-hardening.goal-run-update-audit.json)
- [AO2 Pulse Handoff GoalRun](examples/goals/ao2-pulse-handoff.goal-run.json)
- [AO2 Pulse Handoff Update Audit](examples/goals/ao2-pulse-handoff.goal-run-update-audit.json)
- [Retained GoalRun Evidence Fixture](examples/goals/ao2-retained-evidence.goal-run.json)
- [Release Preview Fixtures](examples/release-preview/)
- [Example Vertical Slice](examples/vertical-slices/risky-pr-factory.factory.json)
- [Example Deterministic Plan](examples/plans/risky-pr-factory-plan.json)
- [Example Covenant Gate Result](examples/gates/allow-local-plan.gate.json)
- [Example Factory Packet](examples/packets/risky-pr-factory-packet.json)

## Local CLI

AO Forge owns durable goal and task state for repeated hardening loops through
the `GoalRun` contract. AO2 Pulse or Codex can update a `GoalRun` after each
iteration; codex-cron should only trigger the loop. Before the next iteration,
the agent must read the latest `GoalRun` and prove the next action still matches
the objective, allowed scope, acceptance criteria, and stop conditions.
Persisted GoalRun evidence is retained under repository-relative durable paths,
not `tmp/` or machine-local directories; see the GoalRun retention policy before
preserving AO2 Pulse handoff state. The GoalRun fixture smoke rejects retained
evidence paths under `tmp/`, `/tmp/`, home directories, parent traversal, or
machine-local absolute paths.

Run the AO2 Pulse readiness entrypoint before a loop iteration:

```sh
scripts/ao2-pulse-goal-readiness.sh --goal-run examples/goals/ao2-retained-evidence.goal-run.json --to verification --out tmp/goal-run-readiness-audit.json
./bin/forge goal validate --goal-run examples/goals/ao2-weekend-hardening.goal-run.json
./bin/forge goal inspect --goal-run examples/goals/ao2-weekend-hardening.goal-run.json --json
./bin/forge goal transitions --goal-run examples/goals/ao2-weekend-hardening.goal-run.json --to implementation
./bin/forge goal readiness --goal-run examples/goals/ao2-retained-evidence.goal-run.json --to verification --json > tmp/goal-run-readiness-audit.json
./bin/forge contract validate --schema docs/contracts/goal-run-readiness-audit-v0.1.schema.json --document tmp/goal-run-readiness-audit.json
./bin/forge contract validate --schema docs/contracts/goal-run-readiness-audit-v0.1.schema.json --document docs/evidence/goals/ao2-weekend-hardening/20260619T180000Z-verification/goal-run-readiness-audit.json
./bin/forge goal update --goal-run examples/goals/ao2-weekend-hardening.goal-run.json --out tmp/ao2-weekend-hardening.goal-run.json --phase implementation --next-task "Implement the smallest verified AO2 hardening task." --evidence examples/goals/ao2-weekend-hardening.goal-run.json
./bin/forge contract validate --schema docs/contracts/goal-run-update-audit-v0.1.schema.json --document examples/goals/ao2-weekend-hardening.goal-run-update-audit.json
./bin/forge goal evidence verify --goal-run examples/goals/ao2-pulse-handoff.goal-run.json
./bin/forge goal evidence lint --goal-run examples/goals/ao2-retained-evidence.goal-run.json
./bin/forge goal evidence lint --update-audit examples/goals/ao2-pulse-handoff.goal-run-update-audit.json
./bin/forge goal evidence lint --goal-run examples/goals/ao2-retained-evidence.goal-run.json --json > tmp/goal-run-evidence-lint.json
./bin/forge contract validate --schema docs/contracts/goal-run-evidence-lint-v0.1.schema.json --document tmp/goal-run-evidence-lint.json
./bin/forge contract validate --schema docs/contracts/goal-run-retained-evidence-v0.1.schema.json --document docs/evidence/goals/ao2-weekend-hardening/20260619T143000Z-implementation/ao2-pulse-handoff-retention-proof.json
./bin/forge goal evidence retention --artifact docs/evidence/goals/ao2-weekend-hardening/20260619T143000Z-implementation/ao2-pulse-handoff-retention-proof.json --json > tmp/goal-run-retained-evidence-audit.json
./bin/forge contract validate --schema docs/contracts/goal-run-retained-evidence-audit-v0.1.schema.json --document tmp/goal-run-retained-evidence-audit.json
./bin/forge goal evidence cleanup --dry-run --json > tmp/goal-run-retained-evidence-cleanup.json
./bin/forge contract validate --schema docs/contracts/goal-run-retained-evidence-cleanup-v0.1.schema.json --document tmp/goal-run-retained-evidence-cleanup.json
./bin/forge goal evidence verify --goal-run examples/goals/ao2-pulse-handoff.goal-run.json --json > tmp/goal-run-evidence-verify.json
./bin/forge contract validate --schema docs/contracts/goal-run-evidence-verify-v0.1.schema.json --document tmp/goal-run-evidence-verify.json
```

Build and run the current skeleton:

```sh
go test ./...
go build -o bin/forge ./cmd/forge
./bin/forge --help
```

Create a deterministic factory plan from the first vertical-slice brief:

```sh
./bin/forge plan --brief examples/vertical-slices/risky-pr-factory.factory.json
./bin/forge plan --brief examples/vertical-slices/risky-pr-factory.factory.json --out tmp/factory-plan.json
```

Apply the local Covenant decision fixture gate:

```sh
./bin/forge gate \
  --plan examples/plans/risky-pr-factory-plan.json \
  --decision-fixture examples/decisions/allow-local-plan.decision.json
```

The gate can allow, deny, or block a plan. Deny and malformed decisions return
non-zero and still emit a machine-readable gate result. Execution remains
disabled after an allow decision until the AO2 adapter slice lands.

Inspect the example operator packet:

```sh
./bin/forge inspect --packet examples/packets/risky-pr-factory-packet.json
```

Validate a machine-readable contract document against its JSON Schema:
`forge contract validate --schema docs/contracts/release-preview-audit-v0.1.schema.json`

```sh
./bin/forge contract validate \
  --schema docs/contracts/release-preview-audit-v0.1.schema.json \
  --document examples/release-preview/dirty-workspace-blocked.audit.json
```

For automation, emit the validation result as JSON:

```sh
./bin/forge contract validate \
  --schema docs/contracts/release-preview-inspect-v0.1.schema.json \
  --document examples/release-preview/dirty-workspace-blocked.inspect.expected.json \
  --json
```

Validate the expected public release artifact inventory before the first public
tag:

```sh
./bin/forge contract validate \
  --schema docs/contracts/release-artifact-inventory-v0.1.schema.json \
  --document examples/release-preview/release-artifact-inventory.v0.1.example.json
```

Validate the public release attestation plan before signing or publishing
release artifacts:

```sh
./bin/forge contract validate \
  --schema docs/contracts/release-attestation-plan-v0.1.schema.json \
  --document examples/release-preview/release-attestation-plan.v0.1.example.json
```

Verify that the local workspace matches the verified baseline:

```sh
./bin/forge doctor --foundation docs/foundation/foundation-baseline.v0.1.json
```

To output the doctor result in machine-readable JSON:

```sh
./bin/forge doctor --foundation docs/foundation/foundation-baseline.v0.1.json --json
```

Write a stable SHA-256 checksum manifest for release artifacts:

```sh
./bin/forge artifact checksums \
  --artifact ./dist/ao-forge_Darwin_arm64.tar.gz \
  --out ./dist/checksums.txt
```

Verify that every artifact still matches a checksum manifest:

```sh
./bin/forge artifact verify-checksums --manifest ./dist/checksums.txt
```

Rehearse a release without creating tags, pushing refs, or publishing GitHub
releases:

```sh
./bin/forge release-preview \
  --workspace . \
  --artifact ./dist/ao-forge_Darwin_arm64.tar.gz \
  --out release-preview-audit.json
```

The preview audit is machine-readable JSON with the resolved tag, HEAD commit,
GitHub repository, release checks, artifact sizes, checksums, and next actions.
See `docs/release/PREVIEW-RELEASE.md` for the operator runbook.

Inspect a release preview audit without reading raw JSON:

```sh
./bin/forge release-preview inspect --audit release-preview-audit.json
```

For automation, emit the inspect summary as JSON:

```sh
./bin/forge release-preview inspect --audit release-preview-audit.json --json
```

Compute the repository production-readiness score from checked-in contracts,
workflows, runbooks, and retained evidence controls:

```sh
./bin/forge production-readiness audit --json
```

Mutating commands fail closed unless the required gate result, clean workspace,
explicit operator confirmation, and release preview evidence are present.

Pull requests and pushes to `main` also run the non-mutating release preview
audit workflow. It uploads `release-preview-audit.json`,
`release-preview-inspect.txt`, `release-preview-inspect.json`, and
`checksums.txt` as CI artifacts.

The `Release Rehearsal` workflow provides a tagged release rehearsal without
publishing. Run it manually with the intended tag, or let it run on pushed `v*`
tags, then review the uploaded `release-rehearsal-evidence` artifact before any
public release mutation.

The `Release Attestation` workflow builds the expected preview artifacts,
generates and verifies `checksums.txt`, runs release preview, creates GitHub
Artifact Attestations for the checksum subjects, verifies the GitHub Artifact Attestations
against the expected repository, ref, commit, and artifact digests, and does not
publish a GitHub release.

The `Release Publish` workflow is manual-only and draft-only. It requires a
successful `Release Rehearsal` run ID, an existing signed annotated tag that
resolves to the publish commit, explicit `confirm_publish=true`, fresh checksums,
release preview, contract validation, GitHub Artifact Attestation verification,
and public-safe release notes before it creates a draft GitHub release. It also
generates `release-evidence-bundle.json`, validates it against the release
evidence bundle contract, signs it with a GitHub Artifact Attestation, verifies
that attestation, and uploads both bundle files with the release assets.

The `Release Verify` workflow is read-only post-release verification. It runs on
published GitHub releases, can be dispatched manually for a tag, and runs on a
weekly schedule for the promoted `v0.1.2` release. It checks release metadata,
expected assets, checksums, release preview evidence, archive attestations, the
release evidence bundle, bundle attestation, and a host-compatible binary smoke
test. Future releases require both the evidence bundle and a signed annotated
release tag by default; use explicit legacy overrides only for releases
published before those controls existed.
It uploads `release-verify-audit.json` so promotion can validate the exact
post-release controls used for the tag.
Signed release tags must be made by an active signer in
`RELEASE-SIGNERS.json`; the public keys live under `docs/release/signers/` and
are imported by the release workflows before tag verification.

The `Release Install Verify` workflow is read-only public asset installation
verification. It downloads the published release assets from GitHub, verifies
checksums, extracts the platform archives, runs the Linux binary from the
downloaded asset, and uploads `release-install-verify-audit.json`.

The `Release Rollback` workflow is the guarded release yank path. It is
manual-only and always requires explicit `confirm_rollback=true` plus a public
correction reason. The `audit-only` path is read-only and emits rollback
evidence without production environment approval; `mark-prerelease` and
`mark-draft` require the `production-release` environment before using
`contents: write`. Rollback audit evidence includes normalized before/after
release comparison for mutation-relevant fields. Rollback must not delete
releases, tags, assets, or evidence.

The `Production Promotion` workflow is the read-only gate for production-stable
release language. It requires a successful `Release Verify` run with
contract-valid verify audit evidence proving default signed-tag and
evidence-bundle controls, a successful `Release Install Verify` run with
contract-valid install audit evidence for the same release, a successful
`Release Rollback` audit-only run, rollback evidence proving mutation-relevant
release fields stayed unchanged, and a completed soak window before it uploads
`production-promotion-audit.json`.
Until that audit passes, releases should stay described as public-preview or
candidate releases. After it passes, production-stable language must stay
within the scope proven by the promotion evidence.

## Continuous Integration

This repository is public. The GitHub Actions workflow runs automatically on every push to a branch and every pull request targeting `main`.

Before pushing or merging work, you can run the local verification checks:

```sh
go run github.com/rhysd/actionlint/cmd/actionlint@v1.7.12 -shellcheck= -pyflakes= .github/workflows/*.yml
go test ./...
go vet ./...
```
