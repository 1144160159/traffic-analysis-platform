#!/usr/bin/env python3
"""Verify repository-side T-KAFKA-003 durable DLQ and offset barriers."""

from __future__ import annotations

import json
from pathlib import Path
from typing import Any


ROOT = Path(__file__).resolve().parents[2]
CONTRACT_PATH = ROOT / "contracts/kafka/dlq-commit-barrier.v1.json"


def verify(contract: dict[str, Any] | None = None) -> dict[str, Any]:
    contract = contract or json.loads(CONTRACT_PATH.read_text(encoding="utf-8"))
    errors: list[str] = []

    if contract.get("remediation_id") != "T-KAFKA-003":
        errors.append("remediation_id must remain T-KAFKA-003")
    if contract.get("status") == "closed":
        errors.append("repository evidence cannot close T-KAFKA-003")
    if contract.get("production_applied") is not False:
        errors.append("repository contract cannot claim production_applied")
    remaining = contract.get("gate", {}).get("remaining", [])
    if not remaining:
        errors.append("live and release gates must remain explicit")

    assertion_results: list[dict[str, Any]] = []
    for assertion in contract.get("source_assertions", []):
        relative = assertion.get("source", "")
        source = ROOT / relative
        if not relative or not source.is_file():
            errors.append(f"missing barrier source: {relative}")
            continue
        text = source.read_text(encoding="utf-8")
        missing = [token for token in assertion.get("required_tokens", []) if token not in text]
        forbidden = [token for token in assertion.get("forbidden_tokens", []) if token in text]
        ordering_failures: list[list[str]] = []
        for pair in assertion.get("ordered_tokens", []):
            if not isinstance(pair, list) or len(pair) != 2:
                ordering_failures.append(["invalid", "ordered_tokens"])
                continue
            before, after = pair
            before_position = text.find(before)
            after_position = text.find(after)
            if before_position < 0 or after_position < 0 or before_position >= after_position:
                ordering_failures.append([before, after])
        if missing:
            errors.append(f"{relative}: missing tokens {missing}")
        if forbidden:
            errors.append(f"{relative}: forbidden fail-open tokens {forbidden}")
        if ordering_failures:
            errors.append(f"{relative}: durability ordering failures {ordering_failures}")
        assertion_results.append({
            "source": relative,
            "missing": missing,
            "forbidden": forbidden,
            "ordering_failures": ordering_failures,
        })

    if not assertion_results:
        errors.append("source_assertions must not be empty")

    return {
        "schema_version": 1,
        "contract_id": contract.get("contract_id"),
        "remediation_id": contract.get("remediation_id"),
        "status": "PASS" if not errors else "FAIL",
        "coverage_status": contract.get("coverage_status"),
        "guarantees": len(contract.get("guarantees", [])),
        "source_assertions": assertion_results,
        "errors": errors,
        "remaining_gates": remaining,
    }


def main() -> int:
    result = verify()
    print(json.dumps(result, ensure_ascii=False, indent=2))
    return 0 if result["status"] == "PASS" else 1


if __name__ == "__main__":
    raise SystemExit(main())
