#!/usr/bin/env python3
"""Generate layer-aware UI visual diff metrics from a page layer contract."""

from __future__ import annotations

import argparse
import json
from datetime import datetime, timezone
from pathlib import Path
from typing import Iterable

from PIL import Image


def parse_args() -> argparse.Namespace:
    parser = argparse.ArgumentParser(description="Compare source and actual screenshots by contracted UI layers.")
    parser.add_argument("--target-id", required=True, help="Visual target id, for example screen or dashboard.")
    parser.add_argument("--route", required=True, help="Frontend route represented by the screenshot.")
    parser.add_argument("--layer-spec", required=True, type=Path, help="Layer JSON from doc/04_assets/ui_suite_gpt_v1/specs/layers.")
    parser.add_argument("--source", required=True, type=Path, help="Source UI target PNG.")
    parser.add_argument("--actual", required=True, type=Path, help="Actual frontend screenshot PNG.")
    parser.add_argument("--metrics", required=True, type=Path, help="Output metrics JSON path.")
    parser.add_argument("--diff-dir", type=Path, help="Optional directory for per-layer diff PNGs.")
    parser.add_argument("--channel-tolerance", type=int, default=0, help="Per-channel tolerance before a pixel is counted as mismatched.")
    parser.add_argument("--max-pixel-ratio", type=float, default=0.015, help="Maximum allowed mismatch ratio per layer.")
    return parser.parse_args()


def read_json(path: Path) -> dict:
    return json.loads(path.read_text(encoding="utf-8"))


def any_channel_over(delta: Iterable[int], tolerance: int) -> bool:
    return any(channel > tolerance for channel in delta)


def pixel_data(image: Image.Image):
    if hasattr(image, "get_flattened_data"):
        return image.get_flattened_data()
    return image.getdata()


def clamp_box(bbox: dict, width: int, height: int) -> tuple[int, int, int, int]:
    x = max(0, int(bbox["x"]))
    y = max(0, int(bbox["y"]))
    w = max(0, int(bbox["w"]))
    h = max(0, int(bbox["h"]))
    return (x, y, min(width, x + w), min(height, y + h))


def compare_images(source: Image.Image, actual: Image.Image, tolerance: int) -> tuple[int, int, float, int, float, float, Image.Image]:
    width = min(source.width, actual.width)
    height = min(source.height, actual.height)
    source_crop = source.crop((0, 0, width, height))
    actual_crop = actual.crop((0, 0, width, height))

    mismatch_pixels = 0
    max_channel_delta = 0
    total_channel_delta = 0
    total_pixel_max_delta = 0
    heatmap_pixels = []
    for source_pixel, actual_pixel in zip(pixel_data(source_crop), pixel_data(actual_crop)):
        delta = tuple(abs(int(a) - int(b)) for a, b in zip(source_pixel, actual_pixel))
        pixel_max_delta = max(delta)
        max_channel_delta = max(max_channel_delta, pixel_max_delta)
        total_channel_delta += sum(delta)
        total_pixel_max_delta += pixel_max_delta
        if any_channel_over(delta, tolerance):
            mismatch_pixels += 1
            heatmap_pixels.append((255, 48, 48, 170))
        else:
            heatmap_pixels.append((0, 0, 0, 0))

    compared_pixels = width * height
    ratio = mismatch_pixels / compared_pixels if compared_pixels else 1.0
    mean_channel_delta = total_channel_delta / (compared_pixels * 4) if compared_pixels else 255.0
    mean_pixel_max_delta = total_pixel_max_delta / compared_pixels if compared_pixels else 255.0
    heatmap = Image.new("RGBA", (width, height))
    heatmap.putdata(heatmap_pixels)
    diff = Image.alpha_composite(actual_crop.copy(), heatmap)
    return compared_pixels, mismatch_pixels, ratio, max_channel_delta, mean_channel_delta, mean_pixel_max_delta, diff


def main() -> int:
    args = parse_args()
    layer_spec = read_json(args.layer_spec)
    source = Image.open(args.source).convert("RGBA")
    actual = Image.open(args.actual).convert("RGBA")
    layers = layer_spec.get("layers") or []
    results = []

    for layer in layers:
        bbox = layer.get("bbox") or {}
        box = clamp_box(bbox, min(source.width, actual.width), min(source.height, actual.height))
        source_layer = source.crop(box)
        actual_layer = actual.crop(box)
        compared_pixels, mismatch_pixels, ratio, max_channel_delta, mean_channel_delta, mean_pixel_max_delta, diff = compare_images(
            source_layer,
            actual_layer,
            args.channel_tolerance,
        )
        diff_path = None
        if args.diff_dir:
            args.diff_dir.mkdir(parents=True, exist_ok=True)
            diff_path = args.diff_dir / f"{layer['id']}-diff.png"
            diff.save(diff_path)
        results.append(
            {
                "id": layer.get("id"),
                "role": layer.get("role"),
                "bbox": bbox,
                "status": "pass" if ratio <= args.max_pixel_ratio else "fail",
                "compared_pixels": compared_pixels,
                "mismatch_pixels": mismatch_pixels,
                "pixel_mismatch_ratio": ratio,
                "max_pixel_ratio": args.max_pixel_ratio,
                "channel_tolerance": args.channel_tolerance,
                "max_channel_delta": max_channel_delta,
                "mean_channel_delta": mean_channel_delta,
                "mean_pixel_max_delta": mean_pixel_max_delta,
                "diff_image": str(diff_path) if diff_path else None,
            }
        )

    failed_layers = [item for item in results if item["status"] != "pass"]
    by_role = {}
    for item in results:
        role = item["role"] or "unknown"
        bucket = by_role.setdefault(
            role,
            {
                "layer_count": 0,
                "failed_count": 0,
                "weighted_mismatch_pixels": 0,
                "weighted_compared_pixels": 0,
                "weighted_channel_delta": 0.0,
                "weighted_pixel_max_delta": 0.0,
            },
        )
        bucket["layer_count"] += 1
        if item["status"] != "pass":
            bucket["failed_count"] += 1
        bucket["weighted_mismatch_pixels"] += item["mismatch_pixels"]
        bucket["weighted_compared_pixels"] += item["compared_pixels"]
        bucket["weighted_channel_delta"] += item["mean_channel_delta"] * item["compared_pixels"]
        bucket["weighted_pixel_max_delta"] += item["mean_pixel_max_delta"] * item["compared_pixels"]
    for bucket in by_role.values():
        compared = bucket["weighted_compared_pixels"]
        bucket["pixel_mismatch_ratio"] = bucket["weighted_mismatch_pixels"] / compared if compared else 1.0
        bucket["mean_channel_delta"] = bucket["weighted_channel_delta"] / compared if compared else 255.0
        bucket["mean_pixel_max_delta"] = bucket["weighted_pixel_max_delta"] / compared if compared else 255.0

    metrics = {
        "target_id": args.target_id,
        "route": args.route,
        "status": "pass" if not failed_layers else "fail",
        "generated_at": datetime.now(timezone.utc).astimezone().isoformat(),
        "layer_spec": str(args.layer_spec),
        "source_image": str(args.source),
        "actual_screenshot": str(args.actual),
        "viewport": {
            "source_width": source.width,
            "source_height": source.height,
            "actual_width": actual.width,
            "actual_height": actual.height,
            "size_ok": source.size == actual.size,
        },
        "summary": {
            "layer_count": len(results),
            "failed_layer_count": len(failed_layers),
            "roles": by_role,
        },
        "layers": results,
    }
    args.metrics.parent.mkdir(parents=True, exist_ok=True)
    args.metrics.write_text(json.dumps(metrics, ensure_ascii=False, indent=2) + "\n", encoding="utf-8")
    print(
        json.dumps(
            {
                "status": metrics["status"],
                "target_id": args.target_id,
                "failed_layer_count": len(failed_layers),
                "layer_count": len(results),
                "worst_layer": max(results, key=lambda item: item["pixel_mismatch_ratio"])["id"] if results else None,
            },
            ensure_ascii=False,
        )
    )
    return 0 if not failed_layers else 1


if __name__ == "__main__":
    raise SystemExit(main())
