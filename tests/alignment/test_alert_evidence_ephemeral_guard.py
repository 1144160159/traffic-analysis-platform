from pathlib import Path
import unittest


ROOT = Path(__file__).resolve().parents[2]


class AlertEvidenceEphemeralGuardTest(unittest.TestCase):
    def test_runner_is_fixed_digest_loopback_tmpfs_guarded_and_self_cleaning(self) -> None:
        source = (ROOT / "scripts/alignment/verify_alert_evidence_ephemeral.py").read_text()
        for token in (
            "postgres@sha256:",
            "minio/minio@sha256:",
            "codex-alert-evidence-",
            "codex.owned=alert-evidence-ephemeral",
            "127.0.0.1:",
            "--tmpfs",
            "/var/lib/postgresql/data",
            "/data:rw,nosuid,nodev",
            "codex_ephemeral_alert_evidence_sentinel",
            "identity_verified",
            "cleanup_sentinel_verified",
            "persistent_volume_attached",
            '["docker", "rm", "-f", "-v", name]',
            '"production_applied": False',
            "refusing to overwrite alert-evidence evidence",
        ):
            self.assertIn(token, source)

    def test_postgres_integration_proves_tenant_revision_and_immutability(self) -> None:
        source = (ROOT / "go/control-plane/internal/alert/api/alert_evidence_manifest_postgres_integration_test.go").read_text()
        for token in (
            "ALERT_EVIDENCE_EPHEMERAL_PG_DSN",
            "codex_ephemeral_alert_evidence_sentinel",
            "tenant-bound manifest",
            "stale manifest revision unexpectedly committed",
            "immutable object identity unexpectedly changed",
            "alert_evidence_manifest_history",
            "alert_evidence_postgres_manifest=pass",
        ):
            self.assertIn(token, source)

    def test_minio_integration_proves_version_and_sha256(self) -> None:
        source = (ROOT / "go/control-plane/internal/alert/api/alert_evidence_minio_integration_test.go").read_text()
        for token in (
            "ALERT_EVIDENCE_EPHEMERAL_MINIO",
            "refusing non-loopback MinIO endpoint",
            "EnableVersioning",
            "VersionID",
            "verifyAlertEvidenceObjectIntegrity",
            "checksum mismatch unexpectedly passed",
            "alert_evidence_minio_integrity=pass",
        ):
            self.assertIn(token, source)

    def test_capture_binds_same_candidate_and_keeps_later_gates_open(self) -> None:
        source = (ROOT / "scripts/alignment/capture_alert_evidence.py").read_text()
        for token in (
            "referenced G0 manifest does not cover the current candidate source",
            "candidate_source_stable",
            "PARTIAL_OWNED_REAL_POSTGRES_MINIO_ALERT_EVIDENCE_MANIFEST_INTEGRITY_G1",
            "OPEN_FOR_APPROVED_RELEASE_CANDIDATE_POSTGRES_CLICKHOUSE_OPENSEARCH_AND_MINIO",
            '"production_applied": False',
            "refusing to overwrite immutable evidence directory",
        ):
            self.assertIn(token, source)


if __name__ == "__main__":
    unittest.main()
