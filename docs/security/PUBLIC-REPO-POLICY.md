# Public Repo Policy

AO Forge and the public AO repos are public-by-default software repositories,
not private operations journals. Public files must be safe for an unknown reader
to clone, mirror, index, and redistribute.

## Public-Safe Content

Public repos may contain:

- source code, tests, examples, schemas, fixtures, and runbooks that do not
  expose private infrastructure, tokens, unreleased customer data, or personal
  workspace paths;
- release evidence, readiness audits, and branch-protection evidence that use
  public repository names, public workflow run IDs, public tags, and redacted
  or synthetic fixtures;
- operator guidance that explains roles, boundaries, gates, and expected
  evidence without exposing private machine state;
- generated release artifacts that include `LICENSE` and `NOTICE` and match the
  public Apache-2.0 license policy.

## Not Public-Safe

Do not commit:

- secrets, tokens, SSH material, API keys, cookies, or credential-derived
  values;
- private repo names, private issue/PR URLs, hidden branch plans, billing data,
  account identifiers, or access-control screenshots;
- local absolute paths, home-directory paths, machine hostnames, private
  workspace names, or unredacted terminal transcripts;
- internal strategy notes that describe unpublished security weaknesses,
  embargoed incidents, or private coordination details;
- release artifacts that omit `LICENSE` or `NOTICE`.

Use synthetic examples or redacted fixtures when a public contract needs a
sample document.

## License Policy

All public AO repos use a single project license:

```text
Apache-2.0
```

Each repo must keep exactly one root `LICENSE` containing the canonical Apache
License 2.0 text and one root `NOTICE`. Do not add `LICENSE-MIT`,
`LICENSE-APACHE`, or a dual-license expression unless the public license policy
is intentionally changed across the stack.

Package metadata must agree with the root license:

- Rust workspaces: `license = "Apache-2.0"`
- Node packages: `"license": "Apache-2.0"`
- Python packages/plugins: `license = "Apache-2.0"`

CI must run `scripts/check-license-policy.sh` to enforce the root files and
metadata. CI must also run `scripts/check-public-repo-policy.sh` against tracked
files so private-key markers, credential-shaped assignments, high-confidence
tokens, and machine-local home paths cannot enter the public repository
unnoticed. Release workflows must run `scripts/check-license-policy.sh
--scan-archives` after building archives and before checksums, attestations, or
publication.

## Release Artifact Rule

Every distributable archive must include:

- `LICENSE`
- `NOTICE`

The license policy guard checks `.tar.gz`, `.tgz`, `.zip`, and `.whl` archives under
`dist*` or `release` when called with `--scan-archives`. Release
workflows should run the guard after building archives and before checksums,
attestations, or publication.

## Review Rule

Before a public release or public-repo cleanup merge, verify:

1. `scripts/check-license-policy.sh` passes.
2. `scripts/check-public-repo-policy.sh` passes.
3. Public export or stabilization checks pass for repos that have them.
4. Release archives include `LICENSE` and `NOTICE`.
5. Evidence files use public-safe repo names, paths, and identifiers.
6. Any non-public operational notes stay outside the public repo.
