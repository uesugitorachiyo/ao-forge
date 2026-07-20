#!/usr/bin/env python3

import argparse
import gzip
import hashlib
import io
import json
import os
import pathlib
import shutil
import stat
import subprocess
import sys
import tarfile
import zipfile

TARGETS = {
    "linux-x86_64": ("linux", "amd64", "forge", "tar.gz", b"\x7fELF"),
    "macos-aarch64": ("darwin", "arm64", "forge", "tar.gz", b"\xcf\xfa\xed\xfe"),
    "windows-x86_64": ("windows", "amd64", "forge.exe", "zip", b"MZ"),
}


def sha256(path):
    return hashlib.sha256(path.read_bytes()).hexdigest()


def write_json(path, value):
    path.write_text(json.dumps(value, sort_keys=True, separators=(",", ":")) + "\n", encoding="utf-8")


def run_candidate(command, root):
    allowed = {"HOME", "PATH", "PATHEXT", "SYSTEMROOT", "TEMP", "TMP", "TMPDIR", "WINDIR"}
    environment = {key: value for key, value in os.environ.items() if key in allowed}
    result = subprocess.run(command, cwd=root, env=environment, text=True, capture_output=True)
    return result, not any(
        key in environment
        for key in ("ANTHROPIC_API_KEY", "AO_PROVIDER_TOKEN", "GH_TOKEN", "GITHUB_TOKEN", "OPENAI_API_KEY")
    )


def inspect_go_binary(path):
    result = subprocess.run(["go", "version", "-m", str(path)], text=True, capture_output=True)
    if result.returncode != 0:
        raise ValueError(f"go version -m failed: {result.stderr.strip()}")
    values = {}
    for line in result.stdout.splitlines():
        fields = line.strip().split()
        if len(fields) == 2 and fields[0] == "build" and "=" in fields[1]:
            key, value = fields[1].split("=", 1)
            values[key] = value
    return values


def make_archive(path, archive_format, files):
    if archive_format == "zip":
        with zipfile.ZipFile(path, "w", compression=zipfile.ZIP_DEFLATED) as archive:
            for name in sorted(files):
                info = zipfile.ZipInfo(name, (1980, 1, 1, 0, 0, 0))
                info.create_system = 3
                mode = 0o755 if name.startswith("forge") else 0o644
                info.external_attr = (stat.S_IFREG | mode) << 16
                archive.writestr(info, files[name])
        return
    with path.open("wb") as raw:
        with gzip.GzipFile(fileobj=raw, mode="wb", filename="", mtime=0) as compressed:
            with tarfile.open(fileobj=compressed, mode="w") as archive:
                for name in sorted(files):
                    info = tarfile.TarInfo(name)
                    info.size = len(files[name])
                    info.mtime = 0
                    info.uid = info.gid = 0
                    info.uname = info.gname = ""
                    info.mode = 0o755 if name.startswith("forge") else 0o644
                    archive.addfile(info, io.BytesIO(files[name]))


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("--binary-path", required=True)
    parser.add_argument("--output", required=True)
    parser.add_argument("--version", required=True)
    parser.add_argument("--tag", required=True)
    parser.add_argument("--source-sha", required=True)
    parser.add_argument("--target", required=True, choices=TARGETS)
    parser.add_argument("--manifest-digest", required=True)
    parser.add_argument("--workflow-identity", required=True)
    parser.add_argument("--workflow-run-id", required=True)
    args = parser.parse_args()

    root = pathlib.Path(__file__).resolve().parents[1]
    source_binary = pathlib.Path(args.binary_path).resolve()
    output = pathlib.Path(args.output).resolve()
    output.mkdir(parents=True, exist_ok=False)
    goos, goarch, binary, archive_format, magic = TARGETS[args.target]
    if not source_binary.read_bytes().startswith(magic):
        raise SystemExit("built binary format does not match declared target")
    metadata = inspect_go_binary(source_binary)
    if metadata.get("GOOS") != goos or metadata.get("GOARCH") != goarch:
        raise SystemExit("built binary GOOS/GOARCH does not match declared target")

    candidate_binary = output / binary
    shutil.copyfile(source_binary, candidate_binary)
    candidate_binary.chmod(0o755)
    shutil.copyfile(root / "LICENSE", output / "LICENSE")
    shutil.copyfile(root / "NOTICE", output / "NOTICE")

    help_result, help_clean = run_candidate([str(candidate_binary), "--help"], root)
    if help_result.returncode != 0 or not help_result.stdout.startswith("AO Forge"):
        raise SystemExit("candidate help smoke failed")
    version_result, version_clean = run_candidate([str(candidate_binary), "--version"], root)
    expected_identity = f"ao-forge version={args.version} source_sha={args.source_sha}"
    if version_result.returncode != 0 or version_result.stdout.strip() != expected_identity:
        raise SystemExit("candidate version identity smoke failed")
    provider_result, provider_clean = run_candidate(
        [
            str(candidate_binary),
            "contract",
            "validate",
            "--schema",
            "docs/contracts/release-candidate-v0.1.schema.json",
            "--document",
            "examples/release-preview/release-candidate.v0.1.example.json",
            "--json",
        ],
        root,
    )
    try:
        provider_readback = json.loads(provider_result.stdout)
    except json.JSONDecodeError as error:
        raise SystemExit(f"candidate provider-free smoke emitted malformed JSON: {error}")
    if provider_result.returncode != 0 or provider_readback.get("status") != "passed":
        raise SystemExit("candidate provider-free functional smoke failed")
    if not (help_clean and version_clean and provider_clean):
        raise SystemExit("candidate smoke environment contains provider credentials")

    smoke = {
        "schema_version": "ao.forge.release-rehearsal-smoke.v1",
        "status": "passed",
        "source_sha": args.source_sha,
        "version": args.version,
        "target": args.target,
        "help": {"command": f"{binary} --help", "status": "passed"},
        "version_identity": {
            "command": f"{binary} --version",
            "output": expected_identity,
            "status": "passed",
        },
        "provider_free": {
            "command": f"{binary} contract validate",
            "provider_environment_present": False,
            "status": "passed",
        },
    }
    provenance = {
        "schema_version": "ao.forge.release-rehearsal-provenance.v1",
        "repository": "ao-forge",
        "source_sha": args.source_sha,
        "version": args.version,
        "tag": args.tag,
        "target": args.target,
        "goos": goos,
        "goarch": goarch,
        "approved_manifest_sha256": args.manifest_digest,
        "workflow_identity": args.workflow_identity,
        "workflow_run_id": args.workflow_run_id,
        "provider_credentials_used": False,
        "publication_attempted": False,
    }
    write_json(output / "smoke-summary.json", smoke)
    write_json(output / "provenance.json", provenance)

    payload = {name: (output / name).read_bytes() for name in (binary, "LICENSE", "NOTICE")}
    extension = "zip" if archive_format == "zip" else "tar.gz"
    archive = f"ao-forge-{args.version}-{args.target}.{extension}"
    make_archive(output / archive, archive_format, payload)

    checksummed = sorted([archive, binary, "LICENSE", "NOTICE", "provenance.json", "smoke-summary.json"])
    manifest = "".join(f"{sha256(output / name)}  {name}\n" for name in checksummed)
    (output / "SHA256SUMS").write_text(manifest, encoding="ascii")
    inventory = sorted(
        [archive, binary, "LICENSE", "NOTICE", "SHA256SUMS", "candidate-summary.json", "provenance.json", "smoke-summary.json"]
    )
    summary = {
        "schema_version": "ao.forge.release-rehearsal-candidate.v1",
        "status": "passed",
        "repository": "ao-forge",
        "source_sha": args.source_sha,
        "version": args.version,
        "tag": args.tag,
        "target": args.target,
        "goos": goos,
        "goarch": goarch,
        "binary": binary,
        "archive": archive,
        "approved_manifest_sha256": args.manifest_digest,
        "workflow_identity": args.workflow_identity,
        "workflow_run_id": args.workflow_run_id,
        "binary_sha256": sha256(output / binary),
        "archive_sha256": sha256(output / archive),
        "checksums_sha256": sha256(output / "SHA256SUMS"),
        "provenance_sha256": sha256(output / "provenance.json"),
        "smoke_summary_sha256": sha256(output / "smoke-summary.json"),
        "inventory": inventory,
    }
    write_json(output / "candidate-summary.json", summary)


if __name__ == "__main__":
    try:
        main()
    except (OSError, ValueError, subprocess.SubprocessError) as error:
        print(f"candidate build failed: {error}", file=sys.stderr)
        raise SystemExit(1)
