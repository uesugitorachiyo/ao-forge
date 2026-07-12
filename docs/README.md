# AO Forge Documentation

This index groups public AO Forge docs by purpose. The top-level README stays
short; this file is the detailed navigation point for contracts, release
evidence, and examples.

## Architecture

- [AO Forge v0.1 Architecture](design/AO-FORGE-V0.1.md)
- [GoalRun Contract](design/GOAL-RUNS.md)
- [AO2 Pulse GoalRun Loop](design/AO2-PULSE-GOAL-RUN-LOOP.md)
- [Phase 0 Roadmap](roadmap/PHASE-0.md)

## Contracts

- [GoalRun v0.1 Schema](contracts/goal-run-v0.1.schema.json)
- [GoalRun Context Handoff v0.1 Schema](contracts/goal-run-context-handoff-v0.1.schema.json)
- [GoalRun Verification v0.1 Schema](contracts/goal-run-verification-v0.1.schema.json)
- [GoalRun Update Audit v0.1 Schema](contracts/goal-run-update-audit-v0.1.schema.json)
- [GoalRun Evidence Verify v0.1 Schema](contracts/goal-run-evidence-verify-v0.1.schema.json)
- [GoalRun Evidence Lint v0.1 Schema](contracts/goal-run-evidence-lint-v0.1.schema.json)
- [GoalRun Retained Evidence v0.1 Schema](contracts/goal-run-retained-evidence-v0.1.schema.json)
- [GoalRun Retained Evidence Audit v0.1 Schema](contracts/goal-run-retained-evidence-audit-v0.1.schema.json)
- [GoalRun Retained Evidence Cleanup v0.1 Schema](contracts/goal-run-retained-evidence-cleanup-v0.1.schema.json)
- [GoalRun Readiness Audit v0.1 Schema](contracts/goal-run-readiness-audit-v0.1.schema.json)
- [GoalRun Lease Expiry Replay v0.1 Schema](contracts/goal-run-lease-expiry-replay-v0.1.schema.json)
- [Live Mutation Dry-Run Plan v0.1 Schema](contracts/live-mutation-dry-run-plan-v0.1.schema.json)
- [Live Docs Execution Guard v0.1 Schema](contracts/live-docs-execution-guard-v0.1.schema.json)
- [Factory Brief v0.1 Schema](contracts/factory-brief-v0.1.schema.json)
- [Factory Plan v0.1 Schema](contracts/factory-plan-v0.1.schema.json)
- [Factory Packet v0.1 Schema](contracts/factory-packet-v0.1.schema.json)
- [Covenant Decision Fixture v0.1 Schema](contracts/covenant-decision-fixture-v0.1.schema.json)
- [Covenant Gate Result v0.1 Schema](contracts/covenant-gate-result-v0.1.schema.json)
- [Release Candidate v0.1 Schema](contracts/release-candidate-v0.1.schema.json)
- [Release Preview Audit v0.1 Schema](contracts/release-preview-audit-v0.1.schema.json)
- [Release Preview Inspect v0.1 Schema](contracts/release-preview-inspect-v0.1.schema.json)
- [Release Artifact Inventory v0.1 Schema](contracts/release-artifact-inventory-v0.1.schema.json)
- [Release Attestation Plan v0.1 Schema](contracts/release-attestation-plan-v0.1.schema.json)
- [Release Evidence Bundle v0.1 Schema](contracts/release-evidence-bundle-v0.1.schema.json)
- [Release Verify Audit v0.1 Schema](contracts/release-verify-audit-v0.1.schema.json)
- [Release Install Verify Audit v0.1 Schema](contracts/release-install-verify-audit-v0.1.schema.json)
- [Release Rollback Audit v0.1 Schema](contracts/release-rollback-audit-v0.1.schema.json)
- [Production Promotion Audit v0.1 Schema](contracts/production-promotion-audit-v0.1.schema.json)
- [Production Readiness Audit v0.1 Schema](contracts/production-readiness-audit-v0.1.schema.json)

## Release And Readiness

- [Public Repo Policy](security/PUBLIC-REPO-POLICY.md)
- [Branch Protection Runbook](release/BRANCH-PROTECTION.md)
- [Branch Protection Evidence](release/BRANCH-PROTECTION-EVIDENCE.md)
- [First Public Release Checklist](release/FIRST-PUBLIC-RELEASE.md)
- [Release Threat Model](security/RELEASE-THREAT-MODEL.md)
- [Verified Foundation Baseline](foundation/VERIFIED-BASELINE.md)
- [Foundation Baseline JSON](foundation/foundation-baseline.v0.1.json)
- [v0.1.0 Release Notes Draft](release/V0.1.0-RELEASE-NOTES.md)
- [v0.1.2 Release Notes Draft](release/V0.1.2-RELEASE-NOTES.md)
- [v0.1.3 Release Notes](release/V0.1.3-RELEASE-NOTES.md)

## Examples

- [Example GoalRun](../examples/goals/ao2-weekend-hardening.goal-run.json)
- [Example GoalRun Context Handoff](../examples/goals/ao2-weekend-hardening.context-handoff.json)
- [Example GoalRun Verification](../examples/goals/ao2-weekend-hardening.goal-run-verification.json)
- [Example GoalRun Update Audit](../examples/goals/ao2-weekend-hardening.goal-run-update-audit.json)
- [AO2 Pulse Handoff GoalRun](../examples/goals/ao2-pulse-handoff.goal-run.json)
- [AO2 Pulse Handoff Update Audit](../examples/goals/ao2-pulse-handoff.goal-run-update-audit.json)
- [GoalRun Lease Expiry Replay Fixture](../examples/goals/month6.goal-run-lease-expiry-replay.v0.1.example.json)
- [Retained GoalRun Evidence Fixture](../examples/goals/ao2-retained-evidence.goal-run.json)
- [Live Mutation Dry-Run Plan Fixture](../examples/live-mutation/docs-only-dry-run-plan.json)
- [Live Mutation Docs Multi Dry-Run Plan Fixture](../examples/live-mutation/docs-multi-dry-run-plan.json)
- [Live Docs Execution Guard Fixture](../examples/live-docs-guard/execution-guard.ready.json)
- [Live Docs Multi Execution Guard Fixture](../examples/live-docs-guard/execution-guard.docs-multi.ready.json)
- [AO Command RSI Manifest Retention Proof](evidence/goals/ao2-weekend-hardening/20260619T180000Z-verification/ao-command-rsi-manifest-retention-proof.json)
- [Bounded RSI Improvement Chain Retention Proof](evidence/goals/ao2-weekend-hardening/20260619T180000Z-verification/bounded-rsi-improvement-chain-retention-proof.json)
- [AO Stack RSI Chain-Binding Readback Retention Proof](evidence/goals/ao2-weekend-hardening/20260619T180000Z-verification/ao-stack-rsi-chain-binding-readback-retention-proof.json)
- [AO Architecture RSI Pin Readback](evidence/architecture/ao-architecture-rsi-pin-readback.json)
- [Release Candidate Handoff Fixture](../examples/release-preview/release-candidate.v0.1.example.json)
- [Release Preview Fixtures](../examples/release-preview/)
- [Example Vertical Slice](../examples/vertical-slices/risky-pr-factory.factory.json)
- [Example Deterministic Plan](../examples/plans/risky-pr-factory-plan.json)
- [Example Covenant Gate Result](../examples/gates/allow-local-plan.gate.json)
- [Example Factory Packet](../examples/packets/risky-pr-factory-packet.json)
