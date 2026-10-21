#!/usr/bin/env python3
"""Point-in-time context check, not a shell parser or authorization boundary."""

import json
import os
from pathlib import Path
import shlex
import shutil
import subprocess
import sys


def has_project(cwd):
    """Detect presence only; azctx owns nearest-file selection and strict parsing."""
    for directory in (cwd, *cwd.parents):
        try:
            (directory / ".azctx.yaml").lstat()
            return True
        except FileNotFoundError:
            continue
    return False


def blocked(reason):
    print(f"azctx: command blocked: {reason}", file=sys.stderr)
    return 2


def main():
    try:
        payload = json.load(sys.stdin)
        if not isinstance(payload, dict):
            raise ValueError
        if not isinstance(payload.get("hook_event_name"), str) or not isinstance(
            payload.get("tool_name"), str
        ):
            raise ValueError
        if payload["hook_event_name"] != "PreToolUse" or payload["tool_name"] != "Bash":
            return 0
        tool_input = payload.get("tool_input")
        if not isinstance(tool_input, dict):
            raise ValueError
        command = tool_input.get("command")
        if not isinstance(command, str) or not command.strip() or "\0" in command:
            raise ValueError
        # Only the first literal word is inspected. Never evaluate command text.
        lexer = shlex.shlex(command, posix=True, punctuation_chars=True)
        lexer.whitespace_split = True
        if lexer.get_token() not in ("az", "azd", "terraform", "bicep"):
            return 0
        raw_cwd = payload.get("cwd")
        if not isinstance(raw_cwd, str) or "\0" in raw_cwd:
            raise ValueError
        cwd = Path(raw_cwd)
        if not cwd.is_absolute() or not cwd.is_dir():
            raise ValueError
    except (OSError, ValueError, RecursionError):
        return blocked("invalid hook input; inspect the command manually.")

    azctx = shutil.which("azctx")
    if azctx is None:
        print("azctx: azctx not found on PATH; context check skipped.", file=sys.stderr)
        return 0
    try:
        if not has_project(cwd):
            print("azctx: no .azctx.yaml in ancestors; context check skipped.", file=sys.stderr)
            return 0
        environment = os.environ.copy()
        environment.update(AZCTX_NO_INPUT="1", AZCTX_NO_UPDATE_CHECK="1", NO_COLOR="1")
        result = subprocess.run(
            [azctx, "guard", "--no-input"],
            cwd=cwd,
            env=environment,
            stdin=subprocess.DEVNULL,
            stdout=subprocess.DEVNULL,
            stderr=subprocess.DEVNULL,
            timeout=10,
            check=False,
        )
    except (OSError, subprocess.TimeoutExpired):
        return blocked("context check unavailable; run azctx guard manually.")
    if result.returncode != 0:
        return blocked(f"azctx guard failed (exit {result.returncode}); inspect context manually.")
    return 0


if __name__ == "__main__":
    sys.exit(main())
