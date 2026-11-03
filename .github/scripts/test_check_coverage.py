"""Exercise the coverage gate through its command-line boundary."""

import subprocess
import sys
import unittest
from pathlib import Path


SCRIPT = Path(__file__).with_name("check_coverage.py")
INTERNAL = "github.com/whiteducksoftware/azctx/internal/"


class CoverageGateTests(unittest.TestCase):
    def gate(self, profile):
        return subprocess.run(
            [sys.executable, "-B", str(SCRIPT)],
            input=profile, text=True, capture_output=True, check=False,
        )

    def test_statement_weighting_not_package_average(self):
        result = self.gate(
            f"mode: atomic\n{INTERNAL}small/a.go:1.1,2.1 1 1\n"
            f"{INTERNAL}large/b.go:1.1,2.1 99 0\n"
        )
        self.assertEqual(result.returncode, 1, result.stderr)
        self.assertIn("1/100 statements (1.00%)", result.stdout)

    def test_exact_threshold_and_no_cmd_gate(self):
        result = self.gate(
            f"mode: atomic\n{INTERNAL}a.go:1.1,2.1 70 1\n"
            f"{INTERNAL}a.go:3.1,4.1 30 0\n"
            "github.com/whiteducksoftware/azctx/cmd/root.go:1.1,2.1 10000 0\n"
        )
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertIn("70/100 statements (70.00%)", result.stdout)

    def test_does_not_round_up_to_pass(self):
        result = self.gate(
            f"mode: count\n{INTERNAL}a.go:1.1,2.1 69999 4\n"
            f"{INTERNAL}a.go:3.1,4.1 30001 0\n"
        )
        self.assertEqual(result.returncode, 1, result.stderr)

    def test_cross_package_blocks_count_once_and_merge_hits(self):
        result = self.gate(
            f"mode: atomic\n{INTERNAL}a.go:1.1,2.1 70 0\n"
            f"{INTERNAL}a.go:3.1,4.1 30 0\n"
            f"{INTERNAL}a.go:1.1,2.1 70 3\n"
            f"{INTERNAL}a.go:3.1,4.1 30 0\n"
        )
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertIn("70/100 statements (70.00%)", result.stdout)

    def test_invalid_or_incomplete_profiles_fail_closed(self):
        for profile in (
            "", "mode: bogus\n", "mode: set\n",
            "mode: atomic\ninternal/a.go:1.1,2.1 100 1\n",
            f"mode: atomic\n{INTERNAL}a.go:1.1,2.1 -1 1\n",
            f"mode: atomic\n{INTERNAL}a.go:1.1,2.1 1 -1\n",
            f"mode: atomic\n{INTERNAL}a.go:1.1,2.1 0 1\n",
            f"mode: atomic\n{INTERNAL}a.go:1.1,2.1 1 1\nbroken\n",
            f"mode: atomic\n{INTERNAL}a.go:1.1,2.1 1 1\n"
            f"{INTERNAL}a.go:1.1,2.1 2 0\n",
            "mode: atomic\ngithub.com/whiteducksoftware/azctx/internalish/a.go:1.1,2.1 1 1\n",
        ):
            with self.subTest(profile=profile):
                result = self.gate(profile)
                self.assertNotEqual(result.returncode, 0)
                self.assertIn("invalid coverage", result.stderr)


if __name__ == "__main__":
    unittest.main()
