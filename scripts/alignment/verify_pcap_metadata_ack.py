#!/usr/bin/env python3
"""Verify repository-side PCAP metadata receipt semantics."""

from __future__ import annotations

import json
from pathlib import Path
from typing import Any


ROOT = Path(__file__).resolve().parents[2]
CONTRACT_PATH = ROOT / "contracts/kafka/pcap-metadata-ack.v1.json"
EXPECTED_IDS = {"F-PROBE-001", "T-KAFKA-003"}


def verify(contract: dict[str, Any] | None = None) -> dict[str, Any]:
    contract = contract or json.loads(CONTRACT_PATH.read_text(encoding="utf-8"))
    errors: list[str] = []
    if set(contract.get("remediation_ids", [])) != EXPECTED_IDS:
        errors.append("PCAP ACK contract must bind F-PROBE-001 and T-KAFKA-003")
    if contract.get("status") == "closed":
        errors.append("repository PCAP evidence cannot close canonical items")
    if contract.get("production_applied") is not False:
        errors.append("repository PCAP contract cannot claim production_applied")
    if not contract.get("remaining_gates"):
        errors.append("PCAP live and release gates must remain explicit")

    assertion_results: list[dict[str, Any]] = []
    for assertion in contract.get("source_assertions", []):
        relative = assertion.get("source", "")
        source = ROOT / relative
        if not relative or not source.is_file():
            errors.append(f"missing PCAP ACK source: {relative}")
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
            after_position = text.find(after, before_position + len(before))
            if before_position < 0 or after_position < 0:
                ordering_failures.append([before, after])
        if missing:
            errors.append(f"{relative}: missing tokens {missing}")
        if forbidden:
            errors.append(f"{relative}: forbidden pseudo-ACK tokens {forbidden}")
        if ordering_failures:
            errors.append(f"{relative}: ACK ordering failures {ordering_failures}")
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
        "remediation_ids": contract.get("remediation_ids", []),
        "status": "PASS" if not errors else "FAIL",
        "coverage_status": contract.get("coverage_status"),
        "source_assertions": assertion_results,
        "errors": errors,
        "remaining_gates": contract.get("remaining_gates", []),
    }


def main() -> int:
    result = verify()
    print(json.dumps(result, ensure_ascii=False, indent=2))
    return 0 if result["status"] == "PASS" else 1


if __name__ == "__main__":
    raise SystemExit(main())
