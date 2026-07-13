#!/usr/bin/env python3
"""Create per-image region overlay and measurement evidence from a breakdown JSON."""

from __future__ import annotations

import argparse
import json
import shutil
from datetime import datetime, timezone
from pathlib import Path
from typing import Any

from PIL import Image, ImageDraw, ImageFont


ROOT = Path(__file__).resolve().parents[3]


COLORS = [
    (30, 156, 255, 230),
    (54, 214, 107, 230),
    (255, 176, 32, 230),
    (255, 77, 79, 230),
    (157, 185, 201, 230),
    (255, 45, 45, 230),
    (64, 169, 255, 230),
    (114, 46, 209, 230),
]


def parse_args() -> argparse.Namespace:
    parser = argparse.ArgumentParser(description="Generate regions-overlay.png and measurement ledgers.")
    parser.add_argument("--record", required=True, type=Path, help="Breakdown JSON path, relative to repo root or absolute.")
    return parser.parse_args()


def repo_path(path: str | Path) -> Path:
    candidate = Path(path)
    return candidate if candidate.is_absolute() else ROOT / candidate


def repo_rel(path: Path) -> str:
    return path.resolve().relative_to(ROOT).as_posix()


def load_json(path: Path) -> dict[str, Any]:
    return json.loads(path.read_text(encoding="utf-8"))


def write_json(path: Path, value: Any) -> None:
    path.parent.mkdir(parents=True, exist_ok=True)
    path.write_text(json.dumps(value, ensure_ascii=False, indent=2) + "\n", encoding="utf-8")


def font() -> ImageFont.ImageFont:
    for candidate in (
        "/usr/share/fonts/dejavu-sans-fonts/DejaVuSans.ttf",
        "/usr/share/fonts/dejavu/DejaVuSans.ttf",
    ):
        if Path(candidate).exists():
            return ImageFont.truetype(candidate, 15)
    return ImageFont.load_default()


def text_size(draw: ImageDraw.ImageDraw, label: str, selected_font: ImageFont.ImageFont) -> tuple[int, int]:
    if hasattr(draw, "textbbox"):
        left, top, right, bottom = draw.textbbox((0, 0), label, font=selected_font)
        return right - left, bottom - top
    return draw.textsize(label, font=selected_font)


def draw_overlay(target: Path, overlay_path: Path, regions: list[dict[str, Any]]) -> None:
    image = Image.open(target).convert("RGBA")
    overlay = Image.new("RGBA", image.size, (0, 0, 0, 0))
    draw = ImageDraw.Draw(overlay)
    selected_font = font()

    for index, region in enumerate(regions):
        bbox = region.get("bbox") or {}
        try:
            x = int(round(float(bbox["x"])))
            y = int(round(float(bbox["y"])))
            w = int(round(float(bbox["w"])))
            h = int(round(float(bbox["h"])))
        except (KeyError, TypeError, ValueError):
            continue
        color = COLORS[index % len(COLORS)]
        fill = (color[0], color[1], color[2], 26)
        draw.rectangle([x, y, x + w, y + h], outline=color, width=3, fill=fill)
        label = f"{index + 1}:{region.get('id') or region.get('name') or 'region'}"
        tw, th = text_size(draw, label, selected_font)
        label_y = max(0, y - th - 8)
        draw.rectangle([x, label_y, x + tw + 10, label_y + th + 8], fill=(3, 17, 28, 220), outline=color, width=1)
        draw.text((x + 5, label_y + 4), label, fill=(234, 247, 255, 255), font=selected_font)

    composite = Image.alpha_composite(image, overlay)
    overlay_path.parent.mkdir(parents=True, exist_ok=True)
    composite.save(overlay_path)


def coverage_summary(canvas: dict[str, Any], regions: list[dict[str, Any]]) -> dict[str, Any]:
    width = int(canvas.get("width") or 0)
    height = int(canvas.get("height") or 0)
    total_area = max(1, width * height)
    region_area = 0
    invalid: list[str] = []
    for region in regions:
        bbox = region.get("bbox") or {}
        try:
            x = float(bbox["x"])
            y = float(bbox["y"])
            w = float(bbox["w"])
            h = float(bbox["h"])
            region_area += max(0, w) * max(0, h)
            if x < 0 or y < 0 or x + w > width or y + h > height:
                invalid.append(str(region.get("id") or region.get("name") or "unnamed"))
        except (KeyError, TypeError, ValueError):
            invalid.append(str(region.get("id") or region.get("name") or "unnamed"))
    return {
        "canvas_width": width,
        "canvas_height": height,
        "region_count": len(regions),
        "summed_region_area": int(region_area),
        "summed_region_area_ratio": region_area / total_area,
        "out_of_canvas_or_invalid_regions": invalid,
    }


def main() -> int:
    args = parse_args()
    record_path = repo_path(args.record)
    record = load_json(record_path)
    category = record["category"]
    image_id = record["id"]
    evidence_dir = ROOT / "evidence/ui-image-breakdowns" / category / image_id
    evidence_dir.mkdir(parents=True, exist_ok=True)

    source = repo_path(record["source_image"])
    target = evidence_dir / "target.png"
    if not target.exists():
        shutil.copyfile(source, target)

    overlay = evidence_dir / "regions-overlay.png"
    measurement = evidence_dir / "measurement.json"
    text_ocr = evidence_dir / "text-ocr.txt"

    regions = list(record.get("regions") or [])
    texts = list(record.get("texts") or [])
    draw_overlay(target, overlay, regions)

    measurement_payload = {
        "generated_by": "generate_image_breakdown_overlay.py",
        "generated_at": datetime.now(timezone.utc).astimezone().isoformat(),
        "id": image_id,
        "category": category,
        "source_image": record["source_image"],
        "target": repo_rel(target),
        "regions_overlay": repo_rel(overlay),
        "canvas": record.get("canvas") or {},
        "coverage": coverage_summary(record.get("canvas") or {}, regions),
        "regions": regions,
        "texts": texts,
    }
    write_json(measurement, measurement_payload)

    text_lines = [
        f"# {image_id} OCR/manual text ledger",
        "Source: JSON texts after manual correction; OCR is auxiliary, this file is not an acceptance shortcut.",
        "",
    ]
    for index, item in enumerate(texts, start=1):
        bbox = item.get("bbox") or {}
        text_lines.append(
            f"{index:03d}\t{item.get('type', '')}\t"
            f"{bbox.get('x', '')},{bbox.get('y', '')},{bbox.get('w', '')},{bbox.get('h', '')}\t"
            f"{item.get('value', '')}"
        )
    text_ocr.write_text("\n".join(text_lines).rstrip() + "\n", encoding="utf-8")

    record.setdefault("evidence", {})
    record["evidence"]["target"] = repo_rel(target)
    record["evidence"]["regions_overlay"] = repo_rel(overlay)
    record["evidence"]["measurement"] = repo_rel(measurement)
    record["evidence"]["text_ocr"] = repo_rel(text_ocr)
    write_json(record_path, record)

    print(
        json.dumps(
            {
                "id": image_id,
                "regions_overlay": repo_rel(overlay),
                "measurement": repo_rel(measurement),
                "text_ocr": repo_rel(text_ocr),
                "regions": len(regions),
                "texts": len(texts),
            },
            ensure_ascii=False,
            indent=2,
        )
    )
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
