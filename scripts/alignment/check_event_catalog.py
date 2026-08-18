#!/usr/bin/env python3
"""Validate Kafka topic, schema and producer/consumer catalog alignment."""

from __future__ import annotations

import json
import re
from collections import Counter
from pathlib import Path
from typing import Any


ROOT = Path(__file__).resolve().parents[2]
CATALOG = ROOT / "contracts/events/kafka-topic-catalog.v1.json"

ENTRY_PATTERN = re.compile(
    r'^\s*"([^"\n]+)"\s*(?:\\|;\s*do)\s*$',
    re.MULTILINE,
)
READINESS = {
    "active",
    "producer_candidate_default_off",
    "consumer_candidate_default_off",
    "producer_only",
    "consumer_only",
    "declared_only",
    "dlq",
}
M03_TOPIC_ROLES = ("session_topic", "feature_topic")
M03_EMBEDDED_FEATURE_MESSAGES = (
    "traffic.v1.FeatureSeq",
    "traffic.v1.FeatureFingerprint",
)
M03_UNAPPROVED_TOPIC_NAMES = ("feature.stats.v1", "feature.fingerprint.v1")


def _topic_entries(path: Path, field_count: int) -> tuple[dict[str, dict[str, Any]], list[str]]:
    entries: dict[str, dict[str, Any]] = {}
    errors: list[str] = []
    for raw in ENTRY_PATTERN.findall(path.read_text(encoding="utf-8")):
        if field_count == 6:
            prefix = raw.split(":", 4)
            fields = [*prefix[:4], *prefix[4].rsplit(":", 1)] if len(prefix) == 5 else prefix
        else:
            fields = raw.split(":")
        if len(fields) != field_count:
            continue
        name = fields[0]
        try:
            values: dict[str, Any] = {
                "name": name,
                "partitions": int(fields[1]),
                "retention_ms": int(fields[2]),
                "retention_bytes": int(fields[3]),
            }
        except ValueError:
            errors.append(f"{path.relative_to(ROOT)} has invalid numeric fields for {name}")
            continue
        if field_count == 6:
            values["key_contract"] = fields[4]
            values["message_type"] = fields[5]
        if name in entries:
            errors.append(f"{path.relative_to(ROOT)} has duplicate topic {name}")
        entries[name] = values
    return entries, errors


def _verify_proto_schema(schema: dict[str, Any], errors: list[str], prefix: str) -> None:
    path = ROOT / str(schema.get("path", ""))
    message = str(schema.get("message", ""))
    if not path.is_file():
        errors.append(f"{prefix} protobuf path does not exist: {schema.get('path')}")
        return
    if not message.startswith("traffic.v1."):
        errors.append(f"{prefix} protobuf message must be fully qualified")
        return
    short_name = message.rsplit(".", 1)[-1]
    source = path.read_text(encoding="utf-8", errors="ignore")
    if not re.search(rf"\bmessage\s+{re.escape(short_name)}\s*\{{", source):
        errors.append(f"{prefix} protobuf message {message} is absent from {schema.get('path')}")


def _verify_json_schema(
    schema: dict[str, Any],
    cache: dict[Path, dict[str, Any]],
    errors: list[str],
    prefix: str,
) -> None:
    path = ROOT / str(schema.get("path", ""))
    definition = str(schema.get("definition", ""))
    if not path.is_file():
        errors.append(f"{prefix} JSON Schema path does not exist: {schema.get('path')}")
        return
    if path not in cache:
        try:
            cache[path] = json.loads(path.read_text(encoding="utf-8"))
        except json.JSONDecodeError as exc:
            errors.append(f"{prefix} invalid JSON Schema: {exc}")
            return
    if definition not in cache[path].get("$defs", {}):
        errors.append(f"{prefix} JSON Schema definition is absent: {definition}")


def _verify_m03_session_feature_contract(
    catalog: dict[str, Any],
    catalog_topics: dict[str, dict[str, Any]],
    managed_topic_names: set[str],
    errors: list[str],
) -> None:
    """Freeze the existing M03 session/feature rail without inventing topics."""
    contract = catalog.get("m03_session_feature_contract")
    if not isinstance(contract, dict):
        errors.append("m03_session_feature_contract is required")
        return

    for role in M03_TOPIC_ROLES:
        frozen = contract.get(role)
        if not isinstance(frozen, dict):
            errors.append(f"m03_session_feature_contract.{role} is required")
            continue
        name = str(frozen.get("name", ""))
        topic = catalog_topics.get(name)
        if topic is None:
            errors.append(f"M03 frozen {role} topic is absent: {name!r}")
            continue
        comparisons = {
            "key_contract": topic.get("key_contract"),
            "message_type": topic.get("message_type"),
            "schema_message": topic.get("schema", {}).get("message"),
            "producers": topic.get("producers"),
            "consumers": topic.get("consumers"),
        }
        for field, actual in comparisons.items():
            if frozen.get(field) != actual:
                errors.append(
                    f"M03 frozen {role} {field} differs: "
                    f"frozen={frozen.get(field)!r} catalog={actual!r}"
                )

    embedded = tuple(contract.get("embedded_feature_messages", []))
    if embedded != M03_EMBEDDED_FEATURE_MESSAGES:
        errors.append(
            "M03 embedded feature messages must remain FeatureSeq and "
            "FeatureFingerprint in canonical order"
        )
    feature_source = (ROOT / "proto/traffic/v1/feature.proto").read_text(encoding="utf-8")
    batch_match = re.search(r"\bmessage\s+FeatureBatch\s*\{(?P<body>.*?)\n\}", feature_source, re.DOTALL)
    batch_body = batch_match.group("body") if batch_match else ""
    if not re.search(r"\brepeated\s+FeatureSeq\s+sequences\s*=\s*2\s*;", batch_body):
        errors.append("FeatureBatch must retain FeatureSeq sequences at field 2")
    if not re.search(r"\brepeated\s+FeatureFingerprint\s+fingerprints\s*=\s*3\s*;", batch_body):
        errors.append("FeatureBatch must retain FeatureFingerprint fingerprints at field 3")

    unapproved = tuple(contract.get("unapproved_topic_names", []))
    if unapproved != M03_UNAPPROVED_TOPIC_NAMES:
        errors.append("M03 unapproved topic names differ from the frozen contract")
    for name in M03_UNAPPROVED_TOPIC_NAMES:
        if name in managed_topic_names:
            errors.append(f"M03 unapproved topic is managed without a separate contract: {name}")
    if not str(contract.get("new_topic_requires", "")).strip():
        errors.append("M03 new topic approval requirement is missing")


def validate() -> dict[str, Any]:
    catalog = json.loads(CATALOG.read_text(encoding="utf-8"))
    source_path = ROOT / catalog["source_topic_catalog"]
    deployment_path = ROOT / catalog["deployment_topic_catalog"]
    source_topics, errors = _topic_entries(source_path, 6)
    deployment_topics, deployment_errors = _topic_entries(deployment_path, 4)
    errors.extend(deployment_errors)

    catalog_topics: dict[str, dict[str, Any]] = {}
    schema_cache: dict[Path, dict[str, Any]] = {}
    readiness_counts: Counter[str] = Counter()
    schema_counts: Counter[str] = Counter()
    for topic in catalog.get("topics", []):
        name = str(topic.get("name", ""))
        prefix = f"topic {name or '<missing>'}"
        if not name:
            errors.append("catalog topic name is required")
            continue
        if name in catalog_topics:
            errors.append(f"catalog has duplicate topic {name}")
            continue
        catalog_topics[name] = topic

        readiness = str(topic.get("readiness", ""))
        readiness_counts[readiness] += 1
        if readiness not in READINESS:
            errors.append(f"{prefix} has invalid readiness {readiness!r}")
        producers = topic.get("producers")
        consumers = topic.get("consumers")
        if not isinstance(producers, list) or not isinstance(consumers, list):
            errors.append(f"{prefix} producers and consumers must be arrays")
            continue
        if readiness == "active" and (not producers or not consumers):
            errors.append(f"{prefix} active readiness requires producers and consumers")
        if readiness == "producer_candidate_default_off" and (not producers or not consumers):
            errors.append(
                f"{prefix} producer_candidate_default_off readiness requires producer and consumer candidates"
            )
        if readiness == "consumer_candidate_default_off" and (not producers or not consumers):
            errors.append(
                f"{prefix} consumer_candidate_default_off readiness requires producer and consumer candidates"
            )
        if readiness == "producer_only" and (not producers or consumers):
            errors.append(f"{prefix} producer_only readiness must have producers and no consumers")
        if readiness == "consumer_only" and (producers or not consumers):
            errors.append(f"{prefix} consumer_only readiness must have consumers and no producers")
        if readiness == "declared_only" and (producers or consumers):
            errors.append(f"{prefix} declared_only readiness cannot claim producers or consumers")
        if readiness in {
            "producer_candidate_default_off", "consumer_candidate_default_off", "producer_only", "consumer_only", "declared_only", "dlq"
        } and not str(
            topic.get("known_gap", "")
        ).strip():
            errors.append(f"{prefix} non-active readiness requires known_gap")
        if readiness == "dlq" and not producers:
            errors.append(f"{prefix} dlq readiness requires at least one producer")

        for role, references in (("producer", producers), ("consumer", consumers)):
            for reference in references:
                path = ROOT / str(reference)
                if not path.is_file():
                    errors.append(f"{prefix} {role} reference does not exist: {reference}")

        schema = topic.get("schema", {})
        kind = schema.get("kind")
        schema_counts[str(kind)] += 1
        if kind == "protobuf":
            _verify_proto_schema(schema, errors, prefix)
        elif kind == "json-schema":
            _verify_json_schema(schema, schema_cache, errors, prefix)
        else:
            errors.append(f"{prefix} has unsupported schema kind {kind!r}")

        projection = topic.get("edge_projection")
        if projection is not None:
            _verify_proto_schema(projection, errors, prefix + " edge projection")

    if len(catalog_topics) != len(source_topics):
        errors.append(
            "canonical Kafka catalog topic count must match the source topic catalog: "
            f"source={len(source_topics)}, catalog={len(catalog_topics)}"
        )

    source_names = set(source_topics)
    catalog_names = set(catalog_topics)
    if missing := sorted(source_names - catalog_names):
        errors.append(f"source topic catalog entries are missing from event catalog: {missing}")
    if extra := sorted(catalog_names - source_names):
        errors.append(f"event catalog topics are absent from source topic catalog: {extra}")

    comparable_fields = (
        "partitions",
        "retention_ms",
        "retention_bytes",
        "key_contract",
        "message_type",
    )
    for name in sorted(source_names & catalog_names):
        for field in comparable_fields:
            if source_topics[name][field] != catalog_topics[name].get(field):
                errors.append(
                    f"topic {name} {field} differs: source={source_topics[name][field]!r} "
                    f"catalog={catalog_topics[name].get(field)!r}"
                )

    allowed_additional: dict[str, dict[str, Any]] = {}
    for item in catalog.get("allowed_additional_topics", []):
        name = str(item.get("name", ""))
        if not name:
            errors.append("allowed additional topic name is required")
            continue
        if name in allowed_additional or name in catalog_topics:
            errors.append(f"duplicate or canonical allowed additional topic {name}")
        allowed_additional[name] = item
        if not str(item.get("owner", "")).strip() or not str(item.get("known_gap", "")).strip():
            errors.append(f"allowed additional topic {name} requires owner and known_gap")

    deployment_names = set(deployment_topics)
    if missing := sorted(catalog_names - deployment_names):
        errors.append(f"canonical topics are missing from Kubernetes init: {missing}")
    unexpected = sorted(deployment_names - catalog_names - set(allowed_additional))
    if unexpected:
        errors.append(f"Kubernetes init has unregistered additional topics: {unexpected}")
    stale_allowed = sorted(set(allowed_additional) - deployment_names)
    if stale_allowed:
        errors.append(f"allowed additional topics are absent from Kubernetes init: {stale_allowed}")

    observed_additional: set[str] = set()
    for section in ("observed_runtime_additional_topics", "observed_environment_test_topics"):
        for item in catalog.get(section, []):
            name = str(item.get("name", ""))
            if not name:
                errors.append(f"{section} topic name is required")
                continue
            if (
                name in observed_additional
                or name in catalog_names
                or name in allowed_additional
            ):
                errors.append(f"duplicate or managed observed topic {name}")
            observed_additional.add(name)
            if not str(item.get("owner", "")).strip() or not str(
                item.get("known_gap", "")
            ).strip():
                errors.append(f"observed topic {name} requires owner and known_gap")

    _verify_m03_session_feature_contract(
        catalog,
        catalog_topics,
        catalog_names | set(allowed_additional) | observed_additional | deployment_names,
        errors,
    )

    deployment_fields = ("partitions", "retention_ms", "retention_bytes")
    for name in sorted(catalog_names & deployment_names):
        for field in deployment_fields:
            if deployment_topics[name][field] != catalog_topics[name].get(field):
                errors.append(
                    f"topic {name} {field} differs: kubernetes={deployment_topics[name][field]!r} "
                    f"catalog={catalog_topics[name].get(field)!r}"
                )

    return {
        "schema_version": 1,
        "result": "pass" if not errors else "blocked",
        "counts": {
            "canonical_topics": len(catalog_topics),
            "kubernetes_additional_topics": len(allowed_additional),
            "observed_additional_topics": len(observed_additional),
            "readiness": dict(sorted(readiness_counts.items())),
            "schema_kinds": dict(sorted(schema_counts.items())),
        },
        "gaps": {
            "producer_only": sorted(
                name for name, topic in catalog_topics.items() if topic["readiness"] == "producer_only"
            ),
            "consumer_only": sorted(
                name for name, topic in catalog_topics.items() if topic["readiness"] == "consumer_only"
            ),
            "dlq": sorted(name for name, topic in catalog_topics.items() if topic["readiness"] == "dlq"),
            "allowed_additional": sorted(allowed_additional),
            "observed_additional": sorted(observed_additional),
        },
        "errors": errors,
    }


def main() -> int:
    result = validate()
    print(json.dumps(result, ensure_ascii=False, indent=2))
    return 0 if result["result"] == "pass" else 1


if __name__ == "__main__":
    raise SystemExit(main())
