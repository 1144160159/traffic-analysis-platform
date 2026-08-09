import copy
import json
import sys
import unittest
from pathlib import Path


ROOT = Path(__file__).resolve().parents[2]
sys.path.insert(0, str(ROOT / "scripts/alignment"))

from verify_pcap_metadata_ack import verify  # noqa: E402


class PcapMetadataAckTest(unittest.TestCase):
    def setUp(self) -> None:
        self.contract = json.loads(
            (ROOT / "contracts/kafka/pcap-metadata-ack.v1.json").read_text(
                encoding="utf-8"
            )
        )

    def test_repository_slice_passes_without_final_index_claim(self) -> None:
        result = verify(self.contract)
        self.assertEqual("PASS", result["status"], result)
        self.assertEqual("PARTIAL_PROBE_TO_KAFKA_RECEIPT", result["coverage_status"])
        self.assertGreater(len(result["remaining_gates"]), 0)

    def test_missing_durable_ack_token_is_rejected(self) -> None:
        candidate = copy.deepcopy(self.contract)
        candidate["source_assertions"][1]["required_tokens"].append("missing-ack-token")
        result = verify(candidate)
        self.assertEqual("FAIL", result["status"])
        self.assertTrue(any("missing tokens" in error for error in result["errors"]))

    def test_pseudo_ack_token_is_rejected(self) -> None:
        candidate = copy.deepcopy(self.contract)
        candidate["source_assertions"][1]["forbidden_tokens"].append("package server")
        result = verify(candidate)
        self.assertEqual("FAIL", result["status"])
        self.assertTrue(any("pseudo-ACK" in error for error in result["errors"]))

    def test_false_closure_is_rejected(self) -> None:
        candidate = copy.deepcopy(self.contract)
        candidate["status"] = "closed"
        candidate["production_applied"] = True
        result = verify(candidate)
        self.assertEqual("FAIL", result["status"])
        self.assertTrue(any("cannot close" in error for error in result["errors"]))
        self.assertTrue(any("production_applied" in error for error in result["errors"]))


if __name__ == "__main__":
    unittest.main()
