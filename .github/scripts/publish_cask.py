"""Future release-job entry point; importing this module never publishes."""

import base64
import json
import os
from pathlib import Path
import re
import sys
import urllib.error
import urllib.request


ENDPOINT = "https://api.github.com/repos/whiteducksoftware/homebrew-tap/contents/Casks/azctx.rb"
ORIGINAL = Path(".release-check/original/azctx.rb")
NORMALIZED = Path(".release-check/cask/azctx.rb")
# Stanzas native style may reorder or reformat but must never change.
IMMUTABLE = ("version", "url", "sha256", "binary", "depends_on")
TOKEN = re.compile(r'"(?:[^"\\]|\\.)*"|\w+:|:\w+')
STRING = re.compile(r'"(?:[^"\\]|\\.)*"')


def cask_semantics(cask):
    """Return sorted (enclosing blocks, stanza, tokens) for every immutable stanza."""
    blocks, statements, pending = [], [], ""
    for line in cask.splitlines():
        line = line.strip()
        if not line or line.startswith("#"):
            continue
        pending = f"{pending} {line}".strip()
        unquoted = STRING.sub('""', pending)
        if unquoted.endswith(",") or unquoted.count("[") + unquoted.count("(") > unquoted.count("]") + unquoted.count(")"):
            continue
        statement, pending = pending, ""
        stanza, _, arguments = statement.partition(" ")
        if statement == "end":
            if not blocks:
                raise ValueError("unbalanced cask block")
            blocks.pop()
        elif unquoted.endswith(" do"):
            blocks.append(stanza)
        elif stanza in IMMUTABLE:
            # Order, whitespace, trailing commas and one-element arrays are formatting only.
            statements.append((tuple(blocks), stanza, tuple(TOKEN.findall(arguments))))
    if pending or blocks:
        raise ValueError("unbalanced cask block")
    return sorted(statements)


def verify_normalized(original, normalized):
    """Reject native normalization that changed a Linux-generated immutable stanza."""
    expected = cask_semantics(original)
    if {stanza for _, stanza, _ in expected} != set(IMMUTABLE):
        raise ValueError("original Linux cask lacks comparable immutable stanzas")
    if cask_semantics(normalized) != expected:
        raise ValueError("normalized cask changed version/url/sha256/binary/depends_on semantics")


def read_casks():
    return ORIGINAL.read_text(encoding="utf-8"), NORMALIZED.read_text(encoding="utf-8")


def payload(cask, tag, previous):
    """Build a contents-API request, never a tap migration or upstream PR."""
    if not re.fullmatch(r"v?[0-9]+\.[0-9]+\.[0-9]+", tag):
        raise ValueError("only stable version tags can publish a cask")
    version = re.search(r'^\s*version "([^"]+)"$', cask, re.MULTILINE)
    if (
        'cask "azctx" do' not in cask
        or "postflight_steps do" not in cask
        or not version
        or version[1] != tag.removeprefix("v")
    ):
        raise ValueError("expected the style-checked azctx cask")
    result = {
        "message": (
            f"build: update azctx cask for {tag}\n\n"
            "Co-authored-by: Copilot <223556219+Copilot@users.noreply.github.com>"
        ),
        "content": base64.b64encode(cask.encode()).decode("ascii"),
        "branch": "main",
    }
    if previous:
        result["sha"] = previous["sha"]
    return result


def main(argv):
    if argv not in ([], ["--check"]):
        sys.exit("usage: publish_cask.py [--check]")
    original, cask = read_casks()
    # Validate the tag and artifacts before any credential use or network request.
    verify_normalized(original, cask)
    request_payload = payload(cask, os.environ["RELEASE_TAG"], None)
    if argv == ["--check"]:
        print("PASS: normalized cask keeps the original Linux cask's immutable semantics")
        return
    token = os.environ["GH_TOKEN"]
    headers = {
        "Authorization": f"Bearer {token}",
        "Accept": "application/vnd.github+json",
        "X-GitHub-Api-Version": "2026-03-10",
    }
    try:
        with urllib.request.urlopen(
            urllib.request.Request(f"{ENDPOINT}?ref=main", headers=headers), timeout=30
        ) as response:
            previous = json.load(response)
    except urllib.error.HTTPError as error:
        error.close()
        if error.code != 404:
            raise RuntimeError(f"tap lookup failed (HTTP {error.code})") from None
        previous = None
    if previous:
        if base64.b64decode(previous["content"]).decode("utf-8") == cask:
            print("Cask already current")
            return
        request_payload["sha"] = previous["sha"]
    headers["Content-Type"] = "application/json"
    try:
        with urllib.request.urlopen(
            urllib.request.Request(
                ENDPOINT, data=json.dumps(request_payload).encode(), headers=headers, method="PUT"
            ),
            timeout=30,
        ) as response:
            if response.status not in (200, 201):
                raise RuntimeError(f"tap update failed (HTTP {response.status})")
    except urllib.error.HTTPError as error:
        error.close()
        raise RuntimeError(f"tap update failed (HTTP {error.code})") from None
    print("Published style-checked Casks/azctx.rb")


if __name__ == "__main__":
    main(sys.argv[1:])
