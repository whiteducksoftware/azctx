"""Test the host/container argv boundary with an offline Docker stand-in.

The fake records argv only; no image is pulled or run and nothing signs in.
"""

import json
import os
import shutil
import subprocess
import sys
import textwrap
import unittest
import uuid
from pathlib import Path


ROOT = Path(__file__).resolve().parents[1]
SHIM = ROOT / "scripts/az-container.sh"
IMAGE = "mcr.microsoft.com/azure-cli@sha256:" + "a" * 64


class ContainerShimTests(unittest.TestCase):
    def setUp(self):
        self.scratch = ROOT / ".test-work" / ("docker-" + uuid.uuid4().hex)
        self.scratch.mkdir(parents=True)
        self.addCleanup(self.cleanup)
        self.record = self.scratch / "argv.json"
        docker = self.scratch / "docker"
        docker.write_text(
            "#!/usr/bin/env python3\n" + textwrap.dedent("""\
                import json, os, sys
                from pathlib import Path

                argv = sys.argv[1:]
                Path(os.environ['DOCKER_TEST_RECORD']).write_text(json.dumps(argv))
                image = argv.index(os.environ['AZCTX_AZURE_CLI_IMAGE'])
                env = {}
                for index, value in enumerate(argv[:image]):
                    if value == '--env':
                        key, separator, literal = argv[index + 1].partition('=')
                        if separator:
                            env[key] = literal
                        elif key in os.environ:
                            env[key] = os.environ[key]
                command = argv[image + 2:]
                if command and command[0] == 'version':
                    output = env.get('AZURE_CORE_OUTPUT', 'json')
                    for index, value in enumerate(command):
                        if value in ('-o', '--output'):
                            output = command[index + 1]
                        elif value.startswith('--output='):
                            output = value.partition('=')[2]
                    version = os.environ.get('DOCKER_TEST_CLI_VERSION', '2.61.0')
                    if output == 'json':
                        print(json.dumps({'azure-cli': version}))
                    elif output == 'tsv':
                        print(version)
                """),
            encoding="utf-8",
        )
        docker.chmod(0o700)
        self.env = {key: value for key, value in os.environ.items() if not key.startswith("AZURE_")}
        self.home = self.scratch / "home"
        (self.home / ".azure").mkdir(parents=True)
        self.env.update({
            "HOME": str(self.home),
            "PATH": os.pathsep.join((str(self.scratch), str(Path(sys.executable).parent), os.environ["PATH"])),
            "DOCKER_TEST_RECORD": str(self.record),
            "AZCTX_AZURE_CLI_IMAGE": IMAGE,
            "AZURE_CONFIG_DIR": str(self.scratch / "azure with spaces"),
        })
        Path(self.env["AZURE_CONFIG_DIR"]).mkdir(mode=0o700)

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
        args = ["account", "set", "--subscription", "synthetic $(never-execute); value"]
        result = self.run_shim(*args)
        self.assertEqual(result.returncode, 0, result.stderr)
        argv = json.loads(self.record.read_text(encoding="utf-8"))
        azure = self.env["AZURE_CONFIG_DIR"]
        self.assertEqual(argv, [
            "run", "--rm", "--interactive", "--pull=never", "--cap-drop=ALL",
            "--security-opt=no-new-privileges", "--user", f"{os.getuid()}:{os.getgid()}",
            "--volume", f"{azure}:{azure}",
            "--env", f"HOME={azure}", "--env", "AZURE_CONFIG_DIR",
            "--env", "AZURE_CORE_ONLY_SHOW_ERRORS",
            "--env", "AZURE_CORE_LOGIN_EXPERIENCE_V2",
            "--env", "AZURE_EXTENSION_USE_DYNAMIC_INSTALL",
            IMAGE, "az", *args,
        ])
        self.assertEqual((result.stdout, result.stderr), ("", ""))

    def test_private_config_uses_host_uid_gid_and_writable_home(self):
        azure = Path(self.env["AZURE_CONFIG_DIR"])
        self.assertEqual(azure.stat().st_mode & 0o777, 0o700)
        self.assertEqual(azure.stat().st_uid, os.getuid())
        result = self.run_shim("account", "clear")
        self.assertEqual(result.returncode, 0, result.stderr)
        argv = json.loads(self.record.read_text(encoding="utf-8"))
        self.assertIn("--user", argv, "private host config requires its owning UID:GID")
        self.assertEqual(argv[argv.index("--user") + 1], f"{os.getuid()}:{os.getgid()}")
        env = [argv[index + 1] for index, value in enumerate(argv) if value == "--env"]
        self.assertIn(f"HOME={azure}", env, "arbitrary container UID needs a writable HOME")

    def test_version_without_output_flag_keeps_default_json(self):
        for version in ("2.61.0", "2.99.0"):
            with self.subTest(version=version):
                self.env["DOCKER_TEST_CLI_VERSION"] = version
                result = self.run_shim("version")
                self.assertEqual(result.returncode, 0, result.stderr)
                self.assertTrue(result.stdout.strip(), "az version without -o must stay JSON")
                self.assertEqual(json.loads(result.stdout)["azure-cli"], version)
                argv = json.loads(self.record.read_text(encoding="utf-8"))
                self.assertEqual(argv[argv.index(IMAGE):], [IMAGE, "az", "version"])
                self.assertEqual(result.stderr, "")

    def test_explicit_output_flags_are_preserved_over_the_json_default(self):
        cases = [
            (["-o", "none"], ""),
            (["--output", "none"], ""),
            (["--output=none"], ""),
            (["--query", '"azure-cli"', "-o", "tsv"], "2.61.0\n"),
            (["--output", "json"], '{"azure-cli": "2.61.0"}\n'),
        ]
        for flags, expected in cases:
            with self.subTest(flags=flags):
                result = self.run_shim("version", *flags)
                self.assertEqual((result.returncode, result.stdout, result.stderr), (0, expected, ""))
                argv = json.loads(self.record.read_text(encoding="utf-8"))
                self.assertEqual(argv[argv.index(IMAGE):], [IMAGE, "az", "version", *flags])

    def test_host_output_override_is_not_forwarded_to_the_isolated_cli(self):
        self.env["AZURE_CORE_OUTPUT"] = "none"
        result = self.run_shim("version")
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertTrue(result.stdout.strip(), "host output overrides must not reach the isolated CLI")
        self.assertEqual(json.loads(result.stdout)["azure-cli"], "2.61.0")

    def test_unresolved_image_and_missing_config_never_launch_docker(self):
        for image in ("", "mcr.microsoft.com/azure-cli:latest", "other/image@" + "a" * 64):
            with self.subTest(image=image):
                self.env["AZCTX_AZURE_CLI_IMAGE"] = image
                self.assertNotEqual(self.run_shim("version").returncode, 0)
                self.assertFalse(self.record.exists())
        self.env["AZCTX_AZURE_CLI_IMAGE"] = IMAGE
        for config in ("", str(self.scratch / "absent"), "relative"):
            with self.subTest(config=config):
                self.env["AZURE_CONFIG_DIR"] = config
                self.assertNotEqual(self.run_shim("version").returncode, 0)
                self.assertFalse(self.record.exists())

    def test_default_host_azure_config_is_never_mounted(self):
        link = self.scratch / "host-link"
        link.symlink_to(self.home / ".azure", target_is_directory=True)
        for config in (self.home / ".azure", link, self.home / ".azure" / "."):
            with self.subTest(config=config):
                self.env["AZURE_CONFIG_DIR"] = str(config)
                result = self.run_shim("version")
                self.assertNotEqual(result.returncode, 0)
                self.assertIn("dedicated", result.stderr)
                self.assertFalse(self.record.exists())


if __name__ == "__main__":
    unittest.main()
