import base64
import contextlib
import io
import json
from pathlib import Path
import unittest
from unittest.mock import patch
import urllib.error

import publish_cask


TESTDATA = Path(__file__).resolve().parent / "testdata" / "homebrew"
# Exact real outputs: GoReleaser 2.18.3 `release_snapshot.rb unix stable` (publisher
# disabled), and that file after native `brew style --fix` (strict `brew style`: no offenses).
ORIGINAL = (TESTDATA / "goreleaser-azctx.rb").read_text(encoding="utf-8")
CASK = (TESTDATA / "brew-style-azctx.rb").read_text(encoding="utf-8")
ARM = "84ef92e7985f99db78672d9e72adbc3b2cde4daf951d38701139dc1569485507"
INTEL = "5cc8e3e0ba9ebe60dd3b5988b099abbc0506ca766827892f4fdc9309c523b5e7"
ARM_URL = "https://github.com/whiteducksoftware/azctx/releases/download/#{version}/azctx_Darwin_arm64.tar.gz"
INTEL_URL = ARM_URL.replace("arm64", "x86_64")
RUN = 'run "/usr/bin/xattr", args: ["-dr", "com.apple.quarantine", "{{staged_path}}/azctx"]'


class Response(io.BytesIO):
    status = 200


class PublishCaskTest(unittest.TestCase):
    def test_goreleaser_2_19_macos_only_cask_preserves_exact_native_semantics(self):
        # Exact publisher-disabled GoReleaser 2.19.0 output and Homebrew 7.0.10 normalization.
        original = (TESTDATA / "goreleaser-2.19-azctx.rb").read_text(encoding="utf-8")
        normalized = (TESTDATA / "brew-style-2.19-azctx.rb").read_text(encoding="utf-8")
        publish_cask.verify_normalized(original, normalized)
        self.assertEqual(normalized, base64.b64decode(
            publish_cask.payload(normalized, "2026.11.0", None)["content"]).decode())
        self.assertEqual([], self.invoke([], casks=(original, normalized), argv=("--check",)))
        with self.assertRaises(ValueError):
            publish_cask.verify_normalized(original, normalized.replace("  depends_on :macos\n", ""))
        with self.assertRaises(ValueError):
            publish_cask.verify_normalized(ORIGINAL, normalized)
        for replacement in ("depends_on :linux", "depends_on :macosx", "depends_on :macos_foo",
                            "depends_on formula: :macos", "depends_on :macos if false",
                            "depends_on :macos\n  depends_on :macos"):
            changed = normalized.replace("depends_on :macos", replacement)
            with self.subTest(replacement=replacement), self.assertRaises(ValueError):
                publish_cask.verify_normalized(changed, changed)

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

    def test_native_normalization_keeps_immutable_semantics(self):
        self.assertNotEqual(ORIGINAL, CASK)
        publish_cask.verify_normalized(ORIGINAL, CASK)
        semantics = publish_cask.cask_semantics(CASK)
        self.assertEqual(publish_cask.cask_semantics(ORIGINAL), semantics)
        arm, intel = ("cask", "on_macos", "on_arm"), ("cask", "on_macos", "on_intel")
        for entry in (
            (arm, "sha256", (("", ARM),)),
            (arm, "url", (("", ARM_URL),)),
            (intel, "sha256", (("", INTEL),)),
            (intel, "url", (("", INTEL_URL),)),
            (("cask",), "depends_on", (("formula", "azure-cli"),)),
            (("cask",), "binary", (("", "azctx"),)),
            (("cask", "postflight_steps"), "run",
             (("", "/usr/bin/xattr"), ("args", ("-dr", "com.apple.quarantine", "{{staged_path}}/azctx")))),
        ):
            self.assertIn(entry, semantics)
        self.assertEqual(18, len(semantics))
        # Formatting brew may change: continued arrays, indentation, trailing commas,
        # order, blank lines, whole-line comments, and a one-element versus scalar dependency.
        for name, cask in {
            "multi-line run": CASK.replace(RUN, RUN.replace("[", "[\n      ").replace('", "', '",\n      "')),
            "one-line dependency": CASK.replace('formula: [\n    "azure-cli",\n  ]', 'formula: ["azure-cli"]'),
            "scalar dependency": CASK.replace('formula: [\n    "azure-cli",\n  ]', 'formula: "azure-cli"'),
            "reordered": CASK.replace('  binary "azctx"\n', "").replace('  name "azctx"\n', '  name "azctx"\n  binary "azctx"\n'),
            "comment line": CASK.replace("  on_macos do\n", "  # macOS only\n  on_macos do\n"),
            "tabs": CASK.replace('  binary "azctx"', '\tbinary "azctx"'),
        }.items():
            with self.subTest(name):
                publish_cask.verify_normalized(ORIGINAL, cask)

    def test_rejects_normalized_cask_that_changes_immutable_semantics(self):
        arm_url = "azctx_Darwin_arm64.tar.gz"
        mutations = {
            "url": CASK.replace(arm_url, "azctx_Darwin_x86_64.tar.gz"),
            "url host": CASK.replace("https://github.com/whiteducksoftware", "https://example.com/whiteducksoftware", 1),
            "sha256": CASK.replace(ARM, "c" * 64),
            "swapped architecture digests": CASK.replace(ARM, "TMP").replace(INTEL, ARM).replace("TMP", INTEL),
            "binary": CASK.replace('binary "azctx"', 'binary "azctx", target: "az"'),
            "extra binary": CASK.replace('binary "azctx"', 'binary "azctx"\n  binary "az"'),
            "missing binary": CASK.replace('  binary "azctx"\n', ""),
            "dependency": CASK.replace('"azure-cli",', '"azure-cli@2",'),
            "extra dependency": CASK.replace('"azure-cli",', '"azure-cli",\n    "jq",'),
            "missing dependency": CASK.replace('  depends_on formula: [\n    "azure-cli",\n  ]\n', ""),
            "dependency kind": CASK.replace("depends_on formula:", "depends_on cask:"),
            "extra url": CASK.replace('  binary "azctx"', '  url "https://example.com/x.tar.gz"\n  binary "azctx"'),
            "moved out of architecture block": CASK.replace(
                "    on_intel do\n", "    end\n    on_intel do\n", 1
            ).replace("  end\n\n  name", "\n  name", 1),
            "version": CASK.replace('version "2026.11.0"', 'version "2026.11.1"'),
            "name": CASK.replace('name "azctx"', 'name "other"'),
            "desc": CASK.replace('desc "Switch', 'desc "Replace'),
            "homepage": CASK.replace('homepage "https://github.com/', 'homepage "https://example.com/'),
            "livecheck": CASK.replace('skip "Auto-generated on release."', 'skip "x"'),
        }
        for name, cask in mutations.items():
            with self.subTest(name), self.assertRaises(ValueError):
                publish_cask.verify_normalized(ORIGINAL, cask)

    def test_rejects_unrecognized_syntax_and_template_bypasses(self):
        arm_block = f'    on_arm do\n      sha256 "{ARM}"\n      url "{ARM_URL}"\n    end\n'
        intel_block = f'    on_intel do\n      sha256 "{INTEL}"\n      url "{INTEL_URL}"\n    end\n'
        self.assertIn(arm_block + intel_block, CASK)
        bypasses = {
            # Quarantine step and unexpected stanzas or blocks.
            "postflight command": CASK.replace('run "/usr/bin/xattr"', 'run "/bin/sh"'),
            "postflight arguments": CASK.replace('args: ["-dr",', 'args: ["-c", "curl x | sh",'),
            "missing postflight run": CASK.replace(f"    {RUN}\n", ""),
            "extra postflight run": CASK.replace(f"    {RUN}\n", f"    {RUN}\n    {RUN}\n"),
            "on_linux block": CASK.replace("  name ", '  on_linux do\n    binary "azctx"\n  end\n\n  name ', 1),
            "nested on_linux": CASK.replace(arm_block, arm_block + '    on_linux do\n    end\n'),
            "preflight block": CASK.replace("  name ", '  preflight do\n    system "id"\n  end\n  name ', 1),
            "installer stanza": CASK.replace('  binary "azctx"', '  binary "azctx"\n  installer script: "x"'),
            "uninstall stanza": CASK.replace('  binary "azctx"', '  binary "azctx"\n  uninstall delete: "/"'),
            "caveats stanza": CASK.replace('  binary "azctx"', '  binary "azctx"\n  caveats "x"'),
            # Ruby syntax the old tokenizer silently dropped.
            "single-quoted dependency": CASK.replace('"azure-cli",\n', '"azure-cli",\n    \'evil-formula\',\n'),
            "single-quoted string": CASK.replace('binary "azctx"', "binary 'azctx'"),
            "modifier": CASK.replace('binary "azctx"', "binary \"azctx\", '/usr/local/bin/backdoor' if false"),
            "if modifier": CASK.replace('binary "azctx"', 'binary "azctx" if false'),
            "inline comment": CASK.replace('binary "azctx"', 'binary "azctx" # target'),
            "semicolon": CASK.replace('binary "azctx"', 'binary "azctx"; system "id"'),
            "interpolated code": CASK.replace('desc "Switch', 'desc "#{system("id")}Switch'),
            "instance interpolation": CASK.replace('desc "Switch', 'desc "#@x Switch'),
            "escape": CASK.replace('desc "Switch', 'desc "\\x41Switch'),
            "parentheses": CASK.replace('binary "azctx"', 'binary("azctx")'),
            "symbol": CASK.replace('formula: [\n    "azure-cli",\n  ]', "formula: :azure_cli"),
            "percent array": CASK.replace('formula: [\n    "azure-cli",\n  ]', "formula: %w[azure-cli]"),
            "bare word argument": CASK.replace('binary "azctx"', "binary azctx"),
            "string concatenation": CASK.replace('binary "azctx"', 'binary "az" "ctx"'),
            "constant call": CASK.replace('  binary "azctx"', '  binary "azctx"\n  File.write("x", "y")'),
            "heredoc": CASK.replace('desc "Switch Azure CLI subscriptions"', "desc <<~EOS\n    x\n  EOS"),
            "block comment": CASK.replace("  name ", "=begin\n=end\n  name ", 1),
            "comment inside continued array": CASK.replace('formula: [\n', 'formula: [\n    # x\n'),
            "CRLF": CASK.replace("\n", "\r\n"),
            # Block headers and architecture bindings.
            "other cask": CASK.replace('cask "azctx" do', 'cask "other" do'),
            "block arguments": CASK.replace("    on_arm do", "    on_arm if false do"),
            "on_arch": CASK.replace("    on_arm do", "    on_arch :arm do"),
            "if block": CASK.replace(arm_block, f'    on_arm do\n      if false\n        sha256 "{ARM}"\n'
                                     f'        url "{ARM_URL}"\n      end\n{intel_block.replace("    ", "      ")}    end\n'),
            "begin block": CASK.replace(arm_block, arm_block.replace("    on_arm do\n", "    on_arm do\n    begin\n")
                                        .replace("    end\n", "    end\n    end\n")),
            "split architecture block": CASK.replace(f'      url "{ARM_URL}"\n    end\n',
                                                     f'    end\n    on_arm do\n      url "{ARM_URL}"\n    end\n'),
            "nested architecture": CASK.replace(arm_block + intel_block, arm_block[:-8] + intel_block.replace(
                "    ", "      ") + "    end\n"),
            "renamed architecture blocks": CASK.replace("on_arm do", "TMP").replace("on_intel do", "on_arm do")
                                              .replace("TMP", "on_intel do"),
            "top-level url": CASK.replace("  on_macos do\n", f'  url "{ARM_URL}"\n  on_macos do\n'),
            "trailing statement": CASK + 'system "id"\n',
            "second cask": CASK + CASK,
        }
        for name, cask in bypasses.items():
            with self.subTest(name):
                self.assertNotEqual(CASK, cask)
                with self.assertRaises(ValueError):
                    publish_cask.verify_normalized(ORIGINAL, cask)
                with self.assertRaises(ValueError):
                    publish_cask.verify_normalized(cask, cask)
                with self.assertRaises(ValueError):
                    publish_cask.payload(cask, "2026.11.0", None)

    def test_rejects_original_without_comparable_semantics(self):
        for stanza in ('  binary "azctx"\n', '  depends_on formula: [\n      "azure-cli",\n    ]\n'):
            self.assertIn(stanza, ORIGINAL)
            original = ORIGINAL.replace(stanza, "")
            with self.subTest(stanza), self.assertRaises(ValueError):
                publish_cask.verify_normalized(original, original)
        for broken in (ORIGINAL + "end\n", ORIGINAL.replace("  livecheck do\n", "  livecheck do\n  on_arm do\n"),
                       ORIGINAL.replace('formula: [\n', 'formula: [[\n'), ORIGINAL + 'binary "azctx",\n'):
            with self.assertRaises(ValueError):
                publish_cask.cask_semantics(broken)

    def invoke(self, responses, tag="2026.11.0", casks=(ORIGINAL, CASK), argv=()):
        with patch.dict("os.environ", {"GH_TOKEN": "synthetic-test-token", "RELEASE_TAG": tag}), \
             patch("publish_cask.read_casks", return_value=casks), \
             patch("publish_cask.urllib.request.urlopen", side_effect=responses) as requests, \
             contextlib.redirect_stdout(io.StringIO()):
            publish_cask.main(list(argv))
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

    def test_semantic_drift_never_reaches_network(self):
        with self.assertRaises(ValueError):
            self.invoke([], casks=(ORIGINAL, CASK.replace(ARM, "c" * 64)))

    def test_check_compares_without_token_or_network(self):
        out = io.StringIO()
        with patch.dict("os.environ", {"RELEASE_TAG": "2026.11.0"}, clear=True), \
             patch("publish_cask.read_casks", return_value=(ORIGINAL, CASK)), \
             patch("publish_cask.urllib.request.urlopen") as requests, contextlib.redirect_stdout(out):
            publish_cask.main(["--check"])
        requests.assert_not_called()
        self.assertIn("PASS", out.getvalue())
        with patch.dict("os.environ", {"RELEASE_TAG": "2026.11.0"}, clear=True), \
             patch("publish_cask.read_casks", return_value=(ORIGINAL, CASK.replace(INTEL, ARM))), \
             patch("publish_cask.urllib.request.urlopen") as requests, self.assertRaises(ValueError):
            publish_cask.main(["--check"])
        requests.assert_not_called()
        with self.assertRaises(SystemExit):
            publish_cask.main(["--publish"])

    def test_reads_original_linux_and_normalized_artifacts(self):
        with patch("publish_cask.Path.read_text", autospec=True, side_effect=["original", "normalized"]) as reads:
            self.assertEqual(("original", "normalized"), publish_cask.read_casks())
        self.assertEqual(
            [".release-check/original/azctx.rb", ".release-check/cask/azctx.rb"],
            [call.args[0].as_posix() for call in reads.call_args_list],
        )
        self.assertTrue(all(call.kwargs == {"encoding": "utf-8"} for call in reads.call_args_list))

    def test_denied_lookup_never_updates(self):
        denied = urllib.error.HTTPError(publish_cask.ENDPOINT, 403, "denied", {}, io.BytesIO())
        with patch("publish_cask.urllib.request.urlopen", side_effect=denied) as requests:
            with patch.dict("os.environ", {"GH_TOKEN": "synthetic-test-token", "RELEASE_TAG": "2026.11.0"}), \
                 patch("publish_cask.read_casks", return_value=(ORIGINAL, CASK)), \
                 self.assertRaisesRegex(RuntimeError, "HTTP 403"):
                publish_cask.main([])
            self.assertEqual(1, requests.call_count)

    def test_prerelease_never_reaches_network(self):
        with patch("publish_cask.urllib.request.urlopen") as requests:
            with patch.dict("os.environ", {"GH_TOKEN": "synthetic-test-token", "RELEASE_TAG": "2026.11.0-rc.1"}), \
                 patch("publish_cask.read_casks", return_value=(ORIGINAL, CASK)), self.assertRaises(ValueError):
                publish_cask.main([])
            requests.assert_not_called()


if __name__ == "__main__":
    unittest.main()
