#!/usr/bin/env python3
"""Generate UI visual diff metrics for the 1:1 frontend gate."""

from __future__ import annotations

import argparse
import json
from datetime import datetime, timezone
from pathlib import Path
from typing import Iterable

from PIL import Image

try:
    import numpy as np
except Exception:  # pragma: no cover - fallback keeps this checker portable.
    np = None


def parse_scoring_region(value: str) -> tuple[int, int, int, int]:
    """Parse a deterministic screenshot ROI in x,y,width,height form."""
    try:
        parts = tuple(int(part.strip()) for part in value.split(","))
    except ValueError as error:
        raise argparse.ArgumentTypeError("scoring region must use x,y,width,height integers") from error
    if len(parts) != 4:
        raise argparse.ArgumentTypeError("scoring region must use x,y,width,height")
    x, y, width, height = parts
    if x < 0 or y < 0 or width <= 0 or height <= 0:
        raise argparse.ArgumentTypeError("scoring region must have non-negative origin and positive size")
    return parts


def parse_args() -> argparse.Namespace:
    parser = argparse.ArgumentParser(description="Compare a source UI image and an actual frontend screenshot.")
    parser.add_argument("--target-id", required=True, help="Visual target id, for example alerts or topics-encrypted-tunnel.")
    parser.add_argument("--route", required=True, help="Frontend route represented by the screenshot.")
    parser.add_argument("--source", required=True, type=Path, help="Source UI target PNG.")
    parser.add_argument("--actual", required=True, type=Path, help="Actual frontend screenshot PNG.")
    parser.add_argument("--diff", required=True, type=Path, help="Output diff PNG path.")
    parser.add_argument("--metrics", required=True, type=Path, help="Output metrics JSON path.")
    parser.add_argument("--max-pixel-ratio", type=float, default=0.015, help="Maximum allowed mismatch ratio.")
    parser.add_argument("--channel-tolerance", type=int, default=0, help="Per-channel tolerance before a pixel is counted as mismatched.")
    parser.add_argument(
        "--scoring-region",
        type=parse_scoring_region,
        metavar="x,y,width,height",
        help="Optional ROI used for pass/fail pixel scoring. Pixels outside it remain diagnostic-only.",
    )
    parser.add_argument("--scoring-region-id", default="", help="Stable specification id for --scoring-region.")
    parser.add_argument("--desktop-status", default="pass", help="Desktop Chrome backend status recorded in metrics.")
    return parser.parse_args()


def any_channel_over(delta: Iterable[int], tolerance: int) -> bool:
    return any(channel > tolerance for channel in delta)


def pixel_data(image: Image.Image):
    if hasattr(image, "get_flattened_data"):
        return image.get_flattened_data()
    return image.getdata()


def build_diff_overlay(source_crop: Image.Image, actual_crop: Image.Image, tolerance: int) -> tuple[int, int, Image.Image]:
    if np is not None:
        source_array = np.asarray(source_crop, dtype=np.int16)
        actual_array = np.asarray(actual_crop, dtype=np.int16)
        delta = np.abs(source_array - actual_array)
        mismatch_mask = np.any(delta > tolerance, axis=2)
        mismatch_pixels = int(np.count_nonzero(mismatch_mask))
        max_channel_delta = int(delta.max()) if delta.size else 0
        heatmap_array = np.zeros((*mismatch_mask.shape, 4), dtype=np.uint8)
        heatmap_array[mismatch_mask] = (255, 48, 48, 170)
        return mismatch_pixels, max_channel_delta, Image.fromarray(heatmap_array, "RGBA")

    mismatch_pixels = 0
    max_channel_delta = 0
    heatmap_pixels = []
    for source_pixel, actual_pixel in zip(pixel_data(source_crop), pixel_data(actual_crop)):
        delta = tuple(abs(int(a) - int(b)) for a, b in zip(source_pixel, actual_pixel))
        max_channel_delta = max(max_channel_delta, max(delta))
        if any_channel_over(delta, tolerance):
            mismatch_pixels += 1
            heatmap_pixels.append((255, 48, 48, 170))
        else:
            heatmap_pixels.append((0, 0, 0, 0))

    heatmap = Image.new("RGBA", source_crop.size)
    heatmap.putdata(heatmap_pixels)
    return mismatch_pixels, max_channel_delta, heatmap


def region_for_comparison(
    scoring_region: tuple[int, int, int, int] | None,
    width: int,
    height: int,
) -> tuple[int, int, int, int]:
    if scoring_region is None:
        return 0, 0, width, height
    x, y, region_width, region_height = scoring_region
    if x + region_width > width or y + region_height > height:
        raise ValueError(
            f"scoring region {x},{y},{region_width},{region_height} exceeds comparable image bounds {width}x{height}"
        )
    return scoring_region


def main() -> int:
    args = parse_args()
    source = Image.open(args.source).convert("RGBA")
    actual = Image.open(args.actual).convert("RGBA")
    size_ok = source.size == actual.size

    width = min(source.width, actual.width)
    height = min(source.height, actual.height)
    source_crop = source.crop((0, 0, width, height))
    actual_crop = actual.crop((0, 0, width, height))

    full_mismatch_pixels, full_max_channel_delta, _ = build_diff_overlay(source_crop, actual_crop, args.channel_tolerance)
    score_x, score_y, score_width, score_height = region_for_comparison(args.scoring_region, width, height)
    source_score = source_crop.crop((score_x, score_y, score_x + score_width, score_y + score_height))
    actual_score = actual_crop.crop((score_x, score_y, score_x + score_width, score_y + score_height))
    mismatch_pixels, max_channel_delta, score_heatmap = build_diff_overlay(source_score, actual_score, args.channel_tolerance)

    compared_pixels = score_width * score_height
    if not size_ok:
        missing_pixels = source.width * source.height + actual.width * actual.height - 2 * (width * height)
        full_mismatch_pixels += max(0, missing_pixels)
    total_pixels = compared_pixels
    mismatch_ratio = mismatch_pixels / total_pixels if total_pixels else 1.0
    full_image_pixels = max(source.width * source.height, actual.width * actual.height)
    full_image_mismatch_ratio = full_mismatch_pixels / full_image_pixels if full_image_pixels else 1.0

    base = actual_crop.copy()
    scoped_heatmap = Image.new("RGBA", actual_crop.size)
    scoped_heatmap.paste(score_heatmap, (score_x, score_y))
    diff = Image.alpha_composite(base, scoped_heatmap)
    args.diff.parent.mkdir(parents=True, exist_ok=True)
    diff.save(args.diff)

    passed = size_ok and mismatch_ratio <= args.max_pixel_ratio
    metrics = {
        "target_id": args.target_id,
        "route": args.route,
        "status": "pass" if passed else "fail",
        "generated_at": datetime.now(timezone.utc).astimezone().isoformat(),
        "desktop_chrome_backend_status": args.desktop_status,
        "source_image": str(args.source),
        "actual_screenshot": str(args.actual),
        "diff_image": str(args.diff),
        "viewport": {
            "width": actual.width,
            "height": actual.height,
        },
        "visual_diff": {
            "size_ok": size_ok,
            "comparison_scope": "scoring-region" if args.scoring_region else "full-image",
            "scoring_region": {
                "id": args.scoring_region_id or ("custom-region" if args.scoring_region else "full-image"),
                "x": score_x,
                "y": score_y,
                "width": score_width,
                "height": score_height,
            },
            "source_width": source.width,
            "source_height": source.height,
            "actual_width": actual.width,
            "actual_height": actual.height,
            "compared_pixels": compared_pixels,
            "total_pixels": total_pixels,
            "mismatch_pixels": mismatch_pixels,
            "pixel_mismatch_ratio": mismatch_ratio,
            "max_pixel_ratio": args.max_pixel_ratio,
            "channel_tolerance": args.channel_tolerance,
            "max_channel_delta": max_channel_delta,
            "full_image_diagnostic": {
                "compared_pixels": width * height,
                "total_pixels": full_image_pixels,
                "mismatch_pixels": full_mismatch_pixels,
                "pixel_mismatch_ratio": full_image_mismatch_ratio,
                "max_channel_delta": full_max_channel_delta,
            },
        },
    }
    args.metrics.parent.mkdir(parents=True, exist_ok=True)
    args.metrics.write_text(json.dumps(metrics, ensure_ascii=False, indent=2) + "\n", encoding="utf-8")
    print(json.dumps({
        "status": metrics["status"],
        "target_id": args.target_id,
        "pixel_mismatch_ratio": mismatch_ratio,
        "comparison_scope": metrics["visual_diff"]["comparison_scope"],
        "scoring_region": metrics["visual_diff"]["scoring_region"],
    }, ensure_ascii=False))
    return 0 if passed else 1


if __name__ == "__main__":
    raise SystemExit(main())
