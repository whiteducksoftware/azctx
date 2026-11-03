"""Fail every reachable finding and invalid govulncheck scan."""

import json
import sys


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
        blocked.add(finding)

    for advisory, module, version in sorted(blocked):
        print(
            f"::error title=Reachable vulnerability::"
            f"{advisory} in {module}@{version}",
            file=output,
        )
    print(f"Reachable vulnerabilities: {len(blocked)} blocking.", file=output)
    return int(bool(blocked))


if __name__ == "__main__":
    try:
        sys.exit(check(sys.stdin.read(), sys.stdout))
    except (ValueError, TypeError, AttributeError) as error:
        print(f"::error title=Invalid govulncheck output::{error}", file=sys.stderr)
        sys.exit(1)
