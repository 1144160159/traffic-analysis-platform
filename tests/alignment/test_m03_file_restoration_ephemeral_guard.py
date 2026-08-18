from __future__ import annotations

import importlib.util
import sys
import unittest
from pathlib import Path


ROOT = Path(__file__).resolve().parents[2]
sys.path.insert(0, str(ROOT / "scripts/alignment"))
SPEC = importlib.util.spec_from_file_location(
    "verify_m03_file_restoration_ephemeral",
    ROOT / "scripts/alignment/verify_m03_file_restoration_ephemeral.py",
)
assert SPEC and SPEC.loader
MODULE = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(MODULE)


class M03FileRestorationEphemeralGuardTest(unittest.TestCase):
    def test_container_names_are_deterministic_and_scoped(self) -> None:
        first = MODULE.names("m03-file-restoration-g1-v1")
        self.assertEqual(first, MODULE.names("m03-file-restoration-g1-v1"))
        self.assertRegex(first[0], r"^codex-m03-restoration-postgres-[0-9a-f]{12}$")
        self.assertRegex(first[1], r"^codex-m03-restoration-minio-[0-9a-f]{12}$")

    def test_empty_run_id_is_rejected(self) -> None:
        with self.assertRaisesRegex(ValueError, "run_id is required"):
            MODULE.names(" ")

    def test_images_are_digest_pinned(self) -> None:
        self.assertIn("postgres@sha256:", MODULE.POSTGRES_IMAGE)
        self.assertIn("minio/minio@sha256:", MODULE.MINIO_IMAGE)
        self.assertNotIn(":latest", MODULE.POSTGRES_IMAGE)
        self.assertNotIn(":latest", MODULE.MINIO_IMAGE)

    def test_migration_is_repository_owned(self) -> None:
        self.assertEqual(MODULE.MIGRATION.parent, ROOT / "deployments/postgres/migrations")
        self.assertTrue(MODULE.MIGRATION.is_file())
        self.assertTrue(MODULE.FRESH_SCHEMA.is_file())
        self.assertTrue(MODULE.KUBERNETES_SCHEMA.is_file())

    def test_kubernetes_expand_job_stays_suspended_and_default_off(self) -> None:
        import yaml

        documents = list(yaml.safe_load_all(MODULE.KUBERNETES_SCHEMA.read_text(encoding="utf-8")))
        job = next(item for item in documents if item and item.get("kind") == "Job")
        self.assertTrue(job["spec"]["suspend"])
        self.assertEqual(job["metadata"]["annotations"]["traffic.platform/production-applied"], "false")
        deployment = (ROOT / "deployments/kubernetes/applications/go-services.yaml").read_text(encoding="utf-8")
        self.assertIn('{name: RESTORATION_ENABLED, value: "false"}', deployment)

    def test_quarantine_bucket_requires_object_lock_and_runtime_cannot_delete(self) -> None:
        lifecycle = (ROOT / "deployments/kubernetes/init-jobs/06-minio-lifecycle.yaml").read_text(encoding="utf-8")
        self.assertIn("mc mb --ignore-existing --with-lock local/forensics-quarantine", lifecycle)
        policy = (ROOT / "deployments/kubernetes/security/minio-service-identities.v1.yaml").read_text(encoding="utf-8")
        forensics_policy = policy.split("traffic-forensics-service-v1.json:", 1)[1].split("traffic-mlops-model-writer-v1.json:", 1)[0]
        self.assertIn('"s3:PutObjectRetention"', forensics_policy)
        self.assertNotIn('"s3:DeleteObject"', forensics_policy)

    def test_credentials_are_not_inherited_from_external_environment(self) -> None:
        self.assertEqual(MODULE.POSTGRES_USER, "m03")
        self.assertTrue(MODULE.POSTGRES_PASSWORD.startswith("m03-restoration-"))
        self.assertEqual(MODULE.MINIO_ACCESS_KEY, "m03minio")
        self.assertTrue(MODULE.MINIO_SECRET_KEY.startswith("m03minio-restoration-"))


if __name__ == "__main__":
    unittest.main()
