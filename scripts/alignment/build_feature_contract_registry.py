#!/usr/bin/env python3
"""Build the F-COMMON-001 canonical Feature Contract registry."""

from __future__ import annotations

import argparse
import hashlib
import json
from pathlib import Path
from typing import Any

from inventory import build_inventory
from validate import _validate_contract


ROOT = Path(__file__).resolve().parents[2]
OUTPUT = ROOT / "contracts/alignment/feature-contract-registry.v1.json"
CANONICAL = ROOT / "contracts/alignment/canonical-registry.json"
PACKAGES = ROOT / "contracts/alignment/work-packages.json"
SCHEMA = ROOT / "contracts/alignment/feature-contract.schema.json"
CONTRACTS = ROOT / "contracts/alignment/features"
AUTHORITY_PATHS = {
    "canonical_id_registry": CANONICAL,
    "work_package_ownership": PACKAGES,
    "feature_contract_shape": SCHEMA,
    "route_action_api_scope_inventory": ROOT / "scripts/alignment/inventory.py",
    "feature_contract_validation": ROOT / "scripts/alignment/validate.py",
    "rest_contract_authority": ROOT / "contracts/openapi/alignment-v1.openapi.json",
    "frontend_route_authority": ROOT / "web/ui/src/routes/routeManifest.tsx",
    "frontend_operation_plan": ROOT / "web/ui/src/services/pageApiPlans.ts",
}
PILOT_FEATURES = {
    "asset_vertical": ["F-ASSET-001", "F-ASSET-002", "F-ASSET-003", "F-ASSET-004", "F-ASSET-005", "F-ASSET-006"],
    "topic_snapshot_and_actions": ["F-TOPIC-001", "F-TOPIC-002", "F-TOPIC-003", "F-TOPIC-004"],
    "alert_query_and_actions": ["F-ALERT-001", "F-ALERT-002", "F-ALERT-003", "F-ALERT-004", "F-ALERT-005", "F-ALERT-006"],
}


def _relative(path: Path) -> str:
    return path.relative_to(ROOT).as_posix()


def _sha256(value: bytes | str) -> str:
    if isinstance(value, str):
        value = value.encode("utf-8")
    return hashlib.sha256(value).hexdigest()


def _canonical_sha256(value: Any) -> str:
    return _sha256(json.dumps(value, ensure_ascii=False, sort_keys=True, separators=(",", ":")))


def _load(path: Path) -> Any:
    return json.loads(path.read_text(encoding="utf-8"))


def _standard_scope(priority: str, scope_mode: str) -> bool:
    return priority == "P0" or (priority == "P1" and scope_mode == "p0_p1")


def build_registry() -> dict[str, Any]:
    canonical = _load(CANONICAL)
    package_document = _load(PACKAGES)
    packages = {item["id"]: item for item in package_document["packages"]}
    inventory = build_inventory()
    openapi = _load(AUTHORITY_PATHS["rest_contract_authority"])
    openapi_by_route: dict[tuple[str, str], dict[str, Any]] = {}
    for api_path, path_item in (openapi.get("paths") or {}).items():
        if not isinstance(path_item, dict):
            continue
        for api_method, operation in path_item.items():
            if api_method.lower() not in {"get", "post", "put", "patch", "delete"} or not isinstance(operation, dict):
                continue
            openapi_by_route[(api_method.upper(), str(api_path))] = operation
    formal: dict[str, tuple[Path, dict[str, Any]]] = {}
    duplicate_contract_ids: list[str] = []
    for path in sorted(CONTRACTS.glob("*.json")):
        contract = _load(path)
        feature_id = str(contract.get("feature_id") or "")
        if feature_id in formal:
            duplicate_contract_ids.append(feature_id)
        formal[feature_id] = (path, contract)

    features: list[dict[str, Any]] = []
    for item in canonical["items"]:
        feature_id = str(item["id"])
        if not feature_id.startswith("F-"):
            continue
        package = packages[item["work_package"]]
        registered = formal.get(feature_id)
        entry: dict[str, Any] = {
            "feature_id": feature_id,
            "priority": item["priority"],
            "work_package": item["work_package"],
            "accountable": package["accountable"],
            "acceptance_case": package["acceptance_case"],
            "rollback_runbook_id": package["rollback_runbook"],
            "standard_24w_scope": _standard_scope(item["priority"], package["scope_mode"]),
            "formal_contract_present": registered is not None,
            "formal_contract": None,
            "blocking_gaps": [],
        }
        if registered is None:
            entry["blocking_gaps"] = [
                "versioned_feature_contract_missing",
                "ui_api_domain_data_permission_performance_acceptance_rollout_traceability_missing",
            ]
        else:
            path, contract = registered
            validation_errors = _validate_contract(contract, inventory)
            contract_api = contract.get("api") or {}
            api_method = str(contract_api.get("method") or "").upper()
            api_path = str(contract_api.get("path") or "")
            operation_id = str(contract_api.get("operation_id") or "")
            bound_operation = openapi_by_route.get((api_method, api_path))
            bound_operation_id = str((bound_operation or {}).get("operationId") or "")
            bound_feature_id = str((bound_operation or {}).get("x-feature-id") or "")
            profile_feature_ids = {
                str(value) for value in (bound_operation or {}).get("x-contract-profile-feature-ids") or []
            }
            if bound_operation is None:
                openapi_binding_status = "MISSING"
            elif operation_id == bound_operation_id and feature_id == bound_feature_id:
                openapi_binding_status = "EXACT"
            elif feature_id in profile_feature_ids:
                openapi_binding_status = "PROFILED"
            else:
                openapi_binding_status = "MISMATCH"
            entry["formal_contract"] = {
                "path": _relative(path),
                "sha256": _sha256(path.read_bytes()),
                "contract_version": contract.get("contract_version"),
                "status": contract.get("status"),
                "operation_id": contract_api.get("operation_id"),
                "api_method": contract_api.get("method"),
                "api_path": contract_api.get("path"),
                "openapi_binding_status": openapi_binding_status,
                "openapi_bound_operation_id": bound_operation_id or None,
                "openapi_bound_feature_id": bound_feature_id or None,
                "required_scopes": (contract.get("permissions") or {}).get("required_scopes") or [],
                "preserved_routes": (contract.get("compatibility") or {}).get("preserved_routes") or [],
                "preserved_actions": (contract.get("compatibility") or {}).get("preserved_actions") or [],
                "preserved_api_operations": (contract.get("compatibility") or {}).get("preserved_api_operations") or [],
                "validation_errors": validation_errors,
            }
            if validation_errors:
                entry["blocking_gaps"].append("formal_contract_validation_failed")
            if contract.get("status") != "draft" and openapi_binding_status not in {"EXACT", "PROFILED"}:
                entry["blocking_gaps"].append("openapi_operation_binding_missing")
        features.append(entry)

    feature_ids = {entry["feature_id"] for entry in features}
    unregistered_contract_ids = sorted(set(formal) - feature_ids)
    formal_entries = [entry for entry in features if entry["formal_contract_present"]]
    standard_entries = [entry for entry in features if entry["standard_24w_scope"]]
    missing_standard = sorted(
        entry["feature_id"] for entry in standard_entries if not entry["formal_contract_present"]
    )
    missing_backlog = sorted(
        entry["feature_id"]
        for entry in features
        if not entry["standard_24w_scope"] and not entry["formal_contract_present"]
    )
    operation_owners: dict[str, list[str]] = {}
    for entry in formal_entries:
        operation_id = str((entry["formal_contract"] or {}).get("operation_id") or "")
        if operation_id:
            operation_owners.setdefault(operation_id, []).append(entry["feature_id"])
    duplicate_operation_ids = {
        operation_id: owners
        for operation_id, owners in sorted(operation_owners.items())
        if len(owners) > 1
    }
    pilot_slices = []
    for pilot_id, members in PILOT_FEATURES.items():
        pilot_slices.append(
            {
                "pilot_id": pilot_id,
                "feature_ids": members,
                "all_formal_contracts_present": all(
                    next(entry for entry in features if entry["feature_id"] == member)["formal_contract_present"]
                    for member in members
                ),
            }
        )

    all_gaps = sorted({gap for entry in features for gap in entry["blocking_gaps"]})
    registry: dict[str, Any] = {
        "schema_version": 1,
        "feature_id": "F-COMMON-001",
        "status": "implementing",
        "production_runtime_dependency": False,
        "authorities": [
            {"domain": name, "path": _relative(path), "sha256": _sha256(path.read_bytes())}
            for name, path in sorted(AUTHORITY_PATHS.items())
        ],
        "policy": {
            "one_canonical_feature_id": True,
            "one_accountable_owner": True,
            "contract_is_build_and_release_gate_not_runtime_dependency": True,
            "frontend_generated_types_required": True,
            "openapi_is_rest_authority": True,
            "protobuf_is_grpc_and_event_authority": True,
            "migration_is_schema_authority": True,
            "compatibility_removal_allowed_in_current_program": False,
            "unknown_enum_or_field_is_silent_success": False,
        },
        "coverage": {
            "canonical_feature_ids": len(features),
            "formal_contracts": len(formal_entries),
            "formal_contracts_valid": sum(
                not (entry["formal_contract"] or {}).get("validation_errors") for entry in formal_entries
            ),
            "formal_contracts_openapi_bound": sum(
                (entry["formal_contract"] or {}).get("openapi_binding_status") in {"EXACT", "PROFILED"}
                for entry in formal_entries
            ),
            "non_draft_openapi_binding_gaps": sorted(
                entry["feature_id"]
                for entry in formal_entries
                if "openapi_operation_binding_missing" in entry["blocking_gaps"]
            ),
            "standard_scope_features": len(standard_entries),
            "standard_scope_formal_contracts": sum(entry["formal_contract_present"] for entry in standard_entries),
            "missing_standard_scope_contracts": missing_standard,
            "missing_backlog_contracts": missing_backlog,
            "p0_features": sum(entry["priority"] == "P0" for entry in features),
            "p0_features_with_formal_contract": sum(
                entry["priority"] == "P0" and entry["formal_contract_present"] for entry in features
            ),
        },
        "integrity": {
            "duplicate_contract_ids": sorted(duplicate_contract_ids),
            "unregistered_contract_ids": unregistered_contract_ids,
            "duplicate_operation_ids": duplicate_operation_ids,
            "contract_validation_errors": {
                entry["feature_id"]: (entry["formal_contract"] or {}).get("validation_errors")
                for entry in formal_entries
                if (entry["formal_contract"] or {}).get("validation_errors")
            },
        },
        "pilot_slices": pilot_slices,
        "features": features,
        "blocking_gaps": all_gaps,
        "acceptance": {
            "repository": [
                "all 54 feature IDs resolve to exactly one accountable work package",
                "all existing formal contracts hash-bind UI API domain data permission performance acceptance and rollout fields",
                "all 38 standard-scope feature contracts are present and the asset topic and alert pilots are traceable",
                "implemented REST operations bind their method path operation_id and feature ownership to the OpenAPI authority",
                "missing backlog contracts remain explicit and cannot be hidden",
            ],
            "remaining": [
                "author backlog contracts without deleting routes actions operations scopes or audit events",
                "implement the explicit audit batch and scope catalog OpenAPI binding gaps before claiming full runtime adoption",
                "implement the draft standard-scope API migration projection and object boundaries before claiming runtime adoption",
                "bind generated TypeScript and compatibility telemetry to every formal contract",
                "capture old-client usage and live contract-version adoption by candidate",
                "complete compatibility diff G2 through G7 and external G8 gates",
            ],
        },
    }
    registry["catalog_sha256"] = _canonical_sha256(registry)
    return registry


def main() -> int:
    parser = argparse.ArgumentParser()
    parser.add_argument("--check", action="store_true")
    args = parser.parse_args()
    registry = build_registry()
    rendered = json.dumps(registry, ensure_ascii=False, indent=2) + "\n"
    if args.check:
        current = OUTPUT.read_text(encoding="utf-8") if OUTPUT.is_file() else ""
        status = "PASS" if current == rendered else "FAIL"
        print(json.dumps({"status": status, "catalog_sha256": registry["catalog_sha256"], "coverage": registry["coverage"]}, ensure_ascii=False, indent=2))
        return 0 if status == "PASS" else 1
    OUTPUT.write_text(rendered, encoding="utf-8")
    print(json.dumps({"status": "PASS", "output": _relative(OUTPUT), "catalog_sha256": registry["catalog_sha256"], "coverage": registry["coverage"]}, ensure_ascii=False, indent=2))
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
