import importlib.util
import io
import json
from pathlib import Path
import unittest
import uuid
import shutil
import subprocess
import sys
from contextlib import redirect_stderr, redirect_stdout
from unittest.mock import patch


ROOT = Path(__file__).resolve().parents[2]
SPEC = importlib.util.spec_from_file_location("check_tool_pins", Path(__file__).with_name("check_tool_pins.py"))
PINS = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(PINS)


class ToolPinTest(unittest.TestCase):
    def setUp(self):
        self.root = ROOT / ".test-work" / ("tool-pins-" + uuid.uuid4().hex)
        self.root.mkdir(parents=True)
        self.addCleanup(shutil.rmtree, self.root)
        self.inventory = {
            "version": 1,
            "tools": [{
                "name": "example",
                "repository": "owner/example",
                "version": "v1.2.3",
                "sources": [{"path": "ci.yaml", "template": "version: {version}", "count": 2}],
            }],
        }
        (self.root / "ci.yaml").write_text("version: v1.2.3\nversion: v1.2.3\n", encoding="utf-8")

    def test_offline_check_detects_drift_missing_sources_and_duplicate_pins(self):
        self.assertEqual(PINS.check(self.root, self.inventory), [])
        for source in ("version: v1.2.4\nversion: v1.2.3\n",
                       "version: v1.2.3-rc.1\nversion: v1.2.3\n",
                       "version: v1.2.3\n", "version: v1.2.3\n" * 3):
            with self.subTest(source=source):
                (self.root / "ci.yaml").write_text(source, encoding="utf-8")
                with self.assertRaisesRegex(ValueError, "example.*ci.yaml"):
                    PINS.check(self.root, self.inventory)
        (self.root / "ci.yaml").unlink()
        with self.assertRaises(OSError):
            PINS.check(self.root, self.inventory)

    def test_online_check_reports_new_stable_major_across_all_tag_pages(self):
        tags = [[{"name": "v3.0.0-rc.1"}, {"name": "v1.2.3"}, {"name": "v3"}],
                [{"name": "v2.0.0"}, {"name": "unrelated-tag"}]]
        with patch("subprocess.run", return_value=subprocess.CompletedProcess([], 0, json.dumps(tags))) as gh:
            updates = PINS.check(self.root, self.inventory, online=True)
        self.assertEqual(updates, [
            "UPDATE: example v1.2.3 -> v2.0.0 (https://github.com/owner/example/tree/v2.0.0)"
        ])
        gh.assert_called_once_with(
            ["gh", "api", "--paginate", "--slurp", "repos/owner/example/tags?per_page=100"],
            check=True, capture_output=True, text=True, timeout=120,
        )
        with patch("subprocess.run", return_value=subprocess.CompletedProcess(
                [], 0, json.dumps([[{"name": "v1.2.3"}, {"name": "v2.0.0-beta.1"}]]))):
            self.assertEqual(PINS.check(self.root, self.inventory, online=True), [])

    def test_go_uses_official_stable_release_feed_not_prereleases_or_older_series(self):
        self.inventory["tools"] = [{
            "name": "go", "release_feed": "https://go.dev/dl/?mode=json", "version": "go1.27.3",
            "sources": [{"path": "go.mod", "template": "toolchain {version}", "count": 1}],
        }]
        (self.root / "go.mod").write_text("go 1.26.0\ntoolchain go1.27.3\n", encoding="utf-8")
        feed = [{"version": "go1.28rc1", "stable": False}, {"version": "go1.26.11", "stable": True},
                {"version": "go1.27.4", "stable": True}]
        with patch("urllib.request.urlopen", return_value=io.BytesIO(json.dumps(feed).encode())) as fetch:
            self.assertEqual(PINS.check(self.root, self.inventory, online=True), [
                "UPDATE: go go1.27.3 -> go1.27.4 (https://go.dev/dl/#go1.27.4)"
            ])
        fetch.assert_called_once_with("https://go.dev/dl/?mode=json", timeout=30)
        for feed in ("{}", '[{"version": 1, "stable": true}]', "[]"):
            with self.subTest(feed=feed), patch("urllib.request.urlopen", return_value=io.BytesIO(feed.encode())):
                code, stdout, stderr = self.run_cli("--online")
                self.assertEqual((code, stdout), (2, ""))
                self.assertIn("ERROR:", stderr)

    def run_cli(self, *args):
        inventory_file = self.root / "pins.json"
        inventory_file.write_text(json.dumps(self.inventory), encoding="utf-8")
        stdout, stderr = io.StringIO(), io.StringIO()
        with patch.object(PINS, "ROOT", self.root), patch.object(PINS, "INVENTORY", inventory_file), \
                patch.object(sys, "argv", ["check_tool_pins.py", *args]), \
                redirect_stdout(stdout), redirect_stderr(stderr):
            code = PINS.main()
        return code, stdout.getvalue(), stderr.getvalue()

    def test_cli_reports_current_updates_and_fail_closed_errors_distinctly(self):
        with patch("subprocess.run") as gh, patch("urllib.request.urlopen") as fetch:
            code, stdout, stderr = self.run_cli()
            self.assertEqual((code, stderr), (0, ""))
            self.assertIn("(offline)", stdout)
            gh.assert_not_called()
            fetch.assert_not_called()
        with patch("subprocess.run", return_value=subprocess.CompletedProcess(
                [], 0, json.dumps([[{"name": "v1.2.3"}, {"name": "v2.0.0"}]]))):
            code, stdout, stderr = self.run_cli("--online")
            self.assertEqual((code, stderr), (1, ""))
            self.assertIn("UPDATE: example", stdout)
            self.assertNotIn("PASS", stdout)
        for result in ("not json", "{}", '[["not a tag"]]', "[]", '[[{"name": "v2.0.0"}]]'):
            with self.subTest(result=result), patch("subprocess.run", return_value=subprocess.CompletedProcess(
                    [], 0, result)):
                code, stdout, stderr = self.run_cli("--online")
                self.assertEqual((code, stdout), (2, ""))
                self.assertIn("ERROR:", stderr)
        for error in (subprocess.CalledProcessError(1, ["gh", "api"]),
                      subprocess.TimeoutExpired(["gh", "api"], 120), OSError("network unavailable")):
            with self.subTest(error=error), patch("subprocess.run", side_effect=error):
                code, stdout, stderr = self.run_cli("--online")
                self.assertEqual((code, stdout), (2, ""))
                self.assertIn("ERROR:", stderr)


if __name__ == "__main__":
    unittest.main()
