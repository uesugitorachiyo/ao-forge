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
- [First Public Release Checklist](docs/release/FIRST-PUBLIC-RELEASE.md)
- [Verified Foundation Baseline](docs/foundation/VERIFIED-BASELINE.md)
- [Foundation Baseline JSON](docs/foundation/foundation-baseline.v0.1.json)
- [Release Threat Model](docs/security/RELEASE-THREAT-MODEL.md)
- [Factory Brief v0.1 Schema](docs/contracts/factory-brief-v0.1.schema.json)
- [Factory Plan v0.1 Schema](docs/contracts/factory-plan-v0.1.schema.json)
- [Factory Packet v0.1 Schema](docs/contracts/factory-packet-v0.1.schema.json)
- [Covenant Decision Fixture v0.1 Schema](docs/contracts/covenant-decision-fixture-v0.1.schema.json)
- [Covenant Gate Result v0.1 Schema](docs/contracts/covenant-gate-result-v0.1.schema.json)
- [Release Preview Audit v0.1 Schema](docs/contracts/release-preview-audit-v0.1.schema.json)
- [Release Preview Inspect v0.1 Schema](docs/contracts/release-preview-inspect-v0.1.schema.json)
- [Release Artifact Inventory v0.1 Schema](docs/contracts/release-artifact-inventory-v0.1.schema.json)
- [Release Attestation Plan v0.1 Schema](docs/contracts/release-attestation-plan-v0.1.schema.json)
- [Release Preview Fixtures](examples/release-preview/)
- [Example Vertical Slice](examples/vertical-slices/risky-pr-factory.factory.json)
- [Example Deterministic Plan](examples/plans/risky-pr-factory-plan.json)
- [Example Covenant Gate Result](examples/gates/allow-local-plan.gate.json)
- [Example Factory Packet](examples/packets/risky-pr-factory-packet.json)

## Local CLI

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

## Continuous Integration

This repository is public. The GitHub Actions workflow runs automatically on every push to a branch and every pull request targeting `main`.

Before pushing or merging work, you can run the local verification checks:

```sh
go test ./...
go vet ./...
```
