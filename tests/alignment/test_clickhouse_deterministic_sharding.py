import json
import shutil
import tempfile
import unittest
from pathlib import Path


ROOT = Path(__file__).resolve().parents[2]

import sys
sys.path.insert(0, str(ROOT / "scripts/alignment"))

from verify_clickhouse_deterministic_sharding import verify  # noqa: E402


REQUIRED = [
    "contracts/clickhouse/deterministic-sharding.v1.json",
    "deployments/clickhouse/migrations/202608031330_alert_feedback_v2.sql",
    "go/control-plane/internal/rules/consumer/model_feedback_inbox_worker.go",
    "go/control-plane/internal/rules/config/config.go",
    "go/control-plane/internal/alert/api/feedback_repository.go",
    "deployments/kubernetes/applications/go-services.yaml",
]


def copy_candidate(parent: Path) -> Path:
    candidate = parent / "repo"
    for relative in REQUIRED:
        target = candidate / relative
        target.parent.mkdir(parents=True, exist_ok=True)
        shutil.copy2(ROOT / relative, target)
    return candidate


class ClickHouseDeterministicShardingTest(unittest.TestCase):
    def test_inventory_and_guarded_candidate_pass_without_live_claim(self) -> None:
        result = verify(ROOT)
        self.assertEqual("PASS", result["status"], result)
        self.assertEqual(18, result["distributed_table_count"])
        self.assertEqual(13, result["rand_sharded_count"])
        self.assertEqual(5, result["deterministic_count"])
        self.assertFalse(result["production_applied"])

    def test_closed_or_production_claim_is_rejected(self) -> None:
        with tempfile.TemporaryDirectory() as directory:
            candidate = copy_candidate(Path(directory))
            path = candidate / REQUIRED[0]
            contract = json.loads(path.read_text(encoding="utf-8"))
            contract["status"] = "closed"
            contract["production_applied"] = True
            path.write_text(json.dumps(contract), encoding="utf-8")
            result = verify(candidate)
            self.assertEqual("FAIL", result["status"])
            self.assertTrue(any("must not claim T-CH-002 closure" in e for e in result["errors"]))
            self.assertTrue(any("must not claim production apply" in e for e in result["errors"]))

    def test_rand_inventory_omission_is_rejected(self) -> None:
        with tempfile.TemporaryDirectory() as directory:
            candidate = copy_candidate(Path(directory))
            path = candidate / REQUIRED[0]
            contract = json.loads(path.read_text(encoding="utf-8"))
            contract["distributed_tables"] = contract["distributed_tables"][:-1]
            path.write_text(json.dumps(contract), encoding="utf-8")
            result = verify(candidate)
            self.assertEqual("FAIL", result["status"])
            self.assertTrue(any("inventory mismatch" in e for e in result["errors"]))

    def test_v2_flag_default_true_is_rejected(self) -> None:
        with tempfile.TemporaryDirectory() as directory:
            candidate = copy_candidate(Path(directory))
            path = candidate / REQUIRED[0]
            contract = json.loads(path.read_text(encoding="utf-8"))
            contract["first_vertical_candidate"]["write_flag_default"] = True
            path.write_text(json.dumps(contract), encoding="utf-8")
            result = verify(candidate)
            self.assertEqual("FAIL", result["status"])
            self.assertTrue(any("must default false" in e for e in result["errors"]))


if __name__ == "__main__":
    unittest.main()
