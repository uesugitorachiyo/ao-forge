# AO Forge Verified Foundation Baseline

Date: 2026-06-17

AO Forge may start Phase 0 work against this baseline. The baseline records
the exact component commits and release references that were verified before
Forge integration work continued.

Machine-readable baseline:

- [foundation-baseline.v0.1.json](foundation-baseline.v0.1.json)

## Components

| Component | Commit | Release | Verification |
| --- | --- | --- | --- |
| AO2 | `fbdea7d4c8d0546e52103b7d3c0cdf01d2013670` | `v0.4.80` | CI `93/93`, Pulse runtime local test |
| ao2-control-plane | `de4e865ef8a3fe00005d27b165aab319e99c6ba1` | `v0.1.13` | CI `31/31`, local workspace tests |
| AO Covenant | `ef815b35d1166b1f26ded2b482f15d088281c568` | `v0.1.0` | CI, Release Readiness, local Go tests |

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

## Next Forge Slice

The next slice should add a foundation doctor command or adapter preflight that
reads `foundation-baseline.v0.1.json`, checks the sibling repositories, and
reports whether the local workspace matches the verified baseline.
