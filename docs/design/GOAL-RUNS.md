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

The schema is `docs/contracts/goal-run-v0.1.schema.json`. The example is
`examples/goals/ao2-weekend-hardening.goal-run.json`.

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
