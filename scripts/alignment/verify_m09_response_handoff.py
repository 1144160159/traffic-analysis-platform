#!/usr/bin/env python3
"""Static and Kubernetes-evidence verifier for T1-M09-N020."""

from __future__ import annotations

import hashlib
import json
from pathlib import Path
from typing import Any


ROOT = Path(__file__).resolve().parents[2]
CONTRACT = Path("contracts/alignment/response-recommendation-handoff.v1.json")
EVIDENCE = Path("doc/02_acceptance/topic1/tasks/t1-m09-n020/k8s-response-handoff-latest.json")
TEXT_PATHS = (
    "go/control-plane/internal/alert/api/handler_alert_actions.go",
    "go/control-plane/internal/alert/api/handler_alert_response_workflow.go",
    "go/control-plane/internal/alert/consumer/alert_response_event_consumer.go",
    "go/control-plane/internal/alert/consumer/alert_response_http_executor.go",
    "go/control-plane/internal/alert/config/config.go",
    "go/control-plane/deployments/kubernetes/alert-service.yaml",
    "web/ui/src/services/alertDetailActionApi.ts",
    "web/ui/src/pages/AlertDetailPage.tsx",
    "contracts/openapi/alignment-v1.openapi.json",
)


def load_json(path: Path) -> dict[str, Any]:
    return json.loads((ROOT / path).read_text(encoding="utf-8"))


def load_texts() -> dict[str, str]:
    return {relative: (ROOT / relative).read_text(encoding="utf-8") for relative in TEXT_PATHS}


def validate_snapshot(texts: dict[str, str], contract: dict[str, Any], evidence: dict[str, Any]) -> list[str]:
    errors: list[str] = []
    if contract.get("task_id") != "T1-M09-N020" or contract.get("status") != "PARTIAL":
        errors.append("N020 contract identity/status must remain PARTIAL")
    rail = contract.get("runtime_rail", {})
    if rail.get("default_enabled") is not False:
        errors.append("response handoff runtime rail is not default-off")
    expected_flags = {
        "ALERT_RESPONSE_EXECUTION_V1_ENABLED",
        "ALERT_RESPONSE_EXTERNAL_EXECUTOR_V1_ENABLED",
        "ALERT_RESPONSE_UNKNOWN_EFFECT_RECONCILIATION_V1_ENABLED",
        "ALERT_RESPONSE_COMPENSATION_EXECUTOR_V1_ENABLED",
    }
    bound_flags = {value for value in rail.values() if isinstance(value, str)}
    if expected_flags - bound_flags:
        errors.append("response handoff contract does not enumerate every runtime flag")

    handler = texts["go/control-plane/internal/alert/api/handler_alert_actions.go"]
    for token in (
        "getAlertResponseExecutionReceipt", "receipt_available", "execution_receipt",
        "WHERE tenant_id=$1 AND alert_id=$2 AND job_id=$3", "effect_ids::text",
        "authority_lookup::text", "decode alert response receipt effect ids",
    ):
        if token not in handler:
            errors.append(f"provider receipt read API missing {token}")

    workflow = texts["go/control-plane/internal/alert/api/handler_alert_response_workflow.go"]
    for token in ("INDEPENDENT_APPROVER_REQUIRED", "expected revision", "approved_awaiting_executor"):
        if token not in workflow:
            errors.append(f"independent approval workflow missing {token}")

    projection = texts["go/control-plane/internal/alert/consumer/alert_response_event_consumer.go"]
    for token in (
        '"simulated_completed", "internal-simulation"', '"blocked_external_executor", "unconfigured"',
        'outcome.EffectState == "unknown"', "provider_receipt_id", "receipt_sha256",
        "INSERT INTO alert_response_execution_receipts", "ALERT_RESPONSE_EXECUTION_",
    ):
        if token not in projection:
            errors.append(f"response projection fail-closed/provider path missing {token}")

    executor = texts["go/control-plane/internal/alert/consumer/alert_response_http_executor.go"]
    for token in ("Idempotency-Key", "ProviderReceiptID", "EffectIDs", "validateAlertResponseExecutionReceipt"):
        if token not in executor:
            errors.append(f"external provider adapter missing {token}")

    config = texts["go/control-plane/internal/alert/config/config.go"]
    manifest = texts["go/control-plane/deployments/kubernetes/alert-service.yaml"]
    for flag in expected_flags:
        if flag not in config and flag not in manifest:
            errors.append(f"runtime flag is not wired: {flag}")
        at = manifest.find(flag)
        if at < 0 or "false" not in manifest[at:at + 120]:
            errors.append(f"runtime flag is not explicitly default-off in K8s: {flag}")

    client = texts["web/ui/src/services/alertDetailActionApi.ts"]
    page = texts["web/ui/src/pages/AlertDetailPage.tsx"]
    for token in ("AlertResponseExecutionReceipt", "fetchAlertResponseAction", "receiptAvailable", "providerReceiptId"):
        if token not in client:
            errors.append(f"typed response receipt client missing {token}")
    for token in ("alert-response-provider-receipt", "Provider 回执", "等待执行回执", "responseActionQuery"):
        if token not in page:
            errors.append(f"response receipt UI missing {token}")

    openapi = texts["contracts/openapi/alignment-v1.openapi.json"]
    for token in ("AlertResponseExecutionReceipt", "AlertResponseActionStatusEnvelope", '"receipt_available"', '"execution_receipt"'):
        if token not in openapi:
            errors.append(f"OpenAPI response receipt schema missing {token}")

    boundary = contract.get("topic_boundary", {})
    if set(boundary.get("not_owned", [])) != {
        "direct production traffic cleaning", "direct production blackhole routing",
        "direct production attack-source blocking", "provider-side network policy implementation",
    }:
        errors.append("topic-one direct-effect boundary drifted")

    latest = contract.get("latest_evidence", {})
    if evidence.get("task_id") != "T1-M09-N020" or evidence.get("status") != "PASS" or evidence.get("run_id") != latest.get("run_id"):
        errors.append("N020 Kubernetes evidence identity/status mismatch")
    for field in (
        "dry_run_receipt", "unconfigured_executor_fail_closed", "provider_receipt_persisted",
        "provider_receipt_read_api", "provider_receipt_visible_in_candidate_bundle",
        "run_scoped_resources_removed",
    ):
        if evidence.get(field) is not True:
            errors.append(f"N020 Kubernetes evidence missing {field}=true")
    for field in (
        "execution_flags_default_enabled", "topic_one_direct_cleaning_owned",
        "topic_one_direct_blackhole_routing_owned", "mock_enabled",
        "shared_postgres_touched", "production_applied",
    ):
        if evidence.get(field) is not False:
            errors.append(f"N020 Kubernetes evidence must keep {field}=false")
    if len(evidence.get("kubernetes_jobs", [])) != 4:
        errors.append("N020 Kubernetes evidence does not contain four successful jobs")
    oracle = evidence.get("postgres_oracle", {})
    for field in ("actions", "receipts", "simulated", "blocked", "completed", "confirmed_external", "provider_receipts", "audit"):
        if not isinstance(oracle.get(field), int) or oracle[field] < 1:
            errors.append(f"N020 PostgreSQL oracle lacks positive {field}")
    if not any("production traffic cleaning or blackhole routing" in item for item in evidence.get("does_not_prove", [])):
        errors.append("N020 evidence overclaims topic-one direct network effects")
    return errors


def main() -> int:
    contract, evidence, texts = load_json(CONTRACT), load_json(EVIDENCE), load_texts()
    errors = validate_snapshot(texts, contract, evidence)
    for relative, expected in evidence.get("inputs", {}).get("source_sha256", {}).items():
        path = ROOT / relative
        actual = hashlib.sha256(path.read_bytes()).hexdigest() if path.is_file() else "missing"
        if actual != expected:
            errors.append(f"Kubernetes evidence source hash drifted: {relative}")
    if errors:
        for error in errors:
            print(f"FAIL: {error}")
        return 1
    print("PASS: T1-M09-N020 response recommendation handoff and Kubernetes evidence are current")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
