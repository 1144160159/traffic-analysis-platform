#!/usr/bin/env python3
"""Replay governed PostgreSQL schema entrypoints in disposable databases.

The verifier only accepts the dedicated Codex playbook PostgreSQL container and
requires the sentinel marker before it creates or drops its exact temporary
databases. It never connects to the shared Kubernetes PostgreSQL service.
"""

from __future__ import annotations

import argparse
import difflib
import hashlib
import json
import re
import subprocess
from pathlib import Path
from typing import Any

import yaml


ROOT = Path(__file__).resolve().parents[2]
SENTINEL_DATABASE = "traffic_platform"
SENTINEL_QUERY = (
    "SELECT marker FROM codex_ephemeral_playbook_v2_sentinel LIMIT 1"
)
K8S_SCHEMA = ROOT / "deployments/kubernetes/init-jobs/02-postgres-schema.yaml"
K8S_FILES = (
    "00-init.sql",
    "01-assets.sql",
    "02-features-rules.sql",
    "03-models-deploy.sql",
    "04-tasks-audit.sql",
    "05-default-data.sql",
    "06-graph.sql",
    "07-campaign-impact-assets.sql",
    "08-alert-campaign-links.sql",
    "09-alert-report-jobs.sql",
    "10-campaign-event-projection-v2.sql",
    "11-campaign-soar-workflow.sql",
    "12-playbook-execution-v2.sql",
    "13-playbook-event-pipeline-v2.sql",
    "14-alert-saved-view-transaction-v2.sql",
    "15-notification-rule-transaction-v2.sql",
    "16-probe-registry-atomic.sql",
    "17-threat-intel-command-atomic.sql",
    "18-forensics-task-atomic.sql",
    "19-whitelist-governance-v2.sql",
    "20-dashboard-task-v2.sql",
    "21-alert-opensearch-projection-reconcile-v1.sql",
    "22-data-quality-control-plane-v1.sql",
    "23-data-quality-governance-v1.sql",
    "24-data-quality-rule-evaluation-v1.sql",
    "25-data-quality-repair-lifecycle-v1.sql",
    "26-data-quality-replay-projection-v1.sql",
    "27-dashboard-task-execution-pipeline-v1.sql",
    "28-dashboard-task-compensation-v1.sql",
    "29-dashboard-task-dlq-receipt-v1.sql",
    "30-alert-response-external-executor-v1.sql",
    "31-alert-response-dlq-receipt-v1.sql",
)
PLAYBOOK_TABLES = (
    "alert_response_actions",
    "alert_response_outbox",
    "alert_response_execution_receipts",
    "alert_response_dlq_receipts",
    "alert_response_approvals",
    "alert_response_control_requests",
    "alert_playbook_executions",
    "alert_playbook_execution_approvals",
    "alert_playbook_execution_controls",
    "alert_playbook_step_receipts",
    "alert_playbook_execution_outbox",
    "alert_playbook_execution_event_projection",
    "alert_playbook_execution_state_projection",
    "alert_saved_views",
    "alert_saved_view_requests",
    "alert_saved_view_history",
    "alert_saved_view_outbox",
    "notification_rules",
    "notification_governance_requests",
    "notification_governance_history",
    "notification_governance_outbox",
    "notification_escalation_policies",
    "notification_silence_rules",
    "alert_notification_settings",
    "user_settings",
    "user_settings_history",
    "user_settings_outbox",
    "user_settings_requests",
    "probes",
    "probe_registry_history",
    "probe_registry_outbox",
    "probe_registry_requests",
    "threat_intel",
    "threat_intel_feeds",
    "threat_intel_event_outbox",
    "threat_intel_command_history",
    "threat_intel_command_requests",
    "tasks",
    "forensics_task_history",
    "forensics_task_outbox",
    "forensics_task_requests",
    "whitelist_entry_versions",
    "whitelist_event_outbox",
    "whitelist_command_requests",
    "whitelist_rule_effects",
    "whitelist_rule_projection",
    "dashboard_tasks",
    "dashboard_task_history",
    "dashboard_task_outbox",
    "dashboard_task_requests",
    "dashboard_task_execution_attempts",
    "dashboard_task_execution_receipts",
    "dashboard_task_event_inbox",
    "dashboard_task_compensation_requests",
    "dashboard_task_compensation_attempts",
    "dashboard_task_compensation_receipts",
    "dashboard_task_dlq_receipts",
    "data_quality_datasets",
    "data_quality_rules",
    "data_quality_baselines",
    "data_quality_watermarks",
    "data_quality_events",
    "data_quality_repairs",
    "data_quality_outbox",
    "data_quality_dataset_history",
    "data_quality_rule_history",
    "data_quality_command_requests",
    "data_quality_rule_evaluations",
    "data_quality_repair_history",
    "data_quality_repair_requests",
    "data_quality_flow_replay_projection",
    "data_quality_replay_projection_receipts",
)
SCHEMA_SNAPSHOT_QUERY = """
SELECT 'C|'||table_name||'|'||ordinal_position||'|'||column_name||'|'||
       data_type||'|'||is_nullable||'|'||COALESCE(column_default,'')
FROM information_schema.columns
WHERE table_schema='public' AND table_name IN ({tables})
UNION ALL
SELECT 'K|'||conrelid::regclass::text||'|'||conname||'|'||pg_get_constraintdef(oid)
FROM pg_constraint
WHERE conrelid IN ({regclasses})
UNION ALL
SELECT 'I|'||tablename||'|'||indexname||'|'||indexdef
FROM pg_indexes
WHERE schemaname='public' AND tablename IN ({tables})
ORDER BY 1
"""


def run(
    command: list[str],
    *,
    input_bytes: bytes | None = None,
    check: bool = True,
) -> subprocess.CompletedProcess[bytes]:
    completed = subprocess.run(
        command,
        cwd=ROOT,
        input=input_bytes,
        stdout=subprocess.PIPE,
        stderr=subprocess.PIPE,
        check=False,
    )
    if check and completed.returncode != 0:
        stdout = completed.stdout.decode(errors="replace").strip()
        stderr = completed.stderr.decode(errors="replace").strip()
        raise RuntimeError(
            f"command failed rc={completed.returncode}: {command!r}; "
            f"stdout={stdout!r}; stderr={stderr!r}"
        )
    return completed


def docker_psql(
    container: str,
    database: str,
    *,
    sql: str | None = None,
    script: bytes | None = None,
) -> bytes:
    command = [
        "docker",
        "exec",
        "-e",
        "PGOPTIONS=--client-min-messages=warning",
    ]
    if script is not None:
        command.append("-i")
    command.extend([container, "psql", "-v", "ON_ERROR_STOP=1", "-U", "postgres", "-d", database])
    if sql is not None:
        command.extend(["-Atqc", sql])
    completed = run(command, input_bytes=script)
    return completed.stdout


def exact_database(container: str, action: str, database: str) -> None:
    command = ["docker", "exec", container, action, "-U", "postgres"]
    if action == "dropdb":
        command.append("--if-exists")
    command.append(database)
    run(command)


def load_k8s_scripts() -> dict[str, bytes]:
    documents = list(yaml.safe_load_all(K8S_SCHEMA.read_text(encoding="utf-8")))
    config_map = next(
        (
            item
            for item in documents
            if isinstance(item, dict)
            and item.get("kind") == "ConfigMap"
            and item.get("metadata", {}).get("name") == "postgres-init-sql"
        ),
        None,
    )
    if not config_map:
        raise RuntimeError("postgres-init-sql ConfigMap is missing")
    data = config_map.get("data", {})
    missing = [name for name in K8S_FILES if name not in data]
    if missing:
        raise RuntimeError(f"Kubernetes PostgreSQL ConfigMap keys missing: {missing}")
    job = next(
        (
            item
            for item in documents
            if isinstance(item, dict)
            and item.get("kind") == "Job"
            and item.get("metadata", {}).get("name") == "init-postgres-schema"
        ),
        None,
    )
    if not job:
        raise RuntimeError("init-postgres-schema Job is missing")
    job_args = "\n".join(
        job["spec"]["template"]["spec"]["containers"][0].get("args", [])
    )
    ordered_names = " ".join(K8S_FILES)
    if ordered_names not in job_args:
        raise RuntimeError("Kubernetes init Job does not execute every schema key in canonical order")
    return {name: data[name].encode() for name in K8S_FILES}


def schema_snapshot(container: str, database: str) -> dict[str, Any]:
    table_literals = ",".join(f"'{name}'" for name in PLAYBOOK_TABLES)
    regclasses = ",".join(f"'{name}'::regclass" for name in PLAYBOOK_TABLES)
    query = SCHEMA_SNAPSHOT_QUERY.format(
        tables=table_literals,
        regclasses=regclasses,
    )
    payload = docker_psql(container, database, sql=query)
    column_count = int(
        docker_psql(
            container,
            database,
            sql=(
                "SELECT count(*) FROM information_schema.columns "
                f"WHERE table_schema='public' AND table_name IN ({table_literals})"
            ),
        ).decode().strip()
    )
    return {
        "sha256": hashlib.sha256(payload).hexdigest(),
        "bytes": len(payload),
        "columns": column_count,
        "tables": list(PLAYBOOK_TABLES),
        "_lines": payload.decode().splitlines(),
    }


def main() -> int:
    parser = argparse.ArgumentParser()
    parser.add_argument("--container", required=True)
    parser.add_argument("--run-id", required=True)
    args = parser.parse_args()

    if not re.fullmatch(r"codex-playbook-v2-pg-[A-Za-z0-9_.-]+", args.container):
        raise SystemExit("refusing container without codex-playbook-v2-pg sentinel prefix")
    sentinel = docker_psql(
        args.container,
        SENTINEL_DATABASE,
        sql=SENTINEL_QUERY,
    ).decode().strip()
    if sentinel != "ephemeral-only":
        raise SystemExit("refusing database without the ephemeral playbook sentinel")

    suffix = hashlib.sha256(args.run_id.encode()).hexdigest()[:10]
    databases = {
        "common": f"pb_common_{suffix}",
        "docker_merged": f"pb_merged_{suffix}",
        "kubernetes_configmap": f"pb_k8s_{suffix}",
    }
    common_files = sorted((ROOT / "common/sql/pg").glob("*.sql"))
    merged = (
        ROOT / "go/control-plane/deployments/docker/init/postgres_merged.sql"
    ).read_bytes()
    k8s_scripts = load_k8s_scripts()
    migrations = tuple(
        (ROOT / "deployments/postgres/migrations" / name).read_bytes()
        for name in (
            "202608021000_playbook_execution_v2.sql",
            "202608021030_playbook_event_pipeline_v2.sql",
            "202608031500_user_settings_transaction_v2.sql",
            "202608031540_probe_registry_atomic.sql",
            "202608031550_threat_intel_command_atomic.sql",
            "202608031600_forensics_task_command_atomic.sql",
            "202608031610_whitelist_governance_v2.sql",
            "202608031620_dashboard_task_v2.sql",
            "202608041100_alert_opensearch_projection_reconciliation_v1.sql",
            "202608041400_data_quality_control_plane_v1.sql",
            "202608041500_data_quality_governance_v1.sql",
            "202608041600_data_quality_rule_evaluation_v1.sql",
            "202608041700_data_quality_repair_lifecycle_v1.sql",
            "202608041800_data_quality_replay_projection_v1.sql",
            "202608041900_alert_report_cancellation_v2.sql",
            "202608041930_dashboard_task_execution_pipeline_v1.sql",
            "202608082100_dashboard_task_compensation_v1.sql",
            "202608090945_dashboard_task_dlq_receipt_v1.sql",
            "202608091130_alert_response_external_executor_v1.sql",
            "202608091230_alert_response_dlq_receipt_v1.sql",
        )
    )

    created: list[str] = []
    try:
        for database in databases.values():
            exact_database(args.container, "dropdb", database)
            exact_database(args.container, "createdb", database)
            created.append(database)

        # Migration replay must start from an explicit canonical baseline. A
        # previously reused sentinel container may already contain domain
        # tables, but a clean verifier container contains only the sentinel.
        for path in common_files:
            docker_psql(
                args.container,
                SENTINEL_DATABASE,
                script=path.read_bytes(),
            )

        for _ in range(2):
            for path in common_files:
                docker_psql(
                    args.container,
                    databases["common"],
                    script=path.read_bytes(),
                )
            docker_psql(
                args.container,
                databases["docker_merged"],
                script=merged,
            )
            for name in K8S_FILES:
                docker_psql(
                    args.container,
                    databases["kubernetes_configmap"],
                    script=k8s_scripts[name],
                )

        for _ in range(2):
            for migration in migrations:
                docker_psql(
                    args.container,
                    SENTINEL_DATABASE,
                    script=migration,
                )

        snapshots = {
            name: schema_snapshot(args.container, database)
            for name, database in databases.items()
        }
        digests = {item["sha256"] for item in snapshots.values()}
        if len(digests) != 1:
            baseline_name = "common"
            baseline_lines = snapshots[baseline_name]["_lines"]
            differences = {}
            for name, snapshot in snapshots.items():
                if name == baseline_name or snapshot["sha256"] == snapshots[baseline_name]["sha256"]:
                    continue
                differences[name] = list(
                    difflib.unified_diff(
                        baseline_lines,
                        snapshot["_lines"],
                        fromfile=baseline_name,
                        tofile=name,
                        lineterm="",
                    )
                )[:160]
            summary = {
                name: {key: value for key, value in snapshot.items() if key != "_lines"}
                for name, snapshot in snapshots.items()
            }
            raise RuntimeError(f"playbook schema entrypoints differ: {summary}; differences={differences}")
        for snapshot in snapshots.values():
            snapshot.pop("_lines", None)
        migration_versions = docker_psql(
            args.container,
            SENTINEL_DATABASE,
            sql=(
                "SELECT version FROM alignment_schema_migrations "
                "WHERE version IN ('202608021000','202608021030','202608031500','202608031540','202608031550','202608031600','202608031610','202608031620','202608041100','202608041400','202608041500','202608041600','202608041700','202608041800','202608041900','202608041930','202608082100','202608090945','202608091130','202608091230') ORDER BY version"
            ),
        ).decode().splitlines()
        if migration_versions != ["202608021000", "202608021030", "202608031500", "202608031540", "202608031550", "202608031600", "202608031610", "202608031620", "202608041100", "202608041400", "202608041500", "202608041600", "202608041700", "202608041800", "202608041900", "202608041930", "202608082100", "202608090945", "202608091130", "202608091230"]:
            raise RuntimeError("versioned transaction migrations were not registered")
        print(
            json.dumps(
                {
                    "result": "pass",
                    "run_id": args.run_id,
                    "passes_per_entrypoint": 2,
                    "migration_versions": migration_versions,
                    "snapshots": snapshots,
                    "temporary_databases_removed": list(databases.values()),
                    "shared_environment_touched": False,
                    "secrets_captured": False,
                },
                indent=2,
            )
        )
        return 0
    finally:
        for database in reversed(created):
            exact_database(args.container, "dropdb", database)


if __name__ == "__main__":
    raise SystemExit(main())
