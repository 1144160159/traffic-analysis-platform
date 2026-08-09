from pathlib import Path
import unittest


ROOT = Path(__file__).resolve().parents[2]


class AlertSavedViewEphemeralGuardTest(unittest.TestCase):
    def test_owned_runner_is_fixed_digest_tmpfs_guarded_and_self_cleaning(self) -> None:
        source = (ROOT / "scripts/alignment/verify_alert_saved_view_ephemeral.py").read_text()
        for token in (
            "postgres@sha256:",
            "codex-alert-saved-view-pg-",
            "--tmpfs",
            "/var/lib/postgresql/data",
            "codex_ephemeral_alert_saved_view_sentinel",
            "container_identity_verified",
            "cleanup_sentinel_verified",
            "persistent_volume_attached",
            '["docker", "rm", "-f", "-v", container]',
            '"production_applied": False',
            "refusing to overwrite saved-view PostgreSQL evidence",
        ):
            self.assertIn(token, source)

    def test_postgres_integration_covers_atomic_revision_boundaries(self) -> None:
        source = (
            ROOT
            / "go/control-plane/internal/alert/api/handler_alert_saved_view_postgres_integration_test.go"
        ).read_text()
        for token in (
            "ALERT_SAVED_VIEW_EPHEMERAL_PG_DSN",
            "codex_ephemeral_alert_saved_view_sentinel",
            "Idempotency-Key",
            '"REVISION_CONFLICT"',
            "cross-tenant list",
            "forced saved-view audit failure",
            "alert_saved_view_history",
            "alert_saved_view_outbox",
            "audit_logs",
            "alert_saved_view_postgres_transaction=pass",
        ):
            self.assertIn(token, source)

    def test_capture_binds_same_candidate_g0_and_keeps_later_gates_open(self) -> None:
        source = (ROOT / "scripts/alignment/capture_alert_saved_view.py").read_text()
        for token in (
            "referenced G0 manifest does not cover the current candidate source",
            "candidate_source_stable",
            "PARTIAL_OWNED_REAL_POSTGRES_ALERT_SAVED_VIEW_ATOMIC_REVISION_G1",
            "OPEN_FOR_APPROVED_RELEASE_CANDIDATE_POSTGRES_AND_KAFKA",
            '"production_applied": False',
            "refusing to overwrite immutable evidence directory",
        ):
            self.assertIn(token, source)


if __name__ == "__main__":
    unittest.main()
