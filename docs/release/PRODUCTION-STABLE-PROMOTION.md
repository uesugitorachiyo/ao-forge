# Production-Stable Promotion

AO Forge releases start as public-preview releases. Do not describe a release as
production-stable until the `Production Promotion` workflow has passed for that
tag and the uploaded `production-promotion-audit` artifact has been reviewed.

## Required Evidence

Before running promotion audit, collect:

- the published release tag;
- a successful `Release Verify` run for the same tag, using default
  `require_evidence_bundle=true` and `require_signed_tag=true`. The Release
  Verify audit artifact must validate against its contract and prove default
  signed-tag and evidence-bundle controls;
- a successful `Release Install Verify` run for the same tag. The install
  verification audit artifact must validate against its contract and match the
  promoted tag and release commit;
- confirmation that the tag signer is active in `RELEASE-SIGNERS.json`;
- a successful `Release Rollback` audit-only run for the same tag;
- confirmation that the release has completed the agreed soak window.

The default soak window is 24 hours after publication. Increase it for releases
with broad user impact, migration risk, or new release automation.

## Promotion Audit

Run the `Production Promotion` workflow manually with:

- `tag`: the published release tag;
- `release_verify_run_id`: the successful `Release Verify` run ID;
- `release_install_verify_run_id`: the successful `Release Install Verify` run ID;
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
- `Release Verify` audit evidence validates against its contract, matches the
  promoted tag and release commit, and proves default signed-tag and
  evidence-bundle controls;
- `Release Install Verify` completed successfully for the release;
- `Release Install Verify` audit evidence validates against its contract and
  matches the promoted tag and release commit;
- `Release Rollback` audit-only completed successfully for the release;
- rollback audit evidence proves no release mutation was requested and
  mutation-relevant release fields stayed unchanged.

If any criterion fails, keep the release status language at public-preview,
candidate, or blocked. Do not describe a release as production-stable in release
notes, README text, issue comments, or external announcements.

## Blockers

Block promotion when any of these are true:

- Release Verify used legacy overrides such as `require_signed_tag=false` or
  `require_evidence_bundle=false`;
- Release Verify audit evidence is missing, invalid, or belongs to a different
  release tag or commit;
- rollback audit evidence is missing, not audit-only, or does not prove
  mutation-relevant release fields stayed unchanged;
- install verification audit evidence is missing, invalid, or belongs to a
  different release tag or commit;
- public assets cannot be downloaded or checksums cannot be verified;
- a known severity-high or severity-critical issue affects the release;
- release notes require correction;
- a signer, reviewer, or release operator disputes the promotion evidence.

Use `Release Rollback` for public correction if a promoted release later becomes
known-bad. Preserve all release, rollback, and promotion evidence.
