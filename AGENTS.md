# AO Forge Agent Instructions

## Status And Role

AO Forge is the active preparation and evidence coordinator for one governed factory run. It owns GoalRun state, factory packets and plans, Covenant decision requests, AO2 delegation material, release gates, retained evidence, and operator actions.

Forge does not create authorization, decide Covenant policy, bypass AO2 controls, or perform target mutation merely because a plan or readiness audit is green. Foundry selects portfolio work; Covenant decides policy; AO2 owns bounded execution and side effects.

## Sources Of Truth

- [docs/design/AO-FORGE-V0.1.md](docs/design/AO-FORGE-V0.1.md) and [docs/design/GOAL-RUNS.md](docs/design/GOAL-RUNS.md) define Forge ownership and GoalRun state.
- [docs/design/AO2-PULSE-GOAL-RUN-LOOP.md](docs/design/AO2-PULSE-GOAL-RUN-LOOP.md) defines the readiness and delegation boundary.
- `docs/contracts/`, `internal/cli/`, and their tests own schemas and implemented behavior. [REFERENCE.md](REFERENCE.md) is the current command reference.
- [docs/security/PUBLIC-REPO-POLICY.md](docs/security/PUBLIC-REPO-POLICY.md), [docs/security/RELEASE-THREAT-MODEL.md](docs/security/RELEASE-THREAT-MODEL.md), and [`.github/workflows/ci.yml`](.github/workflows/ci.yml) define public-safety and CI gates.

## Ownership And Boundaries

- Bind packets, plans, policy requests, approvals, GoalRun transitions, evidence, and release candidates to exact source heads and SHA-256 digests. Missing, stale, tampered, or mismatched provenance must fail closed.
- Require Covenant decisions before governed delegation and preserve AO2 approval and side-effect enforcement. A dry run, readiness audit, readback, or generated operator action grants no mutation authority.
- Treat `docs/evidence/`, `docs/release/`, `docs/foundation/`, and retained GoalRun records as historical or source-owned. Do not rewrite them to make a current gate pass.
- Keep generated binaries, plans, audits, previews, and run output in ignored `bin/`, `build/`, `dist/`, `target/`, or `tmp/`. Do not hand-edit generated evidence or result fields.
- Keep valid and invalid contract fixtures distinct and change them with consuming tests. Never weaken a negative case or inflate a readiness result.
- Do not record secrets, credentials, private keys, account identifiers, machine-local paths, or private logs. Release, deployment, publication, live mutation, credentialed operation, and direct-main changes require separate explicit authority and all executable gates.

## Working Method

- Change the smallest owned preparation surface and preserve GoalRun monotonicity, one-run scope, policy provenance, rollback, evidence retention, and producer/consumer ownership.
- Add negative tests for stale state, digest drift, invalid paths, missing decisions, failed readiness, and over-authority packets.
- Finalize a reviewed draft only when the signed release source, immutable plan, and an explicit descendant finalizer-workflow source are all bound; do not replace a draft or move a signed tag to repair a finalizer.
- Update this file in the same pull request when durable commands, architecture, ownership, or authority boundaries change.

## Verification

- Forge command and GoalRun changes: `go test ./internal/cli -count=1`.
- Foundation or issue-repair changes: run the affected package test, then `go test ./... -count=1`.
- Format relevant Go source with `gofmt -d cmd internal`; run `go vet ./...` and `go build -o bin/forge ./cmd/forge`.
- Run `scripts/check-public-repo-policy.sh`, `scripts/verify-goal-fixtures.sh`, and `go run ./cmd/forge production-readiness audit` for affected contracts or readiness behavior. Release preview, promotion, publication, and live execution commands require separate authority.
- For instruction changes run `python3 ../ao-architecture/scripts/verify_agent_instruction_layout.py --workspace-root .. --repository ao-forge`. Always run `git diff --check`.

## Evidence And Completion

- Record source heads, commands and exits, policy and approval bindings, relevant artifact digests, and retained evidence paths. Report skipped, unavailable, or failed checks explicitly.
- Completion requires focused and broad gates, green pull-request CI, clean synchronized `main`, and task-branch cleanup. Readiness or a release candidate is evidence, not publication authority.
