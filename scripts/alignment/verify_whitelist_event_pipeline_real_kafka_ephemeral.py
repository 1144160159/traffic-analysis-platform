#!/usr/bin/env python3
"""Verify the whitelist pipeline across owned PostgreSQL and real Kafka."""

from __future__ import annotations

import argparse
import hashlib
import json
import os
import socket
import subprocess
import time
from pathlib import Path


ROOT = Path(__file__).resolve().parents[2]
POSTGRES_IMAGE = "postgres:16"
KAFKA_IMAGE = "redpandadata/redpanda:v24.1.12"
TOPIC = "whitelist.events.v2"


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


def container_absent(name: str) -> bool:
    return run(["docker", "container", "inspect", name], check=False).returncode != 0


def main() -> int:
    parser = argparse.ArgumentParser()
    parser.add_argument("--run-id", required=True)
    args = parser.parse_args()

    suffix = hashlib.sha256(args.run_id.encode()).hexdigest()[:12]
    postgres_container = f"codex-whitelist-real-kafka-pg-{suffix}"
    kafka_container = f"codex-whitelist-real-kafka-broker-{suffix}"
    kafka_port = loopback_port()
    result: dict[str, object] = {
        "schema_version": 1,
        "run_id": args.run_id,
        "status": "FAIL",
        "postgres_container": postgres_container,
        "kafka_container": kafka_container,
        "postgres_image": POSTGRES_IMAGE,
        "kafka_image": KAFKA_IMAGE,
        "topic": TOPIC,
        "topic_partitions": 0,
        "schema_files": 0,
        "postgres_sentinel_verified": False,
        "kafka_sentinel_verified": False,
        "loopback_only": True,
        "shared_environment_touched": False,
        "persistent_volume_attached": False,
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
        run(["docker", "image", "inspect", POSTGRES_IMAGE])
        run(["docker", "image", "inspect", KAFKA_IMAGE])

        run(
            [
                "docker", "run", "--name", kafka_container,
                "-p", f"127.0.0.1:{kafka_port}:19092", "-d", KAFKA_IMAGE,
                "redpanda", "start", "--mode", "dev-container", "--check=false",
                "--kafka-addr", "internal://0.0.0.0:9092,external://0.0.0.0:19092",
                "--advertise-kafka-addr",
                f"internal://127.0.0.1:9092,external://127.0.0.1:{kafka_port}",
                "--rpc-addr", "0.0.0.0:33145",
                "--advertise-rpc-addr", "127.0.0.1:33145",
            ]
        )
        kafka_created = True
        for _ in range(60):
            health = run(
                [
                    "docker", "exec", kafka_container, "rpk", "cluster", "health",
                    "--brokers", "127.0.0.1:9092",
                ],
                check=False,
            )
            if health.returncode == 0 and b"Healthy: true" in health.stdout:
                break
            time.sleep(1)
        else:
            logs = run(["docker", "logs", "--tail", "80", kafka_container], check=False)
            raise RuntimeError(f"ephemeral Kafka did not become healthy: {logs.stdout.decode(errors='replace')}")
        run(
            [
                "docker", "exec", kafka_container, "rpk", "topic", "create", TOPIC,
                "--brokers", "127.0.0.1:9092", "--partitions", "1", "--replicas", "1",
                "-c", "cleanup.policy=delete", "-c", "retention.ms=3600000",
            ]
        )
        topic_description = run(
            [
                "docker", "exec", kafka_container, "rpk", "topic", "describe", TOPIC,
                "--brokers", "127.0.0.1:9092", "-p",
            ]
        ).stdout.decode()
        if TOPIC not in topic_description or "0" not in topic_description:
            raise RuntimeError("ephemeral Kafka topic description is incomplete")
        result["topic_partitions"] = 1
        result["kafka_sentinel_verified"] = True

        run(
            [
                "docker", "run", "--name", postgres_container,
                "-e", "POSTGRES_PASSWORD=codex-ephemeral-only",
                "-e", "POSTGRES_DB=traffic_platform",
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
                    postgres_container, "psql", "-v", "ON_ERROR_STOP=1", "-U", "postgres",
                    "-d", "traffic_platform",
                ],
                input_bytes=path.read_bytes(),
            )
        result["schema_files"] = len(schema_files)
        run(
            [
                "docker", "exec", postgres_container, "psql", "-v", "ON_ERROR_STOP=1",
                "-U", "postgres", "-d", "traffic_platform", "-c",
                "CREATE TABLE codex_ephemeral_whitelist_event_pipeline_sentinel(marker text primary key); "
                "INSERT INTO codex_ephemeral_whitelist_event_pipeline_sentinel(marker) VALUES ('ephemeral-only');",
            ]
        )
        result["postgres_sentinel_verified"] = True
        port_output = run(["docker", "port", postgres_container, "5432/tcp"]).stdout.decode().strip()
        postgres_port = port_output.rsplit(":", 1)[-1]
        if not postgres_port.isdigit():
            raise RuntimeError("could not resolve ephemeral PostgreSQL loopback port")

        test_env = os.environ.copy()
        test_env["WHITELIST_EVENT_PIPELINE_EPHEMERAL_PG_DSN"] = (
            f"postgres://postgres:codex-ephemeral-only@127.0.0.1:{postgres_port}/"
            "traffic_platform?sslmode=disable"
        )
        test_env["WHITELIST_EVENT_PIPELINE_EPHEMERAL_KAFKA_BROKER"] = f"127.0.0.1:{kafka_port}"
        test_env["WHITELIST_EVENT_PIPELINE_EPHEMERAL_KAFKA_SENTINEL"] = "ephemeral-only"
        completed = run(
            [
                "go", "-C", "go/control-plane", "test", "./internal/rules/consumer",
                "-run", "^TestWhitelistEventPipelineRealKafka$", "-count=1", "-v",
            ],
            env=test_env,
            check=False,
        )
        result["test_output"] = completed.stdout.decode(errors="replace").strip()
        if completed.returncode != 0:
            raise RuntimeError(f"real Kafka whitelist pipeline integration exited {completed.returncode}")
        result["status"] = "PASS"
    except Exception as exc:
        result["errors"] = [str(exc)]
    finally:
        if postgres_created:
            run(["docker", "rm", "-f", postgres_container], check=False)
        if kafka_created:
            run(["docker", "rm", "-f", kafka_container], check=False)
        result["postgres_container_removed"] = container_absent(postgres_container)
        result["kafka_container_removed"] = container_absent(kafka_container)

    print(json.dumps(result, ensure_ascii=False, indent=2))
    return 0 if (
        result["status"] == "PASS"
        and result["postgres_container_removed"]
        and result["kafka_container_removed"]
    ) else 1


if __name__ == "__main__":
    raise SystemExit(main())
