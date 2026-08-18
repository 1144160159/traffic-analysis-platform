#!/usr/bin/env python3
"""Validate the N002 provenance guard against a mounted Kubernetes workspace."""

from __future__ import annotations

import argparse
import json
import sys
from pathlib import Path


SCRIPT_DIR = Path(__file__).resolve().parent
if str(SCRIPT_DIR) not in sys.path:
    sys.path.insert(0, str(SCRIPT_DIR))

import candidate_snapshot


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--root", type=Path, required=True)
    parser.add_argument("--expected", type=Path, required=True)
    args = parser.parse_args()
    expected = json.loads(args.expected.read_text(encoding="utf-8"))
    actual = candidate_snapshot.scan_candidate_artifact_provenance(args.root)
    errors = []
    if actual.get("status") != expected.get("status"):
        errors.append("status mismatch")
    if actual.get("blocking_codes") != expected.get("blocking_codes"):
        errors.append("blocking code mismatch")
    if len(actual.get("active_first_party_images", [])) != expected.get("active_image_count"):
        errors.append("active first-party image count mismatch")
    if len(actual.get("excluded_prebuilt_artifacts", [])) != expected.get("excluded_prebuilt_count"):
        errors.append("excluded prebuilt count mismatch")
    result = {
        "status": "PASS" if not errors else "FAIL",
        "guard_status": actual.get("status"),
        "blocking_codes": actual.get("blocking_codes"),
        "active_image_count": len(actual.get("active_first_party_images", [])),
        "excluded_prebuilt_count": len(actual.get("excluded_prebuilt_artifacts", [])),
        "g0_status": "BLOCKED",
        "g1_status": "NOT_EXECUTED",
        "g6_status": "NOT_EXECUTED",
        "errors": errors,
    }
    print(json.dumps(result, sort_keys=True))
    return 0 if not errors else 1


if __name__ == "__main__":
    raise SystemExit(main())
