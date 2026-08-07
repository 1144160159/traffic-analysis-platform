#!/usr/bin/env python3
"""Fail closed on F-COMMON-001 Feature Contract registry drift and false coverage."""

from __future__ import annotations

import hashlib
import json
from typing import Any

from build_feature_contract_registry import OUTPUT, ROOT, _canonical_sha256, build_registry


EXPECTED_FEATURES = 54
EXPECTED_STANDARD_SCOPE_FEATURES = 38
EXPECTED_BACKLOG_CONTRACT_GAPS = 16
EXPECTED_NON_DRAFT_OPENAPI_BINDING_GAPS = {"F-AUDIT-001"}
EXPECTED_PILOTS = {"asset_vertical", "topic_snapshot_and_actions", "alert_query_and_actions"}


def verify() -> dict[str, Any]:
    errors: list[str] = []
    if not OUTPUT.is_file():
        return {"status": "FAIL", "errors": [f"missing {OUTPUT.relative_to(ROOT)}"]}
    actual = json.loads(OUTPUT.read_text(encoding="utf-8"))
    expected = build_registry()
    if actual != expected:
        errors.append("Feature Contract registry is stale relative to canonical authorities")
    if actual.get("schema_version") != 1 or actual.get("feature_id") != "F-COMMON-001":
        errors.append("registry identity must be schema v1 and F-COMMON-001")
    if actual.get("status") != "implementing":
        errors.append("registry cannot claim closure while backlog and runtime-adoption gaps remain")
    if actual.get("production_runtime_dependency") is not False:
        errors.append("Feature Contract registry must remain a build and release gate, not a runtime dependency")

    content = dict(actual)
    catalog_sha256 = content.pop("catalog_sha256", None)
    if catalog_sha256 != _canonical_sha256(content):
        errors.append("catalog_sha256 does not match canonical registry content")
    for authority in actual.get("authorities") or []:
        path = ROOT / str(authority.get("path") or "")
        if not path.is_file():
            errors.append(f"authority missing: {authority.get('domain')}")
        elif authority.get("sha256") != hashlib.sha256(path.read_bytes()).hexdigest():
            errors.append(f"authority hash drift: {authority.get('domain')}")

    policy = actual.get("policy") or {}
    for guard in (
        "one_canonical_feature_id",
        "one_accountable_owner",
        "contract_is_build_and_release_gate_not_runtime_dependency",
        "frontend_generated_types_required",
        "openapi_is_rest_authority",
        "protobuf_is_grpc_and_event_authority",
        "migration_is_schema_authority",
    ):
        if policy.get(guard) is not True:
            errors.append(f"required contract policy guard missing: {guard}")
    if policy.get("compatibility_removal_allowed_in_current_program") is not False:
        errors.append("current program cannot silently permit compatibility removals")
    if policy.get("unknown_enum_or_field_is_silent_success") is not False:
        errors.append("unknown enums or fields cannot silently succeed")

    features = actual.get("features") or []
    feature_ids = [str(item.get("feature_id") or "") for item in features]
    if len(features) != EXPECTED_FEATURES or len(set(feature_ids)) != EXPECTED_FEATURES:
        errors.append("registry must contain exactly 54 unique canonical feature IDs")
    if any(not item.get("accountable") for item in features):
        errors.append("every feature must have exactly one accountable owner")
    if any(not item.get("acceptance_case") or not item.get("rollback_runbook_id") for item in features):
        errors.append("every feature must resolve an acceptance case and rollback runbook ID")

    formal = [item for item in features if item.get("formal_contract_present")]
    standard = [item for item in features if item.get("standard_24w_scope")]
    if len(standard) != EXPECTED_STANDARD_SCOPE_FEATURES:
        errors.append("standard-scope feature count drifted from 38")
    for entry in formal:
        contract = entry.get("formal_contract") or {}
        path = ROOT / str(contract.get("path") or "")
        if not path.is_file():
            errors.append(f"{entry.get('feature_id')}: formal contract path missing")
        elif contract.get("sha256") != hashlib.sha256(path.read_bytes()).hexdigest():
            errors.append(f"{entry.get('feature_id')}: formal contract hash drift")
        if contract.get("validation_errors"):
            errors.append(f"{entry.get('feature_id')}: formal contract validation errors are not empty")
        binding_status = contract.get("openapi_binding_status")
        if binding_status not in {"EXACT", "PROFILED", "MISSING", "MISMATCH"}:
            errors.append(f"{entry.get('feature_id')}: OpenAPI binding status is absent or invalid")
        if binding_status in {"MISSING", "MISMATCH"} and contract.get("status") != "draft":
            if "openapi_operation_binding_missing" not in entry.get("blocking_gaps", []):
                errors.append(f"{entry.get('feature_id')}: non-draft OpenAPI binding gap was hidden")
    missing = [item for item in features if not item.get("formal_contract_present")]
    for entry in missing:
        if "versioned_feature_contract_missing" not in entry.get("blocking_gaps", []):
            errors.append(f"{entry.get('feature_id')}: missing formal contract gap was hidden")
    if any(item.get("priority") == "P0" and not item.get("formal_contract_present") for item in features):
        errors.append("all P0 feature IDs must have formal contracts")

    integrity = actual.get("integrity") or {}
    if integrity.get("duplicate_contract_ids"):
        errors.append("duplicate formal contract IDs are forbidden")
    if integrity.get("unregistered_contract_ids"):
        errors.append("formal contracts outside the canonical registry are forbidden")
    if integrity.get("duplicate_operation_ids"):
        errors.append("formal contract operation_id values must be unique")
    if integrity.get("contract_validation_errors"):
        errors.append("formal contract validation errors are forbidden")

    pilots = actual.get("pilot_slices") or []
    if {item.get("pilot_id") for item in pilots} != EXPECTED_PILOTS:
        errors.append("asset topic and alert pilot inventory is incomplete")
    if any(item.get("all_formal_contracts_present") is not True for item in pilots):
        errors.append("every pilot slice must have formal contracts")

    coverage = actual.get("coverage") or {}
    expected_coverage = {
        "canonical_feature_ids": len(features),
        "formal_contracts": len(formal),
        "formal_contracts_valid": sum(
            not (entry.get("formal_contract") or {}).get("validation_errors") for entry in formal
        ),
        "formal_contracts_openapi_bound": sum(
            (entry.get("formal_contract") or {}).get("openapi_binding_status") in {"EXACT", "PROFILED"}
            for entry in formal
        ),
        "non_draft_openapi_binding_gaps": sorted(
            entry["feature_id"]
            for entry in formal
            if "openapi_operation_binding_missing" in entry.get("blocking_gaps", [])
        ),
        "standard_scope_features": len(standard),
        "standard_scope_formal_contracts": sum(item.get("formal_contract_present") is True for item in standard),
        "missing_standard_scope_contracts": sorted(
            item["feature_id"] for item in standard if not item.get("formal_contract_present")
        ),
        "missing_backlog_contracts": sorted(
            item["feature_id"]
            for item in features
            if not item.get("standard_24w_scope") and not item.get("formal_contract_present")
        ),
        "p0_features": sum(item.get("priority") == "P0" for item in features),
        "p0_features_with_formal_contract": sum(
            item.get("priority") == "P0" and item.get("formal_contract_present") is True
            for item in features
        ),
    }
    if coverage != expected_coverage:
        errors.append("Feature Contract coverage counts do not match registry content")
    if expected_coverage["missing_standard_scope_contracts"]:
        errors.append("all 38 standard-scope features must have formal contracts after W1 freeze")
    if set(expected_coverage["non_draft_openapi_binding_gaps"]) != EXPECTED_NON_DRAFT_OPENAPI_BINDING_GAPS:
        errors.append("non-draft OpenAPI binding gaps changed without explicit W1 adjudication")
    if (
        len(expected_coverage["missing_backlog_contracts"]) != EXPECTED_BACKLOG_CONTRACT_GAPS
        or len(coverage.get("missing_backlog_contracts") or []) != EXPECTED_BACKLOG_CONTRACT_GAPS
    ):
        errors.append("repository evidence cannot hide or invent backlog contract gaps")

    return {
        "status": "PASS" if not errors else "FAIL",
        "feature_id": "F-COMMON-001",
        "registry_integrity": "PASS" if not errors else "FAIL",
        "contract_coverage": "STANDARD_SCOPE_COMPLETE_BACKLOG_PARTIAL",
        "catalog_sha256": actual.get("catalog_sha256"),
        "coverage": coverage,
        "errors": errors,
    }


def main() -> int:
    result = verify()
    print(json.dumps(result, ensure_ascii=False, indent=2))
    return 0 if result["status"] == "PASS" else 1


if __name__ == "__main__":
    raise SystemExit(main())
