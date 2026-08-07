from __future__ import annotations

import importlib.util
import unittest
from pathlib import Path


ROOT = Path(__file__).resolve().parents[2]
SPEC = importlib.util.spec_from_file_location(
    "verify_asset_projection_nebula_ephemeral",
    ROOT / "scripts/alignment/verify_asset_projection_nebula_ephemeral.py",
)
assert SPEC and SPEC.loader
MODULE = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(MODULE)


class AssetProjectionNebulaEphemeralGuardTest(unittest.TestCase):
    def test_names_are_deterministic_and_scoped(self) -> None:
        names = MODULE.names("asset-nebula-g1-v1")
        self.assertEqual(names, MODULE.names("asset-nebula-g1-v1"))
        for name in names:
            self.assertRegex(name, r"^codex-asset-projection-nebula-[a-z]+-[0-9a-f]{12}$")

    def test_all_images_are_digest_pinned(self) -> None:
        self.assertIn("nebula-metad@sha256:", MODULE.META_IMAGE)
        self.assertIn("nebula-storaged@sha256:", MODULE.STORAGE_IMAGE)
        self.assertIn("nebula-graphd@sha256:", MODULE.GRAPH_IMAGE)

    def test_sentinel_is_ephemeral_only(self) -> None:
        self.assertEqual(MODULE.SENTINEL_VALUE, "ephemeral-only")

    def test_empty_run_id_is_rejected(self) -> None:
        with self.assertRaisesRegex(ValueError, "run_id is required"):
            MODULE.names(" ")


if __name__ == "__main__":
    unittest.main()
