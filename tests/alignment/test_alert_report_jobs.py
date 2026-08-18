import json
import shutil
import sys
import tempfile
import unittest
from pathlib import Path


ROOT = Path(__file__).resolve().parents[2]
sys.path.insert(0, str(ROOT / "scripts/alignment"))

from verify_alert_report_jobs import verify  # noqa: E402


FILES = (
    "contracts/reporting/alert-report-job.v1.json",
    "contracts/alignment/features/F-ALERT-001.json",
    "contracts/openapi/alignment-v1.openapi.json",
    "go/control-plane/internal/alert/api/handler_alert_reports.go",
    "go/control-plane/internal/alert/api/handler.go",
    "go/control-plane/internal/alert/api/handler_alert_reports_test.go",
    "go/control-plane/internal/alert/api/alert_report_k8s_integration_test.go",
    "go/control-plane/cmd/alert-service/main.go",
    "deployments/kubernetes/applications/go-services.yaml",
    "web/ui/src/services/alertDetailActionApi.ts",
    "web/ui/src/pages/AlertDetailPage.tsx",
    "doc/07_alignment/runbooks/T1-M09-N016-alert-report-jobs.md",
    "doc/02_acceptance/topic1/tasks/t1-m09-n016/k8s-alert-report-latest.json",
)


def copy_candidate(parent: Path) -> Path:
    candidate = parent / "repo"
    for relative in FILES:
        target = candidate / relative
        target.parent.mkdir(parents=True, exist_ok=True)
        shutil.copy2(ROOT / relative, target)
    return candidate


class AlertReportJobsTest(unittest.TestCase):
    def test_current_candidate_passes_without_production_claim(self) -> None:
        result = verify(ROOT)
        self.assertEqual("PASS", result["status"], result)
        self.assertEqual("PARTIAL", result["coverage_status"])
        self.assertFalse(result["production_applied"])
        self.assertFalse(result["feature_flag_default_enabled"])

    def test_default_on_runtime_is_rejected(self) -> None:
        with tempfile.TemporaryDirectory() as directory:
            candidate = copy_candidate(Path(directory))
            main = candidate / "go/control-plane/cmd/alert-service/main.go"
            main.write_text(main.read_text().replace(
                'getBoolEnv("ALERT_REPORT_JOBS_V1_ENABLED", false)',
                'getBoolEnv("ALERT_REPORT_JOBS_V1_ENABLED", true)'))
            result = verify(candidate)
            self.assertEqual("FAIL", result["status"])
            self.assertTrue(any("default-off" in error for error in result["errors"]))

    def test_expiry_fail_closed_removal_is_rejected(self) -> None:
        with tempfile.TemporaryDirectory() as directory:
            candidate = copy_candidate(Path(directory))
            handler = candidate / "go/control-plane/internal/alert/api/handler_alert_reports.go"
            handler.write_text(handler.read_text().replace(
                'http.StatusGone, "REPORT_EXPIRED"',
                'http.StatusOK, "REPORT_AVAILABLE"'))
            result = verify(candidate)
            self.assertEqual("FAIL", result["status"])
            self.assertTrue(any("REPORT_EXPIRED" in error for error in result["errors"]))

    def test_same_key_contract_drift_is_rejected(self) -> None:
        with tempfile.TemporaryDirectory() as directory:
            candidate = copy_candidate(Path(directory))
            contract = candidate / "contracts/reporting/alert-report-job.v1.json"
            payload = json.loads(contract.read_text())
            payload["object"]["same_key_retry"] = False
            contract.write_text(json.dumps(payload))
            result = verify(candidate)
            self.assertEqual("FAIL", result["status"])
            self.assertTrue(any("same-key" in error for error in result["errors"]))

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
