#!/usr/bin/env python3

import pathlib
import re
import sys


def main():
    root = pathlib.Path(__file__).resolve().parents[1]
    version_path = root / "VERSION"
    if version_path.is_file() and not version_path.is_symlink():
        version = version_path.read_text(encoding="ascii").strip()
        if not re.fullmatch(r"[0-9]+\.[0-9]+\.[0-9]+(?:[-.][0-9A-Za-z.-]+)?", version):
            raise SystemExit("repository VERSION is malformed")
        print(version)
        return

    # Compatibility for source commits created before VERSION became canonical.
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
