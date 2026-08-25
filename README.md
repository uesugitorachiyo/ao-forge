# AO Forge

[![Latest release](https://img.shields.io/github/v/release/uesugitorachiyo/ao-forge?label=latest%20release)](https://github.com/uesugitorachiyo/ao-forge/releases/tag/v0.1.5)

AO Forge coordinates one governed factory run. It maintains GoalRun state,
builds factory plans, requests Covenant decisions, delegates bounded execution
to AO2, and retains the evidence needed for operator review.

## Role In AO

- **Inputs:** Authorized objectives, repository context, readiness records,
  Covenant decisions, and AO2 evidence.
- **Outputs:** Factory plans, GoalRun state, release gates, evidence packets,
  and operator actions.
- **Upstream:** AO Foundry.
- **Downstream:** AO Covenant, AO2, AO Command, and release workflows.

See the [AO Architecture guide](https://github.com/uesugitorachiyo/ao-architecture)
and the
[AO Forge component page](https://github.com/uesugitorachiyo/ao-architecture/blob/main/components/ao-forge.md)
for the cross-repository flow.

## Quick Start

```sh
go test ./...
go build -o bin/forge ./cmd/forge
bin/forge inspect \
  --packet examples/packets/risky-pr-factory-packet.json
bin/forge doctor \
  --foundation docs/foundation/foundation-baseline.v0.1.json
bin/forge contract validate \
  --schema docs/contracts/release-preview-audit-v0.1.schema.json \
  --document examples/release-preview/dirty-workspace-blocked.audit.json
```

The [full CLI and operations reference](REFERENCE.md) documents contracts,
release previews, GoalRun workflows, retained evidence, checksums, rollback,
promotion, and every command example.

## Install v0.1.5

The current published release is [v0.1.5](https://github.com/uesugitorachiyo/ao-forge/releases/tag/v0.1.5),
built from `d1723769949269dcd0589916d83769dcb7275f98`.

Download [checksums.txt](https://github.com/uesugitorachiyo/ao-forge/releases/download/v0.1.5/checksums.txt)
with the archive for your platform:

- [macOS Apple Silicon](https://github.com/uesugitorachiyo/ao-forge/releases/download/v0.1.5/ao-forge_Darwin_arm64.tar.gz)
- [Linux x86_64](https://github.com/uesugitorachiyo/ao-forge/releases/download/v0.1.5/ao-forge_Linux_x86_64.tar.gz)
- [Windows x86_64](https://github.com/uesugitorachiyo/ao-forge/releases/download/v0.1.5/ao-forge_Windows_x86_64.zip)

Verify only the downloaded archive:

```sh
grep '  <archive-name>$' checksums.txt > checksums.selected
sha256sum -c checksums.selected # Linux
shasum -a 256 -c checksums.selected # macOS
```

After extraction, run `./forge --help` on macOS or Linux, or
`.\forge.exe --help` in PowerShell.

## Governance Boundary

Forge coordinates a run but does not bypass Covenant policy or AO2 execution
controls. Release and live-mutation paths require their documented approval,
rollback, verification, and evidence gates. A dry-run plan does not permit live
repository mutation.

## Documentation

- [Documentation Index](docs/README.md)
- [AO Forge Design](docs/design/AO-FORGE-V0.1.md)
- [GoalRuns](docs/design/GOAL-RUNS.md)
- [AO2 Pulse GoalRun Loop](docs/design/AO2-PULSE-GOAL-RUN-LOOP.md)
- [Verified Baseline](docs/foundation/VERIFIED-BASELINE.md)
- [Public Repository Policy](docs/security/PUBLIC-REPO-POLICY.md)
- [Release Threat Model](docs/security/RELEASE-THREAT-MODEL.md)
- [Production Stable Promotion](docs/release/PRODUCTION-STABLE-PROMOTION.md)
- [Full Reference](REFERENCE.md)

## Verification

```sh
go test ./...
go vet ./...
go run github.com/rhysd/actionlint/cmd/actionlint@v1.7.12 \
  -shellcheck= -pyflakes= .github/workflows/*.yml
```

## License

AO Forge is licensed under Apache 2.0. See [LICENSE](LICENSE).
