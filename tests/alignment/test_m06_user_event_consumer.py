import hashlib
import json
import unittest
from pathlib import Path

import yaml

ROOT = Path(__file__).resolve().parents[2]
CONTRACT = ROOT / "contracts/events/user-events-consumer.v1.json"
MANIFEST = ROOT / "deployments/kubernetes/flink/flink-user-behavior-job.yaml"


def candidate_digest(files: list[str]) -> str:
    value = hashlib.sha256()
    for relative in files:
        value.update(hashlib.sha256((ROOT / relative).read_bytes()).hexdigest().encode("ascii"))
        value.update(b"  ")
        value.update(relative.encode("utf-8"))
        value.update(b"\n")
    return value.hexdigest()


class M06UserEventConsumerTest(unittest.TestCase):
    @classmethod
    def setUpClass(cls) -> None:
        cls.contract = json.loads(CONTRACT.read_text(encoding="utf-8"))
        cls.manifest = yaml.safe_load(MANIFEST.read_text(encoding="utf-8"))
        cls.acl = json.loads((ROOT / "contracts/events/kafka-acl-catalog.v1.json").read_text(encoding="utf-8"))

    def test_contract_is_consumer_first_and_source_tuple_owned(self) -> None:
        self.assertEqual("T1-M06-N007", self.contract["accountable_task"])
        self.assertEqual("consumer_ready_shadow", self.contract["state"])
        self.assertFalse(self.contract["rollout"]["projection_writes"])
        self.assertEqual("utf8(tenant_id + ':' + user_id)", self.contract["identity"]["key_encoding"])
        self.assertIn("different payload SHA-256", self.contract["identity"]["conflict_identity"])

    def test_job_uses_raw_records_exact_checkpoint_and_two_barrier_sinks(self) -> None:
        source = (ROOT / "java/flink-jobs/flink-user-behavior-job/src/main/java/com/traffic/flink/behavior/user/UserBehaviorJob.java").read_text(encoding="utf-8")
        parser = (ROOT / "java/flink-jobs/flink-user-behavior-job/src/main/java/com/traffic/flink/behavior/user/UserEventParseFunction.java").read_text(encoding="utf-8")
        quality = (ROOT / "java/flink-jobs/flink-user-behavior-job/src/main/java/com/traffic/flink/behavior/user/UserEventTimeFunction.java").read_text(encoding="utf-8")
        self.assertIn("RawKafkaRecordDeserializationSchema", source)
        self.assertNotIn("setValueOnlyDeserializer", source)
        self.assertNotIn("Filter Invalid Events", source)
        self.assertIn("CheckpointingMode.EXACTLY_ONCE", source)
        self.assertGreaterEqual(source.count("DeliveryGuarantee.EXACTLY_ONCE"), 2)
        self.assertIn("duplicate Kafka headers", parser)
        self.assertIn("Kafka key must equal tenant_id:user_id", parser)
        self.assertIn("DUPLICATE_EVENT", quality)
        self.assertIn("EVENT_ID_CONFLICT", quality)
        self.assertIn("EventTimePolicy.isLate", quality)

    def test_manifest_is_candidate_bound_shadow_and_hash_exact(self) -> None:
        metadata = self.manifest["metadata"]
        digest = metadata["annotations"]["traffic.analysis/candidate-sha256"]
        self.assertEqual(candidate_digest(self.contract["rollout"]["candidate_digest"]["files"]), digest)
        self.assertEqual("false", metadata["annotations"]["traffic.analysis/production-applied"])
        pod = self.manifest["spec"]["template"]["spec"]
        self.assertEqual("flink-user-behavior-job", pod["serviceAccountName"])
        self.assertFalse(pod["automountServiceAccountToken"])
        container = pod["containers"][0]
        env = {item["name"]: item for item in container["env"]}
        self.assertEqual("shadow", env["DEPLOYMENT_ACTIVATION_MODE"]["value"])
        self.assertEqual("false", env["PROJECTION_WRITES_ENABLED"]["value"])
        self.assertEqual(digest, env["DEPLOYMENT_CANDIDATE_SHA256"]["value"])
        self.assertEqual("flink-user-behavior-job-shadow-" + digest[:12], env["KAFKA_GROUP_ID"]["value"])
        self.assertTrue(container["securityContext"]["readOnlyRootFilesystem"])

    def test_acl_allows_input_dlq_and_quality_receipt_only(self) -> None:
        bindings = {item["topic"]: item for item in self.acl["topic_bindings"]}
        self.assertIn("flink-user-behavior-job", bindings["user.events.v1"]["consumers"][0]["groups"])
        self.assertIn("flink-user-behavior-job", bindings["dlq.v1"]["producers"])
        self.assertIn("flink-user-behavior-job", bindings["audit.logs"]["producers"])


if __name__ == "__main__":
    unittest.main()
