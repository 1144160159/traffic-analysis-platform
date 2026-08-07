import copy
import json
import sys
import tempfile
import unittest
from pathlib import Path


ROOT = Path(__file__).resolve().parents[2]
sys.path.insert(0, str(ROOT / "scripts/alignment"))

from verify_pg_transaction_outbox import CONTRACT, verify  # noqa: E402


class PostgresTransactionOutboxTest(unittest.TestCase):
    def test_repository_transaction_guard_passes_without_claiming_complete_coverage(self):
        result = verify()
        self.assertEqual("PASS", result["status"], result)
        self.assertEqual("PARTIAL", result["coverage_status"])
        self.assertEqual("IMPLEMENTED_REPOSITORY", result["publisher_status"])
        self.assertEqual(5, result["transaction_facts"])
        self.assertEqual(12, result["outbox_fields"])
        self.assertEqual([], result["noncanonical_slice_ids"])
        self.assertIn("CreateNotificationRule/PatchNotificationRule", result["additional_slices"])
        self.assertIn("CreateNotificationTemplate/PatchNotificationTemplate", result["additional_slices"])
        self.assertIn("CreateNotificationEscalationPolicy/PatchNotificationEscalationPolicy", result["additional_slices"])
        self.assertEqual("notification_rule_transaction_v2", result["additional_schema_groups"][0]["name"])

    def test_missing_outbox_field_is_rejected(self):
        contract = json.loads(CONTRACT.read_text(encoding="utf-8"))
        contract["required_outbox_fields"].append("missing_required_field")
        with tempfile.TemporaryDirectory() as directory:
            path = Path(directory) / "contract.json"
            path.write_text(json.dumps(contract), encoding="utf-8")
            result = verify(path)
        self.assertEqual("FAIL", result["status"])
        self.assertTrue(any("missing_required_field" in error for error in result["errors"]))

    def test_transaction_order_mutation_is_rejected(self):
        contract = json.loads(CONTRACT.read_text(encoding="utf-8"))
        assertion = copy.deepcopy(contract["source_assertions"][0])
        assertion["ordered"] = list(reversed(assertion["ordered"]))
        contract["source_assertions"][0] = assertion
        with tempfile.TemporaryDirectory() as directory:
            path = Path(directory) / "contract.json"
            path.write_text(json.dumps(contract), encoding="utf-8")
            result = verify(path)
        self.assertEqual("FAIL", result["status"])
        self.assertTrue(any("ordering failed" in error for error in result["errors"]))

    def test_publisher_contract_requires_bounded_dead_state(self):
        contract = json.loads(CONTRACT.read_text(encoding="utf-8"))
        assertion = copy.deepcopy(contract["source_assertions"][2])
        assertion["required"].append("missing_saved_view_dead_letter_guard")
        contract["source_assertions"][2] = assertion
        with tempfile.TemporaryDirectory() as directory:
            path = Path(directory) / "contract.json"
            path.write_text(json.dumps(contract), encoding="utf-8")
            result = verify(path)
        self.assertEqual("FAIL", result["status"])
        self.assertTrue(any("missing_saved_view_dead_letter_guard" in error for error in result["errors"]))

    def test_notification_transaction_schema_drift_is_rejected(self):
        contract = json.loads(CONTRACT.read_text(encoding="utf-8"))
        contract["additional_schema_groups"][0]["tokens"].append("missing_notification_revision_guard")
        with tempfile.TemporaryDirectory() as directory:
            path = Path(directory) / "contract.json"
            path.write_text(json.dumps(contract), encoding="utf-8")
            result = verify(path)
        self.assertEqual("FAIL", result["status"])
        self.assertTrue(any("missing_notification_revision_guard" in error for error in result["errors"]))

    def test_noncanonical_slice_id_is_rejected(self):
        contract = json.loads(CONTRACT.read_text(encoding="utf-8"))
        contract["additional_slices"][-1]["feature_id"] = "F-IAM-001"
        with tempfile.TemporaryDirectory() as directory:
            path = Path(directory) / "contract.json"
            path.write_text(json.dumps(contract), encoding="utf-8")
            result = verify(path)
        self.assertEqual("FAIL", result["status"])
        self.assertEqual(["F-IAM-001"], result["noncanonical_slice_ids"])


if __name__ == "__main__":
    unittest.main()
