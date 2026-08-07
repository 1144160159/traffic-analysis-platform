import json
import shutil
import tempfile
import unittest
from pathlib import Path


ROOT = Path(__file__).resolve().parents[2]

import sys
sys.path.insert(0, str(ROOT / "scripts/alignment"))

from verify_redis_reliability_domains import verify  # noqa: E402


REQUIRED_FILES = (
    "contracts/redis/reliability-domains.v1.json",
    "deployments/kubernetes/infrastructure/04-redis.yaml",
    "deployments/kubernetes/applications/go-services.yaml",
    "doc/07_alignment/runbooks/T-REDIS-001-reliability-domain-migration.md",
)


def copy_candidate(parent: Path) -> Path:
    candidate = parent / "repo"
    for relative in REQUIRED_FILES:
        source = ROOT / relative
        target = candidate / relative
        target.parent.mkdir(parents=True, exist_ok=True)
        shutil.copy2(source, target)
    return candidate


class RedisReliabilityDomainsTest(unittest.TestCase):
    def test_repository_safe_hold_passes_without_claiming_live_completion(self) -> None:
        result = verify(ROOT)
        self.assertEqual("PASS", result["status"], result)
        self.assertEqual("PARTIAL_SAFE_HOLD", result["coverage_status"])
        self.assertFalse(result["production_applied"])
        self.assertGreater(result["mixed_clients_remaining"], 0)

    def test_coordination_allkeys_lru_is_rejected(self) -> None:
        with tempfile.TemporaryDirectory() as directory:
            candidate = copy_candidate(Path(directory))
            manifest = candidate / "deployments/kubernetes/infrastructure/04-redis.yaml"
            source = manifest.read_text(encoding="utf-8")
            manifest.write_text(source.replace("- noeviction", "- allkeys-lru", 1), encoding="utf-8")
            result = verify(candidate)
            self.assertEqual("FAIL", result["status"])
            self.assertTrue(any("reliable domain must use noeviction" in error for error in result["errors"]))

    def test_cache_noeviction_is_rejected(self) -> None:
        with tempfile.TemporaryDirectory() as directory:
            candidate = copy_candidate(Path(directory))
            manifest = candidate / "deployments/kubernetes/infrastructure/04-redis.yaml"
            source = manifest.read_text(encoding="utf-8")
            marker = "alignment.traffic-platform.io/reliability-policy: allkeys-lru-discardable"
            before, after = source.split(marker, 1)
            after = after.replace("- allkeys-lru", "- noeviction", 1)
            manifest.write_text(before + marker + after, encoding="utf-8")
            result = verify(candidate)
            self.assertEqual("FAIL", result["status"])
            self.assertTrue(any("redis-cache must use allkeys-lru" in error for error in result["errors"]))

    def test_auth_session_database_drift_is_rejected(self) -> None:
        with tempfile.TemporaryDirectory() as directory:
            candidate = copy_candidate(Path(directory))
            manifest = candidate / "deployments/kubernetes/applications/go-services.yaml"
            source = manifest.read_text(encoding="utf-8")
            anchor = "name: auth-service"
            before, after = source.split(anchor, 1)
            after = after.replace('{name: REDIS_DB, value: "1"}', '{name: REDIS_DB, value: "0"}', 1)
            manifest.write_text(before + anchor + after, encoding="utf-8")
            result = verify(candidate)
            self.assertEqual("FAIL", result["status"])
            self.assertTrue(any("auth-service must use Redis DB 1" in error for error in result["errors"]))

    def test_legacy_mixed_service_cannot_become_a_new_binding(self) -> None:
        with tempfile.TemporaryDirectory() as directory:
            candidate = copy_candidate(Path(directory))
            manifest = candidate / "deployments/kubernetes/infrastructure/04-redis.yaml"
            source = manifest.read_text(encoding="utf-8")
            manifest.write_text(source.replace("new-client-use: forbidden", "new-client-use: allowed", 1), encoding="utf-8")
            result = verify(candidate)
            self.assertEqual("FAIL", result["status"])
            self.assertTrue(any("forbid new clients" in error for error in result["errors"]))

    def test_contract_cannot_claim_closed_or_applied(self) -> None:
        with tempfile.TemporaryDirectory() as directory:
            candidate = copy_candidate(Path(directory))
            path = candidate / "contracts/redis/reliability-domains.v1.json"
            contract = json.loads(path.read_text(encoding="utf-8"))
            contract["status"] = "closed"
            contract["production_applied"] = True
            path.write_text(json.dumps(contract), encoding="utf-8")
            result = verify(candidate)
            self.assertEqual("FAIL", result["status"])
            self.assertTrue(any("must not claim closed" in error for error in result["errors"]))
            self.assertTrue(any("must not claim" in error for error in result["errors"]))


if __name__ == "__main__":
    unittest.main()
