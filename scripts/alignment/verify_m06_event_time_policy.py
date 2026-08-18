#!/usr/bin/env python3
"""Fail-closed semantic verifier for the shared M06 event-time policy."""

from __future__ import annotations

import json
from pathlib import Path
from typing import Any


ROOT = Path(__file__).resolve().parents[2]
CONTRACT_PATH = ROOT / "contracts/events/event-time-policy.v1.json"
SCHEMA_PATH = ROOT / "contracts/events/event-time-policy.schema.json"

ROOT_KEYS = {
    "schema_version",
    "contract_id",
    "status",
    "accountable_task",
    "unit",
    "time_roles",
    "boundaries",
    "classification_order",
    "wire_requirements",
    "implementations",
    "rollout",
    "claim_boundary",
}
CLASSIFICATION_ORDER = [
    "INVALID_EVENT_TIME",
    "INVALID_INGEST_TIME",
    "INVALID_PROCESSING_TIME",
    "FUTURE_EVENT",
    "CLOCK_ROLLBACK",
    "LATE_EVENT",
    "ACCEPT",
]


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
    require(contract.get("contract_id") == "event-time-policy-v1", "CONTRACT_ID")
    require(contract.get("status") == "implemented_shared_library", "STATUS")
    require(contract.get("accountable_task") == "T1-M06-N009", "ACCOUNTABLE_TASK")
    require(contract.get("unit") == "unix_epoch_milliseconds", "UNIT")

    roles = contract.get("time_roles") or {}
    require(
        set(roles) == {"event_time", "ingest_time", "processing_time", "watermark", "as_of"},
        "TIME_ROLES",
    )
    require("min(initialized watermark, processing_time)" in roles["as_of"], "AS_OF")

    boundaries = contract.get("boundaries") or {}
    require(
        set(boundaries)
        == {"late", "future", "clock_rollback", "watermark_uninitialized", "arithmetic"},
        "BOUNDARY_FIELDS",
    )
    require(boundaries.get("late") == "event_time < watermark - allowed_lateness_ms", "LATE_BOUNDARY")
    require(boundaries.get("future") == "event_time > ingest_time + max_future_skew_ms", "FUTURE_BOUNDARY")
    require(
        boundaries.get("clock_rollback")
        == "event_time < identity_max_event_time - max_clock_rollback_ms",
        "ROLLBACK_BOUNDARY",
    )
    require(boundaries.get("watermark_uninitialized") == -(2**63), "WATERMARK_SENTINEL")
    require(boundaries.get("arithmetic") == "signed_int64_saturating", "ARITHMETIC")
    require(contract.get("classification_order") == CLASSIFICATION_ORDER, "CLASSIFICATION_ORDER")

    wire = contract.get("wire_requirements") or {}
    require(wire.get("positive_times") == ["event_time", "ingest_time", "processing_time"], "POSITIVE_TIMES")
    require("explicit offset" in wire.get("timezone", ""), "TIMEZONE")
    require("never reads the wall clock" in wire.get("replay", ""), "REPLAY_CLOCK")

    implementations = contract.get("implementations") or {}
    require(
        implementations.get("java")
        == "java/flink-jobs/flink-common/src/main/java/com/traffic/flink/common/eventtime/EventTimePolicy.java",
        "JAVA_IMPLEMENTATION",
    )
    require(
        implementations.get("go") == "go/control-plane/internal/eventtime/policy.go",
        "GO_IMPLEMENTATION",
    )
    require(
        set(implementations.get("java_consumers") or [])
        == {"flink-log-job", "flink-session-job", "flink-feature-job", "flink-user-behavior-job"},
        "JAVA_CONSUMERS",
    )
    for path in (implementations["java"], implementations["go"]):
        require((ROOT / path).is_file(), f"IMPLEMENTATION_MISSING:{path}")

    rollout = contract.get("rollout") or {}
    require("approved rollout" in rollout.get("kubernetes_effect", ""), "K8S_ACTIVATION_BOUNDARY")
    require("previous immutable image" in rollout.get("rollback", ""), "ROLLBACK")
    claims = contract.get("claim_boundary") or {}
    require(bool(claims.get("proves")) and bool(claims.get("does_not_prove")), "CLAIM_BOUNDARY")
    return {
        "status": "PASS",
        "contract_id": contract["contract_id"],
        "classification_states": len(CLASSIFICATION_ORDER),
        "java_consumers": len(implementations["java_consumers"]),
    }


def main() -> int:
    try:
        result = validate_contract(
            json.loads(CONTRACT_PATH.read_text(encoding="utf-8")),
            json.loads(SCHEMA_PATH.read_text(encoding="utf-8")),
        )
    except (ContractError, json.JSONDecodeError) as error:
        print(json.dumps({"status": "FAIL", "error": str(error)}, indent=2))
        return 1
    print(json.dumps(result, indent=2))
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
