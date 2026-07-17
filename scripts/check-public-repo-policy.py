#!/usr/bin/env python3
import pathlib
import re
import subprocess
import sys


PRIVATE_KEY_RE = re.compile(r"-----BEGIN (RSA |DSA |EC |OPENSSH )?PRIVATE KEY-----")
TOKEN_RE = re.compile(r"(gh[pousr]_[A-Za-z0-9_]{36,}|AKIA[0-9A-Z]{16}|xox[baprs]-[A-Za-z0-9-]{20,})")
ASSIGNMENT_RE = re.compile(
    r"(api[_-]?key|access[_-]?token|auth[_-]?token|password|secret|private[_-]?key)"
    r"\s*[:=]\s*[\"']?[A-Za-z0-9_./+=:-]{20,}",
    re.IGNORECASE,
)
LOCAL_HOME_RE = re.compile(r"(/Users/[A-Za-z0-9._-]+/|/home/[A-Za-z0-9._-]+/|[A-Za-z]:[\\/]+Users[\\/]+[A-Za-z0-9._-]+[\\/]+)")


def tracked_files():
    output = subprocess.check_output(["git", "ls-files", "-z"])
    return [pathlib.Path(item.decode("utf-8")) for item in output.split(b"\0") if item]


def read_text_if_text(path: pathlib.Path):
    if not path.is_file():
        return None
    data = path.read_bytes()
    if b"\0" in data:
        return None
    try:
        return data.decode("utf-8")
    except UnicodeDecodeError:
        try:
            return data.decode("utf-8-sig")
        except UnicodeDecodeError:
            return None


def main() -> int:
    failures = []
    for path in tracked_files():
        text = read_text_if_text(path)
        if text is None:
            continue
        if PRIVATE_KEY_RE.search(text):
            failures.append(f"{path}: private key material marker")
        if TOKEN_RE.search(text):
            failures.append(f"{path}: high-confidence credential token")
        if ASSIGNMENT_RE.search(text):
            failures.append(f"{path}: credential-like assignment")
        if LOCAL_HOME_RE.search(text):
            failures.append(f"{path}: machine-local home path")

    if failures:
        print("public repo policy check failed:", file=sys.stderr)
        for failure in failures:
            print(f" - {failure}", file=sys.stderr)
        return 1
    print("public repo policy check passed")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
