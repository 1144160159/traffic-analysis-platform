#!/usr/bin/env python3
"""Verify the WP-01 common snapshot/error protocol and adapter risk ratchet."""

from __future__ import annotations

import json
from pathlib import Path
from typing import Any

from build_adapter_risk_registry import build_registry as build_adapter_registry
from generate_ts_client import render as render_ts_client
from inventory import build_inventory
from validate import _validate_contract


ROOT = Path(__file__).resolve().parents[2]
PROTOCOL = ROOT / "contracts/alignment/common-response-protocol.v1.json"
ADAPTER_REGISTRY = ROOT / "contracts/alignment/adapter-risk-registry.v1.json"
OPENAPI = ROOT / "contracts/openapi/alignment-v1.openapi.json"
GO_RESPONSE = ROOT / "go/control-plane/internal/common/httpx/response.go"
TS_CLIENT = ROOT / "web/ui/src/generated/alignmentClient.ts"
ADAPTER_SOURCE = ROOT / "web/ui/src/services/pageSnapshotAdapters.ts"
FEATURE_IDS = ("F-COMMON-002", "F-COMMON-004", "F-ADAPTER-002")
SELECTED_PROHIBITED_RULES = {
    "derived_business_collection",
    "derived_summary_fallback",
    "default_demo_resource_id",
    "fabricated_comparison_literal",
    "fabricated_generic_snapshot",
    "fixed_detail_business_fallback",
    "fixed_detail_collection_fallback",
    "fixed_detail_metric_literal",
    "fixed_detail_timeline_literal",
    "fixed_numeric_business_fallback",
    "fixed_page_metric_fallback",
    "fixed_screen_visual_fixture",
    "generated_business_trend",
    "hardcoded_private_ip_literal",
    "runtime_business_fixture_object",
    "runtime_mock_dependency",
    "runtime_visual_fixture_bypass",
    "truthy_numeric_fallback",
}


def _load(path: Path) -> Any:
    return json.loads(path.read_text(encoding="utf-8"))


def validate(root: Path = ROOT) -> dict[str, Any]:
    root = root.resolve()
    errors: list[str] = []
    protocol = _load(root / PROTOCOL.relative_to(ROOT))
    adapter_registry = _load(root / ADAPTER_REGISTRY.relative_to(ROOT))
    openapi = _load(root / OPENAPI.relative_to(ROOT))
    inventory = build_inventory(root)

    contract_errors: dict[str, list[str]] = {}
    for feature_id in FEATURE_IDS:
        path = root / "contracts/alignment/features" / f"{feature_id}.json"
        if not path.is_file():
            contract_errors[feature_id] = ["formal contract missing"]
            continue
        issues = _validate_contract(_load(path), inventory)
        if issues:
            contract_errors[feature_id] = issues
    if contract_errors:
        errors.append(f"feature contract validation failed: {contract_errors}")

    meta = openapi.get("components", {}).get("schemas", {}).get("Meta", {})
    error_schema = openapi.get("components", {}).get("schemas", {}).get("Error", {})
    expected_meta = set(protocol["snapshot_meta"]["required"])
    actual_meta = set(meta.get("required", []))
    if actual_meta != expected_meta:
        errors.append(f"OpenAPI Meta differs from protocol: {sorted(actual_meta ^ expected_meta)}")
    expected_error = set(protocol["error"]["required"])
    actual_error = set(error_schema.get("required", []))
    if actual_error != expected_error:
        errors.append(f"OpenAPI Error differs from protocol: {sorted(actual_error ^ expected_error)}")
    for field in protocol["snapshot_meta"]["required"] + protocol["snapshot_meta"]["optional"]:
        if field not in meta.get("properties", {}):
            errors.append(f"OpenAPI Meta property missing: {field}")
    for field in protocol["error"]["required"] + protocol["error"]["optional"]:
        if field not in error_schema.get("properties", {}):
            errors.append(f"OpenAPI Error property missing: {field}")

    openapi_operations = {
        operation.get("operationId"): operation
        for path_item in (openapi.get("paths") or {}).values()
        if isinstance(path_item, dict)
        for method, operation in path_item.items()
        if method in {"get", "post", "put", "patch", "delete"} and isinstance(operation, dict)
    }
    pilot_operations = protocol.get("pilot_operations") or []
    pilot_operation_ids = [item.get("operation_id") for item in pilot_operations]
    if len(pilot_operation_ids) != len(set(pilot_operation_ids)):
        errors.append("common response pilot operation IDs must be unique")
    for item in pilot_operations:
        if item.get("adoption") != "IMPLEMENTED_G1":
            continue
        operation = openapi_operations.get(item.get("operation_id"))
        if operation is None:
            errors.append(f"implemented G1 pilot operation is absent from OpenAPI: {item.get('operation_id')}")
        elif operation.get("x-feature-id") != item.get("feature_id"):
            errors.append(f"implemented G1 pilot feature ownership differs: {item.get('operation_id')}")

    codes = protocol["error"]["catalog"]
    code_ids = [item["code"] for item in codes]
    if len(code_ids) != len(set(code_ids)):
        errors.append("error catalog codes must be unique")
    for item in codes:
        status = item.get("http_status")
        if not isinstance(status, int) or status < 400 or status > 599:
            errors.append(f"invalid HTTP error status for {item.get('code')}: {status}")
        expected_retryable = status in {429, 502, 503, 504}
        if item.get("code") == "INTERNAL_ERROR":
            expected_retryable = False
        if bool(item.get("retryable")) != expected_retryable:
            errors.append(f"retryability mismatch for {item.get('code')}")

    go_source = (root / GO_RESPONSE.relative_to(ROOT)).read_text(encoding="utf-8")
    for token in (
        "SchemaVersion",
        "GeneratedAt",
        "ResultCode",
        "FieldErrors",
        "OperationID",
        "CurrentRevision",
        "ProjectionStatus",
        "JSONContractError",
        "defaultRetryableStatus",
    ):
        if token not in go_source:
            errors.append(f"Go common response implementation missing {token}")

    generated = (root / TS_CLIENT.relative_to(ROOT)).read_text(encoding="utf-8")
    if generated != render_ts_client():
        errors.append("generated TypeScript alignment client is stale")
    for token in ("AlignmentMeta", "AlignmentError", "AlignmentEnvelope"):
        if f"export type {token}" not in generated:
            errors.append(f"generated TypeScript type missing {token}")

    rebuilt_adapter = build_adapter_registry(root)
    if adapter_registry != rebuilt_adapter:
        errors.append("adapter risk registry is stale")
    finding_ids = [item.get("finding_id") for item in adapter_registry.get("findings", [])]
    if len(finding_ids) != len(set(finding_ids)):
        errors.append("adapter finding IDs must be unique")
    for item in adapter_registry.get("findings", []):
        if not all(item.get(field) for field in ("rule_id", "classification", "status", "owner", "path", "excerpt_sha256")):
            errors.append(f"adapter finding lacks disposition metadata: {item.get('finding_id')}")

    page_findings = [
        item
        for item in adapter_registry.get("findings", [])
        if item.get("path") == "web/ui/src/services/pageSnapshotAdapters.ts"
    ]
    prohibited_page_rules = {
        "hardcoded_private_ip_literal",
        "fabricated_comparison_literal",
        "derived_business_collection",
        "runtime_mock_dependency",
    }
    remaining_prohibited = [
        item["finding_id"] for item in page_findings if item.get("rule_id") in prohibited_page_rules
    ]
    if remaining_prohibited:
        errors.append(f"prohibited adapter findings remain in pageSnapshotAdapters: {remaining_prohibited}")

    selected_prohibited = [
        item["finding_id"]
        for item in adapter_registry.get("findings", [])
        if item.get("rule_id") in SELECTED_PROHIBITED_RULES
    ]
    if selected_prohibited:
        errors.append(f"prohibited selected-source adapter findings remain: {selected_prohibited}")

    source = (root / ADAPTER_SOURCE.relative_to(ROOT)).read_text(encoding="utf-8")
    if "const sourceRows = events.length ? events : users.length ? users : protocols;" in source:
        errors.append("topic tunnel still derives business rows from display collections")
    if "'10.20.4.18'" in source or '"10.20.4.18"' in source:
        errors.append("page snapshot adapter still embeds the registered demo center IP")

    api_source = (root / "web/ui/src/services/api.ts").read_text(encoding="utf-8")
    for prohibited in (
        "buildVisualBreakdownSnapshot",
        "rows.length + index * 3",
        "pageId.toUpperCase()",
    ):
        if prohibited in api_source:
            errors.append(f"generic page API still contains prohibited fallback token: {prohibited}")

    main_source = (root / "web/ui/src/main.tsx").read_text(encoding="utf-8")
    if "if (import.meta.env.DEV && appConfig.useMock)" not in main_source:
        errors.append("Web UI mock dynamic import is not statically guarded by Vite DEV mode")
    runtime_source = (root / "web/ui/src/config/runtime.ts").read_text(encoding="utf-8")
    if "useMock: import.meta.env.DEV &&" not in runtime_source:
        errors.append("Web UI runtime config can enable mock mode outside Vite DEV mode")
    vite_source = (root / "web/ui/vite.config.ts").read_text(encoding="utf-8")
    for token in (
        "production-mock-worker-guard",
        "dist/mockServiceWorker.js",
        "/node_modules/msw/",
        "manifest: true",
    ):
        if token not in vite_source:
            errors.append(f"Web UI production bundle guard missing {token}")

    open_findings = int(adapter_registry.get("coverage", {}).get("open_findings", 0))
    adoption_gaps = list(protocol.get("adoption_gaps", []))
    return {
        "status": "PASS" if not errors else "FAIL",
        "features": list(FEATURE_IDS),
        "repository_integrity": "PASS" if not errors else "FAIL",
        "remediation_coverage": "PARTIAL" if open_findings or adoption_gaps else "COMPLETE",
        "formal_contracts": len(FEATURE_IDS) - len(contract_errors),
        "snapshot_required_fields": sorted(expected_meta),
        "error_catalog_codes": len(codes),
        "adapter_open_findings": open_findings,
        "adapter_counts_by_rule": adapter_registry.get("coverage", {}).get("counts_by_rule", {}),
        "adoption_gaps": adoption_gaps,
        "errors": errors,
    }


def main() -> int:
    result = validate()
    print(json.dumps(result, ensure_ascii=False, indent=2))
    return 0 if result["status"] == "PASS" else 1


if __name__ == "__main__":
    raise SystemExit(main())
