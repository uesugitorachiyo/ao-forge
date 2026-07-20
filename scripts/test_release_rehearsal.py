#!/usr/bin/env python3

import copy
import hashlib
import io
import importlib.util
import json
import os
import pathlib
import shutil
import stat
import subprocess
import sys
import tarfile
import tempfile
import unittest
import zipfile

ROOT = pathlib.Path(__file__).resolve().parents[1]
VERIFY = ROOT / "scripts" / "verify-release-rehearsal.py"
DISCOVER = ROOT / "scripts" / "discover-release-candidate-version.py"
VALIDATE_MANIFEST = ROOT / "scripts" / "validate-release-rehearsal-manifest.py"
BUILD = ROOT / "scripts" / "build-release-rehearsal-candidate.py"
VERSION = "0.1.4"
TAG = "v0.1.4"
SOURCE = "1" * 40
MANIFEST = "2" * 64
WORKFLOW = f".github/workflows/release-rehearsal.yml@{SOURCE}"
RUN_ID = "123456789"

TARGETS = {
    "linux-x86_64": ("linux", "amd64", "forge", "tar.gz", b"\x7fELF"),
    "macos-aarch64": ("darwin", "arm64", "forge", "tar.gz", b"\xcf\xfa\xed\xfe"),
    "windows-x86_64": ("windows", "amd64", "forge.exe", "zip", b"MZ"),
}


def digest(data):
    return hashlib.sha256(data).hexdigest()


def canonical(value):
    return (json.dumps(value, sort_keys=True, separators=(",", ":")) + "\n").encode()


def write_json(path, value):
    path.write_bytes(canonical(value))


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
        import gzip

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


def make_candidate(root, target):
    goos, goarch, binary, archive_format, magic = TARGETS[target]
    candidate = root / target
    candidate.mkdir()
    payload = {
        binary: magic + f" ao-forge {target}\n".encode(),
        "LICENSE": b"test license\n",
        "NOTICE": b"test notice\n",
    }
    for name, data in payload.items():
        (candidate / name).write_bytes(data)

    archive_name = f"ao-forge-{VERSION}-{target}.{'zip' if archive_format == 'zip' else 'tar.gz'}"
    make_archive(candidate / archive_name, archive_format, payload)

    smoke = {
        "schema_version": "ao.forge.release-rehearsal-smoke.v1",
        "status": "passed",
        "source_sha": SOURCE,
        "version": VERSION,
        "target": target,
        "help": {"command": f"{binary} --help", "status": "passed"},
        "version_identity": {
            "command": f"{binary} --version",
            "output": f"ao-forge version={VERSION} source_sha={SOURCE}",
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
        "source_sha": SOURCE,
        "version": VERSION,
        "tag": TAG,
        "target": target,
        "goos": goos,
        "goarch": goarch,
        "approved_manifest_sha256": MANIFEST,
        "workflow_identity": WORKFLOW,
        "workflow_run_id": RUN_ID,
        "provider_credentials_used": False,
        "publication_attempted": False,
    }
    write_json(candidate / "smoke-summary.json", smoke)
    write_json(candidate / "provenance.json", provenance)

    checksummed = [archive_name, binary, "LICENSE", "NOTICE", "provenance.json", "smoke-summary.json"]
    lines = [f"{digest((candidate / name).read_bytes())}  {name}" for name in sorted(checksummed)]
    (candidate / "SHA256SUMS").write_text("\n".join(lines) + "\n", encoding="ascii")

    summary = {
        "schema_version": "ao.forge.release-rehearsal-candidate.v1",
        "status": "passed",
        "repository": "ao-forge",
        "source_sha": SOURCE,
        "version": VERSION,
        "tag": TAG,
        "target": target,
        "goos": goos,
        "goarch": goarch,
        "binary": binary,
        "archive": archive_name,
        "approved_manifest_sha256": MANIFEST,
        "workflow_identity": WORKFLOW,
        "workflow_run_id": RUN_ID,
        "binary_sha256": digest((candidate / binary).read_bytes()),
        "archive_sha256": digest((candidate / archive_name).read_bytes()),
        "checksums_sha256": digest((candidate / "SHA256SUMS").read_bytes()),
        "provenance_sha256": digest((candidate / "provenance.json").read_bytes()),
        "smoke_summary_sha256": digest((candidate / "smoke-summary.json").read_bytes()),
        "inventory": sorted(
            [archive_name, binary, "LICENSE", "NOTICE", "SHA256SUMS", "candidate-summary.json", "provenance.json", "smoke-summary.json"]
        ),
    }
    write_json(candidate / "candidate-summary.json", summary)


class ReleaseRehearsalTest(unittest.TestCase):
    def setUp(self):
        self.temp = pathlib.Path(tempfile.mkdtemp())
        self.candidates = self.temp / "candidates"
        self.candidates.mkdir()
        for target in TARGETS:
            make_candidate(self.candidates, target)
        self.plan = self.temp / "immutable-promotion-plan.json"

    def tearDown(self):
        shutil.rmtree(self.temp)

    def command(self, mode="assemble", **overrides):
        args = [
            sys.executable,
            str(VERIFY),
            mode,
            "--version",
            overrides.get("version", VERSION),
            "--tag",
            overrides.get("tag", TAG),
            "--source-sha",
            overrides.get("source", SOURCE),
            "--manifest-digest",
            overrides.get("manifest", MANIFEST),
            "--workflow-identity",
            overrides.get("workflow", WORKFLOW),
            "--workflow-run-id",
            overrides.get("run_id", RUN_ID),
        ]
        if mode == "assemble":
            args += ["--candidates-dir", str(self.candidates), "--output", str(self.plan)]
        else:
            args += ["--plan", str(self.plan)]
        return subprocess.run(args, text=True, capture_output=True)

    def assert_rejected(self, mutate, expected):
        mutate()
        result = self.command()
        self.assertNotEqual(result.returncode, 0, result.stdout)
        self.assertIn(expected, result.stderr)

    def edit_summary(self, target, edit):
        path = self.candidates / target / "candidate-summary.json"
        value = json.loads(path.read_text())
        edit(value)
        write_json(path, value)

    def retype_windows_archive_member(self, file_type):
        candidate = self.candidates / "windows-x86_64"
        archive_path = candidate / f"ao-forge-{VERSION}-windows-x86_64.zip"
        with zipfile.ZipFile(archive_path) as archive:
            entries = [(info.filename, archive.read(info), info.external_attr) for info in archive.infolist()]
        with zipfile.ZipFile(archive_path, "w", compression=zipfile.ZIP_DEFLATED) as archive:
            for name, data, external_attr in entries:
                info = zipfile.ZipInfo(name, (1980, 1, 1, 0, 0, 0))
                info.create_system = 3
                permissions = (external_attr >> 16) & 0o777
                entry_type = file_type if name == "forge.exe" else stat.S_IFREG
                info.external_attr = (entry_type | permissions) << 16
                archive.writestr(info, data)

        self.rebind_windows_archive_digests(candidate, archive_path)

    def rewrite_windows_archive_path(self, replacement, directory=False):
        candidate = self.candidates / "windows-x86_64"
        archive_path = candidate / f"ao-forge-{VERSION}-windows-x86_64.zip"
        with zipfile.ZipFile(archive_path) as archive:
            entries = [(info.filename, archive.read(info), info.external_attr) for info in archive.infolist()]
        with zipfile.ZipFile(archive_path, "w", compression=zipfile.ZIP_DEFLATED) as archive:
            for name, data, external_attr in entries:
                output_name = replacement if name == "forge.exe" and not directory else name
                info = zipfile.ZipInfo(output_name, (1980, 1, 1, 0, 0, 0))
                info.create_system = 3
                info.external_attr = external_attr
                archive.writestr(info, data)
            if directory:
                info = zipfile.ZipInfo(replacement, (1980, 1, 1, 0, 0, 0))
                info.create_system = 3
                info.external_attr = (stat.S_IFDIR | 0o755) << 16
                archive.writestr(info, b"")

        self.rebind_windows_archive_digests(candidate, archive_path)

    def rebind_windows_archive_digests(self, candidate, archive_path):
        checksums_path = candidate / "SHA256SUMS"
        checksums = {}
        for line in checksums_path.read_text(encoding="ascii").splitlines():
            value, name = line.split("  ", 1)
            checksums[name] = value
        checksums[archive_path.name] = digest(archive_path.read_bytes())
        checksums_path.write_text(
            "".join(f"{checksums[name]}  {name}\n" for name in sorted(checksums)),
            encoding="ascii",
        )
        self.edit_summary(
            "windows-x86_64",
            lambda value: value.update(
                archive_sha256=digest(archive_path.read_bytes()),
                checksums_sha256=digest(checksums_path.read_bytes()),
            ),
        )

    def test_builder_zip_members_are_explicit_regular_files(self):
        spec = importlib.util.spec_from_file_location("release_candidate_builder", BUILD)
        module = importlib.util.module_from_spec(spec)
        spec.loader.exec_module(module)
        archive_path = self.temp / "builder.zip"
        module.make_archive(archive_path, "zip", {"forge.exe": b"MZ", "LICENSE": b"license"})
        with zipfile.ZipFile(archive_path) as archive:
            for info in archive.infolist():
                self.assertEqual(info.create_system, 3)
                self.assertEqual(stat.S_IFMT(info.external_attr >> 16), stat.S_IFREG)

    def test_rejects_non_regular_windows_zip_member_types_after_digest_rebinding(self):
        unsafe_types = {
            "symlink": stat.S_IFLNK,
            "character_device": stat.S_IFCHR,
            "block_device": stat.S_IFBLK,
            "fifo": stat.S_IFIFO,
            "socket": stat.S_IFSOCK,
        }
        for label, file_type in unsafe_types.items():
            with self.subTest(label=label):
                shutil.rmtree(self.candidates)
                self.candidates.mkdir()
                for target in TARGETS:
                    make_candidate(self.candidates, target)
                self.retype_windows_archive_member(file_type)
                result = self.command()
                self.assertNotEqual(result.returncode, 0, result.stdout)
                self.assertIn("ZIP member is not an explicit regular file", result.stderr)

    def test_rejects_directory_and_unsafe_windows_zip_paths_after_digest_rebinding(self):
        unsafe_paths = {
            "directory": ("nested/", True),
            "nested": ("nested/forge.exe", False),
            "traversal": ("../forge.exe", False),
            "posix_absolute": ("/forge.exe", False),
            "windows_drive_absolute": (r"C:\forge.exe", False),
            "windows_unc_absolute": (r"\\server\share\forge.exe", False),
        }
        for label, (path, directory) in unsafe_paths.items():
            with self.subTest(label=label):
                shutil.rmtree(self.candidates)
                self.candidates.mkdir()
                for target in TARGETS:
                    make_candidate(self.candidates, target)
                self.rewrite_windows_archive_path(path, directory=directory)
                result = self.command()
                self.assertNotEqual(result.returncode, 0, result.stdout)
                self.assertIn("archive inventory contains a directory or unsafe path", result.stderr)

    def test_discovers_next_repository_patch_version(self):
        result = subprocess.run([sys.executable, str(DISCOVER)], cwd=ROOT, text=True, capture_output=True)
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual(result.stdout, VERSION + "\n")

    def manifest_command(self, manifest, digest_override=None, encoded_override=None):
        raw = canonical(manifest)
        import base64

        environment = os.environ.copy()
        environment.update(
            {
                "APPROVED_MANIFEST_BASE64": encoded_override or base64.b64encode(raw).decode("ascii"),
                "APPROVED_MANIFEST_DIGEST": digest_override or digest(raw),
                "SOURCE_COMMIT": SOURCE,
                "TAG": TAG,
                "VERSION": VERSION,
            }
        )
        return subprocess.run(
            [sys.executable, str(VALIDATE_MANIFEST), "--output", str(self.temp / "manifest.json")],
            env=environment,
            text=True,
            capture_output=True,
        )

    def valid_manifest(self):
        return {
            "schema_version": "ao.forge.approved-release-manifest.v1",
            "source_sha": SOURCE,
            "version": VERSION,
            "tag": TAG,
            "targets": ["linux-x86_64", "macos-aarch64", "windows-x86_64"],
            "publication_authorized": False,
        }

    def test_manifest_accepts_exact_bounded_contract(self):
        result = self.manifest_command(self.valid_manifest())
        self.assertEqual(result.returncode, 0, result.stderr)

    def test_manifest_rejects_altered_digest(self):
        result = self.manifest_command(self.valid_manifest(), digest_override="9" * 64)
        self.assertNotEqual(result.returncode, 0)
        self.assertIn("digest mismatch", result.stderr)

    def test_manifest_rejects_wrong_identity_and_malformed_base64(self):
        wrong = self.valid_manifest()
        wrong["source_sha"] = "8" * 40
        result = self.manifest_command(wrong)
        self.assertNotEqual(result.returncode, 0)
        self.assertIn("identity or authority mismatch", result.stderr)
        malformed = self.manifest_command(self.valid_manifest(), encoded_override="%%%")
        self.assertNotEqual(malformed.returncode, 0)
        self.assertIn("base64 is malformed", malformed.stderr)

    def test_manifest_rejects_oversized_input(self):
        result = self.manifest_command(self.valid_manifest(), encoded_override="A" * 65_537)
        self.assertNotEqual(result.returncode, 0)
        self.assertIn("encoded size limit", result.stderr)

    def test_accepts_exact_three_target_evidence_and_verifies_plan(self):
        assembled = self.command()
        self.assertEqual(assembled.returncode, 0, assembled.stderr)
        plan = json.loads(self.plan.read_text())
        self.assertEqual(plan["schema_version"], "ao.forge.immutable-promotion-plan.v1")
        self.assertEqual([item["target"] for item in plan["candidates"]], sorted(TARGETS))
        self.assertFalse(plan["publication_authorized"])
        verified = self.command("verify-plan")
        self.assertEqual(verified.returncode, 0, verified.stderr)

    def test_rejects_wrong_source(self):
        self.assert_rejected(
            lambda: self.edit_summary("linux-x86_64", lambda value: value.update(source_sha="3" * 40)),
            "source_sha mismatch",
        )

    def test_rejects_wrong_version(self):
        self.assert_rejected(
            lambda: self.edit_summary("linux-x86_64", lambda value: value.update(version="9.9.9")),
            "version mismatch",
        )

    def test_rejects_target_or_binary_substitution(self):
        self.assert_rejected(
            lambda: self.edit_summary("linux-x86_64", lambda value: value.update(binary="other")),
            "binary mismatch",
        )

    def test_rejects_archive_digest_drift(self):
        archive = next((self.candidates / "linux-x86_64").glob("*.tar.gz"))
        self.assert_rejected(lambda: archive.write_bytes(archive.read_bytes() + b"x"), "archive_sha256 mismatch")

    def test_rejects_evidence_digest_drift(self):
        smoke = self.candidates / "linux-x86_64" / "smoke-summary.json"
        self.assert_rejected(lambda: smoke.write_bytes(smoke.read_bytes() + b" "), "smoke_summary_sha256 mismatch")

    def test_rejects_schema_drift(self):
        self.assert_rejected(
            lambda: self.edit_summary("linux-x86_64", lambda value: value.update(schema_version="invented")),
            "candidate schema mismatch",
        )

    def test_rejects_inventory_drift(self):
        self.assert_rejected(
            lambda: (self.candidates / "linux-x86_64" / "extra.txt").write_text("extra"),
            "candidate inventory mismatch",
        )

    def test_rejects_manifest_drift(self):
        self.assert_rejected(
            lambda: self.edit_summary(
                "linux-x86_64", lambda value: value.update(approved_manifest_sha256="4" * 64)
            ),
            "approved_manifest_sha256 mismatch",
        )

    def test_rejects_malformed_data(self):
        summary = self.candidates / "linux-x86_64" / "candidate-summary.json"
        self.assert_rejected(lambda: summary.write_text("{"), "malformed JSON")

    def test_rejects_mutated_plan_and_sidecar(self):
        self.assertEqual(self.command().returncode, 0)
        plan = json.loads(self.plan.read_text())
        plan["candidates"][0]["binary_sha256"] = "5" * 64
        write_json(self.plan, plan)
        result = self.command("verify-plan")
        self.assertNotEqual(result.returncode, 0)
        self.assertIn("plan digest mismatch", result.stderr)

    def test_rejects_semantically_mutated_plan_with_recomputed_sidecar(self):
        self.assertEqual(self.command().returncode, 0)
        plan = json.loads(self.plan.read_text())
        plan["candidates"][0]["archive"] = "substituted.tar.gz"
        write_json(self.plan, plan)
        pathlib.Path(str(self.plan) + ".sha256").write_text(
            f"{digest(self.plan.read_bytes())}  {self.plan.name}\n",
            encoding="ascii",
        )
        result = self.command("verify-plan")
        self.assertNotEqual(result.returncode, 0)
        self.assertIn("promotion plan archive mismatch", result.stderr)


if __name__ == "__main__":
    unittest.main(verbosity=2)
