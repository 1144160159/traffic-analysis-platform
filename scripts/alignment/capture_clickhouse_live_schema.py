#!/usr/bin/env python3
"""Capture read-only, per-node ClickHouse schema evidence for T-CH-001."""

from __future__ import annotations

import argparse
import hashlib
import json
import re
import subprocess
from collections import Counter
from datetime import datetime, timezone
from pathlib import Path
from typing import Any

from candidate_snapshot import build_snapshot


ROOT = Path(__file__).resolve().parents[2]
DEFAULT_OUTPUT_ROOT = ROOT / "doc/02_acceptance/runs"
QUERIES = {
    "server": """
SELECT hostName() AS host, version() AS version, now64(3) AS captured_at
FORMAT JSONEachRow
""",
    "tables": """
SELECT hostName() AS host, database, name, toString(uuid) AS uuid, engine,
       engine_full, partition_key, sorting_key, primary_key, sampling_key,
       storage_policy, total_rows, total_bytes, metadata_modification_time,
       create_table_query
FROM system.tables
WHERE database = 'traffic'
ORDER BY name
FORMAT JSONEachRow
""",
    "columns": """
SELECT hostName() AS observer, *
FROM system.columns
WHERE database = 'traffic'
ORDER BY table, position
FORMAT JSONEachRow
""",
    "replicas": """
SELECT hostName() AS host, database, table, engine, is_leader, is_readonly,
       is_session_expired, future_parts, parts_to_check, queue_size,
       inserts_in_queue, merges_in_queue, absolute_delay, total_replicas,
       active_replicas, zookeeper_path, replica_name
FROM system.replicas
WHERE database = 'traffic'
ORDER BY table
FORMAT JSONEachRow
""",
    "distributed_ddl_queue": """
SELECT hostName() AS observer, entry, entry_version, initiator_host,
       initiator_port, cluster, host, port, status, exception_code,
       exception_text, query_create_time, query_finish_time, query_duration_ms
FROM system.distributed_ddl_queue
WHERE cluster = 'traffic_cluster'
ORDER BY entry, host
FORMAT JSONEachRow
""",
}


def sha256(path: Path) -> str:
    return hashlib.sha256(path.read_bytes()).hexdigest()


def run(command: list[str]) -> subprocess.CompletedProcess[bytes]:
    return subprocess.run(command, cwd=ROOT, stdout=subprocess.PIPE, stderr=subprocess.PIPE, check=False)


def json_rows(path: Path) -> list[dict[str, Any]]:
    rows = []
    for line in path.read_text(encoding="utf-8").splitlines():
        if line.strip():
            rows.append(json.loads(line))
    return rows


def integer(value: Any) -> int:
    try:
        return int(value)
    except (TypeError, ValueError):
        return 0


def table_definition(row: dict[str, Any]) -> str:
    query = str(row.get("create_table_query", ""))
    return re.sub(r"\s+UUID\s+'[0-9a-fA-F-]+'", "", query)


def column_definition(row: dict[str, Any]) -> dict[str, Any]:
    keys = (
        "name", "position", "type", "default_kind", "default_expression",
        "comment", "compression_codec", "is_in_partition_key",
        "is_in_primary_key", "is_in_sampling_key", "is_in_sorting_key",
    )
    return {key: row.get(key) for key in keys}


def main() -> int:
    parser = argparse.ArgumentParser()
    parser.add_argument("--run-id", required=True)
    parser.add_argument("--namespace", default="middleware")
    parser.add_argument("--selector", default="app=clickhouse")
    parser.add_argument("--output-root", type=Path, default=DEFAULT_OUTPUT_ROOT)
    args = parser.parse_args()

    output = args.output_root.resolve() / args.run_id
    if output.exists():
        raise SystemExit(f"refusing to overwrite immutable evidence directory: {output}")
    output.mkdir(parents=True)
    candidate_before = build_snapshot()

    pod_result = run(
        [
            "kubectl", "--request-timeout=15s", "-n", args.namespace,
            "get", "pods", "-l", args.selector, "-o", "json",
        ]
    )
    (output / "kubectl-pods.stderr.log").write_bytes(pod_result.stderr)
    if pod_result.returncode != 0:
        (output / "kubectl-pods.stdout.log").write_bytes(pod_result.stdout)
        raise SystemExit("failed to enumerate ClickHouse pods; evidence directory retained")
    pod_payload = json.loads(pod_result.stdout)
    pods = sorted(
        item["metadata"]["name"]
        for item in pod_payload.get("items", [])
        if item.get("status", {}).get("phase") == "Running"
    )
    if not pods:
        raise SystemExit("no running ClickHouse pods found; evidence directory retained")

    artifacts: list[dict[str, Any]] = []
    failures: list[dict[str, Any]] = []
    per_pod_counts: dict[str, dict[str, int]] = {}
    table_signatures: dict[str, dict[str, str]] = {}
    column_signatures: dict[str, dict[str, str]] = {}
    replica_health: dict[str, dict[str, Any]] = {}
    ddl_queue_health: dict[str, dict[str, Any]] = {}
    for pod in pods:
        per_pod_counts[pod] = {}
        for query_name, query in QUERIES.items():
            result = run(
                [
                    "kubectl", "--request-timeout=30s", "-n", args.namespace,
                    "exec", "-c", "clickhouse", pod, "--", "sh", "-lc",
                    'exec clickhouse-client --password "$CLICKHOUSE_PASSWORD" --query "$1"',
                    "capture-clickhouse-schema", query.strip(),
                ]
            )
            data_path = output / f"{pod}-{query_name}.jsonl"
            stderr_path = output / f"{pod}-{query_name}.stderr.log"
            data_path.write_bytes(result.stdout)
            stderr_path.write_bytes(result.stderr)
            rows: list[dict[str, Any]] = []
            if result.returncode == 0:
                try:
                    rows = json_rows(data_path)
                except json.JSONDecodeError as error:
                    failures.append({"pod": pod, "query": query_name, "error": str(error)})
            else:
                failures.append(
                    {"pod": pod, "query": query_name, "exit_code": result.returncode}
                )
            per_pod_counts[pod][query_name] = len(rows)
            artifacts.extend(
                [
                    {
                        "path": data_path.name,
                        "sha256": sha256(data_path),
                        "size_bytes": data_path.stat().st_size,
                        "row_count": len(rows),
                    },
                    {
                        "path": stderr_path.name,
                        "sha256": sha256(stderr_path),
                        "size_bytes": stderr_path.stat().st_size,
                    },
                ]
            )
            if query_name == "tables":
                table_signatures[pod] = {
                    str(row["name"]): hashlib.sha256(
                        table_definition(row).encode("utf-8")
                    ).hexdigest()
                    for row in rows
                }
            if query_name == "columns":
                grouped: dict[str, list[dict[str, Any]]] = {}
                for row in rows:
                    grouped.setdefault(str(row["table"]), []).append(row)
                column_signatures[pod] = {
                    table: hashlib.sha256(
                        json.dumps(
                            [column_definition(value) for value in values],
                            ensure_ascii=False,
                            sort_keys=True,
                        ).encode("utf-8")
                    ).hexdigest()
                    for table, values in grouped.items()
                }
            if query_name == "replicas":
                unhealthy = [
                    {
                        key: row.get(key)
                        for key in (
                            "table", "is_readonly", "is_session_expired",
                            "queue_size", "absolute_delay", "total_replicas",
                            "active_replicas",
                        )
                    }
                    for row in rows
                    if integer(row.get("is_readonly")) != 0
                    or integer(row.get("is_session_expired")) != 0
                    or integer(row.get("queue_size")) != 0
                    or integer(row.get("absolute_delay")) != 0
                    or integer(row.get("active_replicas"))
                    != integer(row.get("total_replicas"))
                ]
                replica_health[pod] = {
                    "replica_table_count": len(rows),
                    "unhealthy_count": len(unhealthy),
                    "unhealthy": unhealthy,
                }
            if query_name == "distributed_ddl_queue":
                statuses = Counter(str(row.get("status")) for row in rows)
                exceptions = [
                    {
                        key: row.get(key)
                        for key in (
                            "entry", "host", "status", "exception_code",
                            "exception_text",
                        )
                    }
                    for row in rows
                    if integer(row.get("exception_code")) != 0
                ]
                ddl_queue_health[pod] = {
                    "row_count": len(rows),
                    "status_counts": dict(sorted(statuses.items())),
                    "exception_count": len(exceptions),
                    "exceptions": exceptions,
                }

    candidate_after = build_snapshot()
    stable = candidate_before["content_sha256"] == candidate_after["content_sha256"]
    status = "PASS" if not failures and stable else "FAIL"
    table_sets = {pod: set(values) for pod, values in table_signatures.items()}
    table_union = set().union(*table_sets.values()) if table_sets else set()
    table_intersection = (
        set.intersection(*table_sets.values()) if table_sets else set()
    )
    table_set_drift = {
        pod: {
            "table_count": len(values),
            "missing_from_union": sorted(table_union - values),
            "extra_vs_intersection": sorted(values - table_intersection),
        }
        for pod, values in table_sets.items()
    }
    definition_drift: dict[str, dict[str, list[str]]] = {}
    for table in sorted(table_intersection):
        table_groups: dict[str, list[str]] = {}
        column_groups: dict[str, list[str]] = {}
        for pod in pods:
            table_groups.setdefault(table_signatures[pod][table], []).append(pod)
            column_groups.setdefault(column_signatures[pod][table], []).append(pod)
        if len(table_groups) > 1 or len(column_groups) > 1:
            definition_drift[table] = {
                "table_definition_groups": list(table_groups.values()),
                "column_definition_groups": list(column_groups.values()),
            }
    manifest = {
        "schema_version": 1,
        "run_id": args.run_id,
        "captured_at": datetime.now(timezone.utc).isoformat(),
        "feature_id": "T-CH-001",
        "status": "PARTIAL" if status == "PASS" else "FAIL",
        "coverage_status": "LIVE_READ_ONLY_SCHEMA_INVENTORY",
        "scoped_evidence_status": status,
        "production_applied": False,
        "read_only": True,
        "namespace": args.namespace,
        "selector": args.selector,
        "pods": pods,
        "per_pod_counts": per_pod_counts,
        "table_definition_signatures": table_signatures,
        "column_definition_signatures": column_signatures,
        "table_set_drift": table_set_drift,
        "definition_drift": definition_drift,
        "replica_health": replica_health,
        "distributed_ddl_queue_health": ddl_queue_health,
        "failures": failures,
        "candidate_source": candidate_before,
        "candidate_source_stable": stable,
        "artifacts": artifacts,
        "proven": [
            "Kubernetes API and ClickHouse authentication were reachable for read-only inventory",
            "system.tables system.columns system.replicas and distributed DDL queue were captured independently on every running ClickHouse pod",
            "per-node table and column definition signatures are available for shard and replica drift analysis"
        ],
        "not_proven": [
            "the migration runner has been applied to production",
            "legacy DDL entrypoints have converged",
            "backfill shadow cutover rollback or data reconciliation has passed",
            "T-CH-001 or any project-level gate is closed"
        ],
        "secrets_captured": False,
    }
    manifest_path = output / "manifest.json"
    manifest_path.write_text(json.dumps(manifest, ensure_ascii=False, indent=2) + "\n", encoding="utf-8")
    print(
        json.dumps(
            {
                "status": manifest["status"],
                "scoped_evidence_status": status,
                "pods": pods,
                "manifest": str(manifest_path),
                "manifest_sha256": sha256(manifest_path),
            },
            ensure_ascii=False,
            indent=2,
        )
    )
    return 0 if status == "PASS" else 1


if __name__ == "__main__":
    raise SystemExit(main())
