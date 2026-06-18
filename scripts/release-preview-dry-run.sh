#!/usr/bin/env bash
set -euo pipefail

root="$(git rev-parse --show-toplevel)"
cd "$root"

if [[ -n "$(git status --porcelain)" ]]; then
  echo "release preview dry-run requires a clean workspace" >&2
  git status --short >&2
  exit 1
fi

out_dir="${AO_FORGE_RELEASE_PREVIEW_OUT:-${RUNNER_TEMP:-/tmp}/ao-forge-release-preview}"
tag="${AO_FORGE_RELEASE_PREVIEW_TAG:-v0.0.0-preview}"
mkdir -p "$out_dir"

forge_bin="${AO_FORGE_BIN:-$out_dir/forge}"
if [[ ! -x "$forge_bin" ]]; then
  go build -o "$forge_bin" ./cmd/forge
fi

preview_artifact="$out_dir/ao-forge-preview-artifact.txt"
checksums="$out_dir/checksums.txt"
audit="$out_dir/release-preview-audit.json"
inspect="$out_dir/release-preview-inspect.txt"
inspect_json="$out_dir/release-preview-inspect.json"

{
  echo "AO Forge release preview artifact"
  echo "commit=$(git rev-parse HEAD)"
  echo "tag=$tag"
} > "$preview_artifact"

"$forge_bin" artifact checksums --artifact "$preview_artifact" --out "$checksums"

"$forge_bin" release-preview \
  --workspace "$root" \
  --tag "$tag" \
  --artifact "$preview_artifact" \
  --artifact "$checksums" \
  --out "$audit"

"$forge_bin" release-preview inspect --audit "$audit" > "$inspect"
"$forge_bin" release-preview inspect --audit "$audit" --json > "$inspect_json"

echo "release_preview_audit=$audit"
echo "release_preview_inspect=$inspect"
echo "release_preview_inspect_json=$inspect_json"
