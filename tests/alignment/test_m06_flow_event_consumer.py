import hashlib
import json
import unittest
from pathlib import Path

import yaml


ROOT = Path(__file__).resolve().parents[2]
CONTRACT = ROOT / "contracts/events/flow-events-consumer.v1.json"
MANIFEST = ROOT / "deployments/kubernetes/flink/flink-session-job.yaml"


def candidate_digest(files: list[str]) -> str:
    value = hashlib.sha256()
    for relative in files:
        value.update(hashlib.sha256((ROOT / relative).read_bytes()).hexdigest().encode("ascii"))
        value.update(b"  ")
        value.update(relative.encode("utf-8"))
        value.update(b"\n")
    return value.hexdigest()


class M06FlowEventConsumerTest(unittest.TestCase):
    @classmethod
    def setUpClass(cls) -> None:
        cls.contract = json.loads(CONTRACT.read_text(encoding="utf-8"))
        cls.manifest = yaml.safe_load(MANIFEST.read_text(encoding="utf-8"))
        cls.acl = json.loads(
            (ROOT / "contracts/events/kafka-acl-catalog.v1.json").read_text(encoding="utf-8")
        )

    def test_contract_pins_identity_time_and_checkpoint_barriers(self) -> None:
        self.assertEqual("consumer_ready_shadow", self.contract["state"])
        self.assertEqual("flow.events.v1", self.contract["topic"])
        self.assertEqual(
            "utf8(tenant_id + ':' + community_id)",
            self.contract["identity"]["key_encoding"],
        )
        self.assertIn("different payload SHA-256", self.contract["identity"]["conflict_identity"])
        self.assertEqual("EXACTLY_ONCE", self.contract["barriers"]["checkpoint_mode"])
        self.assertFalse(self.contract["rollout"]["projection_writes"])

    def test_java_pipeline_is_raw_strict_stateful_and_transactional(self) -> None:
        job = (ROOT / "java/flink-jobs/flink-session-job/src/main/java/com/traffic/flink/session/SessionJob.java").read_text()
        parser = (ROOT / "java/flink-jobs/flink-session-job/src/main/java/com/traffic/flink/session/source/FlowEventParseFunction.java").read_text()
        quality = (ROOT / "java/flink-jobs/flink-session-job/src/main/java/com/traffic/flink/session/source/FlowLatenessFunction.java").read_text()
        sink = (ROOT / "java/flink-jobs/flink-session-job/src/main/java/com/traffic/flink/session/sink/KafkaSinkFactory.java").read_text()
        self.assertIn("RawKafkaRecordDeserializationSchema", job)
        self.assertIn("CheckpointingMode.EXACTLY_ONCE", job)
        self.assertIn("ValidatedFlowInput::identityKey", job)
        self.assertNotIn("flow-dlq-external-write-gate", job)
        for header in self.contract["identity"]["required_headers"]:
            self.assertIn('"' + header + '"', parser)
        self.assertIn("DuplicateHeaderNames", parser)
        self.assertIn("DUPLICATE_EVENT", quality)
        self.assertIn("EVENT_ID_CONFLICT", quality)
        self.assertIn("SUPER_LATE_EVENT", quality)
        self.assertGreaterEqual(sink.count("DeliveryGuarantee.EXACTLY_ONCE"), 2)
        self.assertGreaterEqual(sink.count("setTransactionalIdPrefix"), 2)

    def test_kubernetes_submitter_is_candidate_bound_and_executes_flink_run(self) -> None:
        digest = self.manifest["metadata"]["annotations"]["traffic.analysis/candidate-sha256"]
        self.assertEqual(candidate_digest(self.contract["rollout"]["candidate_digest"]["files"]), digest)
        self.assertEqual("false", self.manifest["metadata"]["annotations"]["traffic.analysis/production-applied"])
        pod = self.manifest["spec"]["template"]["spec"]
        self.assertEqual("flink-session-job", pod["serviceAccountName"])
        self.assertFalse(pod["automountServiceAccountToken"])
        container = pod["containers"][0]
        args = container["args"]
        self.assertEqual("run", args[0])
        self.assertIn("flink-jobmanager.flink.svc:8081", args)
        self.assertIn("shadow", args)
        self.assertIn("flink-session-job-shadow-" + digest[:12], args)
        self.assertIn(digest, args)
        self.assertIn("false", args)
        self.assertTrue(container["securityContext"]["readOnlyRootFilesystem"])

    def test_acl_allows_input_dlq_and_quality_topics(self) -> None:
        bindings = {item["topic"]: item for item in self.acl["topic_bindings"]}
        self.assertIn("flink-session-job", bindings["flow.events.v1"]["consumers"][0]["groups"])
        self.assertIn("flink-session-job", bindings["dlq.v1"]["producers"])
        self.assertIn("flink-session-job", bindings["audit.logs"]["producers"])


if __name__ == "__main__":
    unittest.main()
