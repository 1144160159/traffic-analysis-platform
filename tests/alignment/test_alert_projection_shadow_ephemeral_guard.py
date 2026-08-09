#!/usr/bin/env python3

from __future__ import annotations

import sys
import unittest
from pathlib import Path


ROOT = Path(__file__).resolve().parents[2]
sys.path.insert(0, str(ROOT / "scripts/alignment"))

import verify_alert_projection_shadow_ephemeral as runner  # noqa: E402


class AlertProjectionShadowEphemeralGuardTest(unittest.TestCase):
    def test_empty_run_id_is_rejected(self) -> None:
        with self.assertRaises(ValueError):
            runner.names(" ")

    def test_names_are_deterministic_and_scoped(self) -> None:
        first = runner.names("shadow-run")
        self.assertEqual(first, runner.names("shadow-run"))
        self.assertNotEqual(first, runner.names("shadow-run-2"))
        self.assertEqual(len(first), 2)
        self.assertTrue(all(name.startswith("codex-alert-shadow-") for name in first))

    def test_images_are_digest_pinned_and_no_postgres_is_started(self) -> None:
        self.assertRegex(runner.CLICKHOUSE_IMAGE, r"@sha256:[0-9a-f]{64}$")
        self.assertRegex(runner.OPENSEARCH_IMAGE, r"@sha256:[0-9a-f]{64}$")
        source = (ROOT / "scripts/alignment/verify_alert_projection_shadow_ephemeral.py").read_text(encoding="utf-8")
        self.assertNotIn("POSTGRES_IMAGE", source)
        self.assertNotIn('"postgres", "run"', source)
        self.assertIn('"postgres_dependency_present": False', source)

    def test_runner_invokes_production_readers_and_checks_unchanged_hashes(self) -> None:
        source = (ROOT / "scripts/alignment/verify_alert_projection_shadow_ephemeral.py").read_text(encoding="utf-8")
        go_test = (ROOT / "go/control-plane/internal/alert/projection/reconcile_integration_test.go").read_text(encoding="utf-8")
        self.assertIn("TestAlertProjectionShadowRealClickHouseAndOpenSearch", source)
        for token in (
            "BuildShadowManifest", "NewAlertRepository", "NewOpenSearchReconcileTarget",
            "sourceBefore", "sourceAfter", "targetBefore", "targetAfter", "ProductionMutations",
        ):
            self.assertIn(token, go_test)
        self.assertIn(r"shadow_binding_sha256=([0-9a-f]{64})", source)


if __name__ == "__main__":
    unittest.main()
