from __future__ import annotations

import importlib.util
import sys
import unittest
from pathlib import Path


ROOT = Path(__file__).resolve().parents[2]
sys.path.insert(0, str(ROOT / "scripts/alignment"))
SPEC = importlib.util.spec_from_file_location(
    "verify_asset_expand_ephemeral",
    ROOT / "scripts/alignment/verify_asset_expand_ephemeral.py",
)
assert SPEC and SPEC.loader
MODULE = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(MODULE)


class AssetExpandEphemeralGuardTest(unittest.TestCase):
    def test_container_identity_is_deterministic_and_scoped(self) -> None:
        first = MODULE.container_name("asset-expand-g1-v1")
        second = MODULE.container_name("asset-expand-g1-v1")
        self.assertEqual(first, second)
        self.assertRegex(first, r"^codex-asset-expand-pg-[0-9a-f]{12}$")

    def test_bootstrap_is_sentinel_and_documentation_only(self) -> None:
        self.assertIn("codex_ephemeral_asset_expand_sentinel", MODULE.BASE_SCHEMA)
        self.assertIn("ephemeral-only", MODULE.BASE_SCHEMA)
        self.assertIn("192.0.2.10", MODULE.BASE_SCHEMA)
        self.assertIn("postgres@sha256:", MODULE.POSTGRES_IMAGE)

    def test_empty_run_id_is_rejected(self) -> None:
        with self.assertRaisesRegex(ValueError, "run_id is required"):
            MODULE.container_name("  ")


if __name__ == "__main__":
    unittest.main()
