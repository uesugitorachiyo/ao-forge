# AO Forge v0.1 Architecture

## Goal

AO Forge is a clean AI agentic factory that coordinates production work across
AO2, AO Covenant, and ao2-control-plane. Its job is not to be another executor.
Its job is to plan, schedule, gate, observe, and close factory work with
machine-readable evidence.

## Why This Exists

The existing AO family has strong components:

- AO2 is now a governed execution and evidence engine.
- ao2-control-plane is a read-only evidence observer.
- AO Covenant is the policy, provenance, release, and trust layer.
- Deprecated AO Operator and AO Conductor proved useful orchestration ideas but
  are now reference paths, not active execution owners.
- `agy-swarms` is archived/reference-only for AO Foundry work; legacy Forge
  compatibility remains guarded behind explicit operator opt-in.

The missing layer is a durable factory brain: a system that can take an
operator objective, decompose it into workcells, decide what can run now, route
each unit through policy and evidence gates, and produce a factory-level
operator packet that explains what happened and what should happen next.

## Non-Goals

AO Forge v0.1 will not:

- run model providers directly;
- replace AO2 policy checks or evidence generation;
- mutate GitHub releases;
- approve work from the control plane;
- become a generic chat agent;
- hide failed, rejected, or missing evidence behind a green task status.

## Recommended Implementation Direction

Use Go 1.26.x for the first AO Forge CLI and local coordinator.

Why Go:

- one static cross-platform binary for Ubuntu, macOS, and Windows;
- excellent process supervision for invoking AO2 and Covenant CLIs;
- simple concurrency primitives for factory workcells;
- fast startup and low operational friction;
- easier distribution than a Python multi-package runtime;
- less tight coupling to AO2 internals than implementing Forge inside AO2's
  Rust workspace.

AO Forge should treat AO2, AO Covenant, and ao2-control-plane as external
systems connected by signed files, JSON summaries, and command adapters. That
keeps the factory clean and lets each project keep its own release cadence.

## Architecture

AO Forge v0.1 has seven units.

### 1. Factory Brief

Input: an operator objective plus constraints.

Output: a normalized `FactoryBrief` with:

- objective;
- target repository or workspace;
- allowed factory modes;
- release or non-release posture;
- expected evidence classes;
- maximum blast radius;
- stop conditions.

### 2. Factory Planner

Input: `FactoryBrief`.

Output: a `FactoryPlan` containing workcells and dependencies.

A workcell is a bounded unit of factory work. It should be small enough to run,
gate, inspect, retry, or reject independently. Workcells are not vague tasks;
they are typed factory units with inputs, commands, expected outputs, and
evidence requirements.

### 3. Covenant Gate

Input: `FactoryPlan` or individual workcell.

Output: allow, deny, or requires-operator-approval.

The Covenant gate must explain every decision in human-readable and
machine-readable form. It governs:

- repository mutation boundaries;
- network and release mutation boundaries;
- credential handling;
- artifact provenance;
- approval requirements;
- replacement and rollback policy.

### 4. Workcell Scheduler

Input: gated workcells.

Output: ordered workcell runs.

The scheduler should run only what is allowed, independent, and bounded. It can
parallelize later, but v0.1 should prefer deterministic serial execution so the
first evidence chain is simple.

### 5. AO2 Execution Adapter

Input: a gated workcell.

Output: an AO2 run reference and local evidence path.

Forge should call AO2 through a stable CLI adapter first. Direct library
integration can come later if the CLI contract becomes a bottleneck.

### 6. Evidence Router

Input: AO2 run evidence and Covenant decisions.

Output: local packet plus optional ao2-control-plane publication/readback.

The router never treats upload as approval. The control plane remains a
read-only observer. A successful readback means evidence was stored and can be
inspected; it does not mean the work is accepted.

### 7. Operator Packet

Input: plan, policy decisions, workcell runs, evidence links, and readback.

Output: `ao.forge.factory-packet.v0.1`.

The packet is the factory-level truth. It must answer:

- what objective entered the factory;
- what workcells were planned;
- which gates allowed or denied work;
- what AO2 evidence was produced;
- what was published or read back;
- what failed or was skipped;
- what the next recommended action is.

## Lifecycle

```text
forge init
  -> creates local factory state

forge plan --brief brief.md
  -> writes factory-plan.json and policy-request.json

forge gate --plan factory-plan.json
  -> writes covenant-decision.json

forge run --plan factory-plan.json --decision covenant-decision.json
  -> invokes AO2 for allowed workcells

forge packet --run <run-id>
  -> writes factory-packet.json and packet.md

forge inspect --packet factory-packet.json
  -> prints human summary and next actions
```

The first implementation should also support a single command:

```text
forge once --brief brief.md --workspace /path/to/repo
```

`forge once` runs the full v0.1 loop in dry-run-friendly mode and writes one
packet directory.

## Contract Boundaries

AO Forge owns:

- durable goal and task state through `GoalRun`;
- factory brief normalization;
- factory plan and workcell schema;
- scheduler decisions;
- cross-project packet assembly;
- next-action recommendation.

AO2 owns:

- governed execution;
- command evidence;
- approval and evaluator closure;
- replayable run artifacts.

AO2 Pulse or Codex may update a `GoalRun` after each iteration, but must read
the latest `GoalRun` before the next iteration and prove the next action still
matches the objective, allowed scope, acceptance criteria, and stop conditions.
An external scheduler may trigger a loop, but it must not own goal semantics or
stop rules.

AO Covenant owns:

- policy decisions;
- release and replacement trust controls;
- provenance and signature posture.

ao2-control-plane owns:

- evidence ingestion;
- storage verification;
- read-only operator views.

## First Vertical Slice

The first v0.1 vertical slice should support one local repository and one
bounded software-delivery objective:

```text
Objective:
  "Run a governed risky PR improvement against this fixture repository."

Forge behavior:
  1. normalize the objective into a factory brief;
  2. produce a three-workcell plan: prepare, execute, close;
  3. ask Covenant for a policy decision;
  4. invoke AO2 for the allowed execution workcell;
  5. collect AO2 evidence;
  6. optionally publish/read back through ao2-control-plane;
  7. emit a factory packet with next actions.
```

Success means the packet can be inspected without reading terminal scrollback.
Failure also produces a packet.

## Safety Invariants

AO Forge v0.1 must fail closed when:

- the Covenant decision is missing, malformed, or denying;
- AO2 evidence is missing or schema-invalid;
- a workcell tries to exceed its declared mutation boundary;
- provider credentials are discovered in packet output;
- control-plane readback is required but unavailable;
- release mutation is requested without an explicit release-mode gate.

## Public Readiness Bar

AO Forge can be public-preview ready when:

- `forge once` runs on Ubuntu, macOS, and Windows;
- factory packet schema is tested with fixtures;
- every deny/allow decision has an explanation;
- AO2 evidence links are digest-bound;
- no provider credentials appear in artifacts;
- the control plane remains an observer;
- a fresh operator can reproduce the first vertical slice from the README.
