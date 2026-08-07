from __future__ import annotations

import copy
import sys
import unittest
from pathlib import Path
from unittest.mock import patch


ROOT = Path(__file__).resolve().parents[2]
SCRIPTS = ROOT / "scripts/alignment"
if str(SCRIPTS) not in sys.path:
    sys.path.insert(0, str(SCRIPTS))

import verify_minio_tls_cutover as verifier  # noqa: E402


class MinIOTLSCutoverTests(unittest.TestCase):
    @classmethod
    def setUpClass(cls) -> None:
        cls.contract = verifier._load_json(ROOT)
        cls.rendered = verifier._render(ROOT)

    def _verify_contract(self, mutate):
        contract = copy.deepcopy(self.contract)
        mutate(contract)
        with patch.object(verifier, "_load_json", return_value=contract):
            return verifier.verify(ROOT)

    def test_checked_in_bundle_is_valid_but_default_off(self) -> None:
        result = verifier.verify(ROOT)
        self.assertEqual("PASS", result["status"], result["errors"])
        self.assertTrue(result["default_off"])
        self.assertFalse(result["production_applied"])
        self.assertFalse(result["cutover_ready"])
        self.assertEqual(14, result["component_count"])
        self.assertGreaterEqual(result["rendered_resource_count"], 40)

    def test_false_cutover_readiness_claim_is_rejected(self) -> None:
        result = self._verify_contract(lambda value: value.update({"cutover_ready": True}))
        self.assertEqual("FAIL", result["status"])
        self.assertTrue(any("readiness" in error for error in result["errors"]))

    def test_local_image_cannot_claim_registry_distribution(self) -> None:
        def mutate(value):
            value["candidate_images"][0].update(
                {
                    "status": "SIGNED_AND_DISTRIBUTED",
                    "registry_digest": "sha256:" + "0" * 64,
                    "distributed_to_nodes": True,
                    "signed": True,
                }
            )

        result = self._verify_contract(mutate)
        self.assertEqual("FAIL", result["status"])
        self.assertTrue(any("must remain explicitly local" in error for error in result["errors"]))
        self.assertTrue(any("must not claim a registry digest" in error for error in result["errors"]))

    def test_local_image_id_cannot_be_used_as_registry_digest(self) -> None:
        def mutate(value):
            image = value["candidate_images"][0]
            image["registry_digest"] = image["local_image_id"]

        result = self._verify_contract(mutate)
        self.assertEqual("FAIL", result["status"])
        self.assertTrue(any("must not claim a registry digest" in error for error in result["errors"]))

    def test_rendered_workload_cannot_fall_back_to_the_base_image(self) -> None:
        regressed = self.rendered.replace(
            "docker.io/traffic/alert-service:minio-tls-8cb5c91b5f32",
            "docker.io/traffic/alert-service:stale-base",
            1,
        )
        with patch.object(verifier, "_render", return_value=regressed):
            result = verifier.verify(ROOT)
        self.assertEqual("FAIL", result["status"])
        self.assertTrue(any("alert-service does not render" in error for error in result["errors"]))

    def test_missing_component_is_rejected(self) -> None:
        result = self._verify_contract(lambda value: value["components"].pop())
        self.assertEqual("FAIL", result["status"])
        self.assertTrue(any("component inventory" in error for error in result["errors"]))

    def test_plaintext_minio_regression_is_rejected(self) -> None:
        regressed = self.rendered.replace(
            "https://minio.minio.svc:9000", "http://minio.minio.svc:9000", 1
        )
        with patch.object(verifier, "_render", return_value=regressed):
            result = verifier.verify(ROOT)
        self.assertEqual("FAIL", result["status"])
        self.assertTrue(any("http://minio" in error for error in result["errors"]))


if __name__ == "__main__":
    unittest.main()
