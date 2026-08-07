#!/usr/bin/env python3
"""Validate canonical IDs, work packages, inventories and feature contracts."""

from __future__ import annotations

import argparse
import json
import re
import sys
from collections import Counter
from pathlib import Path
from typing import Any

from inventory import build_inventory


ROOT = Path(__file__).resolve().parents[2]
REGISTRY_PATH = ROOT / "contracts/alignment/canonical-registry.json"
PACKAGES_PATH = ROOT / "contracts/alignment/work-packages.json"
CONTRACTS_DIR = ROOT / "contracts/alignment/features"


def _load(path: Path) -> Any:
    return json.loads(path.read_text(encoding="utf-8"))


def _required_contract_fields(contract: dict[str, Any]) -> list[str]:
    required = [
        "feature_id",
        "contract_version",
        "status",
        "compatibility",
        "ui",
        "api",
        "domain",
        "data",
        "permissions",
        "performance",
        "acceptance",
        "rollout",
    ]
    return [field for field in required if field not in contract]


def _validate_contract(contract: dict[str, Any], inventory: dict[str, Any]) -> list[str]:
    errors = _required_contract_fields(contract)
    if errors:
        return errors
    compatibility = contract.get("compatibility", {})
    ui = contract.get("ui", {})
    api = contract.get("api", {})
    permissions = contract.get("permissions", {})
    acceptance = contract.get("acceptance", {})
    rollout = contract.get("rollout", {})
    controls = ui.get("controls", [])
    action_ids = [control.get("action_id") for control in controls if isinstance(control, dict)]
    if len(action_ids) != len(set(action_ids)):
        errors.append("ui.controls action_id values must be unique")
    if len(ui.get("states", [])) < 4:
        errors.append("ui.states must define at least four states")
    if api.get("response_envelope") != "data-meta-error":
        errors.append("api.response_envelope must be data-meta-error")
    if api.get("async") and not api.get("idempotency_key"):
        errors.append("async api must define idempotency_key")
    if permissions.get("tenant_source") != "authenticated_identity":
        errors.append("permissions.tenant_source must be authenticated_identity")
    if len(permissions.get("negative_tests", [])) < 2:
        errors.append("permissions.negative_tests must contain at least two tests")
    if len(acceptance.get("tests", [])) < 4:
        errors.append("acceptance.tests must contain at least four gates")
    if not rollout.get("rollback_runbook"):
        errors.append("rollout.rollback_runbook is required")
    missing_routes = sorted(set(compatibility.get("preserved_routes", [])) - set(inventory["routes"]))
    if missing_routes:
        errors.append(f"preserved_routes absent from route inventory: {missing_routes}")
    missing_actions = sorted(set(compatibility.get("preserved_actions", [])) - set(inventory["actions"]))
    if missing_actions:
        errors.append(f"preserved_actions absent from action inventory: {missing_actions}")
    missing_operations = sorted(
        set(compatibility.get("preserved_api_operations", [])) - set(inventory["api_operations"])
    )
    if missing_operations:
        errors.append(f"preserved_api_operations absent from API inventory: {missing_operations}")
    missing_contract_scopes = sorted(
        set(permissions.get("required_scopes", [])) - set(inventory["go_scopes"]) - {"*"}
    )
    if missing_contract_scopes:
        errors.append(f"required_scopes absent from Go scope catalog: {missing_contract_scopes}")
    return errors


def validate(strict_w1: bool = False) -> dict[str, Any]:
    registry = _load(REGISTRY_PATH)
    package_document = _load(PACKAGES_PATH)
    items = registry["items"]
    packages = {package["id"]: package for package in package_document["packages"]}

    ids = [item["id"] for item in items]
    counts = Counter(item["priority"] for item in items)
    feature_count = sum(item_id.startswith("F-") for item_id in ids)
    technology_count = sum(item_id.startswith("T-") for item_id in ids)
    expected = registry["expected_counts"]

    invalid_ids = sorted(
        item_id
        for item_id in ids
        if not re.fullmatch(r"[FT]-[A-Z0-9-]+-\d{3}", item_id)
    )
    duplicate_ids = sorted(item_id for item_id, count in Counter(ids).items() if count > 1)
    orphan_ids = sorted(item["id"] for item in items if item["work_package"] not in packages)
    duplicate_package_ids = sorted(
        package_id
        for package_id, count in Counter(package["id"] for package in package_document["packages"]).items()
        if count > 1
    )
    p0_p1_without_acceptance_case = sorted(
        item["id"]
        for item in items
        if item["priority"] in {"P0", "P1"}
        and not packages.get(item["work_package"], {}).get("acceptance_case")
    )
    p0_p1_without_rollback = sorted(
        item["id"]
        for item in items
        if item["priority"] in {"P0", "P1"}
        and not packages.get(item["work_package"], {}).get("rollback_runbook")
    )

    standard_scope: list[str] = []
    backlog: list[str] = []
    for item in items:
        mode = packages[item["work_package"]]["scope_mode"]
        included = item["priority"] == "P0" or (
            item["priority"] == "P1" and mode == "p0_p1"
        )
        (standard_scope if included else backlog).append(item["id"])

    inventory = build_inventory()
    contract_errors: dict[str, list[str]] = {}
    contract_ids: list[str] = []
    if CONTRACTS_DIR.exists():
        for path in sorted(CONTRACTS_DIR.glob("*.json")):
            contract = _load(path)
            contract_id = str(contract.get("feature_id", ""))
            errors = _validate_contract(contract, inventory)
            if contract_id not in ids:
                errors.append("feature_id is not in canonical registry")
            if contract_id and path.stem != contract_id:
                errors.append("filename must match feature_id")
            if errors:
                contract_errors[path.name] = errors
            contract_ids.append(contract_id)

    structural_failures = {
        "invalid_ids": invalid_ids,
        "duplicate_ids": duplicate_ids,
        "orphan_ids": orphan_ids,
        "duplicate_accountable_ids": duplicate_package_ids,
        "p0_p1_without_acceptance_case": p0_p1_without_acceptance_case,
        "p0_p1_without_rollback": p0_p1_without_rollback,
        "contract_errors": contract_errors,
    }
    count_failures = {
        "features": feature_count != expected["features"],
        "technologies": technology_count != expected["technologies"],
        "total": len(items) != expected["total"],
        "P0": counts["P0"] != expected["P0"],
        "P1": counts["P1"] != expected["P1"],
        "P2": counts["P2"] != expected["P2"],
        "formal_nav_routes": inventory["counts"]["formal_nav_routes"] != 24,
    }
    blockers = sum(bool(value) for value in structural_failures.values()) + sum(count_failures.values())

    result = {
        "schema_version": 1,
        "result": "pass" if blockers == 0 else "blocked",
        "blockers": blockers,
        "counts": {
            "features": feature_count,
            "technologies": technology_count,
            "total": len(items),
            "priorities": dict(sorted(counts.items())),
            "work_packages": len(packages),
            "standard_24w_scope": len(standard_scope),
            "backlog": len(backlog),
            "feature_contracts": len(contract_ids),
            "formal_nav_routes": inventory["counts"]["formal_nav_routes"],
        },
        "standard_24w_scope": sorted(standard_scope),
        "backlog": sorted(backlog),
        "structural_failures": structural_failures,
        "count_failures": count_failures,
        "w1_scope_gaps": {
            "unknown_required_ui_scopes": inventory["unknown_required_ui_scopes"],
            "unknown_accepted_ui_scopes": inventory["unknown_accepted_ui_scopes"],
            "missing_feature_contracts_for_p0": sorted(
                item["id"]
                for item in items
                if item["priority"] == "P0"
                and item["id"].startswith("F-")
                and item["id"] not in contract_ids
            ),
        },
    }
    if strict_w1 and any(result["w1_scope_gaps"].values()):
        result["result"] = "blocked"
        result["blockers"] += sum(bool(value) for value in result["w1_scope_gaps"].values())
    return result


def main() -> int:
    parser = argparse.ArgumentParser()
    parser.add_argument("--output", type=Path)
    parser.add_argument(
        "--strict-w1",
        action="store_true",
        help="Treat W1 scope gaps as blockers in addition to W0 structure.",
    )
    args = parser.parse_args()
    result = validate(strict_w1=args.strict_w1)
    payload = json.dumps(result, ensure_ascii=False, indent=2) + "\n"
    if args.output:
        args.output.parent.mkdir(parents=True, exist_ok=True)
        args.output.write_text(payload, encoding="utf-8")
    else:
        print(payload, end="")
    return 0 if result["result"] == "pass" else 1


if __name__ == "__main__":
    sys.path.insert(0, str(Path(__file__).resolve().parent))
    raise SystemExit(main())
