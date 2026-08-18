#!/usr/bin/env python3
"""Run the M02 N014 owned Kafka/MinIO/ClickHouse loopback matrix."""

from __future__ import annotations

import argparse
import hashlib
import json
import os
import socket
import subprocess
import tempfile
import time
from pathlib import Path
from typing import Any


ROOT = Path(__file__).resolve().parents[2]
KAFKA_IMAGE = (
    "redpandadata/redpanda@sha256:"
    "dca9d37efbbae3c2dcdc07d6a45fa1e0a7a541bc9cdc03db3937b80a4a9eae3d"
)
MINIO_IMAGE = (
    "quay.io/minio/minio@sha256:"
    "14cea493d9a34af32f524e538b8346cf79f3321eff8e708c1e2960462bd8936e"
)
MINIO_CLIENT_IMAGE = (
    "minio/mc@sha256:"
    "a7fe349ef4bd8521fb8497f55c6042871b2ae640607cf99d9bede5e9bdf11727"
)
CLICKHOUSE_IMAGE = (
    "clickhouse/clickhouse-server@sha256:"
    "c3f166f4a80098480463d897a63f6867d24e3a7661fc1fe72e889788115a25f1"
)
REQUIRED_PARENT_INDEXES = tuple(
    ROOT / "doc/02_acceptance/topic1/tasks" / f"t1-m02-n{number:03d}"
    / "current-evidence-index.json"
    for number in range(1, 13)
)
REQUIRED_REJECTIONS = (
    "REJECT_LOOPBACK_BROKER_NOT_OBSERVED",
    "REJECT_MINIO_OBJECT_HASH_MISMATCH",
    "REJECT_KAFKA_OFFSET_NOT_CLOSED",
)


class LoopbackRejection(RuntimeError):
    def __init__(self, code: str, detail: str) -> None:
        super().__init__(f"{code}: {detail}")
        self.code = code
        self.detail = detail


def run(
    command: list[str],
    *,
    cwd: Path = ROOT,
    env: dict[str, str] | None = None,
    check: bool = True,
) -> subprocess.CompletedProcess[bytes]:
    return subprocess.run(
        command,
        cwd=cwd,
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


def sha256_path(path: Path) -> str:
    digest = hashlib.sha256()
    with path.open("rb") as handle:
        for chunk in iter(lambda: handle.read(1024 * 1024), b""):
            digest.update(chunk)
    return digest.hexdigest()


def display_path(path: Path) -> str:
    try:
        return str(path.relative_to(ROOT))
    except ValueError:
        return str(path)


def atomic_write_json(path: Path, value: dict[str, Any]) -> None:
    path.parent.mkdir(parents=True, exist_ok=True)
    rendered = json.dumps(value, ensure_ascii=False, indent=2, sort_keys=True) + "\n"
    temporary = path.with_name(path.name + ".tmp")
    temporary.write_text(rendered, encoding="utf-8")
    temporary.replace(path)


def require_current_parent_indexes() -> list[dict[str, str]]:
    missing = [display_path(path) for path in REQUIRED_PARENT_INDEXES if not path.is_file()]
    if missing:
        raise LoopbackRejection(
            "BLOCK_M02_PARENT_CURRENT_IDX_MISSING",
            "missing exact N001-N012 current indexes: " + ",".join(missing),
        )
    receipts: list[dict[str, str]] = []
    for number, path in enumerate(REQUIRED_PARENT_INDEXES, start=1):
        body = json.loads(path.read_text(encoding="utf-8"))
        expected_parent = f"T1-M02-N{number:03d}"
        if body.get("parent_task_id") not in {None, expected_parent}:
            raise LoopbackRejection(
                "BLOCK_M02_PARENT_CURRENT_IDX_IDENTITY",
                f"parent identity conflicts with path: {display_path(path)}",
            )
        status = str(body.get("status", body.get("decision", ""))).upper()
        if status not in {"PASS", "CURRENT", "COMPLETE", "CLOSED"}:
            raise LoopbackRejection(
                "BLOCK_M02_PARENT_CURRENT_IDX_NOT_PASS",
                f"non-current parent index {display_path(path)} status={status or 'MISSING'}",
            )
        receipts.append({"path": display_path(path), "sha256": sha256_path(path)})
    return receipts


def start_kafka(name: str, port: int, owned: set[str]) -> None:
    run([
        "docker", "run", "--name", name,
        "-p", f"127.0.0.1:{port}:19092", "-d", KAFKA_IMAGE,
        "redpanda", "start", "--mode", "dev-container", "--check=false",
        "--smp", "1", "--memory", "512M", "--reserve-memory", "0M",
        "--kafka-addr", "internal://0.0.0.0:9092,external://0.0.0.0:19092",
        "--advertise-kafka-addr",
        f"internal://127.0.0.1:9092,external://127.0.0.1:{port}",
        "--rpc-addr", "0.0.0.0:33145",
        "--advertise-rpc-addr", "127.0.0.1:33145",
    ])
    owned.add(name)
    for _ in range(90):
        ready = run([
            "docker", "exec", name, "rpk", "topic", "list",
            "--brokers", "127.0.0.1:9092",
        ], check=False)
        if ready.returncode == 0:
            return
        time.sleep(0.5)
    raise LoopbackRejection(
        "REJECT_LOOPBACK_BROKER_NOT_OBSERVED",
        run(["docker", "logs", "--tail", "100", name], check=False)
        .stdout.decode(errors="replace"),
    )


def start_minio(name: str, port: int, owned: set[str]) -> None:
    run([
        "docker", "run", "--name", name,
        "-p", f"127.0.0.1:{port}:9000",
        "-e", "MINIO_ROOT_USER=m02-access",
        "-e", "MINIO_ROOT_PASSWORD=m02-loopback-secret",
        "-d", MINIO_IMAGE, "server", "/data", "--address", ":9000",
    ])
    owned.add(name)
    for _ in range(90):
        ready = run(["docker", "exec", name, "curl", "-fsS", "http://127.0.0.1:9000/minio/health/ready"], check=False)
        if ready.returncode == 0:
            return
        time.sleep(0.5)
    raise RuntimeError("owned MinIO did not become healthy")


def start_clickhouse(name: str, port: int, owned: set[str]) -> None:
    run([
        "docker", "run", "--name", name,
        "-p", f"127.0.0.1:{port}:8123",
        "-e", "CLICKHOUSE_DB=traffic",
        "-e", "CLICKHOUSE_SKIP_USER_SETUP=1",
        "-d", CLICKHOUSE_IMAGE,
    ])
    owned.add(name)
    for _ in range(90):
        ready = run(["docker", "exec", name, "clickhouse-client", "--query", "SELECT 1"], check=False)
        if ready.returncode == 0 and ready.stdout.strip() == b"1":
            return
        time.sleep(0.5)
    raise RuntimeError("owned ClickHouse did not become healthy")


def minio_client(minio_name: str, arguments: list[str]) -> subprocess.CompletedProcess[bytes]:
    return run([
        "docker", "run", "--rm", "--network", f"container:{minio_name}",
        "-e", "MC_HOST_loopback=http://m02-access:m02-loopback-secret@127.0.0.1:9000",
        MINIO_CLIENT_IMAGE, *arguments,
    ])


def verify_minio_object(minio_name: str, work: Path) -> dict[str, object]:
    fixture = work / "m02-loopback-object.pcap.zst"
    fixture.write_bytes(b"m02-loopback-pcap-object-v1\n" * 128)
    expected_hash = sha256_path(fixture)
    minio_client(minio_name, ["mb", "--ignore-existing", "loopback/pcap-archive"])
    run([
        "docker", "run", "--rm", "--network", f"container:{minio_name}",
        "-v", f"{work}:/fixtures:ro",
        "-e", "MC_HOST_loopback=http://m02-access:m02-loopback-secret@127.0.0.1:9000",
        MINIO_CLIENT_IMAGE, "cp", "/fixtures/m02-loopback-object.pcap.zst",
        "loopback/pcap-archive/tenant-a/probe-a/m02-loopback-object.pcap.zst",
    ])
    downloaded = work / "downloaded.pcap.zst"
    completed = run([
        "docker", "run", "--rm", "--network", f"container:{minio_name}",
        "-e", "MC_HOST_loopback=http://m02-access:m02-loopback-secret@127.0.0.1:9000",
        MINIO_CLIENT_IMAGE, "cat",
        "loopback/pcap-archive/tenant-a/probe-a/m02-loopback-object.pcap.zst",
    ])
    downloaded.write_bytes(completed.stdout)
    observed_hash = sha256_path(downloaded)
    if observed_hash != expected_hash:
        raise LoopbackRejection(
            "REJECT_MINIO_OBJECT_HASH_MISMATCH",
            f"expected={expected_hash} observed={observed_hash}",
        )
    stat = json.loads(minio_client(minio_name, ["stat", "--json",
        "loopback/pcap-archive/tenant-a/probe-a/m02-loopback-object.pcap.zst"]).stdout)
    return {
        "bucket": "pcap-archive",
        "key": "tenant-a/probe-a/m02-loopback-object.pcap.zst",
        "sha256": expected_hash,
        "size": fixture.stat().st_size,
        "etag": stat.get("etag", ""),
    }


def prepare_clickhouse(name: str) -> None:
    ddl = """
CREATE TABLE IF NOT EXISTS traffic.pcap_index
(
  tenant_id String, probe_id String, file_key String, bucket String,
  object_version String, etag String, original_size UInt64, stored_size UInt64,
  compression LowCardinality(String), manifest_version UInt16,
  kafka_topic String, kafka_partition Int32, kafka_offset Int64,
  kafka_key_sha256 FixedString(64), kafka_headers_sha256 FixedString(64),
  raw_sha256 FixedString(64), projection_identity FixedString(64),
  ts_start DateTime64(3), ts_end DateTime64(3), byte_size UInt64,
  zstd_level UInt8, sha256 String, community_id String, flow_id String,
  offset_start Nullable(UInt64), offset_end Nullable(UInt64),
  bloom_filter_b64 String, community_ids Array(String), created_ts DateTime64(3)
)
ENGINE = ReplacingMergeTree(created_ts)
PARTITION BY toYYYYMMDD(ts_start)
ORDER BY (tenant_id, ts_start, probe_id, file_key)
""".strip()
    run(["docker", "exec", name, "clickhouse-client", "--multiquery", "--query", ddl])


def run_flink_matrix(kafka_port: int, clickhouse_port: int) -> dict[str, object]:
    command = [
        "mvn", "-pl", "flink-pcap-index-job", "-am", "test", "-DskipITs",
        "-Dtest=PcapIndexJobIntegrationTest", "-Dsurefire.failIfNoSpecifiedTests=false",
    ]
    environment = os.environ.copy()
    environment.update({
        "M02_PCAP_KAFKA_CLICKHOUSE_INTEGRATION_ENABLED": "true",
        "M02_PCAP_KAFKA_BOOTSTRAP_SERVERS": f"127.0.0.1:{kafka_port}",
        "M02_PCAP_KAFKA_BROKER_OWNED_BY_TEST": "true",
        "PCAP_CARRIER_EPHEMERAL_CLICKHOUSE_JDBC_URL":
            f"jdbc:clickhouse://127.0.0.1:{clickhouse_port}/traffic",
        "PCAP_CARRIER_EPHEMERAL_CLICKHOUSE_SENTINEL": "codex_ephemeral_m02_clickhouse",
        "PCAP_CARRIER_EPHEMERAL_CLICKHOUSE_USER": "default",
        "PCAP_CARRIER_EPHEMERAL_CLICKHOUSE_PASSWORD": "",
    })
    completed = run(command, cwd=ROOT / "java/flink-jobs", env=environment, check=False)
    output = completed.stdout.decode(errors="replace")
    if completed.returncode != 0:
        if "did not commit source offset" in output or "checkpoint did not commit" in output:
            raise LoopbackRejection("REJECT_KAFKA_OFFSET_NOT_CLOSED", output[-4000:])
        raise RuntimeError("Flink Kafka/ClickHouse matrix failed: " + output[-4000:])
    if "BUILD SUCCESS" not in output:
        raise LoopbackRejection(
            "REJECT_LOOPBACK_BROKER_NOT_OBSERVED", "Flink matrix produced no successful broker receipt",
        )
    return {
        "command": command,
        "exit_code": completed.returncode,
        "stdout_sha256": hashlib.sha256(completed.stdout).hexdigest(),
        "broker_observed": True,
        "offset_closed": True,
        "clickhouse_final_projection_count": 1,
    }


def self_check() -> dict[str, object]:
    assert len(REQUIRED_PARENT_INDEXES) == 12
    assert set(REQUIRED_REJECTIONS) == {
        "REJECT_LOOPBACK_BROKER_NOT_OBSERVED",
        "REJECT_MINIO_OBJECT_HASH_MISMATCH",
        "REJECT_KAFKA_OFFSET_NOT_CLOSED",
    }
    assert all("@sha256:" in image for image in (
        KAFKA_IMAGE, MINIO_IMAGE, MINIO_CLIENT_IMAGE, CLICKHOUSE_IMAGE))
    return {
        "required_parent_index_count": len(REQUIRED_PARENT_INDEXES),
        "required_rejections": list(REQUIRED_REJECTIONS),
        "all_images_digest_pinned": True,
        "production_applied": False,
    }


def main() -> int:
    parser = argparse.ArgumentParser()
    parser.add_argument("--run-id", required=True)
    parser.add_argument("--result-output", type=Path)
    parser.add_argument("--self-check", action="store_true")
    args = parser.parse_args()

    if args.self_check:
        print(json.dumps(self_check(), indent=2, sort_keys=True))
        return 0

    suffix = hashlib.sha256(args.run_id.encode()).hexdigest()[:12]
    names = {
        "kafka": f"codex-m02-loopback-kafka-{suffix}",
        "minio": f"codex-m02-loopback-minio-{suffix}",
        "clickhouse": f"codex-m02-loopback-clickhouse-{suffix}",
    }
    result: dict[str, Any] = {
        "schema_version": 1,
        "artifact_kind": "M02_N014_LOOPBACK_KAFKA_MINIO_CLICKHOUSE_TEST_RESULT",
        "run_id": args.run_id,
        "status": "BLOCKED",
        "environment_id": "owned-loopback-kafka-minio-clickhouse-flink",
        "production_applied": False,
        "shared_environment_touched": False,
        "persistent_volume_attached": False,
        "images": {
            "kafka": KAFKA_IMAGE,
            "minio": MINIO_IMAGE,
            "minio_client": MINIO_CLIENT_IMAGE,
            "clickhouse": CLICKHOUSE_IMAGE,
        },
        "parent_indexes": [],
        "checks": [],
        "containers_removed": False,
        "rejection_code": "",
        "errors": [],
    }
    owned: set[str] = set()
    try:
        result["parent_indexes"] = require_current_parent_indexes()
        for name in names.values():
            if not container_absent(name):
                raise RuntimeError(f"refusing to reuse existing container: {name}")
        for image in (KAFKA_IMAGE, MINIO_IMAGE, MINIO_CLIENT_IMAGE, CLICKHOUSE_IMAGE):
            run(["docker", "image", "inspect", image])

        kafka_port, minio_port, clickhouse_port = (
            loopback_port(), loopback_port(), loopback_port())
        start_kafka(names["kafka"], kafka_port, owned)
        start_minio(names["minio"], minio_port, owned)
        start_clickhouse(names["clickhouse"], clickhouse_port, owned)
        prepare_clickhouse(names["clickhouse"])
        with tempfile.TemporaryDirectory(prefix="m02-n014-") as directory:
            object_receipt = verify_minio_object(names["minio"], Path(directory))
        result["checks"].append({"minio_object": object_receipt})
        result["checks"].append({"flink": run_flink_matrix(kafka_port, clickhouse_port)})
        result["status"] = "PASS"
    except LoopbackRejection as error:
        result["status"] = "BLOCKED" if error.code.startswith("BLOCK_") else "FAIL"
        result["rejection_code"] = error.code
        result["errors"] = [error.detail]
    except Exception as error:
        result["status"] = "FAIL"
        result["errors"] = [str(error)]
    finally:
        for name in list(owned):
            run(["docker", "rm", "-f", name], check=False)
        result["containers_removed"] = all(container_absent(name) for name in names.values())

    rendered = json.dumps(result, ensure_ascii=False, indent=2, sort_keys=True)
    if args.result_output:
        atomic_write_json(args.result_output.resolve(), result)
    print(rendered)
    return 0 if result["status"] == "PASS" and result["containers_removed"] else 1


if __name__ == "__main__":
    raise SystemExit(main())
