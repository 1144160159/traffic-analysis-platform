from __future__ import annotations

import importlib.util
import sys
import unittest
from pathlib import Path


ROOT = Path(__file__).resolve().parents[2]
sys.path.insert(0, str(ROOT / "scripts/alignment"))
SPEC = importlib.util.spec_from_file_location(
    "verify_alert_projection_watermark_postgres_ephemeral",
    ROOT / "scripts/alignment/verify_alert_projection_watermark_postgres_ephemeral.py",
)
assert SPEC and SPEC.loader
MODULE = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(MODULE)


class AlertProjectionWatermarkPostgresEphemeralGuardTest(unittest.TestCase):
    def test_identity_is_deterministic_and_scoped(self) -> None:
        name = MODULE.container_name("alert-projection-watermark-g1-v1")
        self.assertEqual(name, MODULE.container_name("alert-projection-watermark-g1-v1"))
        self.assertRegex(name, r"^codex-alert-projection-pg-[0-9a-f]{12}$")

    def test_image_is_digest_pinned_and_migration_is_exact(self) -> None:
        self.assertIn("postgres@sha256:", MODULE.POSTGRES_IMAGE)
        self.assertEqual(
            MODULE.MIGRATION.as_posix(),
            "deployments/postgres/migrations/202608041100_alert_opensearch_projection_reconciliation_v1.sql",
        )

    def test_empty_run_id_is_rejected(self) -> None:
        with self.assertRaisesRegex(ValueError, "run_id is required"):
            MODULE.container_name(" ")


if __name__ == "__main__":
    unittest.main()
