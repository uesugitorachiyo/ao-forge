# Production-Stable Promotion

AO Forge releases start as public-preview releases. Do not describe a release as
production-stable until the `Production Promotion` workflow has passed for that
tag and the uploaded `production-promotion-audit` artifact has been reviewed.

## Required Evidence

Before running promotion audit, collect:

- the published release tag;
- a successful `Release Verify` run for the same tag, using default
  `require_evidence_bundle=true` and `require_signed_tag=true`;
- a successful `Release Rollback` audit-only run for the same tag;
- confirmation that the release has completed the agreed soak window.

The default soak window is 24 hours after publication. Increase it for releases
with broad user impact, migration risk, or new release automation.

## Promotion Audit

Run the `Production Promotion` workflow manually with:

- `tag`: the published release tag;
- `release_verify_run_id`: the successful `Release Verify` run ID;
- `release_rollback_audit_run_id`: the successful `Release Rollback` audit-only
  run ID;
- `min_soak_hours`: the minimum published-release soak window;
- `confirm_default_release_verify=true`: confirms the Release Verify run did not
  use legacy overrides such as `require_signed_tag=false` or
  `require_evidence_bundle=false`;
- `confirm_no_known_blockers=true`: confirms there are no known severity-high or
  severity-critical blockers for the release;
- `confirm_promotion_audit=true`.

The workflow is read-only. It must not publish releases, edit notes, move tags,
or mark releases latest. It writes `production-promotion-audit.json` and uploads
the `production-promotion-audit` artifact for review.

## Pass Criteria

A release may be called production-stable only when all criteria pass:

- release exists and is published, not draft;
- release is not marked prerelease;
- release tag matches the requested tag;
- release age is at least `min_soak_hours`;
- `Release Verify` completed successfully for the release;
- `Release Rollback` audit-only completed successfully for the release;
- rollback audit evidence proves no release mutation was requested.

If any criterion fails, keep the release status language at public-preview,
candidate, or blocked. Do not describe a release as production-stable in release
notes, README text, issue comments, or external announcements.

## Blockers

Block promotion when any of these are true:

- Release Verify used legacy overrides such as `require_signed_tag=false` or
  `require_evidence_bundle=false`;
- rollback audit evidence is missing or not audit-only;
- public assets cannot be downloaded or checksums cannot be verified;
- a known severity-high or severity-critical issue affects the release;
- release notes require correction;
- a signer, reviewer, or release operator disputes the promotion evidence.

Use `Release Rollback` for public correction if a promoted release later becomes
known-bad. Preserve all release, rollback, and promotion evidence.
