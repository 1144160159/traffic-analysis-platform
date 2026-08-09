import copy
import json
import sys
import unittest
from pathlib import Path


ROOT = Path(__file__).resolve().parents[2]
sys.path.insert(0, str(ROOT / "scripts/alignment"))

from verify_flink_state_recovery import verify  # noqa: E402


class FlinkStateRecoveryContractTest(unittest.TestCase):
    def setUp(self) -> None:
        self.contract = json.loads(
            (ROOT / "contracts/flink/state-recovery.v1.json").read_text(encoding="utf-8")
        )
        self.application = json.loads(
            (ROOT / "contracts/flink/application-cluster-migration.v1.json").read_text(
                encoding="utf-8"
            )
        )
        self.acl = json.loads(
            (ROOT / "contracts/events/kafka-acl-catalog.v1.json").read_text(encoding="utf-8")
        )

    def test_repository_satisfies_state_recovery_contract(self) -> None:
        result = verify(self.contract, self.application, self.acl)
        self.assertEqual("PASS", result["status"], result)
        self.assertEqual(9, result["canonical_jobs"])
        self.assertGreaterEqual(result["operator_uids"], 80)
        self.assertEqual(3, len(result["checkpointed_buffers"]))
        self.assertFalse(result["forbidden_hits"])

    def test_missing_uid_is_rejected(self) -> None:
        candidate = copy.deepcopy(self.contract)
        candidate["required_uids"]["flink-log-job"].append("missing-stateful-operator")
        result = verify(candidate, self.application, self.acl)
        self.assertEqual("FAIL", result["status"])
        self.assertTrue(any("UID drift" in error for error in result["errors"]))

    def test_missing_dlq_acl_is_rejected(self) -> None:
        candidate = copy.deepcopy(self.acl)
        binding = next(item for item in candidate["topic_bindings"] if item["topic"] == "dlq.v1")
        binding["producers"].remove("flink-user-behavior-job")
        result = verify(self.contract, self.application, candidate)
        self.assertEqual("FAIL", result["status"])
        self.assertTrue(any("DLQ ACL" in error for error in result["errors"]))


if __name__ == "__main__":
    unittest.main()
