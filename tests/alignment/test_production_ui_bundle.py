import json
import sys
import tempfile
import unittest
from pathlib import Path


ROOT = Path(__file__).resolve().parents[2]
sys.path.insert(0, str(ROOT / "scripts/alignment"))

from verify_production_ui_bundle import verify_bundle  # noqa: E402


class ProductionUiBundleScannerTest(unittest.TestCase):
    def write_bundle(self, root: Path, *, asset_source: str = "console.log('production');") -> None:
        (root / "assets").mkdir(parents=True)
        (root / ".vite").mkdir(parents=True)
        (root / "index.html").write_text('<script type="module" src="/assets/index-safe.js"></script>')
        (root / "assets/index-safe.js").write_text(asset_source)
        (root / ".vite/manifest.json").write_text(
            json.dumps({"index.html": {"file": "assets/index-safe.js", "isEntry": True}})
        )

    def test_accepts_mock_free_production_bundle(self) -> None:
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            self.write_bundle(root)
            result = verify_bundle(root)
            self.assertEqual(result["status"], "PASS", result["errors"])
            self.assertEqual(result["forbidden_paths"], [])
            self.assertEqual(result["forbidden_manifest_sources"], [])

    def test_rejects_worker_and_msw_content(self) -> None:
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            self.write_bundle(root, asset_source='const marker = "msw/passthrough";')
            (root / "mockServiceWorker.js").write_text("Mock Service Worker")
            result = verify_bundle(root)
            self.assertEqual(result["status"], "FAIL")
            self.assertIn("mockServiceWorker.js", result["forbidden_paths"])
            self.assertTrue(result["forbidden_tokens"])

    def test_rejects_mock_source_in_manifest(self) -> None:
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            self.write_bundle(root)
            (root / ".vite/manifest.json").write_text(
                json.dumps({"src/mocks/browser.ts": {"file": "assets/browser-bad.js"}})
            )
            result = verify_bundle(root)
            self.assertEqual(result["status"], "FAIL")
            self.assertEqual(result["forbidden_manifest_sources"], ["src/mocks/browser.ts"])

    def test_rejects_missing_index_asset(self) -> None:
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            self.write_bundle(root)
            (root / "assets/index-safe.js").unlink()
            result = verify_bundle(root)
            self.assertEqual(result["status"], "FAIL")
            self.assertEqual(result["missing_index_assets"], ["assets/index-safe.js"])


if __name__ == "__main__":
    unittest.main()
