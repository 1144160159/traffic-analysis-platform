#!/usr/bin/env python3
"""Render the F-PROBE G2 canary template with validated candidate references."""

from __future__ import annotations

import argparse
import re
from pathlib import Path


ROOT = Path(__file__).resolve().parents[2]
TEMPLATE = ROOT / "deployments/kubernetes/canary/probe-control-g2-canary.template.yaml"
IMAGE_RE = re.compile(r"^[a-zA-Z0-9._:/@-]+$")
NODE_RE = re.compile(r"^[a-z0-9]([-a-z0-9.]*[a-z0-9])?$")


def main() -> int:
    parser = argparse.ArgumentParser()
    parser.add_argument("--source-sha256", required=True)
    parser.add_argument("--node", required=True)
    parser.add_argument("--ingest-image", required=True)
    parser.add_argument("--probe-image", required=True)
    parser.add_argument("--smoke-image", required=True)
    parser.add_argument("--output", type=Path, required=True)
    args = parser.parse_args()

    if not re.fullmatch(r"[0-9a-f]{64}", args.source_sha256):
        raise SystemExit("source SHA-256 must contain exactly 64 lowercase hex characters")
    if not NODE_RE.fullmatch(args.node):
        raise SystemExit("invalid Kubernetes node name")
    for image in (args.ingest_image, args.probe_image, args.smoke_image):
        if not IMAGE_RE.fullmatch(image) or ":latest" in image:
            raise SystemExit(f"invalid or mutable image reference: {image}")
    output = args.output.resolve()
    if output.exists():
        raise SystemExit(f"refusing to overwrite rendered canary manifest: {output}")

    rendered = TEMPLATE.read_text(encoding="utf-8")
    replacements = {
        "__SOURCE_SHA256__": args.source_sha256,
        "__CANARY_NODE__": args.node,
        "__INGEST_IMAGE__": args.ingest_image,
        "__PROBE_IMAGE__": args.probe_image,
        "__SMOKE_IMAGE__": args.smoke_image,
    }
    for placeholder, value in replacements.items():
        rendered = rendered.replace(placeholder, value)
    if "__" in rendered:
        raise SystemExit("unresolved canary template placeholder")
    output.parent.mkdir(parents=True, exist_ok=True)
    output.write_text(rendered, encoding="utf-8")
    print(output)
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
