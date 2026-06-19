# Public Asset Install Verification

`Release Install Verify` checks the release assets exactly as users receive
them from GitHub. It is read-only and must not create, edit, delete, or republish
release state.

## Required Inputs

Run the workflow manually with:

- `tag`: the published release tag;
- `require_evidence_bundle=true` for current releases;
- `require_evidence_bundle=false` only for legacy releases that predate the
  release evidence bundle.

## What It Checks

The workflow downloads public release assets with `gh release download`, verifies
`checksums.txt`, extracts the Linux and Darwin archives, inspects the Windows zip
for `ao-forge.exe`, and runs the Linux binary from the downloaded public asset.

It writes `release-install-verify-audit.json` and uploads the
`release-install-verify-audit` artifact. The audit records the tag, release URL,
asset digests, extraction checks, and Linux smoke test result.

## Promotion Use

`Production Promotion` should require a successful `Release Install Verify` run
before production-stable language is used. A release remains public-preview or a
candidate when public assets cannot be downloaded, extracted, checksum-verified,
or smoke-tested.
