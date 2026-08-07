#!/usr/bin/env python3
"""Capture immutable scoped evidence for the F-DASHBOARD-002 execution pipeline."""

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
SOURCE_ARTIFACTS = (
    "scripts/alignment/capture_dashboard_task_pipeline.py",
    "go/control-plane/internal/alert/api/dashboard_task_v2.go",
    "go/control-plane/internal/alert/api/dashboard_task_compensation.go",
    "go/control-plane/internal/alert/api/dashboard_task_v2_integration_test.go",
    "go/control-plane/internal/alert/api/dashboard_task_pipeline.go",
    "go/control-plane/internal/alert/api/dashboard_task_http_provider.go",
    "go/control-plane/internal/alert/api/dashboard_task_http_provider_test.go",
    "go/control-plane/cmd/alert-service/main.go",
    "contracts/alignment/features/F-DASHBOARD-002.json",
    "contracts/events/kafka-json-events-v1.schema.json",
    "contracts/events/kafka-topic-catalog.v1.json",
    "contracts/events/kafka-acl-catalog.v1.json",
    "contracts/openapi/alignment-v1.openapi.json",
    "deployments/postgres/migrations/202608031620_dashboard_task_v2.sql",
    "deployments/postgres/migrations/202608041930_dashboard_task_execution_pipeline_v1.sql",
    "deployments/postgres/migrations/202608082100_dashboard_task_compensation_v1.sql",
    "common/sql/pg/11-dashboard-task-v2.sql",
    "common/sql/pg/12-dashboard-task-execution-pipeline-v1.sql",
    "common/sql/pg/13-dashboard-task-compensation-v1.sql",
    "common/kafka/create-topics.sh",
    "deployments/kubernetes/init-jobs/00-kafka-acl-plan.yaml",
    "deployments/kubernetes/init-jobs/01-kafka-topics.yaml",
    "deployments/kubernetes/init-jobs/02-postgres-schema.yaml",
    "deployments/kubernetes/applications/go-services.yaml",
    "go/control-plane/deployments/kubernetes/alert-service.yaml",
    "go/control-plane/deployments/docker/init/postgres_merged.sql",
    "doc/07_alignment/runbooks/F-DASHBOARD-002-rollback.md",
    "tests/alignment/test_alignment_registry.py",
)


def sha256(path: Path) -> str:
    digest = hashlib.sha256()
    with path.open("rb") as source:
        for chunk in iter(lambda: source.read(1024 * 1024), b""):
            digest.update(chunk)
    return digest.hexdigest()


def run_command(name: str, command: list[str], output: Path) -> dict[str, Any]:
    log_path = output / f"{name}.log"
    started = datetime.now(timezone.utc)
    environment = os.environ.copy()
    environment["GOSUMDB"] = environment.get("TRAFFIC_GO_SUMDB") or "sum.golang.org"
    print(f"[dashboard-task-pipeline] starting {name}: {' '.join(command)}", flush=True)
    with log_path.open("wb") as log:
        completed = subprocess.run(
            command,
            cwd=ROOT,
            env=environment,
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
        "environment_overrides": {
            "GOSUMDB": environment["GOSUMDB"],
        },
    }
    print(f"[dashboard-task-pipeline] {name}: {result['status']}", flush=True)
    return result


def main() -> int:
    parser = argparse.ArgumentParser()
    parser.add_argument("--run-id", required=True)
    parser.add_argument("--g0-manifest", type=Path, required=True)
    parser.add_argument("--output-root", type=Path, default=DEFAULT_OUTPUT_ROOT)
    args = parser.parse_args()

    g0_manifest = args.g0_manifest.resolve()
    if not g0_manifest.is_file():
        raise SystemExit(f"G0 manifest does not exist: {g0_manifest}")
    g0 = json.loads(g0_manifest.read_text(encoding="utf-8"))
    if g0.get("gate") != "G0" or g0.get("status") != "PASS":
        raise SystemExit("referenced G0 manifest is not a PASS G0 result")

    candidate_before = build_snapshot()
    g0_hash = (g0.get("candidate_source") or {}).get("content_sha256")
    if not g0_hash or g0_hash != candidate_before["content_sha256"]:
        raise SystemExit("referenced G0 manifest does not cover the current candidate source")

    output = args.output_root.resolve() / args.run_id
    if output.exists():
        raise SystemExit(f"refusing to overwrite immutable evidence directory: {output}")
    output.mkdir(parents=True)

    commands = (
        (
            "dashboard-pipeline-go",
            [
                "go", "-C", "go/control-plane", "test",
                "./internal/alert/api", "./cmd/alert-service", "-count=1",
            ],
        ),
        (
            "sentinel-postgres-lifecycle",
            [
                "python3", "scripts/alignment/verify_asset_atomic_ephemeral.py",
                "--run-id", args.run_id + "-pg",
            ],
        ),
        (
            "schema-entrypoints",
            [
                "python3", "scripts/alignment/verify_pg_schema_entrypoints_ephemeral.py",
                "--run-id", args.run_id + "-schema",
            ],
        ),
        ("event-catalog", ["python3", "scripts/alignment/check_event_catalog.py"]),
        (
            "kafka-acl",
            ["python3", "-m", "unittest", "tests.alignment.test_kafka_acl_catalog", "-v"],
        ),
        ("migration-guard", ["python3", "scripts/alignment/check_migrations.py"]),
        (
            "dashboard-contract-regression",
            [
                "python3", "-m", "unittest",
                "tests.alignment.test_alignment_registry.AlignmentRegistryTest.test_dashboard_tasks_are_real_atomic_tenant_scoped_commands",
                "-v",
            ],
        ),
        ("strict-registry", ["python3", "scripts/alignment/validate.py", "--strict-w1"]),
    )

    results: list[dict[str, Any]] = []
    for name, command in commands:
        result = run_command(name, list(command), output)
        results.append(result)
        if result["status"] != "PASS":
            break

    candidate_after = build_snapshot()
    candidate_stable = candidate_before["content_sha256"] == candidate_after["content_sha256"]
    scoped_pass = (
        len(results) == len(commands)
        and all(item["status"] == "PASS" for item in results)
        and candidate_stable
    )

    artifacts = []
    for relative in SOURCE_ARTIFACTS:
        path = ROOT / relative
        if not path.is_file():
            raise SystemExit(f"source artifact does not exist: {relative}")
        artifacts.append(
            {"path": relative, "sha256": sha256(path), "size_bytes": path.stat().st_size}
        )

    manifest = {
        "schema_version": 1,
        "run_id": args.run_id,
        "captured_at": datetime.now(timezone.utc).isoformat(),
        "feature_id": "F-DASHBOARD-002",
        "related_ids": ["T-PG-002", "T-KAFKA-001", "T-SCHEMA-001"],
        "status": "PARTIAL" if scoped_pass else "FAIL",
        "coverage_status": "PARTIAL",
        "scoped_evidence_status": "PASS" if scoped_pass else "FAIL",
        "candidate_source": candidate_before,
        "candidate_source_stable": candidate_stable,
        "g0_reference": {
            "run_id": g0.get("run_id"),
            "manifest": str(g0_manifest.relative_to(ROOT)),
            "manifest_sha256": sha256(g0_manifest),
            "status": g0.get("status"),
            "candidate_source_sha256": g0_hash,
        },
        "gate_status": {
            "G0": "PASS" if scoped_pass else "FAIL",
            "G1": "PASS_FOR_REPOSITORY_COMPONENTS_AND_OWNED_EPHEMERAL_POSTGRESQL_LIFECYCLE",
            "G2": "OPEN_FOR_RELEASE_CANDIDATE_POSTGRESQL_KAFKA_AND_EXECUTOR",
            "G3": "OPEN_FOR_SAME_TRACE_TASK_EVENT_RECEIPT_AUDIT_AND_PROVIDER_EFFECT_RECONCILIATION",
            "G4": "OPEN_FOR_QUEUE_LAG_RETRY_EXECUTOR_P99_AND_RESOURCE_BUDGETS",
            "G5": "OPEN_FOR_CURRENT_CANDIDATE_WINDOWS_CHROME_MOCK_OFF",
            "G6": "HOLD_FOR_EXPAND_SHADOW_CANARY_ROLLBACK_AND_OBSERVATION",
            "G7": "OPEN",
            "G8": "BLOCKED",
        },
        "production_applied": False,
        "commands": results,
        "source_artifacts": artifacts,
        "proven": [
            "task acceptance atomically commits task history audit outbox and exact idempotency receipt in PostgreSQL",
            "the required-acks dispatcher marks an outbox row published only after Kafka acknowledgement and retains bounded retry and dead state",
            "the canonical dashboard.task.events.v1 envelope binds event tenant task trace schema aggregate revision partition key and deterministic event identity",
            "the lifecycle consumer validates topic key headers and body then records an idempotent PostgreSQL inbox before advancing authority",
            "the execution worker uses a stable provider idempotency key and stores the provider receipt task terminal history audit and result outbox atomically",
            "HTTP 2xx alone cannot produce completed; completed requires a durable confirmed receipt with stable effect IDs",
            "executor transport ambiguity becomes partial with unknown effect instead of false success or false definitive failure",
            "compensation acceptance requires an optimistic revision and a confirmed original provider receipt then atomically commits compensating history audit outbox and idempotency receipt",
            "compensated requires a durable confirmed provider receipt with stable effect identities; compensation transport ambiguity remains compensation_partial with unknown effect",
            "owned ephemeral PostgreSQL 16 exercises accepted published running executed terminal compensation-requested compensated result-published and result-consumed plus exact replay and cross-tenant denial",
            "common Docker and Kubernetes schema entrypoints replay twice to the same hash and include queue receipt and inbox tables",
            "the new topic JSON schema ACL workload identity and Kubernetes topic bootstrap catalogs are synchronized",
            "both task and pipeline flags remain default-off and the expand-only rollback preserves in-flight receipts and unknown effects",
        ],
        "open": [
            "run the exact release candidate against approved PostgreSQL Kafka and executor services",
            "prove duplicate outbox publish consumer restart Kafka replay and provider retry do not duplicate effects in the real chain",
            "prove compensation provider retry and lost-response recovery against a real approved provider without duplicating cleanup effects",
            "reconcile task history audit request/result events inbox offsets provider receipt and authoritative provider effect on one trace",
            "measure queue lag retry storm executor timeout P50/P95/P99 throughput and resource stop conditions",
            "capture the same production candidate bundle in designated Windows Chrome through the available external tunnel with mock disabled",
            "execute expand shadow canary rollback and T+0/T+1/T+3/T+7 observation with independent approval",
            "complete G7 sign-off while retaining independent G8 external milestones",
        ],
        "secrets_captured": False,
    }
    manifest_path = output / "manifest.json"
    manifest_path.write_text(
        json.dumps(manifest, ensure_ascii=False, indent=2) + "\n", encoding="utf-8"
    )
    print(
        json.dumps(
            {
                "status": manifest["status"],
                "scoped_evidence_status": manifest["scoped_evidence_status"],
                "manifest": str(manifest_path),
                "manifest_sha256": sha256(manifest_path),
            },
            ensure_ascii=False,
            indent=2,
        ),
        flush=True,
    )
    return 0 if scoped_pass else 1


if __name__ == "__main__":
    raise SystemExit(main())
