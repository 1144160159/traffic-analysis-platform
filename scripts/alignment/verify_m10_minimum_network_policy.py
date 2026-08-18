#!/usr/bin/env python3
"""Verify T1-M10-N009 policy determinism and fail-closed semantics."""

from __future__ import annotations

import json
import sys
from pathlib import Path
from typing import Any

import yaml


ROOT_HINT = Path(__file__).resolve().parents[2]
if str(ROOT_HINT) not in sys.path:
    sys.path.insert(0, str(ROOT_HINT))

from scripts.alignment import build_m10_minimum_network_policy as builder


def load_yaml(path: Path = builder.OUTPUT) -> list[dict[str, Any]]:
    documents = [item for item in yaml.safe_load_all(path.read_text(encoding="utf-8")) if item is not None]
    if not all(isinstance(item, dict) for item in documents):
        raise ValueError("generated manifest contains a non-object document")
    return documents


def validate(contract: dict[str, Any], actual: list[dict[str, Any]]) -> list[str]:
    errors: list[str] = []
    try:
        expected = builder.build(contract)
    except ValueError as exc:
        return [str(exc)]
    if actual != expected:
        errors.append("generated manifest does not equal the deterministic contract rendering")
    if len(actual) != 10:
        errors.append("manifest must contain one default deny and nine workload policies")
    names: list[Any] = []
    workload_selectors: list[Any] = []
    for index, item in enumerate(actual):
        if item.get("apiVersion") != "networking.k8s.io/v1" or item.get("kind") != "NetworkPolicy":
            errors.append(f"object {index} is not a networking.k8s.io/v1 NetworkPolicy")
            continue
        metadata = item.get("metadata", {})
        spec = item.get("spec", {})
        names.append(metadata.get("name"))
        if metadata.get("namespace") != "traffic-analysis":
            errors.append(f"policy {metadata.get('name')} targets the wrong namespace")
        annotations = metadata.get("annotations", {})
        if annotations.get("traffic.analysis/candidate-default-off") != "true" or annotations.get("traffic.analysis/production-applied") != "false":
            errors.append(f"policy {metadata.get('name')} overclaims rollout state")
        if spec.get("policyTypes") != ["Ingress", "Egress"]:
            errors.append(f"policy {metadata.get('name')} must select ingress and egress")
        if metadata.get("name") == "m10-n009-default-deny":
            if spec != {"podSelector": {}, "policyTypes": ["Ingress", "Egress"]}:
                errors.append("default-deny policy was relaxed")
            continue
        selector = spec.get("podSelector", {}).get("matchLabels")
        if not isinstance(selector, dict) or not selector:
            errors.append(f"policy {metadata.get('name')} has an empty workload selector")
        workload_selectors.append(selector)
        for direction, peer_key in (("ingress", "from"), ("egress", "to")):
            rules = spec.get(direction)
            if not isinstance(rules, list) or not rules:
                errors.append(f"policy {metadata.get('name')} has no explicit {direction} rules")
                continue
            for rule_index, rule in enumerate(rules):
                peers = rule.get(peer_key) if isinstance(rule, dict) else None
                ports = rule.get("ports") if isinstance(rule, dict) else None
                if not isinstance(peers, list) or len(peers) != 1:
                    errors.append(f"policy {metadata.get('name')} {direction} {rule_index} must have exactly one peer")
                    continue
                if not isinstance(ports, list) or not ports:
                    errors.append(f"policy {metadata.get('name')} {direction} {rule_index} has no ports")
                    continue
                peer = peers[0]
                namespace = peer.get("namespaceSelector", {}).get("matchLabels", {}).get("kubernetes.io/metadata.name")
                pod_selector = peer.get("podSelector", {}).get("matchLabels")
                ip_block = peer.get("ipBlock", {}).get("cidr")
                if ip_block in ("0.0.0.0/0", "::/0"):
                    errors.append(f"policy {metadata.get('name')} permits Internet-wide egress")
                if namespace is None and ip_block is None:
                    errors.append(f"policy {metadata.get('name')} {direction} {rule_index} peer is not explicit")
                if namespace is not None and (not isinstance(pod_selector, dict) or not pod_selector):
                    errors.append(f"policy {metadata.get('name')} {direction} {rule_index} has an empty peer selector")
                port_numbers = {port.get("port") for port in ports if isinstance(port, dict)}
                if port_numbers.intersection({2379, 2380, 6443, 10250}):
                    errors.append(f"policy {metadata.get('name')} exposes a forbidden control-plane port")
                if namespace == "kube-system" and not (
                    direction == "egress"
                    and pod_selector == {"k8s-app": "kube-dns"}
                    and ports == [{"protocol": "UDP", "port": 53}, {"protocol": "TCP", "port": 53}]
                ):
                    errors.append(f"policy {metadata.get('name')} has a non-DNS kube-system rule")
    if len(names) != len(set(names)):
        errors.append("NetworkPolicy names are not unique")
    normalized_selectors = [json.dumps(item, sort_keys=True) for item in workload_selectors]
    if len(normalized_selectors) != len(set(normalized_selectors)):
        errors.append("multiple workload policies reuse one pod selector")
    if contract.get("production_applied") is not False:
        errors.append("contract overclaims production application")
    pending = [item for item in contract.get("exceptions", []) if item.get("approval_status") != "APPROVED"]
    if not pending:
        errors.append("unapproved site exception disappeared without a replacement contract")
    return errors


def main() -> int:
    if not builder.SCHEMA.is_file() or not builder.CONTRACT.is_file() or not builder.OUTPUT.is_file():
        print("FAIL: N009 schema, contract, or generated manifest is absent")
        return 1
    contract = builder.load()
    errors = validate(contract, load_yaml())
    if errors:
        for error in errors:
            print(f"FAIL: {error}")
        return 1
    print("PASS: T1-M10-N009 candidate is deterministic and fail-closed; production remains default off")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
