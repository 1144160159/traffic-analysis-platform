#!/usr/bin/env python3
"""Verify the T1-M10-N006 live route diff and its fail-closed claims."""

from __future__ import annotations

import json
import sys
from pathlib import Path
from typing import Any


ROOT_HINT = Path(__file__).resolve().parents[2]
if str(ROOT_HINT) not in sys.path:
    sys.path.insert(0, str(ROOT_HINT))

from scripts.alignment import capture_m10_apisix_route_diff as capture


OUTPUT = capture.OUTPUT
SCHEMA = capture.ROOT / "contracts/alignment/m10-apisix-route-diff.schema.json"


def load(path: Path) -> dict[str, Any]:
    value = json.loads(path.read_text(encoding="utf-8"))
    if not isinstance(value, dict):
        raise ValueError("N006 evidence must be an object")
    return value


def validate(expected: dict[str, Any], actual: dict[str, Any]) -> list[str]:
    errors: list[str] = []
    if actual != expected:
        errors.append("evidence does not equal the current read-only Kubernetes capture")
    diff = actual.get("route_diff", {})
    zero_diff = not diff.get("missing_live_ids") and not diff.get("extra_live_ids") and not diff.get("changed_ids")
    if diff.get("zero_diff") is not bool(zero_diff):
        errors.append("route zero-diff flag is inconsistent")
    blockers = actual.get("blocking_codes")
    if not isinstance(blockers, list) or blockers != sorted(set(blockers)):
        errors.append("blocking codes must be a sorted unique list")
        blockers = []
    expected_acceptance = "PASS" if not blockers else "BLOCKED_ROUTE_DIFF"
    if actual.get("acceptance_status") != expected_acceptance:
        errors.append("acceptance status does not match blockers")
    if not diff.get("zero_diff") and "LIVE_ROUTE_SET_OR_CONTENT_DIFF" not in blockers:
        errors.append("live route diff blocker was removed")
    if actual.get("workload_diff", {}).get("matches") is not True:
        if "LIVE_GATEWAY_WORKLOAD_POLICY_DIFF" not in blockers:
            errors.append("live workload diff blocker was removed")
    secrets = actual.get("secret_key_observation", {})
    if secrets.get("values_captured") is not False:
        errors.append("secret value capture is forbidden")
    if actual.get("candidate", {}).get("policy_counts", {}).get("routes_with_blocking_gaps") != 0:
        errors.append("candidate route policy gaps are not zero")
    if actual.get("shared_infrastructure_touched") is not False:
        errors.append("N006 falsely claims shared infrastructure mutation")
    if actual.get("production_applied") is not False:
        errors.append("N006 falsely claims production application")
    return errors


def main() -> int:
    if not SCHEMA.is_file() or not OUTPUT.is_file():
        print("FAIL: N006 schema or evidence is absent")
        return 1
    errors = validate(capture.capture(), load(OUTPUT))
    if errors:
        for error in errors:
            print(f"FAIL: {error}")
        return 1
    print("PASS: T1-M10-N006 APISIX candidate/live diff is current and fail-closed")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
