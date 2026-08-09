#!/usr/bin/env python3
"""Capture immutable scoped G1 evidence for F-ALERT-006 saved views."""

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
    "scripts/alignment/capture_alert_saved_view.py",
    "scripts/alignment/verify_alert_saved_view_ephemeral.py",
    "scripts/alignment/verify_pg_transaction_outbox.py",
    "scripts/alignment/inventory_pg_mutations.py",
    "scripts/alignment/check_openapi.py",
    "go/control-plane/internal/alert/api/handler_alert_actions.go",
    "go/control-plane/internal/alert/api/handler_alert_saved_view_outbox.go",
    "go/control-plane/internal/alert/api/handler_alert_saved_view_transaction_test.go",
    "go/control-plane/internal/alert/api/handler_alert_saved_view_postgres_integration_test.go",
    "go/control-plane/internal/alert/api/handler_alert_saved_view_outbox_test.go",
    "go/control-plane/internal/alert/config/config.go",
    "go/control-plane/internal/alert/config/config_test.go",
    "go/control-plane/cmd/alert-service/main.go",
    "contracts/alignment/features/F-ALERT-006.json",
    "contracts/alignment/feature-contract-registry.v1.json",
    "contracts/openapi/alignment-v1.openapi.json",
    "contracts/postgres/transaction-outbox.v1.json",
    "contracts/postgres/mutable-command-inventory.v1.json",
    "contracts/configuration/configuration-catalog.v1.json",
    "contracts/events/kafka-json-events-v1.schema.json",
    "contracts/events/kafka-topic-catalog.v1.json",
    "deployments/postgres/migrations/202608031100_alert_saved_view_transaction_v2.sql",
    "go/control-plane/deployments/docker/init/postgres_merged.sql",
    "deployments/kubernetes/applications/go-services.yaml",
    "go/control-plane/deployments/kubernetes/alert-service.yaml",
    "web/ui/src/pages/AlertTriagePage.tsx",
    "web/ui/src/services/alertTriageApi.ts",
    "web/ui/src/services/alertTriageApi.test.ts",
    "web/ui/src/generated/alignmentClient.ts",
    "doc/07_alignment/runbooks/T-PG-002-saved-view-transaction-outbox.md",
    "doc/07_alignment/runbooks/F-ALERT-006-rollback.md",
    "tests/alignment/test_alert_saved_view_ephemeral_guard.py",
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
    environment: dict[str, str],
) -> dict[str, Any]:
    log_path = output / f"{name}.log"
    started = datetime.now(timezone.utc)
    print(f"[alert-saved-view] starting {name}: {' '.join(command)}", flush=True)
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
    }
    print(f"[alert-saved-view] {name}: {result['status']}", flush=True)
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
        (
            "alert-saved-view-go",
            [
                "go", "-C", "go/control-plane", "test", "./internal/alert/api",
                "./internal/alert/config", "./cmd/alert-service", "-count=1",
            ],
        ),
        (
            "alert-saved-view-ui",
            [
                "npm", "--prefix", "web/ui", "test", "--", "--run",
                "src/services/alertTriageApi.test.ts",
            ],
        ),
        (
            "postgres-transaction-contract",
            ["python3", "scripts/alignment/verify_pg_transaction_outbox.py"],
        ),
        (
            "postgres-mutation-inventory",
            ["python3", "scripts/alignment/inventory_pg_mutations.py", "--check"],
        ),
        ("openapi-contract", ["python3", "scripts/alignment/check_openapi.py"]),
        (
            "configuration-catalog",
            ["python3", "scripts/alignment/build_configuration_catalog.py", "--check"],
        ),
        ("migration-guard", ["python3", "scripts/alignment/check_migrations.py"]),
        (
            "saved-view-evidence-guards",
            [
                "python3", "-m", "unittest",
                "tests.alignment.test_alert_saved_view_ephemeral_guard", "-v",
            ],
        ),
        (
            "owned-postgres-transaction",
            [
                "python3", "scripts/alignment/verify_alert_saved_view_ephemeral.py",
                "--run-id", args.run_id + "-postgres", "--output", str(owned_result_path),
            ],
        ),
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
    owned_result: dict[str, Any] | None = None
    if owned_result_path.is_file():
        try:
            owned_result = json.loads(owned_result_path.read_text(encoding="utf-8"))
        except json.JSONDecodeError:
            owned_result = None
    owned_facts = {
        "status_passed": bool(owned_result and owned_result.get("status") == "PASS"),
        "sentinel_passed": bool(
            owned_result
            and owned_result.get("sentinel_verified")
            and owned_result.get("cleanup_sentinel_verified")
        ),
        "identity_passed": bool(
            owned_result and owned_result.get("container_identity_verified")
        ),
        "tmpfs_passed": bool(
            owned_result
            and owned_result.get("data_mount_type") == "tmpfs"
            and not owned_result.get("persistent_volume_attached")
        ),
        "atomic_revision_facts_passed": bool(
            owned_result
            and owned_result.get("asserted_facts")
            and all(owned_result["asserted_facts"].values())
        ),
        "container_removed": bool(owned_result and owned_result.get("container_removed")),
    }
    scoped_pass = (
        len(results) == len(commands)
        and all(item["status"] == "PASS" for item in results)
        and candidate_stable
        and all(owned_facts.values())
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
        "feature_id": "F-ALERT-006",
        "related_ids": ["T-PG-002", "T-CONFIG-001", "T-SCHEMA-001"],
        "status": "PARTIAL" if scoped_pass else "FAIL",
        "coverage_status": "PARTIAL_OWNED_REAL_POSTGRES_ALERT_SAVED_VIEW_ATOMIC_REVISION_G1",
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
        "owned_postgres_result": (
            {
                "path": str(owned_result_path.relative_to(ROOT)),
                "sha256": sha256(owned_result_path),
                "status": owned_result.get("status"),
                "coverage_status": owned_result.get("coverage_status"),
                "production_applied": owned_result.get("production_applied"),
            }
            if owned_result and owned_result_path.is_file()
            else None
        ),
        "gate_status": {
            "G0": "PASS" if scoped_pass else "FAIL",
            "G1": "PASS_FOR_OWNED_SENTINEL_PROTECTED_POSTGRES_TRANSACTION",
            "G2": "OPEN_FOR_APPROVED_RELEASE_CANDIDATE_POSTGRES_AND_KAFKA",
            "G3": "OPEN_FOR_APPROVED_SAME_TRACE_POSTGRES_KAFKA_CONSUMER_RECONCILIATION",
            "G4": "OPEN_FOR_CONCURRENT_SAVE_PERFORMANCE_AND_FAULT_BUDGET",
            "G5": "OPEN_FOR_CURRENT_CANDIDATE_WINDOWS_CHROME_MOCK_OFF",
            "G6": "HOLD_FOR_EXPAND_SHADOW_CANARY_ROLLBACK_AND_OBSERVATION",
            "G7": "OPEN",
            "G8": "BLOCKED",
        },
        "commands": results,
        "source_artifacts": artifacts,
        "owned_postgres_facts": owned_facts,
        "production_applied": False,
        "proven": [
            "new saved views require expected revision zero while updates can bind the current revision and stale strict writes conflict before mutation",
            "legacy clients may omit expected_revision and retain their pre-rollout idempotency digest compatibility",
            "view state immutable history outbox audit and durable idempotency receipt commit in one serializable PostgreSQL transaction",
            "exact request replay returns the committed result while a changed payload under the same idempotency key conflicts",
            "an injected audit failure rolls back the view history outbox audit and request receipt together",
            "one trace reconciles the revisioned history outbox and audit facts in an owned fixed-digest sentinel-protected tmpfs PostgreSQL",
            "the outbox dispatcher rollout is default-off in Go configuration and both Kubernetes manifests while authoritative PostgreSQL saves remain enabled",
            "rollback is non-destructive and preserves saved views history outbox and idempotency receipts",
        ],
        "open": [
            "exercise the same candidate image and configuration hash against approved PostgreSQL and Kafka services",
            "add and verify an independently idempotent real saved-view Kafka consumer before enabling the dispatcher",
            "reconcile the same tenant trace event revision and watermarks across approved PostgreSQL Kafka consumer and audit services",
            "record concurrent-save lock contention latency throughput and failure-recovery budgets",
            "capture designated Windows Chrome mock-off cross-session restore conflict and error recovery evidence",
            "complete expand shadow canary rollback T+0 T+1 T+3 T+7 observation and independent G7 sign-off",
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
