#!/usr/bin/env python3
"""Verify the default-off, partial T-OBS-001 trace/watermark/reconcile candidate."""

from __future__ import annotations

import json
import re
from pathlib import Path
from typing import Any

import yaml


ROOT = Path(__file__).resolve().parents[2]
CONTRACT = Path("contracts/observability/trace-watermark-reconcile.v1.json")
PROTO = Path("proto/traffic/v1/alert.proto")
GO_PROTO = Path("go/control-plane/pkg/proto/traffic/v1/alert.pb.go")
JAVA_PROTO = Path("java/flink-jobs/flink-common/src/main/java/com/traffic/proto/traffic/v1/Alert.java")
RUST_PROTO = Path("rust/probe-agent/proto-gen/src/traffic.v1.rs")
HTTP = Path("go/control-plane/internal/common/httpx/request_id.go")
OTEL = Path("go/control-plane/internal/common/otel/tracer.go")
KAFKA_PRODUCER = Path("go/control-plane/internal/common/kafka/producer.go")
KAFKA_CONSUMER = Path("go/control-plane/internal/common/kafka/consumer.go")
ALERT_MODEL = Path("go/control-plane/internal/alert/persistence/alert.go")
GO_CH_WRITER = Path("go/control-plane/internal/alert/persistence/clickhouse.go")
GO_DETECTION_CONSUMER = Path("go/control-plane/internal/alert/consumer/kafka_consumer.go")
GO_ALERT_CONSUMER = Path("go/control-plane/internal/alert/consumer/alert_consumer.go")
FLINK_BEHAVIOR = Path("java/flink-jobs/flink-alert-generator-job/src/main/java/com/traffic/flink/alert/generator/AlertGenerator.java")
FLINK_BUSINESS = Path("java/flink-jobs/flink-alert-generator-job/src/main/java/com/traffic/flink/alert/generator/BusinessAlertGenerator.java")
FLINK_CH = Path("java/flink-jobs/flink-alert-generator-job/src/main/java/com/traffic/flink/alert/sink/ClickHouseAlertSinkFactory.java")
FLINK_OS = Path("java/flink-jobs/flink-alert-generator-job/src/main/java/com/traffic/flink/alert/sink/OpenSearchAlertSinkFactory.java")
CH_MIGRATION = Path("deployments/clickhouse/migrations/202608041300_alert_trace_correlation_v1.sql")
CH_COMMON = Path("common/sql/ch/00-all-tables.sql")
CH_DOCKER = Path("go/control-plane/deployments/docker/init/clickhouse_merged.sql")
CH_K8S = Path("deployments/kubernetes/init-jobs/03-clickhouse-schema.yaml")
OS_MAPPING = Path("common/opensearch/alerts-v2/mappings-component.json")
OS_K8S = Path("deployments/kubernetes/migrations/opensearch/T-OS-002-alerts-v2-expand.yaml")
TOOL = Path("scripts/alignment/cross_store_reconcile.py")
ASSET_G1_RUNNER = Path("scripts/alignment/verify_asset_seven_source_ephemeral.py")
ASSET_G1_TEST = Path("go/control-plane/internal/asset/consumer/asset_seven_source_integration_test.go")
ASSET_G1_GUARD = Path("tests/alignment/test_asset_seven_source_ephemeral_guard.py")
CAPTURE = Path("scripts/alignment/capture_trace_watermark_reconcile.py")
RUNBOOK = Path("doc/07_alignment/runbooks/T-OBS-001-trace-watermark-reconcile.md")
TESTS = (
    Path("go/control-plane/internal/common/httpx/request_id_test.go"),
    Path("go/control-plane/internal/common/kafka/trace_context_test.go"),
    Path("go/control-plane/internal/alert/persistence/alert_trace_test.go"),
    Path("tests/alignment/test_cross_store_reconcile.py"),
    Path("java/flink-jobs/flink-alert-generator-job/src/test/java/com/traffic/flink/alert/generator/AlertGeneratorTest.java"),
    Path("java/flink-jobs/flink-alert-generator-job/src/test/java/com/traffic/flink/alert/sink/OpenSearchAlertSinkFactoryTest.java"),
)


def _load_json(root: Path, path: Path) -> dict[str, Any]:
    return json.loads((root / path).read_text(encoding="utf-8"))


def _require_tokens(errors: list[str], root: Path, path: Path, tokens: tuple[str, ...]) -> None:
    text = (root / path).read_text(encoding="utf-8")
    for token in tokens:
        if token not in text:
            errors.append(f"required trace/reconcile guard missing in {path}: {token}")


def verify(root: Path = ROOT) -> dict[str, Any]:
    errors: list[str] = []
    required = (
        CONTRACT, PROTO, GO_PROTO, JAVA_PROTO, RUST_PROTO, HTTP, OTEL,
        KAFKA_PRODUCER, KAFKA_CONSUMER, ALERT_MODEL, GO_CH_WRITER,
        GO_DETECTION_CONSUMER, GO_ALERT_CONSUMER, FLINK_BEHAVIOR,
        FLINK_BUSINESS, FLINK_CH, FLINK_OS, CH_MIGRATION, CH_COMMON,
        CH_DOCKER, CH_K8S, OS_MAPPING, OS_K8S, TOOL, ASSET_G1_RUNNER,
        ASSET_G1_TEST, ASSET_G1_GUARD, CAPTURE, RUNBOOK, *TESTS,
    )
    missing = [str(path) for path in required if not (root / path).is_file()]
    if missing:
        return {"status": "FAIL", "errors": [f"missing required files: {missing}"]}

    contract = _load_json(root, CONTRACT)
    if contract.get("remediation_id") != "T-OBS-001" or contract.get("work_package") != "WP-03-OBS-DQ":
        errors.append("contract must bind canonical T-OBS-001 to WP-03-OBS-DQ")
    if contract.get("status") in {"closed", "complete", "pass"}:
        errors.append("partial repository candidate must not claim T-OBS-001 closure")
    if contract.get("coverage_status") != "PARTIAL" or contract.get("production_applied") is not False:
        errors.append("candidate must remain PARTIAL with production_applied=false")
    if not contract.get("closure_blockers"):
        errors.append("live trace, performance, browser, rollout and observation blockers must remain explicit")

    trace = contract.get("trace_contract", {})
    if trace.get("format") != "w3c_trace_id_lowercase_32_hex":
        errors.append("trace contract must require lowercase 32-hex W3C IDs")
    if trace.get("http_precedence") != ["valid_traceparent", "valid_x_trace_id", "cryptographic_new_trace_id"]:
        errors.append("traceparent precedence or cryptographic fallback drifted")
    required_stages = {"http", "postgresql", "outbox", "kafka", "flink", "clickhouse", "opensearch", "nebulagraph", "minio_manifest", "audit"}
    if set(trace.get("required_stages", [])) != required_stages:
        errors.append("end-to-end trace stage inventory is incomplete")

    normalized = contract.get("normalized_reconcile_record", {})
    required_sources = {"postgresql", "kafka", "clickhouse", "opensearch", "nebulagraph", "minio", "audit"}
    if set(normalized.get("sources", [])) != required_sources:
        errors.append("normalized reconciliation source inventory must include audit and all six data transports/stores")
    if normalized.get("classifications") != ["missing", "extra", "stale_version", "hash_mismatch", "unparseable"]:
        errors.append("required reconciliation classifications drifted")
    runtime = contract.get("runtime_guards", {})
    if runtime.get("mode") != "plan_only" or runtime.get("automatic_repair") is not False:
        errors.append("repository reconcile candidate must remain plan-only")
    if runtime.get("automatic_delete_extra") is not False or runtime.get("extra_action") != "quarantine_review":
        errors.append("extra projections must never be automatically deleted")
    if runtime.get("maximum_records") != 10000 or runtime.get("wildcard_tenant_forbidden") is not True:
        errors.append("bounded concrete-tenant reconcile guard drifted")
    if runtime.get("production_mutations_in_repository_candidate") != []:
        errors.append("repository candidate must not claim production mutations")
    owned_g1 = contract.get("owned_g1_asset_reconciliation", {})
    if set(owned_g1.get("sources", [])) != required_sources:
        errors.append("owned G1 asset reconciliation must bind all seven sources")
    if owned_g1.get("production_applied") is not False or owned_g1.get("minio_path") != "explicitly_seeded_adapter_not_asset_event_consumer":
        errors.append("owned G1 evidence boundary must keep MinIO adapter and production application explicit")

    _require_tokens(errors, root, PROTO, ("string trace_id = 33;",))
    _require_tokens(errors, root, GO_PROTO, ('protobuf:"bytes,33,opt,name=trace_id', "func (x *Alert) GetTraceId() string"))
    _require_tokens(errors, root, JAVA_PROTO, ("string trace_id = 33", "public java.lang.String getTraceId()"))
    _require_tokens(errors, root, RUST_PROTO, ("pub trace_id: ::prost::alloc::string::String",))
    _require_tokens(errors, root, HTTP, (
        "propagation.TraceContext{}.Extract", "crypto/rand", "trace.ContextWithRemoteSpanContext",
        'r.Header.Set("traceparent"', "HeaderSpanID",
    ))
    _require_tokens(errors, root, OTEL, ("func init()", "propagation.TraceContext{}", "propagation.Baggage{}"))
    _require_tokens(errors, root, KAFKA_PRODUCER, (
        "buildKafkaHeaders", "declared trace_id conflicts with W3C context", '"traceparent"', '"tracestate"',
    ))
    _require_tokens(errors, root, KAFKA_CONSUMER, (
        "func (m *ReceivedMessage) Context", "commonotel.ExtractFromMap", "ContextWithRemoteSpanContext", "handler(messageContext, receivedMsg)",
    ))
    _require_tokens(errors, root, ALERT_MODEL, ('TraceID string `json:"trace_id" ch:"trace_id"`', "TraceId:          a.TraceID", "traceID = b.Header.GetTraceId()"))
    _require_tokens(errors, root, GO_CH_WRITER, ("evidence_ids, event_id, trace_id", "alert.TraceID"))
    _require_tokens(errors, root, GO_DETECTION_CONSUMER, ("traceID = header.GetTraceId()", "TraceID:     traceID"))
    _require_tokens(errors, root, GO_ALERT_CONSUMER, ("TraceID:      pbAlert.TraceId", "TraceId:      traceID"))
    for path in (FLINK_BEHAVIOR, FLINK_BUSINESS):
        _require_tokens(errors, root, path, (".setTraceId(header.getTraceId())",))
    _require_tokens(errors, root, FLINK_CH, ("event_id, trace_id", "alert.getTraceId()", "共 33 个字段"))
    _require_tokens(errors, root, FLINK_OS, ('doc.put("trace_id", alert.getTraceId())',))

    migration_text = (root / CH_MIGRATION).read_text(encoding="utf-8")
    executable_migration = re.sub(r"--[^\n]*", "", migration_text)
    if re.search(r"\bON\s+CLUSTER\b", executable_migration, re.IGNORECASE):
        errors.append("ClickHouse migration runner is node-local; ON CLUSTER is forbidden")
    for table in ("alerts_latest_local", "alerts_latest", "alerts_local", "alerts"):
        if f"ALTER TABLE traffic.{table}" not in migration_text or "ADD COLUMN IF NOT EXISTS trace_id String AFTER event_id" not in migration_text:
            errors.append(f"ClickHouse trace expand migration misses {table}")
    for path in (CH_COMMON, CH_DOCKER, CH_K8S):
        text = (root / path).read_text(encoding="utf-8")
        if "trace_id" not in text or "alerts_local" not in text:
            errors.append(f"ClickHouse schema entrypoint misses alert trace_id: {path}")

    mapping = _load_json(root, OS_MAPPING)
    properties = mapping.get("template", {}).get("mappings", {}).get("properties", {})
    if properties.get("trace_id", {}).get("type") != "keyword":
        errors.append("OpenSearch alerts-v2 trace_id must be keyword")
    manifests = list(yaml.safe_load_all((root / OS_K8S).read_text(encoding="utf-8")))
    configmaps = [item for item in manifests if item and item.get("kind") == "ConfigMap" and item.get("metadata", {}).get("name") == "opensearch-alerts-v2-contract"]
    if len(configmaps) != 1:
        errors.append("OpenSearch alert contract ConfigMap is missing or duplicated")
    else:
        try:
            embedded = json.loads(configmaps[0].get("data", {}).get("mappings-component.json", ""))
        except json.JSONDecodeError:
            embedded = {}
            errors.append("embedded OpenSearch mapping is not valid JSON")
        if embedded != mapping:
            errors.append("embedded OpenSearch trace mapping drifts from common source")

    _require_tokens(errors, root, TOOL, (
        "DEFAULT_MAX_RECORDS = 10_000", "FORBIDDEN_SCOPE_VALUES", '"missing"', '"extra"',
        '"stale_version"', '"hash_mismatch"', '"unparseable"', '"trace_mismatch"',
        '"automatic_execution": False', "quarantine_review_no_delete", "report_sha256", '"audit"',
    ))
    _require_tokens(errors, root, ASSET_G1_RUNNER, (
        "TestAssetSevenSourceTraceReconciliation", "cross_store_reconcile.py",
        '"production_applied": False', "MinIO is explicitly seeded",
    ))
    _require_tokens(errors, root, ASSET_G1_TEST, (
        "UpsertAtomic", "NewAssetOutboxDispatcher", "NewAssetProjectionEventConsumer",
        "NewAssetProjectionWorker", "NewClickHouseWriter", "PutObject",
        'sources := []string{"postgresql", "kafka", "clickhouse", "opensearch", "nebulagraph", "minio", "audit"}',
    ))
    _require_tokens(errors, root, CAPTURE, (
        "build_snapshot", '"production_mutations": []', '"seven_source_same_trace_manifest": None',
        '"G8": "BLOCKED"', "refusing to overwrite immutable evidence directory",
    ))
    tests_text = "\n".join((root / path).read_text(encoding="utf-8") for path in TESTS)
    for token in (
        "TraceparentWinsOverConflictingLegacyHeader", "RejectsSplitBrainTrace", "PropagatesFromDetectionHeaderToProtoProjection",
        "all_required_difference_classes_are_explicit", "wildcard_tenant_and_domain_are_rejected",
        "input_bound_is_fail_closed", "getTraceId()", '"trace_id"',
    ):
        if token not in tests_text:
            errors.append(f"required negative or propagation test is missing: {token}")

    return {
        "status": "PASS" if not errors else "FAIL",
        "remediation_id": contract.get("remediation_id"),
        "coverage_status": contract.get("coverage_status"),
        "production_applied": contract.get("production_applied"),
        "implemented_candidate_stages": trace.get("implemented_candidate_stages", []),
        "remaining_stages": trace.get("remaining_stages", []),
        "maximum_records": runtime.get("maximum_records"),
        "closure_blockers": contract.get("closure_blockers", []),
        "errors": errors,
    }


def main() -> int:
    result = verify()
    print(json.dumps(result, ensure_ascii=False, indent=2))
    return 0 if result["status"] == "PASS" else 1


if __name__ == "__main__":
    raise SystemExit(main())
