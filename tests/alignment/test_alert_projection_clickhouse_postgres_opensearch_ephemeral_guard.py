import importlib.util
import unittest
from pathlib import Path


ROOT = Path(__file__).resolve().parents[2]
SCRIPT = ROOT / "scripts/alignment/verify_alert_projection_clickhouse_postgres_opensearch_ephemeral.py"


def load_module():
    spec = importlib.util.spec_from_file_location("alert_projection_three_store_g1", SCRIPT)
    module = importlib.util.module_from_spec(spec)
    assert spec.loader is not None
    spec.loader.exec_module(module)
    return module


class AlertProjectionClickHousePostgresOpenSearchEphemeralGuardTest(unittest.TestCase):
    def test_images_are_digest_pinned_and_authorities_are_exact(self) -> None:
        module = load_module()
        for image in (module.CLICKHOUSE_IMAGE, module.POSTGRES_IMAGE, module.OPENSEARCH_IMAGE):
            self.assertIn("@sha256:", image)
        self.assertEqual(module.CLICKHOUSE_SCHEMA.as_posix(), "common/sql/ch/00-all-tables.sql")
        self.assertEqual(
            module.POSTGRES_MIGRATION.as_posix(),
            "deployments/postgres/migrations/202608041100_alert_opensearch_projection_reconciliation_v1.sql",
        )

    def test_names_are_deterministic_scoped_and_complete(self) -> None:
        module = load_module()
        first = module.names("run-a")
        self.assertEqual(first, module.names("run-a"))
        self.assertNotEqual(first, module.names("run-b"))
        self.assertEqual(len(first), 3)
        self.assertTrue(all(name.startswith("codex-alert-three-store-") for name in first))

    def test_empty_run_id_is_rejected(self) -> None:
        module = load_module()
        with self.assertRaisesRegex(ValueError, "run_id is required"):
            module.names(" ")

    def test_bootstrap_is_canonical_and_standalone(self) -> None:
        module = load_module()
        sql, schema_hash = module.clickhouse_bootstrap()
        text = sql.decode()
        self.assertEqual(len(schema_hash), 64)
        self.assertIn("CREATE TABLE traffic.alerts", text)
        self.assertIn("CREATE TABLE traffic.alerts_latest AS traffic.alerts", text)
        self.assertIn("CREATE MATERIALIZED VIEW traffic.mv_alerts_latest", text)
        self.assertNotIn(" ON CLUSTER ", text)
        self.assertNotIn("Replicated", text)

    def test_runner_invokes_exact_three_store_production_path(self) -> None:
        source = SCRIPT.read_text(encoding="utf-8")
        self.assertIn("TestAlertProjectionRepairRealClickHousePostgresAndOpenSearch", source)
        self.assertIn('"persistent_volume_attached": False', source)
        self.assertIn('"shared_environment_touched": False', source)
        self.assertIn('"production_applied": False', source)


if __name__ == "__main__":
    unittest.main()
