# AO Forge Release Preview

`forge release-preview` rehearses a release without mutating local or remote
release state. It is intended to run before `forge run --live --confirm-release`
or any manual GitHub release operation.

## What It Checks

- workspace path exists and is a git repository;
- worktree is clean;
- HEAD commit resolves;
- `origin` resolves to a GitHub repository;
- release tag is either available for creation or already points to HEAD;
- declared artifacts exist and have SHA-256 checksums;
- the preview itself does not create tags, push refs, publish releases, or
  require network access.

## Example

Generate the checksum manifest with AO Forge before running the preview:

```sh
forge artifact checksums \
  --artifact ./dist/ao-forge_Linux_x86_64.tar.gz \
  --out ./dist/checksums.txt
```

```sh
forge release-preview \
  --workspace . \
  --artifact ./dist/ao-forge_Linux_x86_64.tar.gz \
  --artifact ./dist/checksums.txt \
  --out release-preview-audit.json
```

If `--tag` is omitted, AO Forge resolves it from `AO_FORGE_RELEASE_TAG` or the
workspace `VERSION` file. The command exits `0` when every check passes. It exits
`1` and still writes the audit when release preview checks are blocked.

Inspect the audit summary before approving a confirmed release:

```sh
forge release-preview inspect --audit release-preview-audit.json
```

GitHub Actions runs the same non-mutating preview path on pull requests and
pushes to `main`. The workflow uploads the machine-readable audit, an inspect
summary, and artifact checksums for review without using write permissions or
live release flags.

## Audit Contract

The audit uses schema version `ao.forge.release-preview-audit.v0.1`. The formal
schema is published at `docs/contracts/release-preview-audit-v0.1.schema.json`
and includes:

- `status`: `passed` or `blocked`;
- `workspace`, `github_repo`, `tag`, and `head_commit`;
- `mutates_releases: false` and `network_required: false`;
- structured `checks`;
- artifact `path`, `size_bytes`, `sha256`, `status`, and `provenance`;
- `release_notes_preview`;
- `next_actions`.

## Operator Rule

Do not run a live confirmed release if the preview audit is `blocked`. Fix the
failed checks, regenerate artifacts if needed, rerun the preview, and only then
proceed to release mutation.

Confirmed release mutation through `forge run`, `forge once`, or `forge resume`
requires the passed audit:

```sh
forge run \
  --plan factory-plan.json \
  --gate-result gate-result.json \
  --live \
  --confirm-release \
  --release-preview-audit release-preview-audit.json
```

AO Forge validates that the audit is passed, non-mutating, local-only, and bound
to the same workspace, tag, and HEAD commit before release mutation can proceed.

## Privacy

The preview audit can include local workspace and artifact paths. Keep ad hoc
operator audits out of public commits unless the paths are intentionally public
and safe to disclose.
