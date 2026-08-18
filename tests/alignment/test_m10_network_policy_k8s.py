from __future__ import annotations

import copy
import unittest

from scripts.alignment import run_m10_network_policy_k8s as runner
from scripts.alignment import verify_m10_network_policy_k8s as verifier


class M10NetworkPolicyK8sEvidenceTests(unittest.TestCase):
    @classmethod
    def setUpClass(cls) -> None:
        cls.evidence = verifier.load(runner.OUTPUT)

    def test_current_evidence_is_fail_closed(self) -> None:
        self.assertEqual([], verifier.validate(self.evidence))

    def test_runner_has_no_mutating_apply_path(self) -> None:
        source = PathLike.read_text(runner.__file__)
        self.assertEqual(1, source.count('kubectl("apply"'))
        self.assertIn('kubectl("apply", "--dry-run=client"', source)
        self.assertNotIn('kubectl("create"', source)
        self.assertNotIn('kubectl("delete"', source)

    def test_enforcement_cni_with_blocked_claim_is_rejected(self) -> None:
        value = copy.deepcopy(self.evidence)
        value["cluster"]["policy_enforcement_cnis"] = ["cilium"]
        self.assertTrue(any("cannot coexist" in error for error in verifier.validate(value)))

    def test_fake_probe_pass_is_rejected(self) -> None:
        value = copy.deepcopy(self.evidence)
        value["negative_probes"]["unauthorized_port"] = "PASS_DENIED"
        self.assertTrue(any("falsely claim" in error for error in verifier.validate(value)))

    def test_production_apply_overclaim_is_rejected(self) -> None:
        value = copy.deepcopy(self.evidence)
        value["production_applied"] = True
        self.assertTrue(any("production application" in error for error in verifier.validate(value)))

    def test_candidate_presence_is_rejected(self) -> None:
        value = copy.deepcopy(self.evidence)
        value["candidate"]["present_in_target_namespace"] = ["m10-n009-default-deny"]
        self.assertTrue(any("candidate apply" in error for error in verifier.validate(value)))

    def test_cni_blocker_removal_is_rejected(self) -> None:
        value = copy.deepcopy(self.evidence)
        value["blocking_codes"].remove("CNI_POLICY_ENFORCEMENT_REQUIRED")
        self.assertTrue(any("required blocker removed" in error for error in verifier.validate(value)))

    def test_source_hash_mutation_is_rejected(self) -> None:
        value = copy.deepcopy(self.evidence)
        first = next(iter(value["source_sha256"]))
        value["source_sha256"][first] = "0" * 64
        self.assertTrue(any("source hashes drifted" in error for error in verifier.validate(value)))


class PathLike:
    @staticmethod
    def read_text(path: str) -> str:
        with open(path, encoding="utf-8") as handle:
            return handle.read()


if __name__ == "__main__":
    unittest.main()
