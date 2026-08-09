from __future__ import annotations

import importlib.util
import sys
import unittest
from pathlib import Path


ROOT = Path(__file__).resolve().parents[2]
sys.path.insert(0, str(ROOT / "scripts/alignment"))
SPEC = importlib.util.spec_from_file_location(
    "verify_asset_seven_source_ephemeral",
    ROOT / "scripts/alignment/verify_asset_seven_source_ephemeral.py",
)
assert SPEC and SPEC.loader
MODULE = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(MODULE)


class AssetSevenSourceEphemeralGuardTest(unittest.TestCase):
    def test_names_are_deterministic_scoped_and_complete(self) -> None:
        scoped = MODULE.names("asset-seven-source-v1")
        self.assertEqual(scoped, MODULE.names("asset-seven-source-v1"))
        self.assertEqual(
            set(scoped),
            {
                "network", "postgresql", "kafka", "clickhouse", "opensearch",
                "minio", "nebula_meta", "nebula_storage", "nebula_graph",
            },
        )
        for value in scoped.values():
            self.assertRegex(value, r"^codex-seven-source-[a-z-]+-[0-9a-f]{12}$")

    def test_all_images_are_digest_pinned(self) -> None:
        for image in (
            MODULE.POSTGRES_IMAGE, MODULE.KAFKA_IMAGE, MODULE.CLICKHOUSE_IMAGE,
            MODULE.OPENSEARCH_IMAGE, MODULE.MINIO_IMAGE, MODULE.META_IMAGE,
            MODULE.STORAGE_IMAGE, MODULE.GRAPH_IMAGE,
        ):
            self.assertIn("@sha256:", image)

    def test_source_scope_and_sentinel_are_fixed(self) -> None:
        self.assertEqual(MODULE.TOPIC, "asset.events.v2")
        self.assertEqual(MODULE.SENTINEL_VALUE, "ephemeral-only")
        self.assertEqual(MODULE.MINIO_BUCKET, "asset-seven-source")

    def test_empty_run_id_is_rejected(self) -> None:
        with self.assertRaisesRegex(ValueError, "run_id is required"):
            MODULE.names(" ")

    def test_runner_invokes_production_path_and_plan_only_oracle(self) -> None:
        source = (ROOT / "scripts/alignment/verify_asset_seven_source_ephemeral.py").read_text(encoding="utf-8")
        self.assertIn("TestAssetSevenSourceTraceReconciliation", source)
        self.assertIn("cross_store_reconcile.py", source)
        self.assertIn('"production_applied": False', source)
        self.assertIn("MinIO is explicitly seeded", source)


if __name__ == "__main__":
    unittest.main()
