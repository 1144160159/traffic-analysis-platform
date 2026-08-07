#!/usr/bin/env python3
"""Render the opt-in T-OS-005 target without weakening kustomize globally."""

from __future__ import annotations

import argparse
import re
import subprocess
from pathlib import Path


ROOT = Path(__file__).resolve().parents[2]
OVERLAY = ROOT / "deployments/kubernetes/security/opensearch-ha-v1"
PLACEHOLDER_DIGEST = "sha256:" + ("0" * 64)


def render() -> str:
    completed = subprocess.run(
        ["kubectl", "kustomize", "--load-restrictor", "LoadRestrictionsNone", str(OVERLAY)],
        cwd=ROOT,
        text=True,
        stdout=subprocess.PIPE,
        stderr=subprocess.PIPE,
        check=False,
    )
    if completed.returncode:
        raise RuntimeError(f"kustomize render failed: {completed.stderr.strip()}")
    payload = completed.stdout
    required = (
        "kind: StatefulSet",
        "traffic.openai.com/contract: opensearch-ha-security-restore-v1",
        "topology.kubernetes.io/zone",
        "plugins.security.ssl.http.clientauth_mode",
        "opensearch-monitor-tls",
    )
    missing = [token for token in required if token not in payload]
    if missing:
        raise RuntimeError(f"rendered target is incomplete: {missing}")
    return payload


def main() -> int:
    parser = argparse.ArgumentParser()
    parser.add_argument("--output", type=Path)
    parser.add_argument(
        "--require-approved-image",
        action="store_true",
        help="fail if the deliberate registry.invalid/all-zero digest apply guard remains",
    )
    args = parser.parse_args()
    payload = render()
    images = re.findall(r"^\s*image:\s*(\S+)\s*$", payload, flags=re.MULTILINE)
    guarded = any("registry.invalid/" in image or PLACEHOLDER_DIGEST in image for image in images)
    if args.require_approved_image and guarded:
        raise SystemExit("refusing rollout render: approved immutable OpenSearch image digest is absent")
    if args.output:
        destination = args.output.resolve()
        destination.parent.mkdir(parents=True, exist_ok=True)
        destination.write_text(payload, encoding="utf-8")
    else:
        print(payload, end="")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
