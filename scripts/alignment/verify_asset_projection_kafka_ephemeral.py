#!/usr/bin/env python3
"""Verify asset.events.v2 delivery into an owned PostgreSQL durable inbox."""

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

from run_exact_go_tests import run_exact_go_tests


ROOT = Path(__file__).resolve().parents[2]
POSTGRES_IMAGE = "postgres@sha256:33f923b05f64ca54ac4401c01126a6b92afe839a0aa0a52bc5aeb5cc958e5f20"
KAFKA_IMAGE = "docker.io/redpandadata/redpanda@sha256:dca9d37efbbae3c2dcdc07d6a45fa1e0a7a541bc9cdc03db3937b80a4a9eae3d"
TOPIC = "asset.events.v2"
SENTINEL_TABLE = "codex_ephemeral_asset_projection_kafka_sentinel"
SENTINEL_VALUE = "ephemeral-only"
PASSWORD = "codex-asset-projection-kafka-ephemeral-only"
TEST_SOURCE = ROOT / "go/control-plane/internal/asset/consumer/asset_projection_real_kafka_integration_test.go"
REQUIRED_TEST_SOURCE_MARKERS = (
    "func TestAssetProjectionRealKafkaDurableInbox",
    "func TestAssetProjectionKafkaPublishFailureKeepsOutboxPending",
)
EXPECTED_ORACLE_MARKERS = {
    "TestAssetProjectionRealKafkaDurableInbox": {
        "ACK_BEFORE_PUBLISHED", "HEADERS_PAYLOAD", "DURABLE_INBOX_OFFSET",
        "EXACT_REPLAY_IDEMPOTENT",
    },
    "TestAssetProjectionKafkaPublishFailureKeepsOutboxPending": {
        "PUBLISH_FAILURE_PENDING",
    },
}


def collect_oracle_markers(events: list[dict[str, Any]]) -> dict[str, list[str]]:
    observed = {test: [] for test in EXPECTED_ORACLE_MARKERS}
    prefix = "TOPIC1_ORACLE PASS "
    for event in events:
        test = event.get("Test")
        output = event.get("Output")
        if test not in observed or not isinstance(output, str) or prefix not in output:
            continue
        observed[test].append(output.split(prefix, 1)[1].strip())
    for test, expected in EXPECTED_ORACLE_MARKERS.items():
        markers = observed[test]
        if set(markers) != expected or len(markers) != len(expected):
            raise ValueError(
                f"real-broker oracle exact-set mismatch for {test}: "
                f"expected={sorted(expected)} observed={markers}"
            )
    return {test: sorted(markers) for test, markers in observed.items()}


def derive_oracle_flags(oracle_results: dict[str, list[str]]) -> dict[str, bool]:
    observed = {marker for markers in oracle_results.values() for marker in markers}
    required = {
        "broker_ack_before_outbox_published_verified": "ACK_BEFORE_PUBLISHED",
        "headers_and_payload_consumed_verified": "HEADERS_PAYLOAD",
        "durable_inbox_offset_verified": "DURABLE_INBOX_OFFSET",
        "exact_replay_idempotent_verified": "EXACT_REPLAY_IDEMPOTENT",
        "publish_failure_remains_pending_verified": "PUBLISH_FAILURE_PENDING",
    }
    flags = {field: marker in observed for field, marker in required.items()}
    if not all(flags.values()):
        raise ValueError(f"real-broker oracle flags are incomplete: {flags}")
    return flags


def require_candidate_sources(candidate_manifest: Path, sources: list[Path]) -> None:
    payload = json.loads(candidate_manifest.read_text(encoding="utf-8"))
    declared = payload.get("source_blob_sha256")
    if not isinstance(declared, dict):
        raise ValueError("candidate manifest lacks source_blob_sha256")
    for source in sources:
        relative = source.relative_to(ROOT).as_posix()
        actual = hashlib.sha256(source.read_bytes()).hexdigest()
        if declared.get(relative) != actual:
            raise ValueError(f"candidate manifest does not bind current source: {relative}")


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
    return f"codex-asset-projection-kafka-pg-{suffix}", f"codex-asset-projection-kafka-broker-{suffix}"


def container_absent(name: str) -> bool:
    return run(["docker", "container", "inspect", name], check=False).returncode != 0


def main() -> int:
    parser = argparse.ArgumentParser()
    parser.add_argument("--run-id", required=True)
    parser.add_argument("--output", type=Path)
    parser.add_argument("--candidate-manifest", type=Path, required=True)
    parser.add_argument("--profile-id", required=True)
    parser.add_argument("--environment-id", required=True)
    args = parser.parse_args()
    postgres_container, kafka_container = names(args.run_id)
    kafka_port = loopback_port()
    candidate_manifest = (ROOT / args.candidate_manifest).resolve()
    if not candidate_manifest.is_relative_to(ROOT) or not candidate_manifest.is_file():
        raise SystemExit(f"unsafe or missing candidate manifest: {args.candidate_manifest}")
    bound_sources = [
        TEST_SOURCE,
        ROOT / "go/control-plane/internal/asset/repository/outbox_dispatcher.go",
    ]
    require_candidate_sources(candidate_manifest, bound_sources)
    result: dict[str, Any] = {
        "schema_version": 1,
        "artifact_kind": "ASSET_PROJECTION_KAFKA_EPHEMERAL_TEST_RESULT",
        "subject_pr_id": "T1-M06-P908-TST-PRE-n004-asset-event-real-broker-ack",
        "run_id": args.run_id,
        "status": "FAIL",
        "candidate_manifest": {
            "path": candidate_manifest.relative_to(ROOT).as_posix(),
            "sha256": hashlib.sha256(candidate_manifest.read_bytes()).hexdigest(),
        },
        "profile_id": args.profile_id,
        "environment_id": args.environment_id,
        "source_blob_sha256": {
            path.relative_to(ROOT).as_posix(): hashlib.sha256(path.read_bytes()).hexdigest()
            for path in bound_sources
        },
        "runner_sha256": hashlib.sha256(Path(__file__).read_bytes()).hexdigest(),
        "exact_runner_sha256": hashlib.sha256(
            (ROOT / "scripts/alignment/run_exact_go_tests.py").read_bytes()
        ).hexdigest(),
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
        "broker_ack_before_outbox_published_verified": False,
        "headers_and_payload_consumed_verified": False,
        "durable_inbox_offset_verified": False,
        "exact_replay_idempotent_verified": False,
        "publish_failure_remains_pending_verified": False,
        "loopback_only": True,
        "persistent_volume_attached": False,
        "shared_environment_touched": False,
        "production_applied": False,
        "postgres_container_removed": False,
        "kafka_container_removed": False,
        "test_output": "",
        "exact_test_events": {},
        "oracle_results": {},
        "test_source_sha256": None,
        "required_test_source_markers": list(REQUIRED_TEST_SOURCE_MARKERS),
        "errors": [],
        "proof_ceiling": "OWNED_EPHEMERAL_KAFKA_G1_ONLY_NOT_PRODUCTION_ACCEPTANCE",
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
            metadata = run(
                ["docker", "exec", kafka_container, "rpk", "topic", "list", "--brokers", "127.0.0.1:9092"],
                check=False,
            )
            if metadata.returncode == 0:
                break
            time.sleep(1)
        else:
            logs = run(["docker", "logs", "--tail", "100", kafka_container], check=False)
            raise RuntimeError("ephemeral Kafka did not become healthy: " + logs.stdout.decode(errors="replace")[-8192:])
        run(
            [
                "docker", "exec", kafka_container, "rpk", "topic", "create", TOPIC,
                "--brokers", "127.0.0.1:9092", "--partitions", "1", "--replicas", "1",
                "-c", "cleanup.policy=delete", "-c", "retention.ms=3600000",
            ]
        )
        topic_listing = run(
            ["docker", "exec", kafka_container, "rpk", "topic", "list", "--brokers", "127.0.0.1:9092"]
        ).stdout.decode()
        topic_description = run(
            ["docker", "exec", kafka_container, "rpk", "topic", "describe", TOPIC, "--brokers", "127.0.0.1:9092", "-p"]
        ).stdout.decode()
        if TOPIC not in topic_listing or "0" not in topic_description:
            raise RuntimeError("ephemeral Kafka topic description is incomplete")
        result["topic_partitions"] = 1
        result["kafka_sentinel_verified"] = True

        run(
            [
                "docker", "run", "--name", postgres_container,
                "-e", f"POSTGRES_PASSWORD={PASSWORD}", "-e", "POSTGRES_DB=traffic_platform",
                "-p", "127.0.0.1::5432", "-d", POSTGRES_IMAGE,
            ]
        )
        postgres_created = True
        for _ in range(30):
            ready = run(
                ["docker", "exec", postgres_container, "pg_isready", "-U", "postgres", "-d", "traffic_platform"],
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

        test_source = TEST_SOURCE.read_text(encoding="utf-8")
        missing_markers = [marker for marker in REQUIRED_TEST_SOURCE_MARKERS if marker not in test_source]
        if missing_markers:
            raise RuntimeError(f"real-broker fixture lacks required after-state oracle markers: {missing_markers}")
        result["test_source_sha256"] = hashlib.sha256(TEST_SOURCE.read_bytes()).hexdigest()

        test_env = os.environ.copy()
        test_env["ASSET_PROJECTION_EPHEMERAL_PG_DSN"] = (
            f"postgres://postgres:{PASSWORD}@127.0.0.1:{postgres_port}/traffic_platform?sslmode=disable"
        )
        test_env["ASSET_PROJECTION_EPHEMERAL_KAFKA_BROKER"] = f"127.0.0.1:{kafka_port}"
        test_env["ASSET_PROJECTION_EPHEMERAL_KAFKA_SENTINEL"] = SENTINEL_VALUE
        completed, events, exact_counts = run_exact_go_tests(
            go_root=ROOT / "go/control-plane",
            package="./internal/asset/consumer",
            test_names=[
                "TestAssetProjectionRealKafkaDurableInbox",
                "TestAssetProjectionKafkaPublishFailureKeepsOutboxPending",
            ],
            env=test_env,
        )
        result["test_output"] = completed.stdout.strip()
        result["exact_test_events"] = exact_counts
        oracle_results = collect_oracle_markers(events)
        result["oracle_results"] = oracle_results
        result.update(derive_oracle_flags(oracle_results))
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

    payload = json.dumps(result, indent=2, ensure_ascii=False) + "\n"
    if args.output:
        output = args.output.resolve()
        if output.exists():
            raise SystemExit(f"refusing to overwrite asset Kafka G1 evidence: {output}")
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
