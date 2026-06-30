# AO Forge

AO Forge is the factory brain for the AO stack and the central place to
understand how the AO repositories work together.

## AO Stack Architecture

This repository is part of the AO agent orchestration stack. Start with the
central architecture guide at
[uesugitorachiyo/ao-architecture](https://github.com/uesugitorachiyo/ao-architecture);
the AO Forge-specific architecture page is
[ao-forge](https://github.com/uesugitorachiyo/ao-architecture/tree/main/ao-forge).

It does not replace AO2, ao2-control-plane, AO Covenant, or AO Command.
Deprecated AO Operator/AO Conductor paths and archived `agy-swarms` materials
are reference-only for AO Foundry work; AO Forge keeps legacy compatibility
paths only behind explicit operator opt-in. Forge coordinates the active stack
into a higher-level agentic factory where work is planned, policy-gated,
executed, evidenced, reviewed, and promoted only when the required gates pass.

## AO Stack Map

The current AO stack is split by authority instead of by convenience. Each repo
owns a different part of the factory so no single component silently plans,
executes, approves, stores, and promotes work by itself.

| Repo | Role | Owns | Must not own |
| --- | --- | --- | --- |
| [AO Forge](https://github.com/uesugitorachiyo/ao-forge) | Factory brain | Factory plans, workcell scheduling, GoalRun state, release gates, production-readiness scoring, retained-evidence policy, operator packets | Provider execution, final policy authority, evidence archive UI, operator command UX |
| [AO2](https://github.com/uesugitorachiyo/ao2) | Governed execution engine | Local agent/work execution, policy-checked runs, exact-digest approvals, replayable evidence, evaluator closure | Cross-repo factory scheduling, release promotion policy, control-plane approval |
| [ao2-control-plane](https://github.com/uesugitorachiyo/ao2-control-plane) | Evidence observer | Signed evidence ingest, content-addressed storage, authenticated read APIs, dashboards, backup/restore and retention reports | Approving AO2 runs, executing providers, deciding release readiness |
| [AO Covenant](https://github.com/uesugitorachiyo/ao-covenant) | Trust and policy kernel | Contracts, policy decisions, provenance, declared side-effect gates, approval tickets, tamper-evident evidence packs | Running the whole factory, storing all AO2 evidence, operator dashboard UX |
| [AO Command](https://github.com/uesugitorachiyo/ao-command) | Operator command surface | Read-only status, next actions, GoalRun inspection, evidence validation, release rehearsal summaries | Replacing AO Forge policy, mutating releases by default, provider writes |

The practical rule is:

```text
AO Command shows what is happening.
AO Forge decides what factory step is allowed next.
AO Covenant decides whether declared side effects are trusted.
AO2 executes governed work and produces evidence.
ao2-control-plane stores and exposes evidence after the fact.
```

## How Work Moves Through The Stack

The normal factory path is evidence-first:

```text
operator objective
-> AO Command status / next-action view
-> AO Forge GoalRun and factory plan
-> AO Covenant policy and side-effect gate
-> AO2 governed execution
-> AO2 evidence pack
-> optional ao2-control-plane ingest and readback
-> AO Forge operator packet, release gate, or production-readiness audit
-> AO Command operator summary
```

AO Forge is intentionally in the middle of this path. It keeps durable task
state and readiness semantics close to the factory contracts, while leaving
execution, trust, evidence storage, and operator UX in the repos that own those
responsibilities.

`agy-swarms` compatibility remains only for legacy/reference scenarios. Dynamic
planning through `forge plan --dynamic` requires
`AO_FORGE_ENABLE_ARCHIVED_AGY_SWARMS=1`; active plans should use AO2 execution
paths.

## Repo Boundaries

- AO Forge owns durable goal/task state through `GoalRun`. AO2 Pulse, Codex, or
  another loop runner may propose updates, but the next iteration must read the
  latest GoalRun and prove the next action still matches objective, scope,
  acceptance criteria, and stop conditions.
- AO Forge owns release-preview, release-verify, install-verify, rollback,
  promotion, retained-evidence, and production-readiness audit contracts.
- AO Forge treats AO Covenant decisions as gates. Missing, malformed, denied,
  or blocked decisions fail closed.
- AO Forge treats AO2 as the execution adapter. AO2 produces local, replayable
  evidence; Forge should not replace AO2's evaluator closure or evidence model.
- AO Forge treats ao2-control-plane as an observer. Control-plane publication or
  readback can support an operator decision, but upload is never approval.
- AO Command consumes AO Forge and Covenant evidence for humans. It is read-only
  by default and should not mutate provider state or release state without a
  future explicit operator-approved path.

## Daily Operating Model

For day-to-day work, start from the operator surface and then drill down into the
owning repo:

1. Use AO Command to ask "what is the current status and next action?"
2. Use AO Forge to inspect the GoalRun, production-readiness audit, release
   preview, promotion gate, or retained-evidence policy that explains the
   recommendation.
3. Use AO Covenant evidence to understand why a side effect was allowed,
   denied, or blocked.
4. Use AO2 evidence to inspect what actually ran and whether evaluator closure
   was reached.
5. Use ao2-control-plane when evidence has been published and should be read
   through the durable observer instead of a local run directory.

This keeps the stack operator-friendly without weakening the separation of
authority that makes the factory auditable.

## Status

AO Forge v0.1.x now has schema-backed factory contracts, guarded dry-run and
live execution paths, durable GoalRun state, release preview, rehearsal,
publish, verify, install, rollback, promotion, retained-evidence, and
production-readiness gates. Mutating paths remain fail-closed behind Covenant
decisions, clean-workspace checks, explicit operator confirmation, release
preview evidence, and release or promotion workflow gates.

The live-mutation dry-run plan contract adds a narrower pre-authority rehearsal
surface for future governed repository mutation. It now models the docs-only
mutation classes `docs_only_single_file` and `docs_only_multi_file`, `test_only`,
and a request-only `low_risk_code` dry-run boundary while keeping
`tiny_documentation_change` as the legacy single-file docs alias. It
requires Covenant authority, isolated branch/worktree planning, verification,
PR lifecycle, rollback rehearsal, provider boundaries, and an operator kill
switch.
The contract does not permit live repository mutation.
The live-docs execution guard turns an approved docs-only request into a
machine-readable eligibility result:

```sh
forge live-docs guard \
  --plan examples/live-mutation/docs-only-dry-run-plan.json \
  --approval-gate examples/live-docs-guard/foundry-approval-gate.ready.json \
  --ticket examples/live-docs-guard/covenant-ticket.approved.json \
  --sentinel examples/live-docs-guard/sentinel.no-hold.json \
  --command-readback examples/live-docs-guard/command-readback.ready.json \
  --out tmp/live-docs-execution-guard.json
```

The guard is fail-closed. It requires the Foundry approval gate to report
class-appropriate authority, the Covenant ticket to be approved, unexpired,
unconsumed, and approver-bound, the dry-run plan to stay within the class
allowlist, Sentinel to report no hold, and AO Command readback to remain
`operator_mode=read_only` with `mutates_repositories=false`.
For `docs_only_multi_file` and `test_only`, the guard accepts the class-bound
Foundry gate, Covenant mutation-class ticket, Sentinel mutation-class no-hold
verdict, and Atlas authority-ladder Command readback. Test-only plans are
limited to one `*_test.go` path and emit
`authority_boundary=test_only_class_only`. For `low_risk_code`, the guard may
emit `safe_to_request=true` only when all low-risk dry-run evidence is present,
but it must keep `safe_to_execute=false` and
`authority_boundary=low_risk_code_dry_run_only`. Multi-repo, complex, and fully
unsupervised mutation classes remain denied.

Even when the guard emits `safe_to_execute=true` for lower classes, it is only
an eligibility artifact for the current class-bound PR rehearsal chain. It does
not execute work, mutate repositories, create branches, call providers, publish,
tag, upload, bypass rollback or kill-switch evidence, or widen the path into
fully unsupervised complex live mutation.

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

## Public Documentation

Start with the focused public docs:

- [AO Forge v0.1 Architecture](docs/design/AO-FORGE-V0.1.md)
- [GoalRun Contract](docs/design/GOAL-RUNS.md)
- [AO2 Pulse GoalRun Loop](docs/design/AO2-PULSE-GOAL-RUN-LOOP.md)
- [Public Repo Policy](docs/security/PUBLIC-REPO-POLICY.md)
- [Release Threat Model](docs/security/RELEASE-THREAT-MODEL.md)
- [Branch Protection Runbook](docs/release/BRANCH-PROTECTION.md)
- [Production Readiness Audit Schema](docs/contracts/production-readiness-audit-v0.1.schema.json)
- [Live Mutation Dry-Run Plan Schema](docs/contracts/live-mutation-dry-run-plan-v0.1.schema.json)

For the full public documentation index, including schemas, release docs,
foundation evidence, and examples, see [docs/README.md](docs/README.md).

## Local CLI

AO Forge owns durable goal and task state for repeated hardening loops through
the `GoalRun` contract. AO2 Pulse or Codex can update a `GoalRun` after each
iteration; an external scheduler may only trigger the loop. Before the next
iteration, the agent must read the latest `GoalRun` and prove the next action
still matches the objective, allowed scope, acceptance criteria, and stop
conditions.
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
./bin/forge goal context validate --goal-run examples/goals/ao2-weekend-hardening.goal-run.json --handoff examples/goals/ao2-weekend-hardening.context-handoff.json --now 2026-06-26T12:00:00Z
./bin/forge contract validate --schema docs/contracts/goal-run-context-handoff-v0.1.schema.json --document examples/goals/ao2-weekend-hardening.context-handoff.json
./bin/forge goal verification validate --verification examples/goals/ao2-weekend-hardening.goal-run-verification.json
./bin/forge contract validate --schema docs/contracts/goal-run-verification-v0.1.schema.json --document examples/goals/ao2-weekend-hardening.goal-run-verification.json
./bin/forge goal update --goal-run examples/goals/ao2-weekend-hardening.goal-run.json --out tmp/ao2-weekend-hardening.goal-run.json --phase implementation --next-task "Implement the smallest verified AO2 hardening task." --evidence examples/goals/ao2-weekend-hardening.goal-run.json
./bin/forge contract validate --schema docs/contracts/goal-run-update-audit-v0.1.schema.json --document examples/goals/ao2-weekend-hardening.goal-run-update-audit.json
./bin/forge goal evidence verify --goal-run examples/goals/ao2-pulse-handoff.goal-run.json
./bin/forge goal evidence lint --goal-run examples/goals/ao2-retained-evidence.goal-run.json
./bin/forge goal evidence lint --update-audit examples/goals/ao2-pulse-handoff.goal-run-update-audit.json
./bin/forge goal evidence lint --goal-run examples/goals/ao2-retained-evidence.goal-run.json --json > tmp/goal-run-evidence-lint.json
./bin/forge contract validate --schema docs/contracts/goal-run-evidence-lint-v0.1.schema.json --document tmp/goal-run-evidence-lint.json
./bin/forge contract validate --schema docs/contracts/goal-run-retained-evidence-v0.1.schema.json --document docs/evidence/goals/ao2-weekend-hardening/20260619T143000Z-implementation/ao2-pulse-handoff-retention-proof.json
./bin/forge contract validate --schema docs/contracts/goal-run-retained-evidence-v0.1.schema.json --document docs/evidence/goals/ao2-weekend-hardening/20260619T180000Z-verification/ao-command-rsi-manifest-retention-proof.json
./bin/forge contract validate --schema docs/contracts/goal-run-retained-evidence-v0.1.schema.json --document docs/evidence/goals/ao2-weekend-hardening/20260619T180000Z-verification/bounded-rsi-improvement-chain-retention-proof.json
./bin/forge contract validate --schema docs/contracts/goal-run-retained-evidence-v0.1.schema.json --document docs/evidence/goals/ao2-weekend-hardening/20260619T180000Z-verification/ao-stack-rsi-chain-binding-readback-retention-proof.json
./bin/forge contract validate --schema docs/contracts/architecture-rsi-pin-readback-v0.1.schema.json --document docs/evidence/architecture/ao-architecture-rsi-pin-readback.json
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
non-zero and still emit a machine-readable gate result. Allowed plans still fail
closed unless the selected execution mode, clean workspace, and required
operator or release confirmations are present.

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

Validate the checked-in release candidate handoff before preview, publish, or
promotion work:

```sh
./bin/forge release-candidate validate --candidate examples/release-preview/release-candidate.v0.1.example.json
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
workflows, runbooks, retained evidence controls, and public-repo policy gates:

```sh
scripts/check-public-repo-policy.sh
./bin/forge production-readiness audit --json
```

The production-readiness audit includes
`goalrun.architecture_rsi_pin_readback`, which requires
`ao-architecture-rsi-pin-readback.json` to show that AO Architecture pins Forge's
retained RSI proofs and that AO Command PR #32 fails closed when those pins are
missing.

Mutating commands fail closed unless the required gate result, clean workspace,
explicit operator confirmation, and release preview evidence are present.

Pull requests and pushes to `main` also run the non-mutating release preview
audit workflow. It uploads `release-preview-audit.json`,
`release-preview-inspect.txt`, `release-preview-inspect.json`, and
`checksums.txt` as CI artifacts.
The CI production-readiness artifact also includes
`public-repo-policy-check.txt` and
`goal-run-retained-evidence-cleanup.json`, a schema-valid cleanup dry-run for
reviewing retained GoalRun evidence eligibility without deleting files.

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
weekly schedule for the promoted `v0.1.3` release. It checks release metadata,
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

The `Production Readiness Ops` workflow is a read-only scheduled and manual
drift check for live repository controls. It runs
`scripts/verify-branch-protection.sh` against `main` so branch-protection drift
is visible outside release-specific workflows.

## Continuous Integration

This repository is public. The GitHub Actions workflow runs automatically on every push to a branch and every pull request targeting `main`.

Before pushing or merging work, you can run the local verification checks:

```sh
go run github.com/rhysd/actionlint/cmd/actionlint@v1.7.12 -shellcheck= -pyflakes= .github/workflows/*.yml
go test ./...
go vet ./...
```

## License

AO Forge is licensed under `Apache-2.0`. See `LICENSE`.
