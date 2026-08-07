#!/usr/bin/env python3
"""Capture immutable T-PG-002 repository inventory evidence."""

from __future__ import annotations

import argparse
import hashlib
import json
import subprocess
from datetime import datetime, timezone
from pathlib import Path
from typing import Any

from candidate_snapshot import build_snapshot


ROOT = Path(__file__).resolve().parents[2]
DEFAULT_OUTPUT_ROOT = ROOT / "doc/02_acceptance/runs"
COMMANDS = (
    ("pg-mutation-inventory", ["python3", "scripts/alignment/inventory_pg_mutations.py", "--check"]),
    ("pg-mutation-inventory-tests", ["python3", "-m", "unittest", "tests.alignment.test_pg_mutation_inventory", "-v"]),
    ("alignment-validate", ["make", "alignment-validate"]),
)
SOURCE_ARTIFACTS = (
    "contracts/postgres/mutable-command-inventory.v1.json",
    "contracts/postgres/transaction-outbox.v1.json",
    "scripts/alignment/inventory_pg_mutations.py",
    "scripts/alignment/capture_pg_mutation_inventory.py",
    "tests/alignment/test_pg_mutation_inventory.py",
    "go/control-plane/internal/alert/whitelist/command_atomic.go",
    "go/control-plane/internal/alert/whitelist/command_atomic_integration_test.go",
    "go/control-plane/cmd/threat-intel-service/command_atomic.go",
    "doc/07_alignment/runbooks/T-PG-002-mutable-command-inventory.md",
    "Makefile",
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
    print(f"[pg-mutation-inventory] starting {name}: {' '.join(command)}", flush=True)
    with log_path.open("wb") as log:
        completed = subprocess.run(command, cwd=ROOT, stdout=log, stderr=subprocess.STDOUT, check=False)
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
    print(f"[pg-mutation-inventory] {name}: {result['status']}", flush=True)
    return result


def main() -> int:
    parser = argparse.ArgumentParser()
    parser.add_argument("--run-id", required=True)
    parser.add_argument("--g0-manifest", type=Path, required=True)
    parser.add_argument("--output-root", type=Path, default=DEFAULT_OUTPUT_ROOT)
    args = parser.parse_args()

    g0_path = args.g0_manifest.resolve()
    if not g0_path.is_file():
        raise SystemExit(f"G0 manifest does not exist: {g0_path}")
    g0 = json.loads(g0_path.read_text(encoding="utf-8"))
    if g0.get("gate") != "G0" or g0.get("status") != "PASS":
        raise SystemExit("referenced G0 manifest is not PASS")

    output = args.output_root.resolve() / args.run_id
    if output.exists():
        raise SystemExit(f"refusing to overwrite immutable evidence directory: {output}")
    output.mkdir(parents=True)
    results: list[dict[str, Any]] = []
    for name, command in COMMANDS:
        result = run_command(name, list(command), output)
        results.append(result)
        if result["status"] != "PASS":
            break
    scoped_status = "PASS" if len(results) == len(COMMANDS) and all(item["status"] == "PASS" for item in results) else "FAIL"

    sources = []
    for relative in SOURCE_ARTIFACTS:
        path = ROOT / relative
        if not path.is_file():
            raise SystemExit(f"source artifact does not exist: {relative}")
        sources.append({"path": relative, "sha256": sha256(path), "size_bytes": path.stat().st_size})

    inventory = json.loads((ROOT / SOURCE_ARTIFACTS[0]).read_text(encoding="utf-8"))
    candidate = build_snapshot()
    g0_hash = (g0.get("candidate_source") or {}).get("content_sha256")
    candidate_matches = candidate.get("content_sha256") == g0_hash
    manifest = {
        "schema_version": 1,
        "run_id": args.run_id,
        "captured_at": datetime.now(timezone.utc).isoformat(),
        "feature_id": "T-PG-002",
        "related_ids": ["F-NOTIFICATION-001", "F-NOTIFY-001", "F-AUTH-001", "F-WHITELIST-001", "F-DASHBOARD-002"],
        "status": "PARTIAL" if scoped_status == "PASS" else "FAIL",
        "coverage_status": "REPOSITORY_STATIC_INVENTORY",
        "scoped_evidence_status": scoped_status,
        "candidate_source": candidate,
        "g0_reference": {
            "run_id": g0.get("run_id"),
            "manifest": str(g0_path.relative_to(ROOT)),
            "manifest_sha256": sha256(g0_path),
            "status": g0.get("status"),
            "candidate_content_sha256": g0_hash,
            "candidate_matches": candidate_matches,
        },
        "inventory_summary": inventory["summary"],
        "gate_status": {
            "G0": "PASS" if candidate_matches else "STALE_REQUIRES_REFRESH",
            "G1": "PASS_FOR_REPOSITORY_SQL_CLASSIFICATION_SNAPSHOT_AND_NEGATIVE_GUARDS" if scoped_status == "PASS" else "FAIL",
            "G2": "OPEN_FOR_REAL_POSTGRES_COMMAND_BOUNDARIES",
            "G3": "OPEN_FOR_TRANSACTION_OUTBOX_AND_CONSUMER_RECONCILIATION",
            "G4": "OPEN_FOR_LOCK_CONTENTION_AND_WRITE_THROUGHPUT_BUDGETS",
            "G5": "OPEN",
            "G6": "HOLD_FOR_MIGRATION_CANARY_ROLLBACK_AND_OBSERVATION",
            "G7": "OPEN",
            "G8": "BLOCKED",
        },
        "production_applied": False,
        "commands": results,
        "source_artifacts": sources,
        "proven": [
            "Go SQL string literals are classified deterministically as PostgreSQL, ClickHouse or fail-closed unclassified",
            "the snapshot freezes source, line, verb, table expression, backend, role and source-level transaction signals",
            "unknown dynamic tables and unknown qualified schemas are rejected by negative tests",
            "the review queue distinguishes source-level evidence gaps from confirmed transaction defects",
            "the legacy whitelist repository exposes no business mutation outside the governed command boundary",
            "exact whitelist and threat-intel audit helpers are recognized only after their transaction history outbox and idempotency signals are all present",
            f"the current review queue contains {inventory['summary']['review_queue']} sources: "
            f"{inventory['summary']['review_priority_counts'].get('P1_REVIEW', 0)} P1, "
            f"{inventory['summary']['review_priority_counts'].get('P2_REVIEW', 0)} P2, "
            f"{inventory['summary']['review_priority_counts'].get('P0_REVIEW', 0)} P0 and "
            f"{inventory['summary']['unclassified']} unclassified",
        ],
        "open": [
            "manually inspect every review-queue command boundary and record applicability decisions",
            f"review and remediate the remaining {inventory['summary']['review_priority_counts'].get('P1_REVIEW', 0)} P1 "
            f"and {inventory['summary']['review_priority_counts'].get('P2_REVIEW', 0)} P2 sources; the P0 review queue is empty",
            "operate and reconcile the notification governance outbox against release-candidate Kafka; the topic remains producer_only",
            "run real PostgreSQL transaction, trigger, lock, rollback and fault injection tests",
            "prove Kafka publication and idempotent consumer reconciliation where an outbox is required",
            "complete performance, browser, release, rollback, observation and external gates",
        ],
        "secrets_captured": False,
    }
    manifest_path = output / "manifest.json"
    manifest_path.write_text(json.dumps(manifest, ensure_ascii=False, indent=2) + "\n", encoding="utf-8")
    print(json.dumps({
        "status": manifest["status"],
        "scoped_evidence_status": scoped_status,
        "g0_status": manifest["gate_status"]["G0"],
        "manifest": str(manifest_path),
        "manifest_sha256": sha256(manifest_path),
    }, ensure_ascii=False, indent=2), flush=True)
    return 0 if scoped_status == "PASS" and candidate_matches else 1


if __name__ == "__main__":
    raise SystemExit(main())
