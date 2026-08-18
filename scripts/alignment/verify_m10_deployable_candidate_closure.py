#!/usr/bin/env python3
"""Verify T1-M10-N001 candidate closure semantics and deterministic identity."""

from __future__ import annotations

import json
import sys
from pathlib import Path
from typing import Any


ROOT_HINT = Path(__file__).resolve().parents[2]
if str(ROOT_HINT) not in sys.path:
    sys.path.insert(0, str(ROOT_HINT))

from scripts.alignment import build_m10_deployable_candidate_closure as builder


ROOT = builder.ROOT
OUTPUT = ROOT / builder.OUTPUT_RELATIVE
SCHEMA = ROOT / "contracts/alignment/m10-deployable-candidate-closure.schema.json"
EXPECTED_DIMENSIONS = tuple(builder.DIMENSION_PATHS)


def load(path: Path) -> dict[str, Any]:
    return builder.load_json(path)


def validate(expected: dict[str, Any], actual: dict[str, Any]) -> list[str]:
    errors: list[str] = []
    if actual != expected:
        errors.append("closure does not equal deterministic builder output")
    if actual.get("environment_kind") != "KUBERNETES":
        errors.append("deployment environment is not Kubernetes")
    dimensions = actual.get("dimensions")
    if not isinstance(dimensions, list):
        errors.append("dimensions must be an array")
        dimensions = []
    names = tuple(item.get("name") for item in dimensions if isinstance(item, dict))
    if names != EXPECTED_DIMENSIONS:
        errors.append("eight-dimension identity/order drifted")
    if len(set(names)) != len(names):
        errors.append("dimension identity is duplicated")
    for item in dimensions:
        if not isinstance(item, dict):
            errors.append("dimension entry must be an object")
            continue
        for ref in item.get("refs", []):
            if not isinstance(ref, dict):
                errors.append("dimension ref must be an object")
                continue
            if ref.get("exists") and not ref.get("sha256"):
                errors.append("existing dimension ref lacks exact hash")
            if not ref.get("exists") and ref.get("sha256") is not None:
                errors.append("missing dimension ref carries a hash")
    closure = actual.get("closure", {})
    bound = sum(
        isinstance(item, dict) and item.get("status") == "BOUND" for item in dimensions
    )
    if closure.get("required_dimension_count") != 8:
        errors.append("required dimension count drifted")
    if closure.get("bound_dimension_count") != bound:
        errors.append("bound dimension count is inconsistent")
    all_bound = len(dimensions) == 8 and bound == 8
    if closure.get("all_dimensions_bound") is not all_bound:
        errors.append("all-dimensions-bound flag is inconsistent")
    blockers = actual.get("blocking_codes")
    if not isinstance(blockers, list) or blockers != sorted(set(blockers)):
        errors.append("blocking codes must be a sorted unique list")
        blockers = []
    if actual.get("status") == "FROZEN":
        if not all_bound or blockers or not actual.get("candidate_id"):
            errors.append("frozen candidate lacks complete bound inputs")
        if closure.get("fail_closed") is not False:
            errors.append("frozen candidate is marked fail-closed")
    else:
        if actual.get("status") != "BLOCKED_INCOMPLETE":
            errors.append("candidate closure status is invalid")
        if actual.get("candidate_id") is not None:
            errors.append("incomplete closure falsely carries candidate identity")
        if not blockers or closure.get("fail_closed") is not True:
            errors.append("incomplete closure is not fail-closed")
    upstream = actual.get("upstream", {})
    if upstream.get("m01_release_pointer", {}).get("exists") is not True and "UPSTREAM_M01_RELEASE_POINTER_REQUIRED" not in blockers:
        errors.append("missing M01 pointer blocker was removed")
    m09 = upstream.get("m09_release_pointer", {})
    if (m09.get("status") != "GO" or not m09.get("candidate_id")) and "UPSTREAM_M09_SAME_CANDIDATE_REQUIRED" not in blockers:
        errors.append("non-GO M09 candidate blocker was removed")
    if actual.get("production_applied") is not False:
        errors.append("N001 falsely claims production application")
    return errors


def main() -> int:
    if not SCHEMA.is_file() or not OUTPUT.is_file():
        print("FAIL: N001 schema or closure artifact is absent")
        return 1
    errors = validate(builder.build(ROOT), load(OUTPUT))
    if errors:
        for error in errors:
            print(f"FAIL: {error}")
        return 1
    print("PASS: T1-M10-N001 candidate dimensions are exact and fail-closed")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
