"""Upload release assets idempotently by SHA-256; never overwrites or deletes an asset."""

import hashlib
import json
import mimetypes
import os
from pathlib import Path
import re
import sys
import urllib.error
import urllib.parse
import urllib.request


API = "https://api.github.com/repos/whiteducksoftware/azctx"
UPLOADS = "https://uploads.github.com/repos/whiteducksoftware/azctx"
PAGE = 100


def validate_tag(tag):
    if not re.fullmatch(r"v?[0-9]+\.[0-9]+\.[0-9]+(?:-[0-9A-Za-z.]+)?", tag):
        raise ValueError("expected a release version tag")
    return tag


def read_files(paths):
    """Read exact files; an unmatched shell glob stays literal and fails here."""
    if not paths:
        raise ValueError("expected release asset files")
    files = {}
    for path in map(Path, paths):
        if not re.fullmatch(r"[0-9A-Za-z][0-9A-Za-z._-]*", path.name) or path.name in files:
            raise ValueError(f"unsafe or duplicate release asset name: {path.name!r}")
        files[path.name] = path.read_bytes()
    return files


def sha256(content):
    return "sha256:" + hashlib.sha256(content).hexdigest()


def plan(files, assets):
    """Return names to upload; raise unless every existing same-name asset is byte-identical."""
    existing = {asset.get("name"): asset for asset in assets}
    conflicts = sorted(
        name for name in files
        if name in existing
        and (existing[name].get("state") != "uploaded" or existing[name].get("digest") != sha256(files[name]))
    )
    if conflicts:
        raise RuntimeError(
            "existing release assets differ, are incomplete or lack a SHA-256 digest; "
            f"refusing to overwrite: {', '.join(conflicts)}"
        )
    return sorted(name for name in files if name not in existing)


def request(token, method, url, data=None, content_type=None):
    headers = {
        "Authorization": f"Bearer {token}",
        "Accept": "application/vnd.github+json",
        "X-GitHub-Api-Version": "2026-03-10",
    }
    if content_type:
        headers["Content-Type"] = content_type
    try:
        with urllib.request.urlopen(
            urllib.request.Request(url, data=data, headers=headers, method=method), timeout=300
        ) as response:
            return json.load(response)
    except urllib.error.HTTPError as error:
        error.close()
        raise RuntimeError(f"GitHub release {method} failed (HTTP {error.code})") from None
    except (urllib.error.URLError, OSError):
        raise RuntimeError(f"GitHub release {method} transport failed") from None


def main(paths):
    tag = validate_tag(os.environ["RELEASE_TAG"])
    files = read_files(paths)
    token = os.environ["GH_TOKEN"]
    release = request(token, "GET", f"{API}/releases/tags/{urllib.parse.quote(tag, safe='')}")
    release_id = release.get("id")
    if not isinstance(release_id, int):
        raise RuntimeError("release lookup returned no release id")
    assets = request(token, "GET", f"{API}/releases/{release_id}/assets?per_page={PAGE}")
    if len(assets) >= PAGE:
        raise RuntimeError("too many release assets to verify on one page")
    uploads = plan(files, assets)
    for name in sorted(set(files) - set(uploads)):
        print(f"Skipped identical {name}")
    for name in uploads:
        stored = request(
            token,
            "POST",
            f"{UPLOADS}/releases/{release_id}/assets?name={urllib.parse.quote(name)}",
            files[name],
            mimetypes.guess_type(name)[0] or "application/octet-stream",
        )
        if (stored.get("name"), stored.get("state"), stored.get("digest")) != (name, "uploaded", sha256(files[name])):
            raise RuntimeError(f"uploaded {name} was not stored with the expected SHA-256")
        print(f"Uploaded {name}")


if __name__ == "__main__":
    main(sys.argv[1:])
