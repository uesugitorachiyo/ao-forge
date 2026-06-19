# AO2 Pulse GoalRun Loop

AO2 Pulse may execute a repeated hardening loop only when AO Forge owns the
durable `GoalRun` state and codex-cron only triggers the loop. AO2 Pulse must
not treat a scheduler tick as permission to mutate a repository.

## Boundary

- AO Forge owns the GoalRun schema, phase transitions, guarded updates, and
  update audit contract.
- AO2 Pulse may inspect a GoalRun, perform bounded work that matches the
  GoalRun, and propose the next GoalRun state through `forge goal update`.
- codex-cron may invoke AO2 Pulse on a schedule, but must not store goal
  semantics, stop conditions, continuation prompts, or phase state.
- If AO2 Pulse cannot prove that the next action matches the GoalRun, it must
  emit backoff or stop instead of continuing.

## Required Loop

Run these commands from the repository that owns the GoalRun contract.

1. Validate the latest GoalRun before doing work:

   ```sh
   forge goal validate --goal-run examples/goals/ao2-weekend-hardening.goal-run.json
   ```

2. Inspect the latest state and use the JSON output as the loop input:

   ```sh
   forge goal inspect --goal-run examples/goals/ao2-weekend-hardening.goal-run.json --json
   ```

3. Prove the next action still matches the objective, allowed scope,
   acceptance criteria, and stop conditions. If it does not match, stop before
   mutating anything.

4. Check the intended phase transition before proposing state changes:

   ```sh
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

9. Decide whether evidence may be reused. AO2 Pulse may reuse a previously
   recorded evidence artifact only when it is intentionally immutable, still
   verifies by SHA-256, and still proves the proposed next task. If repository
   state changed or the artifact is a live-state snapshot, AO2 Pulse must
   collect fresh evidence instead.

## Handoff Fixture

The checked-in handoff pair shows the durable artifact AO2 Pulse should
preserve after a successful iteration:

- `examples/goals/ao2-pulse-handoff.goal-run.json` is the candidate GoalRun
  after moving from `planning` to `implementation`.
- `examples/goals/ao2-pulse-handoff.goal-run-update-audit.json` is the matching
  update audit, including the candidate path, changed fields, and hashed
  evidence attachment.

CI runs `scripts/verify-goal-fixtures.sh` to validate every checked-in GoalRun
and GoalRun update-audit fixture, and to verify every recorded GoalRun evidence
hash, including this handoff pair.

## Stop And Backoff

AO2 Pulse must not continue when any of these checks fail:

- the GoalRun does not validate;
- the latest GoalRun cannot be inspected;
- the next action does not match the objective, allowed scope, acceptance
  criteria, and stop conditions;
- `forge goal transitions` denies the proposed phase change;
- `forge goal update` fails;
- the update audit does not validate;
- the candidate GoalRun does not validate;
- any recorded evidence attachment is missing or has a SHA-256 mismatch.
- evidence is duplicated, stale, or no longer proves the next task.

For a denied transition or scope mismatch, AO2 Pulse should emit a backoff or
stopped result and leave the existing GoalRun unchanged. For terminal phases
`complete` and `stopped`, AO2 Pulse must not resume the loop without a new
operator-approved GoalRun.

## Evidence

Each successful loop iteration must preserve:

- the GoalRun path and `goal_id`;
- the inspected `current_phase` and proposed next phase;
- the `forge goal transitions` result;
- the `forge goal update` audit JSON;
- every hashed evidence attachment listed by the update audit;
- the `forge contract validate` result for that audit;
- the final `forge goal validate` result for the candidate GoalRun;
- the final `forge goal evidence verify` result for the candidate GoalRun.

This keeps AO2 Pulse useful for hardening work while keeping durable goal state,
transition policy, and mutation auditability inside AO Forge.
