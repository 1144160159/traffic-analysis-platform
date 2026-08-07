#!/usr/bin/env python3
"""Verify repository guards for T-PG-002 transactional audit and outbox boundaries."""

from __future__ import annotations

import json
from pathlib import Path
from typing import Any


ROOT = Path(__file__).resolve().parents[2]
CONTRACT = ROOT / "contracts/postgres/transaction-outbox.v1.json"
CANONICAL_REGISTRY = ROOT / "contracts/alignment/canonical-registry.json"


def verify(contract_path: Path = CONTRACT) -> dict[str, Any]:
    contract = json.loads(contract_path.read_text(encoding="utf-8"))
    errors: list[str] = []
    canonical = {
        item["id"]
        for item in json.loads(CANONICAL_REGISTRY.read_text(encoding="utf-8"))["items"]
    }
    slices = [contract.get("implemented_slice") or {}, *(contract.get("additional_slices") or [])]
    noncanonical_slice_ids = sorted({
        item.get("feature_id")
        for item in slices
        if item.get("feature_id") not in canonical
    })
    if noncanonical_slice_ids:
        errors.append(f"transaction slices use non-canonical IDs: {noncanonical_slice_ids}")
    assertions = []
    for assertion in contract.get("source_assertions") or []:
        relative = assertion["source"]
        source = (ROOT / relative).read_text(encoding="utf-8")
        operation = source
        operation_start = assertion.get("operation_start")
        operation_end = assertion.get("operation_end")
        if operation_start:
            if operation_start not in source:
                errors.append(f"{relative}: operation start is missing: {operation_start}")
                operation = ""
            else:
                operation = source.split(operation_start, 1)[1]
                if operation_end:
                    operation = operation.split(operation_end, 1)[0]
        elif "func (h *Handler) SaveAlertView" in source:
            operation = source.split("func (h *Handler) SaveAlertView", 1)[1].split(
                "func (h *Handler) ListAlertViews", 1
            )[0]
        missing = [token for token in assertion.get("required") or [] if token not in source]
        forbidden = [
            token for token in assertion.get("forbidden_in_operation") or [] if token in operation
        ]
        ordered = assertion.get("ordered") or []
        positions = []
        cursor = 0
        for token in ordered:
            position = operation.find(token, cursor)
            positions.append(position)
            if position >= 0:
                cursor = position + len(token)
        ordering_failed = bool(ordered) and any(position < 0 for position in positions)
        if missing:
            errors.append(f"{relative}: missing {missing}")
        if forbidden:
            errors.append(f"{relative}: forbidden non-transactional calls {forbidden}")
        if ordering_failed:
            errors.append(f"{relative}: transaction fact ordering failed")
        assertions.append(
            {"source": relative, "missing": missing, "forbidden": forbidden, "ordering_failed": ordering_failed}
        )

    schema_tokens = [
        "202608031100",
        "alert_saved_view_requests",
        "alert_saved_view_history",
        "alert_saved_view_outbox",
        "idx_alert_saved_view_outbox_ready",
    ] + list(contract.get("required_outbox_fields") or [])
    schema_results = []
    for relative in contract.get("schema_sources") or []:
        source = (ROOT / relative).read_text(encoding="utf-8")
        missing = [token for token in schema_tokens if token not in source]
        if missing:
            errors.append(f"{relative}: missing schema tokens {missing}")
        schema_results.append({"source": relative, "missing": missing})

    additional_schema_results = []
    for group in contract.get("additional_schema_groups") or []:
        group_results = []
        tokens = group.get("tokens") or []
        for relative in group.get("sources") or []:
            source = (ROOT / relative).read_text(encoding="utf-8")
            missing = [token for token in tokens if token not in source]
            if missing:
                errors.append(f"{relative}: missing {group.get('name')} schema tokens {missing}")
            group_results.append({"source": relative, "missing": missing})
        additional_schema_results.append({"name": group.get("name"), "sources": group_results})

    return {
        "schema_version": 1,
        "contract_id": contract.get("contract_id"),
        "remediation_id": "T-PG-002",
        "status": "PASS" if not errors else "FAIL",
        "coverage_status": "PARTIAL",
        "implemented_slice": (contract.get("implemented_slice") or {}).get("operation"),
        "publisher_status": (contract.get("implemented_slice") or {}).get("publisher_status"),
        "transaction_facts": len((contract.get("implemented_slice") or {}).get("transaction_facts") or []),
        "outbox_fields": len(contract.get("required_outbox_fields") or []),
        "source_assertions": assertions,
        "schema_sources": schema_results,
        "additional_slices": [item.get("operation") for item in contract.get("additional_slices") or []],
        "noncanonical_slice_ids": noncanonical_slice_ids,
        "additional_schema_groups": additional_schema_results,
        "errors": errors,
        "remaining_gates": (contract.get("gate") or {}).get("remaining") or [],
    }


def main() -> int:
    result = verify()
    print(json.dumps(result, ensure_ascii=False, indent=2))
    return 0 if result["status"] == "PASS" else 1


if __name__ == "__main__":
    raise SystemExit(main())
