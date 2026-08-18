import copy
import importlib.util
import json
import shutil
import tempfile
import unittest
from pathlib import Path


ROOT = Path(__file__).resolve().parents[2]
SCRIPT = ROOT / "scripts/alignment/verify_minio_object_governance.py"
SPEC = importlib.util.spec_from_file_location("verify_minio_object_governance", SCRIPT)
MODULE = importlib.util.module_from_spec(SPEC)
assert SPEC.loader is not None
SPEC.loader.exec_module(MODULE)


class MinioObjectGovernanceTest(unittest.TestCase):
    def setUp(self):
        self.contract = json.loads(MODULE.CONTRACT.read_text(encoding="utf-8"))

    def test_repository_baseline_passes_without_live_claim(self):
        result = MODULE.verify()
        self.assertEqual(result["status"], "PASS", result["errors"])
        self.assertFalse(result["production_applied"])
        self.assertEqual(result["bucket_count"], 6)
        self.assertEqual(result["object_class_count"], 9)
        self.assertEqual(
            result["governed_bootstrap_buckets"],
            ["argo-artifacts", "flink-checkpoints", "forensics-quarantine", "pcap-archive", "report-artifacts", "traffic-models"],
        )
        self.assertEqual(result["governed_bootstrap_buckets"], result["governed_bootstrap_verified_buckets"])

    def test_missing_governed_bootstrap_bucket_is_rejected(self):
        with tempfile.TemporaryDirectory() as directory:
            candidate = Path(directory) / "repo"
            for source in (MODULE.CONTRACT, MODULE.LIFECYCLE, MODULE.INFRASTRUCTURE, *MODULE.MODEL_SOURCES, *MODULE.WORKFLOWS):
                target = candidate / source.relative_to(ROOT)
                target.parent.mkdir(parents=True, exist_ok=True)
                shutil.copy2(source, target)
            lifecycle = candidate / MODULE.LIFECYCLE.relative_to(ROOT)
            text = lifecycle.read_text(encoding="utf-8")
            text = text.replace("          mc mb --ignore-existing local/traffic-models\n", "")
            lifecycle.write_text(text, encoding="utf-8")
            result = MODULE.verify(root=candidate)
        self.assertEqual(result["status"], "FAIL")
        self.assertTrue(any("bootstrap bucket drift" in error for error in result["errors"]))

    def test_false_closure_or_production_claim_is_rejected(self):
        mutated = copy.deepcopy(self.contract)
        mutated["status"] = "closed"
        mutated["production_applied"] = True
        result = MODULE.verify(contract=mutated)
        self.assertEqual(result["status"], "FAIL")
        self.assertTrue(any("production_applied=false" in error for error in result["errors"]))

    def test_missing_object_class_is_rejected(self):
        mutated = copy.deepcopy(self.contract)
        mutated["bucket_registry"][-1]["object_classes"] = []
        result = MODULE.verify(contract=mutated)
        self.assertEqual(result["status"], "FAIL")
        self.assertTrue(any("object class coverage" in error for error in result["errors"]))

    def test_application_bucket_creation_is_rejected(self):
        errors = MODULE.source_policy_errors(
            "client.make_bucket(bucket)\n",
            "candidate.py",
        )
        self.assertTrue(any("bucket creation" in error for error in errors))

    def test_minio_credential_fallback_is_rejected(self):
        errors = MODULE.source_policy_errors(
            "access_key = os.getenv('MINIO_ACCESS_KEY', 'demo')\n",
            "candidate.py",
        )
        self.assertTrue(any("getenv fallback" in error for error in errors))

    def test_literal_insecure_client_is_rejected(self):
        errors = MODULE.source_policy_errors(
            "client = Minio(endpoint, secure=False)\n",
            "candidate.py",
        )
        self.assertTrue(any("literal insecure" in error for error in errors))


if __name__ == "__main__":
    unittest.main()
