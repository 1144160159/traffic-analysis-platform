#!/usr/bin/env python3
"""Verify the repository-side T-CH-005 retention and lifecycle contract."""

from __future__ import annotations

import json
import re
from pathlib import Path
from typing import Any


ROOT = Path(__file__).resolve().parents[2]
CONTRACT = Path("contracts/clickhouse/retention-lifecycle.v1.json")
COMMON_DDL = Path("common/sql/ch/00-all-tables.sql")
DOCKER_DDL = Path("go/control-plane/deployments/docker/init/clickhouse_merged.sql")
MINIO_LIFECYCLE = Path("deployments/kubernetes/init-jobs/06-minio-lifecycle.yaml")
ROLLUP_MIGRATION = Path("deployments/clickhouse/migrations/202608031600_sessions_daily_rollup_v1.sql")
REQUIRED_DOMAINS = {
    "raw_flow", "session_detail", "session_daily_rollup", "alert_fact",
    "audit_event", "pcap_index", "pcap_object", "topic_snapshot", "report_object",
}


def table_block(source: str, table_pattern: str) -> str:
    match = re.search(
        rf"CREATE\s+TABLE\s+IF\s+NOT\s+EXISTS\s+{table_pattern}\b"
        rf".*?(?=\nCREATE\s+(?:TABLE|MATERIALIZED\s+VIEW)\s+IF\s+NOT\s+EXISTS|\Z)",
        source,
        re.I | re.S,
    )
    return match.group(0) if match else ""


def lifecycle_days(source: str, rule_id: str) -> int | None:
    match = re.search(
        rf'"ID"\s*:\s*"{re.escape(rule_id)}".*?"Expiration"\s*:\s*\{{.*?"Days"\s*:\s*(\d+)',
        source,
        re.S,
    )
    return int(match.group(1)) if match else None


def verify(root: Path = ROOT) -> dict[str, Any]:
    errors: list[str] = []
    required_paths = (CONTRACT, COMMON_DDL, DOCKER_DDL, MINIO_LIFECYCLE, ROLLUP_MIGRATION)
    missing = [path.as_posix() for path in required_paths if not (root / path).is_file()]
    if missing:
        return {"status": "FAIL", "errors": [f"missing required files: {missing}"]}

    contract = json.loads((root / CONTRACT).read_text(encoding="utf-8"))
    if contract.get("remediation_id") != "T-CH-005":
        errors.append("contract remediation_id must be T-CH-005")
    if contract.get("status") in {"closed", "complete", "pass"}:
        errors.append("partial retention slice must not claim T-CH-005 closure")
    if contract.get("production_applied") is not False:
        errors.append("repository retention candidate must not claim production apply")

    policy = contract.get("policy", {})
    for key in (
        "retention_change_requires_capacity_and_compliance_approval",
        "object_retention_must_exceed_reference_retention_by_grace_days",
        "high_granularity_expiry_requires_versioned_rollup",
        "expired_object_reference_requires_explicit_state_or_http_410",
        "ttl_migration_must_use_versioned_migration",
        "live_configuration_must_be_recaptured_before_cutover",
    ):
        if policy.get(key) is not True:
            errors.append(f"retention guard must remain true: {key}")

    matrix = contract.get("retention_matrix", [])
    by_domain = {str(item.get("domain")): item for item in matrix}
    if len(by_domain) != len(matrix):
        errors.append("retention matrix domain names must be unique")
    missing_domains = sorted(REQUIRED_DOMAINS - set(by_domain))
    if missing_domains:
        errors.append(f"retention matrix missing domains: {missing_domains}")
    for domain, item in by_domain.items():
        if not item.get("owner"):
            errors.append(f"retention matrix owner missing: {domain}")
        if int(item.get("retention_days", 0)) <= 0:
            errors.append(f"retention_days must be positive: {domain}")
        if not item.get("expiry_behavior"):
            errors.append(f"expiry_behavior missing: {domain}")

    if not missing_domains:
        detail_days = int(by_domain["session_detail"]["retention_days"])
        rollup_days = int(by_domain["session_daily_rollup"]["retention_days"])
        if rollup_days <= detail_days:
            errors.append("session rollup retention must exceed detail retention")
        if int(by_domain["session_daily_rollup"].get("aggregate_version", 0)) != 1:
            errors.append("session rollup must declare aggregate_version 1")
        pcap_index_days = int(by_domain["pcap_index"]["retention_days"])
        pcap_object = by_domain["pcap_object"]
        pcap_object_days = int(pcap_object["retention_days"])
        grace_days = int(pcap_object.get("grace_days", 0))
        if grace_days < 1 or pcap_object_days < pcap_index_days + grace_days:
            errors.append("PCAP object retention must cover index retention plus positive grace")

    common = (root / COMMON_DDL).read_text(encoding="utf-8")
    docker = (root / DOCKER_DDL).read_text(encoding="utf-8")
    common_sessions = table_block(common, r"traffic\.sessions_local")
    docker_sessions = table_block(docker, r"\$\{CH_DB\}\.sessions_local")
    if not common_sessions or not re.search(
        r"TTL\s+toDateTime\(ts_end\s*/\s*1000\)\s*\+\s*toIntervalDay\(90\)",
        common_sessions,
        re.I,
    ):
        errors.append("common sessions TTL must remain 90 days")
    if not docker_sessions or not re.search(
        r"TTL\s+toDateTime\(ts_end\)\s*\+\s*INTERVAL\s+90\s+DAY",
        docker_sessions,
        re.I,
    ):
        errors.append("Docker sessions TTL must match the 90-day contract")

    lifecycle = (root / MINIO_LIFECYCLE).read_text(encoding="utf-8")
    pcap_days = lifecycle_days(lifecycle, "expire-pcap-archive-after-37-days")
    report_days = lifecycle_days(lifecycle, "expire-report-artifacts-after-90-days")
    if pcap_days != 37:
        errors.append("pcap-archive lifecycle must be 37 days")
    if report_days != 90:
        errors.append("report-artifacts lifecycle must remain 90 days")

    migration = (root / ROLLUP_MIGRATION).read_text(encoding="utf-8")
    executable = re.sub(r"--[^\n]*", "", migration)
    if re.search(r"\bON\s+CLUSTER\b", executable, re.I):
        errors.append("direct-each-node rollup migration must not use ON CLUSTER")
    for token in (
        "traffic.sessions_daily_rollup_v1_local",
        "traffic.sessions_daily_rollup_v1",
        "traffic.mv_sessions_daily_rollup_v1_local",
        "ReplicatedAggregatingMergeTree",
        "SimpleAggregateFunction(sum, UInt64)",
        "SimpleAggregateFunction(min, DateTime)",
        "SimpleAggregateFunction(max, DateTime)",
        "aggregate_version",
        "toUInt16(1) AS aggregate_version",
        "INTERVAL 365 DAY",
        "FROM traffic.sessions_local",
    ):
        if token not in migration:
            errors.append(f"rollup migration missing required token: {token}")

    observed = contract.get("observed_live_before_candidate", {})
    if (
        int(observed.get("sessions_days", 0)) != 90
        or int(observed.get("pcap_index_days", 0)) != 30
        or int(observed.get("pcap_object_days", 0)) != 14
        or int(observed.get("maximum_dangling_reference_days", 0)) != 16
        or observed.get("observation_is_not_production_apply_evidence") is not True
    ):
        errors.append("pre-candidate live mismatch observation must remain explicit")
    if not contract.get("closure_blockers"):
        errors.append("T-CH-005 closure blockers must remain explicit")

    return {
        "status": "PASS" if not errors else "FAIL",
        "coverage_status": contract.get("coverage_status"),
        "production_applied": False,
        "matrix_domains": len(by_domain),
        "sessions_retention_days": by_domain.get("session_detail", {}).get("retention_days"),
        "session_rollup_retention_days": by_domain.get("session_daily_rollup", {}).get("retention_days"),
        "pcap_index_retention_days": by_domain.get("pcap_index", {}).get("retention_days"),
        "pcap_object_retention_days": pcap_days,
        "pcap_object_grace_days": by_domain.get("pcap_object", {}).get("grace_days"),
        "closure_blockers": contract.get("closure_blockers", []),
        "errors": errors,
    }


def main() -> int:
    result = verify()
    print(json.dumps(result, ensure_ascii=False, indent=2))
    return 0 if result["status"] == "PASS" else 1


if __name__ == "__main__":
    raise SystemExit(main())
