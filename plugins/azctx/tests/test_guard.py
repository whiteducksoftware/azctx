"""Offline tests: never execute the proposed command or contact Azure."""

import contextlib
import importlib.util
import io
import json
import os
from pathlib import Path
import subprocess
import unittest
from unittest.mock import patch
import uuid


PLUGIN = Path(__file__).resolve().parents[1]


class GuardTests(unittest.TestCase):
    def setUp(self):
        spec = importlib.util.spec_from_file_location(
            "azctx_guard", PLUGIN / "hooks" / "guard.py"
        )
        self.hook = importlib.util.module_from_spec(spec)
        spec.loader.exec_module(self.hook)
        self.root = Path.cwd() / ".test-work" / str(uuid.uuid4())
        self.cwd = self.root / "repo" / "nested"
        self.cwd.mkdir(parents=True)
        self.addCleanup(self.cleanup)
        (self.root / ".azctx.yaml").write_text("deliberately not parsed by hook\n")
        self.which = patch.object(self.hook.shutil, "which", return_value="/fake/azctx")
        self.run = patch.object(
            self.hook.subprocess,
            "run",
            return_value=subprocess.CompletedProcess([], 0),
        )
        self.which_mock = self.which.start()
        self.run_mock = self.run.start()
        self.addCleanup(self.which.stop)
        self.addCleanup(self.run.stop)

    def cleanup(self):
        import shutil

        shutil.rmtree(self.root)
        with contextlib.suppress(OSError):
            self.root.parent.rmdir()

    def invoke(self, command="az group list", raw=None, **fields):
        payload = {
            "hook_event_name": "PreToolUse",
            "tool_name": "Bash",
            "tool_input": {"command": command},
            "cwd": str(self.cwd),
            # The hook must not open the transcript, even if it is present.
            "transcript_path": str(self.root / "never-read"),
        }
        payload.update(fields)
        stdout, stderr = io.StringIO(), io.StringIO()
        with (
            patch("sys.stdin", io.StringIO(raw if raw is not None else json.dumps(payload))),
            contextlib.redirect_stdout(stdout),
            contextlib.redirect_stderr(stderr),
        ):
            status = self.hook.main()
        self.assertEqual(stdout.getvalue(), "")
        return status, stderr.getvalue()

    def test_watched_commands_are_guarded_without_evaluating_input(self):
        for command in (
            "az group list",
            " \tazd up",
            "terraform apply",
            "bicep build main.bicep",
            "az",
            "'az' account show",
            "az; touch NEVER",
            "az group list\nterraform apply",
            "az group list --name '$(touch NEVER)'",
            "az group list --subscription OTHER",
        ):
            with self.subTest(command=command):
                self.assertEqual(self.invoke(command), (0, ""))
                args, kwargs = self.run_mock.call_args
                self.assertEqual(args, (["/fake/azctx", "guard", "--no-input"],))
                self.assertEqual(Path(kwargs["cwd"]), self.cwd)
                self.assertEqual(kwargs["stdin"], subprocess.DEVNULL)
                self.assertEqual(kwargs["stdout"], subprocess.DEVNULL)
                self.assertEqual(kwargs["stderr"], subprocess.DEVNULL)
                self.assertNotIn("shell", kwargs)
                self.assertEqual(kwargs["timeout"], 10)
                self.assertEqual(kwargs["env"]["AZCTX_NO_INPUT"], "1")
                self.assertEqual(kwargs["env"]["AZCTX_NO_UPDATE_CHECK"], "1")
                self.assertFalse((self.cwd / "NEVER").exists())

    def test_unrelated_commands_do_not_run_guard(self):
        for command in (
            "echo az",
            "azctx guard",
            "azure --help",
            "azd-not-a-command",
            "terraformer",
            "bicep-tools",
            "git status",
            "env FOO=bar az group list",
            "cd other && az group list",
            "bash -c 'az group list'",
            "/custom/bin/az group list",
        ):
            with self.subTest(command=command):
                self.assertEqual(self.invoke(command), (0, ""))
        self.run_mock.assert_not_called()

    def test_every_guard_failure_blocks_with_sanitized_reason(self):
        for status in (1, 2, 4, 37, 130, -15):
            with self.subTest(status=status):
                self.run_mock.return_value = subprocess.CompletedProcess(
                    [], status, stdout="never echo stdout", stderr="never echo secrets"
                )
                code, reason = self.invoke()
                self.assertEqual(code, 2)
                self.assertIn(str(status), reason)
                self.assertIn("blocked", reason)
                self.assertNotIn("never echo", reason)
                self.assertEqual(len(reason.splitlines()), 1)

    def test_missing_azctx_allows_with_one_notice(self):
        self.which_mock.return_value = None
        code, notice = self.invoke()
        self.assertEqual(code, 0)
        self.assertIn("azctx not found", notice)
        self.assertEqual(len(notice.splitlines()), 1)
        self.run_mock.assert_not_called()

    def test_missing_project_allows_with_one_notice(self):
        (self.root / ".azctx.yaml").unlink()
        with patch.object(self.hook, "has_project", return_value=False):
            code, notice = self.invoke()
        self.assertEqual(code, 0)
        self.assertIn("no .azctx.yaml", notice)
        self.assertEqual(len(notice.splitlines()), 1)
        self.run_mock.assert_not_called()

    def test_nearest_project_is_not_parsed_or_skipped(self):
        near = self.cwd / ".azctx.yaml"
        for kind in ("malformed", "directory"):
            with self.subTest(kind=kind):
                if kind == "malformed":
                    near.write_text("login: $(touch NEVER)\n")
                else:
                    near.mkdir()
                self.assertTrue(self.hook.has_project(self.cwd))
                self.assertEqual(self.invoke(), (0, ""))
                if near.is_dir():
                    near.rmdir()
                else:
                    near.unlink()

    def test_project_presence_uses_lstat_without_following_symlinks(self):
        with (
            patch.object(Path, "lstat", return_value=object()) as inspect,
            patch.object(Path, "stat", side_effect=AssertionError("must not follow links")),
        ):
            self.assertTrue(self.hook.has_project(self.cwd))
        inspect.assert_called_once()

    def test_project_search_includes_ancestors_and_stops_at_root(self):
        self.assertTrue(self.hook.has_project(self.cwd))
        (self.root / ".azctx.yaml").unlink()
        with patch.object(Path, "lstat", side_effect=FileNotFoundError):
            self.assertFalse(self.hook.has_project(self.cwd))

    def test_malformed_payload_blocks_without_running_anything(self):
        for raw in (
            "",
            "{",
            "null",
            "[]",
            "{}",
        ):
            with self.subTest(raw=raw):
                code, reason = self.invoke(raw=raw)
                self.assertEqual(code, 2)
                self.assertIn("blocked", reason)
                self.assertNotIn("Traceback", reason)
        self.run_mock.assert_not_called()

    def test_invalid_required_fields_block_without_running_anything(self):
        for fields in (
            {"hook_event_name": None},
            {"tool_name": None},
            {"tool_input": None},
            {"tool_input": {"command": 1}},
            {"tool_input": {"command": ""}},
            {"tool_input": {"command": " \t"}},
            {"tool_input": {"command": "az\0"}},
            {"tool_input": {"command": "'az"}},
            {"cwd": 1},
            {"cwd": ""},
            {"cwd": "relative"},
            {"cwd": str(self.cwd / "missing")},
            {"cwd": str(self.root / ".azctx.yaml")},
            {"cwd": "\0"},
        ):
            with self.subTest(fields=fields):
                code, reason = self.invoke(**fields)
                self.assertEqual(code, 2)
                self.assertIn("blocked", reason)
                self.assertNotIn("Traceback", reason)
        self.run_mock.assert_not_called()

    def test_wrong_event_and_tool_are_ignored(self):
        self.assertEqual(self.invoke(tool_name="Read"), (0, ""))
        self.assertEqual(self.invoke(hook_event_name="PostToolUse"), (0, ""))
        self.run_mock.assert_not_called()

    def test_excessive_json_nesting_blocks_without_traceback(self):
        code, reason = self.invoke(raw="[" * 2000 + "0" + "]" * 2000)
        self.assertEqual(code, 2)
        self.assertNotIn("Traceback", reason)
        # Older supported Python JSON decoders raise instead of parsing deeply.
        with patch.object(self.hook.json, "load", side_effect=RecursionError):
            code, reason = self.invoke()
        self.assertEqual(code, 2)
        self.assertNotIn("Traceback", reason)
        self.run_mock.assert_not_called()

    def test_timeouts_and_execution_errors_block(self):
        for error in (
            subprocess.TimeoutExpired("guard", 10),
            FileNotFoundError("removed after PATH lookup"),
            PermissionError("not executable"),
        ):
            with self.subTest(error=error):
                self.run_mock.side_effect = error
                code, reason = self.invoke()
                self.assertEqual(code, 2)
                self.assertIn("blocked", reason)
                self.assertNotIn(str(error), reason)

    def test_unreadable_project_search_blocks(self):
        with patch.object(self.hook, "has_project", side_effect=PermissionError):
            self.assertEqual(self.invoke()[0], 2)
        self.run_mock.assert_not_called()

    def test_parent_environment_is_not_modified(self):
        with patch.dict(os.environ, {"AZCTX_NO_INPUT": "0"}):
            self.assertEqual(self.invoke(), (0, ""))
            self.assertEqual(os.environ["AZCTX_NO_INPUT"], "0")


if __name__ == "__main__":
    unittest.main()
