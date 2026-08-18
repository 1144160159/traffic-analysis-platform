import json
import shutil
import sys
import tempfile
import unittest
from pathlib import Path


ROOT = Path(__file__).resolve().parents[2]
sys.path.insert(0, str(ROOT / "scripts/alignment"))

from verify_model_feedback_authority import verify  # noqa: E402


FILES = (
    "contracts/alignment/model-feedback-authority.v1.json",
    "contracts/events/model-feedback-event.v1.schema.json",
    "contracts/events/kafka-topic-catalog.v1.json",
    "contracts/events/kafka-acl-catalog.v1.json",
    "contracts/openapi/alignment-v1.openapi.json",
    "contracts/alignment/features/F-ALERT-001.json",
    "go/control-plane/internal/alert/api/handler_model_feedback_revision.go",
    "go/control-plane/internal/alert/api/model_feedback_readiness.go",
    "go/control-plane/internal/alert/api/handler_feedback_outbox.go",
    "go/control-plane/internal/alert/api/handler_feedback.go",
    "go/control-plane/internal/alert/api/handler_model_feedback_revision_test.go",
    "go/control-plane/internal/alert/api/model_feedback_revision_k8s_integration_test.go",
    "go/control-plane/cmd/alert-service/main.go",
    "deployments/kubernetes/applications/go-services.yaml",
    "web/ui/src/services/alertDetailApi.ts",
    "web/ui/src/pages/AlertDetailPage.tsx",
    "doc/07_alignment/runbooks/T1-M09-N017-model-feedback-authority.md",
    "doc/02_acceptance/topic1/tasks/t1-m09-n017/k8s-model-feedback-latest.json",
)


def copy_candidate(parent: Path) -> Path:
    candidate = parent / "repo"
    for relative in FILES:
        target = candidate / relative
        target.parent.mkdir(parents=True, exist_ok=True)
        shutil.copy2(ROOT / relative, target)
    return candidate


class ModelFeedbackAuthorityTest(unittest.TestCase):
    def test_current_candidate_passes_without_producer_claim(self) -> None:
        result = verify(ROOT)
        self.assertEqual("PASS", result["status"], result)
        self.assertEqual("PARTIAL", result["coverage_status"])
        self.assertFalse(result["production_applied"])
        self.assertFalse(result["authority_default_enabled"])
        self.assertFalse(result["producer_default_enabled"])
        self.assertFalse(result["k8s_consumer_schema_present"])

    def test_default_on_producer_is_rejected(self) -> None:
        with tempfile.TemporaryDirectory() as directory:
            candidate = copy_candidate(Path(directory))
            main = candidate / "go/control-plane/cmd/alert-service/main.go"
            main.write_text(main.read_text().replace(
                'getBoolEnv("MODEL_FEEDBACK_REVISION_PRODUCER_V1_ENABLED", false)',
                'getBoolEnv("MODEL_FEEDBACK_REVISION_PRODUCER_V1_ENABLED", true)'))
            result = verify(candidate)
            self.assertEqual("FAIL", result["status"])
            self.assertTrue(any("startup guard" in error for error in result["errors"]))

    def test_advisory_aggregate_lock_removal_is_rejected(self) -> None:
        with tempfile.TemporaryDirectory() as directory:
            candidate = copy_candidate(Path(directory))
            authority = candidate / "go/control-plane/internal/alert/api/handler_model_feedback_revision.go"
            authority.write_text(authority.read_text().replace("pg_advisory_xact_lock", "no_aggregate_lock"))
            result = verify(candidate)
            self.assertEqual("FAIL", result["status"])
            self.assertTrue(any("pg_advisory" in error for error in result["errors"]))

    def test_readiness_receipt_join_removal_is_rejected(self) -> None:
        with tempfile.TemporaryDirectory() as directory:
            candidate = copy_candidate(Path(directory))
            readiness = candidate / "go/control-plane/internal/alert/api/model_feedback_readiness.go"
            readiness.write_text(readiness.read_text().replace(
                "JOIN model_feedback_revision_receipt", "JOIN untrusted_receipt"))
            result = verify(candidate)
            self.assertEqual("FAIL", result["status"])
            self.assertTrue(any("readiness guard" in error for error in result["errors"]))

    def test_topic_activation_without_receipt_is_rejected(self) -> None:
        with tempfile.TemporaryDirectory() as directory:
            candidate = copy_candidate(Path(directory))
            path = candidate / "contracts/events/kafka-topic-catalog.v1.json"
            payload = json.loads(path.read_text())
            topic = next(item for item in payload["topics"] if item["name"] == "model.feedback.v1")
            topic["readiness"] = "active"
            path.write_text(json.dumps(payload))
            result = verify(candidate)
            self.assertEqual("FAIL", result["status"])
            self.assertTrue(any("default-off producer candidate" in error for error in result["errors"]))

    def test_k8s_source_hash_drift_is_rejected(self) -> None:
        with tempfile.TemporaryDirectory() as directory:
            candidate = copy_candidate(Path(directory))
            page = candidate / "web/ui/src/pages/AlertDetailPage.tsx"
            page.write_text(page.read_text() + "\n// drift\n")
            result = verify(candidate)
            self.assertEqual("FAIL", result["status"])
            self.assertTrue(any("source hash drifted" in error for error in result["errors"]))


if __name__ == "__main__":
    unittest.main()
