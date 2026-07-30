#!/usr/bin/env python3
"""Create same-canvas alert-detail visual QA artifacts and numeric diff metrics."""

from __future__ import annotations

import argparse
import json
import math
from pathlib import Path

from PIL import Image, ImageChops, ImageEnhance


def parse_args() -> argparse.Namespace:
    parser = argparse.ArgumentParser()
    parser.add_argument("--reference", required=True, type=Path)
    parser.add_argument("--actual", required=True, type=Path)
    parser.add_argument("--output-dir", required=True, type=Path)
    return parser.parse_args()


def main() -> None:
    args = parse_args()
    reference = Image.open(args.reference).convert("RGB")
    actual = Image.open(args.actual).convert("RGB")
    if reference.size != actual.size:
        actual = actual.resize(reference.size, Image.Resampling.LANCZOS)

    args.output_dir.mkdir(parents=True, exist_ok=True)

    comparison = Image.new("RGB", (reference.width * 2, reference.height))
    comparison.paste(reference, (0, 0))
    comparison.paste(actual, (reference.width, 0))
    comparison.save(args.output_dir / "reference-vs-r814.png")

    difference = ImageChops.difference(reference, actual)
    ImageEnhance.Contrast(difference).enhance(2.0).save(
        args.output_dir / "absolute-diff-enhanced.png"
    )

    histogram = difference.histogram()
    channels = len(histogram) // 256
    squared_error = 0
    absolute_error = 0
    changed_channel_values = 0
    for channel in range(channels):
        for value in range(256):
            count = histogram[channel * 256 + value]
            absolute_error += value * count
            squared_error += value * value * count
            if value:
                changed_channel_values += count

    channel_values = reference.width * reference.height * channels
    mae = absolute_error / channel_values
    rmse = math.sqrt(squared_error / channel_values)
    pixels = reference.width * reference.height
    pixel_diff = difference.convert("L").point(lambda value: 255 if value else 0)
    changed_pixels = sum(pixel_diff.histogram()[1:])

    metrics = {
        "reference": str(args.reference),
        "actual": str(args.actual),
        "comparison": str(args.output_dir / "reference-vs-r814.png"),
        "difference": str(args.output_dir / "absolute-diff-enhanced.png"),
        "viewport": {"width": reference.width, "height": reference.height},
        "mae_0_255": round(mae, 6),
        "normalized_mae": round(mae / 255, 6),
        "rmse_0_255": round(rmse, 6),
        "normalized_rmse": round(rmse / 255, 6),
        "changed_channel_ratio": round(changed_channel_values / channel_values, 6),
        "changed_pixel_ratio": round(changed_pixels / pixels, 6),
        "interpretation": (
            "Whole-frame metrics are diagnostic only: live data and the explicit "
            "user overrides (title-only header, right-aligned back action, no "
            "pagination total) intentionally differ from the original reference."
        ),
    }
    (args.output_dir / "visual-metrics.json").write_text(
        json.dumps(metrics, ensure_ascii=False, indent=2) + "\n",
        encoding="utf-8",
    )
    print(json.dumps(metrics, ensure_ascii=False, indent=2))


if __name__ == "__main__":
    main()
