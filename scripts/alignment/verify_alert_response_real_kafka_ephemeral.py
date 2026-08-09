#!/usr/bin/env python3
"""Verify the alert-response DLQ precommit barrier on owned PG and Redpanda."""

from __future__ import annotations

import argparse
import hashlib
import json
import os
import re
import socket
import subprocess
import time
from pathlib import Path
from typing import Any


ROOT = Path(__file__).resolve().parents[2]
POSTGRES_IMAGE = "postgres@sha256:33f923b05f64ca54ac4401c01126a6b92afe839a0aa0a52bc5aeb5cc958e5f20"
KAFKA_IMAGE = "docker.io/redpandadata/redpanda@sha256:dca9d37efbbae3c2dcdc07d6a45fa1e0a7a541bc9cdc03db3937b80a4a9eae3d"
SOURCE_TOPIC = "alert.response.requested.v1"
DLQ_TOPIC = "dlq.v1"
SENTINEL_TABLE = "codex_ephemeral_alert_response_real_kafka_sentinel"
SENTINEL_VALUE = "alert-response-real-kafka-ephemeral-only"
PASSWORD = "codex-alert-response-real-kafka-ephemeral-only"
POSTGRES_MEMORY_LIMIT = "512m"
POSTGRES_CPU_LIMIT = "1"
REDPANDA_MEMORY_LIMIT = "1g"
REDPANDA_CPU_LIMIT = "2"
PASS_MARKER = "alert_response_real_kafka_dlq=pass"
GROUP_PATTERN = re.compile(r"\bgroup=(alert-response-real-kafka-[0-9a-f-]+)\b")


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


def container_names(run_id: str) -> tuple[str, str]:
    if not run_id.strip():
        raise ValueError("run_id is required")
    suffix = hashlib.sha256(run_id.encode()).hexdigest()[:12]
    return (
        f"codex-alert-response-real-kafka-pg-{suffix}",
        f"codex-alert-response-real-kafka-broker-{suffix}",
    )


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
        "network_io": raw.get("NetIO"),
        "block_io": raw.get("BlockIO"),
        "pids": raw.get("PIDs"),
        "measurement": "post_workload_single_snapshot_not_capacity_evidence",
    }


def parse_group_offset(description: str) -> dict[str, int] | None:
    for raw_line in description.splitlines():
        fields = raw_line.split()
        if len(fields) < 5 or fields[0] != SOURCE_TOPIC:
            continue
        try:
            return {
                "partition": int(fields[1]),
                "current_offset": int(fields[2]),
                "log_end_offset": int(fields[3]),
                "lag": int(fields[4]),
            }
        except ValueError:
            return None
    return None


def main() -> int:
    parser = argparse.ArgumentParser()
    parser.add_argument("--run-id", required=True)
    parser.add_argument("--output", type=Path)
    args = parser.parse_args()

    postgres_container, kafka_container = container_names(args.run_id)
    kafka_port = loopback_port()
    result: dict[str, Any] = {
        "schema_version": 1,
        "run_id": args.run_id,
        "status": "FAIL",
        "coverage_status": "OWNED_REAL_POSTGRES_REDPANDA_ALERT_RESPONSE_POISON_DLQ_PRECOMMIT_G1",
        "postgres_container": postgres_container,
        "kafka_container": kafka_container,
        "postgres_container_id": None,
        "kafka_container_id": None,
        "postgres_image": POSTGRES_IMAGE,
        "kafka_image": KAFKA_IMAGE,
        "postgres_image_id": None,
        "kafka_image_id": None,
        "source_topic": SOURCE_TOPIC,
        "source_topic_partitions": 0,
        "dlq_topic": DLQ_TOPIC,
        "dlq_topic_partitions": 0,
        "schema_files": 0,
        "postgres_sentinel_verified": False,
        "kafka_sentinel_verified": False,
        "required_acks_all_verified": False,
        "real_kafka_external_provider_execution_verified": False,
        "provider_single_effect_verified": False,
        "same_trace_pg_provider_audit_verified": False,
        "dlq_ack_failure_offset_retention_verified": False,
        "poison_redelivery_verified": False,
        "canonical_dlq_payload_verified": False,
        "source_offset_dlq_postgres_audit_verified": False,
        "broker_group_commit_verified": False,
        "broker_group": None,
        "broker_group_offset": None,
        "cleanup_identity_verified": {"postgres": False, "redpanda": False},
        "container_resource_limits": {
            "postgres": {"memory": POSTGRES_MEMORY_LIMIT, "cpus": POSTGRES_CPU_LIMIT},
            "redpanda": {"memory": REDPANDA_MEMORY_LIMIT, "cpus": REDPANDA_CPU_LIMIT},
        },
        "container_resource_snapshots": {"postgres": None, "redpanda": None},
        "single_broker_owned_preflight": True,
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
        postgres_image = json.loads(
            run(["docker", "image", "inspect", POSTGRES_IMAGE]).stdout
        )[0]
        kafka_image = json.loads(
            run(["docker", "image", "inspect", KAFKA_IMAGE]).stdout
        )[0]
        result["postgres_image_id"] = postgres_image.get("Id")
        result["kafka_image_id"] = kafka_image.get("Id")

        kafka_create = run(
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
        result["kafka_container_id"] = kafka_create.stdout.decode().strip()
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
                "ephemeral Redpanda did not become healthy: "
                + logs.stdout.decode(errors="replace")[-8192:]
            )
        for topic in (SOURCE_TOPIC, DLQ_TOPIC):
            run(
                [
                    "docker", "exec", kafka_container, "rpk", "topic", "create", topic,
                    "--brokers", "127.0.0.1:9092", "--partitions", "1", "--replicas", "1",
                    "-c", "cleanup.policy=delete", "-c", "retention.ms=3600000",
                ]
            )
        listing = run(
            ["docker", "exec", kafka_container, "rpk", "topic", "list", "--brokers", "127.0.0.1:9092"]
        ).stdout.decode()
        if SOURCE_TOPIC not in listing or DLQ_TOPIC not in listing:
            raise RuntimeError("ephemeral Redpanda topics are incomplete")
        result["source_topic_partitions"] = 1
        result["dlq_topic_partitions"] = 1
        result["kafka_sentinel_verified"] = True

        postgres_create = run(
            [
                "docker", "run", "--name", postgres_container,
                "--memory", POSTGRES_MEMORY_LIMIT, "--cpus", POSTGRES_CPU_LIMIT,
                "-e", f"POSTGRES_PASSWORD={PASSWORD}", "-e", "POSTGRES_DB=traffic_platform",
                "-p", "127.0.0.1::5432", "-d", POSTGRES_IMAGE,
            ]
        )
        result["postgres_container_id"] = postgres_create.stdout.decode().strip()
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
        test_env["ALERT_RESPONSE_REAL_KAFKA_EPHEMERAL_PG_DSN"] = (
            f"postgres://postgres:{PASSWORD}@127.0.0.1:{postgres_port}/traffic_platform?sslmode=disable"
        )
        test_env["ALERT_RESPONSE_REAL_KAFKA_EPHEMERAL_BROKER"] = f"127.0.0.1:{kafka_port}"
        test_env["ALERT_RESPONSE_REAL_KAFKA_EPHEMERAL_SENTINEL"] = SENTINEL_VALUE
        completed = run(
            [
                "go", "-C", "go/control-plane", "test", "./internal/alert/consumer",
                "-run", "^TestAlertResponseRealKafkaDLQBarrier$", "-count=1", "-v",
            ],
            env=test_env,
            check=False,
        )
        result["test_output"] = completed.stdout.decode(errors="replace").strip()
        if completed.returncode != 0 or PASS_MARKER not in result["test_output"]:
            raise RuntimeError(f"alert response real Kafka integration exited {completed.returncode}")
        match = GROUP_PATTERN.search(result["test_output"])
        if match is None:
            raise RuntimeError("alert response real Kafka group identity is missing")
        result["broker_group"] = match.group(1)
        group_description = run(
            [
                "docker", "exec", kafka_container, "rpk", "group", "describe",
                result["broker_group"], "--brokers", "127.0.0.1:9092",
            ]
        ).stdout.decode(errors="replace")
        offset = parse_group_offset(group_description)
        if offset != {"partition": 0, "current_offset": 2, "log_end_offset": 2, "lag": 0}:
            raise RuntimeError(f"unexpected alert response group offset: {offset!r}")
        result["broker_group_offset"] = offset
        result["broker_group_commit_verified"] = True
        result["required_acks_all_verified"] = True
        result["real_kafka_external_provider_execution_verified"] = True
        result["provider_single_effect_verified"] = True
        result["same_trace_pg_provider_audit_verified"] = True
        result["dlq_ack_failure_offset_retention_verified"] = True
        result["poison_redelivery_verified"] = True
        result["canonical_dlq_payload_verified"] = True
        result["source_offset_dlq_postgres_audit_verified"] = True
        result["status"] = "PASS"
    except Exception as exc:
        result["errors"] = [str(exc)]
    finally:
        if postgres_created:
            result["container_resource_snapshots"]["postgres"] = container_stats(postgres_container)
        if kafka_created:
            result["container_resource_snapshots"]["redpanda"] = container_stats(kafka_container)
        if postgres_created:
            inspect = container_inspect(postgres_container)
            result["cleanup_identity_verified"]["postgres"] = bool(
                inspect
                and inspect.get("Id") == result["postgres_container_id"]
                and inspect.get("Image") == result["postgres_image_id"]
            )
            if result["cleanup_identity_verified"]["postgres"]:
                run(["docker", "rm", "-f", postgres_container], check=False)
            else:
                result["errors"].append("refusing to remove PostgreSQL after identity drift")
                result["status"] = "FAIL"
        if kafka_created:
            inspect = container_inspect(kafka_container)
            result["cleanup_identity_verified"]["redpanda"] = bool(
                inspect
                and inspect.get("Id") == result["kafka_container_id"]
                and inspect.get("Image") == result["kafka_image_id"]
            )
            if result["cleanup_identity_verified"]["redpanda"]:
                run(["docker", "rm", "-f", kafka_container], check=False)
            else:
                result["errors"].append("refusing to remove Redpanda after identity drift")
                result["status"] = "FAIL"
        result["postgres_container_removed"] = container_absent(postgres_container)
        result["kafka_container_removed"] = container_absent(kafka_container)

    payload = json.dumps(result, ensure_ascii=False, indent=2) + "\n"
    if args.output:
        output = args.output.resolve()
        if output.exists():
            raise SystemExit(f"refusing to overwrite alert response real Kafka evidence: {output}")
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
