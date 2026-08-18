#!/usr/bin/env python3
"""Validate and deterministically render the T1-M10-N009 policy candidate."""

from __future__ import annotations

import argparse
import hashlib
import ipaddress
import json
from pathlib import Path
from typing import Any

import yaml


ROOT = Path(__file__).resolve().parents[2]
CONTRACT = ROOT / "contracts/security/m10-minimum-network-policy.v1.json"
SCHEMA = ROOT / "contracts/security/m10-minimum-network-policy.schema.json"
OUTPUT = ROOT / "deployments/kubernetes/security/m10-minimum-network-policies.v1.yaml"
EXPECTED_WORKLOADS = (
    "alert-service",
    "asset-service",
    "auth-service",
    "forensics-service",
    "graph-service",
    "ingest-gateway",
    "rule-manager",
    "threat-intel-service",
    "web-ui",
)
FIXED_FIELDS = {
    "schema_version": 1,
    "policy_id": "M10-N009-MINIMUM-NETWORK-POLICY-V1",
    "task_id": "T1-M10-N009",
    "status": "CANDIDATE_DEFAULT_OFF",
    "production_applied": False,
    "target_namespace": "traffic-analysis",
}


def load(path: Path = CONTRACT) -> dict[str, Any]:
    value = json.loads(path.read_text(encoding="utf-8"))
    if not isinstance(value, dict):
        raise ValueError("contract must be a JSON object")
    return value


def sha256(path: Path) -> str:
    return hashlib.sha256(path.read_bytes()).hexdigest()


def _selector(value: Any, context: str) -> dict[str, str]:
    if not isinstance(value, dict) or not value:
        raise ValueError(f"{context} selector must be non-empty")
    if any(not isinstance(key, str) or not key or not isinstance(item, str) or not item for key, item in value.items()):
        raise ValueError(f"{context} selector labels must be non-empty strings")
    return value


def _ports(value: Any, forbidden_ports: set[int], context: str) -> list[dict[str, Any]]:
    if not isinstance(value, list) or not value:
        raise ValueError(f"{context} ports must be non-empty")
    seen: set[tuple[str, int]] = set()
    for item in value:
        if not isinstance(item, dict) or set(item) != {"protocol", "port"}:
            raise ValueError(f"{context} port must contain only protocol and port")
        protocol, port = item["protocol"], item["port"]
        if protocol not in ("TCP", "UDP") or not isinstance(port, int) or isinstance(port, bool) or not 1 <= port <= 65535:
            raise ValueError(f"{context} contains an invalid port")
        if port in forbidden_ports:
            raise ValueError(f"{context} uses forbidden destination port {port}")
        if (protocol, port) in seen:
            raise ValueError(f"{context} contains a duplicate port")
        seen.add((protocol, port))
    return value


def validate(contract: dict[str, Any]) -> None:
    for field, expected in FIXED_FIELDS.items():
        if contract.get(field) != expected:
            raise ValueError(f"fixed contract field drifted: {field}")
    cni = contract.get("cni_requirement")
    if cni != {
        "api": "networking.k8s.io/v1",
        "enforcement_required": True,
        "yaml_objects_without_enforcement_do_not_pass": True,
        "approved_equivalent_control": None,
    }:
        raise ValueError("CNI requirement must remain fail-closed with no inferred equivalent control")
    forbidden = contract.get("forbidden")
    if not isinstance(forbidden, dict):
        raise ValueError("forbidden policy block is missing")
    if forbidden.get("empty_allow_selectors") is not True or forbidden.get("same_namespace_blanket_allow") is not True:
        raise ValueError("empty and same-namespace blanket allow guards must remain enabled")
    forbidden_namespaces = set(forbidden.get("destination_namespaces", []))
    forbidden_ports = set(forbidden.get("destination_ports", []))
    forbidden_cidrs = {str(ipaddress.ip_network(item, strict=True)) for item in forbidden.get("cidrs", [])}
    if "kube-system" not in forbidden_namespaces or 6443 not in forbidden_ports:
        raise ValueError("control-plane namespace and port guards are incomplete")
    if not {"0.0.0.0/0", "::/0"}.issubset(forbidden_cidrs):
        raise ValueError("Internet-wide CIDR guards are incomplete")

    dns = contract.get("dns_egress")
    if not isinstance(dns, dict) or dns.get("namespace") != "kube-system" or dns.get("pod_selector") != {"k8s-app": "kube-dns"}:
        raise ValueError("DNS exception must target only kube-system/kube-dns")
    if _ports(dns.get("ports"), set(), "DNS egress") != [
        {"protocol": "UDP", "port": 53},
        {"protocol": "TCP", "port": 53},
    ]:
        raise ValueError("DNS exception must contain only UDP/TCP 53 in fixed order")

    exceptions = contract.get("exceptions")
    if not isinstance(exceptions, list):
        raise ValueError("exceptions must be a list")
    exception_map: dict[str, dict[str, Any]] = {}
    for exception in exceptions:
        if not isinstance(exception, dict):
            raise ValueError("exception must be an object")
        exception_id = exception.get("exception_id")
        if not isinstance(exception_id, str) or not exception_id or exception_id in exception_map:
            raise ValueError("exception IDs must be non-empty and unique")
        if exception.get("approval_status") not in ("REQUIRED_NOT_APPROVED", "APPROVED"):
            raise ValueError(f"exception {exception_id} has an invalid approval status")
        for field in ("workload_id", "reason", "scope", "risk_owner_role", "replacement"):
            if not isinstance(exception.get(field), str) or not exception[field]:
                raise ValueError(f"exception {exception_id} is missing {field}")
        exception_map[exception_id] = exception

    workloads = contract.get("workloads")
    if not isinstance(workloads, list):
        raise ValueError("workloads must be a list")
    ids = [item.get("workload_id") for item in workloads if isinstance(item, dict)]
    if sorted(ids) != list(EXPECTED_WORKLOADS) or len(ids) != len(set(ids)):
        raise ValueError("workload closure must be the exact nine-service set")
    referenced_exceptions: set[str] = set()
    for workload in workloads:
        workload_id = workload["workload_id"]
        _selector(workload.get("pod_selector"), f"{workload_id} pod")
        ingress = workload.get("ingress")
        egress = workload.get("egress")
        if not isinstance(ingress, list) or not ingress or not isinstance(egress, list) or not egress:
            raise ValueError(f"{workload_id} must have explicit ingress and egress")
        for index, rule in enumerate(ingress):
            source = rule.get("from") if isinstance(rule, dict) else None
            if not isinstance(source, dict) or set(source) != {"namespace", "pod_selector"}:
                raise ValueError(f"{workload_id} ingress {index} requires namespace and pod selector")
            if not isinstance(source["namespace"], str) or not source["namespace"]:
                raise ValueError(f"{workload_id} ingress {index} namespace is invalid")
            _selector(source["pod_selector"], f"{workload_id} ingress {index}")
            _ports(rule.get("ports"), set(), f"{workload_id} ingress {index}")
        for index, rule in enumerate(egress):
            target = rule.get("to") if isinstance(rule, dict) else None
            if not isinstance(target, dict):
                raise ValueError(f"{workload_id} egress {index} target is missing")
            _ports(rule.get("ports"), forbidden_ports, f"{workload_id} egress {index}")
            if set(target) == {"namespace", "pod_selector"}:
                namespace = target["namespace"]
                if not isinstance(namespace, str) or not namespace:
                    raise ValueError(f"{workload_id} egress {index} namespace is invalid")
                if namespace in forbidden_namespaces:
                    raise ValueError(f"{workload_id} egress {index} targets forbidden namespace {namespace}")
                _selector(target["pod_selector"], f"{workload_id} egress {index}")
                if "exception_id" in rule:
                    raise ValueError(f"{workload_id} egress {index} has an unnecessary exception")
            elif set(target) == {"ip_block"}:
                try:
                    cidr = str(ipaddress.ip_network(target["ip_block"], strict=True))
                except (TypeError, ValueError) as exc:
                    raise ValueError(f"{workload_id} egress {index} CIDR is invalid") from exc
                if cidr in forbidden_cidrs:
                    raise ValueError(f"{workload_id} egress {index} uses forbidden CIDR {cidr}")
                exception_id = rule.get("exception_id")
                if not isinstance(exception_id, str) or exception_id not in exception_map:
                    raise ValueError(f"{workload_id} egress {index} IP block lacks a registered exception")
                exception = exception_map[exception_id]
                if exception["workload_id"] != workload_id:
                    raise ValueError(f"exception {exception_id} workload binding drifted")
                ports = rule["ports"]
                scope = f"{cidr}:{ports[0]['port']}/{ports[0]['protocol']}"
                if len(ports) != 1 or exception["scope"] != scope:
                    raise ValueError(f"exception {exception_id} scope does not exactly match its egress rule")
                referenced_exceptions.add(exception_id)
            else:
                raise ValueError(f"{workload_id} egress {index} target is not explicit")
    if referenced_exceptions != set(exception_map):
        raise ValueError("exception registry contains an unreferenced or missing exception")


def _peer(target: dict[str, Any]) -> dict[str, Any]:
    if "ip_block" in target:
        return {"ipBlock": {"cidr": target["ip_block"]}}
    return {
        "namespaceSelector": {"matchLabels": {"kubernetes.io/metadata.name": target["namespace"]}},
        "podSelector": {"matchLabels": target["pod_selector"]},
    }


def _ports_for_k8s(ports: list[dict[str, Any]]) -> list[dict[str, Any]]:
    return [{"protocol": item["protocol"], "port": item["port"]} for item in ports]


def build(contract: dict[str, Any] | None = None) -> list[dict[str, Any]]:
    value = load() if contract is None else contract
    validate(value)
    namespace = value["target_namespace"]
    common_labels = {
        "app.kubernetes.io/managed-by": "m10-network-policy-builder",
        "traffic.analysis/task": "t1-m10-n009",
    }
    annotations = {
        "traffic.analysis/candidate-default-off": "true",
        "traffic.analysis/production-applied": "false",
        "traffic.analysis/requires-enforcement-capable-cni": "true",
    }
    objects: list[dict[str, Any]] = [{
        "apiVersion": "networking.k8s.io/v1",
        "kind": "NetworkPolicy",
        "metadata": {
            "name": "m10-n009-default-deny",
            "namespace": namespace,
            "labels": common_labels,
            "annotations": annotations,
        },
        "spec": {"podSelector": {}, "policyTypes": ["Ingress", "Egress"]},
    }]
    dns = value["dns_egress"]
    dns_rule = {"to": [_peer(dns)], "ports": _ports_for_k8s(dns["ports"])}
    for workload in sorted(value["workloads"], key=lambda item: item["workload_id"]):
        ingress = [{
            "from": [_peer(rule["from"])],
            "ports": _ports_for_k8s(rule["ports"]),
        } for rule in workload["ingress"]]
        egress = [dns_rule, *[{
            "to": [_peer(rule["to"])],
            "ports": _ports_for_k8s(rule["ports"]),
        } for rule in workload["egress"]]]
        objects.append({
            "apiVersion": "networking.k8s.io/v1",
            "kind": "NetworkPolicy",
            "metadata": {
                "name": f"m10-n009-allow-{workload['workload_id']}",
                "namespace": namespace,
                "labels": common_labels,
                "annotations": annotations,
            },
            "spec": {
                "podSelector": {"matchLabels": workload["pod_selector"]},
                "policyTypes": ["Ingress", "Egress"],
                "ingress": ingress,
                "egress": egress,
            },
        })
    return objects


def render(objects: list[dict[str, Any]]) -> str:
    header = (
        "# Generated T1-M10-N009 candidate. DO NOT APPLY until a policy-enforcing CNI is\n"
        "# verified and the recorded exceptions and release window are approved.\n"
    )
    return header + yaml.safe_dump_all(objects, sort_keys=False, explicit_start=True)


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    mode = parser.add_mutually_exclusive_group(required=True)
    mode.add_argument("--write", action="store_true")
    mode.add_argument("--check", action="store_true")
    args = parser.parse_args()
    content = render(build())
    if args.check:
        if not OUTPUT.is_file() or OUTPUT.read_text(encoding="utf-8") != content:
            print("FAIL: T1-M10-N009 generated NetworkPolicy candidate is stale")
            return 1
        print("PASS: T1-M10-N009 NetworkPolicy candidate is current (10 objects, default off)")
        return 0
    OUTPUT.parent.mkdir(parents=True, exist_ok=True)
    temporary = OUTPUT.with_name(f".{OUTPUT.name}.tmp")
    temporary.write_text(content, encoding="utf-8")
    temporary.replace(OUTPUT)
    print(OUTPUT.relative_to(ROOT))
    print(f"contract_sha256={sha256(CONTRACT)} objects={len(build())} production_applied=false")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
