from __future__ import annotations

import importlib.util
import sys
import unittest
from pathlib import Path


ROOT = Path(__file__).resolve().parents[2]
sys.path.insert(0, str(ROOT / "scripts/alignment"))
SPEC = importlib.util.spec_from_file_location(
    "verify_asset_projection_opensearch_ephemeral",
    ROOT / "scripts/alignment/verify_asset_projection_opensearch_ephemeral.py",
)
assert SPEC and SPEC.loader
MODULE = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(MODULE)


class AssetProjectionOpenSearchEphemeralGuardTest(unittest.TestCase):
    def test_container_identity_is_deterministic_and_scoped(self) -> None:
        name = MODULE.container_name("asset-projection-opensearch-g1-v1")
        self.assertEqual(name, MODULE.container_name("asset-projection-opensearch-g1-v1"))
        self.assertRegex(name, r"^codex-asset-projection-os-[0-9a-f]{12}$")

    def test_authority_uses_pinned_image_and_ephemeral_sentinel(self) -> None:
        self.assertIn("opensearchproject/opensearch@sha256:", MODULE.OPENSEARCH_IMAGE)
        self.assertEqual(MODULE.SENTINEL_VALUE, "ephemeral-only")
        self.assertIn("codex-ephemeral", MODULE.SENTINEL_INDEX)

    def test_asset_template_preserves_strict_mapping(self) -> None:
        template = MODULE.asset_template()
        self.assertEqual(template["template"]["mappings"]["dynamic"], "strict")
        self.assertEqual(template["template"]["settings"]["number_of_replicas"], 0)
        properties = template["template"]["mappings"]["properties"]
        self.assertEqual(properties["revision"]["type"], "long")
        self.assertEqual(properties["asset_id"]["type"], "keyword")

    def test_runner_executes_production_alert_writer_against_strict_mapping(self) -> None:
        source = (ROOT / "scripts/alignment/verify_asset_projection_opensearch_ephemeral.py").read_text(encoding="utf-8")
        self.assertEqual(MODULE.ALERT_MAPPING_AUTHORITY.as_posix(), "common/opensearch/alerts-v2/mappings-component.json")
        self.assertIn("TestAlertWriterRealStrictOpenSearchMapping", source)
        self.assertIn('"production_alert_writer_verified": False', source)

    def test_runner_requires_post_repair_terminal_receipt(self) -> None:
        source = (ROOT / "scripts/alignment/verify_asset_projection_opensearch_ephemeral.py").read_text(encoding="utf-8")
        self.assertIn("TestAlertProjectionRepairTerminalReceiptRealOpenSearch", source)
        self.assertIn('"projection_repair_terminal_receipt_verified": False', source)
        self.assertIn('"projection_watermark_receipt_guard_verified": False', source)

    def test_empty_run_id_is_rejected(self) -> None:
        with self.assertRaisesRegex(ValueError, "run_id is required"):
            MODULE.container_name("  ")


if __name__ == "__main__":
    unittest.main()
