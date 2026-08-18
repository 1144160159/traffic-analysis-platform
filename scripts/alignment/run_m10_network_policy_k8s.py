#!/usr/bin/env python3
"""Capture fail-closed T1-M10-N009 evidence from the Kubernetes environment."""

from __future__ import annotations

import argparse
import datetime as dt
import hashlib
import json
import os
import subprocess
import sys
import uuid
from pathlib import Path
from typing import Any


SCRIPT_DIR = Path(__file__).resolve().parent
ROOT = Path(__file__).resolve().parents[2]
if str(ROOT) not in sys.path:
    sys.path.insert(0, str(ROOT))

from scripts.alignment import build_m10_minimum_network_policy as builder
from scripts.alignment import verify_m10_minimum_network_policy as policy_verifier


OUTPUT = ROOT / "doc/02_acceptance/topic1/tasks/t1-m10-n009/k8s-network-policy-enforcement-latest.json"
N008 = Path("doc/02_acceptance/topic1/tasks/t1-m10-n008/k8s-pki-rotation-negative-tests-latest.json")
SOURCE_FILES = (
    Path("contracts/security/m10-minimum-network-policy.v1.json"),
    Path("contracts/security/m10-minimum-network-policy.schema.json"),
    Path("contracts/alignment/m10-network-policy-k8s-result.schema.json"),
    Path("deployments/kubernetes/security/m10-minimum-network-policies.v1.yaml"),
    Path("scripts/alignment/build_m10_minimum_network_policy.py"),
    Path("scripts/alignment/verify_m10_minimum_network_policy.py"),
    Path("scripts/alignment/run_m10_network_policy_k8s.py"),
    Path("scripts/alignment/verify_m10_network_policy_k8s.py"),
    Path("tests/alignment/test_m10_minimum_network_policy.py"),
    Path("tests/alignment/test_m10_network_policy_k8s.py"),
    Path("doc/07_alignment/runbooks/T1-M10-N009-minimum-network-policy.md"),
    N008,
)
CNI_MARKERS = ("antrea", "calico", "canal", "cilium", "flannel", "kube-router", "multus", "weave")
ENFORCEMENT_MARKERS = ("antrea", "calico", "canal", "cilium", "kube-router", "weave")
PROBE_BLOCKED = "NOT_EXECUTED_CNI_ENFORCEMENT_MISSING"


class EvidenceError(RuntimeError):
    pass


def sha256(path: Path) -> str:
    return hashlib.sha256(path.read_bytes()).hexdigest()


def kubectl(*args: str) -> str:
    environment = os.environ.copy()
    for name in ("HTTP_PROXY", "HTTPS_PROXY", "ALL_PROXY", "http_proxy", "https_proxy", "all_proxy"):
        environment.pop(name, None)
    completed = subprocess.run(
        ["kubectl", *args],
        cwd=ROOT,
        env=environment,
        text=True,
        stdout=subprocess.PIPE,
        stderr=subprocess.PIPE,
        check=False,
    )
    if completed.returncode != 0:
        raise EvidenceError(f"kubectl {' '.join(args)} failed: {completed.stderr.strip()}")
    return completed.stdout


def kubectl_json(*args: str) -> dict[str, Any]:
    value = json.loads(kubectl(*args, "-o", "json"))
    if not isinstance(value, dict):
        raise EvidenceError(f"kubectl {' '.join(args)} did not return an object")
    return value


def compact_identity(item: dict[str, Any]) -> dict[str, Any]:
    metadata = item.get("metadata", {})
    return {
        "name": metadata.get("name"),
        "namespace": metadata.get("namespace"),
        "uid": metadata.get("uid"),
        "resource_version": metadata.get("resourceVersion"),
    }


def daemonset_inventory(value: dict[str, Any]) -> list[dict[str, Any]]:
    inventory: list[dict[str, Any]] = []
    for item in value.get("items", []):
        containers = item.get("spec", {}).get("template", {}).get("spec", {}).get("containers", [])
        images = sorted(str(container.get("image")) for container in containers if container.get("image"))
        marker_text = " ".join([str(item.get("metadata", {}).get("name", "")), *images]).lower()
        markers = sorted(marker for marker in CNI_MARKERS if marker in marker_text)
        if not markers:
            continue
        identity = compact_identity(item)
        identity.update({
            "images": images,
            "markers": markers,
            "desired": item.get("status", {}).get("desiredNumberScheduled", 0),
            "ready": item.get("status", {}).get("numberReady", 0),
        })
        inventory.append(identity)
    return sorted(inventory, key=lambda item: (str(item["namespace"]), str(item["name"])))


def enforcement_cnis(inventory: list[dict[str, Any]]) -> list[str]:
    result = {
        marker
        for item in inventory
        for marker in item.get("markers", [])
        if marker in ENFORCEMENT_MARKERS and item.get("ready", 0) > 0
    }
    return sorted(result)


def capture() -> dict[str, Any]:
    contract = builder.load()
    objects = builder.build(contract)
    local_errors = policy_verifier.validate(contract, policy_verifier.load_yaml())
    if local_errors:
        raise EvidenceError("local candidate validation failed: " + "; ".join(local_errors))
    dry_run_names = [line for line in kubectl("apply", "--dry-run=client", "-f", str(builder.OUTPUT), "-o", "name").splitlines() if line]
    if len(dry_run_names) != len(objects):
        raise EvidenceError("kubectl client dry-run object count drifted")

    version = kubectl_json("version")
    context = kubectl("config", "current-context").strip()
    namespace = kubectl_json("get", "namespace", contract["target_namespace"])
    nodes = kubectl_json("get", "nodes")
    daemonsets = daemonset_inventory(kubectl_json("get", "daemonsets", "--all-namespaces"))
    active_enforcement = enforcement_cnis(daemonsets)
    policies = kubectl_json("get", "networkpolicies", "--all-namespaces")
    api_names = set(kubectl("api-resources", "--api-group", "networking.k8s.io", "-o", "name").splitlines())
    policy_names = [item["metadata"]["name"] for item in objects]
    live_policies = [compact_identity(item) for item in policies.get("items", [])]
    present = sorted(
        item["name"] for item in live_policies
        if item["namespace"] == contract["target_namespace"] and item["name"] in policy_names
    )
    if active_enforcement:
        raise EvidenceError(
            "an enforcement-capable CNI is now present; use the separately authorized isolated probe path before applying N009"
        )
    if present:
        raise EvidenceError("N009 candidate objects are already present although this runner never applies them")

    n008 = json.loads((ROOT / N008).read_text(encoding="utf-8"))
    blockers = {
        "APPROVED_EQUIVALENT_CONTROL_REQUIRED",
        "AUTH_OIDC_NODEPORT_EXCEPTION_APPROVAL_REQUIRED",
        "CNI_POLICY_ENFORCEMENT_REQUIRED",
        "DEPLOYABLE_CANDIDATE_REQUIRED",
        "REAL_UNAUTHORIZED_POD_PORT_EXTERNAL_NEGATIVE_TESTS_REQUIRED",
    }
    if n008.get("acceptance_status") != "PASS":
        blockers.add("N008_ACCEPTANCE_REQUIRED")
    evidence: dict[str, Any] = {
        "schema_version": 1,
        "artifact_kind": "M10_NETWORK_POLICY_K8S_ENFORCEMENT_RESULT",
        "task_id": "T1-M10-N009",
        "atomic_pr_ids": ["T1-M10-P021-OPS-n009-s1", "T1-M10-P022-TST-POST-n009-s2"],
        "run_id": str(uuid.uuid4()),
        "captured_at": dt.datetime.now(dt.timezone.utc).isoformat().replace("+00:00", "Z"),
        "status": "BLOCKED",
        "engineering_status": "PASS",
        "acceptance_status": "BLOCKED_CNI_ENFORCEMENT_MISSING",
        "profile_id": "M10-N009-K8S-MINIMUM-NETWORK-POLICY-V1",
        "environment_kind": "KUBERNETES",
        "candidate_id": n008.get("candidate_id"),
        "contract_sha256": sha256(builder.CONTRACT),
        "generated_manifest_sha256": sha256(builder.OUTPUT),
        "source_sha256": {str(path): sha256(ROOT / path) for path in SOURCE_FILES},
        "cluster": {
            "context": context,
            "server_version": version.get("serverVersion", {}).get("gitVersion"),
            "network_policy_api_available": "networkpolicies.networking.k8s.io" in api_names,
            "target_namespace": compact_identity(namespace),
            "nodes": sorted([compact_identity(item) for item in nodes.get("items", [])], key=lambda item: str(item["name"])),
            "cni_daemonsets": daemonsets,
            "policy_enforcement_cnis": active_enforcement,
            "live_network_policy_count": len(live_policies),
            "live_network_policies": sorted(live_policies, key=lambda item: (str(item["namespace"]), str(item["name"]))),
        },
        "candidate": {
            "object_count": len(objects),
            "policy_names": policy_names,
            "client_dry_run": "PASS",
            "present_in_target_namespace": present,
            "applied_by_run": False,
            "exception_approvals": [{
                "exception_id": item["exception_id"],
                "approval_status": item["approval_status"],
                "risk_owner_role": item["risk_owner_role"],
            } for item in contract["exceptions"]],
        },
        "negative_probes": {
            "unauthorized_pod": PROBE_BLOCKED,
            "unauthorized_port": PROBE_BLOCKED,
            "external_egress": PROBE_BLOCKED,
        },
        "blocking_codes": sorted(blockers),
        "required_gates": {
            "G0": "BLOCKED_BY_DEPLOYABLE_CANDIDATE",
            "G2": "PASS_STATIC_AND_MUTATION_TESTS",
            "G3": "BLOCKED_CNI_ENFORCEMENT_MISSING",
            "G6": "BLOCKED_NOT_APPLIED",
        },
        "shared_infrastructure_touched": False,
        "production_applied": False,
        "run_scoped_resources_created": False,
        "allowed_claims": [
            "The checked-in N009 candidate is deterministic, API-valid, default-off, and blocked before apply because the observed CNI cannot enforce NetworkPolicy"
        ],
        "does_not_prove": [
            "the current NetworkPolicy objects are enforced",
            "the N009 candidate was applied",
            "unauthorized pod, port, or external egress probes failed",
            "the auth OIDC NodePort exception was approved",
            "G3, G6, or production promotion passed",
        ],
    }
    return evidence


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--output", type=Path, default=OUTPUT)
    args = parser.parse_args()
    evidence = capture()
    output = args.output if args.output.is_absolute() else ROOT / args.output
    output.parent.mkdir(parents=True, exist_ok=True)
    temporary = output.with_name(f".{output.name}.tmp")
    temporary.write_text(json.dumps(evidence, indent=2, sort_keys=True) + "\n", encoding="utf-8")
    temporary.replace(output)
    print(json.dumps({
        "output": str(output.relative_to(ROOT)),
        "run_id": evidence["run_id"],
        "status": evidence["status"],
        "acceptance_status": evidence["acceptance_status"],
        "cni_daemonsets": [item["name"] for item in evidence["cluster"]["cni_daemonsets"]],
        "policy_enforcement_cnis": evidence["cluster"]["policy_enforcement_cnis"],
        "production_applied": evidence["production_applied"],
    }, sort_keys=True))
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
