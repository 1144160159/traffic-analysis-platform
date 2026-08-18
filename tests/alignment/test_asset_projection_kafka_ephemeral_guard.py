from __future__ import annotations

import importlib.util
import sys
import unittest
from pathlib import Path


ROOT = Path(__file__).resolve().parents[2]
sys.path.insert(0, str(ROOT / "scripts/alignment"))
SPEC = importlib.util.spec_from_file_location(
    "verify_asset_projection_kafka_ephemeral",
    ROOT / "scripts/alignment/verify_asset_projection_kafka_ephemeral.py",
)
assert SPEC and SPEC.loader
MODULE = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(MODULE)


class AssetProjectionKafkaEphemeralGuardTest(unittest.TestCase):
    def test_container_names_are_deterministic_and_scoped(self) -> None:
        postgres, kafka = MODULE.names("asset-projection-kafka-g1-v1")
        self.assertEqual((postgres, kafka), MODULE.names("asset-projection-kafka-g1-v1"))
        self.assertRegex(postgres, r"^codex-asset-projection-kafka-pg-[0-9a-f]{12}$")
        self.assertRegex(kafka, r"^codex-asset-projection-kafka-broker-[0-9a-f]{12}$")

    def test_images_are_digest_pinned_and_topic_is_canonical(self) -> None:
        self.assertIn("postgres@sha256:", MODULE.POSTGRES_IMAGE)
        self.assertIn("redpandadata/redpanda@sha256:", MODULE.KAFKA_IMAGE)
        self.assertEqual(MODULE.TOPIC, "asset.events.v2")

    def test_sentinel_is_ephemeral_only(self) -> None:
        self.assertEqual(MODULE.SENTINEL_VALUE, "ephemeral-only")
        self.assertEqual(MODULE.SENTINEL_TABLE, "codex_ephemeral_asset_projection_kafka_sentinel")

    def test_empty_run_id_is_rejected(self) -> None:
        with self.assertRaisesRegex(ValueError, "run_id is required"):
            MODULE.names(" ")

    def test_oracle_markers_require_exact_per_test_set(self) -> None:
        events = []
        for test, markers in MODULE.EXPECTED_ORACLE_MARKERS.items():
            for marker in markers:
                events.append({"Test": test, "Output": f"TOPIC1_ORACLE PASS {marker}\n"})
        actual = MODULE.collect_oracle_markers(events)
        self.assertEqual(set(actual), set(MODULE.EXPECTED_ORACLE_MARKERS))
        self.assertTrue(all(MODULE.derive_oracle_flags(actual).values()))

    def test_missing_or_duplicate_oracle_marker_is_rejected(self) -> None:
        events = []
        for test, markers in MODULE.EXPECTED_ORACLE_MARKERS.items():
            for marker in markers:
                events.append({"Test": test, "Output": f"TOPIC1_ORACLE PASS {marker}\n"})
        events.pop()
        with self.assertRaisesRegex(ValueError, "oracle exact-set mismatch"):
            MODULE.collect_oracle_markers(events)

    def test_oracle_flags_cannot_be_self_reported_from_an_incomplete_set(self) -> None:
        incomplete = {
            "TestAssetProjectionRealKafkaDurableInbox": ["ACK_BEFORE_PUBLISHED"],
            "TestAssetProjectionKafkaPublishFailureKeepsOutboxPending": [],
        }
        with self.assertRaisesRegex(ValueError, "oracle flags are incomplete"):
            MODULE.derive_oracle_flags(incomplete)


if __name__ == "__main__":
    unittest.main()
