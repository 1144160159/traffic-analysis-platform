import json
import shutil
import tempfile
import unittest
from pathlib import Path


ROOT = Path(__file__).resolve().parents[2]

import sys
sys.path.insert(0, str(ROOT / "scripts/alignment"))

from verify_kafka_event_envelope import verify  # noqa: E402


REQUIRED_FILES = (
    "contracts/kafka/event-envelope-idempotency.v1.json",
    "proto/traffic/v1/common.proto",
    "proto/traffic/v1/feature.proto",
    "proto/traffic/v1/detection.proto",
    "java/flink-jobs/flink-feature-job/src/main/java/com/traffic/flink/feature/calculator/FeatureCalculator.java",
    "java/flink-jobs/flink-behavior-job/src/main/java/com/traffic/flink/behavior/detector/BehaviorDetectorFunction.java",
    "java/flink-jobs/flink-behavior-job/src/main/java/com/traffic/flink/behavior/detector/SyncBehaviorDetector.java",
    "java/flink-jobs/flink-behavior-job/src/main/java/com/traffic/flink/behavior/detector/BehaviorDetectionEventFactory.java",
    "go/control-plane/internal/alert/consumer/kafka_consumer.go",
    "go/control-plane/internal/alert/consumer/stable_id.go",
    "rust/probe-agent/Cargo.toml",
    "rust/probe-agent/probe-agent/src/aggregator/eviction.rs",
    "doc/07_alignment/runbooks/T-KAFKA-002-event-envelope-idempotency.md",
)


def copy_candidate(parent: Path) -> Path:
    candidate = parent / "repo"
    for relative in REQUIRED_FILES:
        source = ROOT / relative
        target = candidate / relative
        target.parent.mkdir(parents=True, exist_ok=True)
        shutil.copy2(source, target)
    return candidate


class KafkaEventEnvelopeTest(unittest.TestCase):
    def test_detection_vertical_slice_passes_without_live_claim(self) -> None:
        result = verify(ROOT)
        self.assertEqual("PASS", result["status"], result)
        self.assertEqual("PARTIAL_DETECTION_VERTICAL_SLICE", result["coverage_status"])
        self.assertFalse(result["production_applied"])

    def test_event_header_renumbering_is_rejected(self) -> None:
        with tempfile.TemporaryDirectory() as directory:
            candidate = copy_candidate(Path(directory))
            path = candidate / "proto/traffic/v1/common.proto"
            path.write_text(path.read_text(encoding="utf-8").replace("string event_id = 1;", "string event_id = 101;"), encoding="utf-8")
            result = verify(candidate)
            self.assertEqual("FAIL", result["status"])
            self.assertTrue(any("event_id must retain tag 1" in error for error in result["errors"]))

    def test_missing_source_tuple_propagation_is_rejected(self) -> None:
        with tempfile.TemporaryDirectory() as directory:
            candidate = copy_candidate(Path(directory))
            path = candidate / "java/flink-jobs/flink-feature-job/src/main/java/com/traffic/flink/feature/calculator/FeatureCalculator.java"
            path.write_text(path.read_text(encoding="utf-8").replace(".setTuple(session.getTuple())", ".setTuple(com.traffic.proto.traffic.v1.FiveTuple.getDefaultInstance())"), encoding="utf-8")
            result = verify(candidate)
            self.assertEqual("FAIL", result["status"])
            self.assertTrue(any("Feature producer missing" in error for error in result["errors"]))

    def test_behavior_factory_cannot_drop_evidence_propagation(self) -> None:
        with tempfile.TemporaryDirectory() as directory:
            candidate = copy_candidate(Path(directory))
            path = candidate / "java/flink-jobs/flink-behavior-job/src/main/java/com/traffic/flink/behavior/detector/BehaviorDetectionEventFactory.java"
            path.write_text(
                path.read_text(encoding="utf-8").replace(
                    ".addAllEvidenceIds(input.getEvidenceIdsList())",
                    ".clearEvidenceIds()",
                ),
                encoding="utf-8",
            )
            result = verify(candidate)
            self.assertEqual("FAIL", result["status"])
            self.assertTrue(any("BehaviorDetectionEventFactory.java missing" in error for error in result["errors"]))

    def test_consumer_empty_tuple_placeholder_is_rejected(self) -> None:
        with tempfile.TemporaryDirectory() as directory:
            candidate = copy_candidate(Path(directory))
            path = candidate / "go/control-plane/internal/alert/consumer/kafka_consumer.go"
            source = path.read_text(encoding="utf-8").replace("srcIP := tuple.GetSrcIp()", 'srcIP := tuple.GetSrcIp()\n\tsrcIP = ""')
            path.write_text(source, encoding="utf-8")
            result = verify(candidate)
            self.assertEqual("FAIL", result["status"])
            self.assertTrue(any("manufactures empty tuple" in error for error in result["errors"]))

    def test_random_probe_flow_identity_is_rejected(self) -> None:
        with tempfile.TemporaryDirectory() as directory:
            candidate = copy_candidate(Path(directory))
            path = candidate / "rust/probe-agent/probe-agent/src/aggregator/eviction.rs"
            source = path.read_text(encoding="utf-8").replace(
                'event_id: identity.event_id.clone()',
                'event_id: uuid::Uuid::new_v4().to_string()',
                1,
            )
            path.write_text(source, encoding="utf-8")
            result = verify(candidate)
            self.assertEqual("FAIL", result["status"])
            self.assertTrue(any("random v4 identity" in error for error in result["errors"]))

    def test_contract_cannot_claim_closed_or_production_applied(self) -> None:
        with tempfile.TemporaryDirectory() as directory:
            candidate = copy_candidate(Path(directory))
            path = candidate / "contracts/kafka/event-envelope-idempotency.v1.json"
            contract = json.loads(path.read_text(encoding="utf-8"))
            contract["status"] = "closed"
            contract["production_applied"] = True
            path.write_text(json.dumps(contract), encoding="utf-8")
            result = verify(candidate)
            self.assertEqual("FAIL", result["status"])
            self.assertTrue(any("must not claim closure" in error for error in result["errors"]))
            self.assertTrue(any("must not claim production" in error for error in result["errors"]))


if __name__ == "__main__":
    unittest.main()
