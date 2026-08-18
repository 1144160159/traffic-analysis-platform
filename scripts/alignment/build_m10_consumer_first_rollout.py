#!/usr/bin/env python3
"""Build the T1-M10-N010 consumer-first rollout admission policy."""

from __future__ import annotations

import argparse
import hashlib
import json
from pathlib import Path
from typing import Any


ROOT = Path(__file__).resolve().parents[2]
OUTPUT = ROOT / "contracts/deployments/m10-consumer-first-rollout.v1.json"
ACL_CATALOG = Path("contracts/events/kafka-acl-catalog.v1.json")
TOPIC_CATALOG = Path("contracts/events/kafka-topic-catalog.v1.json")
N009_EVIDENCE = Path("doc/02_acceptance/topic1/tasks/t1-m10-n009/k8s-network-policy-enforcement-latest.json")
SOURCE_FILES = (ACL_CATALOG, TOPIC_CATALOG, N009_EVIDENCE)


def load_json(path: Path) -> dict[str, Any]:
    value = json.loads(path.read_text(encoding="utf-8"))
    if not isinstance(value, dict):
        raise ValueError(f"{path} must contain a JSON object")
    return value


def sha256(path: Path) -> str:
    return hashlib.sha256(path.read_bytes()).hexdigest()


def canonical_sha256(value: Any) -> str:
    return hashlib.sha256(json.dumps(value, sort_keys=True, separators=(",", ":")).encode()).hexdigest()


def build() -> dict[str, Any]:
    acl = load_json(ROOT / ACL_CATALOG)
    topics = load_json(ROOT / TOPIC_CATALOG)
    n009 = load_json(ROOT / N009_EVIDENCE)
    if acl.get("topic_catalog") != str(TOPIC_CATALOG):
        raise ValueError("ACL catalog topic authority drifted")
    topic_map = {item.get("name"): item for item in topics.get("topics", []) if isinstance(item, dict)}
    bindings = acl.get("topic_bindings")
    if not isinstance(bindings, list) or set(topic_map) != {item.get("topic") for item in bindings if isinstance(item, dict)}:
        raise ValueError("topic and ACL binding sets must be exact")
    principals = {item.get("id") for item in acl.get("principals", []) if isinstance(item, dict)}
    rails: list[dict[str, Any]] = []
    producer_only: list[dict[str, Any]] = []
    for binding in sorted(bindings, key=lambda item: item["topic"]):
        topic = binding["topic"]
        producers = binding.get("producers")
        consumers = binding.get("consumers")
        if not isinstance(producers, list) or not producers or len(producers) != len(set(producers)):
            raise ValueError(f"{topic} producers must be non-empty and unique")
        if any(item not in principals or "*" in str(item) for item in producers):
            raise ValueError(f"{topic} contains an invalid producer principal")
        if not isinstance(consumers, list):
            raise ValueError(f"{topic} consumers must be a list")
        if not consumers:
            producer_only.append({
                "topic": topic,
                "producers": sorted(producers),
                "state": "BLOCKED_NO_CONSUMER",
                "producer_enablement_allowed": False,
            })
            continue
        consumer_items: list[dict[str, Any]] = []
        consumer_principals: set[str] = set()
        for consumer in sorted(consumers, key=lambda item: item["principal"]):
            principal = consumer.get("principal")
            groups = consumer.get("groups")
            prefixes = consumer.get("group_prefixes", [])
            if principal not in principals or principal in consumer_principals or "*" in str(principal):
                raise ValueError(f"{topic} contains an invalid or duplicate consumer principal")
            if not isinstance(groups, list) or not groups or len(groups) != len(set(groups)):
                raise ValueError(f"{topic}/{principal} consumer groups must be non-empty and unique")
            if not isinstance(prefixes, list) or len(prefixes) != len(set(prefixes)):
                raise ValueError(f"{topic}/{principal} group prefixes must be unique")
            if any(not isinstance(item, str) or not item or "*" in item for item in [*groups, *prefixes]):
                raise ValueError(f"{topic}/{principal} has an invalid consumer group")
            consumer_principals.add(principal)
            consumer_items.append({
                "principal": principal,
                "required_groups": sorted(groups),
                "allowed_shadow_group_prefixes": sorted(prefixes),
            })
        topic_contract = topic_map[topic]
        if not topic_contract.get("consumers"):
            raise ValueError(f"{topic} ACL declares consumers but topic contract does not")
        rails.append({
            "topic": topic,
            "topic_readiness": topic_contract.get("readiness"),
            "key_contract": topic_contract.get("key_contract"),
            "schema": topic_contract.get("schema"),
            "producers": sorted(producers),
            "consumers": consumer_items,
            "required_sequence": [
                "CONSUMER_DEPLOYED_IDLE_COMPATIBLE",
                "CONSUMER_READY_RECEIPT_CURRENT",
                "PRODUCER_TENANT_ENABLE",
            ],
            "producer_enablement_allowed": "ONLY_AFTER_EXACT_RECEIPT_CLOSURE",
        })
    additional = acl.get("additional_topic_bindings")
    if not isinstance(additional, list) or any(item.get("state") != "blocked" for item in additional if isinstance(item, dict)):
        raise ValueError("additional topic bindings must remain explicitly blocked")
    blocked_additional = [{
        "topic": item["topic"],
        "state": "BLOCKED_UNVERSIONED_OR_UNOWNED",
        "reason": item["reason"],
        "producer_enablement_allowed": False,
    } for item in sorted(additional, key=lambda item: item["topic"])]
    payload: dict[str, Any] = {
        "schema_version": 1,
        "artifact_kind": "M10_CONSUMER_FIRST_ROLLOUT_POLICY",
        "policy_id": "M10-N010-CONSUMER-FIRST-ROLLOUT-V1",
        "task_id": "T1-M10-N010",
        "atomic_pr_id": "T1-M10-P024-OPS-n010-s1",
        "status": "CANDIDATE_DEFAULT_OFF",
        "production_applied": False,
        "source_sha256": {str(path): sha256(ROOT / path) for path in SOURCE_FILES},
        "dependency": {
            "task_id": "T1-M10-N009",
            "required_acceptance_status": "PASS",
            "observed_acceptance_status": n009.get("acceptance_status"),
            "observed_run_id": n009.get("run_id"),
            "admission": "DENY_UNLESS_EXACT_PASS",
        },
        "admission_contract": {
            "candidate_id": "REQUIRED_SHA256",
            "tenant_id": "REQUIRED_EXPLICIT_NO_WILDCARD",
            "receipt_max_age_seconds": 300,
            "receipt_candidate_binding": "EXACT",
            "receipt_tenant_binding": "EXACT",
            "ready_replicas_minimum": 1,
            "assigned_partitions_minimum": 1,
            "consumer_state": "READY_IDLE_COMPATIBLE",
            "consumer_writes_enabled": False,
            "producer_without_current_consumer_receipt": "DENY",
            "producer_only_topic": "DENY",
            "unknown_topic_or_principal": "DENY",
            "rollback_order": ["STOP_PRODUCER_ADMISSION", "DRAIN_IN_FLIGHT", "KEEP_CONSUMER_READY", "RECONCILE_OFFSETS_AND_AUTHORITY"],
        },
        "rails": rails,
        "producer_only_blockers": producer_only,
        "additional_topic_blockers": blocked_additional,
        "synchronous_authority_write_exceptions": [],
        "exception_policy": {
            "default": "DENY_UNREGISTERED",
            "required_fields": ["exception_id", "authority", "operation", "tenant_scope", "risk_owner", "approval_id", "expires_at", "rollback"],
            "wildcard_tenant_forbidden": True,
        },
        "counts": {
            "catalog_topics": len(bindings),
            "consumer_first_rails": len(rails),
            "producer_only_blockers": len(producer_only),
            "additional_topic_blockers": len(blocked_additional),
            "synchronous_authority_write_exceptions": 0,
        },
        "allowed_claims": [
            "an exact candidate and tenant producer action may be admitted only after all bound consumer groups have current idle-compatible receipts"
        ],
        "does_not_prove": [
            "N009 passed",
            "any consumer receipt exists in the site",
            "any producer was enabled",
            "a deployable M10 candidate exists",
        ],
    }
    payload["policy_sha256"] = canonical_sha256(payload)
    return payload


def render(value: dict[str, Any]) -> str:
    return json.dumps(value, indent=2, sort_keys=True) + "\n"


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    mode = parser.add_mutually_exclusive_group(required=True)
    mode.add_argument("--write", action="store_true")
    mode.add_argument("--check", action="store_true")
    args = parser.parse_args()
    content = render(build())
    if args.check:
        if not OUTPUT.is_file() or OUTPUT.read_text(encoding="utf-8") != content:
            print("FAIL: T1-M10-N010 consumer-first policy is stale")
            return 1
        print("PASS: T1-M10-N010 consumer-first policy is current")
        return 0
    OUTPUT.parent.mkdir(parents=True, exist_ok=True)
    temporary = OUTPUT.with_name(f".{OUTPUT.name}.tmp")
    temporary.write_text(content, encoding="utf-8")
    temporary.replace(OUTPUT)
    value = build()
    print(OUTPUT.relative_to(ROOT))
    print(json.dumps(value["counts"], sort_keys=True))
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
