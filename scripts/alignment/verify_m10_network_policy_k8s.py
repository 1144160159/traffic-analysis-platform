#!/usr/bin/env python3
"""Verify persisted T1-M10-N009 Kubernetes enforcement evidence."""

from __future__ import annotations

import datetime as dt
import hashlib
import json
import sys
import uuid
from pathlib import Path
from typing import Any


ROOT_HINT = Path(__file__).resolve().parents[2]
if str(ROOT_HINT) not in sys.path:
    sys.path.insert(0, str(ROOT_HINT))

from scripts.alignment import build_m10_minimum_network_policy as builder
from scripts.alignment import run_m10_network_policy_k8s as runner
from scripts.alignment import verify_m10_minimum_network_policy as policy_verifier


def load(path: Path) -> dict[str, Any]:
    value = json.loads(path.read_text(encoding="utf-8"))
    if not isinstance(value, dict):
        raise ValueError("N009 evidence must be an object")
    return value


def validate(actual: dict[str, Any]) -> list[str]:
    errors: list[str] = []
    fixed = {
        "schema_version": 1,
        "artifact_kind": "M10_NETWORK_POLICY_K8S_ENFORCEMENT_RESULT",
        "task_id": "T1-M10-N009",
        "atomic_pr_ids": ["T1-M10-P021-OPS-n009-s1", "T1-M10-P022-TST-POST-n009-s2"],
        "status": "BLOCKED",
        "engineering_status": "PASS",
        "acceptance_status": "BLOCKED_CNI_ENFORCEMENT_MISSING",
        "profile_id": "M10-N009-K8S-MINIMUM-NETWORK-POLICY-V1",
        "environment_kind": "KUBERNETES",
        "required_gates": {
            "G0": "BLOCKED_BY_DEPLOYABLE_CANDIDATE",
            "G2": "PASS_STATIC_AND_MUTATION_TESTS",
            "G3": "BLOCKED_CNI_ENFORCEMENT_MISSING",
            "G6": "BLOCKED_NOT_APPLIED",
        },
    }
    for field, expected in fixed.items():
        if actual.get(field) != expected:
            errors.append(f"fixed evidence field drifted: {field}")
    try:
        parsed = uuid.UUID(str(actual.get("run_id")))
        if str(parsed) != actual.get("run_id"):
            raise ValueError("non-canonical")
    except (ValueError, TypeError, AttributeError):
        errors.append("run_id must be a canonical UUID")
    try:
        captured = dt.datetime.fromisoformat(str(actual.get("captured_at")).replace("Z", "+00:00"))
        if captured.tzinfo is None:
            raise ValueError("timezone missing")
    except (ValueError, TypeError, AttributeError):
        errors.append("captured_at must be timezone-aware RFC3339")

    contract = builder.load()
    local_errors = policy_verifier.validate(contract, policy_verifier.load_yaml())
    errors.extend(f"local candidate: {error}" for error in local_errors)
    if actual.get("contract_sha256") != runner.sha256(builder.CONTRACT):
        errors.append("contract digest drifted")
    if actual.get("generated_manifest_sha256") != runner.sha256(builder.OUTPUT):
        errors.append("generated manifest digest drifted")
    expected_hashes = {str(path): runner.sha256(runner.ROOT / path) for path in runner.SOURCE_FILES}
    if actual.get("source_sha256") != expected_hashes:
        errors.append("source hashes drifted")

    cluster = actual.get("cluster")
    if not isinstance(cluster, dict):
        return [*errors, "cluster snapshot is missing"]
    if cluster.get("network_policy_api_available") is not True:
        errors.append("NetworkPolicy API availability was not recorded")
    cnis = cluster.get("policy_enforcement_cnis")
    if cnis != []:
        errors.append("blocked evidence cannot coexist with an enforcement-capable CNI")
    daemonsets = cluster.get("cni_daemonsets")
    if not isinstance(daemonsets, list) or not any("flannel" in item.get("markers", []) for item in daemonsets if isinstance(item, dict)):
        errors.append("Flannel-only blocker has no bound DaemonSet identity")
    nodes = cluster.get("nodes")
    if not isinstance(nodes, list) or sorted(item.get("name") for item in nodes if isinstance(item, dict)) != ["8-2tb", "zeus-server"]:
        errors.append("two-node Kubernetes identity coverage drifted")
    policies = cluster.get("live_network_policies")
    if not isinstance(policies, list) or cluster.get("live_network_policy_count") != len(policies):
        errors.append("live NetworkPolicy count does not match the bound object list")

    candidate = actual.get("candidate")
    expected_names = [item["metadata"]["name"] for item in builder.build(contract)]
    if not isinstance(candidate, dict):
        return [*errors, "candidate receipt is missing"]
    if candidate.get("object_count") != 10 or candidate.get("policy_names") != expected_names:
        errors.append("candidate object closure drifted")
    if candidate.get("client_dry_run") != "PASS":
        errors.append("client dry-run did not pass")
    if candidate.get("present_in_target_namespace") != [] or candidate.get("applied_by_run") is not False:
        errors.append("read-only evidence claims or observes a candidate apply")
    expected_exceptions = [{
        "exception_id": item["exception_id"],
        "approval_status": item["approval_status"],
        "risk_owner_role": item["risk_owner_role"],
    } for item in contract["exceptions"]]
    if candidate.get("exception_approvals") != expected_exceptions:
        errors.append("exception approval binding drifted")
    if not any(item["approval_status"] != "APPROVED" for item in expected_exceptions):
        errors.append("evidence no longer has the required unapproved exception blocker")

    probes = actual.get("negative_probes")
    expected_probes = {
        "unauthorized_pod": runner.PROBE_BLOCKED,
        "unauthorized_port": runner.PROBE_BLOCKED,
        "external_egress": runner.PROBE_BLOCKED,
    }
    if probes != expected_probes:
        errors.append("negative probes falsely claim execution or have incomplete dimensions")
    blockers = actual.get("blocking_codes")
    if not isinstance(blockers, list) or blockers != sorted(set(blockers)):
        errors.append("blocking codes must be sorted and unique")
        blockers = []
    for required in (
        "APPROVED_EQUIVALENT_CONTROL_REQUIRED",
        "AUTH_OIDC_NODEPORT_EXCEPTION_APPROVAL_REQUIRED",
        "CNI_POLICY_ENFORCEMENT_REQUIRED",
        "DEPLOYABLE_CANDIDATE_REQUIRED",
        "N008_ACCEPTANCE_REQUIRED",
        "REAL_UNAUTHORIZED_POD_PORT_EXTERNAL_NEGATIVE_TESTS_REQUIRED",
    ):
        if required not in blockers:
            errors.append(f"required blocker removed: {required}")
    if actual.get("shared_infrastructure_touched") is not False:
        errors.append("evidence overclaims shared infrastructure mutation")
    if actual.get("production_applied") is not False:
        errors.append("evidence overclaims production application")
    if actual.get("run_scoped_resources_created") is not False:
        errors.append("read-only run claims it created resources")
    does_not_prove = actual.get("does_not_prove")
    if not isinstance(does_not_prove, list) or not any("unauthorized pod" in item for item in does_not_prove):
        errors.append("evidence omits the negative-probe limitation")
    return errors


def main() -> int:
    schema = runner.ROOT / "contracts/alignment/m10-network-policy-k8s-result.schema.json"
    if not schema.is_file() or not runner.OUTPUT.is_file():
        print("FAIL: N009 evidence schema or evidence is absent")
        return 1
    errors = validate(load(runner.OUTPUT))
    if errors:
        for error in errors:
            print(f"FAIL: {error}")
        return 1
    print("PASS: T1-M10-N009 evidence is current, read-only, and correctly blocked by CNI enforcement")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
