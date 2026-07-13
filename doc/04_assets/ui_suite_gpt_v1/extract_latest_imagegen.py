#!/usr/bin/env python3
"""Extract the latest Codex Desktop imagegen result into a workspace PNG."""

from __future__ import annotations

import argparse
import base64
import json
import subprocess
from pathlib import Path

from PIL import Image


def latest_session() -> Path:
    output = subprocess.check_output(
        [
            "bash",
            "-lc",
            "find /root/.codex/sessions -type f -name '*.jsonl' -printf '%T@ %p\\n' | sort -nr | head -1 | cut -d' ' -f2-",
        ],
        text=True,
    ).strip()
    if not output:
        raise SystemExit("No Codex session JSONL found")
    return Path(output)


def iter_imagegen(session: Path):
    with session.open("r", encoding="utf-8") as fh:
        for line in fh:
            try:
                event = json.loads(line)
            except json.JSONDecodeError:
                continue
            payload = event.get("payload") or {}
            if event.get("type") == "response_item" and payload.get("type") == "image_generation_call":
                result = payload.get("result")
                if result:
                    yield event.get("timestamp"), payload.get("id"), result


def main() -> None:
    parser = argparse.ArgumentParser()
    parser.add_argument("target", help="Workspace target PNG path")
    parser.add_argument("--session", default="", help="Codex session JSONL path")
    parser.add_argument("--width", type=int, default=1920)
    parser.add_argument("--height", type=int, default=1080)
    args = parser.parse_args()

    target = Path(args.target)
    session = Path(args.session) if args.session else latest_session()
    images = list(iter_imagegen(session))
    if not images:
        raise SystemExit(f"No image_generation_call with result found in {session}")

    timestamp, image_id, b64 = images[-1]
    target.parent.mkdir(parents=True, exist_ok=True)
    raw = target.with_name(target.stem + ".raw-imagegen.png")
    image_bytes = base64.b64decode(b64)
    raw.write_bytes(image_bytes)

    with Image.open(raw) as img:
        raw_size = (img.width, img.height)
        img = img.convert("RGB").resize((args.width, args.height), Image.Resampling.LANCZOS)
        img.save(target, format="PNG", optimize=True)

    print(
        json.dumps(
            {
                "session": str(session),
                "timestamp": timestamp,
                "image_id": image_id,
                "raw": str(raw),
                "raw_size": raw_size,
                "target": str(target),
                "size": [args.width, args.height],
                "bytes": target.stat().st_size,
            },
            ensure_ascii=False,
        )
    )


if __name__ == "__main__":
    main()
