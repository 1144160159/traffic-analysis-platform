from __future__ import annotations

import importlib.util
import shutil
import tempfile
import unittest
from pathlib import Path


ROOT = Path(__file__).resolve().parents[2]
SPEC = importlib.util.spec_from_file_location(
    "verify_asset_expand_guardrails",
    ROOT / "scripts/alignment/verify_asset_expand_guardrails.py",
)
assert SPEC and SPEC.loader
MODULE = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(MODULE)


class AssetExpandGuardrailsTest(unittest.TestCase):
    def test_repository_controls_pass(self) -> None:
        result = MODULE.verify(ROOT)
        self.assertEqual("PASS", result["status"], result["errors"])
        self.assertFalse(result["production_applied"])

    def test_projection_cannot_be_enabled_in_canonical_baseline(self) -> None:
        with tempfile.TemporaryDirectory() as directory:
            candidate = Path(directory)
            files = [
                MODULE.CONTRACT,
                MODULE.CONFIG,
                MODULE.CANONICAL_DEPLOYMENT,
                MODULE.COMPATIBILITY_DEPLOYMENT,
                MODULE.RENDERER,
                MODULE.EPHEMERAL_G1,
                MODULE.OPENSEARCH_G1,
                MODULE.KAFKA_G1,
                MODULE.OPENSEARCH_DEPLOYMENT,
                MODULE.OPENSEARCH_DOCKERFILE,
                MODULE.IMAGE_LOCK,
                MODULE.RUNBOOK,
            ]
            contract = __import__("json").loads((ROOT / MODULE.CONTRACT).read_text())
            files.extend(
                MODULE.MIGRATION_DIR / path.name
                for version in contract["migration_versions"]
                for path in (ROOT / MODULE.MIGRATION_DIR).glob(f"{version}_*.sql")
            )
            for relative in files:
                target = candidate / relative
                target.parent.mkdir(parents=True, exist_ok=True)
                shutil.copy2(ROOT / relative, target)
            manifest = candidate / MODULE.CANONICAL_DEPLOYMENT
            manifest.write_text(
                manifest.read_text().replace(
                    '{name: ASSET_PROJECTION_ENABLED, value: "false"}',
                    '{name: ASSET_PROJECTION_ENABLED, value: "true"}',
                    1,
                )
            )
            result = MODULE.verify(candidate)
        self.assertEqual("FAIL", result["status"])
        self.assertTrue(any("ASSET_PROJECTION_ENABLED" in error for error in result["errors"]))


if __name__ == "__main__":
    unittest.main()
