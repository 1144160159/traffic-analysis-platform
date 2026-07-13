import json
import subprocess
import sys
import tempfile
import unittest
from pathlib import Path

from PIL import Image


ROOT = Path(__file__).resolve().parents[2]
SCRIPT = ROOT / "tests/e2e/ui_visual_diff_metrics.py"


class UiVisualDiffMetricsTest(unittest.TestCase):
    def test_scoring_region_excludes_public_shell_pixels(self) -> None:
        with tempfile.TemporaryDirectory() as directory:
            directory_path = Path(directory)
            source_path = directory_path / "source.png"
            actual_path = directory_path / "actual.png"
            diff_path = directory_path / "diff.png"
            metrics_path = directory_path / "metrics.json"
            source = Image.new("RGBA", (4, 4), (8, 16, 24, 255))
            actual = source.copy()
            actual.putpixel((0, 0), (255, 0, 0, 255))
            actual.putpixel((2, 2), (255, 0, 0, 255))
            source.save(source_path)
            actual.save(actual_path)

            result = subprocess.run(
                [
                    sys.executable,
                    str(SCRIPT),
                    "--target-id",
                    "roi-test",
                    "--route",
                    "/roi-test",
                    "--source",
                    str(source_path),
                    "--actual",
                    str(actual_path),
                    "--diff",
                    str(diff_path),
                    "--metrics",
                    str(metrics_path),
                    "--max-pixel-ratio",
                    "0.2",
                    "--scoring-region",
                    "2,2,2,2",
                    "--scoring-region-id",
                    "business-content",
                ],
                check=False,
                capture_output=True,
                text=True,
            )

            self.assertEqual(result.returncode, 1)
            metrics = json.loads(metrics_path.read_text(encoding="utf-8"))
            visual_diff = metrics["visual_diff"]
            self.assertEqual(visual_diff["comparison_scope"], "scoring-region")
            self.assertEqual(visual_diff["scoring_region"], {
                "id": "business-content",
                "x": 2,
                "y": 2,
                "width": 2,
                "height": 2,
            })
            self.assertEqual(visual_diff["compared_pixels"], 4)
            self.assertEqual(visual_diff["mismatch_pixels"], 1)
            self.assertEqual(visual_diff["pixel_mismatch_ratio"], 0.25)
            self.assertEqual(visual_diff["full_image_diagnostic"]["mismatch_pixels"], 2)


if __name__ == "__main__":
    unittest.main()
