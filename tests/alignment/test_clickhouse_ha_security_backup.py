import json
import shutil
import tempfile
import unittest
from pathlib import Path


ROOT = Path(__file__).resolve().parents[2]

import sys
sys.path.insert(0, str(ROOT / "scripts/alignment"))

from verify_clickhouse_ha_security_backup import verify  # noqa: E402


FILES = (
    "contracts/clickhouse/ha-security-backup.v1.json",
    "deployments/kubernetes/infrastructure/02-clickhouse.yaml",
    "deployments/kubernetes/observability/clickhouse-ha-alert-rules.yaml",
)


def copy_candidate(parent: Path) -> Path:
    candidate = parent / "repo"
    for relative in FILES:
        target = candidate / relative
        target.parent.mkdir(parents=True, exist_ok=True)
        shutil.copy2(ROOT / relative, target)
    return candidate


class ClickHouseHaSecurityBackupTest(unittest.TestCase):
    def test_repository_slice_passes_without_runtime_or_restore_claim(self) -> None:
        result = verify(ROOT)
        self.assertEqual("PASS", result["status"], result)
        self.assertEqual(10, result["required_signals"])
        self.assertEqual(10, result["alert_rules"])
        self.assertFalse(result["formal_failure_domain_proof"])
        self.assertFalse(result["runtime_collector_present"])
        self.assertIsNone(result["restore_evidence"])

    def test_false_closure_and_production_apply_are_rejected(self) -> None:
        with tempfile.TemporaryDirectory() as directory:
            candidate = copy_candidate(Path(directory))
            path = candidate / "contracts/clickhouse/ha-security-backup.v1.json"
            contract = json.loads(path.read_text())
            contract["status"] = "closed"
            contract["production_applied"] = True
            path.write_text(json.dumps(contract))
            result = verify(candidate)
            self.assertEqual("FAIL", result["status"])
            self.assertTrue(any("must not claim T-CH-006 closure" in error for error in result["errors"]))

    def test_silent_partial_query_setting_is_rejected(self) -> None:
        with tempfile.TemporaryDirectory() as directory:
            candidate = copy_candidate(Path(directory))
            contract_path = candidate / "contracts/clickhouse/ha-security-backup.v1.json"
            contract = json.loads(contract_path.read_text())
            contract["query_profile"]["settings"]["skip_unavailable_shards"] = 1
            contract_path.write_text(json.dumps(contract))
            result = verify(candidate)
            self.assertEqual("FAIL", result["status"])
            self.assertTrue(any("profile settings drift" in error for error in result["errors"]))

    def test_missing_readonly_alert_is_rejected(self) -> None:
        with tempfile.TemporaryDirectory() as directory:
            candidate = copy_candidate(Path(directory))
            path = candidate / "deployments/kubernetes/observability/clickhouse-ha-alert-rules.yaml"
            source = path.read_text()
            source = source.replace("      - alert: ClickHouseReplicaReadonly", "      - alert: ClickHouseReplicaReadonlyRemoved", 1)
            path.write_text(source)
            result = verify(candidate)
            self.assertEqual("FAIL", result["status"])
            self.assertTrue(any("alert catalog drift" in error for error in result["errors"]))

    def test_invented_restore_evidence_is_rejected(self) -> None:
        with tempfile.TemporaryDirectory() as directory:
            candidate = copy_candidate(Path(directory))
            path = candidate / "contracts/clickhouse/ha-security-backup.v1.json"
            contract = json.loads(path.read_text())
            contract["backup_restore_plan"]["last_successful_restore_evidence"] = "fake://restore"
            path.write_text(json.dumps(contract))
            result = verify(candidate)
            self.assertEqual("FAIL", result["status"])
            self.assertTrue(any("must not invent" in error for error in result["errors"]))

    def test_two_nodes_cannot_claim_three_failure_domains(self) -> None:
        with tempfile.TemporaryDirectory() as directory:
            candidate = copy_candidate(Path(directory))
            path = candidate / "contracts/clickhouse/ha-security-backup.v1.json"
            contract = json.loads(path.read_text())
            contract["topology_target"]["formal_failure_domain_proof"] = True
            path.write_text(json.dumps(contract))
            result = verify(candidate)
            self.assertEqual("FAIL", result["status"])
            self.assertTrue(any("must not claim formal" in error for error in result["errors"]))


if __name__ == "__main__":
    unittest.main()
