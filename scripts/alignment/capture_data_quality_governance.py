#!/usr/bin/env python3
"""Capture immutable repository and ephemeral PostgreSQL evidence for T-DQ governance."""

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
    "contracts/data-quality/control-plane.v1.json",
    "contracts/alignment/features/F-DATAQUALITY-001.json",
    "contracts/openapi/alignment-v1.openapi.json",
    "deployments/postgres/migrations/202608041400_data_quality_control_plane_v1.sql",
    "deployments/postgres/migrations/202608041500_data_quality_governance_v1.sql",
    "deployments/postgres/migrations/202608041600_data_quality_rule_evaluation_v1.sql",
    "deployments/postgres/migrations/202608041700_data_quality_repair_lifecycle_v1.sql",
    "scripts/alignment/sync_data_quality_postgres_entrypoints.py",
    "scripts/alignment/verify_data_quality_control_plane.py",
    "scripts/alignment/verify_data_quality_governance_ephemeral.py",
    "go/control-plane/internal/common/dataquality/governance.go",
    "go/control-plane/internal/common/dataquality/governance_test.go",
    "go/control-plane/internal/common/dataquality/governance_integration_test.go",
    "go/control-plane/internal/common/dataquality/evaluation.go",
    "go/control-plane/internal/common/dataquality/evaluation_test.go",
    "go/control-plane/internal/common/dataquality/repair.go",
    "go/control-plane/internal/common/dataquality/repair_test.go",
    "go/control-plane/internal/common/dataquality/repair_evidence.go",
    "go/control-plane/internal/common/dataquality/repair_evidence_test.go",
    "go/control-plane/internal/common/dataquality/repair_executor.go",
    "go/control-plane/internal/common/dataquality/repair_replay_driver.go",
    "go/control-plane/internal/common/dataquality/repair_replay_driver_test.go",
    "go/control-plane/internal/alert/api/handler_data_quality_governance.go",
    "go/control-plane/internal/alert/api/handler_data_quality_governance_integration_test.go",
    "go/control-plane/internal/alert/api/handler_advanced.go",
    "go/control-plane/internal/alert/config/config.go",
    "go/control-plane/internal/alert/config/config_test.go",
    "go/control-plane/cmd/alert-service/main.go",
    "deployments/kubernetes/applications/go-services.yaml",
    "doc/07_alignment/runbooks/T-DQ-001-persistent-quality-control-plane.md",
    "tests/alignment/test_data_quality_control_plane.py",
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
    print(f"[data-quality-governance] starting {name}: {' '.join(command)}", flush=True)
    with log_path.open("wb") as log:
        completed = subprocess.run(
            command,
            cwd=ROOT,
            env=os.environ.copy(),
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
    print(f"[data-quality-governance] {name}: {result['status']}", flush=True)
    return result


def main() -> int:
    parser = argparse.ArgumentParser()
    parser.add_argument("--run-id", required=True)
    parser.add_argument("--output-root", type=Path, default=DEFAULT_OUTPUT_ROOT)
    args = parser.parse_args()

    output = args.output_root.resolve() / args.run_id
    if output.exists():
        raise SystemExit(f"refusing to overwrite immutable evidence directory: {output}")
    output.mkdir(parents=True)
    candidate_before = build_snapshot()
    commands = (
        ("generated-entrypoints", ["python3", "scripts/alignment/sync_data_quality_postgres_entrypoints.py", "--check"]),
        ("control-plane-contract", ["python3", "scripts/alignment/verify_data_quality_control_plane.py"]),
        ("alignment-tests", ["python3", "-m", "unittest", "tests.alignment.test_data_quality_control_plane", "-v"]),
        (
            "go-tests",
            ["go", "-C", "go/control-plane", "test", "./internal/common/dataquality", "./internal/alert/api", "-count=1"],
        ),
        (
            "ephemeral-postgresql-lifecycle",
            ["python3", "scripts/alignment/verify_data_quality_governance_ephemeral.py", "--run-id", args.run_id + "-pg"],
        ),
        ("openapi", ["python3", "scripts/alignment/check_openapi.py"]),
        ("migrations", ["python3", "scripts/alignment/check_migrations.py"]),
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
        artifacts.append({"path": relative, "sha256": sha256(path), "size_bytes": path.stat().st_size})

    manifest = {
        "schema_version": 1,
        "run_id": args.run_id,
        "captured_at": datetime.now(timezone.utc).isoformat(),
        "feature_id": "F-DATAQUALITY-001",
        "remediation_id": "T-DQ-001",
        "status": "PARTIAL" if scoped_pass else "FAIL",
        "scoped_evidence_status": "PASS" if scoped_pass else "FAIL",
        "scope": "G1_EPHEMERAL_POSTGRESQL_HTTP_AUTHORIZATION_RULE_APPROVAL_EVALUATION_AND_REPAIR_LIFECYCLE",
        "candidate_source": candidate_before,
        "candidate_source_stable": candidate_stable,
        "production_applied": False,
        "production_mutations": [],
        "shared_environment_touched": False,
        "commands": results,
        "source_artifacts": artifacts,
        "proven": [
            "tenant-scoped dataset optimistic upsert and exact idempotent replay",
            "rule draft to shadow to approval_pending to active is a bounded state machine",
            "the rule creator cannot approve or reject the same rule",
            "business state, immutable history, outbox, audit and command receipt commit atomically",
            "read and write scope checks plus cross-tenant list isolation hold at the HTTP boundary",
            "repository and HTTP lifecycles pass against owned ephemeral PostgreSQL 16",
            "approved active rules produce deterministic pass/fail evaluations and failure events atomically",
            "exact evaluation replay does not duplicate events, outbox rows or audit records",
            "bounded repair planning, server-derived dry-run state, independent approval, default-off execution, HTTP 202 acceptance and zero-difference reconciliation persist atomically",
            "missing server evidence provider or executor fails closed and client dry-run or reconcile summaries cannot advance state",
            "the production provider derives dry-run counts hashes and watermarks from persisted PostgreSQL scope and a bounded tenant ClickHouse query",
            "executing rows form a durable PostgreSQL work queue with session advisory locking and atomic executed or failed outcomes",
            "the replay driver rechecks row budget deduplicates stable event IDs preserves repair causation and refuses the raw ingest topic",
            "all disposable PostgreSQL resources are removed and no production migration is applied",
        ],
        "remaining_gates": [
            "full G0 rerun bound to this candidate source hash",
            "approved production migration and candidate alert-service rollout",
            "candidate rollout of the default-off evaluator and live quality event evidence",
            "candidate deployment of the server-derived dry-run provider and execution worker after a dedicated projection replay consumer is readiness-validated",
            "authoritative cross-store reconciliation and closure lifecycle",
            "cross-store reconciliation, performance, failure, Windows Chrome, rollback and observation evidence",
        ],
        "secrets_captured": False,
    }
    manifest_path = output / "manifest.json"
    manifest_path.write_text(json.dumps(manifest, ensure_ascii=False, indent=2) + "\n", encoding="utf-8")
    print(
        json.dumps(
            {
                "status": manifest["status"],
                "scoped_evidence_status": manifest["scoped_evidence_status"],
                "manifest": manifest_path.relative_to(ROOT).as_posix(),
                "manifest_sha256": sha256(manifest_path),
                "candidate_source_sha256": candidate_before["content_sha256"],
            },
            ensure_ascii=False,
            indent=2,
        ),
        flush=True,
    )
    return 0 if scoped_pass else 1


if __name__ == "__main__":
    raise SystemExit(main())
