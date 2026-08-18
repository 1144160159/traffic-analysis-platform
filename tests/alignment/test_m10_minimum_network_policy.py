from __future__ import annotations

import copy
import unittest

from scripts.alignment import build_m10_minimum_network_policy as builder
from scripts.alignment import verify_m10_minimum_network_policy as verifier


class M10MinimumNetworkPolicyTests(unittest.TestCase):
    @classmethod
    def setUpClass(cls) -> None:
        cls.contract = builder.load()

    def assert_contract_rejected(self, value: dict, message: str) -> None:
        with self.assertRaisesRegex(ValueError, message):
            builder.validate(value)

    def test_checked_in_candidate_is_exact_and_default_off(self) -> None:
        self.assertEqual([], verifier.validate(self.contract, verifier.load_yaml()))
        self.assertFalse(self.contract["production_applied"])
        self.assertEqual("CANDIDATE_DEFAULT_OFF", self.contract["status"])

    def test_exact_workload_closure_and_default_deny(self) -> None:
        objects = builder.build(self.contract)
        self.assertEqual(10, len(objects))
        self.assertEqual(
            {"podSelector": {}, "policyTypes": ["Ingress", "Egress"]},
            objects[0]["spec"],
        )
        self.assertEqual(
            list(builder.EXPECTED_WORKLOADS),
            [item["metadata"]["name"].removeprefix("m10-n009-allow-") for item in objects[1:]],
        )

    def test_empty_selector_mutation_is_rejected(self) -> None:
        value = copy.deepcopy(self.contract)
        value["workloads"][0]["egress"][0]["to"]["pod_selector"] = {}
        self.assert_contract_rejected(value, "selector must be non-empty")

    def test_control_plane_port_mutation_is_rejected(self) -> None:
        value = copy.deepcopy(self.contract)
        value["workloads"][0]["egress"][0]["ports"][0]["port"] = 6443
        self.assert_contract_rejected(value, "forbidden destination port 6443")

    def test_internet_wide_cidr_mutation_is_rejected(self) -> None:
        value = copy.deepcopy(self.contract)
        rule = value["workloads"][0]["egress"][0]
        rule["to"] = {"ip_block": "0.0.0.0/0"}
        rule["exception_id"] = "EX-N009-AUTH-OIDC-NODEPORT"
        self.assert_contract_rejected(value, "forbidden CIDR 0.0.0.0/0")

    def test_ip_block_without_registered_exception_is_rejected(self) -> None:
        value = copy.deepcopy(self.contract)
        rule = value["workloads"][0]["egress"][0]
        rule["to"] = {"ip_block": "192.0.2.1/32"}
        self.assert_contract_rejected(value, "lacks a registered exception")

    def test_same_namespace_blanket_mutation_is_rejected(self) -> None:
        value = copy.deepcopy(self.contract)
        rule = value["workloads"][0]["egress"][0]
        rule["to"] = {"namespace": "traffic-analysis", "pod_selector": {}}
        self.assert_contract_rejected(value, "selector must be non-empty")

    def test_rollout_overclaim_mutation_is_rejected(self) -> None:
        value = copy.deepcopy(self.contract)
        value["production_applied"] = True
        self.assert_contract_rejected(value, "fixed contract field drifted: production_applied")

    def test_missing_workload_mutation_is_rejected(self) -> None:
        value = copy.deepcopy(self.contract)
        value["workloads"].pop()
        self.assert_contract_rejected(value, "exact nine-service set")

    def test_missing_ingress_mutation_is_rejected(self) -> None:
        value = copy.deepcopy(self.contract)
        value["workloads"][0]["ingress"] = []
        self.assert_contract_rejected(value, "explicit ingress and egress")

    def test_relaxed_rendered_default_deny_is_rejected(self) -> None:
        actual = copy.deepcopy(builder.build(self.contract))
        actual[0]["spec"]["ingress"] = [{}]
        errors = verifier.validate(self.contract, actual)
        self.assertTrue(any("default-deny policy was relaxed" in error for error in errors))

    def test_non_dns_kube_system_render_mutation_is_rejected(self) -> None:
        actual = copy.deepcopy(builder.build(self.contract))
        actual[1]["spec"]["egress"][0]["ports"] = [{"protocol": "TCP", "port": 443}]
        errors = verifier.validate(self.contract, actual)
        self.assertTrue(any("non-DNS kube-system rule" in error for error in errors))


if __name__ == "__main__":
    unittest.main()
