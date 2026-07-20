#!/usr/bin/env python3

import argparse
import hashlib
import json
import pathlib
import re
import stat
import sys
import tarfile
import zipfile

CANDIDATE_SCHEMA = "ao.forge.release-rehearsal-candidate.v1"
PROVENANCE_SCHEMA = "ao.forge.release-rehearsal-provenance.v1"
SMOKE_SCHEMA = "ao.forge.release-rehearsal-smoke.v1"
PLAN_SCHEMA = "ao.forge.immutable-promotion-plan.v1"
SHA_RE = re.compile(r"^[0-9a-f]{40}$")
DIGEST_RE = re.compile(r"^[0-9a-f]{64}$")
VERSION_RE = re.compile(r"^[0-9]+\.[0-9]+\.[0-9]+(?:[-.][0-9A-Za-z.-]+)?$")
RUN_RE = re.compile(r"^[1-9][0-9]*$")

TARGETS = {
    "linux-x86_64": {
        "goos": "linux",
        "goarch": "amd64",
        "binary": "forge",
        "extension": "tar.gz",
        "magic": b"\x7fELF",
    },
    "macos-aarch64": {
        "goos": "darwin",
        "goarch": "arm64",
        "binary": "forge",
        "extension": "tar.gz",
        "magic": b"\xcf\xfa\xed\xfe",
    },
    "windows-x86_64": {
        "goos": "windows",
        "goarch": "amd64",
        "binary": "forge.exe",
        "extension": "zip",
        "magic": b"MZ",
    },
}

CANDIDATE_KEYS = {
    "schema_version", "status", "repository", "source_sha", "version", "tag",
    "target", "goos", "goarch", "binary", "archive",
    "approved_manifest_sha256", "workflow_identity", "workflow_run_id",
    "binary_sha256", "archive_sha256", "checksums_sha256",
    "provenance_sha256", "smoke_summary_sha256", "inventory",
}
PROVENANCE_KEYS = {
    "schema_version", "repository", "source_sha", "version", "tag", "target",
    "goos", "goarch", "approved_manifest_sha256", "workflow_identity",
    "workflow_run_id", "provider_credentials_used", "publication_attempted",
}
SMOKE_KEYS = {
    "schema_version", "status", "source_sha", "version", "target", "help",
    "version_identity", "provider_free",
}
PLAN_KEYS = {
    "schema_version", "status", "repository", "source_sha", "version", "tag",
    "approved_manifest_sha256", "workflow_identity", "workflow_run_id",
    "publication_authorized", "candidates",
}
PLAN_CANDIDATE_KEYS = {
    "target", "goos", "goarch", "binary", "archive", "inventory",
    "binary_sha256", "archive_sha256", "checksums_sha256",
    "candidate_summary_sha256", "provenance_sha256", "smoke_summary_sha256",
}


class VerificationError(Exception):
    pass


def fail(message):
    raise VerificationError(message)


def sha256(path):
    return hashlib.sha256(path.read_bytes()).hexdigest()


def canonical_bytes(value):
    return (json.dumps(value, sort_keys=True, separators=(",", ":")) + "\n").encode()


def load_json(path, label):
    try:
        value = json.loads(path.read_text(encoding="utf-8"))
    except (OSError, UnicodeError, json.JSONDecodeError) as error:
        fail(f"malformed JSON for {label}: {error}")
    if not isinstance(value, dict):
        fail(f"malformed JSON object for {label}")
    return value


def exact_keys(value, expected, label):
    actual = set(value)
    if actual != expected:
        fail(f"{label} fields mismatch: missing={sorted(expected - actual)} unknown={sorted(actual - expected)}")


def expect_equal(value, field, expected, label):
    if value.get(field) != expected:
        fail(f"{label} {field} mismatch")


def validate_args(args):
    if not VERSION_RE.fullmatch(args.version):
        fail("version is malformed")
    if args.tag != "v" + args.version:
        fail("tag must equal v plus version")
    if not SHA_RE.fullmatch(args.source_sha):
        fail("source_sha is malformed")
    if not DIGEST_RE.fullmatch(args.manifest_digest):
        fail("manifest digest is malformed")
    if args.workflow_identity != f".github/workflows/release-rehearsal.yml@{args.source_sha}":
        fail("workflow identity mismatch")
    if not RUN_RE.fullmatch(args.workflow_run_id):
        fail("workflow run id is malformed")


def parse_checksums(path):
    result = {}
    raw = path.read_text(encoding="ascii")
    for line in raw.splitlines():
        match = re.fullmatch(r"([0-9a-f]{64})  ([A-Za-z0-9][A-Za-z0-9._-]*)", line)
        if not match or match.group(2) in result:
            fail("SHA256SUMS is malformed or non-portable")
        result[match.group(2)] = match.group(1)
    canonical = "".join(f"{result[name]}  {name}\n" for name in sorted(result))
    if raw != canonical:
        fail("SHA256SUMS is not canonically sorted")
    return result


def unsafe_archive_member_name(name):
    posix_path = pathlib.PurePosixPath(name)
    windows_path = pathlib.PureWindowsPath(name)
    return (
        not name
        or "\\" in name
        or posix_path.is_absolute()
        or posix_path.name != name
        or any(part in (".", "..") for part in posix_path.parts)
        or bool(windows_path.drive)
    )


def archive_payload(path):
    payload = {}
    if path.name.endswith(".zip"):
        try:
            with zipfile.ZipFile(path) as archive:
                for info in archive.infolist():
                    if info.is_dir() or unsafe_archive_member_name(info.filename):
                        fail("archive inventory contains a directory or unsafe path")
                    unix_mode = info.external_attr >> 16
                    if info.create_system != 3 or stat.S_IFMT(unix_mode) != stat.S_IFREG:
                        fail(f"ZIP member is not an explicit regular file: {info.filename}")
                    if info.filename in payload:
                        fail("archive inventory contains duplicates")
                    payload[info.filename] = archive.read(info)
        except (OSError, zipfile.BadZipFile) as error:
            fail(f"archive is malformed: {error}")
    else:
        try:
            with tarfile.open(path, "r:gz") as archive:
                for member in archive.getmembers():
                    if not member.isfile() or unsafe_archive_member_name(member.name):
                        fail("archive inventory contains a directory or unsafe path")
                    if member.name in payload:
                        fail("archive inventory contains duplicates")
                    stream = archive.extractfile(member)
                    if stream is None:
                        fail("archive member is unreadable")
                    payload[member.name] = stream.read()
        except (OSError, tarfile.TarError) as error:
            fail(f"archive is malformed: {error}")
    return payload


def validate_smoke(smoke, args, target, binary):
    exact_keys(smoke, SMOKE_KEYS, "smoke summary")
    for field, expected in {
        "schema_version": SMOKE_SCHEMA,
        "status": "passed",
        "source_sha": args.source_sha,
        "version": args.version,
        "target": target,
    }.items():
        expect_equal(smoke, field, expected, "smoke summary")
    expected_blocks = {
        "help": {
            "command": f"{binary} --help",
            "status": "passed",
        },
        "version_identity": {
            "command": f"{binary} --version",
            "output": f"ao-forge version={args.version} source_sha={args.source_sha}",
            "status": "passed",
        },
        "provider_free": {
            "command": f"{binary} contract validate",
            "provider_environment_present": False,
            "status": "passed",
        },
    }
    for field, expected in expected_blocks.items():
        if smoke.get(field) != expected:
            fail(f"smoke summary {field} mismatch")


def validate_provenance(provenance, args, target, spec):
    exact_keys(provenance, PROVENANCE_KEYS, "provenance")
    expected = {
        "schema_version": PROVENANCE_SCHEMA,
        "repository": "ao-forge",
        "source_sha": args.source_sha,
        "version": args.version,
        "tag": args.tag,
        "target": target,
        "goos": spec["goos"],
        "goarch": spec["goarch"],
        "approved_manifest_sha256": args.manifest_digest,
        "workflow_identity": args.workflow_identity,
        "workflow_run_id": args.workflow_run_id,
        "provider_credentials_used": False,
        "publication_attempted": False,
    }
    for field, value in expected.items():
        expect_equal(provenance, field, value, "provenance")


def validate_candidate(summary_path, args):
    candidate_dir = summary_path.parent
    summary = load_json(summary_path, "candidate summary")
    exact_keys(summary, CANDIDATE_KEYS, "candidate summary")
    target = summary.get("target")
    if target not in TARGETS:
        fail("candidate target mismatch")
    spec = TARGETS[target]
    binary = spec["binary"]
    archive = f"ao-forge-{args.version}-{target}.{spec['extension']}"
    expected = {
        "status": "passed",
        "repository": "ao-forge",
        "source_sha": args.source_sha,
        "version": args.version,
        "tag": args.tag,
        "target": target,
        "goos": spec["goos"],
        "goarch": spec["goarch"],
        "binary": binary,
        "archive": archive,
        "approved_manifest_sha256": args.manifest_digest,
        "workflow_identity": args.workflow_identity,
        "workflow_run_id": args.workflow_run_id,
    }
    if summary.get("schema_version") != CANDIDATE_SCHEMA:
        fail("candidate schema mismatch")
    for field, value in expected.items():
        expect_equal(summary, field, value, "candidate")

    expected_inventory = sorted([
        archive, binary, "LICENSE", "NOTICE", "SHA256SUMS",
        "candidate-summary.json", "provenance.json", "smoke-summary.json",
    ])
    if summary.get("inventory") != expected_inventory:
        fail("candidate declared inventory mismatch")
    entries = list(candidate_dir.iterdir())
    if any(not path.is_file() or path.is_symlink() for path in entries):
        fail("candidate inventory contains a directory or symbolic link")
    actual_inventory = sorted(path.name for path in entries)
    if actual_inventory != expected_inventory:
        fail("candidate inventory mismatch")

    paths = {name: candidate_dir / name for name in expected_inventory}
    if not paths[binary].read_bytes().startswith(spec["magic"]):
        fail("binary target format mismatch")
    for field, name in {
        "binary_sha256": binary,
        "archive_sha256": archive,
        "checksums_sha256": "SHA256SUMS",
        "provenance_sha256": "provenance.json",
        "smoke_summary_sha256": "smoke-summary.json",
    }.items():
        if summary.get(field) != sha256(paths[name]):
            fail(f"{field} mismatch")

    expected_checksummed = {archive, binary, "LICENSE", "NOTICE", "provenance.json", "smoke-summary.json"}
    checksums = parse_checksums(paths["SHA256SUMS"])
    if set(checksums) != expected_checksummed:
        fail("SHA256SUMS inventory mismatch")
    for name, expected_digest in checksums.items():
        if sha256(paths[name]) != expected_digest:
            fail(f"SHA256SUMS digest mismatch for {name}")

    archived = archive_payload(paths[archive])
    if set(archived) != {binary, "LICENSE", "NOTICE"}:
        fail("archive inventory mismatch")
    for name in archived:
        if archived[name] != paths[name].read_bytes():
            fail(f"archive payload mismatch for {name}")

    provenance = load_json(paths["provenance.json"], "provenance")
    validate_provenance(provenance, args, target, spec)
    smoke = load_json(paths["smoke-summary.json"], "smoke summary")
    validate_smoke(smoke, args, target, binary)

    return {
        "target": target,
        "goos": spec["goos"],
        "goarch": spec["goarch"],
        "binary": binary,
        "archive": archive,
        "inventory": expected_inventory,
        "binary_sha256": summary["binary_sha256"],
        "archive_sha256": summary["archive_sha256"],
        "checksums_sha256": summary["checksums_sha256"],
        "candidate_summary_sha256": sha256(summary_path),
        "provenance_sha256": summary["provenance_sha256"],
        "smoke_summary_sha256": summary["smoke_summary_sha256"],
    }


def assemble(args):
    summaries = sorted(pathlib.Path(args.candidates_dir).rglob("candidate-summary.json"))
    if len(summaries) != len(TARGETS):
        fail(f"expected exactly three candidate summaries, found {len(summaries)}")
    candidates = [validate_candidate(path, args) for path in summaries]
    if {candidate["target"] for candidate in candidates} != set(TARGETS):
        fail("candidate target set mismatch")
    candidates.sort(key=lambda item: item["target"])
    plan = {
        "schema_version": PLAN_SCHEMA,
        "status": "ready",
        "repository": "ao-forge",
        "source_sha": args.source_sha,
        "version": args.version,
        "tag": args.tag,
        "approved_manifest_sha256": args.manifest_digest,
        "workflow_identity": args.workflow_identity,
        "workflow_run_id": args.workflow_run_id,
        "publication_authorized": False,
        "candidates": candidates,
    }
    output = pathlib.Path(args.output)
    output.parent.mkdir(parents=True, exist_ok=True)
    output.write_bytes(canonical_bytes(plan))
    pathlib.Path(str(output) + ".sha256").write_text(f"{sha256(output)}  {output.name}\n", encoding="ascii")


def verify_plan(args):
    plan_path = pathlib.Path(args.plan)
    sidecar = pathlib.Path(str(plan_path) + ".sha256")
    try:
        match = re.fullmatch(r"([0-9a-f]{64})  ([A-Za-z0-9][A-Za-z0-9._-]*)\n", sidecar.read_text(encoding="ascii"))
    except OSError as error:
        fail(f"plan digest sidecar missing: {error}")
    if not match or match.group(2) != plan_path.name or match.group(1) != sha256(plan_path):
        fail("plan digest mismatch")
    plan = load_json(plan_path, "promotion plan")
    if plan_path.read_bytes() != canonical_bytes(plan):
        fail("promotion plan is not canonical JSON")
    exact_keys(plan, PLAN_KEYS, "promotion plan")
    expected = {
        "schema_version": PLAN_SCHEMA,
        "status": "ready",
        "repository": "ao-forge",
        "source_sha": args.source_sha,
        "version": args.version,
        "tag": args.tag,
        "approved_manifest_sha256": args.manifest_digest,
        "workflow_identity": args.workflow_identity,
        "workflow_run_id": args.workflow_run_id,
        "publication_authorized": False,
    }
    for field, value in expected.items():
        expect_equal(plan, field, value, "promotion plan")
    candidates = plan.get("candidates")
    if not isinstance(candidates, list) or [item.get("target") for item in candidates] != sorted(TARGETS):
        fail("promotion plan candidate target set mismatch")
    for item in candidates:
        if not isinstance(item, dict):
            fail("promotion plan candidate is malformed")
        exact_keys(item, PLAN_CANDIDATE_KEYS, "promotion plan candidate")
        spec = TARGETS[item["target"]]
        if item["goos"] != spec["goos"] or item["goarch"] != spec["goarch"] or item["binary"] != spec["binary"]:
            fail("promotion plan candidate target identity mismatch")
        expected_archive = f"ao-forge-{args.version}-{item['target']}.{spec['extension']}"
        if item["archive"] != expected_archive:
            fail("promotion plan archive mismatch")
        expected_inventory = sorted([
            expected_archive, spec["binary"], "LICENSE", "NOTICE", "SHA256SUMS",
            "candidate-summary.json", "provenance.json", "smoke-summary.json",
        ])
        if item["inventory"] != expected_inventory:
            fail("promotion plan inventory mismatch")
        for field in (
            "binary_sha256", "archive_sha256", "checksums_sha256",
            "candidate_summary_sha256", "provenance_sha256", "smoke_summary_sha256",
        ):
            if not DIGEST_RE.fullmatch(item.get(field, "")):
                fail(f"promotion plan {field} is malformed")


def parser():
    result = argparse.ArgumentParser()
    result.add_argument("mode", choices=("assemble", "verify-plan"))
    result.add_argument("--version", required=True)
    result.add_argument("--tag", required=True)
    result.add_argument("--source-sha", required=True)
    result.add_argument("--manifest-digest", required=True)
    result.add_argument("--workflow-identity", required=True)
    result.add_argument("--workflow-run-id", required=True)
    result.add_argument("--candidates-dir")
    result.add_argument("--output")
    result.add_argument("--plan")
    return result


def main():
    args = parser().parse_args()
    try:
        validate_args(args)
        if args.mode == "assemble":
            if not args.candidates_dir or not args.output:
                fail("assemble requires candidates-dir and output")
            assemble(args)
        else:
            if not args.plan:
                fail("verify-plan requires plan")
            verify_plan(args)
    except (OSError, UnicodeError, KeyError, TypeError, VerificationError) as error:
        print(f"release rehearsal verification failed: {error}", file=sys.stderr)
        return 1
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
