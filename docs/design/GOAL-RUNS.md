# AO Forge GoalRun Contract

AO Forge owns durable goal and task state for autonomous or repeated hardening
loops. AO2 Pulse, Codex, or another executor may update a `GoalRun` after an
iteration, but codex-cron should only trigger the loop. codex-cron must not own
goal semantics, stop rules, or continuation prompts.

## Contract

`GoalRun` documents the durable goal state that an agent must read before doing
work:

- `goal_id`: stable identifier for the goal loop.
- `repo`: target repository.
- `objective`: operator-approved goal.
- `acceptance_criteria`: evidence-backed completion criteria.
- `allowed_scope`: explicit mutation and inspection boundaries.
- `stop_conditions`: conditions that require backoff or stop.
- `current_phase`: current state of the loop.
- `next_task`: next bounded task.
- `last_verified_at`: last time the goal state was checked.
- `continuation_prompt`: prompt text for the next loop iteration.
- `loop_owner`: ownership split between AO Forge, executor, and scheduler.
- `next_action_guard`: required preflight proof for the next action.

The GoalRun schema is `docs/contracts/goal-run-v0.1.schema.json`. The update
audit schema is `docs/contracts/goal-run-update-audit-v0.1.schema.json`. The
evidence verification schema is
`docs/contracts/goal-run-evidence-verify-v0.1.schema.json`. The evidence path
lint schema is `docs/contracts/goal-run-evidence-lint-v0.1.schema.json`. The
examples are `examples/goals/ao2-weekend-hardening.goal-run.json` and
`examples/goals/ao2-weekend-hardening.goal-run-update-audit.json`.
AO2 Pulse integration rules live in
`docs/design/AO2-PULSE-GOAL-RUN-LOOP.md`.

## Loop Rule

Before starting a loop iteration, the agent must read the latest `GoalRun` and
state why the next action matches:

- the objective;
- the allowed scope;
- the acceptance criteria;
- the stop conditions.

If the next action does not match, the agent must emit backoff or stop instead
of continuing. This keeps AO2 Pulse and Codex from converting a scheduler tick
into unbounded repository mutation.

## Phase Transitions

`current_phase` is not free text. AO Forge owns the phase transition policy and
agents must check it before proposing or applying a phase change:

| Current phase | Allowed next phases |
| --- | --- |
| `planning` | `implementation`, `blocked`, `backoff`, `stopped` |
| `implementation` | `verification`, `blocked`, `backoff`, `stopped` |
| `verification` | `implementation`, `complete`, `blocked`, `backoff`, `stopped` |
| `blocked` | `planning`, `stopped` |
| `backoff` | `planning`, `stopped` |
| `stopped` | none; terminal |
| `complete` | none; terminal |

Use the non-mutating transition gate before any loop writes a new phase:

```sh
forge goal transitions --goal-run examples/goals/ao2-weekend-hardening.goal-run.json
forge goal transitions --goal-run examples/goals/ao2-weekend-hardening.goal-run.json --to implementation
```

A denied `--to` check returns non-zero. `stopped` and `complete` are terminal
states, so an automated loop must not resume from them without a new operator
approved `GoalRun`.

## Guarded Updates

`forge goal update` creates a validated candidate GoalRun and prints an update
audit. It requires `--out` and refuses to write over the input file. This keeps
agents from mutating durable goal state without a reviewable artifact.

```sh
forge goal update \
  --goal-run examples/goals/ao2-weekend-hardening.goal-run.json \
  --out tmp/ao2-weekend-hardening.goal-run.json \
  --phase implementation \
  --next-task "Implement the smallest verified AO2 hardening task." \
  --evidence examples/goals/ao2-weekend-hardening.goal-run.json
```

The command validates the source GoalRun, checks any requested phase change
against the transition policy, validates the updated GoalRun against the schema,
writes the candidate to `--out`, and emits an `ao.forge.goal-run-update-audit.v0.1`
summary. A denied transition exits non-zero and writes no output file.
Each `--evidence` path must be readable. AO Forge records the evidence path and
SHA-256 in both `last_iteration.evidence` and the emitted update audit, so AO2
Pulse can preserve the artifacts that prove why the next state was selected.

Validate an emitted or stored update audit before preserving it as loop
evidence:

```sh
forge contract validate \
  --schema docs/contracts/goal-run-update-audit-v0.1.schema.json \
  --document examples/goals/ao2-weekend-hardening.goal-run-update-audit.json
```

Verify the bytes behind recorded GoalRun evidence before using a candidate as
the latest durable state:

```sh
forge goal evidence verify --goal-run examples/goals/ao2-pulse-handoff.goal-run.json
forge goal evidence verify --goal-run examples/goals/ao2-pulse-handoff.goal-run.json --json > tmp/goal-run-evidence-verify.json
forge contract validate \
  --schema docs/contracts/goal-run-evidence-verify-v0.1.schema.json \
  --document tmp/goal-run-evidence-verify.json
```

The verifier recalculates SHA-256 for every `last_iteration.evidence` path and
fails if an artifact is missing, has no recorded hash, or no longer matches the
recorded hash. JSON output uses
`ao.forge.goal-run-evidence-verify.v0.1` and must validate against the evidence
verification schema before AO2 Pulse treats it as durable loop evidence.

Lint retained evidence paths before preserving a GoalRun or update audit:

```sh
forge goal evidence lint --goal-run examples/goals/ao2-retained-evidence.goal-run.json
forge goal evidence lint --update-audit examples/goals/ao2-pulse-handoff.goal-run-update-audit.json
forge goal evidence lint --goal-run examples/goals/ao2-retained-evidence.goal-run.json --json > tmp/goal-run-evidence-lint.json
forge contract validate \
  --schema docs/contracts/goal-run-evidence-lint-v0.1.schema.json \
  --document tmp/goal-run-evidence-lint.json
```

The linter validates the source document schema, then rejects persisted evidence
paths under `tmp/`, `/tmp/`, home directories, parent traversal, or
machine-local absolute paths. JSON output uses
`ao.forge.goal-run-evidence-lint.v0.1` and must validate against the evidence
lint schema before AO2 Pulse treats retained evidence paths as durable.

The negative fixture
`examples/goals/invalid/stale-evidence.goal-run.invalid.json` is schema-valid
but intentionally records a stale evidence hash; CI must reject it through
`scripts/verify-goal-fixtures.sh`.

## Evidence Freshness Policy

AO Forge treats GoalRun evidence as immutable proof for the iteration that
selected the next state.

- A single `forge goal update` call must not attach the same evidence path more
  than once. Duplicate paths are denied before the candidate GoalRun is written.
- Reusing evidence from an earlier iteration is allowed only when the artifact is
  intentionally immutable, still matches its recorded SHA-256, and still proves
  the proposed next task. Otherwise AO2 Pulse must collect fresh evidence.
- If `forge goal evidence verify` reports a missing artifact, missing hash, or
  SHA-256 mismatch, AO2 Pulse must emit backoff or stop and leave the existing
  GoalRun unchanged.
- Evidence that depends on live repository state should be refreshed after the
  worktree, branch, or checked commit changes.

## Evidence Retention Policy

Persisted `last_iteration.evidence` paths must point to durable evidence, not
scratch files. `tmp/`, `/tmp/`, runner temp directories, user home directories,
and machine-local absolute paths are allowed while constructing a candidate, but
must not be preserved in the durable GoalRun or update audit.

Use this repository-relative layout for retained loop evidence:

```text
docs/evidence/goals/<goal_id>/<YYYYMMDDTHHMMSSZ>-<phase>/
```

The retained directory should include the candidate GoalRun, the update audit,
the evidence verification JSON, and any source artifacts referenced by
`last_iteration.evidence`. File names should be stable, descriptive, and safe to
compare in code review; avoid timestamps inside individual file names unless
they are part of the iteration directory.

`examples/goals/ao2-retained-evidence.goal-run.json` is the checked-in fixture
for this policy. Its retained artifact lives under `docs/evidence/goals/`, and
`scripts/verify-goal-fixtures.sh` fails if no positive GoalRun fixture uses that
durable layout.

The same fixture smoke runs `forge goal evidence lint` against every checked-in
GoalRun and GoalRun update-audit evidence path. The checked-in negative fixtures
are:

- `examples/goals/invalid/tmp-evidence-path.goal-run.path-invalid.json`
- `examples/goals/invalid/absolute-evidence-path.goal-run.path-invalid.json`
- `examples/goals/invalid/home-evidence-path.goal-run.path-invalid.json`
- `examples/goals/invalid/parent-traversal-evidence-path.goal-run.path-invalid.json`
- `examples/goals/invalid/windows-absolute-evidence-path.goal-run.path-invalid.json`
- `examples/goals/invalid/tmp-evidence-path.goal-run-update-audit.path-invalid.json`
- `examples/goals/invalid/absolute-evidence-path.goal-run-update-audit.path-invalid.json`
- `examples/goals/invalid/home-evidence-path.goal-run-update-audit.path-invalid.json`
- `examples/goals/invalid/parent-traversal-evidence-path.goal-run-update-audit.path-invalid.json`
- `examples/goals/invalid/windows-absolute-evidence-path.goal-run-update-audit.path-invalid.json`

Retention rules:

- Keep evidence for any non-terminal GoalRun while the loop may continue.
- After `complete` or `stopped`, keep loop evidence for at least 90 days.
- Keep release, promotion, and public provenance evidence indefinitely unless a
  later audited retention decision replaces that policy.
- Do not delete an evidence artifact while any retained GoalRun or update audit
  still references its path.
- Cleanup must be reviewable: remove evidence in a separate change that names
  the GoalRun, iteration directory, and reason for removal.
