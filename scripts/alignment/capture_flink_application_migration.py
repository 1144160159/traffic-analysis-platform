#!/usr/bin/env python3
"""Capture immutable G1 evidence for the guarded Flink Application migration."""

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
    ("contract", ["python3", "scripts/alignment/render_flink_application_cluster.py", "--check-contract"]),
    (
        "migration-tests",
        [
            "python3", "-m", "unittest",
            "tests.alignment.test_flink_application_cluster_migration",
            "tests.alignment.test_verify_flink_application_clusters", "-v",
        ],
    ),
    ("identity-sync", ["bash", "tests/alignment/test_kafka_service_identity_sync.sh"]),
    ("java-package", ["mvn", "-q", "-f", "java/flink-jobs/pom.xml", "package", "-DskipTests"]),
    ("java-artifacts", ["python3", "scripts/alignment/verify_flink_application_artifacts.py"]),
)
SOURCE_ARTIFACTS = (
    "contracts/flink/application-cluster-migration.v1.json",
    "contracts/flink/savepoint-manifest.v1.schema.json",
    "contracts/events/kafka-acl-catalog.v1.json",
    "scripts/alignment/render_flink_application_cluster.py",
    "scripts/alignment/verify_flink_application_clusters.py",
    "scripts/alignment/verify_flink_application_artifacts.py",
    "scripts/alignment/verify_flink_nine_jobs.py",
    "scripts/alignment/generate_kafka_acl_plan.py",
    "deployments/kubernetes/security/generated-kafka-service-identities.v1.yaml",
    "deployments/kubernetes/init-jobs/00-kafka-service-principals.yaml",
    "deployments/kubernetes/site-values.template.yaml",
    "java/flink-jobs/deployments/Dockerfile.application",
    "tests/alignment/test_flink_application_cluster_migration.py",
    "tests/alignment/test_verify_flink_application_clusters.py",
    "tests/alignment/test_kafka_service_identity_sync.sh",
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
    print(f"[flink-application] starting {name}: {' '.join(command)}", flush=True)
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
    print(f"[flink-application] {name}: {result['status']}", flush=True)
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
    status = "PASS" if len(results) == len(COMMANDS) and all(item["status"] == "PASS" for item in results) else "FAIL"

    sources = []
    for relative in SOURCE_ARTIFACTS:
        path = ROOT / relative
        if not path.is_file():
            raise SystemExit(f"source artifact does not exist: {relative}")
        sources.append({"path": relative, "sha256": sha256(path), "size_bytes": path.stat().st_size})

    contract = json.loads((ROOT / SOURCE_ARTIFACTS[0]).read_text(encoding="utf-8"))
    jars = []
    for job in contract["jobs"]:
        relative = f"java/flink-jobs/{job['module']}/target/{job['module']}-1.0.0-SNAPSHOT.jar"
        path = ROOT / relative
        if not path.is_file():
            raise SystemExit(f"built job artifact does not exist: {relative}")
        jars.append({"job_id": job["id"], "path": relative, "sha256": sha256(path), "size_bytes": path.stat().st_size})

    manifest = {
        "schema_version": 1,
        "run_id": args.run_id,
        "captured_at": datetime.now(timezone.utc).isoformat(),
        "feature_id": "T-FLINK-001",
        "related_ids": ["T-KAFKA-005", "T-FLINK-002", "T-FLINK-003", "T-MINIO-001"],
        "status": "PARTIAL" if status == "PASS" else "FAIL",
        "scoped_evidence_status": status,
        "candidate_source": build_snapshot(),
        "g0_reference": {
            "run_id": g0.get("run_id"),
            "manifest": str(g0_path.relative_to(ROOT)),
            "manifest_sha256": sha256(g0_path),
            "status": g0.get("status"),
        },
        "gate_status": {
            "G0": "PASS",
            "G1": "PASS_FOR_CONTRACT_RENDER_IDENTITY_BUILD_AND_OFFLINE_ROLLBACK_GUARDS" if status == "PASS" else "FAIL",
            "G2": "OPEN_FOR_REAL_APPLICATION_CLUSTERS",
            "G3": "OPEN_FOR_OFFSETS_SINK_AND_CHECKPOINT_RECONCILIATION",
            "G4": "OPEN_FOR_RESOURCE_AND_FAILURE_BUDGETS",
            "G5": "OPEN",
            "G6": "HOLD_FOR_APPROVED_SERIAL_SAVEPOINT_MIGRATION_AND_ROLLBACK",
            "G7": "OPEN",
            "G8": "BLOCKED",
        },
        "production_applied": False,
        "commands": results,
        "source_artifacts": sources,
        "built_job_artifacts": jars,
        "proven": [
            "the versioned contract covers nine canonical jobs and 128 expected tasks with unique cluster and principal ids",
            "migration is constrained to one application at a time and pauses when the two-JobManager 8 CPU or 22 GiB online expand budget would be exceeded",
            "rendering requires an immutable repository@sha256 image and a MinIO savepoint URI with sha256 and source job id",
            "each pod template exposes only its own Kafka Secret and does not reference the legacy shared Kafka client keys",
            "the launcher uses Native Kubernetes Application Mode, a local single-job JAR, an explicit savepoint and no allowNonRestoredState option",
            "every rendered Application Cluster starts two Kubernetes-HA JobManagers and persists HA metadata and JobResultStore records under isolated MinIO prefixes",
            "an immutable rollback record retains the source Session Cluster id, source job id and savepoint digest, and a suspended rollback Job restores the same JAR to kubernetes-session only after explicit release",
            "all nine shaded JARs build and contain their declared main classes",
            "a multi-endpoint verifier fails unless every Application Cluster has one canonical RUNNING job, restored state, all expected tasks, a new isolated checkpoint and no root exception",
        ],
        "open": [
            "build, scan, sign and publish one digest-pinned image for each job",
            "capture fresh stop-with-savepoint manifests in an approved maintenance window",
            "apply one generated canary at a time and verify real RBAC, Secret authentication and NetworkPolicy behavior",
            "prove Kafka offsets, duplicate/ordering behavior and idempotent CH/OS/PG sinks reconcile after each restore",
            "prove TaskManager, JobManager, MinIO, Kafka and node failures recover within approved budgets",
            "exercise rollback to the retained Session Cluster savepoint before removing the shared identity or old deployment",
            "complete performance, release, observation, Windows Chrome, G7 and external G8 gates",
        ],
        "secrets_captured": False,
    }
    path = output / "manifest.json"
    path.write_text(json.dumps(manifest, ensure_ascii=False, indent=2) + "\n", encoding="utf-8")
    print(json.dumps({"status": manifest["status"], "scoped_evidence_status": status, "manifest": str(path), "manifest_sha256": sha256(path)}, ensure_ascii=False, indent=2), flush=True)
    return 0 if status == "PASS" else 1


if __name__ == "__main__":
    raise SystemExit(main())
