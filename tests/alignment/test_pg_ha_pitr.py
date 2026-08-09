import json
import shutil
import tempfile
import unittest
from pathlib import Path


ROOT = Path(__file__).resolve().parents[2]

import sys
sys.path.insert(0, str(ROOT / "scripts/alignment"))

from verify_pg_ha_pitr import verify  # noqa: E402


REQUIRED_FILES = (
    "contracts/postgres/ha-pitr-fencing.v1.json",
    "deployments/kubernetes/infrastructure/03-postgresql.yaml",
    "doc/07_alignment/runbooks/T-PG-006-ha-pitr-fencing.md",
)


def copy_candidate(parent: Path) -> Path:
    candidate = parent / "repo"
    for relative in REQUIRED_FILES:
        source = ROOT / relative
        target = candidate / relative
        target.parent.mkdir(parents=True, exist_ok=True)
        shutil.copy2(source, target)
    return candidate


class PostgresHaPitrContractTest(unittest.TestCase):
    def test_repository_is_safe_hold_without_claiming_ha_or_pitr_complete(self) -> None:
        result = verify(ROOT)
        self.assertEqual("PASS", result["status"], result)
        self.assertEqual("PARTIAL_SAFE_HOLD", result["coverage_status"])
        self.assertEqual("safe-hold-readiness-only", result["repository_mode"])
        self.assertFalse(result["automated_promotion_enabled"])
        self.assertEqual("NOT_IMPLEMENTED", result["pitr_repository_state"])
        self.assertEqual([], result["unsafe_promotion_hits"])

    def test_reintroduced_pg_promote_is_rejected(self) -> None:
        with tempfile.TemporaryDirectory() as directory:
            candidate = copy_candidate(Path(directory))
            manifest = candidate / "deployments/kubernetes/infrastructure/03-postgresql.yaml"
            source = manifest.read_text(encoding="utf-8")
            manifest.write_text(
                source.replace(
                    "    echo \"$(date -u +%Y-%m-%dT%H:%M:%SZ): PASS:",
                    "    psql -c \"SELECT pg_promote();\"\n    echo \"$(date -u +%Y-%m-%dT%H:%M:%SZ): PASS:",
                    1,
                ),
                encoding="utf-8",
            )
            result = verify(candidate)
            self.assertEqual("FAIL", result["status"])
            self.assertTrue(result["unsafe_promotion_hits"])

    def test_concurrent_readiness_jobs_are_rejected(self) -> None:
        with tempfile.TemporaryDirectory() as directory:
            candidate = copy_candidate(Path(directory))
            manifest = candidate / "deployments/kubernetes/infrastructure/03-postgresql.yaml"
            source = manifest.read_text(encoding="utf-8")
            manifest.write_text(
                source.replace("  concurrencyPolicy: Forbid", "  concurrencyPolicy: Allow", 1),
                encoding="utf-8",
            )
            result = verify(candidate)
            self.assertEqual("FAIL", result["status"])
            self.assertTrue(any("forbid concurrent" in error for error in result["errors"]))

    def test_primary_role_probe_is_required(self) -> None:
        with tempfile.TemporaryDirectory() as directory:
            candidate = copy_candidate(Path(directory))
            manifest = candidate / "deployments/kubernetes/infrastructure/03-postgresql.yaml"
            source = manifest.read_text(encoding="utf-8")
            manifest.write_text(
                source.replace("SELECT NOT pg_is_in_recovery()", "SELECT true", 1),
                encoding="utf-8",
            )
            result = verify(candidate)
            self.assertEqual("FAIL", result["status"])
            self.assertTrue(any("primary readiness probe" in error for error in result["errors"]))

    def test_contract_cannot_claim_unproven_pitr(self) -> None:
        with tempfile.TemporaryDirectory() as directory:
            candidate = copy_candidate(Path(directory))
            contract_path = candidate / "contracts/postgres/ha-pitr-fencing.v1.json"
            contract = json.loads(contract_path.read_text(encoding="utf-8"))
            contract["pitr"]["repository_state"] = "IMPLEMENTED"
            contract_path.write_text(json.dumps(contract), encoding="utf-8")
            result = verify(candidate)
            self.assertEqual("FAIL", result["status"])
            self.assertTrue(any("PITR cannot be marked" in error for error in result["errors"]))


if __name__ == "__main__":
    unittest.main()
