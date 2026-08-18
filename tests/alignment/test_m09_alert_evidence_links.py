import json
from pathlib import Path


ROOT = Path(__file__).resolve().parents[2]


def source(path: str) -> str:
    return (ROOT / path).read_text(encoding="utf-8")


def test_openapi_exposes_revisioned_link_and_unlink_commands():
    document = json.loads(source("contracts/openapi/alignment-v1.openapi.json"))
    path = document["paths"]["/v1/alerts/{id}/evidence-links/{evidence_id}"]
    assert path["put"]["operationId"] == "linkAlertEvidence"
    assert path["delete"]["operationId"] == "unlinkAlertEvidence"
    assert path["put"]["x-rollout-flag"] == "ALERT_EVIDENCE_LINK_WRITER_V1_ENABLED"
    request = document["components"]["schemas"]["AlertEvidenceLinkRequest"]
    assert {
        "expected_revision",
        "expected_manifest_revision",
        "source_store",
        "object_bucket",
        "object_key",
        "object_version",
        "object_sha256",
        "reason",
    }.issubset(request["required"])


def test_writer_is_consumer_first_and_all_runtime_switches_are_default_off():
    config = source("go/control-plane/internal/alert/config/config.go")
    main = source("go/control-plane/cmd/alert-service/main.go")
    deployment = source("deployments/kubernetes/applications/go-services.yaml")
    for marker in (
        'env:"ALERT_EVIDENCE_LINK_CONSUMER_V1_ENABLED" envDefault:"false"',
        'env:"ALERT_EVIDENCE_LINK_DISPATCHER_V1_ENABLED" envDefault:"false"',
    ):
        assert marker in config
    for variable in (
        "ALERT_EVIDENCE_LINK_CONSUMER_V1_ENABLED",
        "ALERT_EVIDENCE_LINK_DISPATCHER_V1_ENABLED",
        "ALERT_EVIDENCE_LINK_WRITER_V1_ENABLED",
    ):
        assert f'{{name: {variable}, value: "false"}}' in deployment
    assert "if !alertEvidenceLinkConsumerEnabled || !alertEvidenceLinkDispatcherEnabled" in main
    assert "writer cannot start before the projection consumer is ready" in main
    assert "SetAlertEvidenceLinkRuntime(true, alertEvidenceLinkConsumer.Ready, producer)" in main


def test_postgres_authority_is_append_only_revisioned_and_broker_acknowledged():
    migration = source("deployments/postgres/migrations/202608160030_m09_alert_evidence_links_v1.sql")
    writer = source("go/control-plane/internal/alert/api/alert_evidence_links_v1.go")
    publisher = source("go/control-plane/internal/alert/api/handler_alert_evidence_link_outbox.go")
    for table in (
        "alert_evidence_links",
        "alert_evidence_link_history",
        "alert_evidence_link_commands",
        "alert_evidence_link_outbox",
        "alert_evidence_link_projection_inbox",
        "alert_evidence_link_projection_deliveries",
        "alert_evidence_link_projection_watermarks",
    ):
        assert f"CREATE TABLE IF NOT EXISTS {table}" in migration
    assert "immutable alert evidence link identity or object reference changed" in migration
    assert "sql.LevelSerializable" in writer
    assert "FOR SHARE" in writer
    assert "OBJECT_IDENTITY_CONFLICT" in writer
    assert "REVISION_CONFLICT" in writer
    assert "FOR UPDATE SKIP LOCKED" in publisher
    assert "broker_acknowledged_at" in publisher


def test_event_contract_and_projection_bind_exact_kafka_coordinates():
    schema = json.loads(source("contracts/events/kafka-json-events-v1.schema.json"))
    catalog = json.loads(source("contracts/events/kafka-topic-catalog.v1.json"))
    projection = source("go/control-plane/internal/alert/api/alert_evidence_link_projection.go")
    consumer = source("go/control-plane/internal/alert/consumer/alert_evidence_link_consumer.go")
    assert "AlertEvidenceLinkLifecycleV1Json" in schema["$defs"]
    topics = {item["name"] for item in catalog["topics"]}
    assert "alert.evidence-links.v1" in topics
    for marker in ("KafkaPartition", "KafkaOffset", "payload_sha256", "relation_revision"):
        assert marker in projection
    assert "DisallowUnknownFields" in consumer
    assert "header/body mismatch" in consumer
    assert "Kafka key/body mismatch" in consumer


def test_k8s_pipeline_evidence_is_run_scoped_immutable_and_cleaned():
    evidence = json.loads(
        source(
            "doc/02_acceptance/topic1/tasks/t1-m09-n012/"
            "k8s-alert-evidence-link-pipeline-latest.json"
        )
    )
    assert evidence["artifact_kind"] == "M09_ALERT_EVIDENCE_LINK_PIPELINE_TEST_RESULT"
    assert evidence["task_id"] == "T1-M09-N012"
    assert evidence["status"] == "PASS"
    assert evidence["topic_receipt"]["topic"] == "alert.evidence-links.v1"
    assert evidence["topic_receipt"]["partitions"] == 6
    assert evidence["run_scoped_resources_removed"] is True
    assert evidence["production_applied"] is False
    assert evidence["shared_postgres_touched"] is False
    assert evidence["shared_kafka_touched"] is False
    assert evidence["shared_clickhouse_touched"] is False
    assert evidence["postgres_cleanup_oracle"]["links"] == 0
    assert evidence["postgres_cleanup_oracle"]["outbox"] == 0
    assert evidence["postgres_cleanup_oracle"]["inbox"] == 0
    assert evidence["clickhouse_cleanup_oracle"]["alert_evidence_links_v1_local"] == 0
    assert len(evidence["kubernetes_dependencies"]) == 3
    assert len(evidence["kubernetes_jobs"]) == 2
    assert all(item["pod_uid"] and item["image_id"] for item in evidence["kubernetes_dependencies"])
    assert all(item["pod_uid"] and item["image_id"] for item in evidence["kubernetes_jobs"])
