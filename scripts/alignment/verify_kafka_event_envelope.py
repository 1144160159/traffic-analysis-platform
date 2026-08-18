#!/usr/bin/env python3
"""Verify the repository-side T-KAFKA-002 detection envelope vertical slice."""

from __future__ import annotations

import json
import re
from pathlib import Path
from typing import Any


ROOT = Path(__file__).resolve().parents[2]
CONTRACT = Path("contracts/kafka/event-envelope-idempotency.v1.json")

HEADER_TAGS = {
    "event_id": 1, "tenant_id": 2, "run_id": 3, "event_ts": 4,
    "ingest_ts": 5, "probe_id": 6, "feature_set_id": 7, "kafka_ts": 8,
    "flink_out_ts": 9, "event_type": 10, "schema_version": 11,
    "aggregate_type": 12, "aggregate_id": 13, "aggregate_version": 14,
    "occurred_at": 15, "produced_at": 16, "trace_id": 17,
    "causation_id": 18, "correlation_id": 19, "idempotency_key": 20,
    "producer": 21,
}


def _load(path: Path) -> dict[str, Any]:
    return json.loads(path.read_text(encoding="utf-8"))


def _message(source: str, name: str) -> str:
    match = re.search(rf"message\s+{re.escape(name)}\s*\{{(.*?)\n\}}", source, re.S)
    return match.group(1) if match else ""


def _field_tag(body: str, field: str) -> int | None:
    match = re.search(rf"\b{re.escape(field)}\s*=\s*(\d+)\s*;", body)
    return int(match.group(1)) if match else None


def verify(root: Path = ROOT) -> dict[str, Any]:
    contract = _load(root / CONTRACT)
    errors: list[str] = []
    if contract.get("schema_version") != 1 or contract.get("remediation_id") != "T-KAFKA-002":
        errors.append("Kafka envelope contract must be schema v1 for T-KAFKA-002")
    if contract.get("status") not in {"implementing", "verifying"}:
        errors.append("contract must not claim closure before live replay and reconciliation")
    if contract.get("production_applied") is not False:
        errors.append("repository contract must not claim production application")
    if contract.get("coverage_status") != "PARTIAL_DETECTION_VERTICAL_SLICE":
        errors.append("contract must retain partial detection-slice coverage")
    compatibility = contract.get("compatibility") or {}
    if compatibility.get("mode") != "additive" or compatibility.get("removed_fields") != [] or compatibility.get("renumbered_fields") != []:
        errors.append("event envelope migration must be additive with no removed or renumbered fields")
    if len(contract.get("remaining_gates") or []) < 7:
        errors.append("contract must retain registry live replay failure performance and rollout gates")

    common = (root / "proto/traffic/v1/common.proto").read_text(encoding="utf-8")
    header = _message(common, "EventHeader")
    if not header:
        errors.append("traffic.v1.EventHeader is missing")
    tag_map = {field: _field_tag(header, field) for field in HEADER_TAGS}
    for field, expected in HEADER_TAGS.items():
        if tag_map[field] != expected:
            errors.append(f"EventHeader {field} must retain tag {expected}, found {tag_map[field]}")
    required = set(contract.get("required_fields") or [])
    expected_required = set(HEADER_TAGS) - {"run_id", "event_ts", "ingest_ts", "probe_id", "feature_set_id", "kafka_ts", "flink_out_ts"}
    if required != expected_required:
        errors.append("contract required_fields do not match the v1 envelope")

    feature = _message((root / "proto/traffic/v1/feature.proto").read_text(encoding="utf-8"), "FeatureStat")
    detection = _message((root / "proto/traffic/v1/detection.proto").read_text(encoding="utf-8"), "DetectionBehavior")
    if _field_tag(feature, "tuple") != 23 or _field_tag(feature, "evidence_ids") != 24:
        errors.append("FeatureStat must add tuple=23 and evidence_ids=24")
    if _field_tag(detection, "tuple") != 11 or _field_tag(detection, "evidence_ids") != 12:
        errors.append("DetectionBehavior must add tuple=11 and evidence_ids=12")

    feature_java = (root / "java/flink-jobs/flink-feature-job/src/main/java/com/traffic/flink/feature/calculator/FeatureCalculator.java").read_text(encoding="utf-8")
    for token in (
        '.setEventType("traffic.feature.stat.v1")', ".setTuple(session.getTuple())",
        ".addAllEvidenceIds(evidenceIds)", '.setProducer("flink-feature-job")',
        ".setCausationId(sourceHeader.getEventId())",
    ):
        if token not in feature_java:
            errors.append(f"Feature producer missing {token}")

    behavior_files = (
        "java/flink-jobs/flink-behavior-job/src/main/java/com/traffic/flink/behavior/detector/BehaviorDetectorFunction.java",
        "java/flink-jobs/flink-behavior-job/src/main/java/com/traffic/flink/behavior/detector/SyncBehaviorDetector.java",
    )
    for relative in behavior_files:
        source = (root / relative).read_text(encoding="utf-8")
        if "BehaviorDetectionEventFactory.build(" not in source:
            errors.append(f"{Path(relative).name} must delegate to the canonical behavior envelope factory")
    behavior_factory = (
        root / "java/flink-jobs/flink-behavior-job/src/main/java/com/traffic/flink/behavior/detector/BehaviorDetectionEventFactory.java"
    ).read_text(encoding="utf-8")
    for token in (
        'EVENT_TYPE = "traffic.detection.behavior.v1"',
        'PRODUCER = "flink-behavior-job"',
        ".setEventType(EVENT_TYPE)",
        ".setTuple(input.getTuple())",
        ".addAllEvidenceIds(input.getEvidenceIdsList())",
        ".setProducer(PRODUCER)",
        ".setIdempotencyKey(eventId)",
        'DeterministicId.uuid(',
    ):
        if token not in behavior_factory:
            errors.append(f"BehaviorDetectionEventFactory.java missing {token}")

    consumer = (root / "go/control-plane/internal/alert/consumer/kafka_consumer.go").read_text(encoding="utf-8")
    for token in (
        "validateDetectionBehavior", 'header.GetEventType() != "traffic.detection.behavior.v1"',
        "header.GetIdempotencyKey() != header.GetEventId()", "tuple.GetSrcIp()", "tuple.GetDstIp()",
        "GetEvidenceIds()", "stableAlertID(tenantID, eventID, fingerprint)",
    ):
        if token not in consumer:
            errors.append(f"alert consumer missing {token}")
    build_start = consumer.find("func (c *Consumer) buildAlert")
    build_end = consumer.find("return alert", build_start)
    build_source = consumer[build_start:build_end]
    if 'srcIP = ""' in build_source or "range []*pb.Evidence{}" in build_source:
        errors.append("alert projection still manufactures empty tuple or evidence placeholders")
    stable = (root / "go/control-plane/internal/alert/consumer/stable_id.go").read_text(encoding="utf-8")
    if "uuid.NewSHA1" not in stable or 'identity = "event:" + identity' not in stable:
        errors.append("alert_id fallback is not deterministic from source event identity")

    cargo = (root / "rust/probe-agent/Cargo.toml").read_text(encoding="utf-8")
    probe = (root / "rust/probe-agent/probe-agent/src/aggregator/eviction.rs").read_text(encoding="utf-8")
    if 'features = ["v4", "v5", "serde"]' not in cargo:
        errors.append("probe-agent UUID dependency must enable deterministic v5 identities")
    for token in (
        "deterministic_flow_uuid", "Uuid::new_v5", 'event_type: "traffic.flow.v1"',
        "aggregate_id: identity.flow_id.clone()", "idempotency_key: identity.idempotency_key",
        'producer: "probe-agent"',
    ):
        if token not in probe:
            errors.append(f"probe FlowEvent producer missing {token}")
    flow_start = probe.find("fn to_flow_event")
    flow_end = probe.find("FlowEvent {", flow_start)
    flow_body_end = probe.find("\n    }", flow_end)
    if "new_v4" in probe[flow_start:flow_body_end]:
        errors.append("probe FlowEvent producer still uses random v4 identity")

    runbook = root / "doc/07_alignment/runbooks/T-KAFKA-002-event-envelope-idempotency.md"
    if not runbook.is_file():
        errors.append("T-KAFKA-002 runbook is missing")
    else:
        source = runbook.read_text(encoding="utf-8")
        for token in ("production_applied=false", "PARTIAL_DETECTION_VERTICAL_SLICE", "fail closed", "tenant_id:community_id", "T+0/T+1/T+3/T+7"):
            if token not in source:
                errors.append(f"T-KAFKA-002 runbook missing {token}")

    return {
        "schema_version": 1,
        "contract_id": contract.get("contract_id"),
        "remediation_id": "T-KAFKA-002",
        "status": "PASS" if not errors else "FAIL",
        "coverage_status": contract.get("coverage_status"),
        "production_applied": contract.get("production_applied"),
        "header_field_count": len(HEADER_TAGS),
        "detection_partition_key": (contract.get("detection_vertical_slice") or {}).get("partition_key"),
        "errors": errors,
        "remaining_gates": contract.get("remaining_gates") or [],
    }


def main() -> int:
    result = verify()
    print(json.dumps(result, ensure_ascii=False, indent=2))
    return 0 if result["status"] == "PASS" else 1


if __name__ == "__main__":
    raise SystemExit(main())
