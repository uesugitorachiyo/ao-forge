# Release Signers

`RELEASE-SIGNERS.json` is the source of truth for release tag signing
eligibility. Release tags for production-stable candidates must be signed by an
active signer in that policy and verified by the release workflows.

## Eligibility

An active release signer must:

- control the GitHub account listed in `RELEASE-SIGNERS.json`;
- publish the matching public key under `docs/release/signers/`;
- use a key fingerprint recorded in `RELEASE-SIGNERS.json`;
- have the key reviewed in a pull request before signing release tags;
- be able to rotate or revoke the key without deleting historical evidence.

The initial active signer is `uesugitorachiyo` with fingerprint
`8D5D9D83E11871F366A02E3FF8C8B4ACE0A1DFD8`.

## Workflow Enforcement

`Release Publish` imports keys from `docs/release/signers/`, verifies the
annotated release tag signature, extracts the signing fingerprint, and rejects
the release when the fingerprint is not active in `RELEASE-SIGNERS.json`.

`Release Verify` checks `RELEASE-SIGNERS.json` the same way for non-legacy
verification. `require_signed_tag=false` remains available only for releases
that predate the signed-tag policy.

## Key Rotation

To rotate a release signing key:

1. Add the replacement public key to `docs/release/signers/`.
2. Add a new `active_signers` entry to `RELEASE-SIGNERS.json`.
3. Move the old signer entry to `retired_signers` after the replacement key has
   signed and verified a release candidate.
4. Keep historical signer metadata and public keys in the repository.
5. Run Release Preview, Release Rehearsal, Release Publish, and Release Verify
   with the replacement key before production-stable promotion.

## Revocation

If a key is lost, compromised, or no longer controlled by the listed signer:

1. Move the signer entry from `active_signers` to `retired_signers`.
2. Set `status` to `revoked` and add the revocation date.
3. Open a public correction issue or release note when any published release may
   be affected.
4. Use a new signed annotated corrective tag for replacement artifacts.
5. Preserve old release evidence; do not delete signatures, tags, or artifacts
   to hide the problem.
