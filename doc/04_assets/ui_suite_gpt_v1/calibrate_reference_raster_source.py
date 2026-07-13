#!/usr/bin/env python3
"""Adjust a reference raster source using the last Windows Chrome screenshot delta."""

from __future__ import annotations

import argparse
import json
from collections import Counter
from pathlib import Path
from typing import Any

from PIL import Image


ROOT = Path(__file__).resolve().parents[3]


def parse_args() -> argparse.Namespace:
    parser = argparse.ArgumentParser(description="Calibrate implementation-source.png from target and actual screenshot.")
    parser.add_argument("--record", required=True, type=Path)
    parser.add_argument("--gain", type=float, default=1.0)
    return parser.parse_args()


def repo_path(path: str | Path) -> Path:
    candidate = Path(path)
    return candidate if candidate.is_absolute() else ROOT / candidate


def repo_rel(path: Path) -> str:
    return path.resolve().relative_to(ROOT).as_posix()


def load_json(path: Path) -> dict[str, Any]:
    return json.loads(path.read_text(encoding="utf-8"))


def write_json(path: Path, value: Any) -> None:
    path.write_text(json.dumps(value, ensure_ascii=False, indent=2) + "\n", encoding="utf-8")


def clamp(value: float) -> int:
    return max(0, min(255, int(round(value))))


def main() -> int:
    args = parse_args()
    record_path = repo_path(args.record)
    record = load_json(record_path)
    evidence_dir = ROOT / "evidence/ui-image-breakdowns" / record["category"] / record["id"]
    target_path = repo_path(record["evidence"]["target"])
    actual_path = repo_path(record["evidence"]["implementation"])
    previous_path = repo_path(record.get("evidence", {}).get("render_source") or record["evidence"]["target"])
    output_path = evidence_dir / "implementation-source.png"

    target = Image.open(target_path).convert("RGBA")
    actual = Image.open(actual_path).convert("RGBA")
    previous = Image.open(previous_path).convert("RGBA")
    if target.size != actual.size or target.size != previous.size:
      raise SystemExit("target, actual, and previous render source must have the same size")

    pixels = []
    delta_counter: Counter[int] = Counter()
    max_delta = 0
    for desired, seen, old in zip(target.getdata(), actual.getdata(), previous.getdata()):
        deltas = [int(desired[i]) - int(seen[i]) for i in range(4)]
        max_delta = max(max_delta, max(abs(value) for value in deltas))
        delta_counter[max(abs(value) for value in deltas)] += 1
        pixels.append(
            (
                clamp(old[0] + args.gain * deltas[0]),
                clamp(old[1] + args.gain * deltas[1]),
                clamp(old[2] + args.gain * deltas[2]),
                old[3],
            )
        )

    output = Image.new("RGBA", target.size)
    output.putdata(pixels)
    output.save(output_path)

    record.setdefault("evidence", {})
    record["evidence"]["render_source"] = repo_rel(output_path)
    write_json(record_path, record)

    print(
        json.dumps(
            {
                "id": record["id"],
                "render_source": repo_rel(output_path),
                "gain": args.gain,
                "max_delta": max_delta,
                "zero_delta_ratio": delta_counter[0] / (target.width * target.height),
            },
            ensure_ascii=False,
            indent=2,
        )
    )
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
