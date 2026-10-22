"""Test the host/container argv boundary with an offline Docker stand-in."""

import json
import os
import shutil
import subprocess
import sys
import unittest
import uuid
from pathlib import Path


ROOT = Path(__file__).resolve().parents[2]
SHIM = ROOT / ".github/scripts/az-container.sh"
IMAGE = "mcr.microsoft.com/azure-cli@sha256:" + "a" * 64


class ContainerShimTests(unittest.TestCase):
    def setUp(self):
        self.scratch = ROOT / ".test-work" / ("docker-" + uuid.uuid4().hex)
        self.scratch.mkdir(parents=True)
        self.addCleanup(self.cleanup)
        self.record = self.scratch / "argv.json"
        docker = self.scratch / "docker"
        docker.write_text(
            "#!/usr/bin/env python3\n"
            "import json, os, sys\n"
            "from pathlib import Path\n"
            "Path(os.environ['DOCKER_TEST_RECORD']).write_text(json.dumps(sys.argv[1:]))\n",
            encoding="utf-8",
        )
        docker.chmod(0o700)
        self.env = dict(os.environ)
        self.env.update({
            "PATH": os.pathsep.join((str(self.scratch), str(Path(sys.executable).parent), os.environ["PATH"])),
            "DOCKER_TEST_RECORD": str(self.record),
            "AZCTX_AZURE_CLI_IMAGE": IMAGE,
            "AZURE_CONFIG_DIR": str(self.scratch / "azure with spaces"),
        })

    def cleanup(self):
        shutil.rmtree(self.scratch)
        try:
            self.scratch.parent.rmdir()
        except OSError:
            pass  # Another test may own a sibling directory.

    def run_shim(self, *args):
        return subprocess.run(
            ["bash", str(SHIM), *args], env=self.env, text=True,
            capture_output=True, check=False,
        )

    def test_immutable_container_and_literal_argv_with_only_allowlisted_mount(self):
        args = ["login", "--federated-token", "synthetic $(never-execute); value"]
        result = self.run_shim(*args)
        self.assertEqual(result.returncode, 0, result.stderr)
        argv = json.loads(self.record.read_text(encoding="utf-8"))
        self.assertEqual(argv[-5:], [IMAGE, "az", *args])
        mounts = [argv[index + 1] for index, value in enumerate(argv) if value == "--volume"]
        azure = self.env["AZURE_CONFIG_DIR"]
        self.assertEqual(mounts, [f"{azure}:{azure}"])
        self.assertIn("--cap-drop=ALL", argv)
        self.assertIn("--security-opt=no-new-privileges", argv)
        self.assertNotIn("/var/run/docker.sock", " ".join(argv))

    def test_unresolved_image_and_missing_config_never_launch_docker(self):
        for image in ("", "mcr.microsoft.com/azure-cli:latest", "other/image@" + "a" * 64):
            with self.subTest(image=image):
                self.env["AZCTX_AZURE_CLI_IMAGE"] = image
                self.assertNotEqual(self.run_shim("version").returncode, 0)
                self.assertFalse(self.record.exists())
        self.env["AZCTX_AZURE_CLI_IMAGE"] = IMAGE
        self.env["AZURE_CONFIG_DIR"] = ""
        self.assertNotEqual(self.run_shim("version").returncode, 0)
        self.assertFalse(self.record.exists())


if __name__ == "__main__":
    unittest.main()
