#!/usr/bin/env python3
"""Verify the T-CH-004 Go alert/evidence append-only writer slice."""

from __future__ import annotations

import json
import re
from pathlib import Path
from typing import Any


ROOT = Path(__file__).resolve().parents[2]
CONTRACT = Path("contracts/clickhouse/append-only-version-semantics.v1.json")
SCAN_ROOT = Path("go/control-plane/internal/alert")
LOCAL_INSERT = re.compile(r"\bINSERT\s+INTO\s+traffic\.[A-Za-z0-9_]+_local\b", re.I)
SYNC_MUTATION = re.compile(r"\bALTER\s+TABLE\s+traffic\.[A-Za-z0-9_]+\s+(?:UPDATE|DELETE)\b", re.I)
DISTRIBUTED_INSERT = re.compile(r"\bINSERT\s+INTO\s+traffic\.(alerts|evidence)\b", re.I)


def go_sources(root: Path) -> list[Path]:
    base = root / SCAN_ROOT
    return sorted(path for path in base.rglob("*.go") if not path.name.endswith("_test.go"))


def verify(root: Path = ROOT) -> dict[str, Any]:
    errors: list[str] = []
    path = root / CONTRACT
    if not path.is_file():
        return {"status": "FAIL", "errors": [f"missing {CONTRACT.as_posix()}"]}
    contract = json.loads(path.read_text(encoding="utf-8"))
    if contract.get("remediation_id") != "T-CH-004":
        errors.append("contract remediation_id must be T-CH-004")
    if contract.get("status") in {"closed", "complete", "pass"}:
        errors.append("partial writer slice must not claim T-CH-004 closure")
    if contract.get("production_applied") is not False:
        errors.append("repository writer slice must not claim production apply")

    local_hits: list[str] = []
    mutation_hits: list[str] = []
    distributed_hits: list[dict[str, Any]] = []
    per_source: dict[tuple[str, str], int] = {}
    for source_path in go_sources(root):
        relative = source_path.relative_to(root).as_posix()
        source = source_path.read_text(encoding="utf-8", errors="ignore")
        local_hits.extend(f"{relative}:{m.group(0)}" for m in LOCAL_INSERT.finditer(source))
        mutation_hits.extend(f"{relative}:{m.group(0)}" for m in SYNC_MUTATION.finditer(source))
        for match in DISTRIBUTED_INSERT.finditer(source):
            table = f"traffic.{match.group(1).lower()}"
            per_source[(relative, table)] = per_source.get((relative, table), 0) + 1
            distributed_hits.append({"source": relative, "table": table})
    if local_hits:
        errors.append(f"Go alert domain retains local-table inserts: {local_hits}")
    if mutation_hits:
        errors.append(f"Go alert domain retains synchronous ClickHouse mutations: {mutation_hits}")

    slice_contract = contract.get("implemented_slice", {})
    if len(distributed_hits) != slice_contract.get("distributed_insert_sites"):
        errors.append("Distributed alert/evidence insert count differs from contract")
    if slice_contract.get("local_insert_sites") != 0 or slice_contract.get("synchronous_mutation_sites") != 0:
        errors.append("implemented slice must declare zero local inserts and mutations")

    declared = {
        (str(item.get("source")), str(item.get("table"))): int(item.get("sites", 0))
        for item in contract.get("writer_inventory", [])
    }
    if declared != per_source:
        errors.append(f"writer inventory mismatch: declared={declared} actual={per_source}")
    policy = contract.get("write_policy", {})
    for key in (
        "append_only_default",
        "local_table_write_requires_explicit_shard_selection_contract",
        "synchronous_request_waiting_for_mutation_forbidden",
        "delete_requires_tombstone_ttl_or_separate_compliance_workflow",
        "replay_requires_stable_event_identity",
        "event_and_ingestion_time_must_be_distinct",
    ):
        if policy.get(key) is not True:
            errors.append(f"append-only guard must remain true: {key}")
    if not contract.get("closure_blockers"):
        errors.append("T-CH-004 closure blockers must remain explicit")

    return {
        "status": "PASS" if not errors else "FAIL",
        "coverage_status": contract.get("coverage_status"),
        "production_applied": False,
        "distributed_insert_sites": len(distributed_hits),
        "local_insert_sites": len(local_hits),
        "synchronous_mutation_sites": len(mutation_hits),
        "closure_blockers": contract.get("closure_blockers", []),
        "errors": errors,
    }


def main() -> int:
    result = verify()
    print(json.dumps(result, ensure_ascii=False, indent=2))
    return 0 if result["status"] == "PASS" else 1


if __name__ == "__main__":
    raise SystemExit(main())
