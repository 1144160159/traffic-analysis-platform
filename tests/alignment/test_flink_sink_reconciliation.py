import copy
import json
import sys
import unittest
from pathlib import Path


ROOT = Path(__file__).resolve().parents[2]
sys.path.insert(0, str(ROOT / "scripts/alignment"))

from verify_flink_sink_reconciliation import verify  # noqa: E402


class FlinkSinkReconciliationContractTest(unittest.TestCase):
    def setUp(self) -> None:
        self.contract = json.loads(
            (ROOT / "contracts/flink/sink-reconciliation.v1.json").read_text(encoding="utf-8")
        )
        self.application = json.loads(
            (ROOT / "contracts/flink/application-cluster-migration.v1.json").read_text(
                encoding="utf-8"
            )
        )

    def test_repository_guards_pass_without_claiming_complete_coverage(self) -> None:
        result = verify(self.contract, self.application)
        self.assertEqual("PASS", result["status"], result)
        self.assertEqual("PARTIAL", result["coverage_status"], result)
        self.assertEqual(9, result["canonical_jobs"])
        self.assertGreater(result["declared_gaps"], 0)
        self.assertFalse(result["runtime_ddl_hits"])

    def test_missing_canonical_job_is_rejected(self) -> None:
        candidate = copy.deepcopy(self.contract)
        candidate["jobs"] = candidate["jobs"][:-1]
        result = verify(candidate, self.application)
        self.assertEqual("FAIL", result["status"])
        self.assertTrue(any("canonical job drift" in error for error in result["errors"]))

    def test_missing_required_source_token_is_rejected(self) -> None:
        candidate = copy.deepcopy(self.contract)
        candidate["source_assertions"][0]["required_tokens"].append("missing-ack-token")
        result = verify(candidate, self.application)
        self.assertEqual("FAIL", result["status"])
        self.assertTrue(any("missing tokens" in error for error in result["errors"]))

    def test_empty_reconciliation_key_is_rejected(self) -> None:
        candidate = copy.deepcopy(self.contract)
        candidate["jobs"][0]["reconciliation_keys"] = []
        result = verify(candidate, self.application)
        self.assertEqual("FAIL", result["status"])
        self.assertTrue(any("reconciliation key" in error for error in result["errors"]))


if __name__ == "__main__":
    unittest.main()
