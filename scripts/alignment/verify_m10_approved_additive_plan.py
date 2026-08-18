#!/usr/bin/env python3
"""Verify T1-M10-N005 proposal hashes, policies, and fail-closed authorization."""

from __future__ import annotations

import json
import sys
from pathlib import Path
from typing import Any


ROOT_HINT = Path(__file__).resolve().parents[2]
if str(ROOT_HINT) not in sys.path:
    sys.path.insert(0, str(ROOT_HINT))

from scripts.alignment import build_m10_approved_additive_plan as builder


ROOT = builder.ROOT
OUTPUT = ROOT / builder.OUTPUT_RELATIVE
SCHEMA = ROOT / "contracts/alignment/m10-approved-additive-plan.schema.json"


def load(path: Path) -> dict[str, Any]:
    return builder.load_json(path)


def validate(expected: dict[str, Any], actual: dict[str, Any]) -> list[str]:
    errors: list[str] = []
    if actual != expected:
        errors.append("plan does not equal deterministic builder output")
    if actual.get("environment_kind") != "KUBERNETES":
        errors.append("plan environment is not Kubernetes")
    artifacts = actual.get("artifacts")
    if not isinstance(artifacts, list):
        errors.append("artifacts must be an array")
        artifacts = []
    expected_identity = tuple((kind, path, milestone) for kind, path, milestone in builder.PROPOSED_ARTIFACTS)
    actual_identity = tuple(
        (item.get("kind"), item.get("path"), item.get("responsible_milestone"))
        for item in artifacts if isinstance(item, dict)
    )
    if actual_identity != expected_identity:
        errors.append("artifact identity/order drifted")
    paths = [item.get("path") for item in artifacts if isinstance(item, dict)]
    if len(paths) != len(set(paths)):
        errors.append("artifact path is duplicated")
    if [item.get("apply_order") for item in artifacts if isinstance(item, dict)] != list(range(1, 7)):
        errors.append("apply order is not exact and contiguous")
    for item in artifacts:
        if not isinstance(item, dict):
            errors.append("artifact entry must be an object")
            continue
        if item.get("exists") and not item.get("sha256"):
            errors.append(f"existing artifact lacks exact hash: {item.get('path')}")
        findings = item.get("destructive_findings")
        if findings and item.get("safety_class") != "NON_ADDITIVE_BLOCKED":
            errors.append(f"destructive artifact is not blocked: {item.get('path')}")
    blockers = actual.get("blocking_codes")
    if not isinstance(blockers, list) or blockers != sorted(set(blockers)):
        errors.append("blocking codes must be a sorted unique list")
        blockers = []
    if actual.get("replay_policy") != builder.REPLAY_POLICY:
        errors.append("exact-hash replay policy drifted")
    if actual.get("half_failure_policy") != builder.HALF_FAILURE_POLICY:
        errors.append("half-failure recovery policy drifted")
    if actual.get("compatibility_policy") != builder.COMPATIBILITY_POLICY:
        errors.append("legacy compatibility policy drifted")
    authorized = actual.get("status") == "AUTHORIZED"
    if authorized != (actual.get("apply_allowed") is True):
        errors.append("status and apply authorization disagree")
    if authorized and blockers:
        errors.append("authorized plan retains blockers")
    if not authorized and actual.get("status") != "BLOCKED_UNAPPROVED":
        errors.append("plan status is invalid")
    if not authorized and not blockers:
        errors.append("blocked plan has no blocking reason")
    candidate = actual.get("candidate", {})
    if candidate.get("status") != "FROZEN" or not candidate.get("candidate_id"):
        if "DEPLOYABLE_CANDIDATE_REQUIRED" not in blockers:
            errors.append("missing deployable-candidate blocker was removed")
    preflight = actual.get("preflight", {})
    if preflight.get("acceptance_status") != "PASS" or preflight.get("g6") != "PASS":
        if "N004_PREFLIGHT_G6_PASS_REQUIRED" not in blockers:
            errors.append("failed N004/G6 blocker was removed")
    if any(item.get("destructive_findings") for item in artifacts if isinstance(item, dict)):
        if "NON_ADDITIVE_ARTIFACT_PRESENT" not in blockers:
            errors.append("non-additive artifact blocker was removed")
    if actual.get("shared_infrastructure_touched") is not False:
        errors.append("plan falsely claims shared infrastructure mutation")
    if actual.get("production_applied") is not False:
        errors.append("plan falsely claims production application")
    return errors


def main() -> int:
    if not SCHEMA.is_file() or not OUTPUT.is_file():
        print("FAIL: N005 schema or plan artifact is absent")
        return 1
    errors = validate(builder.build(ROOT), load(OUTPUT))
    if errors:
        for error in errors:
            print(f"FAIL: {error}")
        return 1
    print("PASS: T1-M10-N005 additive plan is exact and fail-closed")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
