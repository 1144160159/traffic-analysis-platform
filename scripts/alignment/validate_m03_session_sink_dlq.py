#!/usr/bin/env python3
"""Validate M03 N009/N010 Session sink and canonical DLQ barriers."""

from __future__ import annotations

import json
from pathlib import Path


ROOT = Path(__file__).resolve().parents[2]
SESSION = ROOT / "java/flink-jobs/flink-session-job/src/main/java/com/traffic/flink/session"


def require(condition: bool, message: str) -> None:
    if not condition:
        raise SystemExit(f"M03 Session N009/N010 validation failed: {message}")


def main() -> None:
    job = (SESSION / "SessionJob.java").read_text(encoding="utf-8")
    parser = (SESSION / "source/FlowEventParseFunction.java").read_text(encoding="utf-8")
    carrier = (SESSION / "source/ValidatedFlowInput.java").read_text(encoding="utf-8")
    lateness = (SESSION / "source/FlowLatenessFunction.java").read_text(encoding="utf-8")
    session_sink = (SESSION / "sink/CheckpointAwareSessionClickHouseSink.java").read_text(encoding="utf-8")
    flow_sink = (SESSION / "sink/FlowRawClickHouseSinkFunction.java").read_text(encoding="utf-8")
    search_sink = (SESSION / "sink/OpenSearchSinkFunction.java").read_text(encoding="utf-8")
    dlq_sink = (SESSION / "sink/KafkaSinkFactory.java").read_text(encoding="utf-8")
    canonical = (ROOT / "java/flink-jobs/flink-common/src/main/java/com/traffic/flink/common/CanonicalDlqMessage.java").read_text(encoding="utf-8")

    for token in (
        "ValidatedFlowInput",
        "new FlowLatenessFunction",
        "CheckpointAwareSessionClickHouseSink",
        'consumerProps.setProperty("enable.auto.commit", "false")',
        'consumerProps.setProperty("commit.offsets.on.checkpoint", "true")',
        "CheckpointingMode.EXACTLY_ONCE",
    ):
        require(token in job, f"Session job graph omits {token}")
    for forbidden in (
        "ClickHouseAsyncSinkFactory.addAsyncSink",
        "CanonicalDeadLetterMapper",
    ):
        require(forbidden not in job, f"Session job still uses legacy path {forbidden}")

    require("RawKafkaRecord source" in carrier, "validated Flow carrier drops broker source")
    require("CanonicalDlqMessage" in parser, "Flow parse failures are not canonical JSON")
    for token in ("SUPER_LATE_EVENT", "input.getSource()"):
        require(token in lateness, f"late-flow DLQ omits {token}")
    for token in (
        'value.put("original_topic"',
        'value.put("original_partition"',
        'value.put("original_offset"',
        'metadata.put("source_tuple"',
    ):
        require(token in canonical, f"canonical DLQ payload omits {token}")

    for sink, name, rethrow in (
        (session_sink, "sessions", "throw error;"),
        (flow_sink, "flows_raw", "throw e;"),
    ):
        for token in (
            "CheckpointedFunction",
            "snapshotState",
            "flushBuffer();",
            "validateBatchReceipt",
            "Statement.EXECUTE_FAILED",
            "insert_deduplication_token",
        ):
            require(token in sink, f"{name} sink omits {token}")
        require(rethrow in sink, f"{name} sink does not propagate batch failure")
    require("System.currentTimeMillis()" not in flow_sink,
            "flows_raw persistence fabricates a processing timestamp")
    require("validateBulkReceiptCount" in search_sink,
            "OpenSearch sink accepts an incomplete bulk receipt")
    for token in (
        'if (!"dlq.v1".equals(topic))',
        "KafkaSink<CanonicalDlqMessage>",
        'producerProps.setProperty("acks", "all")',
        'producerProps.setProperty("enable.idempotence", "true")',
    ):
        require(token in dlq_sink, f"Session canonical DLQ sink omits {token}")

    print(json.dumps({
        "result": "pass",
        "tasks": ["T1-M03-N009", "T1-M03-N010"],
        "session_sinks": ["flows_raw", "sessions", "opensearch:sessions_v1"],
        "dlq_format": "DLQMessageV1Json",
        "source_offset_policy": "checkpoint_only_after_all_sink_acks",
    }, separators=(",", ":")))


if __name__ == "__main__":
    main()
