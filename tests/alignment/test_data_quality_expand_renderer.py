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
    "render_data_quality_postgres_expand",
    ROOT / "scripts/alignment/render_data_quality_postgres_expand.py",
)
assert SPEC and SPEC.loader
MODULE = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(MODULE)


class DataQualityExpandRendererTest(unittest.TestCase):
    def arguments(self, g0_manifest: Path) -> argparse.Namespace:
        now = datetime.now(timezone.utc)
        return argparse.Namespace(
            run_id="dq-expand-test-001",
            approval_id="CHANGE-001",
            approved_by="independent-reviewer",
            postgres_system_identifier="123456789",
            expected_migration_state="0,0,0,0,0",
            not_before=(now - timedelta(minutes=1)).isoformat(),
            expires_at=(now + timedelta(minutes=30)).isoformat(),
            g0_manifest=g0_manifest,
            output=Path("unused"),
        )

    def test_render_is_suspended_immutable_and_binds_guards(self) -> None:
        candidate = MODULE.build_snapshot()["content_sha256"]
        with tempfile.TemporaryDirectory() as directory:
            manifest = Path(directory) / "g0.json"
            manifest.write_text(
                '{"gate":"G0","status":"PASS","candidate_source":{"content_sha256":"%s"}}' % candidate,
                encoding="utf-8",
            )
            config_map, secret, job = MODULE.render(self.arguments(manifest), datetime.now(timezone.utc))
        self.assertTrue(config_map["immutable"])
        self.assertEqual(set(config_map["data"]), set(MODULE.MIGRATIONS))
        self.assertTrue(secret["immutable"])
        self.assertEqual("0,0,0,0,0", secret["stringData"]["expected_migration_state"])
        self.assertTrue(job["spec"]["suspend"])
        self.assertEqual(0, job["spec"]["backoffLimit"])
        command = job["spec"]["template"]["spec"]["containers"][0]["args"][0]
        for token in (
            "APPROVED_CHANGE_ID", "EXPECTED_CHANGE_ID", "APPROVED_BY", "EXPECTED_APPROVER",
            "APPROVED_POSTGRES_SYSTEM_IDENTIFIER", "APPROVED_EXPECTED_MIGRATION_STATE",
            "EXPECTED_G0_CANDIDATE_SHA256", "EXPECTED_MIGRATION_BUNDLE_SHA256",
            "ON_ERROR_STOP=1", 'test "$final_state" = "1,1,1,1,1"',
        ):
            self.assertIn(token, command)
        env = {item["name"]: item for item in job["spec"]["template"]["spec"]["containers"][0]["env"]}
        self.assertEqual("CHANGE-001", env["EXPECTED_CHANGE_ID"]["value"])
        self.assertEqual("independent-reviewer", env["EXPECTED_APPROVER"]["value"])
        self.assertLessEqual(len(job["metadata"]["labels"]["traffic.io/run-id"]), 63)

    def test_rejects_invalid_state(self) -> None:
        with self.assertRaisesRegex(ValueError, "five comma-separated"):
            MODULE.validate_state("0,0")


if __name__ == "__main__":
    unittest.main()
