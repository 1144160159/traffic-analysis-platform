#!/usr/bin/env python3

from __future__ import annotations

import importlib.util
import json
import unittest
from pathlib import Path


SCRIPT = Path(__file__).with_name("run_m08_governed_k8s_canary.py")
SPEC = importlib.util.spec_from_file_location("run_m08_governed_k8s_canary", SCRIPT)
assert SPEC and SPEC.loader
MODULE = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(MODULE)


class GovernedMLOpsK8sCanaryTest(unittest.TestCase):
    run_id = "11111111-1111-4111-8111-111111111111"
    candidate = "a" * 64

    def test_inputs_bind_contracts_uuid_image_and_node(self) -> None:
        suffix = MODULE.validate_inputs(
            "docker.io/traffic/mlops-trainer:m08-test", self.candidate, self.run_id, "8-2tb"
        )
        self.assertEqual(suffix, "11111111")
        with self.assertRaises(MODULE.CanaryError):
            MODULE.validate_inputs(
                "docker.io/traffic/mlops-trainer:latest", self.candidate, self.run_id, "8-2tb"
            )

    def test_job_is_isolated_immutable_and_non_privileged(self) -> None:
        names = MODULE.resource_names("11111111")
        objects = MODULE.build_objects(
            names, "docker.io/traffic/mlops-trainer:m08-test",
            self.candidate, self.run_id, "8-2tb",
        )
        self.assertEqual([item["kind"] for item in objects], ["ConfigMap", "Job"])
        self.assertEqual(set(objects[0]["data"]), set(MODULE.CONTRACT_FILES))
        pod = objects[1]["spec"]["template"]
        container = pod["spec"]["containers"][0]
        self.assertEqual(container["imagePullPolicy"], "Never")
        self.assertTrue(container["securityContext"]["readOnlyRootFilesystem"])
        self.assertFalse(pod["spec"]["automountServiceAccountToken"])
        self.assertEqual(pod["spec"]["nodeName"], "8-2tb")
        self.assertNotIn("secretKeyRef", json.dumps(objects))
        self.assertNotIn("clickhouse", json.dumps(objects).lower())
        self.assertNotIn("minio", json.dumps(objects).lower())

    def test_result_requires_exact_lineage_counts_and_candidate(self) -> None:
        value = {
            "status": "PASS", "infrastructure": "kubernetes", "production_applied": False,
            "run_id": self.run_id, "candidate_sha256": self.candidate,
            "dataset_id": "22222222-2222-4222-8222-222222222222",
            "dataset_sha256": "b" * 64, "training_run_sha256": "c" * 64,
            "gnn_training_run_sha256": "e" * 64,
            "graph_snapshot_id": "55555555-5555-4555-8555-555555555555",
            "graph_snapshot_sha256": "f" * 64,
            "evaluation_id": "33333333-3333-4333-8333-333333333333",
            "evaluation_sha256": "d" * 64,
            "explanation_id": "44444444-4444-4444-8444-444444444444",
            "explanation_sha256": "9" * 64,
            "model_package_id": "66666666-6666-4666-8666-666666666666",
            "model_package_sha256": "8" * 64,
            "model_registration_receipt_id": "77777777-7777-4777-8777-777777777777",
            "model_registration_receipt_sha256": "7" * 64,
            "model_registration_request_sha256": "6" * 64,
            "artifact_count": 5, "gnn_artifact_count": 7, "evaluation_artifact_count": 2,
            "explanation_artifact_count": 3,
            "model_export_artifact_count": 5,
            "activation_authorized": False, "graph_ablation_state": "EVALUATED",
            "explanation_activation_authorized": False,
            "model_export_activation_authorized": False,
            "model_registration_status": "registered",
            "model_registration_revision": 1,
            "model_registration_activation_event_created": False,
            "model_registration_storage_mode": "synthetic_canary_receipt",
            "model_signing_key_id": f"ephemeral-canary/m08/{self.run_id}",
            "temporary_storage_removed_on_exit": True,
            "split_counts": {
                "train": {"row_count": 40}, "validation": {"row_count": 16},
                "test": {"row_count": 16}, "open_set": {"row_count": 8},
            },
        }
        self.assertEqual(
            MODULE.parse_result(json.dumps(value), self.run_id, self.candidate)["status"], "PASS"
        )
        value["split_counts"]["open_set"]["row_count"] = 0
        with self.assertRaisesRegex(MODULE.CanaryError, "split counts drifted"):
            MODULE.parse_result(json.dumps(value), self.run_id, self.candidate)


if __name__ == "__main__":
    unittest.main(verbosity=2)
