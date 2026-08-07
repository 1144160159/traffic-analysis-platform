from __future__ import annotations

import importlib.util
import unittest
from pathlib import Path


ROOT = Path(__file__).resolve().parents[2]
SPEC = importlib.util.spec_from_file_location(
    "verify_asset_detail_clickhouse_ephemeral",
    ROOT / "scripts/alignment/verify_asset_detail_clickhouse_ephemeral.py",
)
assert SPEC and SPEC.loader
MODULE = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(MODULE)


class AssetDetailClickHouseEphemeralGuardTest(unittest.TestCase):
    def test_names_are_deterministic_and_scoped(self) -> None:
        names = MODULE.names("asset-clickhouse-g1-v1")
        self.assertEqual(names, MODULE.names("asset-clickhouse-g1-v1"))
        self.assertRegex(names[0], r"^codex-asset-detail-clickhouse-net-[0-9a-f]{12}$")
        self.assertRegex(names[1], r"^codex-asset-detail-clickhouse-[0-9a-f]{12}$")

    def test_image_is_repo_locked_digest(self) -> None:
        lock = (ROOT / "deployments/kubernetes/image-digests.lock.json").read_text(encoding="utf-8")
        self.assertIn("clickhouse-server@sha256:", MODULE.IMAGE)
        self.assertIn(MODULE.IMAGE, lock)

    def test_bootstrap_derives_only_relevant_canonical_tables(self) -> None:
        sql, schema_hash = MODULE.bootstrap_sql()
        self.assertEqual(len(schema_hash), 64)
        self.assertIn("CREATE TABLE traffic.sessions", sql)
        self.assertIn("CREATE TABLE traffic.alerts", sql)
        self.assertIn("ENGINE = MergeTree", sql)
        self.assertNotIn(" ON CLUSTER ", sql)
        self.assertNotIn("ReplicatedMergeTree", sql)
        self.assertNotIn("Distributed(", sql)
        self.assertIn("codex_ephemeral_asset_detail_sentinel", sql)

    def test_runner_executes_production_alert_writer_regression(self) -> None:
        source = (ROOT / "scripts/alignment/verify_asset_detail_clickhouse_ephemeral.py").read_text(encoding="utf-8")
        self.assertIn("TestAlertWriterRealCanonicalClickHouseTimestampsAndTrace", source)
        self.assertIn('"production_alert_writer_verified": False', source)

    def test_sentinel_and_empty_run_id_guard(self) -> None:
        self.assertEqual(MODULE.SENTINEL_VALUE, "ephemeral-only")
        self.assertEqual(MODULE.EPHEMERAL_USER, "codex_ephemeral")
        self.assertNotEqual(MODULE.EPHEMERAL_USER, "default")
        with self.assertRaisesRegex(ValueError, "run_id is required"):
            MODULE.names(" ")


if __name__ == "__main__":
    unittest.main()
