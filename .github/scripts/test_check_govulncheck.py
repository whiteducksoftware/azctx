import io
import json
import unittest

from check_govulncheck import check


CONFIG = {
    "config": {
        "protocol_version": "v1.0.0",
        "scan_level": "symbol",
        "scan_mode": "source",
    }
}


def finding(advisory="GO-2026-5970", module="golang.org/x/text", version="v0.21.0"):
    return {
        "finding": {
            "osv": advisory,
            "trace": [{"module": module, "version": version, "function": "Decode"}],
        }
    }


class VulnerabilityPolicyTests(unittest.TestCase):
    def run_check(self, *messages):
        output = io.StringIO()
        status = check("\n".join(json.dumps(m, indent=2) for m in messages), output)
        return status, output.getvalue()

    def test_clean_scan(self):
        status, output = self.run_check(CONFIG, {"progress": {"message": "Scanning"}})
        self.assertEqual(status, 0)
        self.assertIn("0 blocking, 0 temporary exceptions", output)

    def test_exact_baseline_warns(self):
        status, output = self.run_check(
            CONFIG, finding(), finding("GO-2026-6629"), finding()
        )
        self.assertEqual(status, 0)
        self.assertEqual(output.count("::warning"), 2)
        self.assertIn("2 temporary exceptions", output)

    def test_other_advisory_blocks(self):
        status, output = self.run_check(CONFIG, finding("GO-2099-9999"))
        self.assertEqual(status, 1)
        self.assertIn("::error", output)

    def test_other_module_blocks(self):
        status, _ = self.run_check(CONFIG, finding(module="stdlib"))
        self.assertEqual(status, 1)

    def test_reachable_go_2026_6809_is_never_waived(self):
        for version in ("v1.26.9", "v1.27.2"):
            with self.subTest(version=version):
                message = finding("GO-2026-6809", "stdlib", version)
                message["finding"]["trace"][0].update(
                    package="crypto/x509", receiver="Certificate", function="Verify"
                )
                status, output = self.run_check(CONFIG, message)
                self.assertEqual(status, 1)
                self.assertIn(f"GO-2026-6809 in stdlib@{version}", output)
                self.assertIn("::error", output)
                self.assertNotIn("::warning", output)

    def test_other_version_blocks(self):
        status, _ = self.run_check(CONFIG, finding(version="v0.22.0"))
        self.assertEqual(status, 1)

    def test_mixed_findings_still_block(self):
        status, output = self.run_check(CONFIG, finding(), finding(module="stdlib"))
        self.assertEqual(status, 1)
        self.assertIn("::warning", output)
        self.assertIn("::error", output)

    def test_non_reachable_findings_match_default_govulncheck_policy(self):
        for frame in (
            {"module": "example.org/unused", "version": "v1.0.0"},
            {"module": "example.org/unused", "package": "example.org/unused/pkg"},
        ):
            with self.subTest(frame=frame):
                status, _ = self.run_check(
                    CONFIG, {"finding": {"osv": "GO-2099-9999", "trace": [frame]}}
                )
                self.assertEqual(status, 0)

    def test_scan_requires_symbol_source_protocol(self):
        for key in ("protocol_version", "scan_level", "scan_mode"):
            with self.subTest(key=key):
                config = {"config": dict(CONFIG["config"], **{key: "unexpected"})}
                with self.assertRaises(ValueError):
                    self.run_check(config)

    def test_empty_or_missing_config_is_rejected(self):
        for messages in ((), (finding(),)):
            with self.subTest(messages=messages):
                with self.assertRaises(ValueError):
                    self.run_check(*messages)

    def test_invalid_json_is_rejected(self):
        for data in ("not JSON", json.dumps(CONFIG) + "\n{"):
            with self.subTest(data=data):
                with self.assertRaises(ValueError):
                    check(data, io.StringIO())

    def test_unknown_message_is_rejected(self):
        with self.assertRaises(ValueError):
            self.run_check(CONFIG, {"error": "scanner failed"})

    def test_invalid_findings_are_rejected(self):
        for value in ({}, {"osv": "GO-2099-9999", "trace": []}, {"trace": [{}]}):
            with self.subTest(value=value):
                with self.assertRaises(ValueError):
                    self.run_check(CONFIG, {"finding": value})


if __name__ == "__main__":
    unittest.main()
