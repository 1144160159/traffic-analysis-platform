#!/usr/bin/env python3
"""Capture immutable repository-side T-KAFKA-002 detection-slice evidence."""

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
    ("kafka-envelope-contract", ["python3", "scripts/alignment/verify_kafka_event_envelope.py"]),
    ("kafka-envelope-negative-tests", ["python3", "-m", "unittest", "tests.alignment.test_kafka_event_envelope", "-v"]),
    ("proto-lint", ["buf", "lint", "proto"]),
    ("go-alert-consumer", ["go", "-C", "go/control-plane", "test", "./internal/alert/consumer", "-count=1"]),
    ("java-detection-chain", ["mvn", "-f", "java/flink-jobs/pom.xml", "-pl", "flink-feature-job,flink-behavior-job", "-am", "test", "-DskipTests=false"]),
    ("strict-registry", ["python3", "scripts/alignment/validate.py", "--strict-w1"]),
)
SOURCE_ARTIFACTS = (
    "contracts/kafka/event-envelope-idempotency.v1.json",
    "proto/traffic/v1/common.proto",
    "proto/traffic/v1/feature.proto",
    "proto/traffic/v1/detection.proto",
    "go/control-plane/internal/alert/consumer/kafka_consumer.go",
    "go/control-plane/internal/alert/consumer/stable_id.go",
    "go/control-plane/internal/alert/consumer/stable_id_test.go",
    "rust/probe-agent/Cargo.toml",
    "rust/probe-agent/probe-agent/src/aggregator/eviction.rs",
    "java/flink-jobs/flink-feature-job/src/main/java/com/traffic/flink/feature/calculator/FeatureCalculator.java",
    "java/flink-jobs/flink-behavior-job/src/main/java/com/traffic/flink/behavior/detector/BehaviorDetectorFunction.java",
    "java/flink-jobs/flink-behavior-job/src/main/java/com/traffic/flink/behavior/detector/SyncBehaviorDetector.java",
    "scripts/alignment/verify_kafka_event_envelope.py",
    "tests/alignment/test_kafka_event_envelope.py",
    "doc/07_alignment/runbooks/T-KAFKA-002-event-envelope-idempotency.md",
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
    print(f"[kafka-envelope] starting {name}: {' '.join(command)}", flush=True)
    with log_path.open("wb") as log:
        completed = subprocess.run(command, cwd=ROOT, stdout=log, stderr=subprocess.STDOUT, check=False)
    finished = datetime.now(timezone.utc)
    result = {
        "name": name, "command": command, "exit_code": completed.returncode,
        "status": "PASS" if completed.returncode == 0 else "FAIL",
        "started_at": started.isoformat(), "finished_at": finished.isoformat(),
        "duration_seconds": round((finished - started).total_seconds(), 3),
        "artifact": log_path.name, "sha256": sha256(log_path), "size_bytes": log_path.stat().st_size,
    }
    print(f"[kafka-envelope] {name}: {result['status']}", flush=True)
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
    before = build_snapshot()
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
    scoped = "PASS" if len(results) == len(COMMANDS) and all(item["status"] == "PASS" for item in results) else "FAIL"
    after = build_snapshot()
    stable = before["content_sha256"] == after["content_sha256"]
    if not stable:
        scoped = "FAIL"

    sources = []
    for relative in SOURCE_ARTIFACTS:
        path = ROOT / relative
        if not path.is_file():
            raise SystemExit(f"source artifact does not exist: {relative}")
        sources.append({"path": relative, "sha256": sha256(path), "size_bytes": path.stat().st_size})

    manifest = {
        "schema_version": 1, "run_id": args.run_id,
        "captured_at": datetime.now(timezone.utc).isoformat(),
        "feature_id": "T-KAFKA-002", "related_ids": ["T-KAFKA-003", "T-FLINK-002", "T-SCHEMA-001", "T-OBS-001"],
        "status": "PARTIAL" if scoped == "PASS" else "FAIL",
        "coverage_status": "PARTIAL_DETECTION_VERTICAL_SLICE",
        "scoped_evidence_status": scoped, "production_applied": False,
        "candidate_source": before, "candidate_source_stable": stable,
        "g0_reference": {
            "run_id": g0.get("run_id"), "manifest": str(g0_path.relative_to(ROOT)),
            "manifest_sha256": sha256(g0_path), "status": g0.get("status"),
            "candidate_source_sha256": g0_hash,
        },
        "gate_status": {
            "G0": "PASS",
            "G1": "PASS_FOR_ADDITIVE_PROTO_DETECTION_ENVELOPE_SOURCE_CONTEXT_AND_REPLAY_IDENTITY" if scoped == "PASS" else "FAIL",
            "G2": "OPEN_FOR_REAL_KAFKA_VERSIONED_PRODUCER_CONSUMER_CANARY",
            "G3": "OPEN_FOR_DOUBLE_REPLAY_OFFSET_ALERT_EVIDENCE_AND_AUDIT_RECONCILIATION",
            "G4": "OPEN_FOR_ORDERING_LAG_MESSAGE_SIZE_PII_THROUGHPUT_AND_RESOURCE_BUDGETS",
            "G5": "OPEN", "G6": "HOLD_FOR_DRAIN_CUTOVER_FAULT_ROLLBACK_AND_OBSERVATION",
            "G7": "OPEN", "G8": "BLOCKED",
        },
        "commands": results, "source_artifacts": sources,
        "proven": [
            "EventHeader preserves tags 1-9 and adds the standard identity causality timing and producer fields at tags 10-21",
            "Probe flow and event IDs are deterministic UUIDv5 values and Session tuple plus flow references propagate through FeatureStat and DetectionBehavior into the Alert projection",
            "the detection Kafka key is tenant_id:community_id and behavior plus alert identities are deterministic under replay",
            "the strict alert consumer rejects incomplete envelope identity timestamps or source tuple before business persistence",
            "negative tests reject field renumbering missing source propagation empty tuple placeholders and false completion claims",
        ],
        "open": [
            "migrate and inventory every canonical producer and consumer rather than only the detection vertical slice",
            "deploy versioned producers before the strict consumer and safely drain or isolate legacy messages",
            "prove real Kafka double replay produces exactly one event and alert identity set",
            "inject duplicate out-of-order poison and DLQ acknowledgement failures and reconcile offsets",
            "complete schema registry performance privacy canary rollback observation and independent gates",
        ],
        "secrets_captured": False,
    }
    manifest_path = output / "manifest.json"
    manifest_path.write_text(json.dumps(manifest, ensure_ascii=False, indent=2) + "\n", encoding="utf-8")
    print(json.dumps({"status": manifest["status"], "scoped_evidence_status": scoped, "manifest": str(manifest_path), "manifest_sha256": sha256(manifest_path)}, ensure_ascii=False, indent=2), flush=True)
    return 0 if scoped == "PASS" else 1


if __name__ == "__main__":
    raise SystemExit(main())
