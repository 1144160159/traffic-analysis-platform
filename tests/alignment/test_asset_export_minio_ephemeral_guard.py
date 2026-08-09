from __future__ import annotations

import importlib.util
import sys
import unittest
from pathlib import Path


ROOT = Path(__file__).resolve().parents[2]
sys.path.insert(0, str(ROOT / "scripts/alignment"))
SPEC = importlib.util.spec_from_file_location(
    "verify_asset_export_minio_ephemeral",
    ROOT / "scripts/alignment/verify_asset_export_minio_ephemeral.py",
)
assert SPEC and SPEC.loader
MODULE = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(MODULE)


class AssetExportMinIOEphemeralGuardTest(unittest.TestCase):
    def test_container_names_are_deterministic_and_scoped(self) -> None:
        postgres, minio = MODULE.names("asset-export-minio-g1-v1")
        self.assertEqual((postgres, minio), MODULE.names("asset-export-minio-g1-v1"))
        self.assertRegex(postgres, r"^codex-asset-export-minio-pg-[0-9a-f]{12}$")
        self.assertRegex(minio, r"^codex-asset-export-minio-store-[0-9a-f]{12}$")

    def test_images_are_digest_pinned(self) -> None:
        self.assertIn("postgres@sha256:", MODULE.POSTGRES_IMAGE)
        self.assertIn("minio/minio@sha256:", MODULE.MINIO_IMAGE)

    def test_sentinel_and_bucket_are_ephemeral_scoped(self) -> None:
        self.assertEqual(MODULE.SENTINEL_VALUE, "ephemeral-only")
        self.assertEqual(MODULE.SENTINEL_TABLE, "codex_ephemeral_asset_atomic_test_sentinel")
        self.assertEqual(MODULE.MINIO_BUCKET, "asset-export-g1")

    def test_empty_run_id_is_rejected(self) -> None:
        with self.assertRaisesRegex(ValueError, "run_id is required"):
            MODULE.names(" ")


if __name__ == "__main__":
    unittest.main()
