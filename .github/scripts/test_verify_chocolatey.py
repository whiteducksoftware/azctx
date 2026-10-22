import contextlib
import hashlib
import io
from pathlib import Path
import unittest
from unittest.mock import patch
import urllib.error
import warnings
import zipfile

import verify_chocolatey


TESTDATA = Path(__file__).resolve().parent / "testdata" / "chocolatey"
# Exact GoReleaser 2.18.3 output for .goreleaser-windows.yaml (publisher-disabled
# snapshot, tag 2026.11.0), captured before choco pack. GENERATED is the SHA-256 of
# that run's azctx_Windows_x86_64.zip.
GENERATED_SCRIPT = (TESTDATA / "chocolateyinstall.ps1").read_bytes().decode("utf-8")
NUSPEC = (TESTDATA / "azctx.nuspec").read_bytes().decode("utf-8")
GENERATED = "a781cdee4695ec47e1dd91281ecb6fd5b063640747cbb0d2260d16679798cb2c"
ARCHIVE = b"synthetic uploaded Windows x86_64 archive"
DIGEST = hashlib.sha256(ARCHIVE).hexdigest()
URL = "https://github.com/whiteducksoftware/azctx/releases/download/2026.11.0/azctx_Windows_x86_64.zip"


def render(url=URL, checksum=DIGEST):
    """The real generated script with only its embedded URL and checksum replaced."""
    return GENERATED_SCRIPT.replace(URL, url).replace(GENERATED, checksum)


PROPERTIES = "package/services/metadata/core-properties/0123456789abcdef0123456789abcdef.psmdcp"


def package(version="2026.11.0", url=URL, checksum=DIGEST, script=None, entries=None):
    """Build the NuGet OPC layout that choco pack writes for GoReleaser's nuspec."""
    data = io.BytesIO()
    files = {
        "_rels/.rels": "<Relationships/>",
        "[Content_Types].xml": "<Types/>",
        "azctx.nuspec": NUSPEC.replace("<version>2026.11.0</version>", f"<version>{version}</version>"),
        "tools/chocolateyinstall.ps1": script if script is not None else render(url, checksum),
        PROPERTIES: "<coreProperties/>",
    }
    files.update(entries or {})
    with zipfile.ZipFile(data, "w") as archive:
        for name, content in files.items():
            if content is not None:
                archive.writestr(name, content)
    data.seek(0)
    return data


class Response(io.BytesIO):
    status = 200


class VerifyChocolateyTest(unittest.TestCase):
    def test_embedded_checksum_matches_actual_uploaded_archive(self):
        seen = []

        def digest_of(name, url):
            seen.append((name, url))
            return DIGEST

        self.assertEqual(URL, verify_chocolatey.verify(package(), "2026.11.0", digest_of))
        self.assertEqual([("azctx_Windows_x86_64.zip", URL)], seen)
        prefixed = URL.replace("/2026.11.0/", "/v2026.11.0/")
        self.assertEqual(prefixed, verify_chocolatey.verify(package(url=prefixed), "v2026.11.0", digest_of))

    def test_exact_real_generated_install_script_is_accepted(self):
        self.assertIn(f"'{URL}'", GENERATED_SCRIPT)
        self.assertEqual(GENERATED_SCRIPT, render(URL, GENERATED))
        self.assertEqual(URL, verify_chocolatey.verify(package(script=GENERATED_SCRIPT), "2026.11.0",
                                                       lambda name, url: GENERATED))
        with self.assertRaisesRegex(ValueError, "differs"):
            verify_chocolatey.verify(package(script=GENERATED_SCRIPT), "2026.11.0", lambda name, url: DIGEST)

    def test_rejects_unrecognized_install_script_sources(self):
        # Each variant can make Chocolatey download something other than the verified URL/hash.
        script = render(URL, DIGEST)
        other = URL.replace("whiteducksoftware/azctx", "attacker/azctx")
        hashtable_end = "    checksumType64 = 'sha256'\n}\n"
        bypasses = {
            "double-quoted override": script.replace(
                hashtable_end, f'    checksumType64 = \'sha256\'\n    url64bit = "{other}"\n}}\n'),
            "double-quoted source": script.replace(f"'{URL}'", f'"{URL}"'),
            "variable source": script.replace(f"'{URL}'", "$url"),
            "expression source": script.replace(f"'{URL}'", f"('{other}')"),
            "trailing comment": script.replace(f"'{URL}'", f"'{URL}' # pinned"),
            "post-hashtable mutation": script.replace(
                "\nInstall-ChocolateyZipPackage", f"\n$packageArgs.url64bit = '{other}'\nInstall-ChocolateyZipPackage"),
            "extra 32-bit source": script.replace(
                hashtable_end, f"{hashtable_end[:-2]}    url = '{other}'\n}}\n"),
            "extra command": script + f"Invoke-WebRequest '{other}'\n",
            "quote breakout": render(f"{URL}'; $u = '{other}", DIGEST),
            "different installer": script.replace("Install-ChocolateyZipPackage", "Install-ChocolateyPackage"),
            "unknown checksum type": script.replace("'sha256'", "'md5'"),
            "byte order mark": "\ufeff" + script,
            "CRLF": script.replace("\n", "\r\n"),
            "missing trailing newline": script.rstrip("\n"),
        }
        for name, candidate in bypasses.items():
            with self.subTest(name), self.assertRaisesRegex(ValueError, "known x86_64 Chocolatey template"):
                verify_chocolatey.verify(package(script=candidate), "2026.11.0", lambda name, url: DIGEST)
        with self.assertRaises(ValueError):
            verify_chocolatey.verify(package(script=b"\xff"), "2026.11.0", lambda name, url: DIGEST)

    def test_rejects_extra_packaged_scripts_and_payloads(self):
        bypasses = {
            "uninstall script": {"tools/chocolateyuninstall.ps1": "Remove-Item C:\\"},
            "before-modify script": {"tools/chocolateybeforemodify.ps1": "iex 'x'"},
            "helper module": {"tools/helper.psm1": "function x {}"},
            "helper script": {"tools/helper.ps1": "iex 'x'"},
            "embedded executable": {"tools/azctx.exe": "MZ"},
            "embedded archive": {"tools/azctx_Windows_x86_64.zip": "PK"},
            "embedded library": {"tools/x.dll": "MZ"},
            "case-variant install script": {"tools/ChocolateyInstall.ps1": "iex 'x'"},
            "backslash install script": {"tools\\chocolateyinstall.ps1": "iex 'x'"},
            "root install script": {"chocolateyinstall.ps1": "iex 'x'"},
            "legacy content": {"content/azctx.exe": "MZ"},
            "directory entry": {"tools/": ""},
            "second core properties": {PROPERTIES.replace("0123", "fedc"): "<coreProperties/>"},
            "missing core properties": {PROPERTIES: None},
            "missing relationships": {"_rels/.rels": None},
            "second nuspec": {"other.nuspec": NUSPEC},
            "missing script": {"tools/chocolateyinstall.ps1": None},
        }
        for name, entries in bypasses.items():
            with self.subTest(name), self.assertRaisesRegex(ValueError, "must contain only"):
                verify_chocolatey.verify(package(entries=entries), "2026.11.0", lambda name, url: DIGEST)
        duplicate = package()
        with zipfile.ZipFile(duplicate, "a") as archive, warnings.catch_warnings():
            warnings.simplefilter("ignore")
            archive.writestr("tools/chocolateyinstall.ps1", "iex 'x'\n")
        duplicate.seek(0)
        with self.assertRaisesRegex(ValueError, "must contain only"):
            verify_chocolatey.verify(duplicate, "2026.11.0", lambda name, url: DIGEST)

    def test_rejects_package_whose_checksum_differs_from_uploaded_archive(self):
        with self.assertRaisesRegex(ValueError, "differs from the uploaded release archive"):
            verify_chocolatey.verify(package(), "2026.11.0", lambda name, url: "0" * 64)

    def test_rejects_unexpected_package_identity_or_sources(self):
        arm = URL.replace("x86_64", "arm64")
        bad = {
            "version": package(version="2026.11.1"),
            "other tag url": package(url=URL.replace("2026.11.0", "2026.10.0")),
            "foreign host": package(url=URL.replace("github.com", "example.com")),
            "arm archive": package(url=arm),
            "checksum format": package(checksum=DIGEST.upper()),
            "short checksum": package(checksum=DIGEST[:63]),
            "checksum type": package(script=render(URL, DIGEST).replace("'sha256'", "'md5'")),
            "duplicate url": package(script=render(URL, DIGEST).replace(
                "    checksum64", f"    url64bit       = '{arm}'\n    checksum64")),
            "extra 32-bit url": package(script=render(URL, DIGEST).replace(
                "}\n", f"    url            = '{arm}'\n    checksum       = '{DIGEST}'\n    checksumType   = 'sha256'\n}}\n")),
            "missing checksum": package(script=render(URL, DIGEST).replace(
                f"    checksum64     = '{DIGEST}'\n", "")),
            "missing script": package(entries={"tools/chocolateyinstall.ps1": None}),
            "foreign id": package(entries={"azctx.nuspec": NUSPEC.replace("<id>azctx</id>", "<id>other</id>")}),
            "second nuspec": package(entries={"other.nuspec": NUSPEC}),
        }
        for name, candidate in bad.items():
            with self.subTest(name), self.assertRaises(ValueError):
                verify_chocolatey.verify(candidate, "2026.11.0", lambda name, url: DIGEST)

    def test_prerelease_and_unsafe_tags_never_download(self):
        for tag in ("2026.11.0-rc.1", "main", "../2026.11.0", ""):
            with self.subTest(tag=tag), patch("verify_chocolatey.urllib.request.urlopen") as requests, \
                 self.assertRaises(ValueError):
                verify_chocolatey.verify(package(), tag, verify_chocolatey.download_digest)
            requests.assert_not_called()

    def test_downloads_the_exact_embedded_release_url(self):
        with patch("verify_chocolatey.urllib.request.urlopen", return_value=Response(ARCHIVE)) as requests:
            self.assertEqual(URL, verify_chocolatey.verify(package(), "2026.11.0", verify_chocolatey.download_digest))
        self.assertEqual(URL, requests.call_args.args[0].full_url)
        self.assertNotIn("Authorization", requests.call_args.args[0].headers)

    def test_missing_uploaded_archive_fails_closed(self):
        missing = urllib.error.HTTPError(URL, 404, "not found", {}, io.BytesIO())
        with patch("verify_chocolatey.urllib.request.urlopen", side_effect=missing), \
             self.assertRaisesRegex(RuntimeError, "HTTP 404"):
            verify_chocolatey.verify(package(), "2026.11.0", verify_chocolatey.download_digest)
        offline = urllib.error.URLError("offline")
        with patch("verify_chocolatey.urllib.request.urlopen", side_effect=offline), \
             self.assertRaisesRegex(RuntimeError, "download failed"):
            verify_chocolatey.verify(package(), "2026.11.0", verify_chocolatey.download_digest)

    def test_main_verifies_one_same_run_package_before_push(self):
        out = io.StringIO()
        with patch.dict("os.environ", {"RELEASE_TAG": "2026.11.0"}, clear=True), \
             patch("verify_chocolatey.find_package", return_value=package()) as find, \
             patch("verify_chocolatey.urllib.request.urlopen", return_value=Response(ARCHIVE)), \
             contextlib.redirect_stdout(out):
            verify_chocolatey.main([".release-check/chocolatey"])
        find.assert_called_once_with(".release-check/chocolatey")
        self.assertIn("PASS", out.getvalue())

    def test_main_snapshot_mode_hashes_local_archive_without_network(self):
        out = io.StringIO()
        with patch.dict("os.environ", {"RELEASE_TAG": "2026.11.0"}, clear=True), \
             patch("verify_chocolatey.find_package", return_value=package()), \
             patch("verify_chocolatey.Path.open", autospec=True, return_value=io.BytesIO(ARCHIVE)) as opened, \
             patch("verify_chocolatey.urllib.request.urlopen") as requests, contextlib.redirect_stdout(out):
            verify_chocolatey.main([".release-check/windows-stable", "--archives", ".release-check/windows-stable"])
        requests.assert_not_called()
        self.assertEqual(".release-check/windows-stable/azctx_Windows_x86_64.zip", opened.call_args.args[0].as_posix())
        for argv in ([], ["a", "b"], ["a", "--archives"], ["a", "--other", "b"]):
            with self.subTest(argv=argv), self.assertRaises(SystemExit):
                verify_chocolatey.main(argv)

    def test_exactly_one_package_is_required(self):
        for found in ([], ["azctx.2026.11.0.nupkg", "azctx.2026.11.1.nupkg"]):
            with self.subTest(found=found), patch("verify_chocolatey.Path.glob", return_value=iter(found)), \
                 self.assertRaises(ValueError):
                verify_chocolatey.find_package(".release-check/chocolatey")


if __name__ == "__main__":
    unittest.main()
