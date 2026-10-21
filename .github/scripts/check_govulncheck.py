"""Enforce reachable findings, with the exact x/text baseline pending #86."""

import json
import sys


# Remove with the dependency upgrade in #86 (layer 05). New advisories, other
# modules and other versions must never inherit this temporary exception.
TEMPORARY_FINDINGS = {
    ("GO-2026-5970", "golang.org/x/text", "v0.21.0"),
    ("GO-2026-6629", "golang.org/x/text", "v0.21.0"),
}


def messages(data):
    decoder = json.JSONDecoder()
    remaining = data.lstrip()
    while remaining:
        message, end = decoder.raw_decode(remaining)
        if not isinstance(message, dict) or len(message) != 1:
            raise ValueError("expected one govulncheck message per JSON object")
        yield message
        remaining = remaining[end:].lstrip()


def check(data, output):
    stream = iter(messages(data))
    config = next(stream, {}).get("config", {})
    if (
        config.get("protocol_version") != "v1.0.0"
        or config.get("scan_level") != "symbol"
        or config.get("scan_mode") != "source"
    ):
        raise ValueError("expected a govulncheck v1 symbol-level source scan")

    blocked = set()
    allowed = set()
    for message in stream:
        kind, value = next(iter(message.items()))
        if kind in {"progress", "osv", "SBOM"}:
            continue
        if kind != "finding" or not isinstance(value, dict):
            raise ValueError(f"unexpected govulncheck message: {kind}")
        trace = value.get("trace")
        if not isinstance(trace, list) or not trace or not isinstance(trace[0], dict):
            raise ValueError("finding has no valid vulnerable frame")
        frame = trace[0]
        advisory = value.get("osv")
        module = frame.get("module")
        if not advisory or not module:
            raise ValueError("finding is missing its advisory or module")
        if not frame.get("function"):
            continue  # Match govulncheck's default reachable-symbol exit policy.
        finding = (advisory, module, frame.get("version", ""))
        if finding in TEMPORARY_FINDINGS:
            allowed.add(finding)
        else:
            blocked.add(finding)

    for advisory, module, version in sorted(allowed):
        print(
            f"::warning title=Temporary exception pending #86::"
            f"{advisory} in {module}@{version} is reachable; upgrade in layer 05.",
            file=output,
        )
    for advisory, module, version in sorted(blocked):
        print(
            f"::error title=Reachable vulnerability::"
            f"{advisory} in {module}@{version}",
            file=output,
        )
    print(
        f"Reachable vulnerabilities: {len(blocked)} blocking, "
        f"{len(allowed)} temporary exceptions.",
        file=output,
    )
    return int(bool(blocked))


if __name__ == "__main__":
    try:
        sys.exit(check(sys.stdin.read(), sys.stdout))
    except (ValueError, TypeError, AttributeError) as error:
        print(f"::error title=Invalid govulncheck output::{error}", file=sys.stderr)
        sys.exit(1)
