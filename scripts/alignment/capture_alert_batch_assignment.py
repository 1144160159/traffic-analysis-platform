#!/usr/bin/env python3
"""Capture candidate-bound scoped G1 evidence for F-ALERT-004."""

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
    "contracts/alignment/features/F-ALERT-004.json",
    "contracts/openapi/alignment-v1.openapi.json",
    "contracts/alignment/feature-contract-registry.v1.json",
    "contracts/configuration/configuration-catalog.v1.json",
    "go/control-plane/internal/alert/api/alert_batch_assignment_v1.go",
    "go/control-plane/internal/alert/api/alert_batch_assignment_v1_test.go",
    "go/control-plane/internal/alert/api/alert_batch_assignment_postgres_integration_test.go",
    "go/control-plane/cmd/alert-service/main.go",
    "deployments/postgres/migrations/202608091900_alert_batch_assignment_v1.sql",
    "common/sql/pg/19-alert-batch-assignment-v1.sql",
    "go/control-plane/deployments/docker/init/postgres_merged.sql",
    "deployments/kubernetes/init-jobs/02-postgres-schema.yaml",
    "deployments/kubernetes/applications/go-services.yaml",
    "go/control-plane/deployments/kubernetes/alert-service.yaml",
    "web/ui/src/config/runtime.ts",
    "web/ui/src/pages/AlertTriagePage.tsx",
    "web/ui/src/services/alertQueueActionsApi.ts",
    "web/ui/src/services/alertQueueActionsApi.test.ts",
    "web/ui/src/generated/alignmentClient.ts",
    "scripts/alignment/verify_alert_batch_assignment_ephemeral.py",
    "scripts/alignment/capture_alert_batch_assignment.py",
    "tests/alignment/test_alert_batch_assignment_ephemeral_guard.py",
    "doc/07_alignment/runbooks/F-ALERT-004-rollback.md",
)


def sha256(path: Path) -> str:
    digest = hashlib.sha256()
    with path.open("rb") as source:
        for chunk in iter(lambda: source.read(1024 * 1024), b""):
            digest.update(chunk)
    return digest.hexdigest()


def run_command(name: str, command: list[str], output: Path, environment: dict[str, str]) -> dict[str, Any]:
    log_path = output / f"{name}.log"
    started = datetime.now(timezone.utc)
    print(f"[alert-batch] starting {name}: {' '.join(command)}", flush=True)
    with log_path.open("wb") as log:
        completed = subprocess.run(command, cwd=ROOT, env=environment, stdout=log, stderr=subprocess.STDOUT, check=False)
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
    print(f"[alert-batch] {name}: {result['status']}", flush=True)
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
    environment = os.environ.copy()
    environment["GOSUMDB"] = environment.get("TRAFFIC_GO_SUMDB") or "sum.golang.org"
    owned_result_path = output / "owned-postgres-result.json"
    commands = (
        ("alert-batch-go", ["go", "-C", "go/control-plane", "test", "./internal/alert/api", "./cmd/alert-service", "-count=1"]),
        ("alert-batch-ui", ["npm", "--prefix", "web/ui", "test", "--", "--run", "src/services/alertQueueActionsApi.test.ts", "src/services/alertBatchApi.test.ts"]),
        ("openapi-contract", ["python3", "scripts/alignment/check_openapi.py"]),
        ("migration-guard", ["python3", "scripts/alignment/check_migrations.py"]),
        ("feature-contract-registry", ["python3", "scripts/alignment/build_feature_contract_registry.py", "--check"]),
        ("configuration-catalog", ["python3", "scripts/alignment/build_configuration_catalog.py", "--check"]),
        ("alert-batch-guards", ["python3", "-m", "unittest", "tests.alignment.test_alert_batch_assignment_ephemeral_guard", "-v"]),
        ("owned-postgres-frozen-batch", ["python3", "scripts/alignment/verify_alert_batch_assignment_ephemeral.py", "--run-id", args.run_id + "-owned", "--output", str(owned_result_path)]),
        ("strict-registry", ["python3", "scripts/alignment/validate.py", "--strict-w1"]),
    )
    results: list[dict[str, Any]] = []
    for name, command in commands:
        result = run_command(name, command, output, environment)
        results.append(result)
        if result["status"] != "PASS":
            break

    candidate_after = build_snapshot()
    candidate_stable = candidate_before["content_sha256"] == candidate_after["content_sha256"]
    owned_result = json.loads(owned_result_path.read_text(encoding="utf-8")) if owned_result_path.is_file() else None
    container = (owned_result or {}).get("container") or {}
    owned_facts = {
        "status_passed": bool(owned_result and owned_result.get("status") == "PASS"),
        "identity_and_cleanup_passed": bool(container.get("identity_verified") and container.get("cleanup_sentinel_verified") and container.get("removed")),
        "tmpfs_and_no_volume_passed": bool(container.get("data_mount_type") == "tmpfs" and not container.get("persistent_volume_attached")),
        "selection_assignment_atomicity_facts_passed": bool(owned_result and owned_result.get("asserted_facts") and all(owned_result["asserted_facts"].values())),
    }
    scoped_pass = len(results) == len(commands) and all(item["status"] == "PASS" for item in results) and candidate_stable and all(owned_facts.values())

    artifacts = []
    for relative in SOURCE_ARTIFACTS:
        path = ROOT / relative
        if not path.is_file():
            raise SystemExit(f"source artifact does not exist: {relative}")
        artifacts.append({"path": relative, "sha256": sha256(path), "size_bytes": path.stat().st_size})

    manifest = {
        "schema_version": 1,
        "run_id": args.run_id,
        "captured_at": datetime.now(timezone.utc).isoformat(),
        "feature_id": "F-ALERT-004",
        "related_ids": ["T-PG-002", "T-CONFIG-001", "T-SCHEMA-001"],
        "status": "PARTIAL" if scoped_pass else "FAIL",
        "coverage_status": "PARTIAL_OWNED_REAL_POSTGRES_FROZEN_ALERT_BATCH_ASSIGNMENT_G1",
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
        "owned_postgres_result": ({
            "path": str(owned_result_path.relative_to(ROOT)),
            "sha256": sha256(owned_result_path),
            "status": owned_result.get("status"),
            "coverage_status": owned_result.get("coverage_status"),
            "production_applied": owned_result.get("production_applied"),
        } if owned_result and owned_result_path.is_file() else None),
        "gate_status": {
            "G0": "PASS" if scoped_pass else "FAIL",
            "G1": "PASS_FOR_OWNED_SENTINEL_PROTECTED_POSTGRES_FROZEN_SELECTION_AND_ATOMIC_ACCEPTANCE",
            "G2": "OPEN_FOR_APPROVED_RELEASE_CANDIDATE_POSTGRES_KAFKA_CLICKHOUSE_AND_OPENSEARCH",
            "G3": "OPEN_FOR_APPROVED_BATCH_ITEM_OUTBOX_CONSUMER_PROJECTION_AND_AUDIT_RECONCILIATION",
            "G4": "OPEN_FOR_CONCURRENT_BATCH_RETRY_LOCK_OUTBOX_LAG_AND_PARTIAL_RESULT_BUDGET",
            "G5": "OPEN_FOR_CURRENT_CANDIDATE_WINDOWS_CHROME_MOCK_OFF_PARTIAL_RESULT_RECOVERY",
            "G6": "HOLD_FOR_EXPAND_SHADOW_CANARY_ROLLBACK_AND_OBSERVATION",
            "G7": "OPEN",
            "G8": "BLOCKED",
        },
        "commands": results,
        "source_artifacts": artifacts,
        "owned_postgres_facts": owned_facts,
        "production_applied": False,
        "proven": [
            "a tenant-bound expiring server-side token freezes snapshot identity ordered alert identities and expected state revisions before assignment acceptance",
            "selection and assignment commands each provide exact idempotent replay and changed-payload conflict under independent stable keys",
            "the tenant-bound selection bearer token is deterministically derived by a dedicated signing secret while PostgreSQL persists only its SHA-256 digest",
            "cross-tenant expired and already-consumed selection tokens fail without duplicate dispatch or target-count disclosure",
            "batch items immutable histories pending outbox durable request receipt and audit commit in one serializable PostgreSQL transaction under one trace",
            "an injected audit failure rolls back batch items histories outbox request receipt and selection consumption together",
            "202 and query responses keep accepted items partial and never claim that an assignment effect was applied",
            "the rollout remains default-off in the process Kubernetes and Web UI while the legacy per-alert assignment route is preserved",
        ],
        "open": [
            "implement and verify the independently idempotent alert.batch-assignment.requested.v1 consumer and per-item assignment receipts",
            "validate assignee authority and stale item revisions against approved authoritative services and expose explicit forbidden and conflicted results",
            "reconcile one batch across PostgreSQL Kafka ClickHouse OpenSearch and audit under the same tenant trace event revision snapshot and watermarks",
            "capture bounded 100-item concurrency retry lock outbox-lag partial failure and recovery budgets",
            "capture designated Windows Chrome mock-off accepted running partial failed retry and recovery states",
            "implement compensation for only items whose resulting revision still equals the batch revision",
            "complete expand shadow canary rollback T+0 T+1 T+3 T+7 observation and independent G7 sign-off",
        ],
        "secrets_captured": False,
    }
    manifest_path = output / "manifest.json"
    manifest_path.write_text(json.dumps(manifest, ensure_ascii=False, indent=2) + "\n", encoding="utf-8")
    print(json.dumps({"status": manifest["status"], "scoped_evidence_status": manifest["scoped_evidence_status"], "manifest": str(manifest_path), "manifest_sha256": sha256(manifest_path)}, ensure_ascii=False, indent=2), flush=True)
    return 0 if scoped_pass else 1


if __name__ == "__main__":
    raise SystemExit(main())
