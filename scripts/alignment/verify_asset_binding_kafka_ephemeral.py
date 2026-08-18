#!/usr/bin/env python3
"""Verify asset.bindings.v1 durable authority, replay, and DLQ ordering."""

from __future__ import annotations

import argparse
import hashlib
import json
import os
import time
from pathlib import Path
from typing import Any

from run_exact_go_tests import run_exact_go_tests
from verify_asset_projection_kafka_ephemeral import (
    KAFKA_IMAGE,
    POSTGRES_IMAGE,
    container_absent,
    loopback_port,
    require_candidate_sources,
    run,
)


ROOT = Path(__file__).resolve().parents[2]
PASSWORD = "codex-asset-binding-kafka-ephemeral-only"
SENTINEL_TABLE = "codex_ephemeral_asset_binding_test_sentinel"
TEST_SOURCE = ROOT / "go/control-plane/internal/asset/consumer/binding_consumer_test.go"
EXPECTED_MARKERS = {
    "KAFKA_REQUIRED_ACKS_ALL",
    "DURABLE_AUTHORITY_COMMIT_BEFORE_OFFSET",
    "EXACT_REPLAY_IDEMPOTENT",
    "DLQ_ACK_BEFORE_SOURCE_COMMIT",
    "TENANT_FORGERY_QUARANTINED",
}


def names(run_id: str) -> tuple[str, str]:
    if not run_id.strip():
        raise ValueError("run_id is required")
    suffix = hashlib.sha256(run_id.encode()).hexdigest()[:12]
    return f"codex-asset-binding-pg-{suffix}", f"codex-asset-binding-broker-{suffix}"


def collect_markers(events: list[dict[str, Any]]) -> list[str]:
    prefix = "M06_BINDING_ORACLE PASS "
    observed = []
    for event in events:
        if event.get("Test") != "TestBindingConsumerRealKafkaDurableAuthority":
            continue
        output = event.get("Output")
        if isinstance(output, str) and prefix in output:
            observed.append(output.split(prefix, 1)[1].strip())
    if set(observed) != EXPECTED_MARKERS or len(observed) != len(EXPECTED_MARKERS):
        raise ValueError(
            f"asset binding oracle exact-set mismatch: expected={sorted(EXPECTED_MARKERS)} observed={observed}"
        )
    return sorted(observed)


def main() -> int:
    parser = argparse.ArgumentParser()
    parser.add_argument("--run-id", required=True)
    parser.add_argument("--output", type=Path)
    parser.add_argument("--candidate-manifest", type=Path, required=True)
    parser.add_argument("--profile-id", required=True)
    parser.add_argument("--environment-id", required=True)
    args = parser.parse_args()

    candidate_manifest = (ROOT / args.candidate_manifest).resolve()
    if not candidate_manifest.is_relative_to(ROOT) or not candidate_manifest.is_file():
        raise SystemExit(f"unsafe or missing candidate manifest: {args.candidate_manifest}")
    sources = [
        TEST_SOURCE,
        ROOT / "go/control-plane/internal/asset/consumer/binding_consumer.go",
        ROOT / "go/control-plane/internal/asset/service/asset_observations.go",
        ROOT / "go/control-plane/internal/ingest/queue/producer.go",
    ]
    require_candidate_sources(candidate_manifest, sources)
    postgres_container, kafka_container = names(args.run_id)
    kafka_port = loopback_port()
    result: dict[str, Any] = {
        "schema_version": 1,
        "artifact_kind": "M06_ASSET_BINDING_KAFKA_EPHEMERAL_TEST_RESULT",
        "subject_pr_id": "T1-M06-P049-TST-POST-n017-s5",
        "run_id": args.run_id,
        "candidate_id": hashlib.sha256(candidate_manifest.read_bytes()).hexdigest(),
        "candidate_manifest": candidate_manifest.relative_to(ROOT).as_posix(),
        "profile_id": args.profile_id,
        "environment_id": args.environment_id,
        "status": "FAIL",
        "postgres_image": POSTGRES_IMAGE,
        "kafka_image": KAFKA_IMAGE,
        "postgres_container": postgres_container,
        "kafka_container": kafka_container,
        "postgres_sentinel_verified": False,
        "kafka_sentinel_verified": False,
        "topics": {},
        "source_blob_sha256": {
            path.relative_to(ROOT).as_posix(): hashlib.sha256(path.read_bytes()).hexdigest()
            for path in sources
        },
        "runner_sha256": hashlib.sha256(Path(__file__).read_bytes()).hexdigest(),
        "oracle_results": [],
        "test_output": "",
        "exact_test_events": {},
        "loopback_only": True,
        "persistent_volume_attached": False,
        "shared_environment_touched": False,
        "production_applied": False,
        "postgres_container_removed": False,
        "kafka_container_removed": False,
        "errors": [],
        "proof_ceiling": "OWNED_EPHEMERAL_KAFKA_POSTGRES_G1_NOT_SHARED_K8S_ACCEPTANCE",
    }
    postgres_created = False
    kafka_created = False
    try:
        for container in (postgres_container, kafka_container):
            if not container_absent(container):
                raise RuntimeError(f"refusing to reuse existing container: {container}")
        result["postgres_image_id"] = json.loads(run(["docker", "image", "inspect", POSTGRES_IMAGE]).stdout)[0].get("Id")
        result["kafka_image_id"] = json.loads(run(["docker", "image", "inspect", KAFKA_IMAGE]).stdout)[0].get("Id")

        run([
            "docker", "run", "--name", kafka_container,
            "-p", f"127.0.0.1:{kafka_port}:19092", "-d", KAFKA_IMAGE,
            "redpanda", "start", "--mode", "dev-container", "--check=false",
            "--kafka-addr", "internal://0.0.0.0:9092,external://0.0.0.0:19092",
            "--advertise-kafka-addr", f"internal://127.0.0.1:9092,external://127.0.0.1:{kafka_port}",
            "--rpc-addr", "0.0.0.0:33145", "--advertise-rpc-addr", "127.0.0.1:33145",
        ])
        kafka_created = True
        for _ in range(60):
            ready = run(["docker", "exec", kafka_container, "rpk", "topic", "list", "--brokers", "127.0.0.1:9092"], check=False)
            if ready.returncode == 0:
                break
            time.sleep(1)
        else:
            raise RuntimeError("ephemeral Kafka did not become healthy")
        for topic in ("asset.bindings.v1", "dlq.v1"):
            run([
                "docker", "exec", kafka_container, "rpk", "topic", "create", topic,
                "--brokers", "127.0.0.1:9092", "--partitions", "1", "--replicas", "1",
                "-c", "cleanup.policy=delete", "-c", "retention.ms=3600000",
            ])
            description = run([
                "docker", "exec", kafka_container, "rpk", "topic", "describe", topic,
                "--brokers", "127.0.0.1:9092", "-p",
            ]).stdout.decode()
            if "0" not in description:
                raise RuntimeError(f"topic partition verification failed: {topic}")
            result["topics"][topic] = {"partitions": 1, "replicas": 1}
        result["kafka_sentinel_verified"] = True

        run([
            "docker", "run", "--name", postgres_container,
            "-e", f"POSTGRES_PASSWORD={PASSWORD}", "-e", "POSTGRES_DB=traffic_platform",
            "-p", "127.0.0.1::5432", "-d", POSTGRES_IMAGE,
        ])
        postgres_created = True
        for _ in range(30):
            ready = run(["docker", "exec", postgres_container, "pg_isready", "-U", "postgres", "-d", "traffic_platform"], check=False)
            if ready.returncode == 0:
                break
            time.sleep(1)
        else:
            raise RuntimeError("ephemeral PostgreSQL did not become ready")
        schema_files = sorted((ROOT / "common/sql/pg").glob("*.sql"))
        for path in schema_files:
            run([
                "docker", "exec", "-e", "PGOPTIONS=--client-min-messages=warning", "-i",
                postgres_container, "psql", "-X", "-v", "ON_ERROR_STOP=1", "-U", "postgres",
                "-d", "traffic_platform",
            ], input_bytes=path.read_bytes())
        result["schema_files"] = len(schema_files)
        run([
            "docker", "exec", postgres_container, "psql", "-X", "-v", "ON_ERROR_STOP=1",
            "-U", "postgres", "-d", "traffic_platform", "-c",
            f"CREATE TABLE {SENTINEL_TABLE}(marker text primary key); INSERT INTO {SENTINEL_TABLE}(marker) VALUES ('ephemeral-only');",
        ])
        result["postgres_sentinel_verified"] = True
        postgres_port = run(["docker", "port", postgres_container, "5432/tcp"]).stdout.decode().strip().rsplit(":", 1)[-1]
        if not postgres_port.isdigit():
            raise RuntimeError("invalid loopback PostgreSQL port")

        test_env = os.environ.copy()
        test_env.update({
            "ASSET_BINDING_EPHEMERAL_PG_DSN": f"postgres://postgres:{PASSWORD}@127.0.0.1:{postgres_port}/traffic_platform?sslmode=disable",
            "ASSET_BINDING_EPHEMERAL_KAFKA_BROKER": f"127.0.0.1:{kafka_port}",
            "ASSET_BINDING_EPHEMERAL_KAFKA_SENTINEL": "ephemeral-only",
        })
        completed, events, counts = run_exact_go_tests(
            go_root=ROOT / "go/control-plane", package="./internal/asset/consumer",
            test_names=["TestBindingConsumerRealKafkaDurableAuthority"], env=test_env,
        )
        result["test_output"] = completed.stdout.strip()
        result["exact_test_events"] = counts
        result["oracle_results"] = collect_markers(events)
        result["status"] = "PASS"
    except Exception as error:
        result["errors"] = [str(error)]
    finally:
        if postgres_created:
            run(["docker", "rm", "-f", postgres_container], check=False)
        if kafka_created:
            run(["docker", "rm", "-f", kafka_container], check=False)
        result["postgres_container_removed"] = container_absent(postgres_container)
        result["kafka_container_removed"] = container_absent(kafka_container)

    payload = json.dumps(result, sort_keys=True, indent=2) + "\n"
    if args.output:
        output = args.output.resolve()
        if output.exists():
            raise SystemExit(f"refusing to overwrite evidence: {output}")
        output.parent.mkdir(parents=True, exist_ok=True)
        output.write_text(payload, encoding="utf-8")
    print(payload, end="")
    return 0 if result["status"] == "PASS" and result["postgres_container_removed"] and result["kafka_container_removed"] else 1


if __name__ == "__main__":
    raise SystemExit(main())
