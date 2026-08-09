#!/usr/bin/env python3
"""Capture immutable repository-side evidence for T-FLINK-005."""

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
    ("job-registry-contract", ["python3", "scripts/alignment/verify_flink_job_registry.py"]),
    (
        "job-registry-tests",
        [
            "python3", "-m", "unittest",
            "tests.alignment.test_flink_job_registry",
            "tests.alignment.test_flink_application_cluster_migration",
            "tests.alignment.test_flink_checkpoint_ha",
            "-v",
        ],
    ),
    ("application-artifacts", ["python3", "scripts/alignment/verify_flink_application_artifacts.py"]),
    (
        "flink-nine-job-tests",
        ["mvn", "-q", "-f", "java/flink-jobs/pom.xml", "test"],
    ),
)
SOURCE_ARTIFACTS = (
    "contracts/flink/job-registry.v1.json",
    "contracts/flink/application-cluster-migration.v1.json",
    "contracts/flink/state-recovery.v1.json",
    "contracts/flink/checkpoint-ha-upgrade.v1.json",
    "contracts/flink/sink-reconciliation.v1.json",
    "scripts/alignment/verify_flink_job_registry.py",
    "scripts/alignment/build_flink_job_release_registry.py",
    "scripts/alignment/capture_flink_job_registry.py",
    "scripts/alignment/render_flink_application_cluster.py",
    "scripts/alignment/verify_flink_application_artifacts.py",
    "tests/alignment/test_flink_job_registry.py",
    "tests/alignment/test_flink_application_cluster_migration.py",
    "tests/alignment/test_flink_checkpoint_ha.py",
    "doc/07_alignment/runbooks/T-FLINK-005-job-registry-rescale.md",
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
    print(f"[flink-job-registry] starting {name}: {' '.join(command)}", flush=True)
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
    print(f"[flink-job-registry] {name}: {result['status']}", flush=True)
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
    scoped_status = (
        "PASS"
        if len(results) == len(COMMANDS)
        and all(item["status"] == "PASS" for item in results)
        else "FAIL"
    )

    sources = []
    for relative in SOURCE_ARTIFACTS:
        path = ROOT / relative
        if not path.is_file():
            raise SystemExit(f"source artifact does not exist: {relative}")
        sources.append({"path": relative, "sha256": sha256(path), "size_bytes": path.stat().st_size})

    candidate = build_snapshot()
    g0_hash = (g0.get("candidate_source") or {}).get("content_sha256")
    candidate_matches = candidate.get("content_sha256") == g0_hash
    g0_status = "PASS" if candidate_matches else "STALE_REQUIRES_REFRESH_AFTER_REGISTRY_CHANGES"
    manifest = {
        "schema_version": 1,
        "run_id": args.run_id,
        "captured_at": datetime.now(timezone.utc).isoformat(),
        "feature_id": "T-FLINK-005",
        "related_ids": ["T-FLINK-001", "T-FLINK-002", "T-FLINK-003", "T-FLINK-004", "T-MINIO-004"],
        "status": "PARTIAL" if scoped_status == "PASS" else "FAIL",
        "coverage_status": "PARTIAL",
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
        "gate_status": {
            "G0": g0_status,
            "G1": "PASS_FOR_STATIC_JOB_REGISTRY_RELEASE_BINDING_RUNTIME_DIFF_AND_RESCALE_GUARDS" if scoped_status == "PASS" else "FAIL",
            "G2": "OPEN_FOR_DIGEST_BOUND_APPLICATION_CLUSTERS_AND_LIVE_REGISTRY",
            "G3": "OPEN_FOR_SAVEPOINT_KEY_GROUP_SOURCE_OFFSET_AND_SINK_RECONCILIATION",
            "G4": "OPEN_FOR_RESCALE_CHECKPOINT_BACKPRESSURE_AND_RTO_BUDGETS",
            "G5": "OPEN",
            "G6": "HOLD_FOR_APPROVED_SERIAL_UPGRADE_ROLLBACK_AND_OBSERVATION",
            "G7": "OPEN",
            "G8": "BLOCKED",
        },
        "production_applied": False,
        "commands": results,
        "source_artifacts": sources,
        "proven": [
            "the static registry covers exactly the same nine jobs as deployment, state and sink contracts",
            "all nine jobs bind owner, max parallelism and a deterministic hash of their stable operator UID set",
            "Application and rollback commands set pipeline.max-parallelism and reject values below current parallelism",
            "release materialization requires one immutable image, verified artifact and immutable savepoint per canonical job",
            "release validation rejects mutable images, missing jobs, UID drift and contract hash drift",
            "runtime diff blocks unknown, missing or field-mismatched jobs",
            "all nine shaded JARs and entry classes pass artifact verification",
        ],
        "open": [
            "build and publish nine candidate-bound image digests and materialize the real release registry",
            "deploy one approved Application Cluster at a time and capture the live runtime registry",
            "restore and rescale every stateful job while preserving max parallelism and key-group state",
            "reconcile checkpoint, savepoint, source offsets, watermarks, DLQ and sink business keys",
            "inject infrastructure and external sink failures and measure recovery budgets",
            "complete rollback, observation and independent G2 through G8 gates",
        ],
        "secrets_captured": False,
    }
    manifest_path = output / "manifest.json"
    manifest_path.write_text(json.dumps(manifest, ensure_ascii=False, indent=2) + "\n", encoding="utf-8")
    print(json.dumps({
        "status": manifest["status"],
        "coverage_status": manifest["coverage_status"],
        "scoped_evidence_status": scoped_status,
        "g0_status": g0_status,
        "manifest": str(manifest_path),
        "manifest_sha256": sha256(manifest_path),
    }, ensure_ascii=False, indent=2), flush=True)
    return 0 if scoped_status == "PASS" else 1


if __name__ == "__main__":
    raise SystemExit(main())
