from __future__ import annotations

import unittest
import copy

import yaml

from scripts.alignment import capture_m10_apisix_route_diff as capture
from scripts.alignment import verify_m10_apisix_route_diff as verifier


class M10ApisixRouteMaterializationTests(unittest.TestCase):
    @classmethod
    def setUpClass(cls) -> None:
        cls.expected_evidence = capture.capture()

    def test_equal_route_sets_have_zero_diff(self) -> None:
        routes = [{"id": 1, "uri": "/a", "plugins": {"request-id": {}}}]
        self.assertTrue(capture.compare_routes(routes, routes)["zero_diff"])

    def test_missing_extra_and_changed_routes_are_exact(self) -> None:
        candidate = [{"id": 1, "uri": "/a"}, {"id": 2, "uri": "/b"}]
        live = [{"id": 1, "uri": "/changed"}, {"id": 3, "uri": "/c"}]
        diff = capture.compare_routes(candidate, live)
        self.assertEqual(["2"], diff["missing_live_ids"])
        self.assertEqual(["3"], diff["extra_live_ids"])
        self.assertEqual(["1"], diff["changed_ids"])
        self.assertFalse(diff["zero_diff"])

    def test_duplicate_route_id_is_rejected(self) -> None:
        with self.assertRaisesRegex(ValueError, "unique"):
            capture.route_index([{"id": 1}, {"id": 1}])

    def test_candidate_has_complete_route_policy(self) -> None:
        _, routes, statefulset, catalog = capture.load_candidate()
        self.assertEqual(58, len(routes))
        self.assertEqual(0, catalog["counts"]["routes_with_blocking_gaps"])
        self.assertEqual(52, catalog["counts"]["protected_routes_with_gateway_auth"])
        self.assertTrue(capture.workload_policy(statefulset)["policy_complete"])

    def test_candidate_secret_references_are_names_only(self) -> None:
        _, _, statefulset, _ = capture.load_candidate()
        policy = capture.workload_policy(statefulset)
        self.assertEqual("traffic-platform-ca", policy["ca_secret"])
        documents = list(yaml.safe_load_all((capture.ROOT / capture.GATEWAY_SOURCE).read_text()))
        self.assertNotIn("non-production-schema-validation-secret", str(documents[0]))

    def test_current_live_evidence_passes_semantic_verifier(self) -> None:
        actual = verifier.load(verifier.OUTPUT)
        self.assertEqual([], verifier.validate(self.expected_evidence, actual))

    def test_false_acceptance_is_rejected(self) -> None:
        actual = copy.deepcopy(self.expected_evidence)
        actual["acceptance_status"] = "PASS"
        self.assertTrue(any("acceptance status" in error for error in verifier.validate(self.expected_evidence, actual)))

    def test_route_diff_blocker_removal_is_rejected(self) -> None:
        actual = copy.deepcopy(self.expected_evidence)
        actual["blocking_codes"].remove("LIVE_ROUTE_SET_OR_CONTENT_DIFF")
        self.assertTrue(any("route diff blocker" in error for error in verifier.validate(self.expected_evidence, actual)))

    def test_secret_value_overclaim_is_rejected(self) -> None:
        actual = copy.deepcopy(self.expected_evidence)
        actual["secret_key_observation"]["values_captured"] = True
        self.assertTrue(any("secret value capture" in error for error in verifier.validate(self.expected_evidence, actual)))

    def test_production_overclaim_is_rejected(self) -> None:
        actual = copy.deepcopy(self.expected_evidence)
        actual["production_applied"] = True
        self.assertTrue(any("production application" in error for error in verifier.validate(self.expected_evidence, actual)))


if __name__ == "__main__":
    unittest.main()
