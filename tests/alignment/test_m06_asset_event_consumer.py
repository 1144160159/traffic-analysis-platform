import json
import unittest
from pathlib import Path


ROOT = Path(__file__).resolve().parents[2]
CONTRACT = ROOT / "contracts/events/asset-events-consumer.v1.json"


class M06AssetEventConsumerTest(unittest.TestCase):
    @classmethod
    def setUpClass(cls) -> None:
        cls.contract = json.loads(CONTRACT.read_text(encoding="utf-8"))

    def test_contract_separates_source_replay_duplicate_and_conflict(self) -> None:
        identity = self.contract["identity"]
        self.assertEqual("accepted idempotent retry", identity["same_source_tuple_replay"])
        self.assertEqual("duplicate", identity["same_event_and_payload_new_source_tuple"])
        self.assertEqual("conflict", identity["same_event_or_aggregate_version_different_payload"])
        self.assertFalse(self.contract["rollout"]["production_applied"])

    def test_handler_records_quality_before_success_and_after_dlq_ack(self) -> None:
        source = (ROOT / "go/control-plane/internal/asset/consumer/asset_projection_event.go").read_text()
        main = (ROOT / "go/control-plane/cmd/asset-service/main.go").read_text()
        common = (ROOT / "go/control-plane/internal/common/kafka/consumer.go").read_text()
        self.assertIn("AcceptClassified", source)
        self.assertIn("qualityRecorder.Record", source)
        self.assertIn("RecordDLQAcknowledgement", source)
        self.assertIn("kafkaCommon.Permanent", source)
        self.assertIn("DuplicateHeaderNames", source)
        self.assertIn("SetDLQAcknowledgementBarrier", main)
        self.assertIn("CommitOnDLQSuccess:   true", main)
        self.assertIn("DLQPermanentOnly:     true", main)
        self.assertLess(
            common.index("runDLQAcknowledgementBarrier"),
            common.index("commitMessages([]kafka.Message{msg})"),
        )

    def test_topics_and_acl_are_canonical(self) -> None:
        topics = json.loads((ROOT / "contracts/events/kafka-topic-catalog.v1.json").read_text())
        acls = json.loads((ROOT / "contracts/events/kafka-acl-catalog.v1.json").read_text())
        topic = {item["name"]: item for item in topics["topics"]}["asset.events.v2"]
        binding = {item["topic"]: item for item in acls["topic_bindings"]}
        self.assertIn("internal/asset/repository/outbox_dispatcher.go", topic["producers"][0])
        self.assertIn("asset-service", binding["asset.events.v2"]["producers"])
        self.assertIn("asset-service", binding["dlq.v1"]["producers"])


if __name__ == "__main__":
    unittest.main()
