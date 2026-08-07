#!/usr/bin/env python3
"""Capture immutable repository-side T-CH-004 writer-slice evidence."""

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
    ("append-only-contract", ["python3", "scripts/alignment/verify_clickhouse_append_only_semantics.py"]),
    ("append-only-negative-tests", ["python3", "-m", "unittest", "tests.alignment.test_clickhouse_append_only_semantics", "-v"]),
    ("alert-writer-go-tests", ["bash", "-lc", "cd go/control-plane && go test ./internal/alert/evidence ./internal/alert/repository ./internal/alert/persistence ./internal/alert/api -count=1"]),
    ("mutation-inventory", ["python3", "scripts/alignment/inventory_pg_mutations.py", "--check"]),
    ("strict-registry", ["python3", "scripts/alignment/validate.py", "--strict-w1"]),
)
SOURCES = (
    "contracts/clickhouse/append-only-version-semantics.v1.json",
    "go/control-plane/internal/alert/evidence/generator.go",
    "go/control-plane/internal/alert/evidence/auto_generator.go",
    "go/control-plane/internal/alert/repository/clickhouse.go",
    "go/control-plane/internal/alert/persistence/clickhouse.go",
    "scripts/alignment/verify_clickhouse_append_only_semantics.py",
    "scripts/alignment/capture_clickhouse_append_only_semantics.py",
    "tests/alignment/test_clickhouse_append_only_semantics.py",
    "doc/07_alignment/runbooks/T-CH-004-append-only-version-semantics.md",
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
    print(f"[clickhouse-append-only] starting {name}: {' '.join(command)}", flush=True)
    with log_path.open("wb") as log:
        completed = subprocess.run(command, cwd=ROOT, stdout=log, stderr=subprocess.STDOUT, check=False)
    result = {
        "name": name, "command": command, "exit_code": completed.returncode,
        "status": "PASS" if completed.returncode == 0 else "FAIL",
        "duration_seconds": round((datetime.now(timezone.utc) - started).total_seconds(), 3),
        "artifact": log_path.name, "sha256": sha256(log_path), "size_bytes": log_path.stat().st_size,
    }
    print(f"[clickhouse-append-only] {name}: {result['status']}", flush=True)
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
    g0 = json.loads(g0_path.read_text())
    if g0.get("gate") != "G0" or g0.get("status") != "PASS":
        raise SystemExit("referenced G0 manifest is not PASS")
    before = build_snapshot()
    g0_hash = (g0.get("candidate_source") or {}).get("content_sha256")
    if before["content_sha256"] != g0_hash:
        raise SystemExit("current candidate does not match the referenced G0 manifest")
    output = args.output_root.resolve() / args.run_id
    if output.exists():
        raise SystemExit(f"refusing to overwrite immutable evidence directory: {output}")
    output.mkdir(parents=True)
    results = []
    for name, command in COMMANDS:
        result = run_command(name, list(command), output)
        results.append(result)
        if result["status"] != "PASS":
            break
    scoped = "PASS" if len(results) == len(COMMANDS) and all(r["status"] == "PASS" for r in results) else "FAIL"
    after = build_snapshot()
    stable = before["content_sha256"] == after["content_sha256"]
    if not stable:
        scoped = "FAIL"
    artifacts = []
    for relative in SOURCES:
        path = ROOT / relative
        if not path.is_file():
            raise SystemExit(f"source artifact does not exist: {relative}")
        artifacts.append({"path": relative, "sha256": sha256(path), "size_bytes": path.stat().st_size})
    manifest = {
        "schema_version": 1, "run_id": args.run_id,
        "captured_at": datetime.now(timezone.utc).isoformat(), "feature_id": "T-CH-004",
        "status": "PARTIAL" if scoped == "PASS" else "FAIL",
        "coverage_status": "PARTIAL_GO_ALERT_EVIDENCE_WRITER_SLICE",
        "scoped_evidence_status": scoped, "candidate_source": before,
        "candidate_source_stable": stable, "production_applied": False,
        "g0_reference": {"run_id": g0.get("run_id"), "manifest": str(g0_path.relative_to(ROOT)), "manifest_sha256": sha256(g0_path), "candidate_source_sha256": g0_hash},
        "gate_status": {
            "G0": "PASS", "G1": "PASS_FOR_GO_ALERT_EVIDENCE_DISTRIBUTED_WRITERS_AND_NO_SYNC_MUTATION",
            "G2": "OPEN_FOR_RELEASE_CANDIDATE_ALL_WRITERS_AND_ROUTING_FAILURES",
            "G3": "OPEN_FOR_EVENT_VERSION_REPLAY_LATE_DATA_AND_TOMBSTONE_RECONCILIATION",
            "G4": "OPEN_FOR_WRITE_REPLAY_MUTATION_AND_RESOURCE_BUDGETS", "G5": "OPEN",
            "G6": "HOLD_FOR_WRITER_CANARY_ROLLBACK_AND_OBSERVATION", "G7": "OPEN", "G8": "BLOCKED",
        },
        "commands": results, "source_artifacts": artifacts,
        "proven": [
            "eight Go alert and evidence insert sites target logical Distributed tables",
            "the Go alert domain exposes no direct local-table insert and no synchronous ClickHouse update or delete mutation",
            "the unused evidence delete mutation operation is removed",
            "negative tests reject local writes synchronous mutations and false closure",
        ],
        "open": [
            "inventory and approve every Java Flink and maintenance writer",
            "complete event aggregate ingestion tombstone and late-data semantics",
            "run replay partial-failure shard replica and business-key reconciliation",
            "complete performance rollout rollback observation and independent gates",
        ],
        "secrets_captured": False,
    }
    path = output / "manifest.json"
    path.write_text(json.dumps(manifest, ensure_ascii=False, indent=2) + "\n")
    print(json.dumps({"status": manifest["status"], "scoped_evidence_status": scoped, "manifest": str(path), "manifest_sha256": sha256(path)}, indent=2), flush=True)
    return 0 if scoped == "PASS" else 1


if __name__ == "__main__":
    raise SystemExit(main())
