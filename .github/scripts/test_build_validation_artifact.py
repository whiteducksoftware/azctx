from datetime import datetime, timezone
import hashlib
import importlib.util
import json
import os
from pathlib import Path
import shutil
import subprocess
import sys
import tarfile
import unittest
import uuid
from unittest.mock import patch


ROOT = Path(__file__).resolve().parents[2]
SCRIPT = ROOT / ".github/scripts/build_validation_artifact.py"


class ValidationArtifactTest(unittest.TestCase):
    def setUp(self):
        self.work = ROOT / ".test-work" / ("validation build-" + uuid.uuid4().hex)
        self.work.mkdir(parents=True)
        self.addCleanup(shutil.rmtree, self.work)
        self.env = dict(os.environ, AZURE_CONFIG_DIR=str(self.work / "no-azure"),
                        GITHUB_OUTPUT=str(self.work / "github-output"))
        self.revision = subprocess.check_output(
            ["git", "rev-parse", "HEAD"], cwd=ROOT, text=True,
        ).strip()
        self.env["GITHUB_SHA"] = self.revision
        commit_date = subprocess.check_output(
            ["git", "show", "-s", "--format=%cI", "HEAD"], cwd=ROOT, text=True,
        ).strip()
        self.build_date = datetime.fromisoformat(commit_date).astimezone(timezone.utc).isoformat().replace(
            "+00:00", "Z",
        )

    def builder(self):
        spec = importlib.util.spec_from_file_location("build_validation_artifact", SCRIPT)
        builder = importlib.util.module_from_spec(spec)
        spec.loader.exec_module(builder)
        return builder

    def test_native_download_has_executable_checksum_and_tested_revision(self):
        result = subprocess.run(
            [sys.executable, "-B", str(SCRIPT)], cwd=ROOT, env=self.env,
            capture_output=True, text=True,
        )
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
        target = json.loads(subprocess.check_output(
            ["go", "env", "-json", "GOHOSTOS", "GOHOSTARCH", "GOVERSION"],
            cwd=ROOT, text=True,
        ))
        name = f"azctx-validation-{target['GOHOSTOS']}-{target['GOHOSTARCH']}-{self.revision}"
        archive = ROOT / "dist/validation" / (name + ".tar.gz")
        self.addCleanup(archive.unlink)
        self.assertEqual((self.work / "github-output").read_text(encoding="utf-8"), (
            f"artifact-name={name}\narchive-path=dist/validation/{name}.tar.gz\n"
        ))
        binary_name = "azctx.exe" if target["GOHOSTOS"] == "windows" else "azctx"
        with tarfile.open(archive, "r:gz") as downloaded:
            self.assertEqual(sorted(downloaded.getnames()), sorted([
                binary_name, "metadata.json", "SHA256SUMS",
            ]))
            for member in downloaded.getmembers():
                self.assertTrue(member.isfile(), member.name)
                self.assertEqual(member.mode, 0o755 if member.name == binary_name else 0o644)
            binary_bytes = downloaded.extractfile(binary_name).read()
            metadata_bytes = downloaded.extractfile("metadata.json").read()
            checksums = downloaded.extractfile("SHA256SUMS").read().decode("ascii")
        self.assertEqual(checksums, (
            f"{hashlib.sha256(binary_bytes).hexdigest()}  {binary_name}\n"
            f"{hashlib.sha256(metadata_bytes).hexdigest()}  metadata.json\n"
        ))
        metadata = json.loads(metadata_bytes)
        self.assertEqual(metadata["kind"], "validation")
        self.assertEqual(metadata["revision"], self.revision)
        self.assertEqual(metadata["revisionSource"], "checkout HEAD")
        self.assertEqual(metadata["version"], "0.0.0-validation." + self.revision[:12])
        self.assertEqual(metadata["buildDate"], self.build_date)
        self.assertEqual(metadata["schemaVersion"], 1)
        self.assertEqual(metadata["goos"], target["GOHOSTOS"])
        self.assertEqual(metadata["goarch"], target["GOHOSTARCH"])
        self.assertEqual(metadata["binary"], binary_name)
        self.assertEqual(metadata["goVersion"], target["GOVERSION"])
        self.assertFalse(metadata["cgoEnabled"])
        self.assertTrue(metadata["trimpath"])
        binary = self.work / binary_name
        binary.write_bytes(binary_bytes)
        binary.chmod(0o755)
        version = json.loads(subprocess.check_output(
            [str(binary), "version", "-o", "json"], cwd=ROOT, env=self.env, text=True,
        ))
        self.assertEqual(version, {
            "version": metadata["version"], "commit": self.revision,
            "buildDate": metadata["buildDate"], "goVersion": target["GOVERSION"],
        })
        settings = subprocess.check_output(
            ["go", "version", "-m", str(binary)], cwd=ROOT, text=True,
        )
        self.assertIn("CGO_ENABLED=0", settings)
        self.assertIn("-trimpath=true", settings)
        self.assertFalse((self.work / "no-azure").exists())

    def fake_go(self, goos, goarch, binary_content=b"native executable fixture", wrong_version=False):
        real_run = subprocess.run

        def run(args, **kwargs):
            if args[:3] == ["go", "env", "-json"]:
                return subprocess.CompletedProcess(args, 0, json.dumps({
                    "GOHOSTOS": goos, "GOHOSTARCH": goarch, "GOVERSION": "go1.27.3",
                }))
            if args[:2] == ["go", "build"]:
                self.assertIn("-trimpath", args)
                self.assertEqual(kwargs["env"]["CGO_ENABLED"], "0")
                self.assertEqual(kwargs["env"]["GOOS"], goos)
                self.assertEqual(kwargs["env"]["GOARCH"], goarch)
                self.assertEqual(kwargs["env"]["GOFLAGS"], "")
                binary = Path(args[args.index("-o") + 1])
                self.assertEqual(binary.name, "azctx.exe" if goos == "windows" else "azctx")
                if binary_content is not None:
                    binary.write_bytes(binary_content)
                return subprocess.CompletedProcess(args, 0)
            if args[1:] == ["version", "-o", "json"]:
                return subprocess.CompletedProcess(args, 0, json.dumps({
                    "version": "0.0.0-validation." + self.revision[:12],
                    "commit": "b" * 40 if wrong_version else self.revision,
                    "buildDate": self.build_date, "goVersion": "go1.27.3",
                }))
            return real_run(args, **kwargs)

        return run

    def test_platform_archives_name_native_executable_and_never_include_worktree_files(self):
        builder = self.builder()
        (self.work / "not-an-artifact.env").write_text("synthetic fixture only\n", encoding="utf-8")
        for goos, goarch in (("linux", "amd64"), ("darwin", "arm64"), ("windows", "amd64")):
            with self.subTest(goos=goos), patch.dict(os.environ, self.env), \
                    patch("subprocess.run", side_effect=self.fake_go(goos, goarch)):
                archive = builder.build(self.work)
                self.assertEqual(archive.name, f"azctx-validation-{goos}-{goarch}-{self.revision}.tar.gz")
                executable = "azctx.exe" if goos == "windows" else "azctx"
                with tarfile.open(archive, "r:gz") as bundle:
                    self.assertEqual(bundle.getnames(), [executable, "metadata.json", "SHA256SUMS"])
                    self.assertEqual(bundle.extractfile(executable).read(), b"native executable fixture")
                    metadata = bundle.extractfile("metadata.json").read()
                    self.assertEqual(json.loads(metadata)["revision"], self.revision)
                    self.assertEqual(bundle.extractfile("SHA256SUMS").read().decode("ascii"), (
                        f"{hashlib.sha256(b'native executable fixture').hexdigest()}  {executable}\n"
                        f"{hashlib.sha256(metadata).hexdigest()}  metadata.json\n"
                    ))
                self.assertFalse((self.work / "dist/validation-stage").exists())

    def test_missing_empty_or_wrong_revision_binary_cannot_publish_stale_archive(self):
        builder = self.builder()
        archive = self.work / f"dist/validation/azctx-validation-windows-amd64-{self.revision}.tar.gz"
        archive.parent.mkdir(parents=True)
        for content, wrong_version in ((None, False), (b"", False), (b"executable", True)):
            with self.subTest(content=content, wrong_version=wrong_version), \
                    patch.dict(os.environ, self.env), \
                    patch("subprocess.run", side_effect=self.fake_go(
                        "windows", "amd64", binary_content=content, wrong_version=wrong_version,
                    )):
                archive.write_bytes(b"stale artifact")
                with self.assertRaisesRegex(RuntimeError, "expected nonempty|does not match"):
                    builder.build(self.work)
                self.assertFalse(archive.exists())
                self.assertFalse((self.work / "github-output").exists())
                self.assertFalse((self.work / "dist/validation-stage").exists())

    def test_tested_revision_mismatch_or_shell_text_is_rejected_without_outputs(self):
        builder = self.builder()
        marker = self.work / "must-not-execute"
        for revision in ("b" * 40, f"$(touch {marker})"):
            with self.subTest(revision=revision), patch.dict(os.environ, self.env | {"GITHUB_SHA": revision}):
                with self.assertRaisesRegex(ValueError, "checkout HEAD does not match GITHUB_SHA"):
                    builder.build(self.work)
                self.assertFalse((self.work / "dist").exists())
                self.assertFalse((self.work / "github-output").exists())
                self.assertFalse(marker.exists())


if __name__ == "__main__":
    unittest.main()
