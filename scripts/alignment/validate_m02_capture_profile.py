#!/usr/bin/env python3
"""Fail-closed validation for the M02 capture profile and counter method."""

from __future__ import annotations

import copy
import json
from pathlib import Path


ROOT = Path(__file__).resolve().parents[2]
SCHEMA = ROOT / "contracts/quality/m02-capture-profile.schema.json"
PROFILE = ROOT / "contracts/quality/m02-approved-ten-gigabit-or-higher-profile.v1.json"
ATTRIBUTION = ROOT / "contracts/quality/m02-capture-counter-attribution.v1.json"
REQUIRED_AUTHORITIES = ["PROJECT_OWNER", "TEST_OWNER", "ACCEPTANCE_AUTHORITY"]


def validate(profile: dict, attribution: dict) -> None:
    schema = json.loads(SCHEMA.read_text())
    if schema.get("$schema") != "https://json-schema.org/draft/2020-12/schema":
        raise ValueError("profile schema draft drifted")
    expected_fields = set(schema["required"])
    if set(profile) != expected_fields:
        raise ValueError("profile top-level exact-set drifted")
    if profile.get("schema_version") != "1.0.0" or profile.get("artifact_kind") != "M02_CAPTURE_PROFILE":
        raise ValueError("profile identity drifted")
    if profile.get("line_rate_gbps", 0) < 10:
        raise ValueError("line_rate_gbps is below minimum 10")
    if profile.get("profile_status") not in {"PENDING_SIGNATURE", "APPROVED", "SUPERSEDED"}:
        raise ValueError("profile status is invalid")
    if set(profile["stop_thresholds"].values()) != {0}:
        raise ValueError("capture stop thresholds must all be zero")
    points = set(profile["measurement"]["points"])
    required_points = {"GENERATOR_TX", "NIC_RX", "PROBE_CAPTURE", "KAFKA_OFFSET", "MINIO_OBJECT", "CLICKHOUSE_INDEX"}
    if not required_points <= points:
        raise ValueError("measurement point exact minimum is incomplete")
    if attribution.get("artifact_kind") != "M02_CAPTURE_COUNTER_ATTRIBUTION":
        raise ValueError("counter attribution kind drifted")
    if attribution.get("method_status") != "PENDING_SIGNATURE":
        raise ValueError("unsigned counter method claimed a signed status")
    if attribution.get("counter_revision") != 1:
        raise ValueError("counter revision drifted")
    if profile["profile_status"] == "PENDING_SIGNATURE" and profile["approval"]["receipts"]:
        raise ValueError("pending profile contains an approval receipt")
    if profile["profile_status"] == "APPROVED" and not profile["approval"]["receipts"]:
        raise ValueError("approved profile receipts must be non-empty")
    if profile["approval"]["required_authorities"] != REQUIRED_AUTHORITIES:
        raise ValueError("profile approval authority exact-set or order mismatch")
    required = {
        "system_attributable_drop_packets": "capture_allocation_drops + capture_kernel_drops",
        "unexplained_difference_packets": "capture_counter_difference_packets - approved_counter_error_packets",
    }
    for name, formula in required.items():
        if attribution["formulas"].get(name) != formula:
            raise ValueError(f"counter attribution formula drifted: {name}")


def expect_failure(name: str, profile: dict, attribution: dict, expected: str) -> None:
    try:
        validate(profile, attribution)
    except Exception as error:
        if expected not in str(error):
            raise ValueError(f"{name} hit wrong failure: {error}") from error
        return
    raise ValueError(f"malicious capture profile mutation was accepted: {name}")


def main() -> int:
    profile = json.loads(PROFILE.read_text())
    attribution = json.loads(ATTRIBUTION.read_text())
    validate(profile, attribution)

    mutated = copy.deepcopy(profile)
    mutated["line_rate_gbps"] = 1
    expect_failure("sub-ten-gigabit", mutated, attribution, "minimum")
    mutated = copy.deepcopy(profile)
    mutated["profile_status"] = "APPROVED"
    expect_failure("approval-without-receipt", mutated, attribution, "non-empty")
    mutated_method = copy.deepcopy(attribution)
    mutated_method["formulas"]["system_attributable_drop_packets"] = "packets_dropped"
    expect_failure("attribution-drift", profile, mutated_method, "formula drifted")
    mutated = copy.deepcopy(profile)
    mutated["approval"]["required_authorities"] = ["PROJECT_OWNER", "TEST_OWNER"]
    expect_failure("approval-authority-omission", mutated, attribution, "authority exact-set")

    print("PASS M02 capture profile: schema, pending signature ceiling, attribution formulas and mutations")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
