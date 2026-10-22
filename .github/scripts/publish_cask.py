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
RELEASES = "https://github.com/whiteducksoftware/azctx/releases/download/#{version}"
# The only Ruby syntax that current GoReleaser casks and `brew style --fix` emit:
# whole-line comments, double-quoted strings whose only interpolation is
# #{version}, `label:` keywords, bare words and [ ] , punctuation. Anything else
# (comments after code, other quoting or interpolation, symbols, modifiers,
# calls, semicolons, parentheses) is rejected rather than skipped.
SYNTAX = re.compile(
    r"""(?P<newline>\n)|(?P<space>[ \t]+)|(?P<comment>\#[^\n]*)"""
    r"""|(?P<string>"(?:[^"\\\#\n]|\#\{version\})*")"""
    r"""|(?P<label>[a-z_][a-z0-9_]*:)(?![:a-z0-9_])"""
    r"""|(?P<word>[a-z_][a-z0-9_]*)(?![a-z0-9_:])|(?P<punctuation>[\[\],])"""
)
# The known azctx cask template: every block and stanza must occur exactly once.
BLOCKS = {
    (): ("cask",),
    ("cask",): ("on_macos", "livecheck", "postflight_steps"),
    ("cask", "on_macos"): ("on_arm", "on_intel"),
}
ARCHITECTURE = {"on_arm": "arm64", "on_intel": "x86_64"}
QUARANTINE = (("", "/usr/bin/xattr"), ("args", ("-dr", "com.apple.quarantine", "{{staged_path}}/azctx")))
STRING, ARRAY, DEPENDENCY = (str,), (tuple,), (str, tuple)
STANZAS = {
    ("cask",): {
        "version": (("", STRING),), "name": (("", STRING),), "desc": (("", STRING),),
        "homepage": (("", STRING),), "binary": (("", STRING),), "depends_on": (("formula", DEPENDENCY),),
    },
    ("cask", "livecheck"): {"skip": (("", STRING),)},
    ("cask", "postflight_steps"): {"run": (("", STRING), ("args", ARRAY))},
    **{("cask", "on_macos", arch): {"url": (("", STRING),), "sha256": (("", STRING),)} for arch in ARCHITECTURE},
}
TEMPLATE = sorted(
    [(path, "do", block) for path, blocks in BLOCKS.items() for block in blocks]
    + [(path, stanza) for path, stanzas in STANZAS.items() for stanza in stanzas]
)


def statements(cask):
    """Yield (line, tokens) per logical statement; open [ and trailing , continue it."""
    current, depth, position, line = [], 0, 0, 1
    while position <= len(cask):
        match = SYNTAX.match(cask, position) if position < len(cask) else None
        kind, text = (match.lastgroup, match.group()) if match else ("newline", "")
        if position < len(cask) and not match:
            raise ValueError(f"unsupported cask syntax on line {line}")
        position = match.end() if match else position + 1
        if kind == "newline":
            if current and depth == 0 and current[-1][1] != ",":
                yield current[0][2], current
                current = []
            line += 1
        elif kind == "comment":
            # Only whole-line comments between statements are inert; code before a
            # comment on its line, or a continued statement, is still pending.
            if current:
                raise ValueError(f"unsupported cask comment on line {line}")
        elif kind != "space":
            depth += {"[": 1, "]": -1}.get(text, 0) if kind == "punctuation" else 0
            if depth < 0:
                raise ValueError(f"unbalanced cask array on line {line}")
            current.append((kind, text, line))
    if current or depth:
        raise ValueError("unterminated cask statement")


def arguments(tokens, line):
    """Parse `"value", label: "value" | ["value", ...]` without evaluating Ruby."""
    texts = [text for _, text, _ in tokens] + [""]
    result, index = [], 0

    def fail():
        raise ValueError(f"unsupported cask arguments on line {line}")

    def string():
        nonlocal index
        if index >= len(tokens) or tokens[index][0] != "string":
            fail()
        index += 1
        return texts[index - 1][1:-1]

    while index < len(tokens):
        if result:
            if texts[index] != ",":
                fail()
            index += 1
        label = ""
        if index < len(tokens) and tokens[index][0] == "label":
            label, index = texts[index][:-1], index + 1
        if index >= len(tokens) or (not label and any(name for name, _ in result)):
            fail()
        if texts[index] != "[":
            result.append((label, string()))
            continue
        items, index = [], index + 1
        while texts[index] != "]":
            items.append(string())
            if texts[index] == ",":
                index += 1
            elif texts[index] != "]":
                fail()
        index += 1
        result.append((label, tuple(items)))
    return tuple(result)


def cask_semantics(cask):
    """Return the sorted known-template statements, rejecting any unrecognized syntax."""
    path, result = (), []
    for line, tokens in statements(cask):
        words = [text for _, text, _ in tokens]
        stanza = words[0]
        if tokens[0][0] != "word":
            raise ValueError(f"unsupported cask statement on line {line}")
        if words == ["end"]:
            if not path:
                raise ValueError(f"unbalanced cask block on line {line}")
            path = path[:-1]
        elif words[-1] == "do":
            header = ["cask", '"azctx"', "do"] if path == () else [stanza, "do"]
            if stanza not in BLOCKS.get(path, ()) or words != header:
                raise ValueError(f"unexpected cask block on line {line}")
            result.append((path, "do", stanza))
            path += (stanza,)
        else:
            signature = STANZAS.get(path, {}).get(stanza)
            values = arguments(tokens[1:], line)
            if (
                signature is None
                or len(values) != len(signature)
                or any(label != expected or not isinstance(value, kinds)
                       for (label, value), (expected, kinds) in zip(values, signature))
            ):
                raise ValueError(f"unexpected cask stanza on line {line}")
            if stanza == "depends_on" and isinstance(values[0][1], tuple) and len(values[0][1]) == 1:
                # A one-element array and a scalar are the same Homebrew dependency.
                values = (("formula", values[0][1][0]),)
            if stanza == "url" and values[0][1] != f"{RELEASES}/azctx_Darwin_{ARCHITECTURE[path[-1]]}.tar.gz":
                raise ValueError(f"cask url is not this architecture's release archive on line {line}")
            if stanza == "run" and values != QUARANTINE:
                raise ValueError(f"cask postflight is not the quarantine removal step on line {line}")
            if stanza == "sha256" and not re.fullmatch(r"[0-9a-f]{64}", values[0][1]):
                raise ValueError(f"cask sha256 is not a lowercase SHA-256 on line {line}")
            result.append((path, stanza, values))
    if path:
        raise ValueError("unbalanced cask block")
    if sorted(entry if entry[1] == "do" else entry[:2] for entry in result) != TEMPLATE:
        raise ValueError("cask does not contain exactly the known azctx template stanzas")
    return sorted(result)


def verify_normalized(original, normalized):
    """Reject native normalization that changed anything but formatting or stanza order."""
    if cask_semantics(normalized) != cask_semantics(original):
        raise ValueError("normalized cask changed the original Linux cask's semantics")


def read_casks():
    return ORIGINAL.read_text(encoding="utf-8"), NORMALIZED.read_text(encoding="utf-8")


def payload(cask, tag, previous):
    """Build a contents-API request, never a tap migration or upstream PR."""
    if not re.fullmatch(r"v?[0-9]+\.[0-9]+\.[0-9]+", tag):
        raise ValueError("only stable version tags can publish a cask")
    try:
        version = dict(((path, stanza), values) for path, stanza, values in cask_semantics(cask))[(("cask",), "version")]
    except ValueError:
        raise ValueError("expected the style-checked azctx cask") from None
    if version != (("", tag.removeprefix("v")),):
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
