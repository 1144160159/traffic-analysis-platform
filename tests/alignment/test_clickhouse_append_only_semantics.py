import json
import shutil
import tempfile
import unittest
from pathlib import Path


ROOT = Path(__file__).resolve().parents[2]

import sys
sys.path.insert(0, str(ROOT / "scripts/alignment"))

from verify_clickhouse_append_only_semantics import verify  # noqa: E402


def copy_candidate(parent: Path) -> Path:
    candidate = parent / "repo"
    paths = ["contracts/clickhouse/append-only-version-semantics.v1.json"]
    paths.extend(
        path.relative_to(ROOT).as_posix()
        for path in (ROOT / "go/control-plane/internal/alert").rglob("*.go")
        if not path.name.endswith("_test.go")
    )
    for relative in paths:
        target = candidate / relative
        target.parent.mkdir(parents=True, exist_ok=True)
        shutil.copy2(ROOT / relative, target)
    return candidate


class ClickHouseAppendOnlySemanticsTest(unittest.TestCase):
    def test_go_alert_writer_slice_passes_without_live_claim(self) -> None:
        result = verify(ROOT)
        self.assertEqual("PASS", result["status"], result)
        self.assertEqual(8, result["distributed_insert_sites"])
        self.assertEqual(0, result["local_insert_sites"])
        self.assertEqual(0, result["synchronous_mutation_sites"])

    def test_local_insert_regression_is_rejected(self) -> None:
        with tempfile.TemporaryDirectory() as directory:
            candidate = copy_candidate(Path(directory))
            path = candidate / "go/control-plane/internal/alert/evidence/generator.go"
            path.write_text(path.read_text().replace("INSERT INTO traffic.evidence (", "INSERT INTO traffic.evidence_local (", 1))
            result = verify(candidate)
            self.assertEqual("FAIL", result["status"])
            self.assertTrue(any("local-table inserts" in e for e in result["errors"]))

    def test_synchronous_delete_regression_is_rejected(self) -> None:
        with tempfile.TemporaryDirectory() as directory:
            candidate = copy_candidate(Path(directory))
            path = candidate / "go/control-plane/internal/alert/evidence/generator.go"
            path.write_text(path.read_text() + "\nvar forbiddenMutation = `ALTER TABLE traffic.evidence DELETE WHERE tenant_id=?`\n")
            result = verify(candidate)
            self.assertEqual("FAIL", result["status"])
            self.assertTrue(any("synchronous ClickHouse mutations" in e for e in result["errors"]))

    def test_false_closure_is_rejected(self) -> None:
        with tempfile.TemporaryDirectory() as directory:
            candidate = copy_candidate(Path(directory))
            path = candidate / "contracts/clickhouse/append-only-version-semantics.v1.json"
            contract = json.loads(path.read_text())
            contract["status"] = "closed"
            contract["production_applied"] = True
            path.write_text(json.dumps(contract))
            result = verify(candidate)
            self.assertEqual("FAIL", result["status"])
            self.assertTrue(any("must not claim T-CH-004 closure" in e for e in result["errors"]))


if __name__ == "__main__":
    unittest.main()
