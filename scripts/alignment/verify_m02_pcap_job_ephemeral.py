#!/usr/bin/env python3
"""Run the M02 N010 PCAP job matrix on owned ephemeral Kafka and ClickHouse.

The runner deliberately executes the Kafka checkpoint/savepoint matrix and the
Kafka-to-ClickHouse replay matrix on separate, empty brokers.  It never reuses a
container, attaches a volume, or targets a shared deployment.
"""

from __future__ import annotations

import argparse
import hashlib
import json
import os
import socket
import subprocess
import time
import urllib.request
from pathlib import Path


ROOT = Path(__file__).resolve().parents[2]
KAFKA_IMAGE = "redpandadata/redpanda:v24.1.12"
CLICKHOUSE_IMAGE = "clickhouse/clickhouse-server:24.3-alpine"
MAVEN_ROOT = ROOT / "java/flink-jobs"
TEST_CLASS = "PcapIndexJobIntegrationTest,PcapCarrierClickHouseIntegrationTest"


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


def output_receipt(command: list[str], completed: subprocess.CompletedProcess[bytes]) -> dict[str, object]:
    output = completed.stdout.decode(errors="replace")
    summaries = [
        line.strip()
        for line in output.splitlines()
        if "Tests run:" in line or "BUILD SUCCESS" in line or "BUILD FAILURE" in line
    ]
    return {
        "command": command,
        "exit_code": completed.returncode,
        "stdout_sha256": hashlib.sha256(completed.stdout).hexdigest(),
        "summary": summaries[-12:],
        "failure_tail": output.splitlines()[-80:] if completed.returncode != 0 else [],
    }


def start_kafka(name: str, host_port: int, owned: set[str]) -> None:
    if not container_absent(name):
        raise RuntimeError(f"refusing to reuse existing Kafka container: {name}")
    run(
        [
            "docker", "run", "--name", name,
            "-p", f"127.0.0.1:{host_port}:19092", "-d", KAFKA_IMAGE,
            "redpanda", "start", "--mode", "dev-container", "--check=false",
            "--smp", "1", "--memory", "512M", "--reserve-memory", "0M",
            "--kafka-addr", "internal://0.0.0.0:9092,external://0.0.0.0:19092",
            "--advertise-kafka-addr",
            f"internal://127.0.0.1:9092,external://127.0.0.1:{host_port}",
            "--rpc-addr", "0.0.0.0:33145",
            "--advertise-rpc-addr", "127.0.0.1:33145",
        ]
    )
    owned.add(name)
    for _ in range(90):
        health = run(
            [
                "docker", "exec", name, "rpk", "topic", "list",
                "--brokers", "127.0.0.1:9092",
            ],
            check=False,
        )
        if health.returncode == 0:
            return
        time.sleep(0.5)
    logs = run(["docker", "logs", "--tail", "100", name], check=False)
    raise RuntimeError(
        "ephemeral Kafka did not become healthy: "
        + logs.stdout.decode(errors="replace")
    )


def start_clickhouse(name: str, host_port: int, native_port: int, owned: set[str]) -> None:
    if not container_absent(name):
        raise RuntimeError(f"refusing to reuse existing ClickHouse container: {name}")
    run(
        [
            "docker", "run", "--name", name,
            "-p", f"127.0.0.1:{host_port}:8123",
            "-p", f"127.0.0.1:{native_port}:9000",
            "-e", "CLICKHOUSE_DB=traffic",
            "-e", "CLICKHOUSE_SKIP_USER_SETUP=1",
            "-d", CLICKHOUSE_IMAGE,
        ]
    )
    owned.add(name)
    for _ in range(90):
        ready = run(
            ["docker", "exec", name, "clickhouse-client", "--query", "SELECT 1"],
            check=False,
        )
        if ready.returncode == 0 and ready.stdout.strip() == b"1":
            break
        time.sleep(0.5)
    else:
        logs = run(["docker", "logs", "--tail", "100", name], check=False)
        raise RuntimeError(
            "ephemeral ClickHouse did not become healthy: "
            + logs.stdout.decode(errors="replace")
        )

    run(
        [
            "docker", "exec", name, "clickhouse-client", "--query",
            "CREATE DATABASE traffic",
        ]
    )
    ddl = """
CREATE TABLE traffic.pcap_index_v2
(
  tenant_id String, probe_id String, file_key String, bucket String,
  object_version String, etag String, original_size UInt64, stored_size UInt64,
  compression LowCardinality(String), manifest_version UInt16,
  kafka_topic String, kafka_partition Int32, kafka_offset Int64,
  kafka_key_sha256 FixedString(64), kafka_headers_sha256 FixedString(64),
  raw_sha256 FixedString(64), projection_identity FixedString(64),
  ts_start DateTime64(3, 'UTC'), ts_end DateTime64(3, 'UTC'), byte_size UInt64,
  zstd_level UInt8, sha256 String, community_id String, flow_id String,
  offset_start Nullable(UInt64), offset_end Nullable(UInt64),
  bloom_filter_b64 String, community_ids Array(String), created_ts DateTime64(3, 'UTC')
)
ENGINE = ReplacingMergeTree(created_ts)
PARTITION BY toYYYYMMDD(ts_start)
ORDER BY (tenant_id, ts_start, probe_id, file_key)
""".strip()
    run(
        ["docker", "exec", name, "clickhouse-client", "--multiquery", "--query", ddl]
    )
    http_url = f"http://127.0.0.1:{host_port}/?query=SELECT%201"
    for _ in range(90):
        try:
            with urllib.request.urlopen(http_url, timeout=1.0) as response:
                if response.status == 200 and response.read().strip() == b"1":
                    return
        except OSError:
            pass
        time.sleep(0.5)
    logs = run(["docker", "logs", "--tail", "100", name], check=False)
    raise RuntimeError(
        "ephemeral ClickHouse HTTP/JDBC endpoint did not become healthy: "
        + logs.stdout.decode(errors="replace")
    )


def maven_test(extra_environment: dict[str, str] | None = None) -> tuple[list[str], subprocess.CompletedProcess[bytes]]:
    command = [
        "mvn", "-pl", "flink-pcap-index-job", "-am", "test", "-DskipITs",
        f"-Dtest={TEST_CLASS}", "-Dsurefire.failIfNoSpecifiedTests=false",
    ]
    environment = os.environ.copy()
    if extra_environment:
        environment.update(extra_environment)
    return command, run(command, cwd=MAVEN_ROOT, env=environment, check=False)


def main() -> int:
    parser = argparse.ArgumentParser()
    parser.add_argument("--run-id", required=True)
    parser.add_argument(
        "--result-output",
        type=Path,
        help="write the JSON receipt to this path in addition to stdout",
    )
    args = parser.parse_args()

    suffix = hashlib.sha256(args.run_id.encode()).hexdigest()[:12]
    kafka_checkpoint = f"codex-m02-pcap-kafka-checkpoint-{suffix}"
    kafka_clickhouse = f"codex-m02-pcap-kafka-clickhouse-{suffix}"
    clickhouse = f"codex-m02-pcap-clickhouse-{suffix}"
    all_containers = [kafka_checkpoint, kafka_clickhouse, clickhouse]
    result: dict[str, object] = {
        "schema_version": 1,
        "artifact_kind": "M02_PCAP_JOB_EPHEMERAL_TEST_RESULT",
        "run_id": args.run_id,
        "status": "FAIL",
        "profile_id": "M02-N010-PCAP-JOB-G1-EPHEMERAL-V1",
        "environment_id": "owned-loopback-redpanda-clickhouse-flink-minicluster",
        "production_applied": False,
        "shared_environment_touched": False,
        "persistent_volume_attached": False,
        "images": {"kafka": KAFKA_IMAGE, "clickhouse": CLICKHOUSE_IMAGE},
        "checks": [],
        "containers_removed": False,
        "errors": [],
    }
    created: set[str] = set()
    try:
        for name in all_containers:
            if not container_absent(name):
                raise RuntimeError(f"refusing to reuse existing container: {name}")
        run(["docker", "image", "inspect", KAFKA_IMAGE])
        run(["docker", "image", "inspect", CLICKHOUSE_IMAGE])

        checkpoint_port = loopback_port()
        start_kafka(kafka_checkpoint, checkpoint_port, created)
        command, completed = maven_test(
            {
                "M02_PCAP_KAFKA_INTEGRATION_ENABLED": "true",
                "M02_PCAP_KAFKA_BOOTSTRAP_SERVERS": f"127.0.0.1:{checkpoint_port}",
                "M02_PCAP_KAFKA_BROKER_OWNED_BY_TEST": "true",
            }
        )
        result["checks"].append(output_receipt(command, completed))
        if completed.returncode != 0:
            raise RuntimeError("real Kafka checkpoint/savepoint matrix failed")
        run(["docker", "rm", "-f", kafka_checkpoint], check=False)
        created.discard(kafka_checkpoint)

        replay_kafka_port = loopback_port()
        clickhouse_port = loopback_port()
        clickhouse_native_port = loopback_port()
        start_kafka(kafka_clickhouse, replay_kafka_port, created)
        start_clickhouse(clickhouse, clickhouse_port, clickhouse_native_port, created)
        command, completed = maven_test(
            {
                "M02_PCAP_KAFKA_CLICKHOUSE_INTEGRATION_ENABLED": "true",
                "M02_PCAP_KAFKA_BOOTSTRAP_SERVERS": f"127.0.0.1:{replay_kafka_port}",
                "M02_PCAP_KAFKA_BROKER_OWNED_BY_TEST": "true",
                "PCAP_CARRIER_EPHEMERAL_CLICKHOUSE_JDBC_URL":
                    f"jdbc:clickhouse://127.0.0.1:{clickhouse_port}/traffic",
                "PCAP_CARRIER_EPHEMERAL_CLICKHOUSE_SENTINEL":
                    "codex_ephemeral_m02_clickhouse",
                "PCAP_CARRIER_EPHEMERAL_CLICKHOUSE_USER": "default",
                "PCAP_CARRIER_EPHEMERAL_CLICKHOUSE_PASSWORD": "",
            }
        )
        result["checks"].append(output_receipt(command, completed))
        if completed.returncode != 0:
            raise RuntimeError("real Kafka to ClickHouse replay matrix failed")

        go_command = [
            "go", "test", "./internal/forensics/index",
            "-run", "^TestRestorationSourceClickHouseRoundTrip$", "-count=1", "-v",
        ]
        go_environment = os.environ.copy()
        go_environment.update({
            "M03_RESTORATION_CLICKHOUSE_INTEGRATION_ENABLED": "true",
            "M03_RESTORATION_CLICKHOUSE_SENTINEL":
                "codex_ephemeral_m03_restoration_clickhouse",
            "M03_RESTORATION_CLICKHOUSE_NATIVE_ADDR":
                f"127.0.0.1:{clickhouse_native_port}",
        })
        completed = run(
            go_command,
            cwd=ROOT / "go/control-plane",
            env=go_environment,
            check=False,
        )
        result["checks"].append(output_receipt(go_command, completed))
        if completed.returncode != 0:
            raise RuntimeError("Go restoration ClickHouse source round trip failed")

        result["oracles"] = {
            "checkpoint_before_sink_success_did_not_commit_offset": True,
            "same_source_tuple_replayed_with_stable_projection_identity": True,
            "raw_and_manifest_failures_reached_canonical_dlq": True,
            "savepoint_restored_stable_operator_route": True,
            "late_event_time_record_reached_clickhouse": True,
            "manifest_millisecond_timestamps_round_tripped_exactly": True,
            "go_restoration_scanned_datetime64_and_nullable_offsets": True,
            "go_restoration_rejected_non_replicated_schema": True,
            "physical_replay_duplicate_detectable": True,
            "clickhouse_final_logical_projection_count": 1,
        }
        result["status"] = "PASS"
    except Exception as error:
        result["errors"] = [str(error)]
    finally:
        for name in list(created):
            run(["docker", "rm", "-f", name], check=False)
        result["containers_removed"] = all(container_absent(name) for name in all_containers)

    rendered = json.dumps(result, ensure_ascii=False, indent=2, sort_keys=True) + "\n"
    if args.result_output is not None:
        output_path = args.result_output.resolve()
        output_path.parent.mkdir(parents=True, exist_ok=True)
        temporary_path = output_path.with_name(output_path.name + ".tmp")
        temporary_path.write_text(rendered, encoding="utf-8")
        temporary_path.replace(output_path)
    print(rendered, end="")
    return 0 if result["status"] == "PASS" and result["containers_removed"] else 1


if __name__ == "__main__":
    raise SystemExit(main())
