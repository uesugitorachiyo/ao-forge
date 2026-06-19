#!/usr/bin/env bash
set -euo pipefail

usage() {
  cat >&2 <<'USAGE'
usage: scripts/ao2-pulse-goal-readiness.sh --goal-run <goal-run.json> --out <readiness-audit.json> [--to <phase>] [--now <RFC3339>]
USAGE
}

goal_run=""
out=""
to_phase=""
now=""

while [[ $# -gt 0 ]]; do
  case "$1" in
    --goal-run)
      if [[ $# -lt 2 || "$2" == --* ]]; then
        echo "ao2 pulse readiness: --goal-run requires a value" >&2
        exit 2
      fi
      goal_run="$2"
      shift 2
      ;;
    --out)
      if [[ $# -lt 2 || "$2" == --* ]]; then
        echo "ao2 pulse readiness: --out requires a value" >&2
        exit 2
      fi
      out="$2"
      shift 2
      ;;
    --to)
      if [[ $# -lt 2 || "$2" == --* ]]; then
        echo "ao2 pulse readiness: --to requires a value" >&2
        exit 2
      fi
      to_phase="$2"
      shift 2
      ;;
    --now)
      if [[ $# -lt 2 || "$2" == --* ]]; then
        echo "ao2 pulse readiness: --now requires a value" >&2
        exit 2
      fi
      now="$2"
      shift 2
      ;;
    --help|-h)
      usage
      exit 0
      ;;
    *)
      echo "ao2 pulse readiness: unexpected argument $1" >&2
      usage
      exit 2
      ;;
  esac
done

if [[ -z "$goal_run" ]]; then
  echo "ao2 pulse readiness: missing required --goal-run" >&2
  usage
  exit 2
fi
if [[ -z "$out" ]]; then
  echo "ao2 pulse readiness: missing required --out" >&2
  usage
  exit 2
fi

root="$(git rev-parse --show-toplevel)"
cd "$root"

if [[ -n "${AO_FORGE_BIN:-}" ]]; then
  forge_bin="$AO_FORGE_BIN"
else
  forge_bin="${RUNNER_TEMP:-/tmp}/ao-forge-ao2-pulse-readiness/forge"
  mkdir -p "$(dirname "$forge_bin")"
  go build -o "$forge_bin" ./cmd/forge
fi

if [[ ! -x "$forge_bin" ]]; then
  echo "ao2 pulse readiness: forge binary is not executable: $forge_bin" >&2
  exit 1
fi

out_dir="$(dirname "$out")"
mkdir -p "$out_dir"
tmp="$(mktemp "${out_dir}/.goal-readiness.XXXXXX.json")"
trap 'rm -f "$tmp"' EXIT

args=(goal readiness --goal-run "$goal_run" --json)
if [[ -n "$to_phase" ]]; then
  args+=(--to "$to_phase")
fi
if [[ -n "$now" ]]; then
  args+=(--now "$now")
fi

set +e
"$forge_bin" "${args[@]}" > "$tmp"
readiness_code=$?
set -e

"$forge_bin" contract validate \
  --schema docs/contracts/goal-run-readiness-audit-v0.1.schema.json \
  --document "$tmp" >/dev/null

mv "$tmp" "$out"
trap - EXIT

if [[ "$readiness_code" -ne 0 ]]; then
  echo "ao2_pulse_readiness=failed"
  echo "readiness_audit=$out"
  exit "$readiness_code"
fi

echo "ao2_pulse_readiness=passed"
echo "readiness_audit=$out"
