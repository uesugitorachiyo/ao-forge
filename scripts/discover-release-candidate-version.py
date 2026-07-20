#!/usr/bin/env python3

import pathlib
import re
import sys


def main():
    root = pathlib.Path(__file__).resolve().parents[1]
    versions = []
    pattern = re.compile(r"^V(\d+)\.(\d+)\.(\d+)-RELEASE-NOTES\.md$")
    for path in (root / "docs" / "release").glob("V*-RELEASE-NOTES.md"):
        match = pattern.fullmatch(path.name)
        if match:
            versions.append(tuple(int(part) for part in match.groups()))
    if not versions:
        raise SystemExit("no checked-in release notes versions found")
    major, minor, patch = max(versions)
    print(f"{major}.{minor}.{patch + 1}")


if __name__ == "__main__":
    main()
