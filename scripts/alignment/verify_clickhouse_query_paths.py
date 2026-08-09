#!/usr/bin/env python3
"""Verify the guarded T-CH-003 alert-list structured-row slice."""

from __future__ import annotations

import json
from pathlib import Path
from typing import Any


ROOT = Path(__file__).resolve().parents[2]
CONTRACT = Path("contracts/clickhouse/query-path-optimization.v1.json")
SOURCE = Path("go/control-plane/internal/alert/repository/clickhouse.go")


def verify(root: Path = ROOT) -> dict[str, Any]:
    errors: list[str] = []
    contract_path = root / CONTRACT
    source_path = root / SOURCE
    if not contract_path.is_file():
        return {"status": "FAIL", "errors": [f"missing {CONTRACT.as_posix()}"]}
    if not source_path.is_file():
        return {"status": "FAIL", "errors": [f"missing {SOURCE.as_posix()}"]}
    contract = json.loads(contract_path.read_text(encoding="utf-8"))
    source = source_path.read_text(encoding="utf-8")

    if contract.get("remediation_id") != "T-CH-003":
        errors.append("contract remediation_id must be T-CH-003")
    if contract.get("status") in {"closed", "complete", "pass"}:
        errors.append("partial query slice must not claim T-CH-003 closure")
    if contract.get("production_applied") is not False:
        errors.append("repository query slice must not claim production apply")
    slice_contract = contract.get("initial_slice", {})
    if slice_contract.get("maximum_rows_per_page") != 1000:
        errors.append("alert list maximum page must remain 1000")
    preserved = set(slice_contract.get("preserved_semantics", []))
    required_preserved = {
        "tenant predicate", "alerts_latest FINAL source", "exact filtered count",
        "offset compatibility", "validated sort field and direction",
    }
    if not required_preserved.issubset(preserved):
        errors.append("semantic preservation inventory is incomplete")

    list_start = source.find("func (r *AlertRepository) List(")
    list_end = source.find("// GetByID", list_start)
    list_source = source[list_start:list_end]
    if list_start < 0 or list_end < 0:
        errors.append("alert repository List source boundary is missing")
    else:
        forbidden = ("toJSONString(groupArray", "json.Unmarshal", "decodeAlertListPage")
        for token in forbidden:
            if token in list_source:
                errors.append(f"alert list retains JSON page aggregation token: {token}")
        for token in (
            "r.client.Query(ctx, dataSQL",
            "scanAlertListRows",
            "alertListSelectColumns",
            "alertLatestProjection",
            "SELECT count()",
            "OFFSET %d",
            "query.Limit <= 1000",
            "tenant_id = ?",
        ):
            if token not in list_source and token not in source:
                errors.append(f"alert list missing compatibility or bound token: {token}")

    policy = contract.get("optimization_policy", {})
    for key in (
        "final_removal_requires_semantic_reconciliation",
        "exact_count_decoupling_requires_api_contract",
        "offset_removal_forbidden_without_cursor_compatibility",
        "query_profile_required_before_performance_claim",
    ):
        if policy.get(key) is not True:
            errors.append(f"query optimization guard must remain true: {key}")
    live = contract.get("available_live_evidence", {})
    if live.get("correctness_or_performance_claim_supported") is not False:
        errors.append("insufficient live sample must not support a performance claim")
    if not contract.get("closure_blockers"):
        errors.append("T-CH-003 closure blockers must remain explicit")

    return {
        "status": "PASS" if not errors else "FAIL",
        "coverage_status": contract.get("coverage_status"),
        "production_applied": False,
        "initial_slice": slice_contract.get("operation"),
        "maximum_rows_per_page": slice_contract.get("maximum_rows_per_page"),
        "closure_blockers": contract.get("closure_blockers", []),
        "errors": errors,
    }


def main() -> int:
    result = verify()
    print(json.dumps(result, ensure_ascii=False, indent=2))
    return 0 if result["status"] == "PASS" else 1


if __name__ == "__main__":
    raise SystemExit(main())
