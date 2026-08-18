import json
import unittest
from pathlib import Path

import yaml


ROOT = Path(__file__).resolve().parents[2]
EVENT_SCHEMA = ROOT / "contracts/events/model-feedback-event.v1.schema.json"
TOPIC_CATALOG = ROOT / "contracts/events/kafka-topic-catalog.v1.json"
ACL_CATALOG = ROOT / "contracts/events/kafka-acl-catalog.v1.json"
GO_SERVICES = ROOT / "deployments/kubernetes/applications/go-services.yaml"
PG_MIGRATION = (
    ROOT
    / "deployments/postgres/migrations/202608151430_m08_model_feedback_revision_inbox_v1.sql"
)
PG_ENTRYPOINT = ROOT / "deployments/kubernetes/init-jobs/02-postgres-schema.yaml"
CONSUMER = (
    ROOT
    / "go/control-plane/internal/rules/consumer/model_feedback_revision_consumer.go"
)
RULE_MANAGER = ROOT / "go/control-plane/cmd/rule-manager/main.go"


class M08ModelFeedbackConsumerTest(unittest.TestCase):
    def test_event_contract_has_exact_revision_and_adjudication_fields(self) -> None:
        schema = json.loads(EVENT_SCHEMA.read_text(encoding="utf-8"))
        event = schema["$defs"]["ModelFeedbackAdjudicatedV1Json"]
        self.assertFalse(event["additionalProperties"])
        self.assertEqual(
            {
                "event_id",
                "event_type",
                "schema_version",
                "aggregate_version",
                "feedback_id",
                "tenant_id",
                "prediction_id",
                "alert_id",
                "label",
                "label_revision",
                "adjudication_state",
                "reason_code",
                "model_version",
                "rule_version",
                "adjudicated_by",
                "occurred_at_ms",
                "trace_id",
            },
            set(event["required"]),
        )
        self.assertEqual(
            ["PROPOSED", "ADJUDICATED", "RETRACTED"],
            event["properties"]["adjudication_state"]["enum"],
        )

    def test_topic_and_acl_are_consumer_only(self) -> None:
        topics = json.loads(TOPIC_CATALOG.read_text(encoding="utf-8"))
        topic = next(item for item in topics["topics"] if item["name"] == "model.feedback.v1")
        self.assertEqual("consumer_only", topic["readiness"])
        self.assertEqual([], topic["producers"])
        self.assertEqual(
            ["go/control-plane/internal/rules/consumer/model_feedback_revision_consumer.go"],
            topic["consumers"],
        )

        acl = json.loads(ACL_CATALOG.read_text(encoding="utf-8"))
        binding = next(
            item for item in acl["topic_bindings"] if item["topic"] == topic["name"]
        )
        self.assertEqual([], binding["producers"])
        self.assertEqual(
            [
                {
                    "principal": "rule-manager",
                    "groups": ["rule-manager-model-feedback-revision-v1"],
                }
            ],
            binding["consumers"],
        )

    def test_rule_manager_runtime_is_explicitly_default_off(self) -> None:
        documents = list(yaml.safe_load_all(GO_SERVICES.read_text(encoding="utf-8")))
        deployment = next(
            item
            for item in documents
            if item is not None
            and item.get("kind") == "Deployment"
            and item.get("metadata", {}).get("name") == "rule-manager"
        )
        container = deployment["spec"]["template"]["spec"]["containers"][0]
        env = {item["name"]: item.get("value") for item in container["env"]}
        self.assertEqual("false", env["MODEL_FEEDBACK_REVISION_CONSUMER_V1_ENABLED"])
        self.assertEqual("model.feedback.v1", env["KAFKA_MODEL_FEEDBACK_REVISION_TOPIC"])
        self.assertEqual(
            "rule-manager-model-feedback-revision-v1",
            env["KAFKA_MODEL_FEEDBACK_REVISION_EVENT_GROUP"],
        )
        self.assertEqual("0" * 64, env["MODEL_FEEDBACK_REVISION_CANDIDATE_SHA256"])
        self.assertEqual(
            "c60bdb3ed674853da641d2c613530195a1ed9db62ce48606c402870ab283c7d9",
            env["MODEL_FEEDBACK_REVISION_CONTRACT_SHA256"],
        )

    def test_postgres_migration_is_byte_exact_in_kubernetes_entrypoint(self) -> None:
        documents = list(yaml.safe_load_all(PG_ENTRYPOINT.read_text(encoding="utf-8")))
        configmap = next(
            item
            for item in documents
            if item.get("kind") == "ConfigMap"
            and item.get("metadata", {}).get("name") == "postgres-init-sql"
        )
        embedded = configmap["data"]["45-m08-model-feedback-revision-inbox-v1.sql"]
        self.assertEqual(PG_MIGRATION.read_text(encoding="utf-8").rstrip(), embedded.rstrip())
        runner = documents[1]["spec"]["template"]["spec"]["containers"][0]["args"][0]
        self.assertIn(
            "44-m07-campaign-rail-correlation-v1.sql "
            "45-m08-model-feedback-revision-inbox-v1.sql "
            "46-m08-model-drift-candidate-v1.sql "
            "47-m08-model-rollback-v2.sql; do",
            runner,
        )

    def test_consumer_enforces_authority_revision_and_dlq_barrier(self) -> None:
        source = CONSUMER.read_text(encoding="utf-8")
        for marker in (
            "decoder.DisallowUnknownFields()",
            "message.DuplicateHeaderNames()",
            "traffic.alerts_latest FINAL",
            "model feedback prediction authority mismatch",
            "pg_advisory_xact_lock",
            "errModelFeedbackConflict",
            "errModelFeedbackOutOfOrder",
            "errModelFeedbackRetracted",
            "model_feedback_revision_inbox",
            "model_feedback_revision_receipt",
            "model_feedback_consumer_readiness_receipt",
            "NewPostgresModelFeedbackRevisionProjectionWithReadiness",
        ):
            self.assertIn(marker, source)
        wiring = RULE_MANAGER.read_text(encoding="utf-8")
        self.assertIn("DLQPermanentOnly: true", wiring)
        self.assertIn("NewPostgresDLQAcknowledgementBarrier", wiring)


if __name__ == "__main__":
    unittest.main()
