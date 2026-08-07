#!/usr/bin/env python3
"""Reconcile a read-only Kafka topic describe capture with the event catalog."""

from __future__ import annotations

import argparse
import hashlib
import json
import re
from pathlib import Path
from typing import Any


ROOT = Path(__file__).resolve().parents[2]
CATALOG = ROOT / "contracts/events/kafka-topic-catalog.v1.json"
TOPIC_LINE = re.compile(
    r"^Topic:\s+(?P<name>\S+)\s+TopicId:\s+\S+\s+"
    r"PartitionCount:\s+(?P<partitions>\d+)\s+"
    r"ReplicationFactor:\s+(?P<replication>\d+)\s+Configs:\s+(?P<configs>.*)$"
)


def parse_describe(payload: str) -> dict[str, dict[str, Any]]:
    topics: dict[str, dict[str, Any]] = {}
    for line in payload.splitlines():
        match = TOPIC_LINE.match(line.strip())
        if not match:
            continue
        configs: dict[str, str] = {}
        for item in match.group("configs").split(","):
            key, separator, value = item.partition("=")
            if separator:
                configs[key.strip()] = value.strip()
        topics[match.group("name")] = {
            "partitions": int(match.group("partitions")),
            "replication_factor": int(match.group("replication")),
            "retention_ms": int(configs["retention.ms"]) if "retention.ms" in configs else None,
            "retention_bytes": int(configs["retention.bytes"]) if "retention.bytes" in configs else None,
        }
    return topics


def reconcile(payload: str) -> dict[str, Any]:
    catalog = json.loads(CATALOG.read_text(encoding="utf-8"))
    observed = parse_describe(payload)
    canonical = {item["name"]: item for item in catalog["topics"]}
    kubernetes_additional = {item["name"] for item in catalog["allowed_additional_topics"]}
    runtime_additional = {item["name"] for item in catalog["observed_runtime_additional_topics"]}
    environment_test = {item["name"] for item in catalog["observed_environment_test_topics"]}
    registered = set(canonical) | kubernetes_additional | runtime_additional | environment_test
    internal = {"__consumer_offsets"}

    errors: list[str] = []
    missing = sorted(set(canonical) - set(observed))
    if missing:
        errors.append(f"canonical topics missing from live Kafka: {missing}")
    for name in sorted(set(canonical) & set(observed)):
        for field in ("partitions", "retention_ms", "retention_bytes"):
            if observed[name][field] != canonical[name][field]:
                errors.append(
                    f"live topic {name} {field} differs: "
                    f"live={observed[name][field]!r} catalog={canonical[name][field]!r}"
                )
        if observed[name]["replication_factor"] < 3:
            errors.append(
                f"live topic {name} replication factor is "
                f"{observed[name]['replication_factor']}, expected at least 3"
            )

    unknown = sorted(set(observed) - registered - internal)
    if unknown:
        errors.append(f"live Kafka has unregistered topics: {unknown}")

    return {
        "schema_version": 1,
        "result": "pass" if not errors else "blocked",
        "read_only": True,
        "counts": {
            "observed_topics_excluding_internal": len(set(observed) - internal),
            "canonical_expected": len(canonical),
            "canonical_present": len(set(canonical) & set(observed)),
            "kubernetes_additional_present": len(kubernetes_additional & set(observed)),
            "runtime_additional_present": len(runtime_additional & set(observed)),
            "environment_test_present": len(environment_test & set(observed)),
            "unknown": len(unknown),
        },
        "registered_additional_topics": {
            "kubernetes": sorted(kubernetes_additional & set(observed)),
            "runtime_only": sorted(runtime_additional & set(observed)),
            "environment_test": sorted(environment_test & set(observed)),
        },
        "missing_canonical_topics": missing,
        "unknown_topics": unknown,
        "errors": errors,
    }


def main() -> int:
    parser = argparse.ArgumentParser()
    parser.add_argument("--describe-file", type=Path, required=True)
    parser.add_argument("--output", type=Path)
    args = parser.parse_args()
    payload = args.describe_file.read_text(encoding="utf-8")
    result = reconcile(payload)
    result["describe_sha256"] = hashlib.sha256(
        args.describe_file.read_bytes()
    ).hexdigest()
    rendered = json.dumps(result, ensure_ascii=False, indent=2) + "\n"
    if args.output:
        args.output.parent.mkdir(parents=True, exist_ok=True)
        args.output.write_text(rendered, encoding="utf-8")
    print(rendered, end="")
    return 0 if result["result"] == "pass" else 1


if __name__ == "__main__":
    raise SystemExit(main())
