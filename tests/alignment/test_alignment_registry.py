import json
import subprocess
import sys
import tempfile
import unittest
from collections import Counter
from pathlib import Path


ROOT = Path(__file__).resolve().parents[2]
sys.path.insert(0, str(ROOT / "scripts/alignment"))

from compat_diff import compare  # noqa: E402
from build_ledger import build_ledger  # noqa: E402
from candidate_snapshot import build_snapshot  # noqa: E402
from check_event_catalog import validate as validate_event_catalog  # noqa: E402
from check_migrations import DDL_PATTERN  # noqa: E402
from inventory import build_inventory  # noqa: E402
from reconcile_kafka_topics import reconcile as reconcile_kafka_topics  # noqa: E402
from validate import validate  # noqa: E402


class AlignmentRegistryTest(unittest.TestCase):
    def test_kafka_topic_schema_and_implementation_catalog_is_complete(self) -> None:
        result = validate_event_catalog()
        self.assertEqual("pass", result["result"], result)
        self.assertEqual(35, result["counts"]["canonical_topics"])
        self.assertEqual(7, result["counts"]["kubernetes_additional_topics"])
        self.assertEqual(6, result["counts"]["observed_additional_topics"])
        self.assertEqual(
            {"json-schema": 25, "protobuf": 10},
            result["counts"]["schema_kinds"],
        )

    def test_live_kafka_reconcile_rejects_partition_drift(self) -> None:
        catalog = json.loads(
            (ROOT / "contracts/events/kafka-topic-catalog.v1.json").read_text(
                encoding="utf-8"
            )
        )
        lines = []
        for topic in catalog["topics"]:
            partitions = topic["partitions"]
            if topic["name"] == "flow.events.v1":
                partitions -= 1
            lines.append(
                f"Topic: {topic['name']} TopicId: test "
                f"PartitionCount: {partitions} ReplicationFactor: 3 "
                f"Configs: retention.ms={topic['retention_ms']},"
                f"retention.bytes={topic['retention_bytes']}"
            )
        for item in (
            catalog["allowed_additional_topics"]
            + catalog["observed_runtime_additional_topics"]
            + catalog["observed_environment_test_topics"]
        ):
            lines.append(
                f"Topic: {item['name']} TopicId: test PartitionCount: 1 "
                "ReplicationFactor: 3 Configs: retention.ms=1,retention.bytes=1"
            )
        result = reconcile_kafka_topics("\n".join(lines))
        self.assertEqual("blocked", result["result"])
        self.assertTrue(
            any("flow.events.v1 partitions differs" in error for error in result["errors"])
        )

    def test_threat_intel_event_projection_is_authoritative_and_replayable(self) -> None:
        catalog = json.loads(
            (ROOT / "contracts/events/kafka-topic-catalog.v1.json").read_text(
                encoding="utf-8"
            )
        )
        topic = next(
            item for item in catalog["topics"] if item["name"] == "threat.intel.v1"
        )
        self.assertEqual("active", topic["readiness"])
        self.assertEqual(
            ["go/control-plane/internal/alert/consumer/threat_intel_event_consumer.go"],
            topic["consumers"],
        )
        producer = (
            ROOT / "go/control-plane/cmd/threat-intel-service/main.go"
        ).read_text(encoding="utf-8")
        self.assertIn("event.TenantID, event,", producer)
        self.assertIn('Key: "schema_version", Value: "1"', producer)
        self.assertIn(
            'Key: "aggregate_version", Value: strconv.FormatInt(event.AggregateVersion, 10)',
            producer,
        )
        self.assertIn("AggregateVersion: 1", producer)
        self.assertIn("commitThreatIntelCommand", producer)
        command = (
            ROOT / "go/control-plane/cmd/threat-intel-service/command_atomic.go"
        ).read_text(encoding="utf-8")
        self.assertIn("INSERT INTO threat_intel_event_outbox", command)
        self.assertIn("INSERT INTO threat_intel_command_history", command)
        self.assertIn("INSERT INTO threat_intel_command_requests", command)
        self.assertIn("tx.Commit()", command)
        self.assertIn("THREAT_INTEL_OUTBOX_V1_ENABLED", producer)
        consumer = (
            ROOT
            / "go/control-plane/internal/alert/consumer/threat_intel_event_consumer.go"
        ).read_text(encoding="utf-8")
        self.assertIn("jsonb_to_recordset", consumer)
        self.assertIn("threat intel authoritative payload mismatch", consumer)
        self.assertIn("threat intel event identity or Kafka offset collision", consumer)
        migration = (
            ROOT
            / "deployments/postgres/migrations"
            / "202607302330_threat_intel_event_projection.sql"
        ).read_text(encoding="utf-8")
        self.assertIn("threat_intel_event_projection", migration)
        go_services = (
            ROOT / "deployments/kubernetes/applications/go-services.yaml"
        ).read_text(encoding="utf-8")
        self.assertIn("KAFKA_THREAT_INTEL_EVENT_GROUP", go_services)
        self.assertIn("THREAT_INTEL_EVENT_PROJECTION_V1_ENABLED", go_services)
        self.assertIn("THREAT_INTEL_OUTBOX_V1_ENABLED", go_services)

    def test_topic_action_event_projection_contract_is_wired_end_to_end(self) -> None:
        catalog = json.loads(
            (ROOT / "contracts/events/kafka-topic-catalog.v1.json").read_text(
                encoding="utf-8"
            )
        )
        topic = next(
            item for item in catalog["topics"]
            if item["name"] == "traffic.topic.action.v2"
        )
        self.assertEqual("active", topic["readiness"])
        self.assertEqual(
            ["go/control-plane/internal/alert/consumer/topic_action_event_consumer.go"],
            topic["consumers"],
        )

        handler_source = (
            ROOT / "go/control-plane/internal/alert/api/handler_topic_alignment.go"
        ).read_text(encoding="utf-8")
        self.assertIn('"revision": completedRevision', handler_source)
        self.assertIn(
            "event_type, aggregate_version, partition_key, payload",
            handler_source,
        )

        main_source = (
            ROOT / "go/control-plane/cmd/alert-service/main.go"
        ).read_text(encoding="utf-8")
        self.assertIn("NewTopicActionEventConsumer", main_source)
        self.assertIn('DLQTopic: "dlq.v1"', main_source)
        self.assertIn("CommitOnHandlerError: false", main_source)

        migration = (
            ROOT
            / "deployments/postgres/migrations"
            / "202607302130_topic_action_event_projection.sql"
        ).read_text(encoding="utf-8")
        self.assertIn("topic_action_event_projection", migration)
        self.assertIn("topic_action_job_projection", migration)
        self.assertIn("UNIQUE (kafka_partition,kafka_offset)", migration)

    def test_deployment_event_projection_is_replayable_and_fail_closed(self) -> None:
        catalog = json.loads(
            (ROOT / "contracts/events/kafka-topic-catalog.v1.json").read_text(
                encoding="utf-8"
            )
        )
        topic = next(
            item for item in catalog["topics"]
            if item["name"] == "deployment.events.v1"
        )
        self.assertEqual("active", topic["readiness"])
        self.assertEqual(
            ["go/control-plane/internal/rules/consumer/deployment_event_consumer.go"],
            topic["consumers"],
        )
        main_source = (
            ROOT / "go/control-plane/cmd/rule-manager/main.go"
        ).read_text(encoding="utf-8")
        self.assertIn("VerifySchema", main_source)
        self.assertIn("NewDeploymentEventConsumer", main_source)
        self.assertIn('DLQTopic: "dlq.v1"', main_source)
        self.assertIn("CommitOnHandlerError: false", main_source)
        self.assertIn("DeploymentProjectionEnabled", main_source)
        migration = (
            ROOT
            / "deployments/postgres/migrations"
            / "202607302200_deployment_event_projection.sql"
        ).read_text(encoding="utf-8")
        self.assertIn("deployment_event_projection", migration)
        self.assertIn("deployment_state_projection", migration)
        self.assertIn("DEPLOYMENT_EVENT_PROJECTION_V1_ENABLED=false", migration)

    def test_alert_feedback_event_projection_has_stable_envelope_and_mlops_inbox(self) -> None:
        catalog = json.loads(
            (ROOT / "contracts/events/kafka-topic-catalog.v1.json").read_text(
                encoding="utf-8"
            )
        )
        topic = next(
            item for item in catalog["topics"]
            if item["name"] == "alert.feedback.v1"
        )
        self.assertEqual("active", topic["readiness"])
        self.assertEqual(
            ["go/control-plane/internal/rules/consumer/alert_feedback_event_consumer.go"],
            topic["consumers"],
        )
        producer = (
            ROOT / "go/control-plane/internal/alert/api/handler_feedback.go"
        ).read_text(encoding="utf-8")
        for header in (
            '"event_id"',
            '"event_type"',
            '"schema_version"',
            '"aggregate_version"',
            '"feedback_id"',
        ):
            self.assertIn(header, producer)
        self.assertNotIn("h.repo.Insert(ctx, record)", producer)
        self.assertNotIn("h.publishFeedback(ctx, feedback)", producer)
        transaction_source = (
            ROOT
            / "go/control-plane/internal/alert/api/handler_feedback_transaction.go"
        ).read_text(encoding="utf-8")
        self.assertIn("INSERT INTO alert_feedback", transaction_source)
        self.assertIn("INSERT INTO audit_logs", (
            ROOT / "go/control-plane/internal/alert/api/audit_trail_writer.go"
        ).read_text(encoding="utf-8"))
        self.assertIn("INSERT INTO alert_feedback_outbox", transaction_source)
        outbox_source = (
            ROOT / "go/control-plane/internal/alert/api/handler_feedback_outbox.go"
        ).read_text(encoding="utf-8")
        self.assertIn("FOR UPDATE SKIP LOCKED", outbox_source)
        self.assertIn('Key: "aggregate_version"', outbox_source)
        main_source = (
            ROOT / "go/control-plane/cmd/rule-manager/main.go"
        ).read_text(encoding="utf-8")
        self.assertIn("NewAlertFeedbackEventConsumer", main_source)
        self.assertIn("NewModelFeedbackInboxWorker", main_source)
        self.assertIn("AlertFeedbackProjectionEnabled", main_source)
        self.assertIn("FeedbackProjectionEnabled", main_source)
        self.assertIn("CommitOnHandlerError: false", main_source)
        inbox_worker = (
            ROOT
            / "go/control-plane/internal/rules/consumer/model_feedback_inbox_worker.go"
        ).read_text(encoding="utf-8")
        self.assertIn("FOR UPDATE SKIP LOCKED", inbox_worker)
        self.assertIn("insert_deduplication_token", inbox_worker)
        self.assertIn("ClickHouse feedback_id collision", inbox_worker)
        self.assertIn("'dead_letter'", inbox_worker)
        self.assertNotIn("CREATE TABLE", inbox_worker)
        migration = (
            ROOT
            / "deployments/postgres/migrations"
            / "202607302215_alert_feedback_event_projection.sql"
        ).read_text(encoding="utf-8")
        self.assertIn("alert_feedback_event_projection", migration)
        self.assertIn("model_feedback_inbox", migration)
        self.assertIn("ALERT_FEEDBACK_PROJECTION_V1_ENABLED=false", migration)
        transaction_migration = (
            ROOT
            / "deployments/postgres/migrations"
            / "202607302230_alert_feedback_transactional_outbox.sql"
        ).read_text(encoding="utf-8")
        self.assertIn("alert_feedback_outbox", transaction_migration)
        self.assertIn(
            "ALERT_FEEDBACK_TRANSACTIONAL_OUTBOX_V1_ENABLED=false",
            transaction_migration,
        )
        inbox_migration = (
            ROOT
            / "deployments/postgres/migrations"
            / "202607302245_model_feedback_inbox_worker.sql"
        ).read_text(encoding="utf-8")
        self.assertIn("MODEL_FEEDBACK_CLICKHOUSE_PROJECTION_V1_ENABLED=false", inbox_migration)
        self.assertIn("locked_until", inbox_migration)
        self.assertIn("dead_letter", inbox_migration)

    def test_alert_response_requests_are_receipted_without_fake_real_effects(self) -> None:
        catalog = json.loads(
            (ROOT / "contracts/events/kafka-topic-catalog.v1.json").read_text(
                encoding="utf-8"
            )
        )
        topic = next(
            item for item in catalog["topics"]
            if item["name"] == "alert.response.requested.v1"
        )
        self.assertEqual("active", topic["readiness"])
        self.assertEqual(
            ["go/control-plane/internal/alert/consumer/alert_response_event_consumer.go"],
            topic["consumers"],
        )
        producer = (
            ROOT / "go/control-plane/internal/alert/api/handler_alert_actions.go"
        ).read_text(encoding="utf-8")
        self.assertIn("if responseAction && request.DryRun", producer)
        self.assertIn('"outbox_status": outboxStatus', producer)
        self.assertIn('Key: "event_id"', producer)
        consumer = (
            ROOT / "go/control-plane/internal/alert/consumer/alert_response_event_consumer.go"
        ).read_text(encoding="utf-8")
        self.assertIn("simulated_completed", consumer)
        self.assertIn("blocked_external_executor", consumer)
        self.assertIn("external_effect_applied", consumer)
        self.assertIn("CommitOnHandlerError: false", (
            ROOT / "go/control-plane/cmd/alert-service/main.go"
        ).read_text(encoding="utf-8"))
        migration = (
            ROOT / "deployments/postgres/migrations"
            / "202607302300_alert_response_execution_projection.sql"
        ).read_text(encoding="utf-8")
        self.assertIn("alert_response_execution_receipts", migration)
        self.assertIn("ALERT_RESPONSE_EXECUTION_V1_ENABLED=false", migration)

    def test_model_actions_use_outbox_and_non_terminal_execution_inbox(self) -> None:
        catalog = json.loads(
            (ROOT / "contracts/events/kafka-topic-catalog.v1.json").read_text(
                encoding="utf-8"
            )
        )
        topic = next(
            item for item in catalog["topics"]
            if item["name"] == "model-actions.v1"
        )
        self.assertEqual("active", topic["readiness"])
        self.assertEqual(
            ["go/control-plane/internal/rules/consumer/model_action_event_consumer.go"],
            topic["consumers"],
        )
        repository = (
            ROOT / "go/control-plane/internal/rules/repository/model_repository.go"
        ).read_text(encoding="utf-8")
        self.assertIn("INSERT INTO model_action_outbox", repository)
        service = (
            ROOT / "go/control-plane/internal/rules/service/model_action_outbox.go"
        ).read_text(encoding="utf-8")
        self.assertIn("business_completed", service)
        self.assertIn("'dispatched'", service)
        model_service = (
            ROOT / "go/control-plane/internal/rules/service/model_service.go"
        ).read_text(encoding="utf-8")
        self.assertNotIn('"event_type": "model_action_requested"', model_service)
        self.assertNotIn("s.publisher.PublishModelAction", model_service)
        consumer = (
            ROOT / "go/control-plane/internal/rules/consumer/model_action_event_consumer.go"
        ).read_text(encoding="utf-8")
        self.assertIn("awaiting_executor", consumer)
        self.assertIn("business_completed", consumer)
        main_source = (
            ROOT / "go/control-plane/cmd/rule-manager/main.go"
        ).read_text(encoding="utf-8")
        self.assertIn("NewModelActionEventConsumer", main_source)
        self.assertIn("ModelActionInboxEnabled", main_source)
        self.assertIn("CommitOnHandlerError: false", main_source)
        migration = (
            ROOT / "deployments/postgres/migrations"
            / "202607302315_model_action_execution_inbox.sql"
        ).read_text(encoding="utf-8")
        self.assertIn("model_action_outbox", migration)
        self.assertIn("model_action_execution_inbox", migration)
        self.assertIn("MODEL_ACTION_INBOX_V1_ENABLED=false", migration)
        self.assertIn("Kafka acknowledgement was not business completion", migration)

    def test_campaign_aggregate_v2_is_versioned_idempotent_and_fail_closed(self) -> None:
        contract = json.loads(
            (ROOT / "contracts/alignment/features/F-CAMPAIGN-001.json").read_text(
                encoding="utf-8"
            )
        )
        self.assertEqual(13, contract["contract_version"])
        self.assertEqual("verifying", contract["status"])
        self.assertEqual(["campaign:write"], contract["permissions"]["required_scopes"])
        self.assertEqual(["alert:write"], contract["permissions"]["compatibility_scopes"])
        self.assertFalse(contract["rollout"]["default"])
        self.assertIn(
            "GET /v1/campaigns",
            contract["compatibility"]["preserved_api_operations"],
        )
        self.assertIn(
            "GET /v1/campaigns/{id}",
            contract["compatibility"]["preserved_api_operations"],
        )
        self.assertTrue(
            any("content-addressed lifecycle snapshot" in item for item in contract["domain"]["invariants"])
        )
        self.assertEqual(
            [
                "getCampaignSOARJob",
                "decideCampaignSOARJob",
                "cancelCampaignSOARJob",
                "compensateCampaignSOARJob",
            ],
            contract["api"]["soar_operations"],
        )
        self.assertEqual(
            "deployments/postgres/migrations/202608020900_campaign_soar_workflow_v1.sql",
            contract["data"]["soar_workflow_migration"],
        )
        self.assertIn(
            "traffic.campaign.v2.SoarCompensated",
            contract["data"]["event_types"],
        )
        self.assertTrue(
            any("HTTP 2xx without a provider receipt" in item for item in contract["domain"]["invariants"])
        )

        migration = (
            ROOT
            / "deployments/postgres/migrations"
            / "202608010700_campaign_aggregate_v2.sql"
        ).read_text(encoding="utf-8")
        for fragment in (
            "idempotency_key",
            "expected_revision",
            "campaign_aggregate_history",
            "campaign_aggregate_outbox",
            "snapshot_sha256",
            "object_manifest",
        ):
            self.assertIn(fragment, migration)

        source = (
            ROOT / "go/control-plane/internal/alert/api/campaign_aggregate_v2.go"
        ).read_text(encoding="utf-8")
        self.assertIn("Idempotency-Key", source)
        self.assertIn("REVISION_CONFLICT", source)
        self.assertIn("CAMPAIGN_MEMBERSHIP_BACKFILL_REQUIRED", source)
        self.assertIn('result["report_status"] = "accepted"', source)
        self.assertIn('result["object_manifest_status"] = "awaiting_executor"', source)
        self.assertIn('result["approval_status"] = "pending"', source)
        self.assertIn('result["executor_status"] = "not_dispatched"', source)
        self.assertIn("INSERT INTO campaign_soar_jobs", source)
        self.assertIn('payload["report_id"] = reportID', source)
        self.assertIn('payload["snapshot_sha256"] = reportSnapshotSHA', source)
        self.assertIn("CAMPAIGN_SNAPSHOT_CONFLICT", source)
        self.assertIn("commandSnapshot.LastEventID = state.LastEventID", source)

        soar_migration = (
            ROOT
            / "deployments/postgres/migrations"
            / "202608020900_campaign_soar_workflow_v1.sql"
        ).read_text(encoding="utf-8")
        for fragment in (
            "campaign_soar_jobs",
            "campaign_soar_approvals",
            "campaign_soar_execution_receipts",
            "campaign_soar_control_requests",
            "approved_awaiting_executor",
            "compensation_queued",
        ):
            self.assertIn(fragment, soar_migration)

        report_migration = (
            ROOT
            / "deployments/postgres/migrations"
            / "202608011000_campaign_report_executor_v2.sql"
        ).read_text(encoding="utf-8")
        for fragment in (
            "job_id",
            "object_bucket",
            "artifact_sha256",
            "next_attempt_at",
            "locked_until",
            "idx_campaign_reports_executor_pending",
        ):
            self.assertIn(fragment, report_migration)
        report_source = (
            ROOT / "go/control-plane/internal/alert/api/campaign_reports_v2.go"
        ).read_text(encoding="utf-8")
        self.assertIn("StartCampaignReportWorker", report_source)
        self.assertIn("campaignReportMaxAttempts = 5", report_source)
        self.assertIn("REPORT_MANIFEST_MISMATCH", report_source)
        self.assertIn("traffic.campaign.v2.ReportCompleted", report_source)
        self.assertIn("traffic.campaign.v2.ReportFailed", report_source)

        handler = (
            ROOT / "go/control-plane/internal/alert/api/handler_system.go"
        ).read_text(encoding="utf-8")
        self.assertIn("campaignReadScopes()", handler)
        self.assertIn("campaignWriteScopes()", handler)
        self.assertIn("CAMPAIGN_AGGREGATE_UNAVAILABLE", handler)
        self.assertIn("normalizeCampaignCommandCompatibility", handler)
        self.assertIn("submitCampaignAggregateV2Action", handler)

        for manifest_path in (
            ROOT / "deployments/kubernetes/applications/go-services.yaml",
            ROOT / "go/control-plane/deployments/kubernetes/alert-service.yaml",
        ):
            manifest = manifest_path.read_text(encoding="utf-8")
            self.assertIn(
                '{name: CAMPAIGN_AGGREGATE_V2_ENABLED, value: "false"}',
                manifest,
            )

        openapi = json.loads(
            (ROOT / "contracts/openapi/alignment-v1.openapi.json").read_text(
                encoding="utf-8"
            )
        )
        command = openapi["paths"]["/v1/campaigns/{id}/actions"]["post"]
        self.assertEqual("submitCampaignCommand", command["operationId"])
        self.assertIn("202", command["responses"])
        self.assertEqual(
            "listCampaigns",
            openapi["paths"]["/v1/campaigns"]["get"]["operationId"],
        )
        detail = openapi["paths"]["/v1/campaigns/{id}"]["get"]
        self.assertEqual("getCampaign", detail["operationId"])
        self.assertIn("409", detail["responses"])
        members = openapi["paths"]["/v1/campaigns/{id}/members"]["get"]
        self.assertIn("409", members["responses"])
        self.assertTrue(
            any(
                parameter.get("$ref") == "#/components/parameters/CampaignSnapshotId"
                for parameter in members["parameters"]
            )
        )
        metadata_schema = command["requestBody"]["content"]["application/json"]["schema"]["properties"]["metadata"]
        self.assertIn("snapshot_id", metadata_schema["properties"])
        self.assertEqual(
            "getCampaignReport",
            openapi["paths"]["/v1/campaigns/{id}/reports/{report_id}"]["get"]["operationId"],
        )
        self.assertEqual(
            "downloadCampaignReport",
            openapi["paths"]["/v1/campaigns/{id}/reports/{report_id}/download"]["get"]["operationId"],
        )

        merge_migration = (
            ROOT
            / "deployments/postgres/migrations"
            / "202608010900_campaign_merge_v2.sql"
        ).read_text(encoding="utf-8")
        self.assertIn("campaign_merge_receipts", merge_migration)
        self.assertIn("campaign_merge_items", merge_migration)
        self.assertIn("target_expected_revision", merge_migration)
        merge_source = (
            ROOT / "go/control-plane/internal/alert/api/campaign_merge_v2.go"
        ).read_text(encoding="utf-8")
        self.assertIn("sql.LevelSerializable", merge_source)
        self.assertIn("CAMPAIGN_NOT_FOUND", merge_source)
        self.assertIn("CAMPAIGN_MERGE_LIMIT_EXCEEDED", merge_source)

        backfill_migration = (
            ROOT
            / "deployments/postgres/migrations"
            / "202608010930_campaign_membership_backfill_v1.sql"
        ).read_text(encoding="utf-8")
        for fragment in (
            "campaign_membership_backfill_runs",
            "campaign_membership_backfill_campaigns",
            "campaign_membership_backfill_items",
            "skipped_explicit_unlink",
        ):
            self.assertIn(fragment, backfill_migration)
        backfill_source = (
            ROOT
            / "go/control-plane/internal/alert/api/campaign_membership_backfill.go"
        ).read_text(encoding="utf-8")
        self.assertIn("clickhouse_export", backfill_source)
        self.assertIn("sql.LevelSerializable", backfill_source)
        self.assertIn("skipped_explicit_unlink", backfill_source)
        self.assertIn("deterministicCampaignBackfillUUID", backfill_source)

    def test_campaign_membership_v2_is_bidirectional_and_aggregate_bound(self) -> None:
        contract = json.loads(
            (ROOT / "contracts/alignment/features/F-ALERT-002.json").read_text(
                encoding="utf-8"
            )
        )
        self.assertEqual(6, contract["contract_version"])
        self.assertIn(
            "DELETE /v1/alerts/{id}/campaign-links/{campaign_id}",
            contract["compatibility"]["preserved_api_operations"],
        )
        self.assertIn(
            "GET /v1/campaigns/{id}/members",
            contract["compatibility"]["preserved_api_operations"],
        )
        self.assertFalse(contract["rollout"]["default"])

        migration = (
            ROOT
            / "deployments/postgres/migrations"
            / "202608010730_campaign_membership_aggregate_v2.sql"
        ).read_text(encoding="utf-8")
        for fragment in (
            "campaign_membership_commands",
            "request_sha256",
            "expected_relation_revision",
            "expected_campaign_revision",
            "campaign_revision",
            "UNIQUE (tenant_id,idempotency_key)",
        ):
            self.assertIn(fragment, migration)

        source = (
            ROOT / "go/control-plane/internal/alert/api/campaign_membership_v2.go"
        ).read_text(encoding="utf-8")
        for fragment in (
            "sql.LevelSerializable",
            "sql.LevelRepeatableRead",
            "CAMPAIGN_REVISION_CONFLICT",
            "IDEMPOTENCY_KEY_CONFLICT",
            "INSERT INTO campaign_alert_link_history",
            "INSERT INTO campaign_alert_link_outbox",
            "INSERT INTO campaign_aggregate_history",
            "INSERT INTO campaign_aggregate_outbox",
            "UPDATE campaign_workbench_state",
            "membership_backfill",
        ):
            self.assertIn(fragment, source)

        handler = (
            ROOT / "go/control-plane/internal/alert/api/handler.go"
        ).read_text(encoding="utf-8")
        system_handler = (
            ROOT / "go/control-plane/internal/alert/api/handler_system.go"
        ).read_text(encoding="utf-8")
        self.assertIn('"/alerts/{id}/campaign-links/{campaign_id}"', handler)
        self.assertIn('"/campaigns/{id}/members"', system_handler)

        openapi = json.loads(
            (ROOT / "contracts/openapi/alignment-v1.openapi.json").read_text(
                encoding="utf-8"
            )
        )
        unlink = openapi["paths"]["/v1/alerts/{id}/campaign-links/{campaign_id}"]["delete"]
        members = openapi["paths"]["/v1/campaigns/{id}/members"]["get"]
        self.assertEqual("unlinkAlertFromCampaign", unlink["operationId"])
        self.assertEqual("listCampaignMembers", members["operationId"])
        self.assertIn("200", unlink["responses"])
        self.assertIn("200", members["responses"])

        for manifest_path in (
            ROOT / "deployments/kubernetes/applications/go-services.yaml",
            ROOT / "go/control-plane/deployments/kubernetes/alert-service.yaml",
        ):
            manifest = manifest_path.read_text(encoding="utf-8")
            self.assertIn(
                '{name: CAMPAIGN_AGGREGATE_V2_ENABLED, value: "false"}',
                manifest,
            )

    def test_campaign_event_v2_uses_dual_acknowledged_streams_and_durable_inbox(self) -> None:
        aggregate = json.loads(
            (ROOT / "contracts/alignment/features/F-CAMPAIGN-001.json").read_text(
                encoding="utf-8"
            )
        )
        membership = json.loads(
            (ROOT / "contracts/alignment/features/F-ALERT-002.json").read_text(
                encoding="utf-8"
            )
        )
        expected_topics = ["campaign.events.v2", "campaign.membership.events.v2"]
        self.assertEqual(expected_topics, aggregate["rollout"]["event_topics"])
        self.assertEqual(expected_topics, membership["rollout"]["event_topics"])
        self.assertEqual("stream+event_id", aggregate["rollout"]["event_identity"])

        migration = (
            ROOT
            / "deployments/postgres/migrations"
            / "202608010800_campaign_event_delivery_projection_v2.sql"
        ).read_text(encoding="utf-8")
        for fragment in (
            "status IN ('pending','processing','published','dead')",
            "campaign_event_projection_inbox",
            "PRIMARY KEY (stream,event_id)",
            "campaign_event_projection_deliveries",
            "campaign_event_projection_watermarks",
        ):
            self.assertIn(fragment, migration)

        publisher = (
            ROOT / "go/control-plane/internal/alert/api/handler_campaign_outbox.go"
        ).read_text(encoding="utf-8")
        consumer = (
            ROOT / "go/control-plane/internal/alert/consumer/campaign_event_consumer.go"
        ).read_text(encoding="utf-8")
        projection = (
            ROOT / "go/control-plane/internal/alert/api/campaign_event_projection.go"
        ).read_text(encoding="utf-8")
        main = (ROOT / "go/control-plane/cmd/alert-service/main.go").read_text(
            encoding="utf-8"
        )
        self.assertIn("RequiredAcks: \"all\"", main)
        self.assertIn("Async: false", main)
        self.assertIn("CommitOnDLQSuccess: true", main)
        self.assertIn("CommitOnHandlerError: false", main)
        self.assertIn("status='published',published=true", publisher)
        self.assertIn("campaignOutboxMaxAttempts", publisher)
        self.assertIn("target_status", projection)
        self.assertIn("Kafka key/body mismatch", consumer)

        for manifest_path in (
            ROOT / "deployments/kubernetes/applications/go-services.yaml",
            ROOT / "go/control-plane/deployments/kubernetes/alert-service.yaml",
        ):
            manifest = manifest_path.read_text(encoding="utf-8")
            self.assertIn(
                '{name: CAMPAIGN_EVENT_PIPELINE_V2_ENABLED, value: "false"}',
                manifest,
            )
            for topic in expected_topics:
                self.assertIn(topic, manifest)

    def test_campaign_three_target_projection_is_versioned_replayable_and_fail_closed(self) -> None:
        aggregate = json.loads(
            (ROOT / "contracts/alignment/features/F-CAMPAIGN-001.json").read_text(
                encoding="utf-8"
            )
        )
        membership = json.loads(
            (ROOT / "contracts/alignment/features/F-ALERT-002.json").read_text(
                encoding="utf-8"
            )
        )
        for contract in (aggregate, membership):
            self.assertEqual(
                "CAMPAIGN_TARGET_PROJECTION_V2_ENABLED",
                contract["rollout"]["target_projection_feature_flag"],
            )
            self.assertEqual(
                "deployments/postgres/migrations/202608010830_campaign_target_projection_worker_v2.sql",
                contract["data"]["target_projection_migration"],
            )
            self.assertEqual(
                "deployments/clickhouse/migrations/202608010900_campaign_projection_events_v2.sql",
                contract["data"]["clickhouse_projection_migration"],
            )
            self.assertIn(
                "postgresql.campaign_target_projection_watermarks.target+projection_version+event_id+projection_sha256",
                contract["data"]["source_watermarks"],
            )

        self.assertIn(
            "nebulagraph.entity.metadata_json.projection_version",
            aggregate["data"]["source_watermarks"],
        )
        self.assertIn(
            "nebulagraph.entity.metadata_json.projection_sha256",
            aggregate["data"]["source_watermarks"],
        )
        self.assertIn(
            "NebulaGraph space, entity tag and relation edge accept deterministic tenant-scoped writes",
            aggregate["rollout"]["prerequisites"],
        )
        self.assertIn(
            "nebulagraph.relation.attributes_json.relation_revision",
            membership["data"]["source_watermarks"],
        )
        self.assertIn(
            "nebulagraph.relation.attributes_json.projection_sha256",
            membership["data"]["source_watermarks"],
        )

        pg_migration = (
            ROOT
            / "deployments/postgres/migrations"
            / "202608010830_campaign_target_projection_worker_v2.sql"
        ).read_text(encoding="utf-8")
        for fragment in (
            "campaign_target_projection_watermarks",
            "PRIMARY KEY (tenant_id,projection_key,target)",
            "projection_sha256",
            "locked_until",
            "available_at",
            "target_status-'clickhouse'-'opensearch'-'nebulagraph'='{}'::jsonb",
        ):
            self.assertIn(fragment, pg_migration)

        worker = (
            ROOT
            / "go/control-plane/internal/alert/api/campaign_target_projection_worker.go"
        ).read_text(encoding="utf-8")
        for fragment in (
            "FOR UPDATE SKIP LOCKED",
            "pg_advisory_xact_lock",
            "campaign projection watermark identity collision",
            "campaign projection requires clickhouse, opensearch and nebulagraph targets",
            "current.projection_status='processing' AND current.locked_until<now()",
        ):
            self.assertIn(fragment, worker)
        self.assertNotIn("CREATE TABLE", worker)
        self.assertNotIn("ALTER TABLE", worker)

        clickhouse = (
            ROOT
            / "deployments/clickhouse/migrations"
            / "202608010900_campaign_projection_events_v2.sql"
        ).read_text(encoding="utf-8")
        self.assertIn("ReplicatedReplacingMergeTree", clickhouse)
        self.assertIn("cityHash64(tenant_id,projection_key)", clickhouse)
        self.assertNotIn("rand()", clickhouse)

        opensearch = (
            ROOT / "deployments/kubernetes/init-jobs/04-opensearch-templates.yaml"
        ).read_text(encoding="utf-8")
        for fragment in (
            "campaign-projections-v2-template",
            "campaign-projections-v2-write",
            "campaign-projections-v2-read",
            '\"projection_version\":{\"type\":\"long\"}',
            '\"dynamic\":\"strict\"',
        ):
            self.assertIn(fragment, opensearch)

        nebula_schema = (
            ROOT / "deployments/kubernetes/init-jobs/05-nebula-schema.yaml"
        ).read_text(encoding="utf-8")
        self.assertIn("CREATE TAG IF NOT EXISTS entity(", nebula_schema)
        self.assertIn("CREATE EDGE IF NOT EXISTS relation(", nebula_schema)
        self.assertIn("CREATE TAG INDEX IF NOT EXISTS entity_tenant_idx", nebula_schema)
        self.assertIn("CREATE EDGE INDEX IF NOT EXISTS relation_tenant_idx", nebula_schema)

        nebula_store = (
            ROOT / "go/control-plane/internal/graph/nebula/workbench_store.go"
        ).read_text(encoding="utf-8")
        self.assertIn("tenantVIDLiteral", nebula_store)
        self.assertIn("nebulaParameterInt", nebula_store)
        self.assertNotIn("UPSERT VERTEX ON entity $vid", nebula_store)
        self.assertNotIn("UPSERT EDGE ON relation $source_vid", nebula_store)

        projection_targets = (
            ROOT / "go/control-plane/internal/alert/api/campaign_projection_targets.go"
        ).read_text(encoding="utf-8")
        self.assertIn('"projection_sha256":', projection_targets)
        four_store_trace = (
            ROOT
            / "go/control-plane/internal/alert/api/campaign_four_store_trace_integration_test.go"
        ).read_text(encoding="utf-8")
        for fragment in (
            "TestCampaignEventRealKafkaFourStoreTrace",
            "requirePostgresCampaignProjectionReceipts",
            "requireClickHouseCampaignProjectionReceipt",
            "requireOpenSearchCampaignProjectionReceipt",
            "requireNebulaCampaignProjectionReceipts",
        ):
            self.assertIn(fragment, four_store_trace)

        main = (ROOT / "go/control-plane/cmd/alert-service/main.go").read_text(
            encoding="utf-8"
        )
        for fragment in (
            "Campaign target projection requires the campaign event V2 pipeline",
            "projectionWorker.VerifySchema",
            "clickHouseProjection.Ready",
            "openSearchProjection.Ready",
            "nebulaProjection.Ready",
            'AddReadinessCheck("campaign_target_projection"',
        ):
            self.assertIn(fragment, main)

        for manifest_path in (
            ROOT / "deployments/kubernetes/applications/go-services.yaml",
            ROOT / "go/control-plane/deployments/kubernetes/alert-service.yaml",
        ):
            manifest = manifest_path.read_text(encoding="utf-8")
            self.assertIn(
                '{name: CAMPAIGN_TARGET_PROJECTION_V2_ENABLED, value: "false"}',
                manifest,
            )
            self.assertIn("CAMPAIGN_TARGET_PROJECTION_NEBULA_PASSWORD", manifest)
            self.assertIn("secretKeyRef", manifest)

    def test_runtime_schema_gate_ignores_clickhouse_delete_mutations(self) -> None:
        self.assertFalse(
            DDL_PATTERN.search("ALTER TABLE traffic.evidence_local DELETE WHERE tenant_id=?")
        )
        self.assertTrue(DDL_PATTERN.search("CREATE TABLE unsafe_runtime_table(id UUID)"))
        self.assertTrue(
            DDL_PATTERN.search(
                "ALTER TABLE unsafe_runtime_table ADD COLUMN unsafe_value TEXT"
            )
        )

    def test_canonical_registry_and_work_packages_are_complete(self) -> None:
        result = validate()
        self.assertEqual("pass", result["result"], result)
        self.assertEqual(102, result["counts"]["total"])
        self.assertEqual(54, result["counts"]["features"])
        self.assertEqual(48, result["counts"]["technologies"])
        self.assertEqual({"P0": 38, "P1": 61, "P2": 3}, result["counts"]["priorities"])
        self.assertEqual(29, result["counts"]["work_packages"])

    def test_w1_contract_and_scope_gaps_are_closed(self) -> None:
        result = validate(strict_w1=True)
        self.assertEqual("pass", result["result"], result)
        self.assertEqual(
            {
                "unknown_required_ui_scopes": [],
                "unknown_accepted_ui_scopes": [],
                "missing_feature_contracts_for_p0": [],
            },
            result["w1_scope_gaps"],
        )

    def test_remediation_ledger_tracks_every_id_and_owner(self) -> None:
        ledger = build_ledger()
        overrides = json.loads(
            (ROOT / "contracts/alignment/progress-overrides.json").read_text(encoding="utf-8")
        )["overrides"]
        expected_statuses = Counter(item.get("status", "OPEN") for item in overrides.values())
        expected_statuses["OPEN"] += 102 - len(overrides)
        self.assertEqual(102, ledger["counts"]["total"])
        self.assertEqual(dict(sorted(expected_statuses.items())), ledger["counts"]["by_status"])
        self.assertEqual("OPEN", ledger["g7_status"])
        self.assertEqual("BLOCKED", ledger["g8_status"])
        self.assertEqual(102, len({item["id"] for item in ledger["items"]}))
        self.assertTrue(all(item["owner"] for item in ledger["items"]))
        standard_24w_scope = set(validate()["standard_24w_scope"])
        self.assertTrue(
            all(
                item["status"] in {"OPEN", "BLOCKED"}
                for item in ledger["items"]
                if item["priority"] in {"P1", "P2"}
                and item["id"] not in standard_24w_scope
            )
        )

    def test_route_and_action_inventory_is_deterministic(self) -> None:
        first = build_inventory()
        second = build_inventory()
        self.assertEqual(first, second)
        self.assertEqual(24, first["counts"]["formal_nav_routes"])
        self.assertGreater(first["counts"]["actions"], 0)
        self.assertGreater(first["counts"]["api_operations"], 0)
        self.assertIn("asset-upsert", first["actions"])
        self.assertIn("asset-observation-upsert", first["actions"])
        self.assertIn("asset-inactive-sweep", first["actions"])
        self.assertIn("POST /v1/assets", first["api_operations"])
        self.assertIn("gRPC AssetService.UpsertAsset", first["api_operations"])
        self.assertIn("gRPC AssetService.RecordMacIpBinding", first["api_operations"])
        self.assertIn("consumer asset.bindings.v1", first["api_operations"])

    def test_candidate_source_snapshot_is_content_addressed_and_deterministic(self) -> None:
        first = build_snapshot()
        second = build_snapshot()
        self.assertEqual(first, second)
        self.assertGreater(first["file_count"], 0)
        self.assertEqual(64, len(first["content_sha256"]))
        paths = {item["path"] for item in first["files"]}
        self.assertIn("proto/traffic/v1/ingest.proto", paths)
        self.assertIn("rust/probe-agent/probe-agent/src/control.rs", paths)
        self.assertNotIn("rust/probe-agent/target", "\n".join(paths))
        self.assertIn(
            "contracts/alignment/evidence-index.json",
            first["excluded_paths"],
        )
        self.assertNotIn("contracts/alignment/evidence-index.json", paths)
        self.assertNotIn("contracts/alignment/progress-overrides.json", paths)
        self.assertNotIn("contracts/alignment/remediation-ledger.json", paths)

    def test_compatibility_diff_reports_removals(self) -> None:
        baseline = build_inventory()
        candidate = json.loads(json.dumps(baseline))
        removed_route = candidate["routes"].pop()
        with tempfile.TemporaryDirectory() as directory:
            baseline_path = Path(directory) / "baseline.json"
            candidate_path = Path(directory) / "candidate.json"
            baseline_path.write_text(json.dumps(baseline), encoding="utf-8")
            candidate_path.write_text(json.dumps(candidate), encoding="utf-8")
            result = compare(baseline_path, candidate_path)
        self.assertEqual("blocked", result["result"])
        self.assertEqual([removed_route], result["removed_routes"])

    def test_probe_control_topics_consumers_and_deploy_env_stay_aligned(self) -> None:
        topic_names = (
            "probe.control.v2",
            "probe.acks.v2",
            "dlq.probe.acks.v2",
            "probe.events.v2",
        )
        topic_script = (ROOT / "common/kafka/create-topics.sh").read_text(encoding="utf-8")
        topic_job = (
            ROOT / "deployments/kubernetes/init-jobs/01-kafka-topics.yaml"
        ).read_text(encoding="utf-8")
        for topic_name in topic_names:
            self.assertIn(topic_name, topic_script)
            self.assertIn(topic_name, topic_job)
        self.assertIn("kafka-acl-plan-v1", topic_job)
        self.assertIn("KAFKA_ACL_MIGRATION_PHASE", topic_job)
        self.assertNotRegex(topic_job, r"--add[^\n]*--operation All")
        self.assertEqual(3, topic_job.count("--remove --force --allow-principal"))
        self.assertIn("escape_java_property()", topic_job)
        self.assertNotIn("grep -v WARNING || true", topic_job)
        topic_job_script = topic_job.split("        - |\n", maxsplit=1)[1].split(
            "        env:\n", maxsplit=1
        )[0]
        topic_job_script = "\n".join(
            line[10:] if line.startswith("          ") else line
            for line in topic_job_script.splitlines()
        )
        syntax = subprocess.run(
            ["bash", "-n"],
            input=topic_job_script,
            text=True,
            capture_output=True,
            check=False,
        )
        self.assertEqual(0, syntax.returncode, syntax.stderr)

        go_services = (
            ROOT / "deployments/kubernetes/applications/go-services.yaml"
        ).read_text(encoding="utf-8")
        ingest_block, remainder = go_services.split("# ---- Auth Service", maxsplit=1)
        _, alert_block = remainder.split("# ---- Alert Service", maxsplit=1)
        alert_block = alert_block.split("# ---- Asset Service", maxsplit=1)[0]
        self.assertIn("KAFKA_PROBE_CONTROL_TOPIC", ingest_block)
        self.assertIn("KAFKA_PROBE_CONTROL_GROUP", ingest_block)
        self.assertIn("KAFKA_PROBE_ACK_TOPIC", ingest_block)
        self.assertNotIn("KAFKA_PROBE_ACK_GROUP", ingest_block)
        self.assertIn("KAFKA_PROBE_ACK_TOPIC", alert_block)
        self.assertIn("KAFKA_PROBE_ACK_GROUP", alert_block)
        self.assertIn("KAFKA_PROBE_EVENT_TOPIC", alert_block)
        self.assertIn("KAFKA_PROBE_EVENT_GROUP", alert_block)
        self.assertNotIn("KAFKA_PROBE_CONTROL_GROUP", alert_block)

        standalone_alert = (
            ROOT / "go/control-plane/deployments/kubernetes/alert-service.yaml"
        ).read_text(encoding="utf-8")
        self.assertIn("KAFKA_PROBE_ACK_TOPIC", standalone_alert)
        self.assertIn("KAFKA_PROBE_ACK_GROUP", standalone_alert)
        self.assertIn("KAFKA_PROBE_EVENT_TOPIC", standalone_alert)
        self.assertIn("KAFKA_PROBE_EVENT_GROUP", standalone_alert)

        catalog = json.loads(
            (ROOT / "contracts/events/kafka-topic-catalog.v1.json").read_text(
                encoding="utf-8"
            )
        )
        probe_events = next(
            item for item in catalog["topics"] if item["name"] == "probe.events.v2"
        )
        self.assertEqual("active", probe_events["readiness"])
        self.assertEqual(
            ["go/control-plane/internal/alert/consumer/probe_operation_event_consumer.go"],
            probe_events["consumers"],
        )
        ack_source = (
            ROOT / "go/control-plane/internal/alert/api/handler_probe_ack.go"
        ).read_text(encoding="utf-8")
        self.assertIn('"revision": stateRevision', ack_source)
        outbox_source = (
            ROOT / "go/control-plane/internal/alert/api/handler_probe_outbox.go"
        ).read_text(encoding="utf-8")
        self.assertIn('Key: "schema_version"', outbox_source)
        projection_migration = (
            ROOT
            / "deployments/postgres/migrations"
            / "202607302145_probe_operation_event_projection.sql"
        ).read_text(encoding="utf-8")
        self.assertIn("probe_operation_event_projection", projection_migration)
        self.assertIn("probe_operation_state_projection", projection_migration)

    def test_probe_image_and_auth_configuration_fail_closed(self) -> None:
        dockerfile = (
            ROOT / "rust/probe-agent/docker/Dockerfile"
        ).read_text(encoding="utf-8")
        container_config = (
            ROOT / "rust/probe-agent/probe-agent/config.container.yaml"
        )
        probe_manifest = (
            ROOT / "deployments/kubernetes/applications/probe-agent.yaml"
        ).read_text(encoding="utf-8")
        deploy_script = (
            ROOT / "deployments/kubernetes/deploy.sh"
        ).read_text(encoding="utf-8")
        external_secrets = (
            ROOT / "deployments/kubernetes/security/external-secrets-template.yaml"
        ).read_text(encoding="utf-8")

        self.assertTrue(container_config.is_file())
        container_config_text = container_config.read_text(encoding="utf-8")
        self.assertIn(
            'gateway_addr: "${GATEWAY_ADDR:-http://127.0.0.1:50051}"',
            container_config_text,
        )
        self.assertIn("auth_token: null", container_config_text)
        self.assertNotIn("PROBE_AUTH_TOKEN", container_config_text)
        self.assertIn("FROM rust:1.93.0-slim-bookworm AS builder", dockerfile)
        self.assertIn(
            "RUSTUP_TOOLCHAIN=1.93.0-x86_64-unknown-linux-gnu",
            dockerfile,
        )
        self.assertNotIn(" clang llvm lld cmake ", dockerfile)
        self.assertIn("cargo build --locked --release -p probe-agent", dockerfile)
        self.assertIn(
            "COPY probe-agent/config.container.yaml /etc/probe-agent/config.yaml",
            dockerfile,
        )
        self.assertIn('CMD ["/etc/probe-agent/config.yaml"]', dockerfile)
        self.assertNotIn('CMD ["--config"', dockerfile)
        self.assertNotIn("probe-token-default-001", probe_manifest)
        self.assertIn("name: AUTH_TOKEN", probe_manifest)
        self.assertIn("key: PROBE_AUTH_TOKEN", probe_manifest)
        self.assertIn('--from-literal=PROBE_AUTH_TOKEN="$probe_auth_token"', deploy_script)
        self.assertIn(
            "PROBE_AUTH_TOKEN is missing from traffic-credentials; refusing",
            deploy_script,
        )
        self.assertIn("secretKey: PROBE_AUTH_TOKEN", external_secrets)

    def test_probe_g2_canary_is_isolated_and_renderable(self) -> None:
        template = (
            ROOT / "deployments/kubernetes/canary/probe-control-g2-canary.template.yaml"
        ).read_text(encoding="utf-8")
        smoke_source = ROOT / "go/control-plane/cmd/probe-control-smoke/main.go"
        self.assertTrue(smoke_source.is_file())
        self.assertIn("probe-control-g2-smoke-acks-v2", template)
        self.assertIn("ingest-gateway-probe-control-g2-canary-v2", template)
        self.assertIn("imagePullPolicy: Never", template)
        self.assertIn("suspend: true", template)
        self.assertEqual(
            3,
            template.count("node-role.kubernetes.io/control-plane"),
        )
        self.assertIn("key: PROBE_AUTH_TOKEN", template)
        self.assertNotIn("probe-token-default-001", template)
        self.assertNotIn("kind: DaemonSet", template)
        with tempfile.TemporaryDirectory() as directory:
            output = Path(directory) / "canary.yaml"
            result = subprocess.run(
                [
                    sys.executable,
                    str(ROOT / "scripts/alignment/render_probe_canary.py"),
                    "--source-sha256",
                    "a" * 64,
                    "--node",
                    "canary-node",
                    "--ingest-image",
                    "traffic/ingest-gateway:canary-a",
                    "--probe-image",
                    "traffic/probe-agent:canary-a",
                    "--smoke-image",
                    "traffic/probe-control-smoke:canary-a",
                    "--output",
                    str(output),
                ],
                text=True,
                capture_output=True,
                check=False,
            )
            self.assertEqual(0, result.returncode, result.stderr)
            rendered = output.read_text(encoding="utf-8")
            self.assertNotIn("__SOURCE_SHA256__", rendered)
            self.assertIn("kubernetes.io/hostname: \"canary-node\"", rendered)
        capture = ROOT / "scripts/alignment/capture_probe_canary.py"
        self.assertTrue(capture.is_file())
        capture_source = capture.read_text(encoding="utf-8")
        self.assertIn("G2_G3_CANARY", capture_source)
        self.assertIn("payload_values_captured", capture_source)
        self.assertIn("operation_field_absent_after_accepted_ack", capture_source)

    def test_asset_event_projection_has_durable_two_target_contract(self) -> None:
        catalog = json.loads(
            (ROOT / "contracts/events/kafka-topic-catalog.v1.json").read_text(
                encoding="utf-8"
            )
        )
        topic = next(
            item for item in catalog["topics"] if item["name"] == "asset.events.v2"
        )
        self.assertEqual("active", topic["readiness"])
        self.assertEqual("tenant_id+asset_id", topic["key_contract"])
        self.assertEqual(
            ["go/control-plane/internal/asset/repository/outbox_dispatcher.go"],
            topic["producers"],
        )
        self.assertEqual(
            ["go/control-plane/internal/asset/consumer/asset_projection_event.go"],
            topic["consumers"],
        )

        migration = (
            ROOT
            / "deployments/postgres/migrations"
            / "202607310030_asset_projection_inbox.sql"
        ).read_text(encoding="utf-8")
        self.assertIn("asset_projection_inbox", migration)
        self.assertIn("asset_projection_watermarks", migration)
        self.assertIn("UNIQUE (tenant_id,asset_id,aggregate_version)", migration)
        self.assertIn("'opensearch','nebulagraph'", migration)

        worker = (
            ROOT
            / "go/control-plane/internal/asset/consumer/asset_projection_worker.go"
        ).read_text(encoding="utf-8")
        self.assertIn("prior.aggregate_version<current.aggregate_version", worker)
        self.assertIn("prior.status IN ('pending','processing')", worker)
        self.assertIn("projection watermark identity collision", worker)

        targets = (
            ROOT
            / "go/control-plane/internal/asset/consumer/asset_projection_targets.go"
        ).read_text(encoding="utf-8")
        self.assertIn('VersionType: "external_gte"', targets)
        self.assertIn("UpsertAssetEntity", targets)

        main_source = (
            ROOT / "go/control-plane/cmd/asset-service/main.go"
        ).read_text(encoding="utf-8")
        self.assertIn("NewAssetProjectionEventConsumer", main_source)
        self.assertIn("NewAssetProjectionWorker", main_source)
        self.assertIn("CommitOnHandlerError: false", main_source)
        self.assertIn("EnableDLQ:            false", main_source)

        go_services = (
            ROOT / "deployments/kubernetes/applications/go-services.yaml"
        ).read_text(encoding="utf-8")
        self.assertIn("ASSET_EVENT_TOPIC", go_services)
        self.assertIn("asset.events.v2", go_services)
        self.assertIn("ASSET_PROJECTION_OS_WRITE_ALIAS", go_services)
        self.assertIn("ASSET_PROJECTION_NEBULA_SPACE", go_services)

    def test_playbook_execution_v2_is_real_approval_receipt_and_rollback_bound(self) -> None:
        contract = json.loads(
            (ROOT / "contracts/alignment/features/F-PLAYBOOK-001.json").read_text(
                encoding="utf-8"
            )
        )
        self.assertEqual(2, contract["contract_version"])
        self.assertEqual("verifying", contract["status"])
        self.assertEqual("postgresql", contract["data"]["authoritative_store"])
        self.assertEqual(
            "deployments/postgres/migrations/202608021000_playbook_execution_v2.sql",
            contract["data"]["migration"],
        )
        self.assertEqual(
            "deployments/postgres/migrations/202608021030_playbook_event_pipeline_v2.sql",
            contract["data"]["projection_migration"],
        )
        self.assertEqual(
            {
                "requestPlaybookExecution",
                "getPlaybookExecution",
                "decidePlaybookExecution",
                "cancelPlaybookExecution",
                "compensatePlaybookExecution",
            },
            set(contract["api"]["operations"]),
        )
        self.assertTrue(
            {
                "pending_approval",
                "approved_awaiting_executor",
                "running",
                "partial",
                "failed",
                "cancelled",
                "compensation_queued",
                "compensated",
                "compensation_failed",
            }.issubset(set(contract["ui"]["states"]))
        )
        self.assertFalse(contract["rollout"]["default"])

        openapi = json.loads(
            (ROOT / "contracts/openapi/alignment-v1.openapi.json").read_text(
                encoding="utf-8"
            )
        )
        operations = {
            "/v1/playbooks/{name}/execute": (
                "post",
                "requestPlaybookExecution",
                "playbook:execute",
            ),
            "/v1/playbooks/executions/{execution_id}": (
                "get",
                "getPlaybookExecution",
                "playbook:read",
            ),
            "/v1/playbooks/executions/{execution_id}/approval": (
                "post",
                "decidePlaybookExecution",
                "playbook:approve",
            ),
            "/v1/playbooks/executions/{execution_id}/cancel": (
                "post",
                "cancelPlaybookExecution",
                "playbook:execute",
            ),
            "/v1/playbooks/executions/{execution_id}/compensate": (
                "post",
                "compensatePlaybookExecution",
                "playbook:approve",
            ),
        }
        for path, (method, operation_id, scope) in operations.items():
            operation = openapi["paths"][path][method]
            self.assertEqual(operation_id, operation["operationId"])
            self.assertEqual("F-PLAYBOOK-001", operation["x-feature-id"])
            self.assertEqual(scope, operation["x-required-scope"])
        execute = openapi["paths"]["/v1/playbooks/{name}/execute"]["post"]
        self.assertIn("202", execute["responses"])
        alert_context = execute["requestBody"]["content"]["application/json"]["schema"]["properties"]["alert_context"]
        self.assertIn("alert_id", alert_context["required"])

        backend = (
            ROOT / "go/control-plane/internal/alert/api/playbook_execution_v2.go"
        ).read_text(encoding="utf-8")
        for fragment in (
            "sql.LevelSerializable",
            "FOR UPDATE SKIP LOCKED",
            "ALERT_ID_REQUIRED",
            "PLAYBOOK_RUN_LIMIT_REACHED",
            "PLAYBOOK_COOLDOWN_ACTIVE",
            "alert_playbook_step_receipts",
            "alert_playbook_execution_outbox",
            'executorStatus = "not_configured"',
        ):
            self.assertIn(fragment, backend)

        frontend = (
            ROOT / "web/ui/src/pages/PlaybookAutomationPage.tsx"
        ).read_text(encoding="utf-8")
        for fragment in (
            "requestPlaybookExecution",
            "liveAlertId.trim()",
            "approved_awaiting_executor",
            "execution_receipt",
            "compensation_receipt",
        ):
            self.assertIn(fragment, frontend)

        schema_fragments = (
            "alert_playbook_execution_approvals",
            "alert_playbook_execution_controls",
            "alert_playbook_step_receipts",
            "alert_playbook_execution_outbox",
            "workflow_revision",
            "approved_awaiting_executor",
            "compensation_queued",
        )
        for relative_path in (
            "deployments/postgres/migrations/202608021000_playbook_execution_v2.sql",
            "common/sql/pg/04-tasks-audit.sql",
            "go/control-plane/deployments/docker/init/postgres_merged.sql",
            "deployments/kubernetes/init-jobs/02-postgres-schema.yaml",
        ):
            schema_source = (ROOT / relative_path).read_text(encoding="utf-8")
            for fragment in schema_fragments:
                self.assertIn(fragment, schema_source, relative_path)
        migration = (
            ROOT
            / "deployments/postgres/migrations/202608021000_playbook_execution_v2.sql"
        ).read_text(encoding="utf-8")
        self.assertIn("202608021000", migration)
        k8s_schema = (
            ROOT / "deployments/kubernetes/init-jobs/02-postgres-schema.yaml"
        ).read_text(encoding="utf-8")
        self.assertIn("12-playbook-execution-v2.sql", k8s_schema)
        self.assertIn("13-playbook-event-pipeline-v2.sql", k8s_schema)
        self.assertIn(
            "11-campaign-soar-workflow.sql 12-playbook-execution-v2.sql 13-playbook-event-pipeline-v2.sql",
            k8s_schema,
        )
        docker_schema = (
            ROOT / "go/control-plane/deployments/docker/init/postgres_merged.sql"
        ).read_text(encoding="utf-8")
        self.assertIn("CREATE TABLE IF NOT EXISTS replay_tasks", docker_schema)
        self.assertIn("tables.table_type = 'BASE TABLE'", docker_schema)

        event_catalog = json.loads(
            (ROOT / "contracts/events/kafka-topic-catalog.v1.json").read_text(
                encoding="utf-8"
            )
        )
        playbook_topic = next(
            topic
            for topic in event_catalog["topics"]
            if topic["name"] == "playbook.execution.events.v2"
        )
        self.assertEqual("active", playbook_topic["readiness"])
        self.assertEqual("tenant_id+execution_id", playbook_topic["key_contract"])
        self.assertEqual("PlaybookExecutionLifecycleV2Json", playbook_topic["message_type"])
        self.assertEqual(
            ["go/control-plane/internal/alert/api/playbook_execution_outbox.go"],
            playbook_topic["producers"],
        )
        self.assertEqual(
            ["go/control-plane/internal/alert/consumer/playbook_execution_event_consumer.go"],
            playbook_topic["consumers"],
        )
        event_schema = json.loads(
            (ROOT / "contracts/events/kafka-json-events-v1.schema.json").read_text(
                encoding="utf-8"
            )
        )["$defs"]["PlaybookExecutionLifecycleV2Json"]
        self.assertFalse(event_schema["additionalProperties"])
        self.assertEqual(2, event_schema["properties"]["schema_version"]["const"])
        self.assertIn(
            "traffic.playbook.v2.CompensationFailed",
            event_schema["properties"]["event_type"]["enum"],
        )

        topic_script = (ROOT / "common/kafka/create-topics.sh").read_text(
            encoding="utf-8"
        )
        topic_init = (
            ROOT / "deployments/kubernetes/init-jobs/01-kafka-topics.yaml"
        ).read_text(encoding="utf-8")
        for source in (topic_script, topic_init):
            self.assertIn("playbook.execution.events.v2:6:604800000:268435456", source)

        publisher = (
            ROOT / "go/control-plane/internal/alert/api/playbook_execution_outbox.go"
        ).read_text(encoding="utf-8")
        self.assertIn("FOR UPDATE SKIP LOCKED", publisher)
        self.assertIn("playbookExecutionPublish(ctx", publisher)
        self.assertLess(
            publisher.index("playbookExecutionPublish(ctx"),
            publisher.index("SET status='published'"),
        )
        self.assertIn("playbook execution outbox lease lost after Kafka acknowledgement", publisher)

        projection = (
            ROOT
            / "go/control-plane/internal/alert/api/playbook_execution_event_projection.go"
        ).read_text(encoding="utf-8")
        self.assertIn("sql.LevelSerializable", projection)
        self.assertIn("workflow_revision", projection)
        self.assertIn("payload_sha256", projection)
        self.assertIn("playbook execution event replay identity collision", projection)

        consumer_source = (
            ROOT
            / "go/control-plane/internal/alert/consumer/playbook_execution_event_consumer.go"
        ).read_text(encoding="utf-8")
        self.assertIn("header/body mismatch", consumer_source)
        self.assertIn("Kafka key/body mismatch", consumer_source)
        self.assertIn("ApplyPlaybookExecutionEventProjection", consumer_source)

        alert_main = (ROOT / "go/control-plane/cmd/alert-service/main.go").read_text(
            encoding="utf-8"
        )
        self.assertIn("RequiredAcks: \"all\"", alert_main)
        self.assertIn("StartPlaybookExecutionEventOutboxWorker", alert_main)
        self.assertIn("NewPlaybookExecutionEventConsumer", alert_main)
        self.assertIn("CommitOnHandlerError: false", alert_main)

        for manifest_path in (
            ROOT / "deployments/kubernetes/applications/go-services.yaml",
            ROOT / "go/control-plane/deployments/kubernetes/alert-service.yaml",
        ):
            manifest = manifest_path.read_text(encoding="utf-8")
            self.assertIn(
                '{name: PLAYBOOK_EXECUTION_V2_ENABLED, value: "false"}',
                manifest,
            )
            self.assertIn("PLAYBOOK_EXECUTION_PROVIDER_TOKEN", manifest)
            self.assertIn(
                '{name: PLAYBOOK_EXECUTION_EVENT_PIPELINE_V2_ENABLED, value: "false"}',
                manifest,
            )
            self.assertIn("KAFKA_PLAYBOOK_EXECUTION_EVENT_TOPIC", manifest)

        rollback = (
            ROOT / "doc/07_alignment/runbooks/F-PLAYBOOK-001-rollback.md"
        ).read_text(encoding="utf-8")
        self.assertIn("PLAYBOOK_EXECUTION_V2_ENABLED", rollback)
        self.assertIn("PostgreSQL reconciliation", rollback)
        self.assertIn("do not repair it by direct SQL", rollback)

    def test_asset_cursor_is_signed_stable_and_rollout_guarded(self) -> None:
        codec = (
            ROOT / "go/control-plane/internal/asset/api/asset_cursor.go"
        ).read_text(encoding="utf-8")
        handler = (
            ROOT / "go/control-plane/internal/asset/api/http_handler.go"
        ).read_text(encoding="utf-8")
        grpc_handler = (
            ROOT / "go/control-plane/internal/asset/api/grpc_handler.go"
        ).read_text(encoding="utf-8")
        repository = (
            ROOT / "go/control-plane/internal/asset/repository/postgres.go"
        ).read_text(encoding="utf-8")
        migration = (
            ROOT
            / "deployments/postgres/migrations"
            / "202607310045_asset_cursor_v2.sql"
        ).read_text(encoding="utf-8")
        manifest = (
            ROOT / "deployments/kubernetes/applications/go-services.yaml"
        ).read_text(encoding="utf-8")

        self.assertIn("traffic.asset.cursor.v1", codec)
        self.assertIn("hmac.Equal", codec)
        self.assertIn("assetCursorFilterSHA256", codec)
        self.assertIn("cursor and offset cannot be used together", handler)
        self.assertIn('"replacement": "cursor"', handler)
        self.assertIn("req.PageToken", grpc_handler)
        self.assertIn("NextPageToken", grpc_handler)
        self.assertIn("updated_at<=", repository)
        self.assertIn("pg_visible_in_snapshot", repository)
        self.assertIn("pg_current_snapshot", repository)
        self.assertIn("(last_seen,asset_id)<", repository)
        self.assertIn("ORDER BY last_seen DESC,asset_id DESC", repository)
        self.assertIn("CREATE INDEX CONCURRENTLY", migration)
        self.assertIn("idx_assets_cursor_v2", migration)
        self.assertIn(
            '{name: ASSET_CURSOR_V2_ENABLED, value: "false"}',
            manifest,
        )

    def test_dashboard_tasks_are_real_atomic_tenant_scoped_commands(self) -> None:
        contract = json.loads(
            (ROOT / "contracts/alignment/features/F-DASHBOARD-002.json").read_text(
                encoding="utf-8"
            )
        )
        self.assertEqual(2, contract["contract_version"])
        self.assertEqual("authenticated_identity", contract["permissions"]["tenant_source"])
        self.assertEqual(
            {
                "dashboard-task-create",
                "dashboard-evidence-task-create",
                "dashboard-feedback-task-create",
                "dashboard-audit-task-create",
                "dashboard-sla-task-create",
                "dashboard-compliance-task-create",
            },
            {item["action_id"] for item in contract["ui"]["controls"]},
        )

        openapi = json.loads(
            (ROOT / "contracts/openapi/alignment-v1.openapi.json").read_text(
                encoding="utf-8"
            )
        )
        operations = {
            "/v1/dashboard/tasks": ("post", "createDashboardTask"),
            "/v1/dashboard/tasks/evidence": ("post", "createDashboardEvidenceTask"),
            "/v1/dashboard/tasks/feedback": ("post", "createDashboardFeedbackTask"),
            "/v1/dashboard/tasks/audit": ("post", "createDashboardAuditTask"),
            "/v1/dashboard/tasks/sla": ("post", "createDashboardSLATask"),
            "/v1/dashboard/tasks/compliance": ("post", "createDashboardComplianceTask"),
            "/v1/dashboard/tasks/{task_id}": ("get", "getDashboardTask"),
        }
        for path, (method, operation_id) in operations.items():
            operation = openapi["paths"][path][method]
            self.assertEqual(operation_id, operation["operationId"])
            self.assertEqual("F-DASHBOARD-002", operation["x-feature-id"])
            self.assertEqual("dashboard:write", operation["x-required-scope"])
        self.assertIn("202", openapi["paths"]["/v1/dashboard/tasks"]["post"]["responses"])

        backend = (
            ROOT / "go/control-plane/internal/alert/api/dashboard_task_v2.go"
        ).read_text(encoding="utf-8")
        for fragment in (
            "sql.LevelSerializable",
            "INSERT INTO dashboard_tasks",
            "INSERT INTO dashboard_task_history",
            "INSERT INTO dashboard_task_outbox",
            "INSERT INTO dashboard_task_requests",
            "insertDashboardTaskAudit",
            "authenticatedDashboardIdentity",
            "JSONContractAccepted",
        ):
            self.assertIn(fragment, backend)

        pipeline = (
            ROOT / "go/control-plane/internal/alert/api/dashboard_task_pipeline.go"
        ).read_text(encoding="utf-8")
        provider = (
            ROOT / "go/control-plane/internal/alert/api/dashboard_task_http_provider.go"
        ).read_text(encoding="utf-8")
        service = (ROOT / "go/control-plane/cmd/alert-service/main.go").read_text(
            encoding="utf-8"
        )
        for fragment in (
            "dashboard.task.events.v1",
            "dashboard_task_event_inbox",
            "dashboard_task_execution_attempts",
            "dashboard_task_execution_receipts",
            "CommitOnHandlerError: false",
            "DLQPermanentOnly: true",
            'RequiredAcks: "all"',
        ):
            self.assertIn(fragment, pipeline + service)
        self.assertIn("Idempotency-Key", provider)
        self.assertIn("dashboard task executor response exceeds", provider)
        self.assertEqual(
            "dashboard.task.events.v1", contract["data"]["event_topic"]
        )
        self.assertFalse(contract["rollout"]["default"])
        self.assertIn(
            "completed requires effect_state=confirmed and one or more stable effect_ids",
            contract["domain"]["invariants"],
        )

        frontend = (
            ROOT / "web/ui/src/pages/DashboardOperationsPage.tsx"
        ).read_text(encoding="utf-8")
        self.assertIn("submitDashboardTask", frontend)
        self.assertIn("fetchDashboardTask", frontend)
        self.assertIn("任务已受理，尚未最终完成", frontend)
        self.assertNotIn("仿真任务", frontend)
        self.assertNotIn("接口预留", frontend)

        for schema_path in (
            ROOT / "common/sql/pg/11-dashboard-task-v2.sql",
            ROOT / "go/control-plane/deployments/docker/init/postgres_merged.sql",
            ROOT / "deployments/kubernetes/init-jobs/02-postgres-schema.yaml",
            ROOT / "deployments/postgres/migrations/202608031620_dashboard_task_v2.sql",
        ):
            schema = schema_path.read_text(encoding="utf-8")
            for table in (
                "dashboard_tasks",
                "dashboard_task_history",
                "dashboard_task_outbox",
                "dashboard_task_requests",
            ):
                self.assertIn(table, schema)
        pipeline_schema = (
            ROOT
            / "deployments/postgres/migrations"
            / "202608041930_dashboard_task_execution_pipeline_v1.sql"
        ).read_text(encoding="utf-8")
        for table in (
            "dashboard_task_execution_attempts",
            "dashboard_task_execution_receipts",
            "dashboard_task_event_inbox",
        ):
            self.assertIn(table, pipeline_schema)

        for manifest_path in (
            ROOT / "deployments/kubernetes/applications/go-services.yaml",
            ROOT / "go/control-plane/deployments/kubernetes/alert-service.yaml",
        ):
            manifest = manifest_path.read_text(encoding="utf-8")
            self.assertIn(
                '{name: DASHBOARD_TASK_V2_ENABLED, value: "false"}',
                manifest,
            )
            self.assertIn(
                '{name: DASHBOARD_TASK_PIPELINE_V1_ENABLED, value: "false"}',
                manifest,
            )
            self.assertIn("dashboard.task.events.v1", manifest)

        rollback = (
            ROOT / "doc/07_alignment/runbooks/F-DASHBOARD-002-rollback.md"
        ).read_text(encoding="utf-8")
        self.assertIn("DASHBOARD_TASK_V2_ENABLED", rollback)
        self.assertIn("PostgreSQL reconciliation", rollback)
        self.assertIn("不允许用直接SQL伪造completed状态", rollback)
        self.assertIn("DASHBOARD_TASK_PIPELINE_V1_ENABLED", rollback)
        self.assertIn("dashboard_task_execution_receipts", rollback)

    def test_dashboard_snapshot_is_one_tenant_bound_partial_aware_query(self) -> None:
        feature = json.loads(
            (ROOT / "contracts/alignment/features/F-DASHBOARD-001.json").read_text(
                encoding="utf-8"
            )
        )
        self.assertEqual("getDashboardSnapshot", feature["api"]["operation_id"])
        self.assertEqual("/v1/dashboard/snapshot", feature["api"]["path"])
        self.assertEqual(["alert:read"], feature["permissions"]["required_scopes"])
        self.assertFalse(feature["rollout"]["default"])

        openapi = json.loads(
            (ROOT / "contracts/openapi/alignment-v1.openapi.json").read_text(
                encoding="utf-8"
            )
        )
        operation = openapi["paths"]["/v1/dashboard/snapshot"]["get"]
        self.assertEqual("F-DASHBOARD-001", operation["x-feature-id"])
        self.assertEqual("alert:read", operation["x-required-scope"])
        preserved = {
            "/v1/dashboard/stats": "getDashboardStats",
            "/v1/dashboard/alerts/trend": "getDashboardAlertTrend",
            "/v1/dashboard/attack-phases": "getDashboardAttackPhases",
            "/v1/dashboard/top-ips/{type}": "getDashboardTopIPs",
            "/v1/dashboard/encrypted/trend": "getDashboardEncryptedTrend",
        }
        self.assertEqual(
            {f"GET {path}" for path in preserved},
            set(feature["compatibility"]["preserved_api_operations"]),
        )
        for path, operation_id in preserved.items():
            legacy_operation = openapi["paths"][path]["get"]
            self.assertEqual(operation_id, legacy_operation["operationId"])
            self.assertTrue(legacy_operation["deprecated"])
            self.assertEqual("alert:read", legacy_operation["x-required-scope"])

        source = (
            ROOT / "go/control-plane/internal/alert/api/dashboard_snapshot_v1.go"
        ).read_text(encoding="utf-8")
        for token in (
            "authenticatedDashboardIdentity",
            "TENANT_SOURCE_FORBIDDEN",
            "MissingSections",
            "SourceWatermarks",
            "dashboardSnapshotID",
        ):
            self.assertIn(token, source)
        legacy = (
            ROOT / "go/control-plane/internal/alert/api/handler_dashboard.go"
        ).read_text(encoding="utf-8")
        self.assertNotIn('r.URL.Query().Get("tenant_id")', legacy)
        self.assertIn("dashboardReadTenant", legacy)
        page = (
            ROOT / "web/ui/src/pages/DashboardOperationsPage.tsx"
        ).read_text(encoding="utf-8")
        self.assertNotIn("deriveDashboardQueueRow", page)
        self.assertIn("data?.snapshot?.snapshotId", page)
        plans = (ROOT / "web/ui/src/services/pageApiPlans.ts").read_text(
            encoding="utf-8"
        )
        self.assertIn('primary: "/v1/dashboard/snapshot"', plans)
        self.assertNotIn('secondary: ["/v1/dashboard/alerts/trend"', plans)
        self.assertTrue(
            (ROOT / "doc/07_alignment/runbooks/F-DASHBOARD-001-rollback.md").exists()
        )
        capture = (
            ROOT / "scripts/alignment/capture_dashboard_snapshot_v1.py"
        ).read_text(encoding="utf-8")
        self.assertIn('"status": "PARTIAL" if scoped_pass else "FAIL"', capture)
        self.assertIn("OPEN_FOR_RELEASE_CANDIDATE_CLICKHOUSE", capture)

    def test_asset_discovery_jobs_are_durable_and_candidate_isolated(self) -> None:
        service = (
            ROOT / "go/control-plane/internal/asset/service/discovery_jobs.go"
        ).read_text(encoding="utf-8")
        repository = (
            ROOT / "go/control-plane/internal/asset/repository/discovery_jobs.go"
        ).read_text(encoding="utf-8")
        candidate_merge = (
            ROOT
            / "go/control-plane/internal/asset/repository/discovery_candidate_merge.go"
        ).read_text(encoding="utf-8")
        handler = (
            ROOT / "go/control-plane/internal/asset/api/http_handler.go"
        ).read_text(encoding="utf-8")
        scheduler = (
            ROOT / "go/control-plane/internal/asset/service/discovery_scheduler.go"
        ).read_text(encoding="utf-8")
        migration = (
            ROOT
            / "deployments/postgres/migrations"
            / "202607310100_asset_discovery_jobs_v2.sql"
        ).read_text(encoding="utf-8")
        manifest = (
            ROOT / "deployments/kubernetes/applications/go-services.yaml"
        ).read_text(encoding="utf-8")
        openapi = (
            ROOT / "contracts/openapi/alignment-v1.openapi.json"
        ).read_text(encoding="utf-8")

        self.assertIn("SubmitActiveDiscovery", service)
        self.assertIn("observations are worker output", service)
        self.assertIn("ProcessNextDiscoveryJob", service)
        self.assertIn("pg_advisory_xact_lock", repository)
        self.assertIn("target_network && $2::cidr", repository)
        self.assertIn("asset_discovery_candidates", repository)
        self.assertIn("MergeDiscoveryCandidateAtomic", candidate_merge)
        self.assertIn("asset_events", candidate_merge)
        self.assertIn("ASSET_DISCOVERY_CANDIDATE_MERGED", candidate_merge)
        self.assertIn("asset_event_outbox", candidate_merge)
        self.assertIn("asset_discovery_control_requests", candidate_merge)
        self.assertIn("pg_advisory_xact_lock", candidate_merge)
        self.assertNotIn("UpsertAsset(", service)
        self.assertIn("http.StatusAccepted", handler)
        self.assertIn("mergeDiscoveryCandidate", handler)
        self.assertIn("mergeAssetDiscoveryCandidate", openapi)
        self.assertIn("SubmitActiveDiscovery", scheduler)
        self.assertNotIn("RunActiveDiscovery(ctx, req)", scheduler.split("if s.cfg.Discovery.JobsV2Enabled", 1)[1].split("}", 1)[0])
        self.assertIn("asset_discovery_run_history", migration)
        self.assertIn("asset_discovery_outbox", migration)
        self.assertIn("result_payload", migration)
        self.assertIn("'merge_candidate'", migration)
        self.assertIn("idx_asset_discovery_run_reclaim", migration)
        self.assertIn(
            '{name: ASSET_DISCOVERY_JOBS_V2_ENABLED, value: "false"}',
            manifest,
        )
        self.assertIn(
            '{name: ASSET_DISCOVERY_WORKER_ENABLED, value: "false"}',
            manifest,
        )


if __name__ == "__main__":
    unittest.main()
