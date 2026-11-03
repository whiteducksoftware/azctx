"""Future release-job entry point; importing this module never publishes."""

import base64
import json
import os
from pathlib import Path
import re
import urllib.error
import urllib.request


ENDPOINT = "https://api.github.com/repos/whiteducksoftware/homebrew-tap/contents/Casks/azctx.rb"


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


def main():
    token = os.environ["GH_TOKEN"]
    headers = {
        "Authorization": f"Bearer {token}",
        "Accept": "application/vnd.github+json",
        "X-GitHub-Api-Version": "2026-03-10",
    }
    cask = Path(".release-check/cask/azctx.rb").read_text(encoding="utf-8")
    # Validate the tag and artifact before any network request.
    request_payload = payload(cask, os.environ["RELEASE_TAG"], None)
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
    main()
