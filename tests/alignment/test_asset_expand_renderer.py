from __future__ import annotations

import argparse
import importlib.util
import sys
import tempfile
import unittest
from datetime import datetime, timedelta, timezone
from pathlib import Path


ROOT = Path(__file__).resolve().parents[2]
sys.path.insert(0, str(ROOT / "scripts/alignment"))
SPEC = importlib.util.spec_from_file_location(
    "render_asset_postgres_expand",
    ROOT / "scripts/alignment/render_asset_postgres_expand.py",
)
assert SPEC and SPEC.loader
MODULE = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(MODULE)


class AssetExpandRendererTest(unittest.TestCase):
    def arguments(self, g0_manifest: Path) -> argparse.Namespace:
        now = datetime.now(timezone.utc)
        return argparse.Namespace(
            run_id="asset-expand-test-001",
            approval_id="CHANGE-ASSET-001",
            requested_by="platform-requester",
            approved_by="independent-sre",
            postgres_system_identifier="123456789",
            expected_migration_state=",".join("0" for _ in MODULE.MIGRATIONS),
            not_before=(now - timedelta(minutes=1)).isoformat(),
            expires_at=(now + timedelta(minutes=30)).isoformat(),
            g0_manifest=g0_manifest,
            output=Path("unused"),
        )

    def g0_manifest(self, directory: str) -> Path:
        candidate = MODULE.build_snapshot()["content_sha256"]
        manifest = Path(directory) / "manifest.json"
        manifest.write_text(
            '{"gate":"G0","status":"PASS","candidate_source":{"content_sha256":"%s"}}'
            % candidate,
            encoding="utf-8",
        )
        return manifest

    def test_render_is_suspended_immutable_and_hash_bound(self) -> None:
        with tempfile.TemporaryDirectory() as directory:
            documents = MODULE.render(
                self.arguments(self.g0_manifest(directory)), datetime.now(timezone.utc)
            )
        config_map, secret, job = documents
        self.assertTrue(config_map["immutable"])
        self.assertEqual(set(MODULE.MIGRATIONS), set(config_map["data"]))
        self.assertTrue(secret["immutable"])
        self.assertNotEqual(
            secret["stringData"]["requested_by"], secret["stringData"]["approved_by"]
        )
        self.assertTrue(job["spec"]["suspend"])
        self.assertEqual(0, job["spec"]["backoffLimit"])
        command = job["spec"]["template"]["spec"]["containers"][0]["args"][0]
        for token in (
            "APPROVED_POSTGRES_SYSTEM_IDENTIFIER",
            "APPROVED_EXPECTED_MIGRATION_STATE",
            "EXPECTED_G0_CANDIDATE_SHA256",
            "EXPECTED_MIGRATION_BUNDLE_SHA256",
            "actual_bundle_sha",
            'sha256sum "/migrations/$file"',
            "ON_ERROR_STOP=1",
            "REQUESTED_BY",
            "APPROVED_BY",
        ):
            self.assertIn(token, command)
        self.assertLessEqual(len(job["metadata"]["labels"]["traffic.io/run-id"]), 63)

    def test_rejects_self_approval(self) -> None:
        with tempfile.TemporaryDirectory() as directory:
            args = self.arguments(self.g0_manifest(directory))
            args.approved_by = args.requested_by.upper()
            with self.assertRaisesRegex(ValueError, "independent approver"):
                MODULE.render(args, datetime.now(timezone.utc))

    def test_rejects_incomplete_pre_state(self) -> None:
        with self.assertRaisesRegex(ValueError, "10 comma-separated"):
            MODULE.validate_state("0,0")


if __name__ == "__main__":
    unittest.main()
