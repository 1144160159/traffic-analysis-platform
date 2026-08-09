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


if __name__ == "__main__":
    unittest.main()
