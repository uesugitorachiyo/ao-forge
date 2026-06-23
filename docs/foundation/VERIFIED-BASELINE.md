# AO Forge Verified Foundation Baseline

Date: 2026-06-23

AO Forge may run active-spine factory work against this baseline. The baseline
records the exact component commits, release references, and current green
evidence that were verified before Forge readiness work continued.

Machine-readable baseline:

- [foundation-baseline.v0.1.json](foundation-baseline.v0.1.json)

## Components

| Component | Commit | Release | Verification |
| --- | --- | --- | --- |
| AO2 | `b416e932feb73b2794ada01d4facafebcefa311a` | `v0.4.80` | CI `93/93`, Production Readiness Ops `1/1` |
| ao2-control-plane | `15a008a20f83e963b253601e8d9a729e93627f5a` | `v0.1.13` | CI `33/33`, Production Readiness Ops `1/1` |
| AO Covenant | `443e8af0d390e8c8e3036432ce887b313546d724` | `v0.1.0` | CI `4/4`, Release Readiness `3/3`, Production Readiness Ops `1/1` |

## Integration Rules

- Forge consumes AO2, ao2-control-plane, and AO Covenant as external systems.
- Forge pins commits until the next release tags include the verified changes.
- Forge must not copy implementation code from the component repositories.
- Forge must fail closed when the Covenant decision is missing, malformed, or
  denying.
- Forge must treat ao2-control-plane as read-only evidence readback, never as
  approval.
- Forge packets must never contain provider credentials, API tokens, private
  keys, or local-only machine secrets.

## Local Workspace

Expected sibling paths:

```text
../ao2
../ao2-control-plane
../ao-covenant
```

The archived stub folder, if present, is intentionally not used:

```text
../ao-covenant-stub-20260617
```

## Current Forge Slice

The active slice keeps the foundation doctor command in the local release gate.
It reads `foundation-baseline.v0.1.json`, checks the sibling repositories, and
reports whether the local workspace matches the verified active-spine baseline.
