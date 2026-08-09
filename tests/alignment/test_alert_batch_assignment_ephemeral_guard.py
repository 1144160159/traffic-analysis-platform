from pathlib import Path
import unittest


ROOT = Path(__file__).resolve().parents[2]


class AlertBatchAssignmentEphemeralGuardTest(unittest.TestCase):
    def test_runner_is_fixed_digest_loopback_tmpfs_and_identity_guarded(self) -> None:
        source = (ROOT / "scripts/alignment/verify_alert_batch_assignment_ephemeral.py").read_text()
        for token in (
            "postgres@sha256:",
            "codex-alert-batch-pg-",
            "traffic.remediation.owner=alert-batch-g1",
            "127.0.0.1:",
            "--tmpfs",
            "/var/lib/postgresql/data",
            "codex_ephemeral_alert_batch_sentinel",
            "identity_verified",
            "cleanup_sentinel_verified",
            "persistent_volume_attached",
            '["docker", "rm", "-f", "-v", container]',
            '"production_applied": False',
            "refusing to overwrite evidence",
        ):
            self.assertIn(token, source)

    def test_postgres_integration_covers_selection_idempotency_and_atomicity(self) -> None:
        source = (ROOT / "go/control-plane/internal/alert/api/alert_batch_assignment_postgres_integration_test.go").read_text()
        for token in (
            "ALERT_BATCH_ASSIGNMENT_INTEGRATION_DSN",
            "selection retry not idempotent",
            "must not persist the raw bearer token",
            "changed selection retry must conflict",
            "cross-tenant selection token must fail without disclosure",
            "consumed selection token must not dispatch twice",
            "changed assignment retry must conflict",
            "expired selection token must fail",
            "batch facts did not commit atomically",
            "injected audit failure must roll back the entire assignment transaction",
            "alert_batch_assignment_postgres=pass",
        ):
            self.assertIn(token, source)

    def test_api_keeps_acceptance_non_final_and_rollout_default_off(self) -> None:
        api_source = (ROOT / "go/control-plane/internal/alert/api/alert_batch_assignment_v1.go").read_text()
        main_source = (ROOT / "go/control-plane/cmd/alert-service/main.go").read_text()
        manifests = (
            ROOT / "deployments/kubernetes/applications/go-services.yaml"
        ).read_text() + (ROOT / "go/control-plane/deployments/kubernetes/alert-service.yaml").read_text()
        self.assertIn('Status: "accepted"', api_source)
        self.assertIn('OutboxStatus: "pending"', api_source)
        self.assertIn('"alert.assignment.changed.v1 consumer receipt"', api_source)
        self.assertIn('getBoolEnv("ALERT_BATCH_ASSIGNMENT_V1_ENABLED", false)', main_source)
        self.assertIn('getBoolEnv("ALERT_BATCH_ASSIGNMENT_PIPELINE_V1_ENABLED", false)', main_source)
        self.assertIn('consumer.AlertAssignmentEventTopic', main_source)
        self.assertIn('SetDLQAcknowledgementBarrier(pipeline.RecordDLQAcknowledgement)', main_source)
        self.assertIn('ALERT_BATCH_SELECTION_SIGNING_SECRET', main_source)
        self.assertIn('storedReceipt.SelectionToken = ""', api_source)
        self.assertEqual(manifests.count('ALERT_BATCH_ASSIGNMENT_V1_ENABLED, value: "false"'), 2)
        self.assertEqual(manifests.count('ALERT_BATCH_ASSIGNMENT_PIPELINE_V1_ENABLED, value: "false"'), 2)
        self.assertEqual(manifests.count('ALERT_BATCH_ASSIGNMENT_EVENT_TOPIC, value: "alert.assignment.events.v1"'), 2)
        self.assertEqual(manifests.count('ALERT_BATCH_ASSIGNMENT_EVENT_GROUP, value: "alert-service-batch-assignment-execution-v1"'), 2)
        self.assertEqual(manifests.count('ALERT_BATCH_SELECTION_SIGNING_SECRET'), 4)

    def test_execution_pipeline_preflights_authority_and_persists_dlq_barrier(self) -> None:
        pipeline = (ROOT / "go/control-plane/internal/alert/consumer/alert_batch_assignment_pipeline.go").read_text()
        migration = (ROOT / "deployments/postgres/migrations/202608092130_alert_batch_assignment_execution_v1.sql").read_text()
        for token in (
            "validateChangedAuthority",
            "alert_assignment_batch_inbox",
            "alert_assignment_projection_receipts",
            "alert_assignment_batch_dlq_receipts",
            "ProjectAlertAssignment",
            "DLQ acknowledgement",
        ):
            self.assertIn(token, pipeline + migration)
        entrypoint_guard = (ROOT / "scripts/alignment/sync_alert_batch_assignment_execution_entrypoints.py").read_text()
        self.assertIn("35-alert-batch-assignment-execution-v1.sql", entrypoint_guard)
        self.assertIn("generated F-ALERT-004 execution block is missing or stale", entrypoint_guard)

    def test_owned_postgres_execution_g1_is_honest_about_clickhouse_boundary(self) -> None:
        source = (ROOT / "scripts/alignment/verify_alert_batch_assignment_ephemeral.py").read_text()
        for token in (
            "OWNED_REAL_POSTGRES_ASSIGNMENT_PIPELINE_WITH_FAKE_CLICKHOUSE_AUTHORITY_G1",
            "ALERT_BATCH_ASSIGNMENT_EXECUTION_INTEGRATION_DSN",
            "alert_batch_assignment_execution_postgres=pass",
            "dlq_ack_source_tuple_barrier_idempotent",
        ):
            self.assertIn(token, source)

    def test_capture_binds_same_candidate_and_keeps_later_gates_open(self) -> None:
        source = (ROOT / "scripts/alignment/capture_alert_batch_assignment.py").read_text()
        for token in (
            "referenced G0 manifest does not cover the current candidate source",
            "candidate_source_stable",
            "PARTIAL_OWNED_REAL_POSTGRES_ASSIGNMENT_PIPELINE_WITH_FAKE_CLICKHOUSE_AUTHORITY_G1",
            "OPEN_FOR_APPROVED_RELEASE_CANDIDATE_POSTGRES_KAFKA_CLICKHOUSE_AND_OPENSEARCH",
            '"production_applied": False',
            "refusing to overwrite immutable evidence directory",
        ):
            self.assertIn(token, source)


if __name__ == "__main__":
    unittest.main()
