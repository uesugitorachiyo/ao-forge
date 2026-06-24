#!/usr/bin/env bash
set -euo pipefail

failures=()

add_failure() {
  failures+=("$1")
}

scan_file() {
  local file="$1"

  if [[ ! -f "$file" ]]; then
    return
  fi

  if grep -Iq . "$file"; then
    :
  else
    return
  fi

  if grep -InE -e '-----BEGIN (RSA |DSA |EC |OPENSSH )?PRIVATE KEY-----' "$file" >/dev/null; then
    add_failure "$file: private key material marker"
  fi

  if grep -InE -e '(gh[pousr]_[A-Za-z0-9_]{36,}|AKIA[0-9A-Z]{16}|xox[baprs]-[A-Za-z0-9-]{20,})' "$file" >/dev/null; then
    add_failure "$file: high-confidence credential token"
  fi

  if grep -InEi -e '(api[_-]?key|access[_-]?token|auth[_-]?token|password|secret|private[_-]?key)[[:space:]]*[:=][[:space:]]*["'\'']?[A-Za-z0-9_./+=:-]{20,}' "$file" >/dev/null; then
    add_failure "$file: credential-like assignment"
  fi

  if grep -InE -e '(/Users/[A-Za-z0-9._-]+/|/home/[A-Za-z0-9._-]+/|[A-Za-z]:[\\/]+Users[\\/]+[A-Za-z0-9._-]+[\\/]+)' "$file" >/dev/null; then
    add_failure "$file: machine-local home path"
  fi
}

while IFS= read -r -d '' file; do
  scan_file "$file"
done < <(git ls-files -z)

if [[ "${#failures[@]}" -gt 0 ]]; then
  printf 'public repo policy check failed:\n' >&2
  printf ' - %s\n' "${failures[@]}" >&2
  exit 1
fi

printf 'public repo policy check passed\n'
