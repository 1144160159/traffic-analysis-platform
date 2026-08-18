#!/usr/bin/env python3
"""Validate the M03 N007 packet-observation to sequence/fingerprint seam."""

from __future__ import annotations

import json
import re
from pathlib import Path


ROOT = Path(__file__).resolve().parents[2]
PROTO_FILES = [
    ROOT / "proto/traffic/v1/common.proto",
    ROOT / "proto/traffic/v1/flow.proto",
    ROOT / "proto/traffic/v1/session.proto",
    ROOT / "proto/traffic/v1/feature.proto",
]
MIGRATION = ROOT / "common/sql/ch/04-m03-feature-seq-fingerprint-v1.sql"
BASE = ROOT / "common/sql/ch/00-all-tables.sql"
K8S = ROOT / "deployments/kubernetes/init-jobs/03-clickhouse-schema.yaml"
DOCKER_DDL = ROOT / "java/flink-jobs/flink-feature-job/deployments/docker/init-clickhouse.sql"
DOCKER_COMPOSE = ROOT / "java/flink-jobs/flink-feature-job/deployments/docker-compose.yml"
DOCKER_ENTRYPOINT = ROOT / "java/flink-jobs/flink-feature-job/deployments/docker/entrypoint.sh"
PROPERTIES = ROOT / "java/flink-jobs/flink-feature-job/src/main/resources/feature-job.properties"
TOPICS = ROOT / "contracts/events/kafka-topic-catalog.v1.json"


def require(condition: bool, message: str) -> None:
    if not condition:
        raise SystemExit(f"M03 N007 validation failed: {message}")


def main() -> None:
    proto = "\n".join(path.read_text(encoding="utf-8") for path in PROTO_FILES)
    for token in (
        "message TrafficFeatureObservation",
        "repeated sint32 signed_packet_lengths = 3;",
        "repeated uint64 payload_nibble_counts = 5;",
        "TrafficFeatureObservation feature_observation = 24;",
        "TrafficFeatureObservation feature_observation = 51;",
        "string ja4 = 28;",
        "string sni = 29;",
        "string quic_version = 30;",
        "TransportSecurityProtocol transport_security = 31;",
        "string raw_traffic_ref = 32;",
    ):
        require(token in proto, f"missing protobuf contract {token}")

    migration = MIGRATION.read_text(encoding="utf-8")
    uncommented = "\n".join(
        line for line in migration.splitlines() if not line.lstrip().startswith("--")
    )
    require(
        not re.search(r"\b(DROP|DELETE|UPDATE|TRUNCATE|RENAME|REPLACE|MODIFY|MATERIALIZE)\b", uncommented, re.I),
        "migration is not additive",
    )
    for table in ("feature_seq_local", "feature_seq", "feature_fp_local", "feature_fp"):
        require(f"traffic.{table}" in migration, f"migration omits {table}")
    columns = (
        "ja4", "sni", "quic_version", "transport_security", "raw_traffic_ref",
        "feature_category", "availability", "schema_version", "algorithm_version",
        "window_id", "event_time_start_ms", "event_time_end_ms", "source_event_ids",
        "evidence_ids", "missing_fields", "missing_reason",
    )
    for target in (
        migration,
        BASE.read_text(encoding="utf-8"),
        K8S.read_text(encoding="utf-8"),
        DOCKER_DDL.read_text(encoding="utf-8"),
    ):
        require("feature_seq_local" in target, "schema omits feature_seq_local")
        for column in columns:
            require(column in target, f"schema omits {column}")

    topic_catalog = json.loads(TOPICS.read_text(encoding="utf-8"))
    active_names = {topic["name"] for topic in topic_catalog["topics"]}
    require("feature.fingerprint.v1" not in active_names, "unapproved fingerprint topic became active")
    require("feature.stats.v1" not in active_names, "unapproved plural stats topic became active")

    properties = PROPERTIES.read_text(encoding="utf-8")
    require("clickhouse.url=" in properties, "Feature properties use an unread ClickHouse key")
    require("clickhouse.hosts=" not in properties, "stale clickhouse.hosts key remains")
    compose = DOCKER_COMPOSE.read_text(encoding="utf-8")
    entrypoint = DOCKER_ENTRYPOINT.read_text(encoding="utf-8")
    for table in ("feature_stat_local", "feature_seq_local", "feature_fp_local"):
        require(table in compose, f"Docker compose omits local sink {table}")
        require(table in entrypoint, f"Docker entrypoint omits local sink {table}")
    require("--topic dlq.v1" in compose, "Docker compose does not create canonical dlq.v1")
    require("--topic dlq.feature-job" not in compose, "Docker compose still creates legacy DLQ")
    require("--topic l2.trigger.v1" not in compose, "Docker compose creates an unapproved L2 topic")

    rust = (ROOT / "rust/probe-agent/probe-agent/src/parser/security.rs").read_text(encoding="utf-8")
    feature_job = (ROOT / "java/flink-jobs/flink-feature-job/src/main/java/com/traffic/flink/feature/FeatureJob.java").read_text(encoding="utf-8")
    seq = (ROOT / "java/flink-jobs/flink-feature-job/src/main/java/com/traffic/flink/feature/calculator/FeatureSeqCalculator.java").read_text(encoding="utf-8")
    fingerprint = (ROOT / "java/flink-jobs/flink-feature-job/src/main/java/com/traffic/flink/feature/calculator/FeatureFingerprintCalculator.java").read_text(encoding="utf-8")
    for token in ("observe_transport_security", "fn ja3", "fn ja4", "tls_port_without_record_is_not_called_encrypted"):
        require(token in rust, f"Rust packet parser omits {token}")
    for token in (
        "signed-sequence-sha256-wavelet-haar/v1",
        "packet_sequence_truncated",
        "wavelet_fwd.minimum_two_samples",
        "wavelet_bwd.minimum_two_samples",
        "directional_wavelet_not_calculable",
    ):
        require(token in seq, f"sequence calculator omits {token}")
    for token in (
        "ja3-md5-ja4-sha256-sni-sha256-nibble-shannon-chi-square/v1",
        "certificate_not_observed", "no_sni_observed", "quic_client_hello",
        "FEATURE_AVAILABILITY_MISSING_INPUT",
    ):
        require(token in fingerprint, f"fingerprint calculator omits {token}")
    for token in ("createFeatureSequenceSink", "createFeatureFingerprintSink"):
        require(token in feature_job, f"FeatureJob does not wire {token}")

    print(json.dumps({
        "result": "pass",
        "task": "T1-M03-N007",
        "proto_carrier": "traffic.v1.TrafficFeatureObservation",
        "clickhouse_tables": ["feature_seq", "feature_fp"],
        "new_kafka_topics": 0,
    }, separators=(",", ":")))


if __name__ == "__main__":
    main()
