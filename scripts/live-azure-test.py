#!/usr/bin/env python3
"""Manual live Azure check for azctx. Maintainers run it locally; CI must not.

It uses the Azure CLI sign-in that already exists on this machine: it never
signs in, reads or copies credentials, or pulls images. Before it changes
anything it confirms that every requested Azure CLI is signed in to the
expected dedicated test tenant. It then switches between the two test
subscriptions, checks list/current/env/guard output and always tries to
restore the initial subscription, even after a failure or Ctrl+C.

Exit codes: 0 passed, 1 check failed (context restored or unchanged),
2 refused or usage error (nothing changed), 3 RESTORE FAILED (fix manually),
128+N interrupted by signal N (context restored or unchanged).
"""

import argparse
import json
import os
import re
import shutil
import signal
import stat
import subprocess
import sys
import tempfile
from pathlib import Path


SHIM = Path(__file__).resolve().parent / "az-container.sh"
MINIMUM = "2.61.0"
PINNED_MINIMUM = "mcr.microsoft.com/azure-cli@sha256:b83a4620937eccf09244e4dfbf310f774b01b3083b7accf49ab74d77247c8043"
LATEST_TAG = "mcr.microsoft.com/azure-cli:latest"
DIGEST = re.compile(r"mcr\.microsoft\.com/azure-cli@sha256:[a-f0-9]{64}")
GUID = re.compile(r"[0-9a-fA-F]{8}(?:-[0-9a-fA-F]{4}){3}-[0-9a-fA-F]{12}")
ZERO = "00000000-0000-0000-0000-000000000000"
SAFE_CODE = re.compile(r"[a-z_]+")
VERSION = re.compile(r"(\d+)\.(\d+)\.(\d+)")
CI_VARIABLES = ("CI", "GITHUB_ACTIONS", "TF_BUILD")
CONTAINER_VERSIONS = (MINIMUM, "latest")
TIMEOUT = 120
HOST = "host Azure CLI"


class Refused(Exception):
    """Nothing was changed; exit 2."""


class CheckFailed(Exception):
    """A check failed; exit 1."""


class Interrupted(Exception):
    def __init__(self, signum):
        super().__init__(f"INTERRUPTED by {signal.Signals(signum).name}")
        self.signum = signum


class RestoreFailed(Exception):
    """The initial context could not be restored or verified; exit 3."""


class Parser(argparse.ArgumentParser):
    def error(self, message):
        raise Refused(message)


class Leg:
    def __init__(self, name, version=None, config=None):
        self.name, self.version, self.config = name, version, config
        self.image = self.cli_version = self.initial = None
        self.env = self.cwd = None


def guid(value):
    return isinstance(value, str) and GUID.fullmatch(value) is not None and value.lower() != ZERO


def schema(document):
    if not isinstance(document, dict) or type(document.get("schemaVersion")) is not int or document["schemaVersion"] != 1:
        raise CheckFailed("expected schemaVersion 1")


def subscription(document):
    if not isinstance(document, dict):
        raise CheckFailed("expected subscription object")
    for key in ("id", "tenantId"):
        if not guid(document.get(key)):
            raise CheckFailed("invalid subscription identity")
    for key in ("name", "tenantName", "tenantDomain", "state", "environment"):
        if not isinstance(document.get(key), str):
            raise CheckFailed("invalid subscription string field")
    for key in ("isDefault", "delegated"):
        if type(document.get(key)) is not bool:
            raise CheckFailed("invalid subscription boolean field")
    user = document.get("user")
    if not isinstance(user, dict) or any(not isinstance(user.get(key), str) for key in ("name", "type")):
        raise CheckFailed("invalid subscription user shape")
    return document


def subscriptions(document):
    schema(document)
    entries = document.get("subscriptions")
    if not isinstance(entries, list):
        raise CheckFailed("expected subscription array")
    seen = set()
    for entry in entries:
        subscription(entry)
        identity = (entry["environment"].lower(), entry["id"].lower())
        if identity in seen:
            raise CheckFailed("duplicate subscription identity")
        seen.add(identity)
    return entries


def matches(entry, identifier, tenant):
    return (entry["id"].lower() == identifier and entry["tenantId"].lower() == tenant
            and entry["environment"] == "AzureCloud")


def safe_code(stderr):
    """Return the azctx JSON error code only; never echo raw diagnostics."""
    try:
        document = json.loads(stderr.strip().splitlines()[0]) if stderr.strip() else None
    except ValueError:
        return ""
    code = document.get("code") if isinstance(document, dict) else None
    return f" (code {code})" if isinstance(code, str) and SAFE_CODE.fullmatch(code) else ""


def inside(child, parent):
    try:
        return os.path.commonpath((child, parent)) == parent
    except ValueError:
        return False


def parse(argv):
    parser = Parser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    parser.add_argument("--opt-in-live-azure", action="store_true",
                        help="confirm a deliberate manual run against the dedicated test tenant")
    parser.add_argument("--tenant", help="expected dedicated test tenant GUID")
    parser.add_argument("--subscription", action="append", default=[],
                        help="test subscription GUID; pass exactly two distinct values")
    parser.add_argument("--binary", default="./azctx", help="azctx binary to test (default ./azctx)")
    parser.add_argument("--container", action="append", default=[], metavar="VERSION=DIR",
                        help="also test Azure CLI 2.61.0 or latest in Docker, signed in separately "
                             "with the private absolute AZURE_CONFIG_DIR DIR")
    return parser.parse_args(argv)


def validate(argv):
    present = [name for name in CI_VARIABLES if name in os.environ]
    if present:
        raise Refused(f"live Azure checks are a manual maintainer action; {present[0]} is set, so this looks like CI")
    args = parse(argv)
    if not args.opt_in_live_azure:
        raise Refused("pass --opt-in-live-azure to confirm a manual run against the dedicated test tenant")
    if not guid(args.tenant):
        raise Refused("--tenant must be the nonzero GUID of the dedicated test tenant")
    identifiers = [value.lower() for value in args.subscription if guid(value)]
    if len(args.subscription) != 2 or len(identifiers) != 2 or identifiers[0] == identifiers[1]:
        raise Refused("pass --subscription exactly twice with two distinct nonzero test subscription GUIDs")
    binary = Path(args.binary).absolute()
    if not binary.is_file():
        raise Refused("--binary must name an existing azctx executable (run make build)")
    legs = [Leg(HOST)]
    seen = {}
    for spec in args.container:
        version, separator, directory = spec.partition("=")
        if not separator or version not in CONTAINER_VERSIONS or not directory:
            raise Refused("--container must be 2.61.0=DIR or latest=DIR")
        if version in seen:
            raise Refused(f"--container {version} was given twice")
        seen[version] = container_config(directory, seen.values())
    if seen:
        if os.name != "posix":
            raise Refused("container legs require a POSIX host")
        if shutil.which("docker") is None:
            raise Refused("container legs require docker on PATH")
        if not SHIM.is_file():
            raise Refused("container shim scripts/az-container.sh is missing")
    for version in CONTAINER_VERSIONS:
        if version in seen:
            legs.append(Leg(f"container Azure CLI {version}", version, seen[version]))
    return args.tenant.lower(), identifiers, str(binary), legs


def container_config(directory, others):
    if not os.path.isabs(directory):
        raise Refused("container config directories must be absolute paths")
    try:
        info = os.lstat(directory)
    except OSError:
        raise Refused("container config directory does not exist; create it and sign in separately first") from None
    if stat.S_ISLNK(info.st_mode) or not stat.S_ISDIR(info.st_mode):
        raise Refused("container config directory must be a real directory, not a symlink")
    if info.st_uid != os.getuid() or info.st_mode & 0o077:
        raise Refused("container config directory must be owned by you with mode 0700")
    real = os.path.realpath(directory)
    hosts = [os.path.expanduser("~/.azure")]
    if os.environ.get("AZURE_CONFIG_DIR"):
        hosts.append(os.environ["AZURE_CONFIG_DIR"])
    for host in (os.path.realpath(os.path.abspath(path)) for path in hosts):
        if inside(real, host) or inside(host, real):
            raise Refused("container config directory must be dedicated and separate from the host Azure CLI config")
    for other in others:
        if inside(real, os.path.realpath(other)) or inside(os.path.realpath(other), real):
            raise Refused("each container leg needs its own dedicated config directory")
    return os.path.normpath(directory)


class Runner:
    def __init__(self, tenant, identifiers, binary, legs, work):
        self.tenant, self.identifiers, self.binary, self.legs = tenant, identifiers, binary, legs
        self.work = Path(work)
        self.pending = None
        self.restoring = False
        base = {key: value for key, value in os.environ.items() if not key.startswith("AZCTX_")}
        base.update({"AZCTX_NO_INPUT": "1", "AZCTX_NO_UPDATE_CHECK": "1", "NO_COLOR": "1"})
        for index, leg in enumerate(legs):
            leg.cwd = self.work / f"leg{index}"
            (leg.cwd / "azctx").mkdir(parents=True, mode=0o700)
            leg.env = dict(base, AZCTX_CONFIG_DIR=str(leg.cwd / "azctx"))
            if leg.config:
                shim = leg.cwd / "bin" / "az"
                shim.parent.mkdir(mode=0o700)
                shutil.copyfile(SHIM, shim)
                shim.chmod(0o700)
                leg.env["AZURE_CONFIG_DIR"] = leg.config
                leg.env["PATH"] = str(shim.parent) + os.pathsep + base.get("PATH", "")

    def signal(self, signum, _frame):
        if self.pending is None:
            self.pending = signum
        note = "restoring the initial context first" if self.restoring else "stopping and restoring the initial context"
        print(f"{signal.Signals(signum).name} received; {note}", file=sys.stderr, flush=True)

    def checkpoint(self):
        if self.pending is not None and not self.restoring:
            raise Interrupted(self.pending)

    def execute(self, argv, env, cwd):
        self.checkpoint()
        options = {"start_new_session": True} if os.name == "posix" else {
            "creationflags": subprocess.CREATE_NEW_PROCESS_GROUP}
        try:
            process = subprocess.Popen(argv, cwd=cwd, env=env, stdin=subprocess.DEVNULL, stdout=subprocess.PIPE,
                                       stderr=subprocess.PIPE, text=True, encoding="utf-8", errors="replace",
                                       **options)
        except OSError:
            raise CheckFailed("could not start " + Path(argv[0]).name) from None
        try:
            stdout, stderr = process.communicate(timeout=TIMEOUT)
        except subprocess.TimeoutExpired:
            if os.name == "posix":
                os.killpg(process.pid, signal.SIGKILL)
            else:
                process.kill()
            process.communicate()
            raise CheckFailed(f"{Path(argv[0]).name} timed out after {TIMEOUT}s") from None
        self.checkpoint()
        return process.returncode, stdout, stderr

    def azctx(self, leg, *args, code=0):
        command = "`azctx " + " ".join(args) + "`"
        returncode, stdout, stderr = self.execute([self.binary, *args], leg.env, leg.cwd)
        if returncode != code:
            raise CheckFailed(f"{command} exited {returncode}, expected {code}{safe_code(stderr)}")
        if code == 0 and stderr:
            raise CheckFailed(f"{command} wrote unexpected diagnostics")
        return stdout, stderr

    def json(self, leg, *args):
        stdout, _ = self.azctx(leg, *args)
        try:
            return json.loads(stdout)
        except ValueError:
            raise CheckFailed("`azctx " + " ".join(args) + "` did not print JSON") from None

    def current(self, leg):
        document = self.json(leg, "-c", "-o", "json")
        schema(document)
        return subscription(document.get("subscription"))

    def preflight(self, leg):
        if leg.version == MINIMUM:
            reference, wanted = PINNED_MINIMUM, PINNED_MINIMUM
        else:
            reference, wanted = LATEST_TAG, None
        code, stdout, _ = self.execute(["docker", "image", "inspect", "--format", "{{json .RepoDigests}}", reference],
                                       leg.env, leg.cwd)
        if code != 0:
            raise Refused(f"{leg.name}: image {reference} is not present locally; run `docker pull {reference}` "
                          "yourself (this script never pulls)")
        try:
            digests = json.loads(stdout)
        except ValueError:
            digests = None
        digests = [value for value in digests if isinstance(value, str) and DIGEST.fullmatch(value)] \
            if isinstance(digests, list) else []
        if wanted is not None and wanted not in digests:
            raise Refused(f"{leg.name}: local image does not carry the pinned digest {wanted}")
        if wanted is None and len(digests) != 1:
            raise Refused(f"{leg.name}: {LATEST_TAG} must resolve to exactly one MCR digest locally")
        leg.image = wanted or digests[0]
        leg.env["AZCTX_AZURE_CLI_IMAGE"] = leg.image
        code, stdout, _ = self.execute([str(leg.cwd / "bin" / "az"), "version", "-o", "json"], leg.env, leg.cwd)
        try:
            version = json.loads(stdout).get("azure-cli") if code == 0 else None
        except (ValueError, AttributeError):
            version = None
        parsed = VERSION.fullmatch(version) if isinstance(version, str) else None
        if parsed is None:
            raise Refused(f"{leg.name}: could not read the Azure CLI version from {leg.image}")
        number = tuple(int(part) for part in parsed.groups())
        if (leg.version == MINIMUM and version != MINIMUM) or number < (2, 61, 0):
            raise Refused(f"{leg.name}: image reports Azure CLI {version}, expected "
                          + (MINIMUM if leg.version == MINIMUM else f">= {MINIMUM}"))
        leg.cli_version = version

    def verify_signed_in_tenant(self, leg):
        initial = self.current(leg)
        if initial["tenantId"].lower() != self.tenant or initial["environment"] != "AzureCloud":
            raise Refused(f"{leg.name}: the signed-in tenant/cloud is not --tenant in AzureCloud; nothing was changed")
        if initial["id"].lower() not in self.identifiers:
            raise Refused(f"{leg.name}: the current subscription is not one of the two test subscriptions; "
                          "nothing was changed")
        leg.initial = initial["id"].lower()

    def exercise(self, leg):
        entries = subscriptions(self.json(leg, "list", "-o", "json"))
        for identifier in self.identifiers:
            found = [entry for entry in entries if entry["id"].lower() == identifier]
            if len(found) != 1 or not matches(found[0], identifier, self.tenant):
                raise CheckFailed("both test subscriptions must be listed once in --tenant and AzureCloud")
        failure = None
        try:
            for identifier in self.identifiers:
                if self.azctx(leg, "-o", "json", "--", identifier)[0]:
                    raise CheckFailed("switch must be quiet")
                if not matches(self.current(leg), identifier, self.tenant):
                    raise CheckFailed("current context does not match the selected test subscription")
                expected = "".join(f"export {key}='{value}'\n" for key, value in (
                    ("ARM_SUBSCRIPTION_ID", identifier), ("ARM_TENANT_ID", self.tenant),
                    ("AZURE_SUBSCRIPTION_ID", identifier), ("AZURE_TENANT_ID", self.tenant)))
                if self.azctx(leg, "env", "--shell", "bash")[0] != expected:
                    raise CheckFailed("`azctx env --shell bash` does not export the current context")
                if self.azctx(leg, "guard", "--subscription", identifier, "--tenant", self.tenant)[0]:
                    raise CheckFailed("matching `azctx guard` must be quiet")
                other = next(value for value in self.identifiers if value != identifier)
                stdout, stderr = self.azctx(leg, "guard", "--subscription", other, "--tenant", self.tenant,
                                            "-o", "json", code=4)
                try:
                    error = json.loads(stderr)
                except ValueError:
                    error = None
                if stdout or not isinstance(error, dict) or error.get("code") != "context_mismatch" \
                        or error.get("exit_code") != 4:
                    raise CheckFailed("mismatching `azctx guard` broke its JSON error contract")
                if not matches(self.current(leg), identifier, self.tenant):
                    raise CheckFailed("guard changed the current context")
        except BaseException as error:  # Restore after every failure, including bugs.
            failure = error
        reason = self.restore(leg)
        if reason is not None:
            raise RestoreFailed(self.restore_report(leg, reason, failure))
        if failure is not None:
            failure.restored = True
            raise failure
        self.checkpoint()

    def restore(self, leg):
        self.restoring = True
        try:
            if self.azctx(leg, "-o", "json", "--", leg.initial)[0]:
                return "restoring switch printed unexpected output"
            if not matches(self.current(leg), leg.initial, self.tenant):
                return "the current subscription after restoring is not the initial subscription"
            return None
        except CheckFailed as error:
            return str(error)
        finally:
            self.restoring = False

    def restore_report(self, leg, reason, failure):
        original = "none; all checks had passed" if failure is None else str(failure) or type(failure).__name__
        if isinstance(failure, Interrupted):
            original = str(failure)
        lines = [f"RESTORE FAILED: {leg.name}: could not restore and verify the initial subscription: {reason}",
                 f"  original failure: {original}",
                 f"  fix it manually: az account set --subscription {leg.initial}"]
        if leg.config:
            lines[-1] = (f"  fix it manually: AZURE_CONFIG_DIR={leg.config} AZCTX_AZURE_CLI_IMAGE={leg.image} "
                         f"scripts/az-container.sh account set --subscription {leg.initial}")
        return "\n".join(lines)

    def run(self):
        for leg in self.legs[1:]:
            self.preflight(leg)
        for leg in self.legs:
            self.verify_signed_in_tenant(leg)
        for leg in self.legs:
            try:
                self.exercise(leg)
            except (CheckFailed, Interrupted) as error:
                error.leg = leg.name
                raise
        lines = []
        for leg in self.legs:
            label = leg.name if leg.config is None else f"{leg.name} ({leg.cli_version}, {leg.image})"
            lines.append(f"PASS {label}: schema, list, two switches, env, guard match/mismatch, "
                         "initial context restored")
        names = {leg.version for leg in self.legs}
        for version in CONTAINER_VERSIONS:
            if version not in names:
                lines.append(f"NOT RUN container Azure CLI {version}: unverified; "
                             f"pass --container {version}=DIR to cover it")
        if len(self.legs) == 1 + len(CONTAINER_VERSIONS):
            lines.append("PASS: all legs")
        print("\n".join(lines))


def outcome(error):
    where = getattr(error, "leg", None)
    prefix = f"{where}: " if where else ""
    state = "initial context restored" if getattr(error, "restored", False) else "Azure CLI context unchanged"
    return prefix, state


def main(argv=None):
    try:
        tenant, identifiers, binary, legs = validate(sys.argv[1:] if argv is None else argv)
    except Refused as error:
        print(f"REFUSED: {error}", file=sys.stderr)
        return 2
    work = tempfile.mkdtemp(prefix="azctx-live-")
    try:
        runner = Runner(tenant, identifiers, binary, legs, work)
        signal.signal(signal.SIGINT, runner.signal)
        signal.signal(signal.SIGTERM, runner.signal)
        try:
            runner.run()
            return 0
        except Refused as error:
            print(f"REFUSED: {error}", file=sys.stderr)
            return 2
        except RestoreFailed as error:
            print(error, file=sys.stderr)
            return 3
        except Interrupted as error:
            prefix, state = outcome(error)
            print(f"{error}: {prefix}{state}", file=sys.stderr)
            return 128 + error.signum
        except CheckFailed as error:
            prefix, state = outcome(error)
            print(f"FAILED: {prefix}{error}; {state}", file=sys.stderr)
            return 1
    finally:
        shutil.rmtree(work, ignore_errors=True)


if __name__ == "__main__":
    sys.exit(main())
