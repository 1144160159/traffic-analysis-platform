import json
import shutil
import tempfile
import unittest
from pathlib import Path


ROOT = Path(__file__).resolve().parents[2]

import sys
sys.path.insert(0, str(ROOT / "scripts/alignment"))

from verify_clickhouse_query_paths import verify  # noqa: E402


REQUIRED = [
    "contracts/clickhouse/query-path-optimization.v1.json",
    "go/control-plane/internal/alert/repository/clickhouse.go",
]


def copy_candidate(parent: Path) -> Path:
    candidate = parent / "repo"
    for relative in REQUIRED:
        target = candidate / relative
        target.parent.mkdir(parents=True, exist_ok=True)
        shutil.copy2(ROOT / relative, target)
    return candidate


class ClickHouseQueryPathsTest(unittest.TestCase):
    def test_structured_row_slice_passes_without_performance_claim(self) -> None:
        result = verify(ROOT)
        self.assertEqual("PASS", result["status"], result)
        self.assertEqual(1000, result["maximum_rows_per_page"])
        self.assertFalse(result["production_applied"])

    def test_json_page_aggregation_regression_is_rejected(self) -> None:
        with tempfile.TemporaryDirectory() as directory:
            candidate = copy_candidate(Path(directory))
            path = candidate / REQUIRED[1]
            source = path.read_text(encoding="utf-8")
            source = source.replace("SELECT %s, %s AS attack_phase", "SELECT toJSONString(groupArray(%s)), %s AS attack_phase")
            path.write_text(source, encoding="utf-8")
            result = verify(candidate)
            self.assertEqual("FAIL", result["status"])
            self.assertTrue(any("JSON page aggregation" in e for e in result["errors"]))

    def test_false_closure_and_production_claim_are_rejected(self) -> None:
        with tempfile.TemporaryDirectory() as directory:
            candidate = copy_candidate(Path(directory))
            path = candidate / REQUIRED[0]
            contract = json.loads(path.read_text(encoding="utf-8"))
            contract["status"] = "closed"
            contract["production_applied"] = True
            path.write_text(json.dumps(contract), encoding="utf-8")
            result = verify(candidate)
            self.assertEqual("FAIL", result["status"])
            self.assertTrue(any("must not claim T-CH-003 closure" in e for e in result["errors"]))
            self.assertTrue(any("must not claim production apply" in e for e in result["errors"]))

    def test_final_guard_cannot_be_disabled(self) -> None:
        with tempfile.TemporaryDirectory() as directory:
            candidate = copy_candidate(Path(directory))
            path = candidate / REQUIRED[0]
            contract = json.loads(path.read_text(encoding="utf-8"))
            contract["optimization_policy"]["final_removal_requires_semantic_reconciliation"] = False
            path.write_text(json.dumps(contract), encoding="utf-8")
            result = verify(candidate)
            self.assertEqual("FAIL", result["status"])
            self.assertTrue(any("final_removal" in e for e in result["errors"]))


if __name__ == "__main__":
    unittest.main()
