#!/usr/bin/env python3
"""Verify the repository-only M07 fusion consumer-first implementation slice."""

from __future__ import annotations

import hashlib
import json
import re
import sys
from pathlib import Path


ROOT = Path(__file__).resolve().parents[2]


def read(relative: str) -> str:
    return (ROOT / relative).read_text(encoding="utf-8")


def load(relative: str) -> dict:
    return json.loads(read(relative))


def require(condition: bool, message: str, errors: list[str]) -> None:
    if not condition:
        errors.append(message)


def main() -> int:
    errors: list[str] = []
    migration_path = "deployments/postgres/migrations/202608141700_m07_fusion_snapshots_v1.sql"
    migration = read(migration_path)
    required_tables = {
        "fusion_source_sync_jobs",
        "fusion_source_snapshots",
        "fusion_source_entity_facts",
        "fusion_source_relation_facts",
        "fusion_snapshots",
        "fusion_snapshot_sources",
        "fusion_snapshot_entities",
        "fusion_snapshot_relations",
        "fusion_feature_metrics",
        "fusion_feature_ablation_results",
        "fusion_resolution_history",
        "fusion_projection_outbox",
        "fusion_projection_inbox",
        "fusion_projection_readiness_history",
        "fusion_projection_readiness_current",
        "fusion_projection_watermarks",
    }
    created_tables = set(re.findall(r"CREATE TABLE IF NOT EXISTS\s+([a-z0-9_]+)", migration))
    require(required_tables <= created_tables, f"missing fusion tables: {sorted(required_tables-created_tables)}", errors)
    require(
        not re.search(r"(?im)^\s*(DROP|TRUNCATE|DELETE)\b", migration),
        "fusion expand migration contains destructive SQL",
        errors,
    )
    for fragment in (
        "UNIQUE (tenant_id,idempotency_key)",
        "UNIQUE (source_topic,source_partition,source_offset)",
        "publish_state IN ('PENDING','OUTCOME_UNKNOWN','KAFKA_ACKED')",
        "state IN ('ASSIGNED','READY','REVOKED','STOPPED')",
        "FOREIGN KEY (tenant_id,source_snapshot_id)",
        "FOREIGN KEY (tenant_id,data_snapshot_id)",
        "FOREIGN KEY (tenant_id,feature_snapshot_id)",
    ):
        require(fragment in migration, f"migration is missing invariant {fragment}", errors)

    k8s_pg = read("deployments/kubernetes/init-jobs/02-postgres-schema.yaml")
    marker = "# BEGIN GENERATED T1-M07 FUSION SNAPSHOTS V1"
    require(k8s_pg.count(marker) == 1, "Kubernetes PostgreSQL entrypoint lacks one M07 generated block", errors)
    require(
        "38-rule-version-rollback-v1.sql 39-m07-fusion-snapshots-v1.sql; do" in k8s_pg,
        "M07 migration is not the ordered final PostgreSQL runner entry",
        errors,
    )
    migration_lines = "\n".join(f"    {line}" if line else "" for line in migration.rstrip().splitlines())
    require(migration_lines in k8s_pg, "Kubernetes M07 migration bytes differ from source migration", errors)

    topic_catalog = load("contracts/events/kafka-topic-catalog.v1.json")
    topics = {item["name"]: item for item in topic_catalog["topics"]}
    fusion_topic = topics.get("fusion.commands.v1") or {}
    require(fusion_topic.get("message_type") == "FusionSourceSyncRequestedV1Json", "fusion topic schema is not registered", errors)
    require(fusion_topic.get("readiness") == "consumer_candidate_default_off", "fusion topic readiness overclaims deployment", errors)
    acl = load("contracts/events/kafka-acl-catalog.v1.json")
    bindings = {item["topic"]: item for item in acl["topic_bindings"]}
    fusion_acl = bindings.get("fusion.commands.v1") or {}
    require(fusion_acl.get("producers") == ["alert-service"], "fusion producer ACL is not scoped to alert-service", errors)
    require(
        fusion_acl.get("consumers") == [{"principal": "alert-service", "groups": ["alert-service-fusion-projection-v1"]}],
        "fusion consumer ACL group is not exact",
        errors,
    )
    require("fusion.commands.v1:6:604800000:134217728" in read("common/kafka/create-topics.sh"), "local topic entry is absent", errors)
    require("fusion.commands.v1:6:604800000:134217728" in read("deployments/kubernetes/init-jobs/01-kafka-topics.yaml"), "K8s topic entry is absent", errors)
    event_schema = load("contracts/events/kafka-json-events-v1.schema.json")["$defs"].get("FusionSourceSyncRequestedV1Json") or {}
    required_fields = set(event_schema.get("required") or [])
    require(
        {"event_id", "tenant_id", "job_id", "source_id", "window_start", "window_end", "trace_id"} <= required_fields,
        "fusion source-sync event schema omits identity/window fields",
        errors,
    )

    contract = load("contracts/alignment/features/F-FUSION-001.json")
    require(contract.get("status") == "draft", "fusion feature contract must remain draft before live K8s evidence", errors)
    require(contract.get("rollout", {}).get("default") is False, "fusion authority must remain default off", errors)
    require(
        contract.get("rollout", {}).get("activation_order")
        == ["migration-topic-acl", "consumer-assigned-ready-lease", "authority-writer-and-dispatcher", "count-hash-watermark-reconcile"],
        "fusion activation order is not consumer-first",
        errors,
    )

    projector = read("go/control-plane/internal/alert/fusion/projector.go")
    decoder = read("go/control-plane/internal/alert/fusion/source_fact_decoder.go")
    merger = read("go/control-plane/internal/alert/fusion/entity_merger.go")
    feature_projector = read("go/control-plane/internal/alert/fusion/feature_projector.go")
    consumer = read("go/control-plane/internal/alert/consumer/fusion_projection_consumer.go")
    source_handler = read("go/control-plane/internal/alert/api/handler_fusion_source_sync.go")
    outbox = read("go/control-plane/internal/alert/api/handler_fusion_outbox.go")
    readiness = read("go/control-plane/internal/alert/fusion/readiness.go")
    main_go = read("go/control-plane/cmd/alert-service/main.go")
    for text, fragment, label in (
        (projector, "func (projector *Projector) ApplySourceSync(", "projector entrypoint"),
        (projector, "fusion_projection_inbox", "transactional inbox"),
        (projector, '"not_arrived"', "missing-not-zero state"),
        (decoder, "func DecodeSourceFacts(", "four-source decoder"),
        (merger, "func MergeSourceEntities(", "identity merger"),
        (merger, "ErrIdentityConflict", "identity collision fail-close"),
        (feature_projector, "func BuildFeatureProjection(", "feature projection"),
        (feature_projector, '"best_single_source_entity_count"', "single-source baseline"),
        (feature_projector, '"per-source-ablation-v1"', "per-source ablation"),
        (feature_projector, '"not_applicable"', "missing-source ablation state"),
        (consumer, "func (consumer *FusionProjectionConsumer) StartGeneration(", "generation consumer"),
        (readiness, "func (store *ReadinessStore) AssertReadyTx(", "durable readiness gate"),
        (source_handler, "loadFusionSourceSyncReplayTx", "idempotent source-sync writer"),
        (source_handler, "insertFusionAuditTx", "transactional fusion audit"),
        (outbox, "PublishOutcomeUnknownError", "outcome-unknown handling"),
        (outbox, "broker_partition", "broker ACK receipt"),
    ):
        require(fragment in text, f"missing {label}", errors)
    require(
        main_go.find("initFusionProjectionPipeline") < main_go.find("if fusionWriterEnabled"),
        "alert-service starts the fusion writer before its consumer readiness pipeline",
        errors,
    )
    config = read("go/control-plane/internal/alert/config/config.go")
    require('FUSION_PROJECTION_CONSUMER_V1_ENABLED" envDefault:"false"' in config, "consumer default is not false", errors)
    for manifest_path in (
        "deployments/kubernetes/applications/go-services.yaml",
        "go/control-plane/deployments/kubernetes/alert-service.yaml",
    ):
        manifest = read(manifest_path)
        require(manifest.count("FUSION_V1_ENABLED") == 1, f"{manifest_path} fusion writer flag is missing or duplicated", errors)
        require(manifest.count("FUSION_PROJECTION_CONSUMER_V1_ENABLED") == 1, f"{manifest_path} consumer flag is missing or duplicated", errors)
        require("FUSION_CANDIDATE_SHA256" in manifest, f"{manifest_path} candidate binding is absent", errors)

    test_paths = [
        "go/control-plane/internal/alert/fusion/source_fact_decoder_test.go",
        "go/control-plane/internal/alert/fusion/source_fact_reader_test.go",
        "go/control-plane/internal/alert/fusion/projector_test.go",
        "go/control-plane/internal/alert/fusion/feature_projector_test.go",
        "go/control-plane/internal/alert/fusion/readiness_test.go",
        "go/control-plane/internal/alert/consumer/fusion_projection_consumer_test.go",
        "go/control-plane/internal/alert/api/handler_fusion_source_sync_test.go",
        "go/control-plane/internal/alert/api/handler_fusion_outbox_test.go",
        "scripts/alignment/test_sync_m07_fusion_postgres_entrypoint.py",
    ]
    missing_tests = [path for path in test_paths if not (ROOT / path).is_file()]
    require(not missing_tests, f"missing M07 tests: {missing_tests}", errors)

    result = {
        "schema_version": 1,
        "milestone": "T1-M07",
        "feature_id": "F-FUSION-001",
        "status": "PASS" if not errors else "FAIL",
        "scope": "REPOSITORY_CANDIDATE_SOURCE_DATA_FEATURE_N006_PARTIAL",
        "production_applied": False,
        "kubernetes_live_evidence": False,
        "migration_sha256": hashlib.sha256(migration.encode()).hexdigest(),
        "tables": sorted(required_tables),
        "consumer_topic": "fusion.commands.v1",
        "consumer_group": "alert-service-fusion-projection-v1",
        "default_flags": {"consumer": False, "writer": False},
        "closure_blockers": [
            "knowledge fusion projection is not implemented",
            "no candidate image or live K8s generation readiness receipt has been captured",
            "no real four-source window count/hash/watermark reconciliation has been accepted",
            "baseline, graph, attack-chain and campaign trains remain open",
        ],
        "errors": errors,
    }
    print(json.dumps(result, ensure_ascii=False, indent=2))
    return 0 if not errors else 1


if __name__ == "__main__":
    sys.exit(main())
