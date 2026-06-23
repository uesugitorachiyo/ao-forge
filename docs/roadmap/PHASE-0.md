# AO Forge Phase 0 Roadmap

Phase 0 creates the factory foundation before runtime code.

## Slice 0.1: Repository Foundation

- Create README and design spec.
- Define the first packet schema.
- Add a sample vertical-slice factory file.
- Decide initial implementation language and cross-platform posture.

Acceptance:

- design explains how AO Forge differs from AO2, AO Covenant,
  ao2-control-plane, AO Command, and reference-only AO Operator/AO Conductor/
  `agy-swarms` compatibility;
- first schema validates with a standard JSON parser;
- sample factory file is valid JSON;
- repo has no generated secrets, machine-local paths, or private artifacts.

## Slice 0.2: CLI Skeleton

Status: implemented locally in the Go CLI skeleton.

- Add `forge --help`.
- Add `forge plan --brief`.
- Add `forge inspect --packet`.
- Keep execution disabled.

Acceptance:

- builds on Ubuntu, macOS, and Windows;
- help text explains factory terms without marketing copy;
- sample brief produces a deterministic plan fixture.

## Slice 0.3: Covenant Gate Fixture

Status: implemented locally with fixture-backed gate results.

- Add a local fixture adapter for Covenant decisions.
- Require allow/deny explanations.
- Fail closed on missing or malformed decisions.

Acceptance:

- allow fixture passes;
- deny fixture blocks execution;
- malformed fixture exits non-zero and emits a packet.

## Slice 0.4: AO2 Execution Adapter

Status: ready to start against the verified foundation baseline.

Prerequisite:

- `GoalRun` contract exists so AO2 Pulse or Codex loop state is owned by AO
  Forge, while codex-cron remains only a scheduler.

- Invoke AO2 through a CLI adapter.
- Bind each workcell to an AO2 evidence directory.
- Record command, exit code, schema, and digest links.

Acceptance:

- fixture AO2 run produces a factory packet;
- missing AO2 binary fails closed;
- failed AO2 run still emits an inspectable packet.

Prerequisite:

- [Verified Foundation Baseline](../foundation/VERIFIED-BASELINE.md) exists and
  records AO2, ao2-control-plane, and AO Covenant commits verified for Forge
  integration.

## Slice 0.5: Control-Plane Readback

- Add optional ao2-control-plane publish/readback adapter.
- Preserve read-only observer semantics.
- Record readback summary in the factory packet.

Acceptance:

- readback fixture passes;
- unavailable control-plane with readback required fails closed;
- unavailable control-plane with readback optional records a warning.

## Slice 0.6: First End-to-End Factory Run

- Implement `forge once --brief brief.md --workspace <path>`.
- Run the first risky-PR-style factory slice.
- Emit JSON and Markdown packets.

Acceptance:

- one command produces a full packet;
- packet contains objective, plan, policy decision, AO2 evidence, and next
  action;
- tests pass on Ubuntu, macOS, and Windows.
