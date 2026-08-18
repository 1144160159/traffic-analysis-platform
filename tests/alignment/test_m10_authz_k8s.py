from __future__ import annotations

import copy
import unittest

from scripts.alignment import run_m10_authz_k8s as runner
from scripts.alignment import verify_m10_authz_k8s as verifier


class M10AuthzKubernetesTests(unittest.TestCase):
    @classmethod
    def setUpClass(cls) -> None:
        cls.evidence = verifier.load(runner.OUTPUT)

    def test_current_evidence_is_valid(self) -> None:
        self.assertEqual([], verifier.validate(self.evidence))

    def test_input_validation_rejects_latest_and_single_node(self) -> None:
        with self.assertRaisesRegex(Exception, "non-latest"):
            runner.validate_inputs("repo/image:latest", "7c9a327e-3d58-4e42-9a41-0a789b414966", ["8-2tb", "zeus-server"], 180)
        with self.assertRaisesRegex(Exception, "two distinct"):
            runner.validate_inputs("repo/image:v1", "7c9a327e-3d58-4e42-9a41-0a789b414966", ["8-2tb"], 180)

    def test_job_is_non_root_read_only_and_has_no_service_account_token(self) -> None:
        job = runner.job_object("m10-n007-test", "repo/image:v1", "7c9a327e-3d58-4e42-9a41-0a789b414966", "8-2tb")
        pod = job["spec"]["template"]["spec"]
        container = pod["containers"][0]
        self.assertFalse(pod["automountServiceAccountToken"])
        self.assertTrue(pod["securityContext"]["runAsNonRoot"])
        self.assertEqual(65532, pod["securityContext"]["runAsUser"])
        self.assertTrue(container["securityContext"]["readOnlyRootFilesystem"])
        self.assertFalse(container["securityContext"]["allowPrivilegeEscalation"])
        self.assertEqual(["ALL"], container["securityContext"]["capabilities"]["drop"])

    def test_acceptance_overclaim_is_rejected(self) -> None:
        actual = copy.deepcopy(self.evidence)
        actual["acceptance_status"] = "PASS"
        self.assertTrue(any("overclaims" in error for error in verifier.validate(actual)))

    def test_runtime_enablement_blocker_removal_is_rejected(self) -> None:
        actual = copy.deepcopy(self.evidence)
        actual["blocking_codes"].remove("AUTHZ_POLICY_RUNTIME_ENABLEMENT_REQUIRED")
        self.assertTrue(any("required blocker" in error for error in verifier.validate(actual)))

    def test_dependency_blocker_removal_is_rejected(self) -> None:
        actual = copy.deepcopy(self.evidence)
        actual["blocking_codes"].remove("N006_ROUTE_MATERIALIZATION_REQUIRED")
        self.assertTrue(any("dependency blocker" in error for error in verifier.validate(actual)))

    def test_cross_node_image_drift_is_rejected(self) -> None:
        actual = copy.deepcopy(self.evidence)
        actual["kubernetes_jobs"][1]["identity"]["image_id"] = "sha256:drift"
        self.assertTrue(any("immutable image ID" in error for error in verifier.validate(actual)))

    def test_cleanup_overclaim_is_rejected(self) -> None:
        actual = copy.deepcopy(self.evidence)
        actual["run_scoped_resources_removed"] = False
        self.assertTrue(any("not recorded as removed" in error for error in verifier.validate(actual)))


if __name__ == "__main__":
    unittest.main()
