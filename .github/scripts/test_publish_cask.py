import base64
import contextlib
import io
import json
import unittest
from unittest.mock import patch
import urllib.error

import publish_cask


CASK = '''cask "azctx" do
  version "2026.11.0"
  postflight_steps do
    run "/usr/bin/xattr", args: ["-dr", "com.apple.quarantine", "{{staged_path}}/azctx"]
  end
end
'''


class Response(io.BytesIO):
    status = 200


class PublishCaskTest(unittest.TestCase):
    def test_payload_preserves_content_and_compare_and_swap(self):
        result = publish_cask.payload(CASK, "v2026.11.0", {"sha": "previous-sha"})
        self.assertEqual("main", result["branch"])
        self.assertEqual("previous-sha", result["sha"])
        self.assertEqual(CASK, base64.b64decode(result["content"]).decode())
        self.assertIn(
            "Co-authored-by: Copilot <223556219+Copilot@users.noreply.github.com>",
            result["message"],
        )
        self.assertNotIn("sha", publish_cask.payload(CASK, "2026.11.0", None))

    def test_rejects_prerelease_snapshot_and_mismatched_artifact(self):
        for tag in ("2026.11.0-rc.1", "2026.11.0-SNAPSHOT-abcd", "main", "../main", ""):
            with self.subTest(tag=tag), self.assertRaises(ValueError):
                publish_cask.payload(CASK, tag, None)
        for cask in (CASK.replace("2026.11.0", "2026.11.1"), CASK.replace("postflight_steps", "postflight")):
            with self.assertRaises(ValueError):
                publish_cask.payload(cask, "2026.11.0", None)

    def invoke(self, responses, tag="2026.11.0"):
        with patch.dict("os.environ", {"GH_TOKEN": "synthetic-test-token", "RELEASE_TAG": tag}), \
             patch("publish_cask.Path.read_text", return_value=CASK), \
             patch("publish_cask.urllib.request.urlopen", side_effect=responses) as requests, \
             contextlib.redirect_stdout(io.StringIO()):
            publish_cask.main()
        return requests.call_args_list

    def test_new_file_targets_only_cask_on_main(self):
        missing = urllib.error.HTTPError(publish_cask.ENDPOINT, 404, "not found", {}, io.BytesIO())
        calls = self.invoke([missing, Response(b"{}")])
        self.assertEqual(2, len(calls))
        request = calls[1].args[0]
        self.assertEqual(publish_cask.ENDPOINT, request.full_url)
        self.assertEqual("PUT", request.method)
        result = json.loads(request.data)
        self.assertEqual("main", result["branch"])
        self.assertNotIn("sha", result)
        self.assertNotIn("synthetic-test-token", request.data.decode())

    def test_existing_file_is_updated_with_previous_sha(self):
        previous = {"sha": "existing-sha", "content": base64.b64encode(b"old cask").decode()}
        calls = self.invoke([Response(json.dumps(previous).encode()), Response(b"{}")])
        self.assertEqual("existing-sha", json.loads(calls[1].args[0].data)["sha"])

    def test_noop_does_not_create_commit(self):
        previous = {"sha": "existing-sha", "content": base64.b64encode(CASK.encode()).decode()}
        calls = self.invoke([Response(json.dumps(previous).encode())])
        self.assertEqual(1, len(calls))

    def test_denied_lookup_never_updates(self):
        denied = urllib.error.HTTPError(publish_cask.ENDPOINT, 403, "denied", {}, io.BytesIO())
        with patch("publish_cask.urllib.request.urlopen", side_effect=denied) as requests:
            with patch.dict("os.environ", {"GH_TOKEN": "synthetic-test-token", "RELEASE_TAG": "2026.11.0"}), \
                 patch("publish_cask.Path.read_text", return_value=CASK), \
                 self.assertRaisesRegex(RuntimeError, "HTTP 403"):
                publish_cask.main()
            self.assertEqual(1, requests.call_count)

    def test_prerelease_never_reaches_network(self):
        with patch("publish_cask.urllib.request.urlopen") as requests:
            with patch.dict("os.environ", {"GH_TOKEN": "synthetic-test-token", "RELEASE_TAG": "2026.11.0-rc.1"}), \
                 patch("publish_cask.Path.read_text", return_value=CASK), self.assertRaises(ValueError):
                publish_cask.main()
            requests.assert_not_called()


if __name__ == "__main__":
    unittest.main()
