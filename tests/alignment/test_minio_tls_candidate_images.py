from __future__ import annotations

import copy
import sys
import unittest
from pathlib import Path


ROOT = Path(__file__).resolve().parents[2]
SCRIPTS = ROOT / "scripts/alignment"
if str(SCRIPTS) not in sys.path:
    sys.path.insert(0, str(SCRIPTS))

import capture_minio_tls_candidate_images as capture  # noqa: E402


class MinIOTLSCandidateImageTests(unittest.TestCase):
    def setUp(self) -> None:
        self.contract = {
            "workload": "alert-service",
            "status": "BUILT_LOCAL_NOT_DISTRIBUTED",
            "image": "docker.io/traffic/alert-service:minio-tls-" + "a" * 12,
            "local_image_id": "sha256:" + "b" * 64,
            "registry_digest": None,
            "component_source_sha256": "a" * 64,
            "platform": "linux/amd64",
            "distributed_to_nodes": False,
            "signed": False,
        }
        self.inspection = {
            "RepoTags": ["traffic/alert-service:minio-tls-" + "a" * 12],
            "RepoDigests": [],
            "Id": "sha256:" + "b" * 64,
            "Os": "linux",
            "Architecture": "amd64",
            "Config": {
                "Labels": {
                    "org.opencontainers.image.revision": "a" * 64,
                    "traffic.analysis.component-source-sha256": "a" * 64,
                    "traffic.analysis.remediation-ids": "T-MINIO-003,T-PKI-001",
                }
            },
        }

    def test_matching_local_image_is_accepted(self) -> None:
        self.assertEqual([], capture.validate_inspection(self.contract, self.inspection))

    def test_mismatched_local_image_id_is_rejected(self) -> None:
        inspection = copy.deepcopy(self.inspection)
        inspection["Id"] = "sha256:" + "c" * 64
        errors = capture.validate_inspection(self.contract, inspection)
        self.assertTrue(any("local image ID" in error for error in errors))

    def test_registry_digest_cannot_be_silently_invented(self) -> None:
        inspection = copy.deepcopy(self.inspection)
        inspection["RepoDigests"] = ["traffic/alert-service@sha256:" + "d" * 64]
        errors = capture.validate_inspection(self.contract, inspection)
        self.assertTrue(any("registry repo digest" in error for error in errors))

    def test_source_label_drift_is_rejected(self) -> None:
        inspection = copy.deepcopy(self.inspection)
        inspection["Config"]["Labels"]["traffic.analysis.component-source-sha256"] = "e" * 64
        errors = capture.validate_inspection(self.contract, inspection)
        self.assertTrue(any("component source label" in error for error in errors))


if __name__ == "__main__":
    unittest.main()
