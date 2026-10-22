"""Verify the same-run Chocolatey package against the uploaded release archive; never pushes."""

import hashlib
import os
from pathlib import Path
import re
import sys
import urllib.error
import urllib.request
import xml.etree.ElementTree as ElementTree
import zipfile


RELEASES = "https://github.com/whiteducksoftware/azctx/releases/download"
# GoReleaser 2.18.3 embeds only amd64/386 archives; azctx builds amd64 and arm64.
ARCHIVE = "azctx_Windows_x86_64.zip"
INSTALL_SCRIPT = "tools/chocolateyinstall.ps1"
SOURCE_KEYS = {"url", "checksum", "checksumType", "url64bit", "checksum64", "checksumType64"}
ASSIGNMENT = re.compile(r"^\s*(\w+)\s*=\s*'([^']*)'\s*$", re.MULTILINE)


def stable_version(tag):
    if not re.fullmatch(r"v?[0-9]+\.[0-9]+\.[0-9]+", tag):
        raise ValueError("only stable version tags can publish a Chocolatey package")
    return tag.removeprefix("v")


def find_package(directory):
    packages = list(Path(directory).glob("azctx.*.nupkg"))
    if len(packages) != 1:
        raise ValueError("expected exactly one same-run azctx Chocolatey package")
    return packages[0]


def read_package(source):
    """Return ((id, version), embedded install-script sources) from a .nupkg."""
    with zipfile.ZipFile(source) as package:
        names = package.namelist()
        nuspecs = [name for name in names if "/" not in name and name.lower().endswith(".nuspec")]
        scripts = [name for name in names if name.lower() == INSTALL_SCRIPT]
        if nuspecs != ["azctx.nuspec"] or len(scripts) != 1:
            raise ValueError("expected one azctx.nuspec and one Chocolatey install script")
        metadata = ElementTree.fromstring(package.read(nuspecs[0])).find("{*}metadata")
        script = package.read(scripts[0]).decode("utf-8-sig")
    if metadata is None:
        raise ValueError("package nuspec has no metadata")
    sources = {}
    for key, value in ASSIGNMENT.findall(script):
        if key in SOURCE_KEYS:
            if key in sources:
                raise ValueError(f"duplicate Chocolatey {key}")
            sources[key] = value
    return (metadata.findtext("{*}id"), metadata.findtext("{*}version")), sources


def verify(source, tag, digest_of):
    """Require the embedded URL/SHA-256 to match the actual uploaded release archive."""
    version = stable_version(tag)
    identity, sources = read_package(source)
    if identity != ("azctx", version):
        raise ValueError("package identity/version does not match the release tag")
    url = f"{RELEASES}/{tag}/{ARCHIVE}"
    if (
        set(sources) != {"url64bit", "checksum64", "checksumType64"}
        or sources["url64bit"] != url
        or sources["checksumType64"] != "sha256"
        or not re.fullmatch(r"[0-9a-f]{64}", sources["checksum64"])
    ):
        raise ValueError("package must embed only this release's x86_64 archive URL and SHA-256")
    if digest_of(ARCHIVE, url) != sources["checksum64"]:
        raise ValueError("embedded checksum differs from the uploaded release archive")
    return url


def sha256_stream(stream):
    digest = hashlib.sha256()
    for chunk in iter(lambda: stream.read(1 << 20), b""):
        digest.update(chunk)
    return digest.hexdigest()


def download_digest(_name, url):
    """Hash the public asset at the exact URL Chocolatey users will download."""
    try:
        with urllib.request.urlopen(urllib.request.Request(url), timeout=60) as response:
            return sha256_stream(response)
    except urllib.error.HTTPError as error:
        error.close()
        raise RuntimeError(f"uploaded release archive download failed (HTTP {error.code})") from None
    except (urllib.error.URLError, OSError):
        raise RuntimeError("uploaded release archive download failed") from None


def local_digest(directory):
    def digest_of(name, _url):
        with Path(directory, name).open("rb") as stream:
            return sha256_stream(stream)

    return digest_of


def main(argv):
    if len(argv) == 1:
        digest_of = download_digest
    elif len(argv) == 3 and argv[1] == "--archives":
        # Publisher-disabled snapshot acceptance: compare with the same-run local archive.
        digest_of = local_digest(argv[2])
    else:
        sys.exit("usage: verify_chocolatey.py PACKAGE_DIR [--archives ARCHIVE_DIR]")
    tag = os.environ["RELEASE_TAG"]
    stable_version(tag)
    url = verify(find_package(argv[0]), tag, digest_of)
    print(f"PASS: Chocolatey package embeds the SHA-256 of {url}")


if __name__ == "__main__":
    main(sys.argv[1:])
