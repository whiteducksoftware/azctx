"""Build and archive only the native validation binary, metadata and checksums."""

from datetime import datetime, timezone
import hashlib
import json
import os
from pathlib import Path
import re
import shutil
import subprocess
import sys
import tarfile


ROOT = Path(__file__).resolve().parents[2]
VERSION_PACKAGE = "github.com/whiteducksoftware/azctx/internal/version"


def build(root):
    revision = subprocess.check_output(
        ["git", "rev-parse", "HEAD"], cwd=root, text=True, timeout=30,
    ).strip()
    if not re.fullmatch(r"[0-9a-f]{40}", revision):
        raise ValueError("checkout HEAD must be a full commit SHA")
    if os.environ.get("GITHUB_SHA", revision) != revision:
        raise ValueError("checkout HEAD does not match GITHUB_SHA (the tested revision)")
    timestamp = int(subprocess.check_output(
        ["git", "show", "-s", "--format=%ct", "HEAD"], cwd=root, text=True, timeout=30,
    ).strip())
    build_date = datetime.fromtimestamp(timestamp, timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ")
    target = json.loads(subprocess.check_output(
        ["go", "env", "-json", "GOHOSTOS", "GOHOSTARCH", "GOVERSION"],
        cwd=root, text=True, timeout=120,
    ))
    goos, goarch = target["GOHOSTOS"], target["GOHOSTARCH"]
    if goos not in ("linux", "darwin", "windows") or goarch not in ("amd64", "arm64"):
        raise ValueError(f"unsupported native validation target: {goos}/{goarch}")
    version = "0.0.0-validation." + revision[:12]
    name = f"azctx-validation-{goos}-{goarch}-{revision}"
    destination = root / "dist/validation"
    destination.mkdir(parents=True, exist_ok=True)
    archive = destination / (name + ".tar.gz")
    archive.unlink(missing_ok=True)
    stage = root / "dist/validation-stage"
    stage.mkdir()
    try:
        binary = stage / ("azctx.exe" if goos == "windows" else "azctx")
        env = dict(os.environ, CGO_ENABLED="0", GOOS=goos, GOARCH=goarch, GOFLAGS="")
        flags = " ".join(
            f"-X {VERSION_PACKAGE}.{key}={value}"
            for key, value in (("Version", version), ("Commit", revision), ("BuildDate", build_date))
        )
        subprocess.run(
            ["go", "build", "-trimpath", "-ldflags", flags, "-o", str(binary), "."],
            cwd=root, env=env, check=True, timeout=600,
        )
        if not binary.is_file() or binary.stat().st_size == 0:
            raise RuntimeError(f"build did not produce the expected nonempty {binary.name}")
        expected_version = {
            "version": version, "commit": revision, "buildDate": build_date,
            "goVersion": target["GOVERSION"],
        }
        actual_version = json.loads(subprocess.check_output(
            [str(binary), "version", "-o", "json"], cwd=root, env=env, text=True, timeout=30,
        ))
        if actual_version != expected_version:
            raise RuntimeError("native binary version does not match the tested checkout/toolchain")
        metadata = stage / "metadata.json"
        metadata.write_text(json.dumps({
            "schemaVersion": 1, "kind": "validation", "revision": revision,
            "revisionSource": "checkout HEAD", "version": version, "buildDate": build_date,
            "goos": goos, "goarch": goarch, "binary": binary.name,
            "goVersion": target["GOVERSION"], "cgoEnabled": False, "trimpath": True,
        }, indent=2) + "\n", encoding="utf-8", newline="\n")
        checksums = stage / "SHA256SUMS"
        checksums.write_text("".join(
            f"{hashlib.sha256(path.read_bytes()).hexdigest()}  {path.name}\n"
            for path in (binary, metadata)
        ), encoding="ascii", newline="\n")
        with tarfile.open(archive, "w:gz") as bundle:
            for path in (binary, metadata, checksums):
                entry = bundle.gettarinfo(str(path), arcname=path.name)
                entry.mode = 0o755 if path == binary else 0o644
                entry.uid = entry.gid = 0
                entry.uname = entry.gname = ""
                entry.mtime = timestamp
                with path.open("rb") as source:
                    bundle.addfile(entry, source)
        print(f"Built {archive.relative_to(root).as_posix()} ({version}, checkout {revision})")
        if os.environ.get("GITHUB_OUTPUT"):
            with open(os.environ["GITHUB_OUTPUT"], "a", encoding="utf-8", newline="\n") as output:
                output.write(f"artifact-name={name}\narchive-path={archive.relative_to(root).as_posix()}\n")
        return archive
    finally:
        shutil.rmtree(stage)


if __name__ == "__main__":
    try:
        build(ROOT)
    except (OSError, ValueError, RuntimeError, subprocess.SubprocessError) as error:
        print(f"ERROR: validation artifact: {error}", file=sys.stderr)
        sys.exit(1)
