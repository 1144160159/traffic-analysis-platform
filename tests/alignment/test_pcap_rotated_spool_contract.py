from __future__ import annotations

import copy
import json
import unittest
from pathlib import Path


ROOT = Path(__file__).resolve().parents[2]
SCHEMA = ROOT / "contracts/capture/pcap-rotated-spool.schema.json"
CONTRACT = ROOT / "contracts/capture/pcap-rotated-spool.v1.json"


class PcapRotatedSpoolContractTests(unittest.TestCase):
    @classmethod
    def setUpClass(cls) -> None:
        cls.schema = json.loads(SCHEMA.read_text(encoding="utf-8"))
        cls.contract = json.loads(CONTRACT.read_text(encoding="utf-8"))

    def validate_semantics(self, contract: dict) -> None:
        expected_states = {
            "ROTATED",
            "SPOOLED",
            "PENDING",
            "OBJECT_WRITTEN",
            "METADATA_ACCEPTED",
            "CLEANUP_AUTHORIZED",
            "DELETED",
            "RETRY_WAIT",
            "QUARANTINED",
        }
        self.assertEqual(expected_states, set(contract["states"]))
        self.assertEqual(
            {"configuration_key": "archiver.durable_spool_enabled", "default": False},
            contract["activation_guard"],
        )
        transitions = {(item["from"], item["to"]): item for item in contract["transitions"]}
        for edge in [
            ("ROTATED", "SPOOLED"),
            ("SPOOLED", "PENDING"),
            ("PENDING", "OBJECT_WRITTEN"),
            ("OBJECT_WRITTEN", "METADATA_ACCEPTED"),
            ("METADATA_ACCEPTED", "CLEANUP_AUTHORIZED"),
            ("CLEANUP_AUTHORIZED", "DELETED"),
            ("PENDING", "RETRY_WAIT"),
            ("OBJECT_WRITTEN", "RETRY_WAIT"),
            ("RETRY_WAIT", "PENDING"),
            ("RETRY_WAIT", "OBJECT_WRITTEN"),
        ]:
            self.assertIn(edge, transitions)
            self.assertTrue(transitions[edge]["durability_barrier"])

        forbidden = {(item["from"], item["to"]) for item in contract["forbidden_transitions"]}
        for edge in [
            ("PENDING", "METADATA_ACCEPTED"),
            ("PENDING", "CLEANUP_AUTHORIZED"),
            ("OBJECT_WRITTEN", "PENDING"),
            ("OBJECT_WRITTEN", "CLEANUP_AUTHORIZED"),
            ("METADATA_ACCEPTED", "DELETED"),
            ("DELETED", "PENDING"),
        ]:
            self.assertIn(edge, forbidden)
        self.assertFalse(set(transitions).intersection(forbidden))

        fields = set(contract["immutable_record_fields"])
        self.assertTrue(
            {
                "task_id",
                "capture_uuid",
                "manifest_hash",
                "object_key",
                "canonical_local_path",
                "stored_size",
                "sha256",
            }.issubset(fields)
        )
        rejections = set(contract["rejection_codes"])
        self.assertIn("REJECT_CLEANUP_WITHOUT_EXACT_CLAIM", rejections)
        self.assertIn("REJECT_OBJECT_RECEIPT_NOT_DURABLE", rejections)
        self.assertIn("REJECT_SHUTDOWN_ORDER_OR_DEADLINE", rejections)

    def test_contract_and_schema_are_valid_json_and_semantically_complete(self) -> None:
        self.assertEqual("https://json-schema.org/draft/2020-12/schema", self.schema["$schema"])
        self.assertFalse(self.schema["additionalProperties"])
        self.validate_semantics(self.contract)

    def test_missing_object_receipt_transition_fails_closed(self) -> None:
        mutated = copy.deepcopy(self.contract)
        mutated["transitions"] = [
            item
            for item in mutated["transitions"]
            if (item["from"], item["to"]) != ("PENDING", "OBJECT_WRITTEN")
        ]
        with self.assertRaises(AssertionError):
            self.validate_semantics(mutated)

    def test_cleanup_shortcut_fails_closed(self) -> None:
        mutated = copy.deepcopy(self.contract)
        mutated["transitions"].append(
            {
                "from": "METADATA_ACCEPTED",
                "event": "delete_by_age",
                "to": "DELETED",
                "durability_barrier": "none",
            }
        )
        with self.assertRaises(AssertionError):
            self.validate_semantics(mutated)

    def test_retry_wait_without_exact_phase_resumption_fails_closed(self) -> None:
        mutated = copy.deepcopy(self.contract)
        mutated["transitions"] = [
            item
            for item in mutated["transitions"]
            if (item["from"], item["to"]) != ("RETRY_WAIT", "OBJECT_WRITTEN")
        ]
        with self.assertRaises(AssertionError):
            self.validate_semantics(mutated)


if __name__ == "__main__":
    unittest.main()
