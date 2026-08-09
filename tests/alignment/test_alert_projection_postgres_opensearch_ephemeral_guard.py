import importlib.util
import unittest
from pathlib import Path


ROOT = Path(__file__).resolve().parents[2]
SCRIPT = ROOT / "scripts/alignment/verify_alert_projection_postgres_opensearch_ephemeral.py"


def load_module():
    spec = importlib.util.spec_from_file_location("alert_projection_combined_g1", SCRIPT)
    module = importlib.util.module_from_spec(spec)
    assert spec.loader is not None
    spec.loader.exec_module(module)
    return module


class AlertProjectionPostgresOpenSearchEphemeralGuardTest(unittest.TestCase):
    def test_images_are_digest_pinned_and_migration_is_exact(self) -> None:
        module = load_module()
        self.assertIn("@sha256:", module.POSTGRES_IMAGE)
        self.assertIn("@sha256:", module.OPENSEARCH_IMAGE)
        self.assertEqual(
            module.MIGRATION.as_posix(),
            "deployments/postgres/migrations/202608041100_alert_opensearch_projection_reconciliation_v1.sql",
        )

    def test_names_are_deterministic_and_scoped(self) -> None:
        module = load_module()
        first = module.names("run-a")
        self.assertEqual(first, module.names("run-a"))
        self.assertNotEqual(first, module.names("run-b"))
        self.assertTrue(first[0].startswith("codex-alert-reconcile-pg-"))
        self.assertTrue(first[1].startswith("codex-alert-reconcile-os-"))

    def test_empty_run_id_is_rejected(self) -> None:
        module = load_module()
        with self.assertRaisesRegex(ValueError, "run_id is required"):
            module.names(" ")

    def test_runner_invokes_exact_combined_production_path(self) -> None:
        source = SCRIPT.read_text(encoding="utf-8")
        self.assertIn("TestAlertProjectionRepairRealPostgresAndOpenSearch", source)
        self.assertIn('"persistent_volume_attached": False', source)
        self.assertIn('"shared_environment_touched": False', source)
        self.assertIn('"production_applied": False', source)


if __name__ == "__main__":
    unittest.main()
