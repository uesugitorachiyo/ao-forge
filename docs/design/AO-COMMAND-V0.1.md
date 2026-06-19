# AO Command v0.1

AO Command is the operator command surface for AO Forge, AO2, ao2-control-plane,
and AO Covenant evidence. It is not another orchestration engine. Its first job
is to make the governed factory usable every day by answering what happened,
what is blocked, and what should happen next.

## Product Intent

AO Forge remains the factory brain and the owner of factory contracts, GoalRun
state, release gates, retained evidence policy, and production-readiness audit
semantics. AO Command consumes those contracts and presents a single
operator-ready view across goals, packets, readiness audits, release evidence,
and Covenant allow, deny, or block decisions.

The v0.1 product should optimize for repeated solo-operator use:

- inspect current production-readiness status without hunting across CI pages;
- inspect GoalRuns, current phase, next task, acceptance criteria, and retained
  evidence;
- inspect factory packets and Covenant decisions;
- read AO2 and ao2-control-plane evidence without mutating either system;
- trigger safe dry-runs and rehearsals;
- answer "what should happen next?" from the latest AO Forge evidence.

## Non-Goals

- Do not replace AO Forge planning, GoalRun state, release gates, or production
  readiness scoring.
- Do not replace AO2 execution, ao2-control-plane storage, or AO Covenant policy
  decisions.
- Do not perform dangerous writes by default.
- Do not display secrets, tokens, private credentials, or raw provider payloads.
- Do not build ao-arena benchmarks in this slice.
- Do not introduce a second policy engine by reimplementing AO Forge or Covenant
  decisions.

## Read-Only First Surface

The initial CLI surface should be read-only by default. Commands may invoke AO
Forge dry-run and rehearsal commands that already prove `mutates_releases=false`
and `network_required=false`, but publish, promote, rollback mutation, provider
execution, and release writes must require an explicit later command family.

Suggested v0.1 commands:

- `ao command status`: summarize production-readiness percentage, passing and
  failing gates, branch protection status, latest release preview evidence, and
  latest release rehearsal evidence.
- `ao command goals`: list and inspect GoalRuns, current phase, next task,
  stop conditions, retained evidence, evidence freshness, and readiness audits.
- `ao command packets`: inspect AO Forge operator packets, workcells, execution
  mode, AO2 evidence links, and packet status.
- `ao command decisions`: show Covenant allow, deny, and block decisions with
  reason, trust boundary, and next safe action.
- `ao command evidence`: read AO Forge, AO2, and ao2-control-plane evidence by
  schema, verify hashes where available, and highlight stale or missing
  artifacts.
- `ao command next`: derive the next recommended operator action from GoalRun,
  release-preview, release-verify, production-promotion, and
  production-readiness evidence.
- `ao command rehearse`: trigger safe dry-runs and rehearsals only, returning
  the produced audit paths and schema-validation status.

## Evidence Sources

AO Command v0.1 should read evidence from stable contracts instead of scraping
logs whenever possible:

- AO Forge JSON outputs from `forge production-readiness audit`,
  `forge release-preview inspect --json`, `forge goal inspect --json`,
  `forge goal readiness --json`, `forge goal evidence verify --json`,
  `forge goal evidence lint --json`, and `forge goal evidence cleanup --dry-run
  --json`;
- checked-in AO Forge contract, fixture, retained evidence, and release
  runbook paths;
- CI artifacts such as `production-readiness-audit.json`,
  `release-preview-audit.json`, `release-preview-inspect.json`,
  `goal-run-retained-evidence-cleanup.json`, and release rehearsal evidence;
- AO2 and ao2-control-plane evidence read-only exports;
- AO Covenant decision fixtures and gate results.

When a source is missing, stale, malformed, or schema-invalid, AO Command should
report that condition directly and recommend the smallest safe command to
refresh it.

## Safety Model

- Read-only by default.
- Dry-runs and rehearsals are allowed only when their AO Forge evidence proves
  they do not mutate releases or require network access.
- Any future write command must require a clean workspace, explicit operator
  confirmation, the relevant AO Forge audit contract, and a summarized
  before/after operator packet.
- Public-safe output must redact secrets, tokens, credentials, machine-local
  paths, and provider payloads unless an operator explicitly requests a local
  debug view.
- AO Command must not reimplement policy or scoring decisions. It should call
  AO Forge and AO Covenant owned commands, validate their JSON schemas, and
  present the result.

## V0.1 Operator Flows

### Morning Status

The operator runs `ao command status`. AO Command reads the latest production
readiness audit, branch protection evidence, release preview inspect output, and
retained evidence cleanup dry-run. It reports the current percentage, failed
gates if any, and the next recommended action.

### Goal Hardening Loop

The operator runs `ao command goals --goal ao2-weekend-hardening`. AO Command
loads the GoalRun, validates retained evidence paths, verifies evidence hashes,
shows the current phase and next task, then calls the AO Forge readiness command
before recommending AO2 Pulse continuation, backoff, or stop.

### Release Candidate Review

The operator runs `ao command rehearse --tag v0.1.3`. AO Command calls the AO
Forge release-preview dry-run path, validates the release preview audit and
inspect contracts, verifies checksums, and prints the next release actions
without creating or pushing a tag.

### Promotion Readiness Review

The operator runs `ao command next --promotion`. AO Command reads release
verify, install verify, rollback, retained public provenance, and production
promotion evidence. It explains which gate blocks promotion or confirms that the
next action is operator-approved production promotion.

## Contract With AO Forge

AO Forge owns the durable goal/task model and state machine. AO Command consumes
GoalRun, factory packet, release-preview, release-verify, production-promotion,
and production-readiness contracts. It must prove every recommendation by naming
the contract document, schema, status, and relevant failed or passed checks.

AO Command should shell to or link against AO Forge commands for authoritative
decisions. If AO Command cannot prove that the next action matches the latest
GoalRun, policy gate, release evidence, and readiness audit, it must recommend
backoff, evidence refresh, or operator review instead of inventing a new action.

## Acceptance Criteria

- The operator can answer "what is the current production-readiness percentage?"
  from one command.
- The operator can inspect the active GoalRun, current phase, next task,
  acceptance criteria, retained evidence, and readiness status.
- The operator can list latest CI, release-preview, release rehearsal, and
  production promotion evidence and validate their contracts.
- The operator can see Covenant allow, deny, and block decisions with their
  reasons and trust boundaries.
- Rehearsal commands are dry-run/read-only by default and return audit evidence
  paths.
- AO Command recommendations cite AO Forge or Covenant evidence and do not
  reimplement gate logic.

## Cut Line

AO Command v0.1 should be a small CLI or TUI-friendly command center, not a web
application and not ao-arena. The first useful release is successful when it
makes AO Forge daily operation easier, safer, and more repeatable without adding
new mutation paths.
