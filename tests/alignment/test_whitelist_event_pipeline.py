import json
import unittest
from pathlib import Path


ROOT = Path(__file__).resolve().parents[2]


class WhitelistEventPipelineContractTests(unittest.TestCase):
    def test_topic_schema_acl_and_default_off_deployments_are_bound(self):
        topics = json.loads(
            (ROOT / "contracts/events/kafka-topic-catalog.v1.json").read_text(encoding="utf-8")
        )
        topic = next(item for item in topics["topics"] if item["name"] == "whitelist.events.v2")
        self.assertEqual("producer_candidate_default_off", topic["readiness"])
        self.assertEqual("tenant_id+entry_id", topic["key_contract"])
        self.assertEqual("WhitelistLifecycleV2Json", topic["message_type"])
        self.assertEqual(
            ["go/control-plane/internal/alert/whitelist/outbox_dispatcher.go"],
            topic["producers"],
        )
        self.assertEqual(
            ["go/control-plane/internal/rules/consumer/whitelist_rule_effect_consumer.go"],
            topic["consumers"],
        )

        schema = json.loads(
            (ROOT / "contracts/events/kafka-json-events-v1.schema.json").read_text(encoding="utf-8")
        )["$defs"]["WhitelistLifecycleV2Json"]
        self.assertFalse(schema["additionalProperties"])
        self.assertEqual(2, schema["properties"]["schema_version"]["const"])
        self.assertIn("desired_rule_state", schema["required"])

        acl = json.loads(
            (ROOT / "contracts/events/kafka-acl-catalog.v1.json").read_text(encoding="utf-8")
        )
        binding = next(item for item in acl["topic_bindings"] if item["topic"] == "whitelist.events.v2")
        self.assertEqual(["alert-service"], binding["producers"])
        self.assertEqual(
            [{"principal": "rule-manager", "groups": ["rule-manager-whitelist-rule-effect-v2"]}],
            binding["consumers"],
        )

        for relative in (
            "deployments/kubernetes/applications/go-services.yaml",
            "go/control-plane/deployments/kubernetes/alert-service.yaml",
            "go/control-plane/deployments/kubernetes/rule-manager.yaml",
        ):
            source = (ROOT / relative).read_text(encoding="utf-8")
            self.assertIn("whitelist.events.v2", source)
            self.assertIn("WHITELIST_EVENT_PIPELINE_V2_ENABLED", source)
            self.assertIn('value: "false"', source)

        alert = (ROOT / "deployments/kubernetes/applications/go-services.yaml").read_text(encoding="utf-8")
        for flag in (
            "WHITELIST_EVENT_CONSUMER_V2_ENABLED",
            "WHITELIST_EVENT_PRODUCER_V2_ENABLED",
            "WHITELIST_DETECTION_MATCHER_V2_ENABLED",
        ):
            self.assertIn(flag, alert)

    def test_projection_schema_is_present_in_every_postgres_entrypoint(self):
        for relative in (
            "deployments/postgres/migrations/202608071930_whitelist_rule_projection_v1.sql",
            "common/sql/pg/10-whitelist-governance-v2.sql",
            "deployments/kubernetes/init-jobs/02-postgres-schema.yaml",
            "go/control-plane/deployments/docker/init/postgres_merged.sql",
        ):
            source = (ROOT / relative).read_text(encoding="utf-8")
            self.assertIn("whitelist_rule_projection", source)
            self.assertIn("202608071930", source)

    def test_consumer_readiness_migration_is_in_runtime_entrypoints(self):
        for relative in (
            "deployments/postgres/migrations/202608161100_m09_whitelist_consumer_readiness_v2.sql",
            "deployments/kubernetes/init-jobs/02-postgres-schema.yaml",
            "go/control-plane/deployments/docker/init/postgres_merged.sql",
        ):
            source = (ROOT / relative).read_text(encoding="utf-8")
            self.assertIn("whitelist_consumer_readiness_receipt", source)
            self.assertIn("202608161100", source)


if __name__ == "__main__":
    unittest.main()
