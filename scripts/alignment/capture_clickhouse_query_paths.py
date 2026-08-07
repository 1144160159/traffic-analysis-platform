#!/usr/bin/env python3
"""Capture immutable repository-side T-CH-003 query-slice evidence."""

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
    ("query-path-contract", ["python3", "scripts/alignment/verify_clickhouse_query_paths.py"]),
    ("query-path-negative-tests", ["python3", "-m", "unittest", "tests.alignment.test_clickhouse_query_paths", "-v"]),
    ("alert-query-go-tests", ["bash", "-lc", "cd go/control-plane && go test ./internal/alert/repository ./internal/alert/api -count=1"]),
    ("strict-registry", ["python3", "scripts/alignment/validate.py", "--strict-w1"]),
)
SOURCES = (
    "contracts/clickhouse/query-path-optimization.v1.json",
    "go/control-plane/internal/alert/repository/clickhouse.go",
    "go/control-plane/internal/alert/repository/clickhouse_list_test.go",
    "scripts/alignment/verify_clickhouse_query_paths.py",
    "scripts/alignment/capture_clickhouse_query_paths.py",
    "tests/alignment/test_clickhouse_query_paths.py",
    "doc/07_alignment/runbooks/T-CH-003-query-path-optimization.md",
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
    print(f"[clickhouse-query] starting {name}: {' '.join(command)}", flush=True)
    with log_path.open("wb") as log:
        completed = subprocess.run(command, cwd=ROOT, stdout=log, stderr=subprocess.STDOUT, check=False)
    finished = datetime.now(timezone.utc)
    result = {
        "name": name,
        "command": command,
        "exit_code": completed.returncode,
        "status": "PASS" if completed.returncode == 0 else "FAIL",
        "duration_seconds": round((finished - started).total_seconds(), 3),
        "artifact": log_path.name,
        "sha256": sha256(log_path),
        "size_bytes": log_path.stat().st_size,
    }
    print(f"[clickhouse-query] {name}: {result['status']}", flush=True)
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
    before = build_snapshot()
    g0_hash = (g0.get("candidate_source") or {}).get("content_sha256")
    if not g0_hash or before["content_sha256"] != g0_hash:
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
    scoped = "PASS" if len(results) == len(COMMANDS) and all(r["status"] == "PASS" for r in results) else "FAIL"
    after = build_snapshot()
    stable = before["content_sha256"] == after["content_sha256"]
    if not stable:
        scoped = "FAIL"

    source_artifacts = []
    for relative in SOURCES:
        path = ROOT / relative
        if not path.is_file():
            raise SystemExit(f"source artifact does not exist: {relative}")
        source_artifacts.append({"path": relative, "sha256": sha256(path), "size_bytes": path.stat().st_size})

    manifest = {
        "schema_version": 1,
        "run_id": args.run_id,
        "captured_at": datetime.now(timezone.utc).isoformat(),
        "feature_id": "T-CH-003",
        "status": "PARTIAL" if scoped == "PASS" else "FAIL",
        "coverage_status": "PARTIAL_ALERT_LIST_STRUCTURED_ROWS",
        "scoped_evidence_status": scoped,
        "candidate_source": before,
        "candidate_source_stable": stable,
        "g0_reference": {
            "run_id": g0.get("run_id"),
            "manifest": str(g0_path.relative_to(ROOT)),
            "manifest_sha256": sha256(g0_path),
            "candidate_source_sha256": g0_hash,
        },
        "production_applied": False,
        "gate_status": {
            "G0": "PASS",
            "G1": "PASS_FOR_BOUNDED_STRUCTURED_ALERT_ROWS_AND_SEMANTIC_REGRESSION_GUARDS",
            "G2": "OPEN_FOR_RELEASE_CANDIDATE_QUERY_LOG_AND_RESULT_COMPARISON",
            "G3": "OPEN_FOR_LATEST_COUNT_CURSOR_AND_SHARD_RECONCILIATION",
            "G4": "OPEN_FOR_COLD_WARM_P50_P95_P99_AND_RESOURCE_BUDGETS",
            "G5": "OPEN",
            "G6": "HOLD_FOR_QUERY_CANARY_ROLLBACK_AND_OBSERVATION",
            "G7": "OPEN",
            "G8": "BLOCKED",
        },
        "commands": results,
        "source_artifacts": source_artifacts,
        "proven": [
            "the alert list no longer serializes a bounded page through ClickHouse JSON aggregation and Go JSON decoding",
            "the page remains bounded to one thousand typed rows",
            "tenant time FINAL exact-count offset sort and attack-phase semantics remain explicit",
            "negative tests reject JSON aggregation regression false closure production claims and disabling FINAL reconciliation guards",
        ],
        "open": [
            "complete the full query-pattern inventory",
            "add compatible stable cursors and decouple exact counts",
            "prove alternatives to FINAL with business-key reconciliation",
            "capture release-candidate query_log profiles correctness and resource budgets",
            "complete failure canary rollback observation and independent gates",
        ],
        "secrets_captured": False,
    }
    path = output / "manifest.json"
    path.write_text(json.dumps(manifest, ensure_ascii=False, indent=2) + "\n", encoding="utf-8")
    print(json.dumps({
        "status": manifest["status"], "scoped_evidence_status": scoped,
        "manifest": str(path), "manifest_sha256": sha256(path),
    }, ensure_ascii=False, indent=2), flush=True)
    return 0 if scoped == "PASS" else 1


if __name__ == "__main__":
    raise SystemExit(main())
