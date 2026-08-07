import json
import shutil
import tempfile
import unittest
from pathlib import Path


ROOT = Path(__file__).resolve().parents[2]

import sys
sys.path.insert(0, str(ROOT / "scripts/alignment"))

from verify_clickhouse_retention_lifecycle import verify  # noqa: E402


FILES = (
    "contracts/clickhouse/retention-lifecycle.v1.json",
    "common/sql/ch/00-all-tables.sql",
    "go/control-plane/deployments/docker/init/clickhouse_merged.sql",
    "deployments/kubernetes/init-jobs/06-minio-lifecycle.yaml",
    "deployments/clickhouse/migrations/202608031600_sessions_daily_rollup_v1.sql",
)


def copy_candidate(parent: Path) -> Path:
    candidate = parent / "repo"
    for relative in FILES:
        target = candidate / relative
        target.parent.mkdir(parents=True, exist_ok=True)
        shutil.copy2(ROOT / relative, target)
    return candidate


class ClickHouseRetentionLifecycleTest(unittest.TestCase):
    def test_repository_retention_slice_passes_without_live_apply_claim(self) -> None:
        result = verify(ROOT)
        self.assertEqual("PASS", result["status"], result)
        self.assertEqual(9, result["matrix_domains"])
        self.assertEqual(90, result["sessions_retention_days"])
        self.assertEqual(365, result["session_rollup_retention_days"])
        self.assertEqual(37, result["pcap_object_retention_days"])

    def test_object_retention_shorter_than_reference_is_rejected(self) -> None:
        with tempfile.TemporaryDirectory() as directory:
            candidate = copy_candidate(Path(directory))
            path = candidate / "contracts/clickhouse/retention-lifecycle.v1.json"
            contract = json.loads(path.read_text())
            item = next(value for value in contract["retention_matrix"] if value["domain"] == "pcap_object")
            item["retention_days"] = 14
            path.write_text(json.dumps(contract))
            result = verify(candidate)
            self.assertEqual("FAIL", result["status"])
            self.assertTrue(any("index retention" in error for error in result["errors"]))

    def test_docker_sessions_ttl_drift_is_rejected(self) -> None:
        with tempfile.TemporaryDirectory() as directory:
            candidate = copy_candidate(Path(directory))
            path = candidate / "go/control-plane/deployments/docker/init/clickhouse_merged.sql"
            source = path.read_text()
            block = "TTL toDateTime(ts_end) + INTERVAL 90 DAY"
            self.assertIn(block, source)
            path.write_text(source.replace(block, "TTL toDateTime(ts_end) + INTERVAL 30 DAY", 1))
            result = verify(candidate)
            self.assertEqual("FAIL", result["status"])
            self.assertTrue(any("Docker sessions TTL" in error for error in result["errors"]))

    def test_rollup_without_aggregate_version_is_rejected(self) -> None:
        with tempfile.TemporaryDirectory() as directory:
            candidate = copy_candidate(Path(directory))
            path = candidate / "deployments/clickhouse/migrations/202608031600_sessions_daily_rollup_v1.sql"
            path.write_text(path.read_text().replace("toUInt16(1) AS aggregate_version", "toUInt16(1) AS version", 1))
            result = verify(candidate)
            self.assertEqual("FAIL", result["status"])
            self.assertTrue(any("aggregate_version" in error for error in result["errors"]))

    def test_false_closure_is_rejected(self) -> None:
        with tempfile.TemporaryDirectory() as directory:
            candidate = copy_candidate(Path(directory))
            path = candidate / "contracts/clickhouse/retention-lifecycle.v1.json"
            contract = json.loads(path.read_text())
            contract["status"] = "closed"
            contract["production_applied"] = True
            path.write_text(json.dumps(contract))
            result = verify(candidate)
            self.assertEqual("FAIL", result["status"])
            self.assertTrue(any("must not claim T-CH-005 closure" in error for error in result["errors"]))


if __name__ == "__main__":
    unittest.main()
