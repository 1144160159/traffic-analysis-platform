#!/usr/bin/env python3
"""Capture immutable repository-side G1 evidence for T-FLINK-003."""

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
    ("checkpoint-ha-contract", ["python3", "scripts/alignment/verify_flink_checkpoint_ha.py"]),
    (
        "checkpoint-ha-negative-tests",
        ["python3", "-m", "unittest", "tests.alignment.test_flink_checkpoint_ha", "-v"],
    ),
    (
        "application-ha-tests",
        ["python3", "-m", "unittest", "tests.alignment.test_flink_application_cluster_migration", "-v"],
    ),
    (
        "source-startup-component-tests",
        [
            "mvn", "-q", "-f", "java/flink-jobs/pom.xml", "-pl",
            "flink-pcap-index-job,flink-feature-job,flink-rule-job,flink-cep-job,flink-alert-generator-job",
            "-am", "test",
        ],
    ),
)
SOURCE_ARTIFACTS = (
    "contracts/flink/checkpoint-ha-upgrade.v1.json",
    "contracts/flink/application-cluster-migration.v1.json",
    "scripts/alignment/verify_flink_checkpoint_ha.py",
    "scripts/alignment/capture_flink_checkpoint_ha.py",
    "scripts/alignment/render_flink_application_cluster.py",
    "tests/alignment/test_flink_checkpoint_ha.py",
    "tests/alignment/test_flink_application_cluster_migration.py",
    "java/flink-jobs/flink-common/src/main/java/com/traffic/flink/common/KafkaStartingOffsets.java",
    "java/flink-jobs/flink-common/src/test/java/com/traffic/flink/common/KafkaStartingOffsetsTest.java",
    "deployments/kubernetes/infrastructure/07-flink.yaml",
    "deployments/kubernetes/observability/flink-checkpoint-slo.yaml",
    "doc/07_alignment/runbooks/T-FLINK-003-checkpoint-ha-upgrade.md",
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
    print(f"[flink-checkpoint-ha] starting {name}: {' '.join(command)}", flush=True)
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
    print(f"[flink-checkpoint-ha] {name}: {result['status']}", flush=True)
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
        if len(results) == len(COMMANDS) and all(item["status"] == "PASS" for item in results)
        else "FAIL"
    )

    sources = []
    for relative in SOURCE_ARTIFACTS:
        path = ROOT / relative
        if not path.is_file():
            raise SystemExit(f"source artifact does not exist: {relative}")
        sources.append(
            {"path": relative, "sha256": sha256(path), "size_bytes": path.stat().st_size}
        )

    manifest = {
        "schema_version": 1,
        "run_id": args.run_id,
        "captured_at": datetime.now(timezone.utc).isoformat(),
        "feature_id": "T-FLINK-003",
        "related_ids": ["T-FLINK-001", "T-FLINK-002", "T-FLINK-005", "T-MINIO-001"],
        "status": "PARTIAL" if scoped_status == "PASS" else "FAIL",
        "scoped_evidence_status": scoped_status,
        "candidate_source": build_snapshot(),
        "g0_reference": {
            "run_id": g0.get("run_id"),
            "manifest": str(g0_path.relative_to(ROOT)),
            "manifest_sha256": sha256(g0_path),
            "status": g0.get("status"),
        },
        "gate_status": {
            "G0": "PASS",
            "G1": "PASS_FOR_DURABLE_HA_STARTUP_POLICY_SLO_AND_UPGRADE_GUARDS" if scoped_status == "PASS" else "FAIL",
            "G2": "OPEN_FOR_LIVE_HA_TOPOLOGY_CREDENTIAL_RESOLUTION_AND_RESTART",
            "G3": "OPEN_FOR_CHECKPOINT_SAVEPOINT_OFFSET_AND_SINK_RECONCILIATION",
            "G4": "OPEN_FOR_CHECKPOINT_SLO_STATE_SCALE_AND_RTO",
            "G5": "OPEN",
            "G6": "HOLD_FOR_APPROVED_SERIAL_CANARY_AND_ROLLBACK",
            "G7": "OPEN",
            "G8": "BLOCKED",
        },
        "production_applied": False,
        "commands": results,
        "source_artifacts": sources,
        "proven": [
            "Session and Application Cluster contracts declare at least two Kubernetes-HA JobManagers",
            "checkpoint, savepoint, HA metadata and JobResultStore paths use literal durable flink-checkpoints S3 prefixes rather than node-local file paths",
            "Flink S3 credentials are injected through AWS environment variables backed by Secret references instead of unexpanded shell syntax inside FLINK_PROPERTIES",
            "five formerly latest-by-default Kafka data sources use a shared committed-or-earliest policy; all nine production jobs contain no hard-coded latest initializer",
            "all nine jobs enable checkpointing and Application rendering includes standby JobManagers, isolated HA metadata and retained JobResultStore records",
            "Prometheus rules encode the initial 99.9 percent checkpoint success, half-interval duration and five-minute recovery thresholds",
            "negative tests reject a single JobManager and a reintroduced hard-coded latest source",
            "the runbook requires hashed stop-with-savepoint, isolated restore, output diff, serial canary, reconciliation and rollback without allowNonRestoredState",
        ],
        "open": [
            "prove actual pods resolve MinIO credentials and expose one leader plus standby JobManagers",
            "inject JobManager, TaskManager, node and MinIO failures and record checkpoint continuity and recovery time",
            "capture completed, failed, duration, alignment, state size, external path and restore metrics at approved state scale",
            "reconcile savepoint, source offsets, deterministic output IDs, late/DLQ and external sinks for the same frozen input range",
            "execute one-job-at-a-time production canary, rollback and T+0/T+1/T+3/T+7 observation",
            "complete G2 through G8 independent gates",
        ],
        "secrets_captured": False,
    }
    manifest_path = output / "manifest.json"
    manifest_path.write_text(
        json.dumps(manifest, ensure_ascii=False, indent=2) + "\n", encoding="utf-8"
    )
    print(
        json.dumps(
            {
                "status": manifest["status"],
                "scoped_evidence_status": scoped_status,
                "manifest": str(manifest_path),
                "manifest_sha256": sha256(manifest_path),
            },
            ensure_ascii=False,
            indent=2,
        ),
        flush=True,
    )
    return 0 if scoped_status == "PASS" else 1


if __name__ == "__main__":
    raise SystemExit(main())
