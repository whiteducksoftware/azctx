"""Offline prerequisite and schema regressions; never invoke Azure."""

import copy
import importlib.util
import json
import os
import subprocess
import sys
import unittest
from pathlib import Path


SCRIPT = Path(__file__).with_name("azure_integration.py")
ROOT = SCRIPT.resolve().parents[2]
PREFIX = "AZCTX_TEST_"
VARIABLES = {
    "AZCTX_INTEGRATION_ENABLED": "true",
    "AZCTX_INTEGRATION_DEDICATED_TENANT": "true",
    PREFIX + "CLIENT_ID": "cccccccc-cccc-cccc-cccc-cccccccccccc",
    PREFIX + "TENANT_ID": "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa",
    PREFIX + "SUBSCRIPTION_ID": "11111111-1111-1111-1111-111111111111",
    PREFIX + "SECOND_SUBSCRIPTION_ID": "22222222-2222-2222-2222-222222222222",
}


class PrerequisiteTests(unittest.TestCase):
    def run_script(self, variables, *args):
        env = {key: value for key, value in os.environ.items()
               if not key.startswith(("AZCTX_INTEGRATION_", PREFIX))}
        env.update(variables)
        return subprocess.run(
            [sys.executable, "-B", str(SCRIPT), *args],
            env=env, text=True, capture_output=True, check=False,
        )

    def test_disabled_is_explicit_skip_not_live_success(self):
        result = self.run_script({}, "prerequisites")
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual(result.stdout, "ready=false\n")
        self.assertIn("SKIP", result.stderr)
        self.assertNotIn("PASS", result.stderr)

    def test_explicit_dedicated_tenant_and_two_subscriptions_required(self):
        result = self.run_script(VARIABLES, "prerequisites")
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual(result.stdout, "ready=true\n")
        for key in VARIABLES:
            missing = dict(VARIABLES)
            del missing[key]
            result = self.run_script(missing, "prerequisites")
            if key == "AZCTX_INTEGRATION_ENABLED":
                self.assertEqual(result.stdout, "ready=false\n")
            else:
                self.assertNotEqual(result.returncode, 0, key)
                self.assertNotIn("ready=true", result.stdout)

    def test_invalid_identifiers_and_duplicate_subscriptions_rejected(self):
        for value in ("", "$(echo injected)", "not-a-guid",
                      "00000000-0000-0000-0000-000000000000"):
            bad = dict(VARIABLES)
            bad[PREFIX + "TENANT_ID"] = value
            self.assertNotEqual(self.run_script(bad, "prerequisites").returncode, 0)
        bad = dict(VARIABLES)
        bad[PREFIX + "SECOND_SUBSCRIPTION_ID"] = bad[PREFIX + "SUBSCRIPTION_ID"].upper()
        self.assertNotEqual(self.run_script(bad, "prerequisites").returncode, 0)

    def test_check_cannot_launch_even_binary_without_opt_in(self):
        result = self.run_script({}, "check", "--binary", "must-not-execute")
        self.assertNotEqual(result.returncode, 0)
        self.assertIn("not enabled", result.stderr)
        self.assertNotIn("Traceback", result.stderr)


class SchemaTests(unittest.TestCase):
    def setUp(self):
        spec = importlib.util.spec_from_file_location("azure_integration", SCRIPT)
        self.module = importlib.util.module_from_spec(spec)
        spec.loader.exec_module(self.module)
        fixture = ROOT / "cmd/testdata/contract/list.golden"
        self.good = json.loads(fixture.read_text(encoding="utf-8"))

    def test_schema_one_existing_contract_accepts_additive_fields(self):
        self.good["future"] = True
        self.good["subscriptions"][0]["future"] = True
        result = self.module.subscriptions(self.good)
        self.assertEqual(len(result), 2)

    def test_invalid_schema_and_subscription_shapes_fail_closed(self):
        invalid = [None, [], {}, {"schemaVersion": 2, "subscriptions": []},
                   {"schemaVersion": True, "subscriptions": []},
                   {"schemaVersion": 1, "subscriptions": {}}]
        for field, value in (
            ("id", "bad"), ("tenantId", "not-a-guid"), ("isDefault", "true"),
            ("delegated", 0), ("user", "private"), ("environment", None),
        ):
            bad = copy.deepcopy(self.good)
            bad["subscriptions"][0][field] = value
            invalid.append(bad)
        bad = copy.deepcopy(self.good)
        del bad["subscriptions"][0]["tenantDomain"]
        invalid.append(bad)
        bad = copy.deepcopy(self.good)
        bad["subscriptions"].append(copy.deepcopy(bad["subscriptions"][0]))
        invalid.append(bad)
        for document in invalid:
            with self.subTest(document=document):
                with self.assertRaises(ValueError):
                    self.module.subscriptions(document)


if __name__ == "__main__":
    unittest.main()
