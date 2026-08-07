import json
import shutil
import sys
import tempfile
import unittest
from pathlib import Path


ROOT = Path(__file__).resolve().parents[2]
sys.path.insert(0, str(ROOT / "scripts/alignment"))

from verify_trace_watermark_reconcile import (  # noqa: E402
    ALERT_MODEL,
    CH_COMMON,
    CH_DOCKER,
    CH_K8S,
    CH_MIGRATION,
    CAPTURE,
    CONTRACT,
    FLINK_BEHAVIOR,
    FLINK_BUSINESS,
    FLINK_CH,
    FLINK_OS,
    GO_ALERT_CONSUMER,
    GO_CH_WRITER,
    GO_DETECTION_CONSUMER,
    GO_PROTO,
    HTTP,
    JAVA_PROTO,
    KAFKA_CONSUMER,
    KAFKA_PRODUCER,
    OS_K8S,
    OS_MAPPING,
    OTEL,
    PROTO,
    RUNBOOK,
    RUST_PROTO,
    TESTS,
    TOOL,
    verify,
)


FILES = (
    CONTRACT, PROTO, GO_PROTO, JAVA_PROTO, RUST_PROTO, HTTP, OTEL,
    KAFKA_PRODUCER, KAFKA_CONSUMER, ALERT_MODEL, GO_CH_WRITER,
    GO_DETECTION_CONSUMER, GO_ALERT_CONSUMER, FLINK_BEHAVIOR,
    FLINK_BUSINESS, FLINK_CH, FLINK_OS, CH_MIGRATION, CH_COMMON,
    CH_DOCKER, CH_K8S, OS_MAPPING, OS_K8S, TOOL, CAPTURE, RUNBOOK, *TESTS,
)


def copy_candidate(parent: Path) -> Path:
    candidate = parent / "repo"
    for relative in FILES:
        target = candidate / relative
        target.parent.mkdir(parents=True, exist_ok=True)
        shutil.copy2(ROOT / relative, target)
    return candidate


class TraceWatermarkReconcileVerifierTest(unittest.TestCase):
    def test_candidate_passes_without_false_live_claim(self) -> None:
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

    def test_proto_field_number_drift_is_rejected(self) -> None:
        with tempfile.TemporaryDirectory() as directory:
            candidate = copy_candidate(Path(directory))
            path = candidate / PROTO
            path.write_text(path.read_text().replace("string trace_id = 33;", "string trace_id = 34;"))
            result = verify(candidate)
            self.assertEqual("FAIL", result["status"])
            self.assertTrue(any("alert.proto" in error for error in result["errors"]))

    def test_split_trace_publish_guard_removal_is_rejected(self) -> None:
        with tempfile.TemporaryDirectory() as directory:
            candidate = copy_candidate(Path(directory))
            path = candidate / KAFKA_PRODUCER
            path.write_text(path.read_text().replace("declared trace_id conflicts with W3C context", "trace mismatch ignored"))
            result = verify(candidate)
            self.assertEqual("FAIL", result["status"])
            self.assertTrue(any("producer.go" in error for error in result["errors"]))

    def test_unbounded_or_automatic_repair_contract_is_rejected(self) -> None:
        with tempfile.TemporaryDirectory() as directory:
            candidate = copy_candidate(Path(directory))
            path = candidate / CONTRACT
            payload = json.loads(path.read_text())
            payload["runtime_guards"]["maximum_records"] = 0
            payload["runtime_guards"]["automatic_repair"] = True
            path.write_text(json.dumps(payload))
            result = verify(candidate)
            self.assertEqual("FAIL", result["status"])
            self.assertTrue(any("plan-only" in error or "bounded" in error for error in result["errors"]))

    def test_opensearch_trace_mapping_removal_is_rejected(self) -> None:
        with tempfile.TemporaryDirectory() as directory:
            candidate = copy_candidate(Path(directory))
            path = candidate / OS_MAPPING
            payload = json.loads(path.read_text())
            del payload["template"]["mappings"]["properties"]["trace_id"]
            path.write_text(json.dumps(payload))
            result = verify(candidate)
            self.assertEqual("FAIL", result["status"])
            self.assertTrue(any("trace_id must be keyword" in error for error in result["errors"]))

    def test_opensearch_expand_manifest_link_drift_is_rejected(self) -> None:
        with tempfile.TemporaryDirectory() as directory:
            candidate = copy_candidate(Path(directory))
            path = candidate / OS_K8S
            path.write_text(path.read_text().replace(
                "name: opensearch-alerts-v2-contract",
                "name: removed-alerts-v2-contract",
                1,
            ))
            result = verify(candidate)
            self.assertEqual("FAIL", result["status"])
            self.assertTrue(any("ConfigMap is missing" in error for error in result["errors"]))

    def test_on_cluster_migration_is_rejected(self) -> None:
        with tempfile.TemporaryDirectory() as directory:
            candidate = copy_candidate(Path(directory))
            path = candidate / CH_MIGRATION
            path.write_text(path.read_text().replace("ALTER TABLE traffic.alerts_latest_local", "ALTER TABLE traffic.alerts_latest_local ON CLUSTER traffic_cluster", 1))
            result = verify(candidate)
            self.assertEqual("FAIL", result["status"])
            self.assertTrue(any("ON CLUSTER" in error for error in result["errors"]))

    def test_flink_trace_propagation_removal_is_rejected(self) -> None:
        with tempfile.TemporaryDirectory() as directory:
            candidate = copy_candidate(Path(directory))
            path = candidate / FLINK_BEHAVIOR
            path.write_text(path.read_text().replace(".setTraceId(header.getTraceId())", ".clearTraceId()"))
            result = verify(candidate)
            self.assertEqual("FAIL", result["status"])
            self.assertTrue(any("AlertGenerator.java" in error for error in result["errors"]))


if __name__ == "__main__":
    unittest.main()
