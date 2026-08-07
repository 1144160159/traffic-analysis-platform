import copy
import importlib.util
import json
import shutil
import tempfile
import unittest
from pathlib import Path


ROOT = Path(__file__).resolve().parents[2]
SCRIPT = ROOT / "scripts/alignment/verify_minio_service_identities.py"
SPEC = importlib.util.spec_from_file_location("verify_minio_service_identities", SCRIPT)
MODULE = importlib.util.module_from_spec(SPEC)
assert SPEC.loader is not None
SPEC.loader.exec_module(MODULE)


class MinioServiceIdentitiesTest(unittest.TestCase):
    def setUp(self):
        self.contract = json.loads(MODULE.CONTRACT.read_text(encoding="utf-8"))

    def copy_fixture(self, target_root: Path) -> None:
        paths = {MODULE.CONTRACT, MODULE.IDENTITY_MANIFEST, MODULE.EXTERNAL_SECRETS}
        for identity in self.contract["identities"]:
            paths.update(ROOT / relative for relative in identity["consumers"])
        for source in paths:
            target = target_root / source.relative_to(ROOT)
            target.parent.mkdir(parents=True, exist_ok=True)
            shutil.copy2(source, target)

    def test_repository_expand_phase_passes_without_live_claim(self):
        result = MODULE.verify()
        self.assertEqual(result["status"], "PASS", result["errors"])
        self.assertFalse(result["production_applied"])
        self.assertEqual(result["identity_count"], 7)
        self.assertEqual(result["policy_count"], 7)
        self.assertEqual(result["consumer_manifest_count"], 11)

    def test_unsuspended_bootstrap_job_is_rejected(self):
        with tempfile.TemporaryDirectory() as directory:
            candidate = Path(directory) / "repo"
            self.copy_fixture(candidate)
            manifest = candidate / MODULE.IDENTITY_MANIFEST.relative_to(ROOT)
            text = manifest.read_text(encoding="utf-8").replace("  suspend: true\n", "  suspend: false\n", 1)
            manifest.write_text(text, encoding="utf-8")
            result = MODULE.verify(root=candidate)
        self.assertEqual(result["status"], "FAIL")
        self.assertTrue(any("must be suspended" in error for error in result["errors"]))

    def test_wildcard_action_is_rejected(self):
        with tempfile.TemporaryDirectory() as directory:
            candidate = Path(directory) / "repo"
            self.copy_fixture(candidate)
            manifest = candidate / MODULE.IDENTITY_MANIFEST.relative_to(ROOT)
            text = manifest.read_text(encoding="utf-8").replace('"s3:PutObject"', '"s3:*"', 1)
            manifest.write_text(text, encoding="utf-8")
            result = MODULE.verify(root=candidate)
        self.assertEqual(result["status"], "FAIL")
        self.assertTrue(any("wildcard or administrative action" in error for error in result["errors"]))

    def test_application_root_credential_reference_is_rejected(self):
        with tempfile.TemporaryDirectory() as directory:
            candidate = Path(directory) / "repo"
            self.copy_fixture(candidate)
            manifest = candidate / "deployments/kubernetes/applications/probe-agent.yaml"
            text = manifest.read_text(encoding="utf-8")
            text = text.replace("name: minio-probe-agent-credentials\n              key: accesskey", "name: traffic-credentials\n              key: MINIO_ACCESS_KEY", 1)
            manifest.write_text(text, encoding="utf-8")
            result = MODULE.verify(root=candidate)
        self.assertEqual(result["status"], "FAIL")
        self.assertTrue(any("application root MinIO credential" in error for error in result["errors"]))

    def test_missing_external_identity_secret_is_rejected(self):
        with tempfile.TemporaryDirectory() as directory:
            candidate = Path(directory) / "repo"
            self.copy_fixture(candidate)
            manifest = candidate / MODULE.EXTERNAL_SECRETS.relative_to(ROOT)
            text = manifest.read_text(encoding="utf-8").replace(
                "metadata: {name: minio-probe-agent-credentials, namespace: traffic-analysis}",
                "metadata: {name: removed-probe-agent-credentials, namespace: traffic-analysis}",
                1,
            )
            manifest.write_text(text, encoding="utf-8")
            result = MODULE.verify(root=candidate)
        self.assertEqual(result["status"], "FAIL")
        self.assertTrue(any("minio-probe-agent-credentials ExternalSecret is missing" in error for error in result["errors"]))

    def test_policy_size_budget_is_enforced(self):
        mutated = copy.deepcopy(self.contract)
        mutated["guardrails"]["policy_max_bytes"] = 100
        result = MODULE.verify(contract=mutated)
        self.assertEqual(result["status"], "FAIL")
        self.assertTrue(any("exceeds 100" in error for error in result["errors"]))


if __name__ == "__main__":
    unittest.main()
