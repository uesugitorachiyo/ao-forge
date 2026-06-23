# AO Forge Branch Protection Evidence

This document records the latest read-only verification of live GitHub branch
protection for `uesugitorachiyo/ao-forge` `main`.

## Verification Command

```sh
scripts/verify-branch-protection.sh
```

The script queries GitHub with `gh api`, verifies the live `main` protection
state, checks repository rulesets for visibility, and emits
`ao.forge.branch-protection-audit.v0.1` JSON.

## Latest Evidence

- Verified at: `2026-06-23T07:57:58Z`
- Repository: `uesugitorachiyo/ao-forge`
- Branch: `main`
- Status: `passed`
- Rulesets observed: `0`

Verified controls:

- Required pull request review protection is enabled.
- Stale pull request approvals are dismissed when new commits are pushed.
- Required status checks are strict.
- Required checks include `Go ubuntu-latest`, `Go macos-latest`,
  `Go windows-latest`, `License policy`, `Workflow lint`,
  `GoalRun fixture smoke`, `Production readiness audit`, and
  `Release preview dry-run audit`.
- Admins are included in enforcement.
- Linear history is required.
- Force pushes are disabled.
- Branch deletions are disabled.

Rulesets are not required for this repository because the classic branch
protection rule on `main` enforces the required production-readiness controls.
If AO Forge later moves from classic branch protection to rulesets, update this
evidence document and `scripts/verify-branch-protection.sh` in the same pull
request.
