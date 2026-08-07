#!/usr/bin/env python3
"""Capture immutable repository-side T-CH-002 candidate evidence."""

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
    ("deterministic-sharding-contract", ["python3", "scripts/alignment/verify_clickhouse_deterministic_sharding.py"]),
    ("deterministic-sharding-negative-tests", ["python3", "-m", "unittest", "tests.alignment.test_clickhouse_deterministic_sharding", "-v"]),
    ("model-feedback-v2-go-tests", ["bash", "-lc", "cd go/control-plane && go test ./internal/rules/consumer ./internal/alert/api -count=1"]),
    ("schema-authority-regression", ["python3", "scripts/alignment/verify_clickhouse_schema_authority.py"]),
    ("strict-registry", ["python3", "scripts/alignment/validate.py", "--strict-w1"]),
)
SOURCE_ARTIFACTS = (
    "contracts/clickhouse/deterministic-sharding.v1.json",
    "deployments/clickhouse/migrations/202608031330_alert_feedback_v2.sql",
    "go/control-plane/internal/rules/config/config.go",
    "go/control-plane/internal/rules/consumer/model_feedback_inbox_worker.go",
    "go/control-plane/internal/rules/consumer/model_feedback_inbox_worker_test.go",
    "go/control-plane/internal/alert/api/feedback_repository.go",
    "go/control-plane/cmd/rule-manager/main.go",
    "deployments/kubernetes/applications/go-services.yaml",
    "scripts/alignment/verify_clickhouse_deterministic_sharding.py",
    "scripts/alignment/capture_clickhouse_deterministic_sharding.py",
    "tests/alignment/test_clickhouse_deterministic_sharding.py",
    "doc/07_alignment/runbooks/T-CH-002-deterministic-sharding-v2.md",
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
    print(f"[clickhouse-sharding] starting {name}: {' '.join(command)}", flush=True)
    with log_path.open("wb") as log:
        completed = subprocess.run(
            command, cwd=ROOT, stdout=log, stderr=subprocess.STDOUT, check=False
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
    print(f"[clickhouse-sharding] {name}: {result['status']}", flush=True)
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
    candidate_before = build_snapshot()
    g0_hash = (g0.get("candidate_source") or {}).get("content_sha256")
    if not g0_hash or candidate_before["content_sha256"] != g0_hash:
        raise SystemExit("current candidate does not match the referenced G0 manifest")

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
    scoped_status = (
        "PASS"
        if len(results) == len(COMMANDS) and all(item["status"] == "PASS" for item in results)
        else "FAIL"
    )
    candidate_after = build_snapshot()
    stable = candidate_before["content_sha256"] == candidate_after["content_sha256"]
    if not stable:
        scoped_status = "FAIL"

    sources = []
    for relative in SOURCE_ARTIFACTS:
        path = ROOT / relative
        if not path.is_file():
            raise SystemExit(f"source artifact does not exist: {relative}")
        sources.append({"path": relative, "sha256": sha256(path), "size_bytes": path.stat().st_size})

    manifest = {
        "schema_version": 1,
        "run_id": args.run_id,
        "captured_at": datetime.now(timezone.utc).isoformat(),
        "feature_id": "T-CH-002",
        "status": "PARTIAL" if scoped_status == "PASS" else "FAIL",
        "coverage_status": "PARTIAL_ALERT_FEEDBACK_V2_REPOSITORY_CANDIDATE",
        "scoped_evidence_status": scoped_status,
        "candidate_source": candidate_before,
        "candidate_source_stable": stable,
        "g0_reference": {
            "run_id": g0.get("run_id"),
            "manifest": str(g0_path.relative_to(ROOT)),
            "manifest_sha256": sha256(g0_path),
            "candidate_source_sha256": g0_hash,
        },
        "gate_status": {
            "G0": "PASS",
            "G1": "PASS_FOR_COMPLETE_18_TABLE_INVENTORY_EXPAND_ONLY_V2_AND_FAIL_CLOSED_DUAL_WRITE_CODE",
            "G2": "OPEN_FOR_PRODUCTION_EXPAND_DUAL_WRITE_AND_SHADOW_READ",
            "G3": "OPEN_FOR_BUSINESS_KEY_COUNT_HASH_SAMPLE_AND_REPLICA_RECONCILIATION",
            "G4": "OPEN_FOR_BACKFILL_SHADOW_QUERY_AND_RESOURCE_BUDGETS",
            "G5": "OPEN",
            "G6": "HOLD_FOR_CUTOVER_ROLLBACK_AND_OBSERVATION",
            "G7": "OPEN",
            "G8": "BLOCKED",
        },
        "production_applied": False,
        "commands": results,
        "source_artifacts": sources,
        "proven": [
            "all eighteen live Distributed tables are represented with writer reader business identity and target key ownership",
            "thirteen rand-sharded and five already deterministic tables remain explicitly distinguishable",
            "alert_feedback has an additive deterministic V2 migration with no data movement or destructive DDL",
            "V2 dual-write is disabled by default and an enabled V2 failure prevents PostgreSQL inbox acknowledgement",
            "legacy alert feedback code no longer exposes a direct alert_feedback_local write bypass",
        ],
        "open": [
            "apply expand migration on every node and capture replica health",
            "execute guarded dual-write and partition backfill",
            "reconcile business keys counts hashes and samples",
            "implement shadow reads and compare correctness and performance",
            "migrate the remaining twelve rand-sharded tables",
            "complete fault rollback observation independent gates and project G8",
        ],
        "secrets_captured": False,
    }
    manifest_path = output / "manifest.json"
    manifest_path.write_text(json.dumps(manifest, ensure_ascii=False, indent=2) + "\n", encoding="utf-8")
    print(json.dumps({
        "status": manifest["status"],
        "scoped_evidence_status": scoped_status,
        "manifest": str(manifest_path),
        "manifest_sha256": sha256(manifest_path),
    }, ensure_ascii=False, indent=2), flush=True)
    return 0 if scoped_status == "PASS" else 1


if __name__ == "__main__":
    raise SystemExit(main())
