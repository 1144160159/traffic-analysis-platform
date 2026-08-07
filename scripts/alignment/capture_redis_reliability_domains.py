#!/usr/bin/env python3
"""Capture immutable repository-side T-REDIS-001 safe-hold evidence."""

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
    ("redis-domain-contract", ["python3", "scripts/alignment/verify_redis_reliability_domains.py"]),
    ("redis-domain-negative-tests", ["python3", "-m", "unittest", "tests.alignment.test_redis_reliability_domains", "-v"]),
    ("redis-infrastructure-client-dry-run", ["kubectl", "apply", "--dry-run=client", "--validate=false", "-f", "deployments/kubernetes/infrastructure/04-redis.yaml", "-o", "name"]),
    ("strict-registry", ["python3", "scripts/alignment/validate.py", "--strict-w1"]),
)
SOURCE_ARTIFACTS = (
    "contracts/redis/reliability-domains.v1.json",
    "deployments/kubernetes/infrastructure/04-redis.yaml",
    "deployments/kubernetes/applications/go-services.yaml",
    "scripts/alignment/verify_redis_reliability_domains.py",
    "scripts/alignment/capture_redis_reliability_domains.py",
    "tests/alignment/test_redis_reliability_domains.py",
    "doc/07_alignment/runbooks/T-REDIS-001-reliability-domain-migration.md",
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
    print(f"[redis-domains] starting {name}: {' '.join(command)}", flush=True)
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
    print(f"[redis-domains] {name}: {result['status']}", flush=True)
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
    g0_hash = (g0.get("candidate_source") or {}).get("content_sha256")
    candidate_before = build_snapshot()
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
    scoped_status = "PASS" if len(results) == len(COMMANDS) and all(item["status"] == "PASS" for item in results) else "FAIL"
    candidate_after = build_snapshot()
    candidate_stable = candidate_before["content_sha256"] == candidate_after["content_sha256"]
    if not candidate_stable:
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
        "feature_id": "T-REDIS-001",
        "related_ids": ["T-REDIS-002", "T-OBS-001", "T-DR-001"],
        "status": "PARTIAL" if scoped_status == "PASS" else "FAIL",
        "coverage_status": "PARTIAL_SAFE_HOLD",
        "scoped_evidence_status": scoped_status,
        "candidate_source": candidate_before,
        "candidate_source_stable": candidate_stable,
        "g0_reference": {
            "run_id": g0.get("run_id"),
            "manifest": str(g0_path.relative_to(ROOT)),
            "manifest_sha256": sha256(g0_path),
            "status": g0.get("status"),
            "candidate_source_sha256": g0_hash,
        },
        "gate_status": {
            "G0": "PASS",
            "G1": "PASS_FOR_NOEVICTION_SAFE_HOLD_ISOLATED_CACHE_TARGET_AND_WORKLOAD_DB_BINDINGS" if scoped_status == "PASS" else "FAIL",
            "G2": "OPEN_FOR_RELEASE_CANDIDATE_EFFECTIVE_CONFIG_PREFIX_MIGRATION_AND_CACHE_BYPASS",
            "G3": "OPEN_FOR_ZERO_CROSS_DOMAIN_KEYS_AND_AUTHORITY_RECONCILIATION",
            "G4": "OPEN_FOR_EVICTION_STAMPEDE_MEMORY_AND_P99_BUDGETS",
            "G5": "OPEN",
            "G6": "HOLD_FOR_SERIAL_CUTOVER_FAILOVER_ROLLBACK_AND_OBSERVATION",
            "G7": "OPEN",
            "G8": "BLOCKED",
        },
        "production_applied": False,
        "commands": results,
        "source_artifacts": sources,
        "proven": [
            "reliable coordination master and replicas declare AOF and noeviction",
            "session workloads use a separate logical database on the reliable domain",
            "an isolated discardable allkeys-lru cache target exists without AOF or RDB persistence",
            "all six current production workload bindings declare a reliability domain and database",
            "mixed clients stay in the noeviction safe-hold until code-level client separation",
            "negative tests reject reliable-domain eviction, cache noeviction, session DB drift, legacy Service reuse and false completion",
        ],
        "open": [
            "split mixed rule-manager graph-service and forensics-service clients by semantic domain",
            "deploy the release candidate and capture effective Redis and Sentinel configuration",
            "inventory live key prefixes without values and prove zero cross-domain keys",
            "prove cache bypass and authoritative rebuild under eviction and outage",
            "run Sentinel failover reconnect session and coordination fault drills",
            "complete performance security rollout rollback observation and independent gates",
        ],
        "secrets_captured": False,
    }
    manifest_path = output / "manifest.json"
    manifest_path.write_text(json.dumps(manifest, ensure_ascii=False, indent=2) + "\n", encoding="utf-8")
    print(json.dumps({"status": manifest["status"], "scoped_evidence_status": scoped_status, "manifest": str(manifest_path), "manifest_sha256": sha256(manifest_path)}, ensure_ascii=False, indent=2), flush=True)
    return 0 if scoped_status == "PASS" else 1


if __name__ == "__main__":
    raise SystemExit(main())
