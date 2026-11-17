"""Opt-in dedicated-tenant checks. Never inspect credentials or token caches."""

import argparse
import json
import os
import re
import subprocess
import sys
from pathlib import Path


GUID = re.compile(r"[0-9a-fA-F]{8}(?:-[0-9a-fA-F]{4}){3}-[0-9a-fA-F]{12}")
ZERO = "00000000-0000-0000-0000-000000000000"


def guid(value):
    return isinstance(value, str) and GUID.fullmatch(value) is not None and value != ZERO


def prerequisites():
    if os.environ.get("AZCTX_INTEGRATION_ENABLED") != "true":
        return None
    if os.environ.get("AZCTX_INTEGRATION_DEDICATED_TENANT") != "true":
        raise ValueError("dedicated non-production test tenant attestation is required")
    names = ("CLIENT_ID", "TENANT_ID", "SUBSCRIPTION_ID", "SECOND_SUBSCRIPTION_ID")
    values = {}
    for name in names:
        value = os.environ.get("AZCTX_TEST_" + name, "")
        if not guid(value):
            raise ValueError("AZCTX_TEST_" + name + " must be a nonzero GUID")
        values[name] = value.lower()
    if values["SUBSCRIPTION_ID"] == values["SECOND_SUBSCRIPTION_ID"]:
        raise ValueError("two distinct test subscriptions are required")
    return values


def schema(document):
    if not isinstance(document, dict) or type(document.get("schemaVersion")) is not int or document["schemaVersion"] != 1:
        raise ValueError("expected schemaVersion 1")


def subscription(document):
    if not isinstance(document, dict):
        raise ValueError("expected subscription object")
    for key in ("id", "tenantId"):
        if not guid(document.get(key)):
            raise ValueError("invalid subscription identity")
    for key in ("name", "tenantName", "tenantDomain", "state", "environment"):
        if not isinstance(document.get(key), str):
            raise ValueError("invalid subscription string field")
    for key in ("isDefault", "delegated"):
        if type(document.get(key)) is not bool:
            raise ValueError("invalid subscription boolean field")
    user = document.get("user")
    if not isinstance(user, dict) or any(not isinstance(user.get(key), str) for key in ("name", "type")):
        raise ValueError("invalid subscription user shape")
    return document


def subscriptions(document):
    schema(document)
    entries = document.get("subscriptions")
    if not isinstance(entries, list):
        raise ValueError("expected subscription array")
    seen = set()
    for entry in entries:
        subscription(entry)
        identity = (entry["environment"].lower(), entry["id"].lower())
        if identity in seen:
            raise ValueError("duplicate subscription identity")
        seen.add(identity)
    return entries


def invoke(binary, *args, code=0):
    result = subprocess.run(
        [binary, *args], stdin=subprocess.DEVNULL, capture_output=True,
        text=True, check=False, timeout=60,
    )
    if result.returncode != code:
        # Do not print captured identities, Azure diagnostics or command arguments.
        raise ValueError(f"azctx command returned {result.returncode}; expected {code}")
    if code == 0 and result.stderr:
        raise ValueError("unexpected azctx diagnostic output")
    return result


def current(binary):
    document = json.loads(invoke(binary, "-c", "-o", "json").stdout)
    schema(document)
    return subscription(document.get("subscription"))


def require_context(entry, identifier, tenant):
    if entry["id"].lower() != identifier or entry["tenantId"].lower() != tenant or entry["environment"] != "AzureCloud":
        raise ValueError("test subscription or dedicated tenant context does not match")


def check(binary, values):
    binary = str(Path(binary).resolve(strict=True))
    tenant = values["TENANT_ID"]
    identifiers = [values["SUBSCRIPTION_ID"], values["SECOND_SUBSCRIPTION_ID"]]
    entries = subscriptions(json.loads(invoke(binary, "list", "-o", "json").stdout))
    for identifier in identifiers:
        found = [entry for entry in entries if entry["id"].lower() == identifier]
        if len(found) != 1:
            raise ValueError("both configured test subscriptions must be visible")
        require_context(found[0], identifier, tenant)
    original = current(binary)
    if original["id"].lower() not in identifiers:
        raise ValueError("initial context must be one of the dedicated test subscriptions")
    require_context(original, original["id"].lower(), tenant)
    try:
        for identifier in identifiers:
            result = invoke(binary, "-o", "json", "--", identifier)
            if result.stdout:
                raise ValueError("switch must be quiet")
            require_context(current(binary), identifier, tenant)
            exports = invoke(binary, "env", "--shell", "bash").stdout
            expected = "".join(f"export {key}='{value}'\n" for key, value in (
                ("ARM_SUBSCRIPTION_ID", identifier), ("ARM_TENANT_ID", tenant),
                ("AZURE_SUBSCRIPTION_ID", identifier), ("AZURE_TENANT_ID", tenant),
            ))
            if exports != expected:
                raise ValueError("environment export does not match current context")
            if invoke(binary, "guard", "--subscription", identifier, "--tenant", tenant).stdout:
                raise ValueError("successful guard must be quiet")
            other = next(value for value in identifiers if value != identifier)
            mismatch = invoke(binary, "guard", "--subscription", other, "--tenant", tenant, "-o", "json", code=4)
            failure = json.loads(mismatch.stderr)
            if mismatch.stdout or failure.get("code") != "context_mismatch" or failure.get("exit_code") != 4:
                raise ValueError("guard mismatch contract changed")
            require_context(current(binary), identifier, tenant)
    finally:
        invoke(binary, "-o", "json", "--", original["id"])
        require_context(current(binary), original["id"].lower(), tenant)
    print("PASS: real CLI profile/schema, list, two switches, env, matching/mismatching guard, restoration")


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("operation", choices=("prerequisites", "check"))
    parser.add_argument("--binary", default="./azctx")
    args = parser.parse_args()
    values = prerequisites()
    if args.operation == "prerequisites":
        print("ready=" + ("true" if values else "false"))
        if values is None:
            print("SKIP: nightly Azure integration not enabled; dedicated tenant acceptance is unverified", file=sys.stderr)
        return
    if values is None:
        raise ValueError("live integration is not enabled")
    check(args.binary, values)


if __name__ == "__main__":
    try:
        main()
    except (ValueError, OSError, subprocess.TimeoutExpired) as error:
        print(f"integration failed: {error}", file=sys.stderr)
        sys.exit(1)
