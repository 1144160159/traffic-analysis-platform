#!/usr/bin/env python3
"""Semantic verifier for the M06 four-source ingress receipt contract."""

from __future__ import annotations

import json
from pathlib import Path
from typing import Any

ROOT = Path(__file__).resolve().parents[2]
CONTRACT = ROOT / "contracts/data-quality/source-quality-receipt.v1.json"
SCHEMA = ROOT / "contracts/data-quality/source-quality-receipt.schema.json"
RAILS = ["flow", "asset", "device_log", "user_behavior"]
CATEGORIES = ["accepted", "rejected", "invalid", "late", "duplicate", "conflict", "missing"]


class ContractError(ValueError):
    pass


def require(value: bool, code: str) -> None:
    if not value:
        raise ContractError(code)


def validate_contract(contract: dict[str, Any], schema: dict[str, Any]) -> dict[str, Any]:
    required = set(schema.get("required") or [])
    require(schema.get("additionalProperties") is False, "SCHEMA_OPEN")
    require(set(contract) == required, "ROOT_FIELDS")
    require(contract.get("contract_id") == "source-quality-receipt-v1", "CONTRACT_ID")
    require(contract.get("accountable_tasks") == ["T1-M06-N010", "T1-M06-N011"], "TASK")
    require(contract.get("status") == "implemented_candidate_ready_not_production_applied", "STATUS")
    require(contract.get("rails") == RAILS, "RAILS")
    require(contract.get("categories") == CATEGORIES, "CATEGORIES")
    identity = contract.get("identity") or {}
    require(identity.get("source_tuple") == ["topic", "partition", "offset"], "SOURCE_TUPLE")
    require("tenant_id" in identity.get("receipt_id", ""), "TENANT_IDENTITY")
    require("fails closed" in identity.get("collision_policy", ""), "COLLISION")
    barrier = contract.get("offset_barrier") or {}
    require(barrier.get("order") == ["validate", "persist durable receipt", "commit source offset"], "OFFSET_ORDER")
    require("do not invoke" in barrier.get("receipt_failure", ""), "RECEIPT_FAILURE")
    persistence = contract.get("persistence") or {}
    require(persistence.get("transport") == "existing audit.logs AuditEventV1Json envelope", "TRANSPORT")
    require(persistence.get("authority") == "existing PostgreSQL audit_logs append-only materialization", "AUTHORITY")
    require(persistence.get("new_table") is False and persistence.get("new_topic") is False, "NEW_AUTHORITY")
    governance = contract.get("governance_boundary") or {}
    require(set(governance.get("excluded") or []) == {"repair", "replay", "approval", "baseline", "quality rule authority", "fusion"}, "GOVERNANCE_SCOPE")
    implementations = contract.get("implementations") or {}
    require(set(implementations) == {
        "go_receipt", "go_repository", "go_reconcile", "java_receipt",
        "flow_writer", "asset_writer", "device_log_writer", "user_behavior_writer",
    }, "IMPLEMENTATIONS")
    for relative in implementations.values():
        require((ROOT / relative).is_file(), f"MISSING:{relative}")
    reconciliation = contract.get("reconciliation") or {}
    require("BuildMissingReceipts" in reconciliation.get("missing_signal", ""), "MISSING_SIGNAL")
    require("ReconcileAllRails" in reconciliation.get("pass", ""), "ALL_RAIL_GATE")
    require(contract.get("rollout", {}).get("enabled") is False, "ROLLOUT_DEFAULT")
    return {"status": "PASS", "contract_id": contract["contract_id"], "rails": 4, "categories": 7}


def main() -> int:
    try:
        result = validate_contract(
            json.loads(CONTRACT.read_text(encoding="utf-8")),
            json.loads(SCHEMA.read_text(encoding="utf-8")),
        )
    except (ContractError, json.JSONDecodeError) as error:
        print(json.dumps({"status": "FAIL", "error": str(error)}, indent=2))
        return 1
    print(json.dumps(result, indent=2))
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
