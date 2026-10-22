import contextlib
import hashlib
import io
import json
import unittest
from unittest.mock import patch
import urllib.error

import upload_release_assets as upload


ZIP = b"synthetic Windows archive"
SUMS = b"synthetic checksums"
FILES = {
    "azctx_Windows_x86_64.zip": ZIP,
    "azctx_2026.11.0_Windows_checksums.txt": SUMS,
}
PATHS = [f".release-check/windows/{name}" for name in FILES]


def digest(content):
    return "sha256:" + hashlib.sha256(content).hexdigest()


def asset(name, content, state="uploaded"):
    return {"name": name, "state": state, "digest": digest(content), "size": len(content)}


class Response(io.BytesIO):
    status = 200


def respond(value, status=200):
    response = Response(json.dumps(value).encode())
    response.status = status
    return response


class UploadReleaseAssetsTest(unittest.TestCase):
    def run_upload(self, existing, uploads=(), tag="2026.11.0", files=None):
        responses = [respond({"id": 42}), respond(existing), *uploads]
        out = io.StringIO()
        with patch.dict("os.environ", {"GH_TOKEN": "synthetic-test-token", "RELEASE_TAG": tag}, clear=True), \
             patch("upload_release_assets.read_files", return_value=dict(files or FILES)), \
             patch("upload_release_assets.urllib.request.urlopen", side_effect=responses) as requests, \
             contextlib.redirect_stdout(out):
            upload.main(PATHS)
        return [call.args[0] for call in requests.call_args_list], out.getvalue()

    def test_uploads_missing_assets_and_verifies_stored_digest(self):
        requests, out = self.run_upload(
            [asset("azctx_Linux_x86_64.tar.gz", b"linux")],
            [respond(asset(name, content), 201) for name, content in sorted(FILES.items())],
        )
        self.assertEqual(
            [
                "https://api.github.com/repos/whiteducksoftware/azctx/releases/tags/2026.11.0",
                "https://api.github.com/repos/whiteducksoftware/azctx/releases/42/assets?per_page=100",
                "https://uploads.github.com/repos/whiteducksoftware/azctx/releases/42/assets?name=azctx_2026.11.0_Windows_checksums.txt",
                "https://uploads.github.com/repos/whiteducksoftware/azctx/releases/42/assets?name=azctx_Windows_x86_64.zip",
            ],
            [request.full_url for request in requests],
        )
        self.assertEqual(["GET", "GET", "POST", "POST"], [request.get_method() for request in requests])
        self.assertEqual([SUMS, ZIP], [request.data for request in requests[2:]])
        self.assertTrue(all(request.get_header("Authorization") == "Bearer synthetic-test-token" for request in requests))
        self.assertEqual("application/zip", requests[3].get_header("Content-type"))
        self.assertIn("Uploaded azctx_Windows_x86_64.zip", out)

    def test_rerun_skips_identical_assets_without_upload(self):
        requests, out = self.run_upload([asset(name, content) for name, content in FILES.items()])
        self.assertEqual(["GET", "GET"], [request.get_method() for request in requests])
        self.assertIn("Skipped identical azctx_Windows_x86_64.zip", out)

    def test_partial_rerun_uploads_only_missing_assets(self):
        name = "azctx_Windows_x86_64.zip"
        requests, _ = self.run_upload([asset(name, ZIP)], [respond(asset("azctx_2026.11.0_Windows_checksums.txt", SUMS), 201)])
        self.assertEqual(3, len(requests))
        self.assertTrue(requests[2].full_url.endswith("name=azctx_2026.11.0_Windows_checksums.txt"))

    def test_divergent_or_unverifiable_existing_assets_fail_before_any_upload(self):
        name = "azctx_Windows_x86_64.zip"
        cases = {
            "different digest": asset(name, b"rebuilt archive"),
            "missing digest": {"name": name, "state": "uploaded", "digest": None},
            "non-sha256 digest": {"name": name, "state": "uploaded", "digest": "md5:" + "0" * 32},
            "incomplete upload": asset(name, ZIP, state="starter"),
        }
        for case, existing in cases.items():
            with self.subTest(case), self.assertRaisesRegex(RuntimeError, "refusing to overwrite"):
                self.run_upload([existing], [respond(asset("x", b""), 201)] * 2)

    def test_upload_that_is_not_stored_byte_identically_fails(self):
        for stored in (asset("azctx_2026.11.0_Windows_checksums.txt", b"other"),
                       asset("azctx_2026.11.0_Windows_checksums.txt", SUMS, state="starter"),
                       asset("renamed.txt", SUMS)):
            with self.subTest(stored=stored), self.assertRaisesRegex(RuntimeError, "expected SHA-256"):
                self.run_upload([asset("azctx_Windows_x86_64.zip", ZIP)], [respond(stored, 201)])

    def test_api_failures_are_never_ignored(self):
        lookup = urllib.error.HTTPError("url", 404, "missing", {}, io.BytesIO())
        with patch.dict("os.environ", {"GH_TOKEN": "synthetic-test-token", "RELEASE_TAG": "2026.11.0"}, clear=True), \
             patch("upload_release_assets.read_files", return_value=dict(FILES)), \
             patch("upload_release_assets.urllib.request.urlopen", side_effect=lookup) as requests, \
             self.assertRaisesRegex(RuntimeError, "HTTP 404"):
            upload.main(PATHS)
        self.assertEqual(1, requests.call_count)
        conflict = urllib.error.HTTPError("url", 422, "already_exists", {}, io.BytesIO())
        with self.assertRaisesRegex(RuntimeError, "HTTP 422"):
            self.run_upload([], [conflict])
        offline = urllib.error.URLError("offline")
        with self.assertRaisesRegex(RuntimeError, "transport failed"):
            self.run_upload([], [offline])

    def test_unbounded_asset_listing_fails_closed(self):
        many = [asset(f"azctx_extra_{index}.txt", b"x") for index in range(100)]
        with self.assertRaisesRegex(RuntimeError, "too many"):
            self.run_upload(many)

    def test_invalid_input_never_reaches_network(self):
        cases = [
            ({"RELEASE_TAG": "../main"}, PATHS),
            ({"RELEASE_TAG": "main"}, PATHS),
            ({"RELEASE_TAG": "2026.11.0"}, []),
            ({"RELEASE_TAG": "2026.11.0"}, [PATHS[0], "other/" + PATHS[0].rsplit("/", 1)[1]]),
            ({"RELEASE_TAG": "2026.11.0"}, [".release-check/windows/bad name.zip"]),
        ]
        for env, paths in cases:
            with self.subTest(env=env, paths=paths), \
                 patch.dict("os.environ", {"GH_TOKEN": "synthetic-test-token", **env}, clear=True), \
                 patch("upload_release_assets.Path.read_bytes", autospec=True, return_value=b"x"), \
                 patch("upload_release_assets.urllib.request.urlopen") as requests, self.assertRaises(ValueError):
                upload.main(paths)
            requests.assert_not_called()
        self.assertEqual("2026.11.0-rc.1", upload.validate_tag("2026.11.0-rc.1"))

    def test_reads_exact_files_and_fails_on_unmatched_glob(self):
        with patch("upload_release_assets.Path.read_bytes", autospec=True, side_effect=[ZIP, SUMS]) as reads:
            self.assertEqual(FILES, upload.read_files(PATHS))
        self.assertEqual(PATHS, [call.args[0].as_posix() for call in reads.call_args_list])
        with self.assertRaises(ValueError):
            upload.read_files([".release-check/windows/*.zip"])
        with patch("upload_release_assets.Path.read_bytes", autospec=True, side_effect=FileNotFoundError), \
             self.assertRaises(FileNotFoundError):
            upload.read_files([".release-check/windows/azctx_Windows_arm64.zip"])


if __name__ == "__main__":
    unittest.main()
