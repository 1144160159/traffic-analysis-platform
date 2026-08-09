import copy
import json
import sys
import unittest
from pathlib import Path


ROOT = Path(__file__).resolve().parents[2]
sys.path.insert(0, str(ROOT / "scripts/alignment"))

from verify_kafka_dlq_commit_barrier import verify  # noqa: E402


class KafkaDLQCommitBarrierTest(unittest.TestCase):
    def setUp(self) -> None:
        self.contract = json.loads(
            (ROOT / "contracts/kafka/dlq-commit-barrier.v1.json").read_text(
                encoding="utf-8"
            )
        )

    def test_repository_barrier_passes_without_live_or_closure_claim(self) -> None:
        result = verify(self.contract)
        self.assertEqual("PASS", result["status"], result)
        self.assertEqual(
            "PARTIAL_SHARED_GO_CONSUMER_WITH_DASHBOARD_OWNED_G1",
            result["coverage_status"],
        )
        self.assertGreater(len(result["remaining_gates"]), 0)

    def test_missing_ack_barrier_token_is_rejected(self) -> None:
        candidate = copy.deepcopy(self.contract)
        candidate["source_assertions"][0]["required_tokens"].append(
            "missing-durable-commit-token"
        )
        result = verify(candidate)
        self.assertEqual("FAIL", result["status"])
        self.assertTrue(any("missing tokens" in error for error in result["errors"]))

    def test_fail_open_token_is_rejected(self) -> None:
        candidate = copy.deepcopy(self.contract)
        candidate["source_assertions"][0]["forbidden_tokens"].append("package kafka")
        result = verify(candidate)
        self.assertEqual("FAIL", result["status"])
        self.assertTrue(any("fail-open tokens" in error for error in result["errors"]))

    def test_commit_after_processed_order_is_rejected(self) -> None:
        candidate = copy.deepcopy(self.contract)
        candidate["source_assertions"][0]["ordered_tokens"][1].reverse()
        result = verify(candidate)
        self.assertEqual("FAIL", result["status"])
        self.assertTrue(any("ordering failures" in error for error in result["errors"]))

    def test_false_production_or_closure_claim_is_rejected(self) -> None:
        candidate = copy.deepcopy(self.contract)
        candidate["status"] = "closed"
        candidate["production_applied"] = True
        result = verify(candidate)
        self.assertEqual("FAIL", result["status"])
        self.assertTrue(any("cannot close" in error for error in result["errors"]))
        self.assertTrue(any("production_applied" in error for error in result["errors"]))


if __name__ == "__main__":
    unittest.main()
