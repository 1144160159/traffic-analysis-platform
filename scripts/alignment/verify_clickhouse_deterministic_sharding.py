#!/usr/bin/env python3
"""Verify T-CH-002 inventory and the guarded alert_feedback V2 candidate."""

from __future__ import annotations

import json
import re
from pathlib import Path
from typing import Any


ROOT = Path(__file__).resolve().parents[2]
CONTRACT = Path("contracts/clickhouse/deterministic-sharding.v1.json")
EXPECTED_TABLES = {
    "alert_feedback", "alerts", "alerts_latest", "attack_chain_recommendations",
    "campaigns", "detections_behavior", "device_logs", "dlq_events",
    "entity_graph_edges", "entity_graph_nodes", "evidence", "feature_fp",
    "feature_stat", "flows_raw", "graph_query_log", "pcap_index", "sessions",
    "user_events",
}


def executable_sql(source: str) -> str:
    source = re.sub(r"/\*.*?\*/", "", source, flags=re.DOTALL)
    return re.sub(r"--[^\n]*", "", source)


def verify(root: Path = ROOT) -> dict[str, Any]:
    errors: list[str] = []
    path = root / CONTRACT
    if not path.is_file():
        return {"status": "FAIL", "errors": [f"missing {CONTRACT.as_posix()}"]}
    contract = json.loads(path.read_text(encoding="utf-8"))

    if contract.get("remediation_id") != "T-CH-002":
        errors.append("contract remediation_id must be T-CH-002")
    if contract.get("status") in {"closed", "complete", "pass"}:
        errors.append("partial repository candidate must not claim T-CH-002 closure")
    if contract.get("production_applied") is not False:
        errors.append("repository candidate must not claim production apply")
    policy = contract.get("migration_policy", {})
    if policy.get("in_place_rewrite_forbidden") is not True:
        errors.append("in-place rewrite must remain forbidden")
    if policy.get("silent_secondary_write_failure_forbidden") is not True:
        errors.append("secondary write failure must remain fail-closed")

    entries = contract.get("distributed_tables", [])
    names = [entry.get("table") for entry in entries]
    if len(names) != len(set(names)):
        errors.append("distributed table inventory contains duplicate names")
    if set(names) != EXPECTED_TABLES:
        errors.append(
            f"distributed table inventory mismatch: missing={sorted(EXPECTED_TABLES-set(names))} "
            f"unexpected={sorted(set(names)-EXPECTED_TABLES)}"
        )
    rand_tables = [entry for entry in entries if entry.get("current_sharding_key") == "rand()"]
    stable_tables = [entry for entry in entries if entry.get("current_sharding_key") != "rand()"]
    if len(rand_tables) != 13 or len(stable_tables) != 5:
        errors.append("live baseline must retain 13 rand and 5 deterministic tables")
    for entry in entries:
        target = str(entry.get("target_sharding_key", ""))
        if not target.startswith("cityHash64(tenant_id,"):
            errors.append(f"{entry.get('table')} target key is not tenant-scoped composite hash")
        if not entry.get("business_identity"):
            errors.append(f"{entry.get('table')} has no business identity")
        if not entry.get("writers") or not entry.get("readers"):
            errors.append(f"{entry.get('table')} writer/reader ownership is incomplete")

    candidate = contract.get("first_vertical_candidate", {})
    if candidate.get("table") != "alert_feedback":
        errors.append("first vertical candidate must be alert_feedback")
    if candidate.get("write_flag_default") is not False:
        errors.append("V2 dual-write flag must default false")
    if candidate.get("read_cutover_implemented") is not False:
        errors.append("candidate must not claim read cutover")
    if candidate.get("backfill_executed") is not False:
        errors.append("candidate must not claim backfill")

    migration_path = root / str(candidate.get("migration", ""))
    if not migration_path.is_file():
        errors.append("alert_feedback V2 expand migration is missing")
    else:
        sql = executable_sql(migration_path.read_text(encoding="utf-8"))
        for token in (
            "traffic.alert_feedback_v2_local",
            "traffic.alert_feedback_v2",
            "cityHash64(tenant_id,alert_id)",
            "ReplicatedMergeTree",
        ):
            if token not in sql:
                errors.append(f"V2 migration missing token: {token}")
        if re.search(r"\bON\s+CLUSTER\b", sql, re.IGNORECASE):
            errors.append("direct_each_node V2 migration must not contain ON CLUSTER")
        if re.search(r"\b(?:INSERT|ALTER|DROP|RENAME|TRUNCATE)\b", sql, re.IGNORECASE):
            errors.append("expand-only V2 migration contains data movement or destructive DDL")

    worker = root / "go/control-plane/internal/rules/consumer/model_feedback_inbox_worker.go"
    worker_source = worker.read_text(encoding="utf-8") if worker.is_file() else ""
    for token in (
        "NewModelFeedbackInboxWorkerWithOptions",
        "V2Enabled",
        "traffic.alert_feedback_v2",
        "insert_deduplication_token",
        "projectTarget",
        "clickhouse_projection_total",
    ):
        if token not in worker_source:
            errors.append(f"V2 worker missing guard token: {token}")
    if "alert_feedback_local" in worker_source:
        errors.append("V2 worker must not bypass the Distributed table with a local write")

    fallback_repository = root / "go/control-plane/internal/alert/api/feedback_repository.go"
    fallback_source = (
        fallback_repository.read_text(encoding="utf-8")
        if fallback_repository.is_file() else ""
    )
    if "INSERT INTO traffic.alert_feedback_local" in fallback_source:
        errors.append("legacy alert feedback fallback must not expose a local-table write bypass")

    config_paths = [
        root / "go/control-plane/internal/rules/config/config.go",
        root / "deployments/kubernetes/applications/go-services.yaml",
    ]
    config_source = "\n".join(
        item.read_text(encoding="utf-8") for item in config_paths if item.is_file()
    )
    if "MODEL_FEEDBACK_CLICKHOUSE_PROJECTION_V2_ENABLED" not in config_source:
        errors.append("V2 projection flag is not wired through config and deployment")
    if not re.search(
        r"MODEL_FEEDBACK_CLICKHOUSE_PROJECTION_V2_ENABLED[^\n]*(?:envDefault:\"false\"|value: \"false\")",
        config_source,
    ):
        errors.append("V2 projection flag default false is not explicit")

    if not contract.get("closure_blockers"):
        errors.append("T-CH-002 closure blockers must remain explicit")

    return {
        "status": "PASS" if not errors else "FAIL",
        "coverage_status": contract.get("coverage_status"),
        "production_applied": False,
        "distributed_table_count": len(entries),
        "rand_sharded_count": len(rand_tables),
        "deterministic_count": len(stable_tables),
        "first_vertical_candidate": candidate.get("table"),
        "closure_blockers": contract.get("closure_blockers", []),
        "errors": errors,
    }


def main() -> int:
    result = verify()
    print(json.dumps(result, ensure_ascii=False, indent=2))
    return 0 if result["status"] == "PASS" else 1


if __name__ == "__main__":
    raise SystemExit(main())
