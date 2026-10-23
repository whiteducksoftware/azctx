"""Check real embedded tool pins; Dependabot owns go.mod and action references."""

import argparse
import json
from pathlib import Path
import re
import subprocess
import sys
import urllib.request


ROOT = Path(__file__).resolve().parents[2]
INVENTORY = ROOT / ".github/tool-pins.json"


def version_tuple(version):
    return tuple(map(int, re.sub(r"^(?:v|go)", "", version).split(".")))


def upstream_versions(tool):
    if "release_feed" in tool:
        if tool["name"] != "go" or tool["release_feed"] != "https://go.dev/dl/?mode=json":
            raise ValueError(f"{tool['name']}: unsupported release feed")
        with urllib.request.urlopen(tool["release_feed"], timeout=30) as response:
            releases = json.load(response)
        if not isinstance(releases, list) or not all(
                isinstance(release, dict) and isinstance(release.get("version"), str) and
                type(release.get("stable")) is bool for release in releases):
            raise ValueError("go: invalid upstream release response")
        return [release["version"] for release in releases
                if release.get("stable") is True and re.fullmatch(r"go\d+\.\d+\.\d+", release["version"])]
    repository = tool["repository"]
    if not re.fullmatch(r"[A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+", repository):
        raise ValueError(f"{tool['name']}: invalid upstream repository")
    result = subprocess.run(
        ["gh", "api", "--paginate", "--slurp", f"repos/{repository}/tags?per_page=100"],
        check=True, capture_output=True, text=True, timeout=120,
    )
    pages = json.loads(result.stdout)
    if not isinstance(pages, list) or not all(isinstance(page, list) for page in pages):
        raise ValueError(f"{tool['name']}: invalid upstream tag response")
    if not all(isinstance(tag, dict) and isinstance(tag.get("name"), str) for page in pages for tag in page):
        raise ValueError(f"{tool['name']}: invalid upstream tag entries")
    tags = [tag["name"] for page in pages for tag in page]
    return [tag for tag in tags if re.fullmatch(r"v\d+\.\d+\.\d+", tag)]


def check(root, inventory, online=False):
    if inventory.get("version") != 1 or not inventory.get("tools"):
        raise ValueError("expected tool pin inventory version 1 with real tools")
    names = set()
    for tool in inventory["tools"]:
        name, version = tool["name"], tool["version"]
        if name in names or not re.fullmatch(r"(?:v|go)?\d+\.\d+\.\d+", version):
            raise ValueError(f"{name}: duplicate tool or non-stable version")
        names.add(name)
        if not tool["sources"]:
            raise ValueError(f"{name}: missing real pin sources")
        for source in tool["sources"]:
            path = root / source["path"]
            if not path.resolve().is_relative_to(root.resolve()):
                raise ValueError(f"{name}: source must stay inside the repository")
            template, count = source["template"], source["count"]
            if template.count("{version}") != 1 or type(count) is not int or count < 1:
                raise ValueError(f"{name}: invalid source selector")
            pattern = re.escape(template).replace(re.escape("{version}"), r"([A-Za-z0-9.+-]+)")
            found = re.findall(pattern, path.read_text(encoding="utf-8"))
            if found != [version] * count:
                raise ValueError(f"{name}: {source['path']} expected {count} pins at {version}, found {found}")
    updates = []
    if online:
        for tool in inventory["tools"]:
            versions = upstream_versions(tool)
            if not versions:
                raise ValueError(f"{tool['name']}: no stable upstream versions")
            if "repository" in tool and tool["version"] not in versions:
                raise ValueError(f"{tool['name']}: pinned stable tag is missing upstream")
            newest = max(versions, key=version_tuple)
            if version_tuple(newest) > version_tuple(tool["version"]):
                url = (f"https://github.com/{tool['repository']}/tree/{newest}"
                       if "repository" in tool else f"https://go.dev/dl/#{newest}")
                updates.append(f"UPDATE: {tool['name']} {tool['version']} -> {newest} ({url})")
    return updates


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--online", action="store_true", help="query upstream tags and Go's release feed (read-only)")
    args = parser.parse_args()
    try:
        inventory = json.loads(INVENTORY.read_text(encoding="utf-8"))
        updates = check(ROOT, inventory, online=args.online)
    except (KeyError, OSError, ValueError, subprocess.SubprocessError) as error:
        print(f"ERROR: {error}", file=sys.stderr)
        return 2
    if updates:
        print("\n".join(updates))
        return 1
    scope = "current upstream stable tags" if args.online else "their real sources (offline)"
    print(f"PASS: {len(inventory['tools'])} embedded tool pins match {scope}")
    return 0


if __name__ == "__main__":
    sys.exit(main())
