# AO2 Pulse GoalRun Loop

AO2 Pulse may execute a repeated hardening loop only when AO Forge owns the
durable `GoalRun` state and codex-cron only triggers the loop. AO2 Pulse must
not treat a scheduler tick as permission to mutate a repository.

## Boundary

- AO Forge owns the GoalRun schema, phase transitions, guarded updates, and
  readiness and update audit contracts.
- AO2 Pulse may inspect a GoalRun, perform bounded work that matches the
  GoalRun, and propose the next GoalRun state through `forge goal update`.
- codex-cron may invoke AO2 Pulse on a schedule, but must not store goal
  semantics, stop conditions, continuation prompts, or phase state.
- If AO2 Pulse cannot prove that the next action matches the GoalRun, it must
  emit backoff or stop instead of continuing.

## Required Loop

Run these commands from the repository that owns the GoalRun contract.

1. Run the AO Forge readiness entrypoint before doing work:

   ```sh
   scripts/ao2-pulse-goal-readiness.sh \
     --goal-run examples/goals/ao2-retained-evidence.goal-run.json \
     --to verification \
     --out tmp/goal-run-readiness-audit.json
   ```

   The entrypoint runs `forge goal readiness --json`, validates the emitted JSON
   against `docs/contracts/goal-run-readiness-audit-v0.1.schema.json`, writes it
   to `--out`, and exits non-zero when readiness fails. AO2 Pulse and codex-cron
   must treat a non-zero exit as backoff or stop, not as permission to mutate.

2. Use the readiness audit JSON as the loop input:

   ```sh
   forge goal readiness \
     --goal-run examples/goals/ao2-retained-evidence.goal-run.json \
     --to verification \
     --json > tmp/goal-run-readiness-audit.json
   ```

   The readiness audit includes GoalRun validation, inspection, phase transition
   status, evidence path linting, evidence hash verification, and retained
   evidence retention audits. AO2 Pulse may read the embedded check summaries,
   but it must not reimplement these policies outside AO Forge.

3. Prove the next action still matches the objective, allowed scope,
   acceptance criteria, and stop conditions. If it does not match the readiness
   audit and latest GoalRun, stop before mutating anything.

4. If the loop needs lower-level diagnostics, inspect the latest state or phase
   transition manually; do not replace the readiness gate with these commands:

   ```sh
   forge goal inspect --goal-run examples/goals/ao2-weekend-hardening.goal-run.json --json
   forge goal transitions \
     --goal-run examples/goals/ao2-weekend-hardening.goal-run.json \
     --to implementation
   ```

5. Write the next GoalRun candidate through AO Forge, not by editing JSON
   directly:

   ```sh
   forge goal update \
     --goal-run examples/goals/ao2-weekend-hardening.goal-run.json \
     --out tmp/ao2-weekend-hardening.goal-run.json \
     --phase implementation \
     --next-task "Implement the smallest verified AO2 hardening task." \
     --last-verified-at 2026-06-19T14:30:00Z \
     --evidence examples/goals/ao2-weekend-hardening.goal-run.json \
     --json > tmp/ao2-weekend-hardening.goal-run-update-audit.json
   ```

   Each `--evidence` path must point to a readable artifact. AO Forge records
   the path and SHA-256 in the candidate GoalRun and in the update audit, giving
   AO2 Pulse a durable handoff for the proof used to choose the next state.
   Before preserving a candidate as durable state, move any scratch evidence out
   of `tmp/` and into the retained evidence layout:

   ```text
   docs/evidence/goals/<goal_id>/<YYYYMMDDTHHMMSSZ>-<phase>/
   ```

6. Validate the emitted update audit before preserving it:

   ```sh
   forge contract validate \
     --schema docs/contracts/goal-run-update-audit-v0.1.schema.json \
     --document tmp/ao2-weekend-hardening.goal-run-update-audit.json
   ```

7. Validate the candidate GoalRun before it becomes the latest state:

   ```sh
   forge goal validate --goal-run tmp/ao2-weekend-hardening.goal-run.json
   ```

8. Verify every recorded evidence attachment before preserving the candidate:

   ```sh
   forge goal evidence verify --goal-run tmp/ao2-weekend-hardening.goal-run.json
   ```

9. Lint retained evidence paths before preserving the candidate:

   ```sh
   forge goal evidence lint --goal-run tmp/ao2-weekend-hardening.goal-run.json
   forge goal evidence lint --update-audit tmp/ao2-weekend-hardening.goal-run-update-audit.json
   ```

10. Decide whether evidence may be reused. AO2 Pulse may reuse a previously
   recorded evidence artifact only when it is intentionally immutable, still
   verifies by SHA-256, and still proves the proposed next task. If repository
   state changed or the artifact is a live-state snapshot, AO2 Pulse must
   collect fresh evidence instead.

11. Retain the handoff evidence. The durable handoff must include the readiness
    audit JSON, candidate GoalRun, update audit, evidence verification JSON, and
    every artifact named in `last_iteration.evidence`. Persist these under
    `docs/evidence/goals/<goal_id>/<YYYYMMDDTHHMMSSZ>-<phase>/` or stop before
    replacing the latest GoalRun.

## Handoff Fixture

The checked-in handoff pair shows the durable artifact AO2 Pulse should
preserve after a successful iteration:

- `examples/goals/ao2-pulse-handoff.goal-run.json` is the candidate GoalRun
  after moving from `planning` to `implementation`.
- `examples/goals/ao2-pulse-handoff.goal-run-update-audit.json` is the matching
  update audit, including the candidate path, changed fields, and hashed
  evidence attachment.
- `examples/goals/ao2-retained-evidence.goal-run.json` shows a GoalRun whose
  retained evidence path is durable and repository-relative under
  `docs/evidence/goals/`.
- `docs/evidence/goals/ao2-weekend-hardening/20260619T180000Z-verification/goal-run-readiness-audit.json`
  is the retained readiness audit AO2 Pulse should preserve before continuing
  from `implementation` to `verification`.

CI runs `scripts/verify-goal-fixtures.sh` to validate every checked-in GoalRun
and GoalRun update-audit fixture, and to verify every recorded GoalRun evidence
hash, including this handoff pair. The same smoke test also verifies that
`scripts/ao2-pulse-goal-readiness.sh` produces schema-valid readiness audit JSON
for positive fixtures and fails closed while preserving failed readiness JSON for
negative fixtures. It validates retained readiness audit JSON under
`docs/evidence/goals/` against
`docs/contracts/goal-run-readiness-audit-v0.1.schema.json`. The same smoke test
inspects the failed readiness audit for
`examples/goals/invalid/stale-evidence.goal-run.invalid.json`, confirms the
failed `evidence_verify` result and errors were preserved, and proves
`forge goal update` cannot write an advanced candidate after readiness fails.
The same smoke test
rejects
`examples/goals/invalid/tampered-readiness-audit.goal-run-readiness-audit.invalid.json`
to prove tampered readiness evidence cannot be accepted before loop
continuation. The same smoke test verifies that
`examples/goals/invalid/stale-evidence.goal-run.invalid.json` fails closed when
its recorded evidence hash does not match the artifact bytes, and that at least
one positive GoalRun fixture uses the retained evidence layout. It also runs
`forge goal evidence lint` and rejects the path-policy fixtures under
`examples/goals/invalid/` that record retained evidence in `tmp/`, `/tmp/`, or a
home-directory path.

## Stop And Backoff

AO2 Pulse must not continue when any of these checks fail:

- the GoalRun does not validate;
- the latest GoalRun cannot be inspected;
- the next action does not match the objective, allowed scope, acceptance
  criteria, and stop conditions;
- `scripts/ao2-pulse-goal-readiness.sh` exits non-zero;
- the readiness audit JSON does not validate against
  `docs/contracts/goal-run-readiness-audit-v0.1.schema.json`;
- `forge goal update` fails;
- `forge goal update` rejects the source GoalRun because existing evidence no
  longer lints or verifies;
- the update audit does not validate;
- the candidate GoalRun does not validate;
- any recorded evidence attachment is missing or has a SHA-256 mismatch.
- `forge goal evidence lint` rejects a retained evidence path.
- evidence is duplicated, stale, or no longer proves the next task.

For a denied transition or scope mismatch, AO2 Pulse should emit a backoff or
stopped result and leave the existing GoalRun unchanged. For terminal phases
`complete` and `stopped`, AO2 Pulse must not resume the loop without a new
operator-approved GoalRun.

## Evidence

Each successful loop iteration must preserve:

- the GoalRun path and `goal_id`;
- the readiness audit JSON from `scripts/ao2-pulse-goal-readiness.sh`;
- the inspected `current_phase` and proposed next phase from that readiness
  audit;
- the readiness `goal_transition` result;
- the `forge goal update` audit JSON;
- every hashed evidence attachment listed by the update audit;
- the `forge contract validate` result for that audit;
- the final `forge goal validate` result for the candidate GoalRun;
- the final `forge goal evidence lint` result for the candidate GoalRun and
  update audit;
- the final `forge goal evidence verify` result for the candidate GoalRun.

Preserved evidence paths must be repository-relative and durable. Do not record
`tmp/`, `/tmp/`, runner temp directories, home-directory paths, or other
machine-local absolute paths in `last_iteration.evidence`. Keep retained
evidence until the GoalRun reaches a terminal phase and for at least 90 days
afterward; public release or promotion evidence is retained indefinitely.

This keeps AO2 Pulse useful for hardening work while keeping durable goal state,
transition policy, and mutation auditability inside AO Forge.
