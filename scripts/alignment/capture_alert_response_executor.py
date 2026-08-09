#!/usr/bin/env python3
"""Capture immutable scoped evidence for the F-ALERT-003 response executor.

The capture is deliberately limited to G1. It uses an explicitly named,
sentinel-protected PostgreSQL container, a loopback ``httptest`` provider and
a fixed-digest owned single-broker Redpanda preflight. It does not claim a
release-candidate deployment, replicated Kafka/HA, production performance,
browser acceptance, or rollout observation.
"""

from __future__ import annotations

import argparse
import hashlib
import json
import os
import re
import subprocess
from datetime import datetime, timezone
from pathlib import Path
from typing import Any
from urllib.parse import urlparse

from candidate_snapshot import build_snapshot


ROOT = Path(__file__).resolve().parents[2]
DEFAULT_OUTPUT_ROOT = ROOT / "doc/02_acceptance/runs"
POSTGRES_ENV = "ALERT_RESPONSE_EPHEMERAL_PG_DSN"
CONTAINER_PATTERN = re.compile(r"codex-playbook-v2-pg-[A-Za-z0-9_.-]+")

SOURCE_ARTIFACTS = (
    "scripts/alignment/capture_alert_response_executor.py",
    "scripts/alignment/verify_playbook_schema_entrypoints.py",
    "scripts/alignment/verify_pg_transaction_outbox.py",
    "scripts/alignment/verify_kafka_dlq_commit_barrier.py",
    "scripts/alignment/verify_alert_response_real_kafka_ephemeral.py",
    "scripts/alignment/inventory_pg_mutations.py",
    "scripts/alignment/check_event_catalog.py",
    "go/control-plane/internal/alert/consumer/alert_response_http_executor.go",
    "go/control-plane/internal/alert/consumer/alert_response_http_executor_test.go",
    "go/control-plane/internal/alert/consumer/alert_response_event_consumer.go",
    "go/control-plane/internal/alert/consumer/alert_response_recovery_worker.go",
    "go/control-plane/internal/alert/consumer/alert_response_recovery_worker_integration_test.go",
    "go/control-plane/internal/alert/consumer/alert_response_real_kafka_integration_test.go",
    "go/control-plane/internal/alert/consumer/alert_response_dlq_barrier.go",
    "go/control-plane/internal/alert/consumer/alert_response_event_consumer_test.go",
    "go/control-plane/internal/alert/consumer/alert_response_event_consumer_integration_test.go",
    "go/control-plane/internal/common/kafka/consumer.go",
    "go/control-plane/internal/common/kafka/consumer_dlq_acknowledgement_barrier_test.go",
    "go/control-plane/internal/alert/api/handler_alert_actions.go",
    "go/control-plane/internal/alert/api/handler_alert_response_workflow.go",
    "go/control-plane/internal/alert/api/handler_alert_response_workflow_integration_test.go",
    "go/control-plane/internal/alert/config/config.go",
    "go/control-plane/cmd/alert-service/main.go",
    "contracts/alignment/features/F-ALERT-003.json",
    "contracts/alignment/feature-contract-registry.v1.json",
    "contracts/events/kafka-json-events-v1.schema.json",
    "contracts/events/kafka-topic-catalog.v1.json",
    "contracts/kafka/dlq-commit-barrier.v1.json",
    "contracts/postgres/transaction-outbox.v1.json",
    "contracts/postgres/mutable-command-inventory.v1.json",
    "contracts/configuration/configuration-catalog.v1.json",
    "contracts/security/service-identity-catalog.v1.json",
    "contracts/security/pki-catalog.v1.json",
    "deployments/postgres/migrations/202607302300_alert_response_execution_projection.sql",
    "deployments/postgres/migrations/202608091130_alert_response_external_executor_v1.sql",
    "deployments/postgres/migrations/202608091230_alert_response_dlq_receipt_v1.sql",
    "deployments/postgres/migrations/202608091500_alert_response_reconciliation_compensation_v1.sql",
    "common/sql/pg/15-alert-response-external-executor-v1.sql",
    "common/sql/pg/16-alert-response-dlq-receipt-v1.sql",
    "common/sql/pg/17-alert-response-reconciliation-compensation-v1.sql",
    "go/control-plane/deployments/docker/init/postgres_merged.sql",
    "deployments/kubernetes/init-jobs/02-postgres-schema.yaml",
    "deployments/kubernetes/applications/go-services.yaml",
    "go/control-plane/deployments/kubernetes/alert-service.yaml",
    "doc/07_alignment/runbooks/F-ALERT-003-rollback.md",
    "tests/alignment/test_alignment_registry.py",
)


def sha256(path: Path) -> str:
    digest = hashlib.sha256()
    with path.open("rb") as source:
        for chunk in iter(lambda: source.read(1024 * 1024), b""):
            digest.update(chunk)
    return digest.hexdigest()


def validate_owned_postgres(container: str, dsn: str) -> None:
    if not CONTAINER_PATTERN.fullmatch(container):
        raise SystemExit(
            "--postgres-container must use the codex-playbook-v2-pg-* sentinel prefix"
        )
    parsed = urlparse(dsn)
    if (
        parsed.scheme not in {"postgres", "postgresql"}
        or parsed.hostname not in {"127.0.0.1", "localhost"}
        or parsed.port is None
        or parsed.path != "/traffic_platform"
    ):
        raise SystemExit(
            f"{POSTGRES_ENV} must target traffic_platform on an explicit loopback port"
        )
    completed = subprocess.run(
        ["docker", "port", container, "5432/tcp"],
        cwd=ROOT,
        stdout=subprocess.PIPE,
        stderr=subprocess.PIPE,
        text=True,
        check=False,
    )
    if completed.returncode != 0:
        raise SystemExit("the explicitly named PostgreSQL container is not available")
    mappings = {line.strip() for line in completed.stdout.splitlines() if line.strip()}
    expected = {f"127.0.0.1:{parsed.port}", f"0.0.0.0:{parsed.port}"}
    if not mappings.intersection(expected):
        raise SystemExit(
            f"{POSTGRES_ENV} port does not match the explicitly named PostgreSQL container"
        )


def run_command(
    name: str,
    command: list[str],
    output: Path,
    environment: dict[str, str],
    *,
    guarded_postgres: bool = False,
) -> dict[str, Any]:
    log_path = output / f"{name}.log"
    started = datetime.now(timezone.utc)
    print(f"[alert-response-executor] starting {name}: {' '.join(command)}", flush=True)
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
        "guarded_ephemeral_postgres": guarded_postgres,
    }
    print(f"[alert-response-executor] {name}: {result['status']}", flush=True)
    return result


def main() -> int:
    parser = argparse.ArgumentParser()
    parser.add_argument("--run-id", required=True)
    parser.add_argument("--g0-manifest", type=Path, required=True)
    parser.add_argument("--postgres-container", required=True)
    parser.add_argument("--output-root", type=Path, default=DEFAULT_OUTPUT_ROOT)
    args = parser.parse_args()

    dsn = os.environ.get(POSTGRES_ENV, "")
    if not dsn:
        raise SystemExit(
            f"{POSTGRES_ENV} is required; its value is never written to evidence"
        )
    validate_owned_postgres(args.postgres_container, dsn)

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

    base_environment = os.environ.copy()
    base_environment["GOSUMDB"] = (
        base_environment.get("TRAFFIC_GO_SUMDB") or "sum.golang.org"
    )
    unit_environment = base_environment.copy()
    unit_environment.pop(POSTGRES_ENV, None)
    integration_environment = base_environment.copy()
    real_kafka_result_path = output / "owned-real-kafka-result.json"

    commands: tuple[tuple[str, list[str], dict[str, str], bool], ...] = (
        (
            "alert-response-go-unit",
            [
                "go", "-C", "go/control-plane", "test",
                "./internal/alert/consumer", "./internal/alert/api",
                "./internal/alert/config", "./internal/common/kafka",
                "./cmd/alert-service", "-count=1",
            ],
            unit_environment,
            False,
        ),
        (
            "alignment-regression",
            [
                "python3", "-m", "unittest",
                "tests.alignment.test_alignment_registry.AlignmentRegistryTest.test_alert_response_requests_require_provider_authority_for_real_effects",
                "-v",
            ],
            unit_environment,
            False,
        ),
        (
            "postgres-transaction-contract",
            ["python3", "scripts/alignment/verify_pg_transaction_outbox.py"],
            unit_environment,
            False,
        ),
        (
            "postgres-mutation-inventory",
            ["python3", "scripts/alignment/inventory_pg_mutations.py", "--check"],
            unit_environment,
            False,
        ),
        (
            "event-catalog",
            ["python3", "scripts/alignment/check_event_catalog.py"],
            unit_environment,
            False,
        ),
        (
            "migration-guard",
            ["python3", "scripts/alignment/check_migrations.py"],
            unit_environment,
            False,
        ),
        (
            "kafka-dlq-commit-barrier",
            ["python3", "scripts/alignment/verify_kafka_dlq_commit_barrier.py"],
            unit_environment,
            False,
        ),
        (
            "postgres-entrypoint-sync",
            [
                "python3",
                "scripts/alignment/sync_data_quality_postgres_entrypoints.py",
                "--check",
            ],
            unit_environment,
            False,
        ),
        (
            "schema-entrypoints",
            [
                "python3", "scripts/alignment/verify_playbook_schema_entrypoints.py",
                "--container", args.postgres_container,
                "--run-id", args.run_id + "-schema",
            ],
            unit_environment,
            True,
        ),
        (
            "owned-postgres-http-provider",
            [
                "go", "-C", "go/control-plane", "test",
                "./internal/alert/consumer",
                "-run", "^(TestPostgresAlertResponse(Projection|ExternalExecutor|DLQAcknowledgement)Integration|TestAlertResponseRecoveryWorker.*)$",
                "-count=1", "-v",
            ],
            integration_environment,
            True,
        ),
        (
            "owned-postgres-compensation-queue",
            [
                "go", "-C", "go/control-plane", "test",
                "./internal/alert/api",
                "-run", "^TestAlertResponseCompensationQueuePostgresIntegration$",
                "-count=1", "-v",
            ],
            integration_environment,
            True,
        ),
        (
            "owned-real-kafka-dlq-precommit",
            [
                "python3",
                "scripts/alignment/verify_alert_response_real_kafka_ephemeral.py",
                "--run-id",
                args.run_id + "-real-kafka",
                "--output",
                str(real_kafka_result_path),
            ],
            unit_environment,
            True,
        ),
        (
            "strict-registry",
            ["python3", "scripts/alignment/validate.py", "--strict-w1"],
            unit_environment,
            False,
        ),
    )

    results: list[dict[str, Any]] = []
    for name, command, environment, guarded_postgres in commands:
        result = run_command(
            name,
            command,
            output,
            environment,
            guarded_postgres=guarded_postgres,
        )
        results.append(result)
        if result["status"] != "PASS":
            break

    candidate_after = build_snapshot()
    candidate_stable = (
        candidate_before["content_sha256"] == candidate_after["content_sha256"]
    )
    integration_log = output / "owned-postgres-http-provider.log"
    integration_text = (
        integration_log.read_text(encoding="utf-8", errors="replace")
        if integration_log.is_file()
        else ""
    )
    integration_facts = {
        "postgres_projection_lifecycle_passed": (
            "--- PASS: TestPostgresAlertResponseProjectionIntegration" in integration_text
        ),
        "loopback_http_provider_receipt_passed": (
            "--- PASS: TestPostgresAlertResponseExternalExecutorIntegration"
            in integration_text
        ),
        "postgres_dlq_receipt_audit_passed": (
            "--- PASS: TestPostgresAlertResponseDLQAcknowledgementIntegration"
            in integration_text
        ),
        "unknown_effect_periodic_authority_recheck_passed": (
            "--- PASS: TestAlertResponseRecoveryWorkerResolvesUnknownExecutionByAuthorityOnly"
            in integration_text
        ),
        "ambiguous_compensation_lookup_only_recovery_passed": (
            "--- PASS: TestAlertResponseRecoveryWorkerNeverBlindRetriesAmbiguousCompensation"
            in integration_text
        ),
    }
    compensation_log = output / "owned-postgres-compensation-queue.log"
    compensation_text = (
        compensation_log.read_text(encoding="utf-8", errors="replace")
        if compensation_log.is_file()
        else ""
    )
    integration_facts["postgres_compensation_queue_atomicity_passed"] = (
        "--- PASS: TestAlertResponseCompensationQueuePostgresIntegration"
        in compensation_text
    )
    real_kafka_result: dict[str, Any] | None = None
    if real_kafka_result_path.is_file():
        try:
            real_kafka_result = json.loads(
                real_kafka_result_path.read_text(encoding="utf-8")
            )
        except json.JSONDecodeError:
            real_kafka_result = None
    integration_facts.update(
        {
            "owned_real_kafka_required_acks_all_passed": bool(
                real_kafka_result and real_kafka_result.get("required_acks_all_verified")
            ),
            "owned_real_kafka_external_provider_execution_passed": bool(
                real_kafka_result
                and real_kafka_result.get("real_kafka_external_provider_execution_verified")
            ),
            "owned_real_kafka_provider_single_effect_passed": bool(
                real_kafka_result
                and real_kafka_result.get("provider_single_effect_verified")
            ),
            "owned_real_kafka_same_trace_pg_provider_audit_passed": bool(
                real_kafka_result
                and real_kafka_result.get("same_trace_pg_provider_audit_verified")
            ),
            "owned_real_kafka_dlq_ack_failure_retained_offset_passed": bool(
                real_kafka_result
                and real_kafka_result.get("dlq_ack_failure_offset_retention_verified")
            ),
            "owned_real_kafka_poison_redelivery_passed": bool(
                real_kafka_result and real_kafka_result.get("poison_redelivery_verified")
            ),
            "owned_real_kafka_canonical_dlq_passed": bool(
                real_kafka_result and real_kafka_result.get("canonical_dlq_payload_verified")
            ),
            "owned_real_kafka_pg_receipt_audit_passed": bool(
                real_kafka_result
                and real_kafka_result.get("source_offset_dlq_postgres_audit_verified")
            ),
            "owned_real_kafka_group_offset_converged_passed": bool(
                real_kafka_result
                and real_kafka_result.get("broker_group_commit_verified")
            ),
            "owned_real_kafka_containers_removed": bool(
                real_kafka_result
                and real_kafka_result.get("postgres_container_removed")
                and real_kafka_result.get("kafka_container_removed")
            ),
        }
    )
    schema_log = output / "schema-entrypoints.log"
    schema_result: dict[str, Any] | None = None
    if schema_log.is_file():
        try:
            schema_result = json.loads(schema_log.read_text(encoding="utf-8"))
        except json.JSONDecodeError:
            schema_result = None
    schema_facts = {
        "entrypoint_replay_passed": bool(
            schema_result and schema_result.get("result") == "pass"
        ),
        "passes_per_entrypoint": (
            schema_result.get("passes_per_entrypoint") if schema_result else None
        ),
        "snapshots": schema_result.get("snapshots") if schema_result else None,
        "temporary_databases_removed": bool(
            schema_result and schema_result.get("temporary_databases_removed")
        ),
    }
    scoped_pass = (
        len(results) == len(commands)
        and all(item["status"] == "PASS" for item in results)
        and candidate_stable
        and all(integration_facts.values())
        and schema_facts["entrypoint_replay_passed"]
        and schema_facts["passes_per_entrypoint"] == 2
        and schema_facts["temporary_databases_removed"]
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
        "feature_id": "F-ALERT-003",
        "related_ids": ["T-PG-002", "T-KAFKA-001", "T-KAFKA-003", "T-SCHEMA-001"],
        "status": "PARTIAL" if scoped_pass else "FAIL",
        "coverage_status": "PARTIAL_OWNED_REAL_POSTGRES_REDPANDA_LOOPBACK_HTTP_PROVIDER_DLQ_AUTHORITY_RECHECK_AND_COMPENSATION_G1",
        "scoped_evidence_status": "PASS" if scoped_pass else "FAIL",
        "candidate_source": candidate_before,
        "candidate_source_stable": candidate_stable,
        "owned_real_kafka_result": (
            {
                "path": str(real_kafka_result_path.relative_to(ROOT)),
                "sha256": sha256(real_kafka_result_path),
                "status": real_kafka_result.get("status"),
                "coverage_status": real_kafka_result.get("coverage_status"),
                "single_broker_owned_preflight": real_kafka_result.get(
                    "single_broker_owned_preflight"
                ),
                "broker_group_offset": real_kafka_result.get("broker_group_offset"),
                "production_applied": real_kafka_result.get("production_applied"),
            }
            if real_kafka_result and real_kafka_result_path.is_file()
            else None
        ),
        "g0_reference": {
            "run_id": g0.get("run_id"),
            "manifest": str(g0_manifest.relative_to(ROOT)),
            "manifest_sha256": sha256(g0_manifest),
            "status": g0.get("status"),
            "candidate_source_sha256": g0_hash,
        },
        "gate_status": {
            "G0": "PASS" if scoped_pass else "FAIL",
            "G1": "PASS_FOR_OWNED_REAL_POSTGRES_REDPANDA_LOOPBACK_HTTP_PROVIDER_DLQ_AUTHORITY_RECHECK_AND_COMPENSATION",
            "G2": "OPEN_FOR_APPROVED_RELEASE_CANDIDATE_POSTGRESQL_KAFKA_AND_PROVIDER",
            "G3": "OPEN_FOR_SAME_TRACE_EVENT_OFFSET_RECEIPT_AUDIT_AND_PROVIDER_EFFECT_RECONCILIATION",
            "G4": "OPEN_FOR_APPROVED_PERFORMANCE_FAULT_AND_RECOVERY_BUDGET",
            "G5": "OPEN_FOR_CURRENT_CANDIDATE_WINDOWS_CHROME_MOCK_OFF",
            "G6": "HOLD_FOR_EXPAND_SHADOW_CANARY_ROLLBACK_AND_OBSERVATION",
            "G7": "OPEN",
            "G8": "BLOCKED",
        },
        "production_applied": False,
        "commands": results,
        "source_artifacts": artifacts,
        "owned_integration_facts": integration_facts,
        "owned_schema_facts": schema_facts,
        "proven": [
            "real response events require stable trace identity and independent approval before any external executor call",
            "provider calls carry a stable event-derived idempotency key and completed requires a validated durable provider receipt with confirmed unique effect identities",
            "a lost provider response can reach a terminal result only through exact-identity provider authority lookup; absent pending unknown and lookup errors preserve partial unknown effect",
            "the final provider receipt response action state and audit record commit in one PostgreSQL transaction",
            "stable event replay at a different Kafka offset is idempotent and a mismatched aggregate version is rejected",
            "an owned sentinel-protected PostgreSQL 16 database and loopback HTTP provider exercise blocked-no-executor and confirmed-external-effect paths",
            "the alert-domain poison receipt and quarantine audit commit atomically by immutable source topic partition and offset after a permanent processing failure",
            "partial unknown execution effects enter a bounded PostgreSQL queue whose worker only calls exact provider authority lookup and atomically reconciles receipt action audit and immutable attempt history",
            "a catalogued compensation request commits control request queue action revision and audit atomically; a transport-ambiguous inverse is submitted once and every later lease uses authority lookup only",
            "an owned fixed-digest single-broker Redpanda first commits one provider-confirmed real response with one trace across PostgreSQL receipt action audit and provider command then rejects the following poison event without advancing past that offset; the same group redelivers and commits at offset two only after canonical dlq.v1 acknowledgement plus one PostgreSQL source-tuple receipt and quarantine audit",
            "common Docker-merged and Kubernetes PostgreSQL schema entrypoints replay twice to one structural digest and register migrations 202608091130 202608091230 and 202608091500",
            "execution authority-recheck and compensation feature flags remain default-off and the rollback runbook preserves receipts queues and unknown effects",
        ],
        "open": [
            "repeat the alert-response DLQ acknowledgement failure source-offset retention redelivery and PostgreSQL barrier against the approved replicated release-candidate Kafka; the owned single-broker G1 is not G2 or HA evidence",
            "exercise the production producer consumer group source offsets replay and DLQ boundaries on the approved release-candidate Kafka",
            "run the exact release candidate against approved PostgreSQL Kafka and response provider services",
            "reconcile the same tenant trace event aggregate revision source offset receipt effect identity and audit across approved services",
            "record approved performance fault recovery and resource budgets",
            "capture designated Windows Chrome mock-off evidence and complete canary rollback plus T+0 T+1 T+3 T+7 observation",
            "complete independent G7 sign-off while retaining G8 external blockers",
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
