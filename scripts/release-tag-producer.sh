#!/usr/bin/env bash
set -euo pipefail

mode=""
source_sha=""
version=""
tag=""
manifest_digest=""
workflow_run_id=""
plan=""
expected_plan_digest=""
confirmation=""

while [[ $# -gt 0 ]]; do
  case "$1" in
    --dry-run|--live) mode="$1"; shift ;;
    --source-sha) source_sha="${2:-}"; shift 2 ;;
    --version) version="${2:-}"; shift 2 ;;
    --tag) tag="${2:-}"; shift 2 ;;
    --manifest-digest) manifest_digest="${2:-}"; shift 2 ;;
    --workflow-run-id) workflow_run_id="${2:-}"; shift 2 ;;
    --plan) plan="${2:-}"; shift 2 ;;
    --expected-plan-digest) expected_plan_digest="${2:-}"; shift 2 ;;
    --confirmation) confirmation="${2:-}"; shift 2 ;;
    *) echo "unknown argument: $1" >&2; exit 2 ;;
  esac
done

[[ "$mode" == "--dry-run" || "$mode" == "--live" ]] || { echo "exactly one of --dry-run or --live is required" >&2; exit 2; }
[[ "$source_sha" =~ ^[0-9a-f]{40}$ ]] || { echo "source SHA is malformed" >&2; exit 2; }
[[ "$version" =~ ^[0-9]+\.[0-9]+\.[0-9]+([-.][0-9A-Za-z.-]+)?$ ]] || { echo "version is malformed" >&2; exit 2; }
[[ "$tag" == "v$version" ]] || { echo "tag must equal v plus version" >&2; exit 2; }
[[ "$manifest_digest" =~ ^[0-9a-f]{64}$ ]] || { echo "manifest digest is malformed" >&2; exit 2; }
[[ "$workflow_run_id" =~ ^[1-9][0-9]*$ ]] || { echo "workflow run id is malformed" >&2; exit 2; }
[[ "$expected_plan_digest" =~ ^[0-9a-f]{64}$ ]] || { echo "plan digest is malformed" >&2; exit 2; }
[[ -f "$plan" && ! -L "$plan" ]] || { echo "immutable plan must be a regular file" >&2; exit 2; }
[[ "$(git rev-parse HEAD)" == "$source_sha" ]] || { echo "working source does not equal qualified source" >&2; exit 1; }
git diff --quiet
git diff --cached --quiet

actual_plan_digest=$(shasum -a 256 "$plan" | awk '{print $1}')
[[ "$actual_plan_digest" == "$expected_plan_digest" ]] || { echo "immutable plan digest mismatch" >&2; exit 1; }
python3 scripts/verify-release-rehearsal.py verify-plan \
  --version "$version" \
  --tag "$tag" \
  --source-sha "$source_sha" \
  --manifest-digest "$manifest_digest" \
  --workflow-identity ".github/workflows/release-rehearsal.yml@${source_sha}" \
  --workflow-run-id "$workflow_run_id" \
  --plan "$plan"

git show-ref --verify --quiet "refs/tags/$tag" && { echo "local tag already exists" >&2; exit 1; }
remote_tag=$(git ls-remote --tags origin "refs/tags/$tag" "refs/tags/$tag^{}")
[[ -z "$remote_tag" ]] || { echo "remote tag already exists" >&2; exit 1; }

if [[ "$mode" == "--dry-run" ]]; then
  [[ -z "$confirmation" ]] || { echo "dry run must not carry live confirmation" >&2; exit 1; }
  printf 'tag_production_status=not_attempted\ntag_creation_attempted=false\ntag_push_attempted=false\n'
  exit 0
fi

expected_confirmation="publish-tag:${tag}:${source_sha}:${workflow_run_id}:${expected_plan_digest}"
[[ "$confirmation" == "$expected_confirmation" ]] || { echo "exact confirmation mismatch" >&2; exit 1; }
git tag -s "$tag" "$source_sha" -m "AO Forge $tag" -m "source=$source_sha rehearsal_run=$workflow_run_id plan_sha256=$expected_plan_digest"
git verify-tag "$tag"
git push --atomic origin "refs/tags/$tag"
printf 'tag_production_status=published\ntag_creation_attempted=true\ntag_push_attempted=true\n'
