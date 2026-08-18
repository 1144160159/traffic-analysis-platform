from __future__ import annotations

import copy
import unittest

from scripts.alignment import run_m10_site_preflight_k8s as preflight
from scripts.alignment import validate_m10_site_values as validator


class M10SitePreflightEvaluationTests(unittest.TestCase):
    @classmethod
    def setUpClass(cls) -> None:
        cls.values = validator.load(validator.DEFAULT_INPUT)
        dependency = lambda item: {
            "id": item["id"], "dns_status": "PASS", "tcp_status": "PASS", "tls": item["tls"],
            "certificate": {"status": "PASS", "days_remaining": 90} if item["tls"] else None,
        }
        cls.probes = [{
            "observed_at_epoch_ms": 1000 + index,
            "cpu": {"logical_processors": 64}, "memory": {"total_gib": 256},
            "numa": {"nodes": ["node0", "node1"]},
            "nics": [{"name": "eth0", "operstate": "up", "speed_mbps": 100000}],
            "disk": {"observed": True, "free_gib": 4096},
            "dependencies": [dependency(item) for item in cls.values["site"]["externalDependencies"]],
        } for index in range(2)]
        cls.rbac = [{"allowed": True}]
        cls.secrets = [{"secret_exists": True, "key_exists": True}]

    def evaluate(self, probes=None, rbac=None, secrets=None):
        return preflight.evaluate(
            self.values, probes or copy.deepcopy(self.probes),
            rbac or copy.deepcopy(self.rbac), secrets or copy.deepcopy(self.secrets),
        )

    def test_complete_fixture_passes(self) -> None:
        self.assertEqual("PASS", self.evaluate()["status"])

    def test_missing_node_blocks(self) -> None:
        result = self.evaluate(probes=copy.deepcopy(self.probes[:1]))
        self.assertIn("NODE_COUNT_BELOW_SITE_MINIMUM", result["blocking_codes"])

    def test_capacity_shortfall_blocks(self) -> None:
        probes = copy.deepcopy(self.probes)
        for item in probes:
            item["disk"]["free_gib"] = 1
        self.assertIn("DISK_CAPACITY_BELOW_SITE_QUOTA", self.evaluate(probes=probes)["blocking_codes"])

    def test_dns_and_tcp_failure_block(self) -> None:
        probes = copy.deepcopy(self.probes)
        probes[0]["dependencies"][0]["dns_status"] = "BLOCKED"
        probes[0]["dependencies"][0]["tcp_status"] = "BLOCKED"
        result = self.evaluate(probes=probes)
        self.assertIn("DEPENDENCY_DNS_UNRESOLVED", result["blocking_codes"])
        self.assertIn("DEPENDENCY_TCP_UNREACHABLE", result["blocking_codes"])

    def test_certificate_expiry_blocks(self) -> None:
        probes = copy.deepcopy(self.probes)
        probes[0]["dependencies"][0]["certificate"]["days_remaining"] = 2
        self.assertIn("TLS_CERTIFICATE_EXPIRES_WITHIN_30_DAYS", self.evaluate(probes=probes)["blocking_codes"])

    def test_rbac_and_secret_failures_block(self) -> None:
        result = self.evaluate(rbac=[{"allowed": False}], secrets=[{"secret_exists": False, "key_exists": False}])
        self.assertIn("DEPLOYMENT_RBAC_INSUFFICIENT", result["blocking_codes"])
        self.assertIn("SECRET_OR_CA_REFERENCE_MISSING", result["blocking_codes"])


if __name__ == "__main__":
    unittest.main()
