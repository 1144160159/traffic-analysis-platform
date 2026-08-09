#!/usr/bin/env python3
"""Verify dashboard tasks across owned PostgreSQL, Redpanda and HTTP providers."""

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
KAFKA_IMAGE = "docker.io/redpandadata/redpanda@sha256:dca9d37efbbae3c2dcdc07d6a45fa1e0a7a541bc9cdc03db3937b80a4a9eae3d"
TOPIC = "dashboard.task.events.v1"
SENTINEL_TABLE = "codex_ephemeral_dashboard_task_real_components_sentinel"
SENTINEL_VALUE = "dashboard-real-components-ephemeral-only"
PASSWORD = "codex-dashboard-real-components-ephemeral-only"
POSTGRES_MEMORY_LIMIT = "512m"
POSTGRES_CPU_LIMIT = "1"
REDPANDA_MEMORY_LIMIT = "1g"
REDPANDA_CPU_LIMIT = "2"


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


def names(run_id: str) -> tuple[str, str]:
    if not run_id.strip():
        raise ValueError("run_id is required")
    suffix = hashlib.sha256(run_id.encode()).hexdigest()[:12]
    return (
        f"codex-dashboard-real-components-pg-{suffix}",
        f"codex-dashboard-real-components-broker-{suffix}",
    )


def container_absent(name: str) -> bool:
    return run(["docker", "container", "inspect", name], check=False).returncode != 0


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
        "network_io": raw.get("NetIO"),
        "block_io": raw.get("BlockIO"),
        "pids": raw.get("PIDs"),
        "measurement": "post_workload_single_snapshot_not_capacity_evidence",
    }


def main() -> int:
    parser = argparse.ArgumentParser()
    parser.add_argument("--run-id", required=True)
    parser.add_argument("--output", type=Path)
    parser.add_argument(
        "--mode", choices=("correctness", "bounded-profile"), default="correctness"
    )
    parser.add_argument("--profile-result", type=Path)
    args = parser.parse_args()
    if args.mode == "bounded-profile" and args.profile_result is None:
        parser.error("--profile-result is required in bounded-profile mode")
    if args.mode == "correctness" and args.profile_result is not None:
        parser.error("--profile-result is only valid in bounded-profile mode")

    postgres_container, kafka_container = names(args.run_id)
    kafka_port = loopback_port()
    result: dict[str, Any] = {
        "schema_version": 1,
        "run_id": args.run_id,
        "status": "FAIL",
        "mode": args.mode,
        "coverage_status": (
            "OWNED_BOUNDED_POSTGRES_REDPANDA_HTTP_G4_PREFLIGHT_NOT_APPROVED_G4"
            if args.mode == "bounded-profile"
            else "OWNED_REAL_POSTGRES_REDPANDA_LOOPBACK_HTTP_PROVIDER_AUTHORITY_LOOKUP_G1"
        ),
        "postgres_container": postgres_container,
        "kafka_container": kafka_container,
        "postgres_image": POSTGRES_IMAGE,
        "kafka_image": KAFKA_IMAGE,
        "postgres_image_id": None,
        "kafka_image_id": None,
        "topic": TOPIC,
        "topic_partitions": 0,
        "schema_files": 0,
        "postgres_sentinel_verified": False,
        "kafka_sentinel_verified": False,
        "required_acks_all_verified": False,
        "durable_inbox_verified": False,
        "consumer_restart_replay_verified": False,
        "out_of_order_authority_rejection_verified": False,
        "http_execution_provider_verified": False,
        "http_compensation_provider_verified": False,
        "provider_idempotency_verified": False,
        "execution_transport_ambiguity_verified": False,
        "compensation_transport_ambiguity_verified": False,
        "execution_authority_lookup_recovery_verified": False,
        "compensation_authority_lookup_recovery_verified": False,
        "authority_lookup_atomic_audit_verified": False,
        "tenant_isolation_verified": False,
        "same_trace_reconciliation_verified": False,
        "bounded_profile_verified": False,
        "bounded_profile": None,
        "container_resource_limits": {
            "postgres": {"memory": POSTGRES_MEMORY_LIMIT, "cpus": POSTGRES_CPU_LIMIT},
            "redpanda": {"memory": REDPANDA_MEMORY_LIMIT, "cpus": REDPANDA_CPU_LIMIT},
        },
        "container_resource_snapshots": {"postgres": None, "redpanda": None},
        "loopback_only": True,
        "persistent_volume_attached": False,
        "shared_environment_touched": False,
        "production_applied": False,
        "postgres_container_removed": False,
        "kafka_container_removed": False,
        "test_output": "",
        "errors": [],
        "secrets_captured": False,
    }
    postgres_created = False
    kafka_created = False
    try:
        for container in (postgres_container, kafka_container):
            if not container_absent(container):
                raise RuntimeError(f"refusing to reuse existing container: {container}")
        postgres_image = json.loads(run(["docker", "image", "inspect", POSTGRES_IMAGE]).stdout)[0]
        kafka_image = json.loads(run(["docker", "image", "inspect", KAFKA_IMAGE]).stdout)[0]
        result["postgres_image_id"] = postgres_image.get("Id")
        result["kafka_image_id"] = kafka_image.get("Id")

        run(
            [
                "docker", "run", "--name", kafka_container,
                "--memory", REDPANDA_MEMORY_LIMIT, "--cpus", REDPANDA_CPU_LIMIT,
                "-p", f"127.0.0.1:{kafka_port}:19092", "-d", KAFKA_IMAGE,
                "redpanda", "start", "--mode", "dev-container", "--check=false",
                "--smp", "1", "--memory", "512M", "--reserve-memory", "0M",
                "--kafka-addr", "internal://0.0.0.0:9092,external://0.0.0.0:19092",
                "--advertise-kafka-addr",
                f"internal://127.0.0.1:9092,external://127.0.0.1:{kafka_port}",
                "--rpc-addr", "0.0.0.0:33145",
                "--advertise-rpc-addr", "127.0.0.1:33145",
            ]
        )
        kafka_created = True
        for _ in range(60):
            metadata = run(
                [
                    "docker", "exec", kafka_container, "rpk", "topic", "list",
                    "--brokers", "127.0.0.1:9092",
                ],
                check=False,
            )
            if metadata.returncode == 0:
                break
            time.sleep(1)
        else:
            logs = run(["docker", "logs", "--tail", "100", kafka_container], check=False)
            raise RuntimeError(
                "ephemeral Kafka did not become healthy: "
                + logs.stdout.decode(errors="replace")[-8192:]
            )
        run(
            [
                "docker", "exec", kafka_container, "rpk", "topic", "create", TOPIC,
                "--brokers", "127.0.0.1:9092", "--partitions", "1", "--replicas", "1",
                "-c", "cleanup.policy=delete", "-c", "retention.ms=3600000",
            ]
        )
        topic_listing = run(
            [
                "docker", "exec", kafka_container, "rpk", "topic", "list",
                "--brokers", "127.0.0.1:9092",
            ]
        ).stdout.decode()
        topic_description = run(
            [
                "docker", "exec", kafka_container, "rpk", "topic", "describe", TOPIC,
                "--brokers", "127.0.0.1:9092", "-p",
            ]
        ).stdout.decode()
        if TOPIC not in topic_listing or "0" not in topic_description:
            raise RuntimeError("ephemeral Kafka topic description is incomplete")
        result["topic_partitions"] = 1
        result["kafka_sentinel_verified"] = True

        run(
            [
                "docker", "run", "--name", postgres_container,
                "--memory", POSTGRES_MEMORY_LIMIT, "--cpus", POSTGRES_CPU_LIMIT,
                "-e", f"POSTGRES_PASSWORD={PASSWORD}", "-e", "POSTGRES_DB=traffic_platform",
                "-p", "127.0.0.1::5432", "-d", POSTGRES_IMAGE,
            ]
        )
        postgres_created = True
        for _ in range(30):
            ready = run(
                [
                    "docker", "exec", postgres_container, "pg_isready", "-U", "postgres",
                    "-d", "traffic_platform",
                ],
                check=False,
            )
            if ready.returncode == 0:
                break
            time.sleep(1)
        else:
            raise RuntimeError("ephemeral PostgreSQL did not become ready")

        schema_files = sorted((ROOT / "common/sql/pg").glob("*.sql"))
        for path in schema_files:
            run(
                [
                    "docker", "exec", "-e", "PGOPTIONS=--client-min-messages=warning", "-i",
                    postgres_container, "psql", "-X", "-v", "ON_ERROR_STOP=1", "-U", "postgres",
                    "-d", "traffic_platform",
                ],
                input_bytes=path.read_bytes(),
            )
        result["schema_files"] = len(schema_files)
        run(
            [
                "docker", "exec", postgres_container, "psql", "-X", "-v", "ON_ERROR_STOP=1",
                "-U", "postgres", "-d", "traffic_platform", "-c",
                f"CREATE TABLE {SENTINEL_TABLE}(marker text primary key); "
                f"INSERT INTO {SENTINEL_TABLE}(marker) VALUES ('{SENTINEL_VALUE}');",
            ]
        )
        result["postgres_sentinel_verified"] = True
        port_output = run(["docker", "port", postgres_container, "5432/tcp"]).stdout.decode().strip()
        postgres_port = port_output.rsplit(":", 1)[-1]
        if not postgres_port.isdigit():
            raise RuntimeError(f"invalid loopback PostgreSQL port mapping: {port_output!r}")

        test_env = os.environ.copy()
        test_env["GOSUMDB"] = test_env.get("TRAFFIC_GO_SUMDB") or "sum.golang.org"
        test_env["DASHBOARD_TASK_REAL_COMPONENTS_EPHEMERAL_PG_DSN"] = (
            f"postgres://postgres:{PASSWORD}@127.0.0.1:{postgres_port}/traffic_platform?sslmode=disable"
        )
        test_env["DASHBOARD_TASK_REAL_COMPONENTS_EPHEMERAL_KAFKA_BROKER"] = f"127.0.0.1:{kafka_port}"
        test_env["DASHBOARD_TASK_REAL_COMPONENTS_EPHEMERAL_SENTINEL"] = SENTINEL_VALUE
        test_name = "TestDashboardTaskRealComponents"
        if args.mode == "bounded-profile":
            profile_result = args.profile_result.resolve()
            if profile_result.exists():
                raise RuntimeError(
                    f"refusing to overwrite dashboard bounded profile: {profile_result}"
                )
            profile_result.parent.mkdir(parents=True, exist_ok=True)
            test_env["DASHBOARD_TASK_BOUNDED_PROFILE_RESULT"] = str(profile_result)
            test_name = "TestDashboardTaskBoundedPerformanceProfile"
        completed = run(
            [
                "go", "-C", "go/control-plane", "test", "./internal/alert/api",
                "-run", f"^{test_name}$", "-count=1", "-v",
            ],
            env=test_env,
            check=False,
        )
        result["test_output"] = completed.stdout.decode(errors="replace").strip()
        if completed.returncode != 0:
            raise RuntimeError(f"dashboard real-component integration exited {completed.returncode}")
        if args.mode == "bounded-profile":
            profile = json.loads(profile_result.read_text(encoding="utf-8"))
            if profile.get("status") != "PASS":
                raise RuntimeError("dashboard bounded profile did not pass its stop conditions")
            result["bounded_profile"] = profile
            result["bounded_profile_verified"] = True
            result["required_acks_all_verified"] = True
            result["durable_inbox_verified"] = True
            result["http_execution_provider_verified"] = True
            result["provider_idempotency_verified"] = True
            result["execution_transport_ambiguity_verified"] = True
        else:
            for field in (
                "required_acks_all_verified",
                "durable_inbox_verified",
                "consumer_restart_replay_verified",
                "out_of_order_authority_rejection_verified",
                "http_execution_provider_verified",
                "http_compensation_provider_verified",
                "provider_idempotency_verified",
                "execution_transport_ambiguity_verified",
                "compensation_transport_ambiguity_verified",
                "execution_authority_lookup_recovery_verified",
                "compensation_authority_lookup_recovery_verified",
                "authority_lookup_atomic_audit_verified",
                "tenant_isolation_verified",
                "same_trace_reconciliation_verified",
            ):
                result[field] = True
        result["status"] = "PASS"
    except Exception as exc:
        result["errors"] = [str(exc)]
    finally:
        if postgres_created:
            result["container_resource_snapshots"]["postgres"] = container_stats(
                postgres_container
            )
        if kafka_created:
            result["container_resource_snapshots"]["redpanda"] = container_stats(
                kafka_container
            )
        if postgres_created:
            run(["docker", "rm", "-f", postgres_container], check=False)
        if kafka_created:
            run(["docker", "rm", "-f", kafka_container], check=False)
        result["postgres_container_removed"] = container_absent(postgres_container)
        result["kafka_container_removed"] = container_absent(kafka_container)

    payload = json.dumps(result, ensure_ascii=False, indent=2) + "\n"
    if args.output:
        output = args.output.resolve()
        if output.exists():
            raise SystemExit(f"refusing to overwrite dashboard real-component evidence: {output}")
        output.parent.mkdir(parents=True, exist_ok=True)
        output.write_text(payload, encoding="utf-8")
    print(payload, end="")
    return 0 if (
        result["status"] == "PASS"
        and result["postgres_container_removed"]
        and result["kafka_container_removed"]
    ) else 1


if __name__ == "__main__":
    raise SystemExit(main())
