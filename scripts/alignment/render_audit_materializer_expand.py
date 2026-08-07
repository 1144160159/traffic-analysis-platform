#!/usr/bin/env python3
"""Render the audit materializer expand manifest with an immutable image."""

from __future__ import annotations

import argparse
import re
from pathlib import Path


ROOT = Path(__file__).resolve().parents[2]
TEMPLATE = (
    ROOT
    / "deployments/kubernetes/canary/audit-materializer-expand.template.yaml"
)
IMMUTABLE_IMAGE = re.compile(r"^[A-Za-z0-9./:_-]+@sha256:[0-9a-f]{64}$")


def render(image: str) -> str:
    image = image.strip()
    if not IMMUTABLE_IMAGE.fullmatch(image):
        raise ValueError("audit materializer image must be repository@sha256:<64 lowercase hex>")
    template = TEMPLATE.read_text(encoding="utf-8")
    marker = "${AUDIT_MATERIALIZER_IMAGE}"
    if template.count(marker) != 1:
        raise ValueError("audit materializer template must contain exactly one image marker")
    rendered = template.replace(marker, image)
    if "${" in rendered:
        raise ValueError("audit materializer manifest contains unresolved variables")
    return rendered


def main() -> int:
    parser = argparse.ArgumentParser()
    parser.add_argument("--image", required=True)
    parser.add_argument("--output", type=Path)
    args = parser.parse_args()
    try:
        rendered = render(args.image)
    except ValueError as exc:
        parser.error(str(exc))
    if args.output:
        args.output.write_text(rendered, encoding="utf-8")
    else:
        print(rendered, end="")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
