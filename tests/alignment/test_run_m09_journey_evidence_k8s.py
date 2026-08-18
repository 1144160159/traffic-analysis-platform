from __future__ import annotations

import unittest

from scripts.alignment import run_m09_journey_evidence_k8s as runner


class M09JourneyEvidenceKubernetesRunnerTests(unittest.TestCase):
    def test_required_files_are_repository_scoped(self) -> None:
        files = runner.required_files()
        self.assertGreaterEqual(len(files), 14)
        self.assertEqual(len(files), len(set(files)))
        for relative in files:
            self.assertTrue((runner.ROOT / relative).is_file(), relative)

    def test_job_is_read_only_non_root_and_has_no_service_account_token(self) -> None:
        objects = runner.build_objects(
            runner.names("0123456789"),
            "traffic/m09-journey-evidence-test:m09-n023-20260816-r1",
            "01234567-89ab-4cde-8fab-0123456789ab",
            "8-2tb",
        )
        self.assertEqual(["ConfigMap", "Job"], [item["kind"] for item in objects])
        job = objects[1]
        pod_spec = job["spec"]["template"]["spec"]
        self.assertFalse(pod_spec["automountServiceAccountToken"])
        self.assertTrue(pod_spec["securityContext"]["runAsNonRoot"])
        container = pod_spec["containers"][0]
        self.assertTrue(container["securityContext"]["readOnlyRootFilesystem"])
        self.assertFalse(container["securityContext"]["allowPrivilegeEscalation"])
        self.assertEqual(
            ["--root", "/workspace", "--input-only", "--json"],
            container["args"],
        )
        annotations = job["metadata"]["annotations"]
        for value in annotations.values():
            self.assertEqual("false", value)

    def test_invalid_uuid_is_rejected_before_cluster_access(self) -> None:
        with self.assertRaisesRegex(runner.CanaryError, "run-id must be a UUID"):
            runner.validate_inputs("traffic/test:v1", "not-a-uuid", "8-2tb", 300)

    def test_invalid_node_is_rejected_before_cluster_access(self) -> None:
        with self.assertRaisesRegex(runner.CanaryError, "invalid Kubernetes node"):
            runner.validate_inputs(
                "traffic/test:v1",
                "01234567-89ab-4cde-8fab-0123456789ab",
                "../../node",
                300,
            )

    def test_timeout_is_bounded(self) -> None:
        with self.assertRaisesRegex(runner.CanaryError, "timeout"):
            runner.validate_inputs(
                "traffic/test:v1",
                "01234567-89ab-4cde-8fab-0123456789ab",
                "8-2tb",
                10,
            )


if __name__ == "__main__":
    unittest.main()
