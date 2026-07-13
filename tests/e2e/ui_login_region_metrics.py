#!/usr/bin/env python3
"""Report region-level visual deltas for the login page iteration loop."""

from __future__ import annotations

import argparse
import json
from datetime import datetime, timezone
from pathlib import Path

from PIL import Image, ImageChops, ImageStat


LOGIN_REGIONS = [
    ("full", (0, 0, 1920, 1080)),
    ("left_visual", (0, 0, 960, 1080)),
    ("right_panel", (1018, 132, 1738, 897)),
    ("campus_band", (0, 330, 900, 545)),
    ("hero_shield", (220, 210, 735, 520)),
    ("hero_title", (110, 520, 870, 650)),
    ("capability_buttons", (155, 660, 800, 735)),
    ("assurance_list", (250, 748, 720, 900)),
    ("panel_tabs", (1050, 165, 1705, 238)),
    ("panel_fields", (1050, 280, 1705, 650)),
    ("panel_footer", (1050, 675, 1705, 842)),
    ("bottom_wave", (0, 880, 1920, 1080)),
]


def parse_args() -> argparse.Namespace:
    parser = argparse.ArgumentParser(description="Compare login source/actual screenshots by named regions.")
    parser.add_argument("--source", type=Path, required=True, help="Target login PNG.")
    parser.add_argument("--actual", type=Path, required=True, help="Actual login screenshot.")
    parser.add_argument("--output", type=Path, required=True, help="Output JSON report.")
    parser.add_argument("--tolerance", type=int, default=12, help="Per-channel tolerance for approximate mismatch.")
    return parser.parse_args()


def pixel_data(image: Image.Image):
    if hasattr(image, "get_flattened_data"):
        return image.get_flattened_data()
    return image.getdata()


def region_metrics(source: Image.Image, actual: Image.Image, box: tuple[int, int, int, int], tolerance: int) -> dict:
    source_crop = source.crop(box).convert("RGB")
    actual_crop = actual.crop(box).convert("RGB")
    diff = ImageChops.difference(source_crop, actual_crop)
    stat = ImageStat.Stat(diff)
    total = max(1, diff.width * diff.height)
    mismatch = 0
    strict_mismatch = 0
    for pixel in pixel_data(diff):
      if any(channel > 0 for channel in pixel):
          strict_mismatch += 1
      if any(channel > tolerance for channel in pixel):
          mismatch += 1
    return {
        "box": {"x": box[0], "y": box[1], "w": box[2] - box[0], "h": box[3] - box[1]},
        "pixels": total,
        "strict_mismatch_ratio": strict_mismatch / total,
        "mismatch_ratio": mismatch / total,
        "mean_delta_rgb": [round(value, 3) for value in stat.mean],
        "rms_delta_rgb": [round(value, 3) for value in stat.rms],
        "mean_delta": round(sum(stat.mean) / 3, 3),
        "rms_delta": round(sum(stat.rms) / 3, 3),
        "weighted_rms_delta": round((sum(stat.rms) / 3) * total, 3),
    }


def main() -> int:
    args = parse_args()
    source = Image.open(args.source).convert("RGB")
    actual = Image.open(args.actual).convert("RGB")
    regions = []
    for name, box in LOGIN_REGIONS:
        metrics = region_metrics(source, actual, box, args.tolerance)
        regions.append({"name": name, **metrics})
    ranked = sorted(regions, key=lambda item: (item["mismatch_ratio"], item["rms_delta"]), reverse=True)
    payload = {
        "generated_at": datetime.now(timezone.utc).astimezone().isoformat(),
        "source": str(args.source),
        "actual": str(args.actual),
        "tolerance": args.tolerance,
        "source_size": {"width": source.width, "height": source.height},
        "actual_size": {"width": actual.width, "height": actual.height},
        "regions": regions,
        "ranked_regions": ranked,
    }
    args.output.parent.mkdir(parents=True, exist_ok=True)
    args.output.write_text(json.dumps(payload, ensure_ascii=False, indent=2) + "\n", encoding="utf-8")
    print(json.dumps({
        "ok": True,
        "output": str(args.output),
        "top_regions": [
            {
                "name": item["name"],
                "mismatch_ratio": item["mismatch_ratio"],
                "rms_delta": item["rms_delta"],
            }
            for item in ranked[:5]
        ],
    }, ensure_ascii=False))
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
