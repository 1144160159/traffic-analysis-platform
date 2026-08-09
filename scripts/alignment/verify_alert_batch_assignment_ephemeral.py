#!/usr/bin/env python3
"""Run F-ALERT-004 G1 against an owned, disposable PostgreSQL container."""

from __future__ import annotations

import argparse
import hashlib
import json
import os
import socket
import subprocess
import time
from pathlib import Path
from typing import Any


ROOT = Path(__file__).resolve().parents[2]
POSTGRES_IMAGE = "postgres@sha256:33f923b05f64ca54ac4401c01126a6b92afe839a0aa0a52bc5aeb5cc958e5f20"
PASSWORD = "codex-alert-batch-ephemeral-only"
MEMORY_LIMIT = "512m"
CPU_LIMIT = "1"
DATA_TMPFS = "/var/lib/postgresql/data:rw,nosuid,nodev,size=384m"
SENTINEL_TABLE = "codex_ephemeral_alert_batch_sentinel"
SENTINEL_VALUE = "ephemeral-only"
PASS_MARKER = "alert_batch_assignment_postgres=pass"
EXECUTION_PASS_MARKER = "alert_batch_assignment_execution_postgres=pass"
TERMINAL_QUERY_PASS_MARKER = "alert_batch_assignment_terminal_query_postgres=pass"
COMPENSATION_EXECUTION_PASS_MARKER = "alert_batch_assignment_compensation_execution_postgres=pass"
COMPENSATION_TERMINAL_QUERY_PASS_MARKER = "alert_batch_assignment_compensation_terminal_query_postgres=pass"


def run(command: list[str], *, input_bytes: bytes | None = None, env: dict[str, str] | None = None, check: bool = True) -> subprocess.CompletedProcess[bytes]:
    return subprocess.run(command, cwd=ROOT, input=input_bytes, env=env, stdout=subprocess.PIPE, stderr=subprocess.STDOUT, check=check)


def free_loopback_port() -> int:
    with socket.socket(socket.AF_INET, socket.SOCK_STREAM) as listener:
        listener.bind(("127.0.0.1", 0))
        return int(listener.getsockname()[1])


def owned_name(run_id: str) -> str:
    if not run_id.strip():
        raise ValueError("run_id is required")
    return "codex-alert-batch-pg-" + hashlib.sha256(run_id.encode()).hexdigest()[:12]


def inspect_container(name: str) -> dict[str, Any] | None:
    completed = run(["docker", "container", "inspect", name], check=False)
    if completed.returncode != 0:
        return None
    return json.loads(completed.stdout)[0]


def container_absent(name: str) -> bool:
    return inspect_container(name) is None


def container_stats(name: str) -> dict[str, Any] | None:
    completed = run(["docker", "stats", "--no-stream", "--format", "{{json .}}", name], check=False)
    if completed.returncode != 0 or not completed.stdout.strip():
        return None
    raw = json.loads(completed.stdout)
    return {
        "cpu_percent": raw.get("CPUPerc"),
        "memory_usage": raw.get("MemUsage"),
        "memory_percent": raw.get("MemPerc"),
        "block_io": raw.get("BlockIO"),
        "pids": raw.get("PIDs"),
        "measurement": "post_workload_single_snapshot_not_performance_evidence",
    }


def psql(container: str, sql: str) -> str:
    return run(["docker", "exec", container, "psql", "-X", "-v", "ON_ERROR_STOP=1", "-U", "postgres", "-d", "traffic_platform", "-Atc", sql]).stdout.decode().strip()


def main() -> int:
    parser = argparse.ArgumentParser()
    parser.add_argument("--run-id", required=True)
    parser.add_argument("--output", type=Path)
    args = parser.parse_args()

    container = owned_name(args.run_id)
    port = free_loopback_port()
    result: dict[str, Any] = {
        "schema_version": 1,
        "run_id": args.run_id,
        "status": "FAIL",
        "coverage_status": "OWNED_REAL_POSTGRES_ASSIGNMENT_AND_COMPENSATION_PIPELINE_WITH_FAKE_CLICKHOUSE_AUTHORITY_G1",
        "production_applied": False,
        "shared_environment_touched": False,
        "loopback_only": True,
        "container_resource_limits": {"memory": MEMORY_LIMIT, "cpus": CPU_LIMIT},
        "container": {
            "name": container,
            "container_id": None,
            "image": POSTGRES_IMAGE,
            "image_id": None,
            "identity_verified": False,
            "cleanup_sentinel_verified": False,
            "data_mount_type": None,
            "persistent_volume_attached": False,
            "resource_snapshot": None,
            "removed": False,
        },
        "schema_entrypoints": [
            "go/control-plane/deployments/docker/init/postgres_merged.sql",
            "deployments/postgres/migrations/202608091900_alert_batch_assignment_v1.sql",
            "common/sql/pg/19-alert-batch-assignment-v1.sql",
            "deployments/postgres/migrations/202608092130_alert_batch_assignment_execution_v1.sql",
            "common/sql/pg/20-alert-batch-assignment-execution-v1.sql",
            "deployments/postgres/migrations/202608092300_alert_batch_assignment_compensation_v1.sql",
            "common/sql/pg/21-alert-batch-assignment-compensation-v1.sql",
        ],
        "schema_replay_count": 0,
        "test_output": "",
        "asserted_facts": {
            "server_side_selection_frozen": False,
            "selection_exact_idempotent_replay": False,
            "selection_token_plaintext_not_persisted": False,
            "changed_selection_payload_conflict": False,
            "cross_tenant_selection_rejected": False,
            "expired_selection_rejected": False,
            "consumed_selection_not_dispatched_twice": False,
            "assignment_exact_idempotent_replay": False,
            "changed_assignment_payload_conflict": False,
            "ordered_per_item_results_complete": False,
            "history_outbox_request_audit_same_trace": False,
            "audit_failure_full_rollback": False,
            "accepted_state_explicitly_non_final": False,
            "requested_and_changed_events_consumed_independently": False,
            "assignee_permission_and_item_revision_rechecked": False,
            "deterministic_projection_intents_and_receipts_persisted": False,
            "terminal_batch_and_item_accounting_complete": False,
            "terminal_status_query_returns_latest_outbox_and_item_receipts": False,
            "exact_kafka_redelivery_does_not_repeat_projection": False,
            "dlq_ack_source_tuple_barrier_idempotent": False,
            "pre_assignment_status_and_assignee_authority_captured": False,
            "compensation_exact_idempotent_replay": False,
            "compensation_changed_payload_conflict": False,
            "one_compensation_request_per_batch": False,
            "compensation_requested_and_changed_events_consumed_independently": False,
            "compensation_intervening_revision_not_overwritten": False,
            "compensation_terminal_item_receipts_complete": False,
            "compensation_exact_kafka_redelivery_does_not_repeat_projection": False,
            "all_data_mounts_tmpfs": False,
            "no_persistent_volume": False,
        },
        "errors": [],
        "secrets_captured": False,
    }
    created = False
    try:
        if not container_absent(container):
            raise RuntimeError(f"refusing to reuse existing container: {container}")
        image = json.loads(run(["docker", "image", "inspect", POSTGRES_IMAGE]).stdout)[0]
        result["container"]["image_id"] = image.get("Id")
        launched = run([
            "docker", "run", "--name", container,
            "--label", "traffic.remediation.owner=alert-batch-g1",
            "--label", f"traffic.remediation.run-id={args.run_id}",
            "--memory", MEMORY_LIMIT, "--cpus", CPU_LIMIT,
            "--tmpfs", DATA_TMPFS,
            "-e", f"POSTGRES_PASSWORD={PASSWORD}", "-e", "POSTGRES_DB=traffic_platform",
            "-p", f"127.0.0.1:{port}:5432", "-d", POSTGRES_IMAGE,
        ])
        result["container"]["container_id"] = launched.stdout.decode().strip()
        created = True
        for _ in range(30):
            if run(["docker", "exec", container, "pg_isready", "-U", "postgres", "-d", "traffic_platform"], check=False).returncode == 0:
                break
            time.sleep(1)
        else:
            raise RuntimeError("owned PostgreSQL did not become ready")

        inspected = inspect_container(container)
        if inspected is None:
            raise RuntimeError("owned PostgreSQL disappeared")
        volume_mounts = [mount for mount in inspected.get("Mounts") or [] if mount.get("Type") == "volume"]
        if volume_mounts:
            result["container"]["persistent_volume_attached"] = True
            raise RuntimeError("refusing persistent Docker volume")
        if "/var/lib/postgresql/data" not in (inspected.get("HostConfig", {}).get("Tmpfs") or {}):
            raise RuntimeError("PostgreSQL data directory is not tmpfs-backed")
        result["container"]["data_mount_type"] = "tmpfs"

        for index, schema_path in enumerate(result["schema_entrypoints"]):
            schema = ROOT / schema_path
            repeats = 1 if index == 0 else 2
            for _ in range(repeats):
                run(["docker", "exec", "-e", "PGOPTIONS=--client-min-messages=warning", "-i", container, "psql", "-X", "-v", "ON_ERROR_STOP=1", "-U", "postgres", "-d", "traffic_platform"], input_bytes=schema.read_bytes())
                result["schema_replay_count"] += 1

        psql(container, f"CREATE TABLE {SENTINEL_TABLE}(marker text primary key); INSERT INTO {SENTINEL_TABLE}(marker) VALUES ('{SENTINEL_VALUE}')")
        if psql(container, f"SELECT marker FROM {SENTINEL_TABLE}") != SENTINEL_VALUE:
            raise RuntimeError("owned PostgreSQL sentinel mismatch")

        test_env = os.environ.copy()
        test_env["GOSUMDB"] = test_env.get("TRAFFIC_GO_SUMDB") or "sum.golang.org"
        dsn = f"postgres://postgres:{PASSWORD}@127.0.0.1:{port}/traffic_platform?sslmode=disable"
        test_env["ALERT_BATCH_ASSIGNMENT_INTEGRATION_DSN"] = dsn
        test_env["ALERT_BATCH_ASSIGNMENT_EXECUTION_INTEGRATION_DSN"] = dsn
        acceptance = run(["go", "-C", "go/control-plane", "test", "./internal/alert/api", "-run", "^TestAlertBatchAssignmentPostgresIntegration$", "-count=1", "-v"], env=test_env, check=False)
        execution = run(["go", "-C", "go/control-plane", "test", "./internal/alert/consumer", "-run", "^TestAlertBatchAssignmentPipelinePostgresIntegration$", "-count=1", "-v"], env=test_env, check=False)
        terminal_query = run(["go", "-C", "go/control-plane", "test", "./internal/alert/api", "-run", "^TestAlertBatchAssignmentTerminalQueryPostgresIntegration$", "-count=1", "-v"], env=test_env, check=False)
        compensation_execution = run(["go", "-C", "go/control-plane", "test", "./internal/alert/consumer", "-run", "^TestAlertBatchAssignmentCompensationPipelinePostgresIntegration$", "-count=1", "-v"], env=test_env, check=False)
        compensation_terminal_query = run(["go", "-C", "go/control-plane", "test", "./internal/alert/api", "-run", "^TestAlertBatchAssignmentCompensationTerminalQueryPostgresIntegration$", "-count=1", "-v"], env=test_env, check=False)
        result["test_output"] = (acceptance.stdout + b"\n" + execution.stdout + b"\n" + terminal_query.stdout + b"\n" + compensation_execution.stdout + b"\n" + compensation_terminal_query.stdout).decode(errors="replace").strip()
        if acceptance.returncode != 0 or PASS_MARKER not in result["test_output"]:
            raise RuntimeError(f"alert batch PostgreSQL acceptance integration exited {acceptance.returncode}")
        if execution.returncode != 0 or EXECUTION_PASS_MARKER not in result["test_output"]:
            raise RuntimeError(f"alert batch PostgreSQL execution integration exited {execution.returncode}")
        if terminal_query.returncode != 0 or TERMINAL_QUERY_PASS_MARKER not in result["test_output"]:
            raise RuntimeError(f"alert batch PostgreSQL terminal-query integration exited {terminal_query.returncode}")
        if compensation_execution.returncode != 0 or COMPENSATION_EXECUTION_PASS_MARKER not in result["test_output"]:
            raise RuntimeError(f"alert batch PostgreSQL compensation execution integration exited {compensation_execution.returncode}")
        if compensation_terminal_query.returncode != 0 or COMPENSATION_TERMINAL_QUERY_PASS_MARKER not in result["test_output"]:
            raise RuntimeError(f"alert batch PostgreSQL compensation terminal-query integration exited {compensation_terminal_query.returncode}")
        result["asserted_facts"] = {key: True for key in result["asserted_facts"]}
        result["status"] = "PASS"
    except Exception as exc:
        result["errors"] = [str(exc)]
    finally:
        if created:
            result["container"]["resource_snapshot"] = container_stats(container)
            inspected = inspect_container(container)
            result["container"]["identity_verified"] = bool(inspected and inspected.get("Id") == result["container"]["container_id"] and inspected.get("Image") == result["container"]["image_id"])
            if result["container"]["identity_verified"]:
                marker = run(["docker", "exec", container, "psql", "-X", "-U", "postgres", "-d", "traffic_platform", "-Atc", f"SELECT marker FROM {SENTINEL_TABLE}"], check=False)
                result["container"]["cleanup_sentinel_verified"] = marker.returncode == 0 and marker.stdout.decode().strip() == SENTINEL_VALUE
            if result["container"]["identity_verified"] and result["container"]["cleanup_sentinel_verified"]:
                run(["docker", "rm", "-f", "-v", container], check=False)
            else:
                result["errors"].append("refusing cleanup after identity or sentinel drift")
                result["status"] = "FAIL"
        result["container"]["removed"] = container_absent(container)

    payload = json.dumps(result, ensure_ascii=False, indent=2) + "\n"
    if args.output:
        output = args.output.resolve()
        if output.exists():
            raise SystemExit(f"refusing to overwrite evidence: {output}")
        output.parent.mkdir(parents=True, exist_ok=True)
        output.write_text(payload, encoding="utf-8")
    print(payload, end="")
    passed = result["status"] == "PASS" and result["container"]["removed"] and result["container"]["identity_verified"] and result["container"]["cleanup_sentinel_verified"] and result["container"]["data_mount_type"] == "tmpfs" and not result["container"]["persistent_volume_attached"] and all(result["asserted_facts"].values())
    return 0 if passed else 1


if __name__ == "__main__":
    raise SystemExit(main())
