#!/usr/bin/env python3
"""Validate the M03 N009/N010 Feature sink and DLQ checkpoint barriers."""

from __future__ import annotations

import json
from pathlib import Path


ROOT = Path(__file__).resolve().parents[2]
FEATURE = ROOT / "java/flink-jobs/flink-feature-job/src/main/java/com/traffic/flink/feature"


def require(condition: bool, message: str) -> None:
    if not condition:
        raise SystemExit(f"M03 N009/N010 validation failed: {message}")


def main() -> None:
    job = (FEATURE / "FeatureJob.java").read_text(encoding="utf-8")
    factory = (FEATURE / "sink/ClickHouseSinkFactory.java").read_text(encoding="utf-8")
    batch_sink = (FEATURE / "sink/CheckpointAwareClickHouseSink.java").read_text(encoding="utf-8")
    processor = (FEATURE / "processor/FeatureProcessFunctionV3.java").read_text(encoding="utf-8")
    dlq = (FEATURE / "sink/DLQSinkFactory.java").read_text(encoding="utf-8")
    canonical = (ROOT / "java/flink-jobs/flink-common/src/main/java/com/traffic/flink/common/CanonicalDlqMessage.java").read_text(encoding="utf-8")

    for token in (
        "CheckpointedFunction",
        "snapshotState",
        "flushBuffer();",
        "statement.executeBatch()",
        "validateBatchReceipt",
        "Statement.EXECUTE_FAILED",
        "insert_deduplication_token",
        "throw error;",
    ):
        require(token in batch_sink, f"checkpoint-aware batch sink omits {token}")
    require("JdbcSink.sink" not in factory, "Feature factory still uses receipt-blind JdbcSink")
    require("recordInsertSuccess" not in factory, "parameter binding still records fake sink success")
    for token in (
        "createFeatureSink",
        "createFeatureSequenceSink",
        "createFeatureFingerprintSink",
    ):
        require(token in factory, f"Feature factory omits {token}")

    for token in (
        'properties.setProperty("enable.auto.commit", "false")',
        'properties.setProperty("commit.offsets.on.checkpoint", "true")',
        "CheckpointingMode.EXACTLY_ONCE",
        "allowed.lateness.ms",
    ):
        require(token in job, f"Feature checkpoint/source barrier omits {token}")
    for token in ("SUPER_LATE_EVENT", "lateDataFailure", "ctx.currentWatermark()"):
        require(token in processor, f"Feature late-data handoff omits {token}")
    for token in (
        'if (!"dlq.v1".equals(topic))',
        "DeliveryGuarantee.AT_LEAST_ONCE",
        'producerProps.setProperty("acks", "all")',
    ):
        require(token in dlq, f"canonical DLQ sink omits {token}")
    for token in (
        'value.put("original_topic"',
        'value.put("original_partition"',
        'value.put("original_offset"',
        'metadata.put("source_tuple"',
    ):
        require(token in canonical, f"canonical DLQ payload omits {token}")

    print(json.dumps({
        "result": "pass",
        "tasks": ["T1-M03-N009", "T1-M03-N010"],
        "feature_sinks": ["feature_stat", "feature_seq", "feature_fp"],
        "source_offset_policy": "checkpoint_only_after_all_sink_acks",
        "dlq_topic": "dlq.v1",
    }, separators=(",", ":")))


if __name__ == "__main__":
    main()
