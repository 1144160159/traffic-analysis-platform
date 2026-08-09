import json
import shutil
import sys
import tempfile
import unittest
from pathlib import Path


ROOT = Path(__file__).resolve().parents[2]
sys.path.insert(0, str(ROOT / "scripts/alignment"))

from verify_opensearch_projection_reconciliation import (  # noqa: E402
    CONTRACT, FEATURE, DUAL, DEBT, OS_WRITER, CH_REPOSITORY, CONSUMER, COMMON_CONSUMER, WORKER, RECONCILE,
    CLI, CONFIG, MAIN, MIGRATION, DOCKER_SQL, K8S_SQL, DEPLOYMENT, STANDALONE_DEPLOYMENT, CAPTURE, RUNBOOK, TESTS, verify,
)
from capture_opensearch_projection_reconciliation import projection_metric_lines  # noqa: E402

FILES = (CONTRACT, FEATURE, DUAL, DEBT, OS_WRITER, CH_REPOSITORY, CONSUMER, COMMON_CONSUMER, WORKER, RECONCILE,
         CLI, CONFIG, MAIN, MIGRATION, DOCKER_SQL, K8S_SQL, DEPLOYMENT, STANDALONE_DEPLOYMENT, CAPTURE, RUNBOOK, *TESTS)


def copy_candidate(parent: Path) -> Path:
    candidate = parent / "repo"
    for relative in FILES:
        target = candidate / relative
        target.parent.mkdir(parents=True, exist_ok=True)
        shutil.copy2(ROOT / relative, target)
    return candidate


class OpenSearchProjectionReconciliationTest(unittest.TestCase):
    def test_candidate_passes_without_live_apply_claim(self) -> None:
        result = verify(ROOT)
        self.assertEqual("PASS", result["status"], result)
        self.assertEqual("PARTIAL", result["coverage_status"])
        self.assertFalse(result["production_applied"])

    def test_false_closure_is_rejected(self) -> None:
        with tempfile.TemporaryDirectory() as directory:
            candidate = copy_candidate(Path(directory))
            path = candidate / CONTRACT
            payload = json.loads(path.read_text())
            payload["status"] = "closed"
            payload["production_applied"] = True
            path.write_text(json.dumps(payload))
            result = verify(candidate)
            self.assertEqual("FAIL", result["status"])
            self.assertTrue(any("must not claim" in error for error in result["errors"]))

    def test_default_on_worker_is_rejected(self) -> None:
        with tempfile.TemporaryDirectory() as directory:
            candidate = copy_candidate(Path(directory))
            path = candidate / CONFIG
            path.write_text(path.read_text().replace(
                'env:"OPENSEARCH_ALERT_PROJECTION_RECONCILE_V1_ENABLED" envDefault:"false"',
                'env:"OPENSEARCH_ALERT_PROJECTION_RECONCILE_V1_ENABLED" envDefault:"true"'))
            result = verify(candidate)
            self.assertEqual("FAIL", result["status"])
            self.assertTrue(any("config.go" in error for error in result["errors"]))

    def test_offset_commit_without_debt_guard_is_rejected(self) -> None:
        with tempfile.TemporaryDirectory() as directory:
            candidate = copy_candidate(Path(directory))
            path = candidate / CONSUMER
            path.write_text(path.read_text().replace(
                "outcome.ClickHouseCommitted && outcome.DebtRecorded",
                "outcome.ClickHouseCommitted"))
            result = verify(candidate)
            self.assertEqual("FAIL", result["status"])
            self.assertTrue(any("consumer" in error for error in result["errors"]))

    def test_external_version_removal_is_rejected(self) -> None:
        with tempfile.TemporaryDirectory() as directory:
            candidate = copy_candidate(Path(directory))
            path = candidate / OS_WRITER
            path.write_text(path.read_text().replace('VersionType: "external_gte"', 'VersionType: "internal"'))
            result = verify(candidate)
            self.assertEqual("FAIL", result["status"])
            self.assertTrue(any("external_gte" in error for error in result["errors"]))

    def test_unbounded_reconcile_claim_is_rejected(self) -> None:
        with tempfile.TemporaryDirectory() as directory:
            candidate = copy_candidate(Path(directory))
            path = candidate / CONTRACT
            payload = json.loads(path.read_text())
            payload["rebuild_scope"]["maximum_documents"] = 0
            path.write_text(json.dumps(payload))
            result = verify(candidate)
            self.assertEqual("FAIL", result["status"])
            self.assertTrue(any("bounded rebuild" in error for error in result["errors"]))

    def test_missing_migration_entrypoint_is_rejected(self) -> None:
        with tempfile.TemporaryDirectory() as directory:
            candidate = copy_candidate(Path(directory))
            path = candidate / K8S_SQL
            path.write_text(path.read_text().replace("alert_opensearch_reconcile_runs", "removed_reconcile_runs"))
            result = verify(candidate)
            self.assertEqual("FAIL", result["status"])
            self.assertTrue(any("migration entrypoint" in error for error in result["errors"]))

    def test_automatic_extra_delete_claim_is_rejected(self) -> None:
        with tempfile.TemporaryDirectory() as directory:
            candidate = copy_candidate(Path(directory))
            path = candidate / CONTRACT
            payload = json.loads(path.read_text())
            payload["rebuild_scope"]["automatic_delete_extra"] = True
            path.write_text(json.dumps(payload))
            result = verify(candidate)
            self.assertEqual("FAIL", result["status"])
            self.assertTrue(any("no-auto-delete" in error for error in result["errors"]))

    def test_terminal_receipt_without_post_repair_query_is_rejected(self) -> None:
        with tempfile.TemporaryDirectory() as directory:
            candidate = copy_candidate(Path(directory))
            path = candidate / CONTRACT
            payload = json.loads(path.read_text())
            payload["rebuild_scope"]["repair_terminal_receipt"] = "write_ack_only"
            path.write_text(json.dumps(payload))
            result = verify(candidate)
            self.assertEqual("FAIL", result["status"])
            self.assertTrue(any("terminal receipt" in error for error in result["errors"]))

    def test_clickhouse_timestamp_alias_shadow_is_rejected(self) -> None:
        with tempfile.TemporaryDirectory() as directory:
            candidate = copy_candidate(Path(directory))
            path = candidate / CH_REPOSITORY
            path.write_text(path.read_text().replace("AS last_seen_time", "AS last_seen"))
            result = verify(candidate)
            self.assertEqual("FAIL", result["status"])
            self.assertTrue(any("clickhouse.go" in error for error in result["errors"]))

    def test_legacy_fingerprint_adapter_removal_is_rejected(self) -> None:
        with tempfile.TemporaryDirectory() as directory:
            candidate = copy_candidate(Path(directory))
            path = candidate / OS_WRITER
            path.write_text(path.read_text().replace(
                'DedupFingerprint string `json:"dedup_fingerprint"`',
                'DedupFingerprint string `json:"removed_legacy_fingerprint"`'))
            result = verify(candidate)
            self.assertEqual("FAIL", result["status"])
            self.assertTrue(any("dedup_fingerprint" in error for error in result["errors"]))

    def test_metrics_service_targeting_unbound_distroless_port_is_rejected(self) -> None:
        with tempfile.TemporaryDirectory() as directory:
            candidate = copy_candidate(Path(directory))
            path = candidate / DEPLOYMENT
            path.write_text(path.read_text().replace(
                "name: metrics, port: 9093, targetPort: 8082",
                "name: metrics, port: 9093, targetPort: 9093"))
            result = verify(candidate)
            self.assertEqual("FAIL", result["status"])
            self.assertTrue(any("metrics deployment mapping" in error for error in result["errors"]))

    def test_projection_metric_capture_excludes_unrelated_metrics(self) -> None:
        payload = b"""# HELP go_gc_duration_seconds GC latency
go_gc_duration_seconds 0.1
alert_consumer_lag{topic=\"alerts\",partition=\"0\"} 2
alert_consumer_last_committed_offset{topic=\"alerts\",partition=\"0\"} 19
http_requests_total 100
"""
        self.assertEqual(
            [
                'alert_consumer_lag{topic="alerts",partition="0"} 2',
                'alert_consumer_last_committed_offset{topic="alerts",partition="0"} 19',
            ],
            projection_metric_lines(payload),
        )

    def test_projection_metric_filter_removal_is_rejected(self) -> None:
        with tempfile.TemporaryDirectory() as directory:
            candidate = copy_candidate(Path(directory))
            path = candidate / CAPTURE
            path.write_text(path.read_text().replace(
                "if line.startswith(prefixes)",
                "if line.strip()"))
            result = verify(candidate)
            self.assertEqual("FAIL", result["status"])
            self.assertTrue(any("capture_opensearch" in error for error in result["errors"]))


if __name__ == "__main__":
    unittest.main()
