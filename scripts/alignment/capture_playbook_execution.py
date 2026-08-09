#!/usr/bin/env python3
"""Capture immutable generic playbook execution evidence."""

from __future__ import annotations

import argparse
import hashlib
import json
import os
import subprocess
from datetime import datetime, timezone
from pathlib import Path
from typing import Any

from candidate_snapshot import build_snapshot


ROOT = Path(__file__).resolve().parents[2]
DEFAULT_OUTPUT_ROOT = ROOT / "doc/02_acceptance/runs"
POSTGRES_ENV = "PLAYBOOK_EXECUTION_EPHEMERAL_PG_DSN"
KAFKA_ENV = "PLAYBOOK_EXECUTION_EPHEMERAL_KAFKA_BROKER"

SOURCE_ARTIFACTS = (
    "scripts/alignment/capture_playbook_execution.py",
    "scripts/alignment/verify_playbook_schema_entrypoints.py",
    "deployments/postgres/migrations/202608021000_playbook_execution_v2.sql",
    "deployments/postgres/migrations/202608021030_playbook_event_pipeline_v2.sql",
    "common/sql/pg/04-tasks-audit.sql",
    "common/kafka/create-topics.sh",
    "go/control-plane/deployments/docker/init/postgres_merged.sql",
    "deployments/kubernetes/init-jobs/01-kafka-topics.yaml",
    "deployments/kubernetes/init-jobs/02-postgres-schema.yaml",
    "go/control-plane/internal/alert/api/playbook_execution_v2.go",
    "go/control-plane/internal/alert/api/playbook_execution_v2_integration_test.go",
    "go/control-plane/internal/alert/api/campaign_four_store_trace_integration_test.go",
    "go/control-plane/internal/alert/api/playbook_execution_http_provider.go",
    "go/control-plane/internal/alert/api/playbook_execution_http_provider_test.go",
    "go/control-plane/internal/alert/api/playbook_execution_outbox.go",
    "go/control-plane/internal/alert/api/playbook_execution_outbox_test.go",
    "go/control-plane/internal/alert/api/playbook_execution_event_projection.go",
    "go/control-plane/internal/alert/api/playbook_execution_event_projection_test.go",
    "go/control-plane/internal/alert/consumer/playbook_execution_event_consumer.go",
    "go/control-plane/internal/alert/consumer/playbook_execution_event_consumer_test.go",
    "go/control-plane/internal/alert/config/config.go",
    "go/control-plane/internal/alert/config/config_test.go",
    "go/control-plane/internal/alert/api/advanced_repository.go",
    "go/control-plane/internal/alert/api/handler_advanced.go",
    "go/control-plane/cmd/alert-service/main.go",
    "web/ui/src/services/playbookAutomationApi.ts",
    "web/ui/src/services/playbookAutomationApi.test.ts",
    "web/ui/src/services/pageApiPlans.ts",
    "web/ui/src/services/pageApiPlans.test.ts",
    "web/ui/src/pages/PlaybookAutomationPage.tsx",
    "contracts/alignment/features/F-PLAYBOOK-001.json",
    "contracts/openapi/alignment-v1.openapi.json",
    "contracts/events/kafka-json-events-v1.schema.json",
    "contracts/events/kafka-topic-catalog.v1.json",
    "deployments/kubernetes/applications/go-services.yaml",
    "go/control-plane/deployments/kubernetes/alert-service.yaml",
    "doc/07_alignment/runbooks/F-PLAYBOOK-001-rollback.md",
    "tests/alignment/test_alignment_registry.py",
)


def sha256(path: Path) -> str:
    digest = hashlib.sha256()
    with path.open("rb") as source:
        for chunk in iter(lambda: source.read(1024 * 1024), b""):
            digest.update(chunk)
    return digest.hexdigest()


def run_command(
    name: str,
    command: list[str],
    output: Path,
    env: dict[str, str],
) -> dict[str, Any]:
    log_path = output / f"{name}.log"
    started = datetime.now(timezone.utc)
    print(f"[playbook-execution] starting {name}: {' '.join(command)}", flush=True)
    with log_path.open("wb") as log:
        completed = subprocess.run(
            command,
            cwd=ROOT,
            env=env,
            stdout=log,
            stderr=subprocess.STDOUT,
            check=False,
        )
    finished = datetime.now(timezone.utc)
    result = {
        "name": name,
        "command": command,
        "exit_code": completed.returncode,
        "status": "PASS" if completed.returncode == 0 else "FAIL",
        "started_at": started.isoformat(),
        "finished_at": finished.isoformat(),
        "duration_seconds": round((finished - started).total_seconds(), 3),
        "artifact": log_path.name,
        "sha256": sha256(log_path),
        "size_bytes": log_path.stat().st_size,
    }
    print(f"[playbook-execution] {name}: {result['status']}", flush=True)
    return result


def main() -> int:
    parser = argparse.ArgumentParser()
    parser.add_argument("--run-id", required=True)
    parser.add_argument("--g0-manifest", type=Path, required=True)
    parser.add_argument("--postgres-container", required=True)
    parser.add_argument("--kafka-broker", required=True)
    parser.add_argument("--output-root", type=Path, default=DEFAULT_OUTPUT_ROOT)
    args = parser.parse_args()

    if not os.environ.get(POSTGRES_ENV):
        raise SystemExit(
            f"{POSTGRES_ENV} is required; the integration test rejects databases without its sentinel table"
        )
    host, separator, port = args.kafka_broker.rpartition(":")
    if separator != ":" or host not in {"127.0.0.1", "localhost"} or not port.isdigit():
        raise SystemExit("--kafka-broker must be an explicit loopback host:port")
    g0_manifest = args.g0_manifest.resolve()
    if not g0_manifest.is_file():
        raise SystemExit(f"G0 manifest does not exist: {g0_manifest}")
    g0 = json.loads(g0_manifest.read_text(encoding="utf-8"))
    if g0.get("status") != "PASS" or g0.get("gate") != "G0":
        raise SystemExit("referenced G0 manifest is not a PASS result")

    source_before = build_snapshot()
    g0_source_sha = g0.get("candidate_source", {}).get("content_sha256")
    if not g0_source_sha or g0_source_sha != source_before["content_sha256"]:
        raise SystemExit(
            "referenced G0 manifest does not cover the current candidate source"
        )

    output = args.output_root.resolve() / args.run_id
    if output.exists():
        raise SystemExit(f"refusing to overwrite immutable evidence directory: {output}")
    output.mkdir(parents=True)

    commands = (
        (
            "schema-entrypoints",
            [
                "python3",
                "scripts/alignment/verify_playbook_schema_entrypoints.py",
                "--container",
                args.postgres_container,
                "--run-id",
                args.run_id,
            ],
        ),
        (
            "postgres-lifecycle",
            [
                "go",
                "-C",
                "go/control-plane",
                "test",
                "./internal/alert/api",
                "-run",
                "^TestPlaybookExecutionV2PostgresApprovalAndCancelIntegration$",
                "-count=1",
                "-v",
            ],
        ),
        (
            "alert-api",
            [
                "go",
                "-C",
                "go/control-plane",
                "test",
                "./internal/alert/api",
                "./internal/alert/consumer",
                "./internal/alert/config",
                "./cmd/alert-service",
                "-count=1",
            ],
        ),
        (
            "web-contracts",
            [
                "npm",
                "--prefix",
                "web/ui",
                "run",
                "test",
                "--",
                "--run",
                "src/services/playbookAutomationApi.test.ts",
                "src/services/pageApiPlans.test.ts",
            ],
        ),
        ("web-build", ["npm", "--prefix", "web/ui", "run", "build"]),
        (
            "alignment-regression",
            [
                "python3",
                "-m",
                "unittest",
                "tests.alignment.test_alignment_registry.AlignmentRegistryTest.test_playbook_execution_v2_is_real_approval_receipt_and_rollback_bound",
                "-v",
            ],
        ),
        ("openapi", ["python3", "scripts/alignment/check_openapi.py"]),
        ("event-catalog", ["python3", "scripts/alignment/check_event_catalog.py"]),
        ("migrations", ["python3", "scripts/alignment/check_migrations.py"]),
        ("registry", ["python3", "scripts/alignment/validate.py", "--strict-w1"]),
    )
    env = os.environ.copy()
    env[KAFKA_ENV] = args.kafka_broker
    results: list[dict[str, Any]] = []
    for name, command in commands:
        result = run_command(name, list(command), output, env)
        results.append(result)
        if result["status"] != "PASS":
            break

    source_after = build_snapshot()
    source_stable = source_before["content_sha256"] == source_after["content_sha256"]
    status = (
        "PASS"
        if len(results) == len(commands)
        and all(item["status"] == "PASS" for item in results)
        and source_stable
        else "FAIL"
    )
    sources = []
    for relative in SOURCE_ARTIFACTS:
        path = ROOT / relative
        if not path.is_file():
            raise SystemExit(f"source artifact does not exist: {relative}")
        sources.append(
            {
                "path": relative,
                "sha256": sha256(path),
                "size_bytes": path.stat().st_size,
            }
        )

    manifest = {
        "schema_version": 1,
        "run_id": args.run_id,
        "captured_at": datetime.now(timezone.utc).isoformat(),
        "feature_id": "F-PLAYBOOK-001",
        "related_ids": ["T-KAFKA-001", "T-PG-001", "T-SCHEMA-001"],
        "status": status,
        "candidate_source": source_before,
        "candidate_source_stable": source_stable,
        "g0_reference": {
            "run_id": g0.get("run_id"),
            "manifest": str(g0_manifest.relative_to(ROOT)),
            "manifest_sha256": sha256(g0_manifest),
            "status": g0.get("status"),
            "candidate_source_sha256": g0_source_sha,
        },
        "gate_status": {
            "G0": "PASS" if status == "PASS" else "FAIL",
            "G1": "PASS" if status == "PASS" else "FAIL",
            "G2": "PASS_EPHEMERAL_POSTGRES_AND_REAL_KAFKA",
            "G3": "OPEN_FOR_PRODUCTION_AND_CROSS_STORE_RECONCILIATION",
            "G4": "OPEN",
            "G5": "OPEN",
            "G6": "HOLD",
            "G7": "OPEN",
            "G8": "BLOCKED",
        },
        "production_applied": False,
        "commands": results,
        "source_artifacts": sources,
        "proven": [
            "live execution requires an approved enabled definition, stable idempotency, current definition version, explicit reason and real alert identity",
            "request, independent approval, cancellation, provider execution and independent compensation use optimistic workflow revisions",
            "cross-tenant reads, self-approval, self-compensation and run-budget overflow fail closed without creating execution side effects",
            "accepted HTTP responses do not imply provider success and an absent provider remains approved_awaiting_executor/not_configured",
            "provider execution and compensation persist exact per-step durable receipt identities and classify terminal state from those receipts",
            "business state, minimum audit and versioned outbox transition commit in one PostgreSQL transaction",
            "common SQL, Docker merged SQL and Kubernetes ConfigMap schema entrypoints replay twice to the same structural digest",
            "the versioned migrations replay twice and remain registered as 202608021000 and 202608021030",
            "the playbook.execution.events.v2 catalog, JSON Schema, topic bootstrap and default-off deployment configuration agree",
            "the leased outbox publisher marks an event published only after broker acknowledgement and moves exhausted deliveries to dead",
            "the strict consumer rejects topic, header, key and envelope drift and commits an immutable PostgreSQL event plus monotonic state projection",
            "a disposable loopback Kafka broker acknowledges all eight lifecycle messages before PostgreSQL marks the outbox rows published, and the exact broker records are read back into the immutable projection",
            "the Web UI binds a real alert ID and distinguishes request, approval, execution, failure, cancellation and compensation receipts",
        ],
        "open": [
            "approved real provider sandbox, credentials and partial/timeout/ambiguous-effect fault drills",
            "production Kafka topic/ACL deployment, consumer-group offsets and production reconciliation",
            "additional ClickHouse, OpenSearch and NebulaGraph terminal projections and reconciliation where required by consuming journeys",
            "execution concurrency, retry, queue age and provider latency performance budget",
            "production candidate deployment and Windows Chrome request-to-compensation journey with mock disabled",
            "canary rollback with in-flight work plus T+0/T+1/T+3/T+7 observation",
            "G7 independent sign-off and G8 external milestones",
        ],
        "secrets_captured": False,
    }
    manifest_path = output / "manifest.json"
    manifest_path.write_text(
        json.dumps(manifest, ensure_ascii=False, indent=2) + "\n",
        encoding="utf-8",
    )
    print(
        json.dumps(
            {
                "status": status,
                "manifest": str(manifest_path),
                "manifest_sha256": sha256(manifest_path),
            },
            ensure_ascii=False,
            indent=2,
        ),
        flush=True,
    )
    return 0 if status == "PASS" else 1


if __name__ == "__main__":
    raise SystemExit(main())
