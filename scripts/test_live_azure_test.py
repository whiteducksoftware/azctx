"""Offline regressions for the manual live Azure script.

Every azctx, az and docker process here is a local recording fake. Nothing
signs in, pulls an image, contacts Azure or reads Azure credential files.
"""

import json
import os
import shutil
import signal
import subprocess
import sys
import textwrap
import unittest
import uuid
from pathlib import Path


ROOT = Path(__file__).resolve().parents[1]
SCRIPT = ROOT / "scripts/live-azure-test.py"
TENANT = "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa"
FOREIGN = "bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb"
FIRST = "11111111-1111-1111-1111-111111111111"
SECOND = "22222222-2222-2222-2222-222222222222"
OTHER = "33333333-3333-3333-3333-333333333333"
PINNED = "mcr.microsoft.com/azure-cli@sha256:b83a4620937eccf09244e4dfbf310f774b01b3083b7accf49ab74d77247c8043"
LATEST_TAG = "mcr.microsoft.com/azure-cli:latest"
LATEST = "mcr.microsoft.com/azure-cli@sha256:" + "c" * 64
CI_VARIABLES = ("CI", "GITHUB_ACTIONS", "TF_BUILD")
SWITCH = ["-o", "json", "--"]
CURRENT = ["-c", "-o", "json"]
LIST = ["list", "-o", "json"]

# Stateful azctx stand-in: one current subscription per AZURE_CONFIG_DIR.
# FAKE_OVERRIDES maps a zero-based call index to a forced result or a signal
# sent to the script (its parent) before the call completes normally.
FAKE_AZCTX = textwrap.dedent("""\
    import json, os, shutil, subprocess, sys, time
    from pathlib import Path

    argv = sys.argv[1:]
    record = Path(os.environ["FAKE_RECORD"])
    with record.open("a", encoding="utf-8") as handle:
        handle.write(json.dumps({
            "argv": argv,
            "azure": os.environ.get("AZURE_CONFIG_DIR", ""),
            "azctx": os.environ.get("AZCTX_CONFIG_DIR", ""),
            "no_input": os.environ.get("AZCTX_NO_INPUT"),
            "no_update": os.environ.get("AZCTX_NO_UPDATE_CHECK"),
            "az": shutil.which("az") or "",
            "cwd": os.getcwd(),
        }) + "\\n")
    index = len(record.read_text(encoding="utf-8").splitlines()) - 1
    override = json.loads(os.environ.get("FAKE_OVERRIDES", "{}")).get(str(index), {})
    if "signal" in override:
        os.kill(os.getppid(), override["signal"])
        time.sleep(0.3)
    if "code" in override:
        sys.stdout.write(override.get("stdout", ""))
        sys.stderr.write(override.get("stderr", ""))
        sys.exit(override["code"])

    state_path = Path(os.environ["FAKE_STATE"])
    state = json.loads(state_path.read_text(encoding="utf-8"))
    key = os.environ.get("AZURE_CONFIG_DIR", "host")
    subscriptions = json.loads(os.environ["FAKE_SUBSCRIPTIONS"])
    tenant_of = {entry["id"]: entry["tenantId"] for entry in subscriptions}
    current = state[key]

    def describe(identifier, default):
        return {"id": identifier, "name": "Test " + identifier[:4], "tenantId": tenant_of[identifier],
                "tenantName": "Test tenant", "tenantDomain": "example.invalid", "state": "Enabled",
                "isDefault": default, "user": {"name": "", "type": "user"},
                "environment": "AzureCloud", "delegated": False}

    if argv == ["-c", "-o", "json"]:
        print(json.dumps({"schemaVersion": 1, "subscription": describe(current, True)}))
    elif argv == ["list", "-o", "json"]:
        print(json.dumps({"schemaVersion": 1, "subscriptions": [
            describe(entry["id"], entry["id"] == current) for entry in subscriptions]}))
    elif argv[:3] == ["-o", "json", "--"] and len(argv) == 4:
        target = argv[3]
        if target not in tenant_of:
            sys.stderr.write('{"code":"no_match","exit_code":1}\\n')
            sys.exit(1)
        az = subprocess.run(["az", "account", "set", "--subscription", target], check=False)
        if az.returncode != 0:
            sys.exit(1)
        if not override.get("skip_state"):
            state[key] = target
            state_path.write_text(json.dumps(state), encoding="utf-8")
    elif argv == ["env", "--shell", "bash"]:
        for name, value in (("ARM_SUBSCRIPTION_ID", current), ("ARM_TENANT_ID", tenant_of[current]),
                            ("AZURE_SUBSCRIPTION_ID", current), ("AZURE_TENANT_ID", tenant_of[current])):
            print(f"export {name}='{value}'")
    elif argv[:1] == ["guard"]:
        if argv[1:3] == ["--subscription", current] and argv[3:5] == ["--tenant", tenant_of[current]]:
            sys.exit(0)
        if argv[-2:] == ["-o", "json"]:
            sys.stderr.write('{"code":"context_mismatch","exit_code":4}\\n')
        sys.exit(4)
    else:
        sys.stderr.write("unexpected fake azctx arguments\\n")
        sys.exit(64)
    """)

FAKE_AZ = textwrap.dedent("""\
    import json, os, sys
    from pathlib import Path
    with Path(os.environ["FAKE_HOST_AZ_RECORD"]).open("a", encoding="utf-8") as handle:
        handle.write(json.dumps(sys.argv[1:]) + "\\n")
    """)

FAKE_DOCKER = textwrap.dedent("""\
    import json, os, sys
    from pathlib import Path
    argv = sys.argv[1:]
    with Path(os.environ["FAKE_DOCKER_RECORD"]).open("a", encoding="utf-8") as handle:
        handle.write(json.dumps({"argv": argv, "azure": os.environ.get("AZURE_CONFIG_DIR", "")}) + "\\n")
    images = json.loads(os.environ.get("FAKE_IMAGES", "{}"))
    if argv[:4] == ["image", "inspect", "--format", "{{json .RepoDigests}}"] and len(argv) == 5:
        if argv[4] not in images:
            sys.stderr.write("Error: No such image\\n")
            sys.exit(1)
        print(json.dumps(images[argv[4]]))
        sys.exit(0)
    if argv[:1] == ["run"]:
        image = next(value for value in argv if value.startswith("mcr.microsoft.com/azure-cli@"))
        command = argv[argv.index(image) + 1:]
        if command == ["az", "version", "-o", "json"]:
            print(json.dumps({"azure-cli": json.loads(os.environ["FAKE_VERSIONS"])[image]}))
        sys.exit(0)
    sys.stderr.write("unexpected fake docker arguments\\n")
    sys.exit(64)
    """)


def executable(path, source):
    path.write_text("#!" + sys.executable + "\n" + source, encoding="utf-8")
    path.chmod(0o700)
    return path


def host_sequence(initial=FIRST):
    sequence = [CURRENT, LIST]
    for identifier, other in ((FIRST, SECOND), (SECOND, FIRST)):
        sequence += [
            SWITCH + [identifier], CURRENT, ["env", "--shell", "bash"],
            ["guard", "--subscription", identifier, "--tenant", TENANT],
            ["guard", "--subscription", other, "--tenant", TENANT, "-o", "json"],
            CURRENT,
        ]
    return sequence + [SWITCH + [initial], CURRENT]


RESTORE = len(host_sequence()) - 2  # Index of the final restoring switch.


class LiveScriptTest(unittest.TestCase):
    def setUp(self):
        self.scratch = ROOT / ".test-work" / ("live-" + uuid.uuid4().hex)
        self.scratch.mkdir(parents=True)
        self.addCleanup(self.cleanup)
        self.bin = self.scratch / "bin"
        self.bin.mkdir()
        self.binary = executable(self.scratch / "azctx", FAKE_AZCTX)
        executable(self.bin / "az", FAKE_AZ)
        executable(self.bin / "docker", FAKE_DOCKER)
        self.temp = self.scratch / "tmp"
        self.temp.mkdir()
        self.home = self.scratch / "home"
        (self.home / ".azure").mkdir(parents=True, mode=0o700)
        self.containers = {}
        for name in ("old", "new"):
            directory = self.scratch / ("container-" + name)
            directory.mkdir()
            directory.chmod(0o700)
            self.containers[name] = directory
        self.record = self.scratch / "azctx.jsonl"
        self.host_az = self.scratch / "az.jsonl"
        self.docker = self.scratch / "docker.jsonl"
        self.state = self.scratch / "state.json"
        self.write_state(host=FIRST, old=SECOND, new=FIRST)
        self.subscriptions = [
            {"id": FIRST, "tenantId": TENANT}, {"id": SECOND, "tenantId": TENANT},
            {"id": OTHER, "tenantId": FOREIGN},
        ]
        self.env = {key: value for key, value in os.environ.items()
                    if key not in CI_VARIABLES and not key.startswith(("AZURE_", "AZCTX_", "FAKE_"))}
        self.env.update({
            "PATH": os.pathsep.join((str(self.bin), os.environ["PATH"])),
            "HOME": str(self.home),
            "TMPDIR": str(self.temp),
            "FAKE_RECORD": str(self.record),
            "FAKE_HOST_AZ_RECORD": str(self.host_az),
            "FAKE_DOCKER_RECORD": str(self.docker),
            "FAKE_STATE": str(self.state),
            "FAKE_IMAGES": json.dumps({
                PINNED: [PINNED],
                LATEST_TAG: ["mcr.microsoft.com/azure-cli@sha256:" + "d" * 63 + "x", LATEST],
            }),
            "FAKE_VERSIONS": json.dumps({PINNED: "2.61.0", LATEST: "2.79.0"}),
        })

    def cleanup(self):
        shutil.rmtree(self.scratch)
        try:
            self.scratch.parent.rmdir()
        except OSError:
            pass  # Another test may own a sibling directory.

    def write_state(self, host, old=SECOND, new=FIRST):
        self.state.write_text(json.dumps({
            "host": host, str(self.containers["old"]): old, str(self.containers["new"]): new,
        }), encoding="utf-8")

    def arguments(self, *extra, opt_in=True, containers=False):
        args = ["--binary", str(self.binary), "--tenant", TENANT,
                "--subscription", FIRST, "--subscription", SECOND, *extra]
        if opt_in:
            args.insert(0, "--opt-in-live-azure")
        if containers:
            args += ["--container", "2.61.0=" + str(self.containers["old"]),
                     "--container", "latest=" + str(self.containers["new"])]
        return args

    def run_script(self, args, overrides=None, **variables):
        env = dict(self.env)
        env["FAKE_SUBSCRIPTIONS"] = json.dumps(self.subscriptions)
        env["FAKE_OVERRIDES"] = json.dumps({str(key): value for key, value in (overrides or {}).items()})
        env.update(variables)
        return subprocess.run([sys.executable, "-B", str(SCRIPT), *args], env=env, text=True,
                              capture_output=True, check=False, timeout=60)

    def calls(self, path=None):
        path = path or self.record
        if not path.exists():
            return []
        return [json.loads(line) for line in path.read_text(encoding="utf-8").splitlines()]

    def argv(self):
        return [call["argv"] for call in self.calls()]

    def current(self, key="host"):
        return json.loads(self.state.read_text(encoding="utf-8"))[key]

    def assert_refused_without_processes(self, result):
        self.assertEqual(result.returncode, 2, result.stderr)
        self.assertIn("REFUSED", result.stderr)
        self.assertNotIn("PASS", result.stdout + result.stderr)
        self.assertNotIn("Traceback", result.stderr)
        for record in (self.record, self.host_az, self.docker):
            self.assertFalse(record.exists(), record.name + " shows an external call")
        self.assertEqual(list(self.temp.iterdir()), [], "refusal must not create a work directory")

    def test_refuses_without_explicit_opt_in_before_any_process(self):
        self.assert_refused_without_processes(self.run_script(self.arguments(opt_in=False)))
        result = self.run_script([])
        self.assert_refused_without_processes(result)
        self.assertIn("--opt-in-live-azure", result.stderr)

    def test_refuses_in_ci_even_with_opt_in(self):
        for name in CI_VARIABLES:
            for value in ("true", ""):
                with self.subTest(name=name, value=value):
                    result = self.run_script(self.arguments(), **{name: value})
                    self.assert_refused_without_processes(result)
                    self.assertIn("manual", result.stderr)

    def test_refuses_missing_invalid_or_duplicate_identifiers(self):
        base = ["--opt-in-live-azure", "--binary", str(self.binary)]
        cases = {
            "missing tenant": base + ["--subscription", FIRST, "--subscription", SECOND],
            "zero tenant": base + ["--tenant", "00000000-0000-0000-0000-000000000000",
                                   "--subscription", FIRST, "--subscription", SECOND],
            "shell tenant": base + ["--tenant", "$(touch never)", "--subscription", FIRST, "--subscription", SECOND],
            "one subscription": base + ["--tenant", TENANT, "--subscription", FIRST],
            "three subscriptions": self.arguments("--subscription", OTHER),
            "duplicate subscriptions": base + ["--tenant", TENANT, "--subscription", FIRST,
                                               "--subscription", FIRST.upper()],
            "invalid subscription": base + ["--tenant", TENANT, "--subscription", FIRST, "--subscription", "prod"],
            "missing binary": ["--opt-in-live-azure", "--binary", str(self.scratch / "absent"),
                               "--tenant", TENANT, "--subscription", FIRST, "--subscription", SECOND],
        }
        for name, args in cases.items():
            with self.subTest(name):
                self.assert_refused_without_processes(self.run_script(args))
        self.assertFalse((ROOT / "never").exists())

    def test_refuses_unsafe_or_ambiguous_container_configuration(self):
        old, new = self.containers["old"], self.containers["new"]
        link = self.scratch / "link"
        link.symlink_to(old, target_is_directory=True)
        shared = self.scratch / "shared"
        shared.mkdir()
        shared.chmod(0o750)
        nested = self.home / ".azure" / "nested"
        nested.mkdir(mode=0o700)
        custom = self.scratch / "custom-host"
        (custom / "inner").mkdir(parents=True)
        (custom / "inner").chmod(0o700)
        custom.chmod(0o700)
        cases = {
            "unknown version": (["--container", "2.70.0=" + str(old)], {}),
            "malformed": (["--container", str(old)], {}),
            "duplicate version": (["--container", "latest=" + str(old), "--container", "latest=" + str(new)], {}),
            "shared directory": (["--container", "2.61.0=" + str(old), "--container", "latest=" + str(old)], {}),
            "default host config": (["--container", "latest=" + str(self.home / ".azure")], {}),
            "inside default host config": (["--container", "latest=" + str(nested)], {}),
            "explicit host config": (["--container", "latest=" + str(old)], {"AZURE_CONFIG_DIR": str(old)}),
            "inside explicit host config": (["--container", "latest=" + str(custom / "inner")],
                                            {"AZURE_CONFIG_DIR": str(custom)}),
            "contains host config": (["--container", "latest=" + str(custom)],
                                     {"AZURE_CONFIG_DIR": str(custom / "inner")}),
            "symlink": (["--container", "latest=" + str(link)], {}),
            "group readable": (["--container", "latest=" + str(shared)], {}),
            "missing": (["--container", "latest=" + str(self.scratch / "absent")], {}),
            "relative": (["--container", "latest=relative"], {}),
        }
        for name, (extra, variables) in cases.items():
            with self.subTest(name):
                self.assert_refused_without_processes(self.run_script(self.arguments(*extra), **variables))

    def test_signed_in_tenant_mismatch_refuses_after_one_read_only_check(self):
        self.write_state(host=OTHER)
        result = self.run_script(self.arguments())
        self.assertEqual(result.returncode, 2, result.stderr)
        self.assertIn("REFUSED", result.stderr)
        self.assertIn("tenant", result.stderr)
        self.assertIn("nothing was changed", result.stderr)
        self.assertEqual(self.argv(), [CURRENT])
        self.assertFalse(self.host_az.exists())
        self.assertEqual(self.current(), OTHER)
        self.assertNotIn("PASS", result.stdout)

    def test_host_leg_runs_exact_sequence_restores_and_reports_unverified_containers(self):
        self.write_state(host=SECOND)
        result = self.run_script(self.arguments())
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual(self.argv(), host_sequence(initial=SECOND))
        self.assertEqual(self.current(), SECOND)
        self.assertEqual([json.loads(line) for line in self.host_az.read_text().splitlines()], [
            ["account", "set", "--subscription", FIRST], ["account", "set", "--subscription", SECOND],
            ["account", "set", "--subscription", SECOND],
        ])
        self.assertFalse(self.docker.exists(), "host default must never start containers")
        calls = self.calls()
        private = {call["azctx"] for call in calls}
        self.assertEqual(len(private), 1)
        self.assertTrue(Path(private.pop()).is_relative_to(self.temp), "azctx state must be isolated")
        self.assertTrue(all(call["azure"] == "" for call in calls), "host leg keeps the existing login")
        self.assertTrue(all(call["no_input"] == "1" and call["no_update"] == "1" for call in calls))
        self.assertTrue(all(Path(call["cwd"]).is_relative_to(self.temp) for call in calls))
        self.assertEqual(list(self.temp.iterdir()), [], "private work directory must be removed")
        self.assertIn("PASS host Azure CLI", result.stdout)
        self.assertIn("NOT RUN container Azure CLI 2.61.0", result.stdout)
        self.assertIn("NOT RUN container Azure CLI latest", result.stdout)
        self.assertIn("unverified", result.stdout)
        self.assertNotIn("PASS: all legs", result.stdout)
        self.assertEqual(result.stderr, "")

    def test_container_legs_use_only_explicit_dedicated_configs_and_frozen_images(self):
        result = self.run_script(self.arguments(containers=True))
        self.assertEqual(result.returncode, 0, result.stderr)
        old, new = str(self.containers["old"]), str(self.containers["new"])
        self.assertEqual((self.current(), self.current(old), self.current(new)), (FIRST, SECOND, FIRST))
        calls = self.calls()
        by_leg = {}
        for call in calls:
            by_leg.setdefault(call["azure"], []).append(call)
        self.assertEqual(set(by_leg), {"", old, new})
        self.assertEqual([call["argv"] for call in by_leg[""]], host_sequence())
        self.assertEqual([call["argv"] for call in by_leg[old]], host_sequence(initial=SECOND))
        self.assertEqual([call["argv"] for call in by_leg[new]], host_sequence())
        # Every leg verifies its signed-in tenant before any leg switches.
        self.assertEqual([call["argv"] for call in calls[:3]], [CURRENT] * 3)
        for leg in (old, new):
            shims = {call["az"] for call in by_leg[leg]}
            self.assertEqual(len(shims), 1)
            shim = Path(shims.pop())
            self.assertTrue(shim.is_relative_to(self.temp) and shim.name == "az")
        self.assertEqual(len({call["azctx"] for call in calls}), 3, "each leg gets fresh azctx state")
        self.assertEqual(len(self.host_az.read_text().splitlines()), 3, "only the host leg uses host az")
        docker = self.calls(self.docker)
        self.assertEqual([call["argv"] for call in docker if call["argv"][0] == "image"], [
            ["image", "inspect", "--format", "{{json .RepoDigests}}", PINNED],
            ["image", "inspect", "--format", "{{json .RepoDigests}}", LATEST_TAG],
        ])
        runs = [call for call in docker if call["argv"][0] == "run"]
        self.assertEqual(len(runs), 8, "one version check and three switches per container leg")
        for call in runs:
            argv, azure = call["argv"], call["azure"]
            self.assertIn(azure, (old, new))
            image = PINNED if azure == old else LATEST
            self.assertEqual(argv[:argv.index(image)], [
                "run", "--rm", "--interactive", "--pull=never", "--cap-drop=ALL",
                "--security-opt=no-new-privileges", "--user", f"{os.getuid()}:{os.getgid()}",
                "--volume", azure + ":" + azure, "--env", "HOME=" + azure, "--env", "AZURE_CONFIG_DIR",
                "--env", "AZURE_CORE_ONLY_SHOW_ERRORS", "--env", "AZURE_CORE_LOGIN_EXPERIENCE_V2",
                "--env", "AZURE_EXTENSION_USE_DYNAMIC_INSTALL",
            ])
            self.assertIn(argv[argv.index(image) + 1:][:2], (["az", "version"], ["az", "account"]))
            self.assertNotIn(str(self.home / ".azure"), " ".join(argv))
        self.assertIn("PASS container Azure CLI 2.61.0 (2.61.0, " + PINNED + ")", result.stdout)
        self.assertIn("PASS container Azure CLI latest (2.79.0, " + LATEST + ")", result.stdout)
        self.assertIn("PASS: all legs", result.stdout)
        self.assertNotIn("NOT RUN", result.stdout)
        self.assertEqual(list(self.temp.iterdir()), [])

    def test_container_preflight_failures_refuse_before_any_switch(self):
        cases = {
            "image not present": {"FAKE_IMAGES": json.dumps({})},
            "wrong minimum version": {"FAKE_VERSIONS": json.dumps({PINNED: "2.61.1", LATEST: "2.79.0"})},
            "latest below minimum": {"FAKE_VERSIONS": json.dumps({PINNED: "2.61.0", LATEST: "2.60.9"})},
            "ambiguous latest digest": {"FAKE_IMAGES": json.dumps({PINNED: [PINNED], LATEST_TAG: [LATEST, PINNED]})},
            "unpinned minimum digest": {"FAKE_IMAGES": json.dumps({PINNED: [LATEST], LATEST_TAG: [LATEST]})},
        }
        for name, variables in cases.items():
            with self.subTest(name):
                for record in (self.record, self.docker):
                    record.unlink(missing_ok=True)
                result = self.run_script(self.arguments(containers=True), **variables)
                self.assertEqual(result.returncode, 2, result.stderr)
                self.assertIn("REFUSED", result.stderr)
                self.assertEqual(self.argv(), [], "image checks precede every azctx call")
                self.assertFalse(self.host_az.exists())
                self.assertNotIn("PASS", result.stdout)
                self.assertEqual(list(self.temp.iterdir()), [])
        self.assertIn("docker pull", self.run_script(self.arguments(containers=True),
                                                     FAKE_IMAGES=json.dumps({})).stderr)

    def test_container_tenant_mismatch_refuses_before_host_switch(self):
        self.write_state(host=FIRST, new=OTHER)
        result = self.run_script(self.arguments(containers=True))
        self.assertEqual(result.returncode, 2, result.stderr)
        self.assertIn("container Azure CLI latest", result.stderr)
        self.assertEqual(self.argv(), [CURRENT] * 3)
        self.assertFalse(self.host_az.exists())

    def test_check_failure_restores_initial_context_without_leaking_output(self):
        secret = "AccountKey=c2VjcmV0LXNob3VsZC1uZXZlci1hcHBlYXI="
        result = self.run_script(self.arguments(), {4: {"code": 0, "stdout": "export X='" + secret + "'\n"}})
        self.assertEqual(result.returncode, 1, result.stderr)
        self.assertIn("FAILED", result.stderr)
        self.assertIn("env --shell bash", result.stderr)
        self.assertIn("initial context restored", result.stderr)
        self.assertNotIn(secret, result.stdout + result.stderr)
        self.assertEqual(self.argv()[-2:], [SWITCH + [FIRST], CURRENT])
        self.assertEqual(self.current(), FIRST)
        self.assertNotIn("PASS", result.stdout)

    def test_failed_command_reports_only_safe_json_error_code(self):
        stderr = '{"code":"az_failed","exit_code":1,"message":"AccountKey=leak"}\n'
        result = self.run_script(self.arguments(), {5: {"code": 1, "stderr": stderr}})
        self.assertEqual(result.returncode, 1, result.stderr)
        self.assertIn("az_failed", result.stderr)
        self.assertNotIn("leak", result.stderr)
        result = self.run_script(self.arguments(), {5: {"code": 1, "stderr": '{"code":"\\u001b[31m"}'}})
        self.assertNotIn("\x1b", result.stderr)

    def test_failure_before_mutation_does_not_switch(self):
        result = self.run_script(self.arguments(), {1: {"code": 0, "stdout": '{"schemaVersion":2}'}})
        self.assertEqual(result.returncode, 1, result.stderr)
        self.assertEqual(self.argv(), [CURRENT, LIST])
        self.assertIn("unchanged", result.stderr)
        self.assertFalse(self.host_az.exists())

    def test_missing_test_subscription_or_foreign_tenant_fails_before_switch(self):
        for subscriptions in ([{"id": FIRST, "tenantId": TENANT}, {"id": SECOND, "tenantId": FOREIGN}],
                              [{"id": FIRST, "tenantId": TENANT}]):
            with self.subTest(subscriptions=subscriptions):
                self.record.unlink(missing_ok=True)
                self.subscriptions = subscriptions
                result = self.run_script(self.arguments())
                self.assertEqual(result.returncode, 1, result.stderr)
                self.assertEqual(self.argv(), [CURRENT, LIST])
                self.assertFalse(self.host_az.exists())

    def test_guard_mismatch_must_fail_with_json_contract(self):
        result = self.run_script(self.arguments(), {6: {"code": 0}})
        self.assertEqual(result.returncode, 1, result.stderr)
        self.assertIn("guard", result.stderr)
        self.assertEqual(self.current(), FIRST)
        self.assertEqual(self.argv()[-2:], [SWITCH + [FIRST], CURRENT])

    def test_failed_restore_command_is_visible_and_never_success_shaped(self):
        result = self.run_script(self.arguments(), {
            6: {"code": 0}, 7: {"code": 1, "stderr": '{"code":"az_failed","exit_code":1}\n'},
        })
        self.assertEqual(result.returncode, 3, result.stderr)
        self.assertIn("RESTORE FAILED", result.stderr)
        self.assertIn("az_failed", result.stderr)
        self.assertIn("az account set --subscription " + FIRST, result.stderr)
        self.assertIn("guard", result.stderr, "the original failure must remain visible")
        self.assertNotIn("PASS", result.stdout)
        self.assertNotIn("context restored", result.stderr)

    def test_unverified_restore_is_a_restore_failure(self):
        result = self.run_script(self.arguments(), {RESTORE: {"skip_state": True}})
        self.assertEqual(result.returncode, 3, result.stderr)
        self.assertIn("RESTORE FAILED", result.stderr)
        self.assertNotIn("PASS", result.stdout)
        self.assertEqual(self.current(), SECOND)

    def test_restore_failure_after_successful_checks_still_fails(self):
        result = self.run_script(self.arguments(), {RESTORE: {"code": 1}})
        self.assertEqual(result.returncode, 3, result.stderr)
        self.assertIn("RESTORE FAILED", result.stderr)
        self.assertNotIn("PASS", result.stdout)

    def test_sigint_during_switch_restores_and_exits_130(self):
        result = self.run_script(self.arguments(), {2: {"signal": int(signal.SIGINT)}})
        self.assertEqual(result.returncode, 130, result.stderr)
        self.assertIn("INTERRUPTED", result.stderr)
        self.assertIn("initial context restored", result.stderr)
        self.assertEqual(self.argv(), [CURRENT, LIST, SWITCH + [FIRST], SWITCH + [FIRST], CURRENT])
        self.assertEqual(self.current(), FIRST)
        self.assertNotIn("PASS", result.stdout)
        self.assertEqual(list(self.temp.iterdir()), [])

    def test_repeated_sigint_cannot_abort_restoration(self):
        self.write_state(host=SECOND)
        signals = {index: {"signal": int(signal.SIGINT)} for index in (2, 3, 4)}
        result = self.run_script(self.arguments(), signals)
        self.assertEqual(result.returncode, 130, result.stderr)
        self.assertEqual(self.argv()[2:], [SWITCH + [FIRST], SWITCH + [SECOND], CURRENT])
        self.assertEqual(self.current(), SECOND)
        self.assertIn("initial context restored", result.stderr)

    def test_sigint_before_mutation_exits_without_switching(self):
        result = self.run_script(self.arguments(), {0: {"signal": int(signal.SIGINT)}})
        self.assertEqual(result.returncode, 130, result.stderr)
        self.assertEqual(self.argv(), [CURRENT])
        self.assertIn("unchanged", result.stderr)

    def test_sigterm_restores_and_exits_143(self):
        result = self.run_script(self.arguments(), {5: {"signal": int(signal.SIGTERM)}})
        self.assertEqual(result.returncode, 143, result.stderr)
        self.assertEqual(self.current(), FIRST)
        self.assertEqual(self.argv()[-2:], [SWITCH + [FIRST], CURRENT])

    def test_interrupt_with_failed_restore_reports_restore_failure(self):
        result = self.run_script(self.arguments(), {2: {"signal": int(signal.SIGINT)}, 3: {"code": 1}})
        self.assertEqual(result.returncode, 3, result.stderr)
        self.assertIn("RESTORE FAILED", result.stderr)
        self.assertIn("INTERRUPTED", result.stderr)

    def test_make_target_builds_then_forwards_only_maintainer_arguments(self):
        makefile = (ROOT / "Makefile").read_text(encoding="utf-8")
        self.assertIn(" live-test", makefile.splitlines()[0])
        recipe = makefile.split("\nlive-test: build\n", 1)[1].split("\n\n", 1)[0]
        self.assertEqual(recipe.strip(), "python3 -B scripts/live-azure-test.py --binary ./azctx $(LIVE_TEST_ARGS)")


if __name__ == "__main__":
    unittest.main()
