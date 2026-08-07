#!/usr/bin/env python3
"""Capture immutable T-CH-005 repository and read-only live evidence."""

from __future__ import annotations

import argparse
import hashlib
import json
import re
import subprocess
from datetime import datetime, timezone
from pathlib import Path
from typing import Any

from candidate_snapshot import build_snapshot


ROOT = Path(__file__).resolve().parents[2]
DEFAULT_OUTPUT_ROOT = ROOT / "doc/02_acceptance/runs"
COMMANDS = (
    ("retention-contract", ["python3", "scripts/alignment/verify_clickhouse_retention_lifecycle.py"]),
    ("retention-negative-tests", ["python3", "-m", "unittest", "tests.alignment.test_clickhouse_retention_lifecycle", "-v"]),
    ("migration-guard", ["python3", "scripts/alignment/check_migrations.py"]),
    ("minio-manifest-dry-run", ["kubectl", "apply", "--dry-run=client", "-f", "deployments/kubernetes/init-jobs/06-minio-lifecycle.yaml"]),
    ("strict-registry", ["python3", "scripts/alignment/validate.py", "--strict-w1"]),
)
SOURCES = (
    "contracts/clickhouse/retention-lifecycle.v1.json",
    "common/sql/ch/00-all-tables.sql",
    "go/control-plane/deployments/docker/init/clickhouse_merged.sql",
    "deployments/clickhouse/migrations/202608031600_sessions_daily_rollup_v1.sql",
    "deployments/kubernetes/init-jobs/06-minio-lifecycle.yaml",
    "scripts/alignment/verify_clickhouse_retention_lifecycle.py",
    "scripts/alignment/capture_clickhouse_retention_lifecycle.py",
    "tests/alignment/test_clickhouse_retention_lifecycle.py",
    "doc/07_alignment/runbooks/T-CH-005-retention-lifecycle-rollup.md",
    "Makefile",
)
LIVE_QUERY = """
SELECT hostName() AS host, name, engine, engine_full, create_table_query
FROM system.tables
WHERE database='traffic'
  AND name IN (
    'sessions_local',
    'pcap_index_local',
    'sessions_daily_rollup_v1_local',
    'sessions_daily_rollup_v1',
    'mv_sessions_daily_rollup_v1_local'
  )
ORDER BY name
FORMAT JSONEachRow
"""


def sha256(path: Path) -> str:
    digest = hashlib.sha256()
    with path.open("rb") as source:
        for chunk in iter(lambda: source.read(1024 * 1024), b""):
            digest.update(chunk)
    return digest.hexdigest()


def run(command: list[str]) -> subprocess.CompletedProcess[bytes]:
    return subprocess.run(command, cwd=ROOT, stdout=subprocess.PIPE, stderr=subprocess.PIPE, check=False)


def run_command(name: str, command: list[str], output: Path) -> dict[str, Any]:
    log_path = output / f"{name}.log"
    started = datetime.now(timezone.utc)
    print(f"[clickhouse-retention] starting {name}: {' '.join(command)}", flush=True)
    with log_path.open("wb") as log:
        completed = subprocess.run(command, cwd=ROOT, stdout=log, stderr=subprocess.STDOUT, check=False)
    result = {
        "name": name,
        "command": command,
        "exit_code": completed.returncode,
        "status": "PASS" if completed.returncode == 0 else "FAIL",
        "duration_seconds": round((datetime.now(timezone.utc) - started).total_seconds(), 3),
        "artifact": log_path.name,
        "sha256": sha256(log_path),
        "size_bytes": log_path.stat().st_size,
    }
    print(f"[clickhouse-retention] {name}: {result['status']}", flush=True)
    return result


def json_lines(payload: bytes) -> list[dict[str, Any]]:
    return [json.loads(line) for line in payload.decode("utf-8").splitlines() if line.strip()]


def ttl_days(query: str) -> int | None:
    match = re.search(r"(?:toIntervalDay\(|INTERVAL\s+)(\d+)(?:\)|\s+DAY)", query, re.I)
    return int(match.group(1)) if match else None


def capture_live(output: Path, namespace: str, selector: str) -> tuple[dict[str, Any], list[dict[str, Any]]]:
    artifacts: list[dict[str, Any]] = []
    errors: list[dict[str, Any]] = []
    pods_result = run([
        "kubectl", "--request-timeout=15s", "-n", namespace,
        "get", "pods", "-l", selector, "-o", "json",
    ])
    pods_path = output / "clickhouse-pods.json"
    pods_stderr = output / "clickhouse-pods.stderr.log"
    pods_path.write_bytes(pods_result.stdout)
    pods_stderr.write_bytes(pods_result.stderr)
    artifacts.extend([
        {"path": pods_path.name, "sha256": sha256(pods_path), "size_bytes": pods_path.stat().st_size},
        {"path": pods_stderr.name, "sha256": sha256(pods_stderr), "size_bytes": pods_stderr.stat().st_size},
    ])
    pods: list[str] = []
    if pods_result.returncode == 0:
        payload = json.loads(pods_result.stdout)
        pods = sorted(
            item["metadata"]["name"]
            for item in payload.get("items", [])
            if item.get("status", {}).get("phase") == "Running"
        )
    if not pods:
        errors.append({"scope": "clickhouse-pods", "error": "no running ClickHouse pods"})

    tables_by_pod: dict[str, dict[str, dict[str, Any]]] = {}
    for pod in pods:
        result = run([
            "kubectl", "--request-timeout=30s", "-n", namespace,
            "exec", "-c", "clickhouse", pod, "--", "sh", "-lc",
            'exec clickhouse-client --password "$CLICKHOUSE_PASSWORD" --query "$1"',
            "capture-clickhouse-retention", LIVE_QUERY.strip(),
        ])
        data_path = output / f"{pod}-retention-tables.jsonl"
        stderr_path = output / f"{pod}-retention-tables.stderr.log"
        data_path.write_bytes(result.stdout)
        stderr_path.write_bytes(result.stderr)
        artifacts.extend([
            {"path": data_path.name, "sha256": sha256(data_path), "size_bytes": data_path.stat().st_size},
            {"path": stderr_path.name, "sha256": sha256(stderr_path), "size_bytes": stderr_path.stat().st_size},
        ])
        if result.returncode != 0:
            errors.append({"scope": pod, "error": "ClickHouse retention query failed", "exit_code": result.returncode})
            continue
        rows = json_lines(result.stdout)
        tables_by_pod[pod] = {str(row["name"]): row for row in rows}

    minio_result = run([
        "kubectl", "--request-timeout=15s", "-n", "minio", "get", "configmap",
        "minio-lifecycle-policy", "-o", "json",
    ])
    minio_path = output / "minio-lifecycle-configmap.json"
    minio_stderr = output / "minio-lifecycle-configmap.stderr.log"
    minio_path.write_bytes(minio_result.stdout)
    minio_stderr.write_bytes(minio_result.stderr)
    artifacts.extend([
        {"path": minio_path.name, "sha256": sha256(minio_path), "size_bytes": minio_path.stat().st_size},
        {"path": minio_stderr.name, "sha256": sha256(minio_stderr), "size_bytes": minio_stderr.stat().st_size},
    ])
    pcap_object_days: int | None = None
    if minio_result.returncode == 0:
        configmap = json.loads(minio_result.stdout)
        lifecycle_text = configmap.get("data", {}).get("pcap-archive-lifecycle.json", "")
        lifecycle = json.loads(lifecycle_text) if lifecycle_text else {}
        rules = lifecycle.get("Rules", [])
        if len(rules) == 1:
            pcap_object_days = int(rules[0].get("Expiration", {}).get("Days", 0))
    else:
        errors.append({"scope": "minio-lifecycle", "error": "ConfigMap read failed", "exit_code": minio_result.returncode})

    per_pod = {}
    for pod, values in tables_by_pod.items():
        per_pod[pod] = {
            "sessions_days": ttl_days(str(values.get("sessions_local", {}).get("create_table_query", ""))),
            "pcap_index_days": ttl_days(str(values.get("pcap_index_local", {}).get("create_table_query", ""))),
            "rollup_local_present": "sessions_daily_rollup_v1_local" in values,
            "rollup_distributed_present": "sessions_daily_rollup_v1" in values,
            "rollup_view_present": "mv_sessions_daily_rollup_v1_local" in values,
            "rollup_days": ttl_days(str(values.get("sessions_daily_rollup_v1_local", {}).get("create_table_query", ""))),
        }
    live = {
        "read_only": True,
        "pods": pods,
        "per_pod": per_pod,
        "pcap_object_days": pcap_object_days,
        "candidate_pcap_object_days": 37,
        "candidate_rollup_days": 365,
        "candidate_applied": bool(per_pod)
        and all(item["rollup_local_present"] and item["rollup_distributed_present"] and item["rollup_view_present"] for item in per_pod.values())
        and pcap_object_days == 37,
        "errors": errors,
    }
    return live, artifacts


def main() -> int:
    parser = argparse.ArgumentParser()
    parser.add_argument("--run-id", required=True)
    parser.add_argument("--g0-manifest", type=Path, required=True)
    parser.add_argument("--namespace", default="middleware")
    parser.add_argument("--selector", default="app=clickhouse")
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
    repository_pass = len(results) == len(COMMANDS) and all(item["status"] == "PASS" for item in results)
    live, live_artifacts = capture_live(output, args.namespace, args.selector) if repository_pass else ({"errors": [{"error": "skipped after repository failure"}]}, [])
    scoped = "PASS" if repository_pass and not live.get("errors") else "FAIL"
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
        "feature_id": "T-CH-005",
        "status": "PARTIAL" if scoped == "PASS" else "FAIL",
        "coverage_status": "PARTIAL_REPOSITORY_RETENTION_MATRIX_AND_READ_ONLY_LIVE_DRIFT",
        "scoped_evidence_status": scoped,
        "candidate_source": before,
        "candidate_source_stable": stable,
        "production_applied": False,
        "read_only_live_capture": True,
        "g0_reference": {
            "run_id": g0.get("run_id"),
            "manifest": str(g0_path.relative_to(ROOT)),
            "manifest_sha256": sha256(g0_path),
            "candidate_source_sha256": g0_hash,
        },
        "gate_status": {
            "G0": "PASS",
            "G1": "PASS_FOR_RETENTION_MATRIX_PCAP_GRACE_SESSION_TTL_AND_VERSIONED_ROLLUP_CANDIDATE",
            "G2": "PARTIAL_READ_ONLY_PRE_ROLLOUT_TTL_AND_MINIO_LIFECYCLE_CAPTURE",
            "G3": "OPEN_FOR_DETAIL_ROLLUP_OBJECT_REFERENCE_AND_LATE_EVENT_RECONCILIATION",
            "G4": "OPEN_FOR_CAPACITY_TTL_MERGE_BACKFILL_QUERY_AND_RESOURCE_BUDGETS",
            "G5": "OPEN",
            "G6": "HOLD_FOR_APPROVED_LIFECYCLE_IMPORT_BACKFILL_CANARY_ROLLBACK_AND_OBSERVATION",
            "G7": "OPEN",
            "G8": "BLOCKED",
        },
        "commands": results,
        "live_observation": live,
        "source_artifacts": source_artifacts,
        "live_artifacts": live_artifacts,
        "proven": [
            "repository retention matrix covers raw flow session alert audit PCAP aggregate topic snapshot and report object domains",
            "PCAP object candidate retention covers index retention plus a seven-day grace window",
            "common and Docker session TTL definitions agree at ninety days",
            "the expand-only daily session rollup carries aggregate_version one and a 365-day TTL",
            "current ClickHouse TTL and MinIO lifecycle state were captured read-only from the live cluster",
        ],
        "open": [
            "storage and compliance approval for PCAP 37-day retention",
            "production lifecycle import and rollup migration",
            "bounded historical backfill detail-rollup reconciliation late event and replay proof",
            "complete TTL and object-class inventory explicit expired states performance rollback and observation",
        ],
        "secrets_captured": False,
    }
    path = output / "manifest.json"
    path.write_text(json.dumps(manifest, ensure_ascii=False, indent=2) + "\n")
    print(json.dumps({
        "status": manifest["status"],
        "scoped_evidence_status": scoped,
        "live_candidate_applied": live.get("candidate_applied"),
        "manifest": str(path),
        "manifest_sha256": sha256(path),
    }, indent=2), flush=True)
    return 0 if scoped == "PASS" else 1


if __name__ == "__main__":
    raise SystemExit(main())
