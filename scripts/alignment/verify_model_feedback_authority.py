#!/usr/bin/env python3
"""Verify the default-off T1-M09-N017 model feedback authority candidate."""

from __future__ import annotations

import hashlib
import json
from pathlib import Path
from typing import Any


ROOT = Path(__file__).resolve().parents[2]
CONTRACT = Path("contracts/alignment/model-feedback-authority.v1.json")
EVENT_SCHEMA = Path("contracts/events/model-feedback-event.v1.schema.json")
TOPICS = Path("contracts/events/kafka-topic-catalog.v1.json")
ACLS = Path("contracts/events/kafka-acl-catalog.v1.json")
OPENAPI = Path("contracts/openapi/alignment-v1.openapi.json")
FEATURE = Path("contracts/alignment/features/F-ALERT-001.json")
AUTHORITY = Path("go/control-plane/internal/alert/api/handler_model_feedback_revision.go")
READINESS = Path("go/control-plane/internal/alert/api/model_feedback_readiness.go")
OUTBOX = Path("go/control-plane/internal/alert/api/handler_feedback_outbox.go")
HANDLER = Path("go/control-plane/internal/alert/api/handler_feedback.go")
UNIT_TEST = Path("go/control-plane/internal/alert/api/handler_model_feedback_revision_test.go")
K8S_TEST = Path("go/control-plane/internal/alert/api/model_feedback_revision_k8s_integration_test.go")
MAIN = Path("go/control-plane/cmd/alert-service/main.go")
DEPLOYMENT = Path("deployments/kubernetes/applications/go-services.yaml")
WEB_CLIENT = Path("web/ui/src/services/alertDetailApi.ts")
WEB_PAGE = Path("web/ui/src/pages/AlertDetailPage.tsx")
RUNBOOK = Path("doc/07_alignment/runbooks/T1-M09-N017-model-feedback-authority.md")
EVIDENCE = Path("doc/02_acceptance/topic1/tasks/t1-m09-n017/k8s-model-feedback-latest.json")


def load_json(root: Path, path: Path) -> dict[str, Any]:
    return json.loads((root / path).read_text(encoding="utf-8"))


def verify(root: Path = ROOT) -> dict[str, Any]:
    errors: list[str] = []
    required = (
        CONTRACT, EVENT_SCHEMA, TOPICS, ACLS, OPENAPI, FEATURE, AUTHORITY,
        READINESS, OUTBOX, HANDLER, UNIT_TEST, K8S_TEST, MAIN, DEPLOYMENT,
        WEB_CLIENT, WEB_PAGE, RUNBOOK, EVIDENCE,
    )
    missing = [str(path) for path in required if not (root / path).is_file()]
    if missing:
        return {"status": "FAIL", "errors": [f"missing required files: {missing}"]}

    contract = load_json(root, CONTRACT)
    if contract.get("task_id") != "T1-M09-N017" or set(contract.get("feature_ids", [])) != {
        "F-ALERT-001", "F-MODEL-001", "F-MLOPS-001",
    }:
        errors.append("authority contract must bind N017 to alert, model and MLOps features")
    event_contract = contract.get("event_contract", {})
    schema_digest = hashlib.sha256((root / EVENT_SCHEMA).read_bytes()).hexdigest()
    if event_contract.get("topic") != "model.feedback.v1" or event_contract.get("schema_sha256") != schema_digest:
        errors.append("model.feedback.v1 schema hash binding drifted")
    if event_contract.get("new_event_or_api_versions") != 0:
        errors.append("N017 must add no event or API version")
    authority_contract = contract.get("authority", {})
    if authority_contract.get("migration") != "none_reuses_alert_feedback_and_alert_feedback_outbox":
        errors.append("N017 must not claim a new migration")
    aggregate = authority_contract.get("aggregate", {})
    if aggregate.get("compare_and_set") != "expected_label_revision_equals_current_head" or aggregate.get("terminal_state") != "RETRACTED":
        errors.append("label revision compare-and-set or terminal retraction contract drifted")
    runtime = contract.get("runtime", {})
    if runtime.get("authority_default_enabled") is not False or runtime.get("producer_default_enabled") is not False:
        errors.append("model feedback authority and producer must remain default-off")
    if runtime.get("consumer_candidate_sha256") != "0" * 64:
        errors.append("current candidate must remain zero until a real consumer receipt exists")
    if runtime.get("producer_startup_gate") != [
        "non_zero_exact_candidate_sha256", "exact_contract_sha256",
        "READY_consumer_receipt", "matching_ACCEPTED_broker_projection_receipt",
    ]:
        errors.append("consumer-first producer startup gate drifted")
    closure = contract.get("closure", {})
    if closure.get("status") != "PARTIAL" or closure.get("production_applied") is not False or not closure.get("open_items"):
        errors.append("N017 must remain PARTIAL and non-production with explicit blockers")

    schema = load_json(root, EVENT_SCHEMA).get("$defs", {}).get("ModelFeedbackAdjudicatedV1Json", {})
    required_fields = set(schema.get("required", []))
    for field in (
        "event_id", "feedback_id", "tenant_id", "prediction_id", "alert_id", "label",
        "label_revision", "adjudication_state", "model_version", "rule_version",
        "adjudicated_by", "occurred_at_ms", "trace_id",
    ):
        if field not in required_fields:
            errors.append(f"model feedback schema required field missing: {field}")

    topics = load_json(root, TOPICS).get("topics", [])
    topic = next((item for item in topics if item.get("name") == "model.feedback.v1"), {})
    if topic.get("readiness") != "producer_candidate_default_off":
        errors.append("model.feedback.v1 must be a default-off producer candidate")
    if topic.get("producers") != ["go/control-plane/internal/alert/api/handler_model_feedback_revision.go"]:
        errors.append("model feedback producer owner drifted")
    if topic.get("key_contract") != "tenant_id+feedback_id":
        errors.append("model feedback partition key contract drifted")
    acls = load_json(root, ACLS).get("topic_bindings", [])
    acl = next((item for item in acls if item.get("topic") == "model.feedback.v1"), {})
    if acl.get("producers") != ["alert-service"] or acl.get("consumers") != [
        {"principal": "rule-manager", "groups": ["rule-manager-model-feedback-revision-v1"]}
    ]:
        errors.append("model feedback ACL ownership drifted")

    openapi = load_json(root, OPENAPI)
    feedback_path = openapi.get("paths", {}).get("/v1/alerts/{id}/feedback", {})
    if feedback_path.get("post", {}).get("operationId") != "adjudicateAlertFeedback" or "409" not in feedback_path.get("post", {}).get("responses", {}):
        errors.append("versioned feedback OpenAPI operation or conflict response is missing")
    request_properties = feedback_path.get("post", {}).get("requestBody", {}).get("content", {}).get("application/json", {}).get("schema", {}).get("properties", {})
    if "expected_label_revision" not in request_properties or "adjudication_state" not in request_properties:
        errors.append("feedback OpenAPI additive revision controls are missing")

    authority_source = (root / AUTHORITY).read_text(encoding="utf-8")
    for token in (
        "modelFeedbackAggregateIdentity", "modelFeedbackRevisionEventIdentity",
        "sha256.Sum256([]byte(event.TenantID", "pg_advisory_xact_lock",
        "command.ExpectedLabelRevision != head.LabelRevision", 'head.AdjudicationState == "RETRACTED"',
        "event.PreviousEventID = head.EventID", 'Action: "MODEL_FEEDBACK_ADJUDICATED"',
        "existingComment != command.Comment", "existingAddToWhitelist != command.AddToWhitelist",
        "validateModelFeedbackAuthorityEvent", "INSERT INTO alert_feedback_outbox",
    ):
        if token not in authority_source:
            errors.append(f"model feedback authority guard missing: {token}")
    readiness_source = (root / READINESS).read_text(encoding="utf-8")
    for token in (
        "model_feedback_consumer_readiness_receipt", "JOIN model_feedback_revision_receipt",
        "e.event_id=r.event_id", "e.outcome='ACCEPTED'", 'strings.Repeat("0", 64)',
    ):
        if token not in readiness_source:
            errors.append(f"producer readiness guard missing: {token}")
    outbox_source = (root / OUTBOX).read_text(encoding="utf-8")
    if "payload->>'event_type'=$3" not in outbox_source or "envelope.LabelRevision != envelope.AggregateVersion" not in outbox_source:
        errors.append("typed outbox ownership or revision headers are missing")
    handler_source = (root / HANDLER).read_text(encoding="utf-8")
    if "h.revisionAuthorityEnabled" not in handler_source or "submitModelFeedbackRevision" not in handler_source:
        errors.append("feedback handler default-compatible authority branch is missing")

    main = (root / MAIN).read_text(encoding="utf-8")
    deployment = (root / DEPLOYMENT).read_text(encoding="utf-8")
    for token in (
        'getBoolEnv("MODEL_FEEDBACK_REVISION_AUTHORITY_V1_ENABLED", false)',
        'getBoolEnv("MODEL_FEEDBACK_REVISION_PRODUCER_V1_ENABLED", false)',
        "VerifyModelFeedbackProducerReadiness", "StartModelFeedbackRevisionOutboxWorker",
    ):
        if token not in main:
            errors.append(f"application startup guard missing: {token}")
    for token in (
        'MODEL_FEEDBACK_REVISION_AUTHORITY_V1_ENABLED, value: "false"',
        'MODEL_FEEDBACK_REVISION_PRODUCER_V1_ENABLED, value: "false"',
        'MODEL_FEEDBACK_CONSUMER_CANDIDATE_SHA256, value: "0000000000000000000000000000000000000000000000000000000000000000"',
    ):
        if token not in deployment:
            errors.append(f"Kubernetes default-off guard missing: {token}")

    tests = (root / UNIT_TEST).read_text(encoding="utf-8") + (root / K8S_TEST).read_text(encoding="utf-8")
    for token in (
        "FirstRevisionCommitsAuthorityAuditAndOutbox", "NextRevisionLinksPreviousEvent",
        "RejectsStaleRevisionAndRetractedHead", "IdempotentReplayDoesNotWriteAgain",
        "IdempotencyRejectsChangedCommand", "OutboxClaimIsEventTypeScoped",
        "IdentitiesAreTenantBound", "zero candidate authorized producer",
        "producer_published", "consumer_schema_present", "CleanupOracle",
    ):
        if token not in tests:
            errors.append(f"model feedback test coverage missing: {token}")

    web_client = (root / WEB_CLIENT).read_text(encoding="utf-8")
    web_page = (root / WEB_PAGE).read_text(encoding="utf-8")
    for token in ("expected_label_revision", "adjudication_state", "labelRevision", "previousEventId"):
        if token not in web_client:
            errors.append(f"typed feedback client field missing: {token}")
    if "当前仲裁版本" not in web_page or "expectedLabelRevision: snapshot.feedback.labelRevision" not in web_page:
        errors.append("Web feedback revision display or compare-and-set command is missing")

    evidence = load_json(root, EVIDENCE)
    if evidence.get("task_id") != "T1-M09-N017" or evidence.get("status") != "PASS" or evidence.get("production_applied") is not False:
        errors.append("Kubernetes feedback evidence must be a task-bound non-production PASS")
    for field in (
        "postgres_feedback_audit_outbox_atomic", "idempotent_replay_single_revision",
        "stale_and_terminal_conflicts_rejected", "tenant_bound_prediction_aggregate",
        "revision_chain_traceable", "producer_outbox_rows_remain_unpublished",
        "zero_candidate_rejected", "run_scoped_postgres_rows_removed",
        "readiness_gate_failed_closed_on_missing_schema",
    ):
        if evidence.get(field) is not True:
            errors.append(f"Kubernetes feedback evidence missing: {field}")
    if evidence.get("real_kafka_consumer_receipt_observed") is not False or evidence.get("k8s_consumer_schema_present") is not False:
        errors.append("current K8s evidence must not invent consumer schema or a real Kafka receipt")
    for relative, expected in evidence.get("inputs", {}).get("source_sha256", {}).items():
        path = root / relative
        if not path.is_file() or hashlib.sha256(path.read_bytes()).hexdigest() != expected:
            errors.append(f"Kubernetes feedback source hash drifted: {relative}")
    validation = contract.get("kubernetes_validation", {})
    if validation.get("status") != "PASS" or validation.get("run_id") != evidence.get("run_id") or validation.get("consumer_schema_present") is not False:
        errors.append("authority contract K8s validation does not bind the current truthful run")

    return {
        "status": "PASS" if not errors else "FAIL",
        "task_id": contract.get("task_id"),
        "coverage_status": closure.get("status"),
        "production_applied": closure.get("production_applied"),
        "authority_default_enabled": runtime.get("authority_default_enabled"),
        "producer_default_enabled": runtime.get("producer_default_enabled"),
        "kubernetes_run_id": validation.get("run_id"),
        "k8s_consumer_schema_present": validation.get("consumer_schema_present"),
        "closure_blockers": closure.get("open_items", []),
        "errors": errors,
    }


def main() -> int:
    result = verify()
    print(json.dumps(result, ensure_ascii=False, indent=2))
    return 0 if result["status"] == "PASS" else 1


if __name__ == "__main__":
    raise SystemExit(main())
