#!/usr/bin/env python3
"""Verify F-ALERT-006 transactions in an owned, disposable PostgreSQL."""

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
PASSWORD = "codex-alert-saved-view-ephemeral-only"
SENTINEL_TABLE = "codex_ephemeral_alert_saved_view_sentinel"
SENTINEL_VALUE = "ephemeral-only"
MEMORY_LIMIT = "512m"
CPU_LIMIT = "1"
DATA_TMPFS = "/var/lib/postgresql/data:rw,nosuid,nodev,size=384m"
PASS_MARKER = "alert_saved_view_postgres_transaction=pass"


def run(
    command: list[str],
    *,
    input_bytes: bytes | None = None,
    env: dict[str, str] | None = None,
    check: bool = True,
) -> subprocess.CompletedProcess[bytes]:
    return subprocess.run(
        command,
        cwd=ROOT,
        input=input_bytes,
        env=env,
        stdout=subprocess.PIPE,
        stderr=subprocess.STDOUT,
        check=check,
    )


def loopback_port() -> int:
    with socket.socket(socket.AF_INET, socket.SOCK_STREAM) as listener:
        listener.bind(("127.0.0.1", 0))
        return int(listener.getsockname()[1])


def container_name(run_id: str) -> str:
    if not run_id.strip():
        raise ValueError("run_id is required")
    return "codex-alert-saved-view-pg-" + hashlib.sha256(run_id.encode()).hexdigest()[:12]


def container_absent(name: str) -> bool:
    return run(["docker", "container", "inspect", name], check=False).returncode != 0


def container_inspect(name: str) -> dict[str, Any] | None:
    completed = run(["docker", "container", "inspect", name], check=False)
    if completed.returncode != 0:
        return None
    return json.loads(completed.stdout)[0]


def container_stats(name: str) -> dict[str, Any] | None:
    completed = run(
        ["docker", "stats", "--no-stream", "--format", "{{json .}}", name],
        check=False,
    )
    if completed.returncode != 0:
        return None
    payload = completed.stdout.decode(errors="replace").strip()
    if not payload:
        return None
    raw = json.loads(payload)
    return {
        "cpu_percent": raw.get("CPUPerc"),
        "memory_usage": raw.get("MemUsage"),
        "memory_percent": raw.get("MemPerc"),
        "block_io": raw.get("BlockIO"),
        "pids": raw.get("PIDs"),
        "measurement": "post_workload_single_snapshot_not_performance_evidence",
    }


def main() -> int:
    parser = argparse.ArgumentParser()
    parser.add_argument("--run-id", required=True)
    parser.add_argument("--output", type=Path)
    args = parser.parse_args()

    container = container_name(args.run_id)
    host_port = loopback_port()
    result: dict[str, Any] = {
        "schema_version": 1,
        "run_id": args.run_id,
        "status": "FAIL",
        "coverage_status": "OWNED_REAL_POSTGRES_ALERT_SAVED_VIEW_ATOMIC_REVISION_G1",
        "container": container,
        "container_id": None,
        "postgres_image": POSTGRES_IMAGE,
        "postgres_image_id": None,
        "schema_entrypoint": "go/control-plane/deployments/docker/init/postgres_merged.sql",
        "schema_files": 0,
        "sentinel_verified": False,
        "cleanup_sentinel_verified": False,
        "container_identity_verified": False,
        "container_resource_limits": {"memory": MEMORY_LIMIT, "cpus": CPU_LIMIT},
        "container_resource_snapshot": None,
        "loopback_only": True,
        "data_mount_type": None,
        "persistent_volume_attached": False,
        "shared_environment_touched": False,
        "production_applied": False,
        "container_removed": False,
        "test_output": "",
        "asserted_facts": {
            "create_revision_one": False,
            "exact_idempotent_replay": False,
            "changed_payload_conflict": False,
            "revisioned_update": False,
            "stale_revision_rejected": False,
            "cross_tenant_isolation": False,
            "audit_failure_full_rollback": False,
            "history_outbox_audit_same_trace": False,
        },
        "errors": [],
        "secrets_captured": False,
    }
    created = False
    try:
        if not container_absent(container):
            raise RuntimeError(f"refusing to reuse existing container: {container}")
        image = json.loads(run(["docker", "image", "inspect", POSTGRES_IMAGE]).stdout)[0]
        result["postgres_image_id"] = image.get("Id")
        created_result = run(
            [
                "docker", "run", "--name", container,
                "--memory", MEMORY_LIMIT, "--cpus", CPU_LIMIT,
                "--tmpfs", DATA_TMPFS,
                "-e", f"POSTGRES_PASSWORD={PASSWORD}",
                "-e", "POSTGRES_DB=traffic_platform",
                "-p", f"127.0.0.1:{host_port}:5432", "-d", POSTGRES_IMAGE,
            ]
        )
        result["container_id"] = created_result.stdout.decode().strip()
        created = True
        for _ in range(30):
            ready = run(
                [
                    "docker", "exec", container, "pg_isready", "-U", "postgres",
                    "-d", "traffic_platform",
                ],
                check=False,
            )
            if ready.returncode == 0:
                break
            time.sleep(1)
        else:
            raise RuntimeError("ephemeral PostgreSQL did not become ready")

        inspect = container_inspect(container)
        if inspect is None:
            raise RuntimeError("ephemeral PostgreSQL disappeared before schema load")
        mounts = inspect.get("Mounts") or []
        volume_mounts = [item for item in mounts if item.get("Type") == "volume"]
        if volume_mounts:
            result["persistent_volume_attached"] = True
            raise RuntimeError("refusing to continue with a persistent Docker volume")
        tmpfs = inspect.get("HostConfig", {}).get("Tmpfs") or {}
        if "/var/lib/postgresql/data" not in tmpfs:
            raise RuntimeError("owned PostgreSQL data directory is not tmpfs-backed")
        result["data_mount_type"] = "tmpfs"

        schema = ROOT / result["schema_entrypoint"]
        run(
            [
                "docker", "exec", "-e", "PGOPTIONS=--client-min-messages=warning", "-i",
                container, "psql", "-X", "-v", "ON_ERROR_STOP=1", "-U", "postgres",
                "-d", "traffic_platform",
            ],
            input_bytes=schema.read_bytes(),
        )
        result["schema_files"] = 1
        run(
            [
                "docker", "exec", container, "psql", "-X", "-v", "ON_ERROR_STOP=1",
                "-U", "postgres", "-d", "traffic_platform", "-c",
                f"CREATE TABLE {SENTINEL_TABLE}(marker text primary key); "
                f"INSERT INTO {SENTINEL_TABLE}(marker) VALUES ('{SENTINEL_VALUE}');",
            ]
        )
        marker = run(
            [
                "docker", "exec", container, "psql", "-X", "-U", "postgres",
                "-d", "traffic_platform", "-Atc",
                f"SELECT marker FROM {SENTINEL_TABLE} WHERE marker='{SENTINEL_VALUE}'",
            ]
        ).stdout.decode().strip()
        if marker != SENTINEL_VALUE:
            raise RuntimeError("ephemeral PostgreSQL sentinel mismatch")
        result["sentinel_verified"] = True

        test_env = os.environ.copy()
        test_env["GOSUMDB"] = test_env.get("TRAFFIC_GO_SUMDB") or "sum.golang.org"
        test_env["ALERT_SAVED_VIEW_EPHEMERAL_PG_DSN"] = (
            f"postgres://postgres:{PASSWORD}@127.0.0.1:{host_port}/"
            "traffic_platform?sslmode=disable"
        )
        completed = run(
            [
                "go", "-C", "go/control-plane", "test", "./internal/alert/api",
                "-run", "^TestAlertSavedViewPostgresIntegration$", "-count=1", "-v",
            ],
            env=test_env,
            check=False,
        )
        result["test_output"] = completed.stdout.decode(errors="replace").strip()
        if completed.returncode != 0 or PASS_MARKER not in result["test_output"]:
            raise RuntimeError(f"saved-view PostgreSQL integration exited {completed.returncode}")
        result["asserted_facts"] = {key: True for key in result["asserted_facts"]}
        result["status"] = "PASS"
    except Exception as exc:
        result["errors"] = [str(exc)]
    finally:
        if created:
            result["container_resource_snapshot"] = container_stats(container)
            inspect = container_inspect(container)
            result["container_identity_verified"] = bool(
                inspect
                and inspect.get("Id") == result["container_id"]
                and inspect.get("Image") == result["postgres_image_id"]
            )
            if result["container_identity_verified"]:
                marker_result = run(
                    [
                        "docker", "exec", container, "psql", "-X", "-U", "postgres",
                        "-d", "traffic_platform", "-Atc",
                        f"SELECT marker FROM {SENTINEL_TABLE} WHERE marker='{SENTINEL_VALUE}'",
                    ],
                    check=False,
                )
                result["cleanup_sentinel_verified"] = (
                    marker_result.returncode == 0
                    and marker_result.stdout.decode().strip() == SENTINEL_VALUE
                )
            if result["container_identity_verified"] and result["cleanup_sentinel_verified"]:
                run(["docker", "rm", "-f", "-v", container], check=False)
            else:
                result["errors"].append("refusing to remove PostgreSQL after identity or sentinel drift")
                result["status"] = "FAIL"
        result["container_removed"] = container_absent(container)

    payload = json.dumps(result, ensure_ascii=False, indent=2) + "\n"
    if args.output:
        output = args.output.resolve()
        if output.exists():
            raise SystemExit(f"refusing to overwrite saved-view PostgreSQL evidence: {output}")
        output.parent.mkdir(parents=True, exist_ok=True)
        output.write_text(payload, encoding="utf-8")
    print(payload, end="")
    scoped_pass = (
        result["status"] == "PASS"
        and result["sentinel_verified"]
        and result["cleanup_sentinel_verified"]
        and result["container_identity_verified"]
        and result["data_mount_type"] == "tmpfs"
        and not result["persistent_volume_attached"]
        and result["container_removed"]
        and all(result["asserted_facts"].values())
    )
    return 0 if scoped_pass else 1


if __name__ == "__main__":
    raise SystemExit(main())
