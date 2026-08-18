#!/usr/bin/env python3
"""Fail-closed semantic verifier for the M06 entity-resolution contract."""

from __future__ import annotations

import json
from pathlib import Path
from typing import Any


ROOT = Path(__file__).resolve().parents[2]
CONTRACT_PATH = ROOT / "contracts/entity/entity-resolution.v1.json"
SCHEMA_PATH = ROOT / "contracts/entity/entity-resolution.schema.json"

ROOT_KEYS = {
    "schema_version",
    "contract_id",
    "status",
    "accountable_task",
    "rule_version",
    "identity_roles",
    "tenant_boundary",
    "source_rails",
    "identifier_rules",
    "decision_states",
    "determinism",
    "rollout",
    "claim_boundary",
}
EXPECTED_ROLES = [
    "subject",
    "source",
    "destination",
    "device",
    "sensor",
    "account",
    "correlation",
]
EXPECTED_STATES = ["accepted", "partial", "ambiguous", "conflict", "insufficient"]
EXPECTED_RAILS = {
    "flow": ({"ip", "probe_id", "community_id"}, {"probe"}),
    "asset_authority": ({"asset_id", "ip", "mac", "probe_id"}, {"asset"}),
    "asset_binding": ({"ip", "mac", "probe_id"}, {"probe"}),
    "device_log": ({"ip"}, set()),
    "user_behavior": ({"user_id", "ip"}, {"user"}),
    "probe_ingest": ({"ip", "mac", "probe_id"}, {"probe"}),
}
EXPECTED_RULES = {
    "ER-ASSET-ID-EXACT": ("asset_id", "asset", 1_000_000, True, None),
    "ER-USER-ID-EXACT": ("user_id", "user", 1_000_000, True, None),
    "ER-PROBE-ID-EXACT": ("probe_id", "probe", 1_000_000, True, None),
    "ER-MAC-ASSET-TEMPORAL": ("mac", "asset", 950_000, False, 2_592_000_000),
    "ER-IP-ASSET-TEMPORAL": ("ip", "asset", 700_000, False, 3_600_000),
    "ER-COMMUNITY-CORRELATION-ONLY": (
        "community_id",
        "correlation",
        400_000,
        False,
        None,
    ),
}


class ContractError(ValueError):
    pass


def require(condition: bool, code: str) -> None:
    if not condition:
        raise ContractError(code)


def validate_contract(contract: dict[str, Any], schema: dict[str, Any]) -> dict[str, Any]:
    require(schema.get("$schema") == "https://json-schema.org/draft/2020-12/schema", "SCHEMA_DRAFT")
    require(schema.get("additionalProperties") is False, "SCHEMA_ROOT_OPEN")
    require(set(contract) == ROOT_KEYS, "ROOT_FIELDS")
    require(contract.get("schema_version") == 1, "SCHEMA_VERSION")
    require(contract.get("contract_id") == "entity-resolution-v1", "CONTRACT_ID")
    require(contract.get("status") == "frozen_default_off", "STATUS_NOT_FROZEN")
    require(contract.get("accountable_task") == "T1-M06-N008", "ACCOUNTABLE_TASK")
    require(contract.get("rule_version") == "entity-resolution/v1", "RULE_VERSION")
    require(contract.get("identity_roles") == EXPECTED_ROLES, "IDENTITY_ROLES")
    require(contract.get("decision_states") == EXPECTED_STATES, "DECISION_STATES")

    tenant = contract.get("tenant_boundary") or {}
    require(set(tenant) == {"partition_key", "cross_tenant_action"}, "TENANT_FIELDS")
    require(tenant.get("partition_key") == "tenant_id", "TENANT_PARTITION")
    require(tenant.get("cross_tenant_action") == "reject", "CROSS_TENANT_ACTION")

    rails = contract.get("source_rails")
    require(isinstance(rails, list), "SOURCE_RAILS_TYPE")
    by_rail: dict[str, dict[str, Any]] = {}
    for item in rails:
        require(isinstance(item, dict), "SOURCE_RAIL_ITEM")
        require(
            set(item) == {"rail", "allowed_identifiers", "anchor_authority"},
            "SOURCE_RAIL_FIELDS",
        )
        rail = item.get("rail")
        require(isinstance(rail, str) and rail not in by_rail, "SOURCE_RAIL_DUPLICATE")
        by_rail[rail] = item
    require(set(by_rail) == set(EXPECTED_RAILS), "SOURCE_RAIL_EXACT_SET")
    for rail, (identifiers, anchors) in EXPECTED_RAILS.items():
        require(set(by_rail[rail]["allowed_identifiers"]) == identifiers, f"RAIL_IDENTIFIERS:{rail}")
        require(set(by_rail[rail]["anchor_authority"]) == anchors, f"RAIL_ANCHORS:{rail}")

    rules = contract.get("identifier_rules")
    require(isinstance(rules, list), "RULES_TYPE")
    by_rule: dict[str, dict[str, Any]] = {}
    for item in rules:
        require(isinstance(item, dict), "RULE_ITEM")
        rule_id = item.get("rule_id")
        require(isinstance(rule_id, str) and rule_id not in by_rule, "RULE_DUPLICATE")
        require(
            item.get("ambiguity_action") == "preserve_candidates_without_merge",
            f"AMBIGUITY_ACTION:{rule_id}",
        )
        require(isinstance(item.get("normalization"), str) and item["normalization"], f"NORMALIZATION:{rule_id}")
        by_rule[rule_id] = item
    require(set(by_rule) == set(EXPECTED_RULES), "RULE_EXACT_SET")
    identifiers_seen: set[str] = set()
    for rule_id, expected in EXPECTED_RULES.items():
        identifier, scope, confidence, may_create, max_age = expected
        item = by_rule[rule_id]
        require(item.get("identifier") == identifier, f"RULE_IDENTIFIER:{rule_id}")
        require(identifier not in identifiers_seen, f"IDENTIFIER_RULE_DUPLICATE:{identifier}")
        identifiers_seen.add(identifier)
        require(item.get("scope") == scope, f"RULE_SCOPE:{rule_id}")
        require(item.get("confidence_ppm") == confidence, f"RULE_CONFIDENCE:{rule_id}")
        require(item.get("may_create_entity") is may_create, f"RULE_CREATION:{rule_id}")
        require(item.get("max_link_age_ms") == max_age, f"RULE_MAX_AGE:{rule_id}")

    rollout = contract.get("rollout") or {}
    require(
        set(rollout) == {"default_enabled", "writes_external_store", "rollback"},
        "ROLLOUT_FIELDS",
    )
    require(rollout.get("default_enabled") is False, "DEFAULT_ENABLED")
    require(rollout.get("writes_external_store") is False, "EXTERNAL_WRITES")
    require(
        rollout.get("rollback")
        == "disable projector; preserve input facts and resolution results",
        "ROLLBACK",
    )

    determinism = contract.get("determinism") or {}
    require(
        set(determinism)
        == {"resolution_id", "decision_digest", "ordering", "duplicate_source_tuple"},
        "DETERMINISM_FIELDS",
    )
    require("sha256" in determinism.get("resolution_id", ""), "RESOLUTION_ID")
    require("sha256" in determinism.get("decision_digest", ""), "DECISION_DIGEST")

    claims = contract.get("claim_boundary") or {}
    require(set(claims) == {"proves", "does_not_prove"}, "CLAIM_FIELDS")
    require(bool(claims.get("proves")) and bool(claims.get("does_not_prove")), "CLAIM_EMPTY")
    return {
        "status": "PASS",
        "contract_id": contract["contract_id"],
        "rule_version": contract["rule_version"],
        "source_rails": len(by_rail),
        "identifier_rules": len(by_rule),
        "default_enabled": False,
    }


def main() -> int:
    contract = json.loads(CONTRACT_PATH.read_text(encoding="utf-8"))
    schema = json.loads(SCHEMA_PATH.read_text(encoding="utf-8"))
    try:
        result = validate_contract(contract, schema)
    except (ContractError, json.JSONDecodeError) as error:
        print(json.dumps({"status": "FAIL", "error": str(error)}, indent=2))
        return 1
    print(json.dumps(result, indent=2))
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
