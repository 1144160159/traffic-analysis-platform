#!/usr/bin/env python3
"""Verify one bounded asset revision across seven owned ephemeral sources."""

from __future__ import annotations

import argparse
import hashlib
import json
import os
import socket
import subprocess
import tempfile
import time
import urllib.error
import urllib.request
from pathlib import Path
from typing import Any

import verify_asset_detail_clickhouse_ephemeral as clickhouse_guard
import verify_asset_export_minio_ephemeral as minio_guard
import verify_asset_projection_opensearch_ephemeral as opensearch_guard


ROOT = Path(__file__).resolve().parents[2]
POSTGRES_IMAGE = "postgres@sha256:33f923b05f64ca54ac4401c01126a6b92afe839a0aa0a52bc5aeb5cc958e5f20"
KAFKA_IMAGE = "docker.io/redpandadata/redpanda@sha256:dca9d37efbbae3c2dcdc07d6a45fa1e0a7a541bc9cdc03db3937b80a4a9eae3d"
MINIO_IMAGE = minio_guard.MINIO_IMAGE
META_IMAGE = "docker.io/vesoft/nebula-metad@sha256:6d72a76fd44a738d1353186d8f2a8d467752239f4f85030f56c1b53b657b21d8"
STORAGE_IMAGE = "docker.io/vesoft/nebula-storaged@sha256:29b9dccaecc339ed0e98b21575105eb45afc44afe84b8ca0cc9b0dca03c14fae"
GRAPH_IMAGE = "docker.io/vesoft/nebula-graphd@sha256:0457a789213499fbfa2a07fb01cbea174843e9617771536a5b67cd89d96bcbf9"
CLICKHOUSE_IMAGE = clickhouse_guard.IMAGE
OPENSEARCH_IMAGE = opensearch_guard.OPENSEARCH_IMAGE
TOPIC = "asset.events.v2"
SENTINEL_VALUE = "ephemeral-only"
PG_PASSWORD = "codex-seven-source-pg-only"
MINIO_ACCESS_KEY = "codexseven"
MINIO_SECRET_KEY = "codex-seven-source-minio-only"
MINIO_BUCKET = "asset-seven-source"


def run(
    command: list[str], *, input_bytes: bytes | None = None,
    env: dict[str, str] | None = None, check: bool = True,
) -> subprocess.CompletedProcess[bytes]:
    return subprocess.run(
        command, cwd=ROOT, input=input_bytes, env=env,
        stdout=subprocess.PIPE, stderr=subprocess.STDOUT, check=check,
    )


def names(run_id: str) -> dict[str, str]:
    if not run_id.strip():
        raise ValueError("run_id is required")
    suffix = hashlib.sha256(run_id.encode()).hexdigest()[:12]
    return {
        "network": f"codex-seven-source-net-{suffix}",
        "postgresql": f"codex-seven-source-pg-{suffix}",
        "kafka": f"codex-seven-source-kafka-{suffix}",
        "clickhouse": f"codex-seven-source-clickhouse-{suffix}",
        "opensearch": f"codex-seven-source-opensearch-{suffix}",
        "minio": f"codex-seven-source-minio-{suffix}",
        "nebula_meta": f"codex-seven-source-nebula-meta-{suffix}",
        "nebula_storage": f"codex-seven-source-nebula-storage-{suffix}",
        "nebula_graph": f"codex-seven-source-nebula-graph-{suffix}",
    }


def absent(kind: str, name: str) -> bool:
    return run(["docker", kind, "inspect", name], check=False).returncode != 0


def mapped_port(container: str, port: int) -> int:
    output = run(["docker", "port", container, f"{port}/tcp"]).stdout.decode().strip()
    value = output.rsplit(":", 1)[-1]
    if not value.isdigit():
        raise RuntimeError(f"invalid loopback mapping for {container}:{port}: {output!r}")
    return int(value)


def wait_tcp(port: int, timeout: float = 60) -> None:
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
    req = urllib.request.Request(
        base_url + path, data=payload, method=method,
        headers={"Content-Type": "application/json"},
    )
    try:
        with urllib.request.urlopen(req, timeout=10) as response:
            content = response.read()
    except urllib.error.HTTPError as exc:
        raise RuntimeError(
            f"OpenSearch {method} {path} returned {exc.code}: "
            + exc.read().decode(errors="replace")[:4096]
        ) from exc
    return json.loads(content) if content else {}


def docker_run(
    container: str, network: str, image: str,
    options: list[str], command: list[str] | None = None,
) -> None:
    run(["docker", "run", "--name", container, "--network", network, *options, "-d", image, *(command or [])])


def setup_postgres(container: str, network: str) -> int:
    docker_run(container, network, POSTGRES_IMAGE, [
        "-e", f"POSTGRES_PASSWORD={PG_PASSWORD}", "-e", "POSTGRES_DB=traffic_platform",
        "-p", "127.0.0.1::5432",
    ])
    port = mapped_port(container, 5432)
    for _ in range(60):
        ready = run(["docker", "exec", container, "pg_isready", "-U", "postgres", "-d", "traffic_platform"], check=False)
        if ready.returncode == 0:
            break
        time.sleep(1)
    else:
        raise RuntimeError("ephemeral PostgreSQL did not become ready")
    for path in sorted((ROOT / "common/sql/pg").glob("*.sql")):
        run([
            "docker", "exec", "-e", "PGOPTIONS=--client-min-messages=warning", "-i", container,
            "psql", "-X", "-v", "ON_ERROR_STOP=1", "-U", "postgres", "-d", "traffic_platform",
        ], input_bytes=path.read_bytes())
    run([
        "docker", "exec", container, "psql", "-X", "-v", "ON_ERROR_STOP=1", "-U", "postgres",
        "-d", "traffic_platform", "-c",
        "CREATE TABLE codex_ephemeral_seven_source_sentinel(marker text primary key); "
        "INSERT INTO codex_ephemeral_seven_source_sentinel VALUES ('ephemeral-only');",
    ])
    return port


def setup_kafka(container: str, network: str, external_port: int) -> None:
    docker_run(container, network, KAFKA_IMAGE, [
        "-p", f"127.0.0.1:{external_port}:19092",
    ], [
        "redpanda", "start", "--mode", "dev-container", "--check=false",
        "--kafka-addr", "internal://0.0.0.0:9092,external://0.0.0.0:19092",
        "--advertise-kafka-addr", f"internal://{container}:9092,external://127.0.0.1:{external_port}",
        "--rpc-addr", "0.0.0.0:33145", "--advertise-rpc-addr", f"{container}:33145",
    ])
    for _ in range(60):
        ready = run(["docker", "exec", container, "rpk", "topic", "list", "--brokers", "127.0.0.1:9092"], check=False)
        if ready.returncode == 0:
            break
        time.sleep(1)
    else:
        raise RuntimeError("ephemeral Kafka did not become ready")
    run([
        "docker", "exec", container, "rpk", "topic", "create", TOPIC,
        "--brokers", "127.0.0.1:9092", "--partitions", "1", "--replicas", "1",
        "-c", "cleanup.policy=delete", "-c", "retention.ms=3600000",
    ])


def setup_clickhouse(container: str, network: str) -> int:
    docker_run(container, network, CLICKHOUSE_IMAGE, [
        "--ulimit", "nofile=262144:262144", "-p", "127.0.0.1::9000",
        "-e", f"CLICKHOUSE_USER={clickhouse_guard.EPHEMERAL_USER}",
        "-e", f"CLICKHOUSE_PASSWORD={clickhouse_guard.EPHEMERAL_PASSWORD}",
    ])
    port = mapped_port(container, 9000)
    wait_tcp(port)
    sql, _ = clickhouse_guard.bootstrap_sql()
    deadline = time.monotonic() + 60
    while time.monotonic() < deadline:
        completed = run([
            "docker", "exec", "-i", container, "clickhouse-client",
            "--user", clickhouse_guard.EPHEMERAL_USER,
            "--password", clickhouse_guard.EPHEMERAL_PASSWORD, "--multiquery",
        ], input_bytes=sql.encode(), check=False)
        if completed.returncode == 0:
            return port
        time.sleep(1)
    raise RuntimeError("ephemeral ClickHouse schema bootstrap failed: " + completed.stdout.decode(errors="replace")[-4096:])


def setup_opensearch(container: str, network: str) -> tuple[int, str]:
    docker_run(container, network, OPENSEARCH_IMAGE, [
        "-e", "discovery.type=single-node", "-e", "DISABLE_SECURITY_PLUGIN=true",
        "-e", "DISABLE_INSTALL_DEMO_CONFIG=true", "-e", "OPENSEARCH_JAVA_OPTS=-Xms512m -Xmx512m",
        "-p", "127.0.0.1::9200",
    ])
    port = mapped_port(container, 9200)
    base_url = f"http://127.0.0.1:{port}"
    for _ in range(120):
        try:
            if request(base_url, "GET", "/").get("version", {}).get("number") == "2.14.0":
                break
        except (OSError, RuntimeError, json.JSONDecodeError):
            pass
        time.sleep(1)
    else:
        raise RuntimeError("ephemeral OpenSearch did not become ready")
    request(base_url, "PUT", f"/{opensearch_guard.SENTINEL_INDEX}", {"settings": {"number_of_replicas": 0}})
    request(base_url, "PUT", f"/{opensearch_guard.SENTINEL_INDEX}/_doc/{SENTINEL_VALUE}?refresh=true", {"marker": SENTINEL_VALUE})
    template = opensearch_guard.asset_template()
    request(base_url, "PUT", "/_index_template/assets-v2-template", template)
    request(base_url, "PUT", "/assets-v2-000001", {"aliases": {"assets-v2-write": {"is_write_index": True}, "assets-v2-read": {}}})
    return port, base_url


def setup_minio(container: str, network: str) -> int:
    docker_run(container, network, MINIO_IMAGE, [
        "-e", f"MINIO_ROOT_USER={MINIO_ACCESS_KEY}", "-e", f"MINIO_ROOT_PASSWORD={MINIO_SECRET_KEY}",
        "-p", "127.0.0.1::9000",
    ], ["server", "/data"])
    port = mapped_port(container, 9000)
    deadline = time.monotonic() + 60
    while time.monotonic() < deadline:
        try:
            with urllib.request.urlopen(f"http://127.0.0.1:{port}/minio/health/ready", timeout=2) as response:
                if response.status == 200:
                    return port
        except OSError:
            pass
        time.sleep(1)
    raise RuntimeError("ephemeral MinIO did not become ready")


def setup_nebula(scoped: dict[str, str], network: str) -> int:
    meta, storage, graph = scoped["nebula_meta"], scoped["nebula_storage"], scoped["nebula_graph"]
    docker_run(meta, network, META_IMAGE, ["-p", "127.0.0.1::9559"], [
        f"--meta_server_addrs={meta}:9559", f"--local_ip={meta}",
        "--port=9559", "--ws_ip=0.0.0.0", "--ws_http_port=19559", "--data_path=/data/meta", "--log_dir=/data/logs",
    ])
    wait_tcp(mapped_port(meta, 9559), 45)
    docker_run(storage, network, STORAGE_IMAGE, ["-p", "127.0.0.1::9779"], [
        f"--meta_server_addrs={meta}:9559", f"--local_ip={storage}",
        "--port=9779", "--ws_ip=0.0.0.0", "--ws_http_port=19779", "--data_path=/data/storage", "--log_dir=/data/logs",
    ])
    wait_tcp(mapped_port(storage, 9779), 45)
    docker_run(graph, network, GRAPH_IMAGE, ["-p", "127.0.0.1::9669"], [
        f"--meta_server_addrs={meta}:9559", f"--local_ip={graph}",
        "--port=9669", "--ws_ip=0.0.0.0", "--ws_http_port=19669", "--log_dir=/data/logs",
        "--enable_authorize=true", "--auth_type=password",
    ])
    port = mapped_port(graph, 9669)
    wait_tcp(port, 45)
    return port


def main() -> int:
    parser = argparse.ArgumentParser()
    parser.add_argument("--run-id", required=True)
    parser.add_argument("--output", type=Path)
    args = parser.parse_args()
    scoped = names(args.run_id)
    containers = [value for key, value in scoped.items() if key != "network"]
    result: dict[str, Any] = {
        "schema_version": 1, "run_id": args.run_id, "status": "FAIL",
        "network": scoped["network"], "containers": {key: value for key, value in scoped.items() if key != "network"},
        "images": {
            "postgresql": POSTGRES_IMAGE, "kafka": KAFKA_IMAGE, "clickhouse": CLICKHOUSE_IMAGE,
            "opensearch": OPENSEARCH_IMAGE, "minio": MINIO_IMAGE,
            "nebula_meta": META_IMAGE, "nebula_storage": STORAGE_IMAGE, "nebula_graph": GRAPH_IMAGE,
        },
        "image_ids": {}, "seven_sources": ["postgresql", "kafka", "clickhouse", "opensearch", "nebulagraph", "minio", "audit"],
        "production_asset_event_path_verified": False, "production_clickhouse_writer_verified": False,
        "minio_seeded_adapter_verified": False, "audit_reconciled": False, "oracle_status": None,
        "normalized_manifest_sha256": None, "oracle_report_sha256": None,
        "loopback_only": True, "persistent_volume_attached": False,
        "shared_environment_touched": False, "production_applied": False,
        "containers_removed": False, "network_removed": False, "test_output": "", "errors": [],
        "evidence_boundary": (
            "G1 owned integration. PostgreSQL/outbox/Kafka/inbox/OpenSearch/NebulaGraph use the production asset path; "
            "ClickHouse uses the production alert writer with the same identity; MinIO is explicitly seeded. "
            "This does not prove candidate deployment, Flink, production G3, performance, browser, rollback or observation."
        ),
    }
    created: list[str] = []
    network_created = False
    try:
        if not absent("network", scoped["network"]):
            raise RuntimeError(f"refusing to reuse existing network: {scoped['network']}")
        for container in containers:
            if not absent("container", container):
                raise RuntimeError(f"refusing to reuse existing container: {container}")
        for label, image in result["images"].items():
            inspected = json.loads(run(["docker", "image", "inspect", image]).stdout)[0]
            result["image_ids"][label] = inspected.get("Id")
        run(["docker", "network", "create", "--driver", "bridge", scoped["network"]])
        network_created = True

        kafka_port = 0
        with socket.socket(socket.AF_INET, socket.SOCK_STREAM) as listener:
            listener.bind(("127.0.0.1", 0))
            kafka_port = int(listener.getsockname()[1])
        setup_kafka(scoped["kafka"], scoped["network"], kafka_port)
        created.append(scoped["kafka"])
        pg_port = setup_postgres(scoped["postgresql"], scoped["network"])
        created.append(scoped["postgresql"])
        ch_port = setup_clickhouse(scoped["clickhouse"], scoped["network"])
        created.append(scoped["clickhouse"])
        _, os_url = setup_opensearch(scoped["opensearch"], scoped["network"])
        created.append(scoped["opensearch"])
        minio_port = setup_minio(scoped["minio"], scoped["network"])
        created.append(scoped["minio"])
        nebula_port = setup_nebula(scoped, scoped["network"])
        created.extend([scoped["nebula_meta"], scoped["nebula_storage"], scoped["nebula_graph"]])

        with tempfile.TemporaryDirectory(prefix="codex-seven-source-") as temp_dir:
            manifest_path = Path(temp_dir) / "manifest.json"
            report_path = Path(temp_dir) / "report.json"
            test_env = {
                **os.environ,
                "ASSET_SEVEN_SOURCE_SENTINEL": SENTINEL_VALUE,
                "ASSET_SEVEN_SOURCE_MANIFEST": str(manifest_path),
                "ASSET_SEVEN_SOURCE_PG_DSN": f"postgres://postgres:{PG_PASSWORD}@127.0.0.1:{pg_port}/traffic_platform?sslmode=disable",
                "ASSET_SEVEN_SOURCE_KAFKA_BROKER": f"127.0.0.1:{kafka_port}",
                "ASSET_SEVEN_SOURCE_OS_URL": os_url,
                "ASSET_SEVEN_SOURCE_CLICKHOUSE_HOST": f"127.0.0.1:{ch_port}",
                "ASSET_SEVEN_SOURCE_CLICKHOUSE_USER": clickhouse_guard.EPHEMERAL_USER,
                "ASSET_SEVEN_SOURCE_CLICKHOUSE_PASSWORD": clickhouse_guard.EPHEMERAL_PASSWORD,
                "ASSET_SEVEN_SOURCE_MINIO_ENDPOINT": f"127.0.0.1:{minio_port}",
                "ASSET_SEVEN_SOURCE_MINIO_ACCESS_KEY": MINIO_ACCESS_KEY,
                "ASSET_SEVEN_SOURCE_MINIO_SECRET_KEY": MINIO_SECRET_KEY,
                "ASSET_SEVEN_SOURCE_MINIO_BUCKET": MINIO_BUCKET,
                "ASSET_SEVEN_SOURCE_NEBULA_ADDRESS": f"127.0.0.1:{nebula_port}",
                "ASSET_SEVEN_SOURCE_NEBULA_STORAGE_HOST": scoped["nebula_storage"],
            }
            completed = run([
                "go", "-C", "go/control-plane", "test", "./internal/asset/consumer",
                "-run", "^TestAssetSevenSourceTraceReconciliation$", "-count=1", "-v",
            ], env=test_env, check=False)
            result["test_output"] = completed.stdout.decode(errors="replace").strip()
            if completed.returncode != 0:
                raise RuntimeError(f"seven-source Go integration exited {completed.returncode}")
            if not manifest_path.is_file():
                raise RuntimeError("seven-source integration did not emit its normalized manifest")
            manifest_bytes = manifest_path.read_bytes()
            result["normalized_manifest_sha256"] = hashlib.sha256(manifest_bytes).hexdigest()
            oracle = run([
                "python3", "scripts/alignment/cross_store_reconcile.py",
                "--input", str(manifest_path), "--output", str(report_path), "--max-records", "7",
            ], check=False)
            if oracle.returncode != 0:
                raise RuntimeError("seven-source reconciliation oracle failed: " + oracle.stdout.decode(errors="replace")[-4096:])
            report = json.loads(report_path.read_text(encoding="utf-8"))
            if report.get("status") != "PASS" or any(report.get("counts", {}).values()):
                raise RuntimeError(f"unexpected seven-source oracle report: {report}")
            result["oracle_status"] = report["status"]
            result["oracle_report_sha256"] = hashlib.sha256(report_path.read_bytes()).hexdigest()

        result["production_asset_event_path_verified"] = True
        result["production_clickhouse_writer_verified"] = True
        result["minio_seeded_adapter_verified"] = True
        result["audit_reconciled"] = True
        result["status"] = "PASS"
    except Exception as exc:
        result["errors"] = [str(exc)]
        for container in created:
            logs = run(["docker", "logs", "--tail", "60", container], check=False)
            if logs.stdout:
                result.setdefault("container_log_tails", {})[container] = logs.stdout.decode(errors="replace")[-4096:]
    finally:
        for container in reversed(containers):
            run(["docker", "rm", "-f", container], check=False)
        if network_created:
            run(["docker", "network", "rm", scoped["network"]], check=False)
        result["containers_removed"] = all(absent("container", container) for container in containers)
        result["network_removed"] = absent("network", scoped["network"])

    payload = json.dumps(result, indent=2, ensure_ascii=False) + "\n"
    if args.output:
        output = args.output.resolve()
        if output.exists():
            raise SystemExit(f"refusing to overwrite seven-source G1 evidence: {output}")
        output.parent.mkdir(parents=True, exist_ok=True)
        output.write_text(payload, encoding="utf-8")
    print(payload, end="")
    return 0 if result["status"] == "PASS" and result["containers_removed"] and result["network_removed"] else 1


if __name__ == "__main__":
    raise SystemExit(main())
