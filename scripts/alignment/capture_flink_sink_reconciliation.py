#!/usr/bin/env python3
"""Capture immutable repository-side evidence for T-FLINK-004 sink reconciliation."""

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
    (
        "sink-reconciliation-contract",
        ["python3", "scripts/alignment/verify_flink_sink_reconciliation.py"],
    ),
    (
        "sink-reconciliation-contract-tests",
        [
            "python3",
            "-m",
            "unittest",
            "tests.alignment.test_flink_sink_reconciliation",
            "tests.alignment.test_flink_state_recovery",
            "-v",
        ],
    ),
    ("migration-policy", ["python3", "scripts/alignment/check_migrations.py"]),
    (
        "flink-sink-component-tests",
        [
            "mvn",
            "-q",
            "-f",
            "java/flink-jobs/pom.xml",
            "-pl",
            "flink-session-job,flink-pcap-index-job,flink-alert-generator-job,flink-log-job,flink-user-behavior-job",
            "-am",
            "test",
        ],
    ),
    (
        "user-behavior-submission-shell",
        ["bash", "-n", "java/flink-jobs/scripts/submit-user-behavior-job.sh"],
    ),
)
SOURCE_ARTIFACTS = (
    "contracts/flink/sink-reconciliation.v1.json",
    "contracts/flink/state-recovery.v1.json",
    "scripts/alignment/verify_flink_sink_reconciliation.py",
    "scripts/alignment/capture_flink_sink_reconciliation.py",
    "tests/alignment/test_flink_sink_reconciliation.py",
    "tests/alignment/test_flink_state_recovery.py",
    "java/flink-jobs/flink-pcap-index-job/src/main/java/com/traffic/flink/pcap/sink/ClickHousePcapSinkFactory.java",
    "java/flink-jobs/flink-pcap-index-job/src/main/java/com/traffic/flink/pcap/metrics/PcapIndexMetrics.java",
    "java/flink-jobs/flink-pcap-index-job/src/main/java/com/traffic/flink/pcap/process/PcapIndexProcessFunction.java",
    "java/flink-jobs/flink-pcap-index-job/src/test/java/com/traffic/flink/pcap/sink/ClickHousePcapSinkFactoryTest.java",
    "java/flink-jobs/flink-user-behavior-job/src/main/java/com/traffic/flink/behavior/user/UserBehaviorJob.java",
    "java/flink-jobs/flink-user-behavior-job/src/main/java/com/traffic/flink/behavior/user/model/AnomalyEvent.java",
    "java/flink-jobs/flink-user-behavior-job/src/main/java/com/traffic/flink/behavior/user/sink/ClickHouseAnomalySink.java",
    "java/flink-jobs/flink-user-behavior-job/src/test/java/com/traffic/flink/behavior/user/sink/ClickHouseAnomalySinkTest.java",
    "java/flink-jobs/flink-session-job/src/main/java/com/traffic/flink/session/sink/FlowRawClickHouseSinkFunction.java",
    "java/flink-jobs/flink-session-job/src/main/java/com/traffic/flink/session/sink/OpenSearchSinkFunction.java",
    "java/flink-jobs/flink-alert-generator-job/src/main/java/com/traffic/flink/alert/sink/OpenSearchAlertSinkFactory.java",
    "java/flink-jobs/flink-log-job/src/main/java/com/traffic/flink/log/sink/OpenSearchSinkFactory.java",
    "java/flink-jobs/flink-log-job/src/main/java/com/traffic/flink/log/sink/LokiSinkFactory.java",
    "java/flink-jobs/scripts/submit-user-behavior-job.sh",
    "deployments/clickhouse/migrations/202608031000_user_anomalies_v2.sql",
    "common/sql/ch/00-all-tables.sql",
    "deployments/kubernetes/init-jobs/03-clickhouse-schema.yaml",
    "go/control-plane/deployments/docker/init/clickhouse_merged.sql",
    "doc/07_alignment/runbooks/T-FLINK-004-sink-reconciliation.md",
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
    print(f"[flink-sink-reconciliation] starting {name}: {' '.join(command)}", flush=True)
    with log_path.open("wb") as log:
        completed = subprocess.run(
            command,
            cwd=ROOT,
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
    print(f"[flink-sink-reconciliation] {name}: {result['status']}", flush=True)
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
        sources.append(
            {"path": relative, "sha256": sha256(path), "size_bytes": path.stat().st_size}
        )

    candidate = build_snapshot()
    g0_candidate_hash = g0.get("candidate_source", {}).get("content_sha256")
    candidate_matches_g0 = candidate.get("content_sha256") == g0_candidate_hash
    g0_status = "PASS" if candidate_matches_g0 else "STALE_REQUIRES_REFRESH_AFTER_SINK_CHANGES"

    manifest = {
        "schema_version": 1,
        "run_id": args.run_id,
        "captured_at": datetime.now(timezone.utc).isoformat(),
        "feature_id": "T-FLINK-004",
        "related_ids": ["T-FLINK-001", "T-FLINK-002", "T-FLINK-003", "T-CH-001", "T-OS-001"],
        "status": "PARTIAL" if scoped_status == "PASS" else "FAIL",
        "coverage_status": "PARTIAL",
        "scoped_evidence_status": scoped_status,
        "candidate_source": candidate,
        "g0_reference": {
            "run_id": g0.get("run_id"),
            "manifest": str(g0_path.relative_to(ROOT)),
            "manifest_sha256": sha256(g0_path),
            "status": g0.get("status"),
            "candidate_content_sha256": g0_candidate_hash,
            "candidate_matches": candidate_matches_g0,
        },
        "gate_status": {
            "G0": g0_status,
            "G1": "PASS_FOR_PCAP_FALSE_ACK_REMOVAL_USER_ANOMALY_ACK_BUFFER_OS_ITEM_FAILURE_AND_CONTRACT_GUARDS" if scoped_status == "PASS" else "FAIL",
            "G2": "OPEN_FOR_LIVE_EXTERNAL_ACK_RETRY_CHECKPOINT_AND_REPLAY",
            "G3": "OPEN_FOR_EVENT_KEY_WATERMARK_AND_CROSS_SINK_RECONCILIATION",
            "G4": "OPEN_FOR_SINK_THROUGHPUT_RETRY_AND_BACKPRESSURE_BUDGETS",
            "G5": "OPEN",
            "G6": "HOLD_FOR_APPROVED_CANARY_MIGRATION_REPLAY_AND_ROLLBACK",
            "G7": "OPEN",
            "G8": "BLOCKED",
        },
        "production_applied": False,
        "commands": results,
        "source_artifacts": sources,
        "proven": [
            "PCAP ClickHouse parameter binding no longer increments an unowned success counter or exposes a fake recordInsertSuccess API",
            "user-anomaly ClickHouse batches retain pending records until executeBatch returns a complete non-failed per-record receipt",
            "user-anomaly sink rejects missing stable keys and versions, checkpoints pending records and propagates exhausted write failures",
            "user-anomaly rows carry stable anomaly_id, event_version and replay_id into a versioned deterministic ClickHouse table",
            "alert and log OpenSearch sinks inspect every bulk item and fail the sink when any item fails",
            "the versioned contract inventories all nine canonical jobs, 21 declared sinks and the required eight-metric vocabulary",
            "contract mutation tests reject missing jobs, source assertions and reconciliation keys",
            "ClickHouse migration sources are synchronized and production Flink Java contains no runtime DDL",
        ],
        "open": [
            "close the 26 contract-declared sink, replay, metric and reconciliation gaps across all nine jobs",
            "inject ClickHouse, OpenSearch, Loki, Kafka and HTTP partial acknowledgements and prove pending-state recovery after checkpoints",
            "run replay ranges with a stable replay_id and reconcile event_id or derived business key rather than aggregate counts",
            "capture input, accepted, dropped, late, failed, DLQ, sink-success and last-watermark metrics for every canonical job",
            "measure throughput, retry amplification, backpressure and external sink P99 against approved budgets",
            "execute migration, canary, rollback, observation and independent G2 through G8 gates",
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
                "coverage_status": manifest["coverage_status"],
                "scoped_evidence_status": scoped_status,
                "g0_status": g0_status,
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
