#!/usr/bin/env python3
"""Capture immutable G1 evidence for T-FLINK-002 source and component recovery semantics."""

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
    ("state-recovery-contract", ["python3", "scripts/alignment/verify_flink_state_recovery.py"]),
    (
        "state-recovery-contract-tests",
        ["python3", "-m", "unittest", "tests.alignment.test_flink_state_recovery", "-v"],
    ),
    ("kafka-acl-generated", ["python3", "scripts/alignment/generate_kafka_acl_plan.py", "--check-generated"]),
    (
        "flink-component-tests",
        [
            "mvn", "-q", "-f", "java/flink-jobs/pom.xml",
            "-pl",
            "flink-session-job,flink-feature-job,flink-rule-job,flink-behavior-job,flink-alert-generator-job,flink-cep-job,flink-log-job,flink-user-behavior-job",
            "-am", "test",
        ],
    ),
)
SOURCE_ARTIFACTS = (
    "contracts/flink/state-recovery.v1.json",
    "contracts/flink/application-cluster-migration.v1.json",
    "contracts/events/kafka-acl-catalog.v1.json",
    "scripts/alignment/verify_flink_state_recovery.py",
    "scripts/alignment/capture_flink_state_recovery.py",
    "tests/alignment/test_flink_state_recovery.py",
    "java/flink-jobs/flink-common/src/main/java/com/traffic/flink/common/DeterministicId.java",
    "java/flink-jobs/flink-session-job/src/main/java/com/traffic/flink/session/sink/FlowRawClickHouseSinkFunction.java",
    "java/flink-jobs/flink-session-job/src/main/java/com/traffic/flink/session/sink/OpenSearchSinkFunction.java",
    "java/flink-jobs/flink-log-job/src/main/java/com/traffic/flink/log/sink/LokiSinkFactory.java",
    "java/flink-jobs/flink-user-behavior-job/src/main/java/com/traffic/flink/behavior/user/LateUserEventRouter.java",
    "java/flink-jobs/flink-behavior-job/src/main/java/com/traffic/flink/behavior/detector/BehaviorDetectorFunction.java",
    "deployments/kubernetes/init-jobs/00-kafka-acl-plan.yaml",
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
    print(f"[flink-state-recovery] starting {name}: {' '.join(command)}", flush=True)
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
    print(f"[flink-state-recovery] {name}: {result['status']}", flush=True)
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
        "feature_id": "T-FLINK-002",
        "related_ids": ["T-FLINK-001", "T-KAFKA-002", "T-KAFKA-005"],
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
            "G1": "PASS_FOR_DETERMINISTIC_ID_STATE_BUFFER_LATE_DATA_AND_ASYNC_RETRY_COMPONENT_SCOPE" if scoped_status == "PASS" else "FAIL",
            "G2": "OPEN_FOR_LIVE_CHECKPOINT_RESTART_AND_EXTERNAL_ACK_FAILURES",
            "G3": "OPEN_FOR_LATE_DUPLICATE_AND_LOSS_RECONCILIATION",
            "G4": "OPEN_FOR_ASYNC_AND_SINK_RESOURCE_BUDGETS",
            "G5": "OPEN",
            "G6": "HOLD_FOR_APPROVED_APPLICATION_CLUSTER_CANARY_AND_ROLLBACK",
            "G7": "OPEN",
            "G8": "BLOCKED",
        },
        "production_applied": False,
        "commands": results,
        "source_artifacts": sources,
        "proven": [
            "all nine canonical entrypoints expose the contract-declared stable operator UIDs",
            "production Flink Java contains no random UUID, Math.random sampling or ambiguous nameUUID identity path outside generated protobuf code",
            "feature, rule, behavior, alert, CEP, user-anomaly and error-session identities derive from stable source events, versions and business windows",
            "raw-flow ClickHouse, session OpenSearch and Loki pending buffers participate in operator state and clear only after external acknowledgement",
            "Loki uses Unix epoch nanoseconds and valid JSON, retries bounded failures and propagates failure while retaining its pending buffer",
            "user behavior uses bounded out-of-orderness, an explicit allowed-lateness boundary and a replay-stable durable dlq.v1 side output",
            "behavior asynchronous inference has explicit capacity, timeout and bounded retry configuration",
            "contract mutation tests reject UID drift and missing DLQ least-privilege ACL",
        ],
        "open": [
            "deploy the digest-bound candidate and inject TaskManager, JobManager, ClickHouse, OpenSearch, Loki and Kafka failures",
            "prove checkpoint restore yields no missing business IDs and only contract-permitted idempotent duplicates",
            "reconcile source offsets, sink IDs, late DLQ records and checkpoint/savepoint watermarks for the same trace",
            "measure async capacity, retry amplification, backpressure and external sink P99 against approved budgets",
            "complete production canary, rollback, observation, G7 and independent G8 gates",
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
