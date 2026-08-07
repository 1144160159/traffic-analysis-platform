import json
import shutil
import sys
import tempfile
import unittest
from pathlib import Path


ROOT = Path(__file__).resolve().parents[2]
sys.path.insert(0, str(ROOT / "scripts/alignment"))

from verify_opensearch_search_pagination import verify  # noqa: E402


FILES = (
    "contracts/opensearch/search-pagination.v1.json",
    "contracts/alignment/features/F-SEARCH-001.json",
    "contracts/openapi/alignment-v1.openapi.json",
    "go/control-plane/internal/alert/repository/opensearch.go",
    "go/control-plane/internal/alert/repository/opensearch_cursor.go",
    "go/control-plane/internal/alert/repository/opensearch_cursor_test.go",
    "go/control-plane/internal/alert/api/handler.go",
    "go/control-plane/internal/alert/api/handler_search_cursor_test.go",
    "go/control-plane/internal/alert/service/alert_service.go",
    "go/control-plane/internal/alert/config/config.go",
    "go/control-plane/cmd/alert-service/main.go",
    "deployments/kubernetes/applications/go-services.yaml",
    "doc/07_alignment/runbooks/T-OS-003-search-pagination-pit.md",
)


def copy_candidate(parent: Path) -> Path:
    candidate = parent / "repo"
    for relative in FILES:
        target = candidate / relative
        target.parent.mkdir(parents=True, exist_ok=True)
        shutil.copy2(ROOT / relative, target)
    return candidate


class OpenSearchSearchPaginationTest(unittest.TestCase):
    def test_repository_candidate_passes_without_live_apply_claim(self) -> None:
        result = verify(ROOT)
        self.assertEqual("PASS", result["status"], result)
        self.assertEqual("PARTIAL", result["coverage_status"])
        self.assertFalse(result["production_applied"])
        self.assertFalse(result["feature_flag_default_enabled"])

    def test_false_closure_and_production_apply_are_rejected(self) -> None:
        with tempfile.TemporaryDirectory() as directory:
            candidate = copy_candidate(Path(directory))
            path = candidate / "contracts/opensearch/search-pagination.v1.json"
            payload = json.loads(path.read_text())
            payload["status"] = "closed"
            payload["production_applied"] = True
            path.write_text(json.dumps(payload))
            result = verify(candidate)
            self.assertEqual("FAIL", result["status"])
            self.assertTrue(any("must not claim" in error for error in result["errors"]))

    def test_default_on_runtime_flag_is_rejected(self) -> None:
        with tempfile.TemporaryDirectory() as directory:
            candidate = copy_candidate(Path(directory))
            path = candidate / "go/control-plane/internal/alert/config/config.go"
            path.write_text(path.read_text().replace(
                'env:"OPENSEARCH_SEARCH_CURSOR_V1_ENABLED" envDefault:"false"',
                'env:"OPENSEARCH_SEARCH_CURSOR_V1_ENABLED" envDefault:"true"'))
            result = verify(candidate)
            self.assertEqual("FAIL", result["status"])
            self.assertTrue(any("config budget missing" in error for error in result["errors"]))

    def test_stable_tie_breaker_removal_is_rejected(self) -> None:
        with tempfile.TemporaryDirectory() as directory:
            candidate = copy_candidate(Path(directory))
            path = candidate / "go/control-plane/internal/alert/repository/opensearch.go"
            path.write_text(path.read_text().replace(
                '"alert_id": map[string]interface{}{"order": sortOrder}',
                '"event_id": map[string]interface{}{"order": sortOrder}'))
            result = verify(candidate)
            self.assertEqual("FAIL", result["status"])
            self.assertTrue(any("alert_id" in error for error in result["errors"]))

    def test_partial_search_enable_is_rejected(self) -> None:
        with tempfile.TemporaryDirectory() as directory:
            candidate = copy_candidate(Path(directory))
            path = candidate / "go/control-plane/internal/alert/repository/opensearch.go"
            path.write_text(path.read_text().replace(
                "WithAllowPartialSearchResults(false)", "WithAllowPartialSearchResults(true)"))
            result = verify(candidate)
            self.assertEqual("FAIL", result["status"])
            self.assertTrue(any("WithAllowPartialSearchResults" in error for error in result["errors"]))

    def test_tenant_filter_context_removal_is_rejected(self) -> None:
        with tempfile.TemporaryDirectory() as directory:
            candidate = copy_candidate(Path(directory))
            path = candidate / "go/control-plane/internal/alert/repository/opensearch.go"
            path.write_text(path.read_text().replace(
                'filter := []map[string]interface{}{{"term": map[string]interface{}{"tenant_id": query.TenantID}}}',
                'filter := []map[string]interface{}{}'))
            result = verify(candidate)
            self.assertEqual("FAIL", result["status"])
            self.assertTrue(any("tenant_id" in error for error in result["errors"]))


if __name__ == "__main__":
    unittest.main()
