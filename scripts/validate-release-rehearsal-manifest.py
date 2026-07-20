#!/usr/bin/env python3

import argparse
import base64
import binascii
import hashlib
import json
import os
import pathlib
import sys


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("--output", required=True)
    args = parser.parse_args()

    encoded = os.environ.get("APPROVED_MANIFEST_BASE64", "")
    if len(encoded) > 65_536:
        raise ValueError("approved manifest input exceeds encoded size limit")
    try:
        raw = base64.b64decode(encoded, validate=True)
    except binascii.Error as error:
        raise ValueError(f"approved manifest base64 is malformed: {error}") from error
    if not raw or len(raw) > 49_152:
        raise ValueError("approved manifest decoded size is invalid")

    actual = hashlib.sha256(raw).hexdigest()
    if actual != os.environ.get("APPROVED_MANIFEST_DIGEST"):
        raise ValueError("approved manifest digest mismatch")
    try:
        manifest = json.loads(raw)
    except (UnicodeDecodeError, json.JSONDecodeError) as error:
        raise ValueError(f"approved manifest JSON is malformed: {error}") from error

    expected = {
        "schema_version": "ao.forge.approved-release-manifest.v1",
        "source_sha": os.environ.get("SOURCE_COMMIT"),
        "version": os.environ.get("VERSION"),
        "tag": os.environ.get("TAG"),
        "targets": ["linux-x86_64", "macos-aarch64", "windows-x86_64"],
        "publication_authorized": False,
    }
    if not isinstance(manifest, dict) or set(manifest) != set(expected):
        raise ValueError("approved manifest fields mismatch")
    if manifest != expected:
        raise ValueError("approved manifest identity or authority mismatch")

    output = pathlib.Path(args.output)
    output.parent.mkdir(parents=True, exist_ok=True)
    output.write_bytes(raw)


if __name__ == "__main__":
    try:
        main()
    except (OSError, KeyError, ValueError) as error:
        print(f"approved manifest validation failed: {error}", file=sys.stderr)
        raise SystemExit(1)
