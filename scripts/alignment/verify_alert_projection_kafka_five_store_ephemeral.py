#!/usr/bin/env python3
"""Verify alert Kafka commits after owned Redis, CH, OS and PG receipts."""

from __future__ import annotations

import argparse
import hashlib
import json
import os
import re
import socket
import subprocess
import time
import urllib.error
import urllib.request
from pathlib import Path
from typing import Any


ROOT = Path(__file__).resolve().parents[2]
CLICKHOUSE_IMAGE = "docker.io/clickhouse/clickhouse-server@sha256:458f72c00e4abc80c2950d8070bc5723b538544d72ea4a6a58d9ff4f8fadf8d7"
POSTGRES_IMAGE = "postgres@sha256:33f923b05f64ca54ac4401c01126a6b92afe839a0aa0a52bc5aeb5cc958e5f20"
OPENSEARCH_IMAGE = "docker.io/opensearchproject/opensearch@sha256:466a49f379bb8889af29d615475e69b7b990898c6987d28470cd7105df9046ff"
REDIS_IMAGE = "docker.io/library/redis@sha256:6ab0b6e7381779332f97b8ca76193e45b0756f38d4c0dcda72dbb3c32061ab99"
KAFKA_IMAGE = "docker.io/redpandadata/redpanda@sha256:dca9d37efbbae3c2dcdc07d6a45fa1e0a7a541bc9cdc03db3937b80a4a9eae3d"
CLICKHOUSE_SCHEMA = Path("common/sql/ch/00-all-tables.sql")
POSTGRES_MIGRATION = Path("deployments/postgres/migrations/202608041100_alert_opensearch_projection_reconciliation_v1.sql")
ALERT_MAPPING = Path("common/opensearch/alerts-v2/mappings-component.json")
TOPIC = "detections.alert-projection-receipt.v1"
SENTINEL = "ephemeral-only"
CLICKHOUSE_USER = "codex_ephemeral"
CLICKHOUSE_PASSWORD = "codex-ephemeral-only"
POSTGRES_PASSWORD = "ephemeral-alert-kafka-only"


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


def names(run_id: str) -> tuple[str, str, str, str, str]:
    if not run_id.strip():
        raise ValueError("run_id is required")
    suffix = hashlib.sha256(run_id.encode()).hexdigest()[:12]
    return tuple(f"codex-alert-kafka-receipt-{kind}-{suffix}" for kind in ("ch", "pg", "os", "redis", "broker"))


def absent(name: str) -> bool:
    return run(["docker", "container", "inspect", name], check=False).returncode != 0


def mapped_port(container: str, port: str) -> int:
    output = run(["docker", "port", container, port]).stdout.decode().strip()
    value = output.rsplit(":", 1)[-1]
    if not value.isdigit():
        raise RuntimeError(f"invalid loopback port mapping for {container}: {output!r}")
    return int(value)


def loopback_port() -> int:
    with socket.socket(socket.AF_INET, socket.SOCK_STREAM) as listener:
        listener.bind(("127.0.0.1", 0))
        return int(listener.getsockname()[1])


def wait_tcp(port: int, timeout: float) -> None:
    deadline = time.monotonic() + timeout
    while time.monotonic() < deadline:
        try:
            with socket.create_connection(("127.0.0.1", port), timeout=1):
                return
        except OSError:
            time.sleep(0.5)
    raise RuntimeError(f"loopback port {port} did not become ready within {timeout}s")


def request(base_url: str, method: str, path: str, body: Any | None = None) -> dict[str, Any]:
    payload = None if body is None else json.dumps(body, separators=(",", ":")).encode()
    req = urllib.request.Request(base_url + path, data=payload, method=method, headers={"Content-Type": "application/json"})
    try:
        with urllib.request.urlopen(req, timeout=10) as response:
            content = response.read()
    except urllib.error.HTTPError as exc:
        content = exc.read()
        raise RuntimeError(
            f"OpenSearch {method} {path} returned {exc.code}: {content.decode(errors='replace')[:4096]}"
        ) from exc
    return json.loads(content) if content else {}


def clickhouse_bootstrap() -> tuple[bytes, str]:
    source_bytes = (ROOT / CLICKHOUSE_SCHEMA).read_bytes()
    source = source_bytes.decode()
    pattern = re.compile(
        r"CREATE TABLE IF NOT EXISTS traffic\.alerts_local ON CLUSTER traffic_cluster \(.*?\n"
        r"SETTINGS index_granularity = 8192;",
        re.DOTALL,
    )
    matches = pattern.findall(source)
    if len(matches) != 1:
        raise RuntimeError(f"expected one canonical alerts_local DDL, found {len(matches)}")
    alerts = matches[0].replace(
        "CREATE TABLE IF NOT EXISTS traffic.alerts_local ON CLUSTER traffic_cluster",
        "CREATE TABLE traffic.alerts",
        1,
    )
    alerts, count = re.subn(
        r"ENGINE = ReplicatedMergeTree\('[^']+', '\{replica\}'\)",
        "ENGINE = MergeTree",
        alerts,
        count=1,
    )
    if count != 1 or " ON CLUSTER " in alerts or "Replicated" in alerts:
        raise RuntimeError("canonical alerts DDL could not be made standalone")
    sql = "\n".join(
        [
            "CREATE DATABASE IF NOT EXISTS traffic;",
            alerts,
            "CREATE TABLE traffic.alerts_latest AS traffic.alerts ENGINE=ReplacingMergeTree(updated_at) ORDER BY (tenant_id,alert_id);",
            "CREATE MATERIALIZED VIEW traffic.mv_alerts_latest TO traffic.alerts_latest AS SELECT * FROM traffic.alerts;",
            "CREATE TABLE traffic.codex_ephemeral_alert_reconcile_sentinel(marker String) ENGINE=Memory;",
            f"INSERT INTO traffic.codex_ephemeral_alert_reconcile_sentinel VALUES ('{SENTINEL}');",
        ]
    ) + "\n"
    return sql.encode(), hashlib.sha256(source_bytes).hexdigest()


def main() -> int:
    parser = argparse.ArgumentParser()
    parser.add_argument("--run-id", required=True)
    parser.add_argument("--output", type=Path)
    args = parser.parse_args()
    containers = names(args.run_id)
    ch_container, pg_container, os_container, redis_container, kafka_container = containers
    kafka_port = loopback_port()
    result: dict[str, Any] = {
        "schema_version": 1,
        "run_id": args.run_id,
        "status": "FAIL",
        "containers": dict(zip(("clickhouse", "postgres", "opensearch", "redis", "kafka"), containers)),
        "images": {
            "clickhouse": CLICKHOUSE_IMAGE,
            "postgres": POSTGRES_IMAGE,
            "opensearch": OPENSEARCH_IMAGE,
            "redis": REDIS_IMAGE,
            "kafka": KAFKA_IMAGE,
        },
        "image_ids": {},
        "clickhouse_schema": CLICKHOUSE_SCHEMA.as_posix(),
        "clickhouse_schema_sha256": None,
        "postgres_migration": POSTGRES_MIGRATION.as_posix(),
        "alert_mapping": ALERT_MAPPING.as_posix(),
        "topic": TOPIC,
        "sentinels_verified": False,
        "production_alert_consumer_verified": False,
        "redis_dedup_verified": False,
        "clickhouse_authority_verified": False,
        "opensearch_projection_verified": False,
        "postgres_applied_receipt_verified": False,
        "broker_committed_offset_verified": False,
        "last_event_id_observer_verified": False,
        "cross_store_hash_and_version_verified": False,
        "receipt_failure_blocks_commit_unit_verified": False,
        "postgres_receipt_failure_offset_retained_verified": False,
        "same_group_restart_redelivery_verified": False,
        "retry_cross_store_convergence_verified": False,
        "redis_exact_event_replay_count_stable_verified": False,
        "source_version_hash_stable_across_restart_verified": False,
        "event_identity_collision_rejected_verified": False,
        "loopback_only": True,
        "persistent_volume_attached": False,
        "shared_environment_touched": False,
        "production_applied": False,
        "containers_removed": {},
        "test_output": "",
        "errors": [],
    }
    created: list[str] = []
    try:
        if any(not absent(container) for container in containers):
            raise RuntimeError("refusing to reuse an existing five-store container")
        for key, image in result["images"].items():
            result["image_ids"][key] = json.loads(run(["docker", "image", "inspect", image]).stdout)[0].get("Id")

        run([
            "docker", "run", "--name", ch_container, "--ulimit", "nofile=262144:262144",
            "-p", "127.0.0.1::9000", "-e", f"CLICKHOUSE_USER={CLICKHOUSE_USER}",
            "-e", f"CLICKHOUSE_PASSWORD={CLICKHOUSE_PASSWORD}", "-d", CLICKHOUSE_IMAGE,
        ])
        created.append(ch_container)
        ch_port = mapped_port(ch_container, "9000/tcp")
        wait_tcp(ch_port, 60)
        ch_sql, result["clickhouse_schema_sha256"] = clickhouse_bootstrap()
        for _ in range(60):
            bootstrap = run([
                "docker", "exec", "-i", ch_container, "clickhouse-client", "--user", CLICKHOUSE_USER,
                "--password", CLICKHOUSE_PASSWORD, "--multiquery",
            ], input_bytes=ch_sql, check=False)
            if bootstrap.returncode == 0:
                break
            time.sleep(1)
        else:
            raise RuntimeError("ClickHouse schema bootstrap failed: " + bootstrap.stdout.decode(errors="replace")[-4096:])

        run([
            "docker", "run", "--name", pg_container, "-e", f"POSTGRES_PASSWORD={POSTGRES_PASSWORD}",
            "-e", "POSTGRES_DB=traffic_platform", "-p", "127.0.0.1::5432", "-d", POSTGRES_IMAGE,
        ])
        created.append(pg_container)
        for _ in range(60):
            if run(["docker", "exec", pg_container, "pg_isready", "-U", "postgres", "-d", "traffic_platform"], check=False).returncode == 0:
                break
            time.sleep(1)
        else:
            raise RuntimeError("ephemeral PostgreSQL did not become ready")
        run([
            "docker", "exec", "-i", pg_container, "psql", "-X", "-v", "ON_ERROR_STOP=1",
            "-U", "postgres", "-d", "traffic_platform",
        ], input_bytes=(ROOT / POSTGRES_MIGRATION).read_bytes())
        run([
            "docker", "exec", pg_container, "psql", "-X", "-v", "ON_ERROR_STOP=1", "-U", "postgres",
            "-d", "traffic_platform", "-c",
            "CREATE TABLE codex_ephemeral_alert_projection_sentinel(marker text primary key); "
            "INSERT INTO codex_ephemeral_alert_projection_sentinel(marker) VALUES ('ephemeral-only');",
        ])
        pg_port = mapped_port(pg_container, "5432/tcp")

        run([
            "docker", "run", "--name", os_container, "-e", "discovery.type=single-node",
            "-e", "DISABLE_SECURITY_PLUGIN=true", "-e", "DISABLE_INSTALL_DEMO_CONFIG=true",
            "-e", "OPENSEARCH_JAVA_OPTS=-Xms512m -Xmx512m", "-p", "127.0.0.1::9200", "-d", OPENSEARCH_IMAGE,
        ])
        created.append(os_container)
        os_port = mapped_port(os_container, "9200/tcp")
        base_url = f"http://127.0.0.1:{os_port}"
        for _ in range(120):
            try:
                if request(base_url, "GET", "/").get("version", {}).get("number") == "2.14.0":
                    break
            except (OSError, RuntimeError, json.JSONDecodeError):
                pass
            time.sleep(1)
        else:
            raise RuntimeError("ephemeral OpenSearch did not become ready")
        request(base_url, "PUT", "/codex-ephemeral-alert-reconcile-sentinel", {"settings": {"number_of_replicas": 0}})
        request(base_url, "PUT", f"/codex-ephemeral-alert-reconcile-sentinel/_doc/{SENTINEL}?refresh=true", {"marker": SENTINEL})
        mapping = json.loads((ROOT / ALERT_MAPPING).read_text(encoding="utf-8"))
        request(base_url, "PUT", "/alerts-v2-000001", {
            "settings": {"number_of_shards": 1, "number_of_replicas": 0},
            "mappings": mapping["template"]["mappings"],
            "aliases": {"alerts-v2-write": {"is_write_index": True}, "alerts-v2-read": {}},
        })

        run(["docker", "run", "--name", redis_container, "-p", "127.0.0.1::6379", "-d", REDIS_IMAGE])
        created.append(redis_container)
        for _ in range(60):
            if run(["docker", "exec", redis_container, "redis-cli", "ping"], check=False).stdout.strip() == b"PONG":
                break
            time.sleep(1)
        else:
            raise RuntimeError("ephemeral Redis did not become ready")
        run(["docker", "exec", redis_container, "redis-cli", "SET", "codex:ephemeral:alert-consumer-sentinel", SENTINEL])
        redis_port = mapped_port(redis_container, "6379/tcp")

        run([
            "docker", "run", "--name", kafka_container, "-p", f"127.0.0.1:{kafka_port}:19092", "-d", KAFKA_IMAGE,
            "redpanda", "start", "--mode", "dev-container", "--check=false",
            "--kafka-addr", "internal://0.0.0.0:9092,external://0.0.0.0:19092",
            "--advertise-kafka-addr", f"internal://127.0.0.1:9092,external://127.0.0.1:{kafka_port}",
            "--rpc-addr", "0.0.0.0:33145", "--advertise-rpc-addr", "127.0.0.1:33145",
        ])
        created.append(kafka_container)
        for _ in range(60):
            if run(["docker", "exec", kafka_container, "rpk", "topic", "list", "--brokers", "127.0.0.1:9092"], check=False).returncode == 0:
                break
            time.sleep(1)
        else:
            raise RuntimeError("ephemeral Kafka did not become ready")
        run([
            "docker", "exec", kafka_container, "rpk", "topic", "create", TOPIC,
            "--brokers", "127.0.0.1:9092", "--partitions", "1", "--replicas", "1",
            "-c", "cleanup.policy=delete", "-c", "retention.ms=3600000",
        ])
        result["sentinels_verified"] = True

        env = os.environ.copy()
        env.update({
            "ALERT_PROJECTION_KAFKA_EPHEMERAL_CH_HOST": f"127.0.0.1:{ch_port}",
            "ALERT_PROJECTION_KAFKA_EPHEMERAL_CH_USER": CLICKHOUSE_USER,
            "ALERT_PROJECTION_KAFKA_EPHEMERAL_CH_PASSWORD": CLICKHOUSE_PASSWORD,
            "ALERT_PROJECTION_KAFKA_EPHEMERAL_PG_DSN":
                f"postgres://postgres:{POSTGRES_PASSWORD}@127.0.0.1:{pg_port}/traffic_platform?sslmode=disable",
            "ALERT_PROJECTION_KAFKA_EPHEMERAL_OS_URL": base_url,
            "ALERT_PROJECTION_KAFKA_EPHEMERAL_REDIS_ADDR": f"127.0.0.1:{redis_port}",
            "ALERT_PROJECTION_KAFKA_EPHEMERAL_BROKER": f"127.0.0.1:{kafka_port}",
            "ALERT_PROJECTION_KAFKA_EPHEMERAL_SENTINEL": SENTINEL,
        })
        integration = run([
            "go", "-C", "go/control-plane", "test", "./internal/alert/consumer",
            "-run", "^TestAlertProjectionReceiptRealKafka$", "-count=1", "-v",
        ], env=env, check=False)
        barrier = run([
            "go", "-C", "go/control-plane", "test", "./internal/alert/persistence",
            "-run", "^TestWriteBatchAppliedReceiptFailureBlocksCommit$", "-count=1", "-v",
        ], check=False)
        result["test_output"] = "\n".join([
            integration.stdout.decode(errors="replace").strip(),
            barrier.stdout.decode(errors="replace").strip(),
        ])
        if integration.returncode != 0 or barrier.returncode != 0:
            raise RuntimeError(
                f"alert Kafka receipt integration exited integration={integration.returncode} barrier={barrier.returncode}"
            )
        for field in (
            "production_alert_consumer_verified", "redis_dedup_verified", "clickhouse_authority_verified",
            "opensearch_projection_verified", "postgres_applied_receipt_verified",
            "broker_committed_offset_verified", "last_event_id_observer_verified",
            "cross_store_hash_and_version_verified", "receipt_failure_blocks_commit_unit_verified",
            "postgres_receipt_failure_offset_retained_verified", "same_group_restart_redelivery_verified",
            "retry_cross_store_convergence_verified",
            "redis_exact_event_replay_count_stable_verified",
            "source_version_hash_stable_across_restart_verified",
            "event_identity_collision_rejected_verified",
        ):
            result[field] = True
        result["status"] = "PASS"
    except Exception as exc:
        result["errors"] = [str(exc)]
    finally:
        for container in reversed(created):
            run(["docker", "rm", "-f", container], check=False)
        result["containers_removed"] = {container: absent(container) for container in containers}

    payload = json.dumps(result, ensure_ascii=False, indent=2) + "\n"
    if args.output:
        output = args.output.resolve()
        if output.exists():
            raise SystemExit(f"refusing to overwrite alert Kafka five-store evidence: {output}")
        output.parent.mkdir(parents=True, exist_ok=True)
        output.write_text(payload, encoding="utf-8")
    print(payload, end="")
    return 0 if result["status"] == "PASS" and all(result["containers_removed"].values()) else 1


if __name__ == "__main__":
    raise SystemExit(main())
