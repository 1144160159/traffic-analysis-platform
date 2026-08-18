#!/usr/bin/env python3
"""Verify persisted T1-M10-N008 Kubernetes atomic PKI evidence."""

from __future__ import annotations

import hashlib
import json
import sys
import uuid
from pathlib import Path
from typing import Any


SCRIPT_DIR = Path(__file__).resolve().parent
if str(SCRIPT_DIR) not in sys.path:
    sys.path.insert(0, str(SCRIPT_DIR))

import run_m10_pki_rotation_k8s as runner


def load(path: Path) -> dict[str, Any]:
    value = json.loads(path.read_text(encoding="utf-8"))
    if not isinstance(value, dict):
        raise ValueError("N008 evidence must be an object")
    return value


def validate(actual: dict[str, Any]) -> list[str]:
    errors: list[str] = []
    catalog = load(runner.ROOT / runner.PKI_CATALOG)
    n007 = load(runner.ROOT / runner.N007)
    fixed = {
        "schema_version": 1,
        "artifact_kind": "M10_PKI_ROTATION_K8S_NEGATIVE_TEST_RESULT",
        "task_id": "T1-M10-N008",
        "atomic_pr_ids": ["T1-M10-P018-OPS-n008-s1", "T1-M10-P019-TST-POST-n008-s2"],
        "status": "PASS",
        "engineering_status": "PASS",
        "acceptance_status": "BLOCKED_NOT_ENABLED",
        "profile_id": "M10-N008-K8S-ATOMIC-PKI-ROTATION-V1",
        "environment_kind": "KUBERNETES",
        "build_toolchain": "go version go1.25.12 linux/amd64",
        "required_gates": {
            "G0": "BLOCKED_BY_DEPLOYABLE_CANDIDATE",
            "G2": "PASS_UNIT_AND_K8S_SELF_TEST",
            "G3": "PASS_CROSS_NODE_SELF_TEST",
            "G6": "BLOCKED_NOT_ENABLED",
        },
    }
    for field, expected in fixed.items():
        if actual.get(field) != expected:
            errors.append(f"fixed evidence field drifted: {field}")
    try:
        parsed_run_id = uuid.UUID(str(actual.get("run_id")))
        if str(parsed_run_id) != actual.get("run_id"):
            raise ValueError("non-canonical UUID")
    except (ValueError, TypeError, AttributeError):
        errors.append("run_id must be a canonical UUID")
    if actual.get("candidate_id") != n007.get("candidate_id"):
        errors.append("candidate binding drifted from N007")
    if catalog.get("status") != "candidate_default_off" or catalog.get("production_applied") is not False:
        errors.append("PKI catalog overclaims production rollout")
    if actual.get("pki_catalog_sha256") != catalog.get("catalog_sha256"):
        errors.append("PKI catalog hash drifted")
    expected_hashes = {
        str(path): hashlib.sha256((runner.ROOT / path).read_bytes()).hexdigest()
        for path in runner.SOURCE_FILES
    }
    if actual.get("source_sha256") != expected_hashes:
        errors.append("source hashes drifted")
    if actual.get("catalog_counts") != catalog.get("counts"):
        errors.append("PKI catalog counts drifted")
    if actual.get("fail_closed_dimensions") != runner.EXPECTED_DIMENSIONS:
        errors.append("fail-closed dimension set drifted")
    jobs = actual.get("kubernetes_jobs")
    if not isinstance(jobs, list) or len(jobs) != 2:
        errors.append("exactly two Kubernetes job receipts are required")
        jobs = []
    nodes: list[Any] = []
    image_ids: list[Any] = []
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
    required = (
        "APPROVED_ISSUER_AND_OFFLINE_ROOT_CUSTODY_REQUIRED",
        "LIVE_REVOCATION_AND_ROLLBACK_REQUIRED",
        "PER_PROBE_UNIQUE_CERTIFICATE_REQUIRED",
        "TLS_ROTATION_RUNTIME_ENABLEMENT_REQUIRED",
        "TWO_CONSECUTIVE_LIVE_ROTATIONS_REQUIRED",
    )
    for code in required:
        if code not in blockers:
            errors.append(f"required blocker removed: {code}")
    if n007.get("acceptance_status") != "PASS" and "N007_AUTHZ_ENABLEMENT_REQUIRED" not in blockers:
        errors.append("N007 dependency blocker removed")
    if n007.get("candidate_id") is None and "DEPLOYABLE_CANDIDATE_REQUIRED" not in blockers:
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
        print("FAIL: N008 Kubernetes evidence is absent")
        return 1
    errors = validate(load(runner.OUTPUT))
    if errors:
        for error in errors:
            print(f"FAIL: {error}")
        return 1
    print("PASS: T1-M10-N008 Kubernetes atomic PKI evidence is current and fail-closed")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
