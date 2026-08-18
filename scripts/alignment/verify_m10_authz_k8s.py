#!/usr/bin/env python3
"""Verify persisted T1-M10-N007 Kubernetes self-test evidence."""

from __future__ import annotations

import hashlib
import json
import sys
from pathlib import Path
from typing import Any

SCRIPT_DIR = Path(__file__).resolve().parent
if str(SCRIPT_DIR) not in sys.path:
    sys.path.insert(0, str(SCRIPT_DIR))

import run_m10_authz_k8s as runner


def load(path: Path) -> dict[str, Any]:
    value = json.loads(path.read_text(encoding="utf-8"))
    if not isinstance(value, dict):
        raise ValueError("N007 evidence must be an object")
    return value


def validate(actual: dict[str, Any]) -> list[str]:
    errors: list[str] = []
    policy = load(runner.ROOT / runner.POLICY)
    n006 = load(runner.ROOT / runner.N006)
    if actual.get("policy_sha256") != policy.get("policy_sha256"):
        errors.append("policy hash drifted")
    expected_hashes = {str(path): hashlib.sha256((runner.ROOT / path).read_bytes()).hexdigest() for path in runner.SOURCE_FILES}
    if actual.get("source_sha256") != expected_hashes:
        errors.append("source hashes drifted")
    if actual.get("policy_counts") != policy.get("counts"):
        errors.append("policy counts drifted")
    if actual.get("fail_closed_dimensions") != runner.EXPECTED_DIMENSIONS:
        errors.append("fail-closed dimension set drifted")
    jobs = actual.get("kubernetes_jobs")
    if not isinstance(jobs, list) or len(jobs) != 2:
        errors.append("exactly two Kubernetes job receipts are required")
        jobs = []
    nodes = []
    image_ids = []
    for item in jobs:
        if not isinstance(item, dict):
            errors.append("Kubernetes job receipt is invalid")
            continue
        try:
            runner.validate_result(item["result"])
        except Exception as exc:
            errors.append(str(exc))
        identity = item.get("identity", {})
        nodes.append(identity.get("node"))
        image_ids.append(identity.get("image_id"))
    if sorted(nodes) != ["8-2tb", "zeus-server"]:
        errors.append("Kubernetes node coverage drifted")
    if len(set(image_ids)) != 1 or not image_ids or not image_ids[0]:
        errors.append("both Kubernetes jobs must use one immutable image ID")
    blockers = actual.get("blocking_codes")
    if not isinstance(blockers, list) or blockers != sorted(set(blockers)):
        errors.append("blocking codes must be a sorted unique list")
        blockers = []
    for required in ("AUTHZ_POLICY_RUNTIME_ENABLEMENT_REQUIRED", "POST_ENABLE_LIVE_NEGATIVE_TESTS_REQUIRED"):
        if required not in blockers:
            errors.append(f"required blocker removed: {required}")
    if n006.get("acceptance_status") != "PASS" and "N006_ROUTE_MATERIALIZATION_REQUIRED" not in blockers:
        errors.append("N006 dependency blocker removed")
    if n006.get("candidate_id") is None and "DEPLOYABLE_CANDIDATE_REQUIRED" not in blockers:
        errors.append("deployable candidate blocker removed")
    if actual.get("acceptance_status") != "BLOCKED_NOT_ENABLED":
        errors.append("acceptance status overclaims runtime enablement")
    if actual.get("shared_infrastructure_touched") is not False or actual.get("production_applied") is not False:
        errors.append("evidence overclaims an infrastructure mutation")
    if actual.get("run_scoped_resources_removed") is not True:
        errors.append("run-scoped Kubernetes resources were not recorded as removed")
    return errors


def main() -> int:
    if not runner.OUTPUT.is_file():
        print("FAIL: N007 Kubernetes evidence is absent")
        return 1
    errors = validate(load(runner.OUTPUT))
    if errors:
        for error in errors:
            print(f"FAIL: {error}")
        return 1
    print("PASS: T1-M10-N007 Kubernetes self-test evidence is current and fail-closed")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
