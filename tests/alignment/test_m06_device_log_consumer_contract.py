import base64
import hashlib
import json
import unittest
from pathlib import Path

import yaml


ROOT = Path(__file__).resolve().parents[2]
CONTRACT_PATH = ROOT / "contracts/events/device-logs-producer.v1.json"
MANIFEST_PATH = ROOT / "deployments/kubernetes/flink/flink-log-job.yaml"
def candidate_digest(files: list[str]) -> str:
    aggregate = hashlib.sha256()
    for relative in files:
        path = ROOT / relative
        aggregate.update(hashlib.sha256(path.read_bytes()).hexdigest().encode("ascii"))
        aggregate.update(b"  ")
        aggregate.update(relative.encode("utf-8"))
        aggregate.update(b"\n")
    return aggregate.hexdigest()


class M06DeviceLogConsumerContractTest(unittest.TestCase):
    @classmethod
    def setUpClass(cls) -> None:
        cls.contract = json.loads(CONTRACT_PATH.read_text(encoding="utf-8"))
        cls.manifest = yaml.safe_load(MANIFEST_PATH.read_text(encoding="utf-8"))
        cls.topic_catalog = json.loads(
            (ROOT / "contracts/events/kafka-topic-catalog.v1.json").read_text(
                encoding="utf-8"
            )
        )
        cls.acl_catalog = json.loads(
            (ROOT / "contracts/events/kafka-acl-catalog.v1.json").read_text(
                encoding="utf-8"
            )
        )

    def test_contract_binds_current_proto_and_keeps_producer_default_off(self) -> None:
        proto = ROOT / self.contract["value"]["proto_file"]
        self.assertEqual(
            hashlib.sha256(proto.read_bytes()).hexdigest(),
            self.contract["value"]["proto_file_sha256"],
        )
        self.assertEqual("producer_candidate_default_off", self.contract["state"])
        self.assertFalse(self.contract["rollout"]["producer_enabled"])
        self.assertEqual("device.logs.v1", self.contract["topic"])
        self.assertEqual(
            "utf8(tenant_id + ':' + device_ip)",
            self.contract["identity"]["key_encoding"],
        )
        self.assertEqual(
            "no completed checkpoint when canonical DLQ write fails",
            self.contract["failure_barrier"]["source_offset_policy"],
        )
        self.assertEqual(
            "no completed checkpoint when either DLQ or source-quality receipt write fails",
            self.contract["source_quality_barrier"]["source_offset_policy"],
        )

    def test_java_pipeline_keeps_source_tuple_and_has_checkpoint_coupled_dlq(self) -> None:
        job = (
            ROOT
            / "java/flink-jobs/flink-log-job/src/main/java/com/traffic/flink/log/LogJob.java"
        ).read_text(encoding="utf-8")
        parser = (
            ROOT
            / "java/flink-jobs/flink-log-job/src/main/java/com/traffic/flink/log/source/DeviceLogParseFunction.java"
        ).read_text(encoding="utf-8")
        event_time = (
            ROOT
            / "java/flink-jobs/flink-log-job/src/main/java/com/traffic/flink/log/source/DeviceLogEventTimeFunction.java"
        ).read_text(encoding="utf-8")
        sink = (
            ROOT
            / "java/flink-jobs/flink-log-job/src/main/java/com/traffic/flink/log/sink/LogDlqSinkFactory.java"
        ).read_text(encoding="utf-8")
        quality_sink = (
            ROOT
            / "java/flink-jobs/flink-log-job/src/main/java/com/traffic/flink/log/sink/LogSourceQualitySinkFactory.java"
        ).read_text(encoding="utf-8")

        self.assertIn("RawKafkaRecordDeserializationSchema", job)
        self.assertNotIn("setValueOnlyDeserializer", job)
        self.assertIn("CheckpointingMode.EXACTLY_ONCE", job)
        self.assertIn("DeviceLogParseFunction", job)
        self.assertIn("DeviceLogEventTimeFunction", job)
        self.assertIn("LogDlqSinkFactory.create(config)", job)
        self.assertIn("LogSourceQualitySinkFactory.create(config)", job)
        for header in self.contract["identity"]["required_headers"]:
            self.assertIn('"' + header + '"', parser)
        self.assertIn("CLOCK_ROLLBACK", event_time)
        self.assertIn("LATE_EVENT", event_time)
        self.assertIn("DeliveryGuarantee.EXACTLY_ONCE", sink)
        self.assertIn("setTransactionalIdPrefix", sink)
        self.assertIn("DeliveryGuarantee.EXACTLY_ONCE", quality_sink)
        self.assertIn("setTransactionalIdPrefix", quality_sink)

    def test_kubernetes_job_is_shadow_isolated_and_projection_off(self) -> None:
        metadata = self.manifest["metadata"]
        candidate = metadata["annotations"]["traffic.analysis/candidate-sha256"]
        self.assertRegex(candidate, r"^[0-9a-f]{64}$")
        self.assertEqual(
            candidate_digest(self.contract["rollout"]["candidate_digest"]["files"]),
            candidate,
        )
        self.assertEqual("source-tree-sha256", metadata["annotations"]["traffic.analysis/candidate-kind"])
        self.assertEqual("false", metadata["annotations"]["traffic.analysis/production-applied"])

        pod = self.manifest["spec"]["template"]["spec"]
        self.assertEqual("flink-log-job", pod["serviceAccountName"])
        self.assertFalse(pod["automountServiceAccountToken"])
        container = pod["containers"][0]
        env = {item["name"]: item for item in container["env"]}
        self.assertEqual("shadow", env["DEPLOYMENT_ACTIVATION_MODE"]["value"])
        self.assertEqual(candidate, env["DEPLOYMENT_CANDIDATE_SHA256"]["value"])
        self.assertEqual("false", env["PROJECTION_WRITES_ENABLED"]["value"])
        self.assertEqual(
            "flink-log-job-shadow-" + candidate[:12],
            env["KAFKA_GROUP_ID"]["value"],
        )
        self.assertEqual(
            "kafka-flink-log-job-credentials",
            env["KAFKA_SASL_USERNAME"]["valueFrom"]["secretKeyRef"]["name"],
        )
        self.assertEqual("SASL_SSL", env["KAFKA_SECURITY_PROTOCOL"]["value"])
        self.assertTrue(container["securityContext"]["readOnlyRootFilesystem"])

    def test_catalog_and_acl_bind_default_off_producer_and_dlq_barrier(self) -> None:
        topics = {item["name"]: item for item in self.topic_catalog["topics"]}
        device = topics["device.logs.v1"]
        self.assertEqual("producer_candidate_default_off", device["readiness"])
        self.assertEqual(
            ["deployments/log-collectors/device-logs.yaml"], device["producers"]
        )
        self.assertEqual(
            "contracts/events/device-logs-producer.v1.json",
            device["consumer_contract"],
        )
        self.assertIn(
            "java/flink-jobs/flink-log-job/src/main/java/com/traffic/flink/log/sink/LogDlqSinkFactory.java",
            topics["dlq.v1"]["producers"],
        )

        bindings = {item["topic"]: item for item in self.acl_catalog["topic_bindings"]}
        device_consumer = bindings["device.logs.v1"]["consumers"][0]
        self.assertEqual(
            ["device-log-collector"], bindings["device.logs.v1"]["producers"]
        )
        self.assertEqual(["flink-log-job-shadow-"], device_consumer["group_prefixes"])
        self.assertIn("flink-log-job", bindings["dlq.v1"]["producers"])
        self.assertIn("flink-log-job", bindings["audit.logs"]["producers"])

    def test_vector_collector_is_inert_durable_and_fail_closed(self) -> None:
        config_doc = yaml.safe_load(
            (ROOT / self.contract["collector"]["config"]).read_text(encoding="utf-8")
        )
        vector = config_doc["data"]["vector.yaml"]
        self.assertEqual("{}", config_doc["data"]["device-tenant-map.json"])
        for required in (
            'get_env_var!("DEVICE_TENANT_MAP_JSON")',
            "device_ip = string!(.host)",
            "parse_syslog!(raw)",
            "to_unix_timestamp!(structured.timestamp",
            'variant: "SHA-256"',
            "codec: protobuf",
            "message_type: traffic.v1.DeviceLog",
            "headers_key: kafka_headers",
            "key_field: kafka_key",
            "type: disk",
            "when_full: block",
            "normalize_device_log.dropped",
        ):
            self.assertIn(required, vector)
        self.assertNotIn("now()", vector)

        descriptor = base64.b64decode(config_doc["binaryData"]["device-log.desc"])
        self.assertIn(b"traffic.v1", descriptor)
        self.assertIn(b"DeviceLog", descriptor)
        for field in (
            "log_id", "tenant_id", "device_ip", "device_type", "facility",
            "severity", "timestamp", "message", "parsed", "source",
        ):
            self.assertIn(field.encode("ascii"), descriptor)

        docs = list(
            yaml.safe_load_all(
                (ROOT / self.contract["collector"]["manifest"]).read_text(encoding="utf-8")
            )
        )
        stateful = next(item for item in docs if item["kind"] == "StatefulSet")
        self.assertEqual(0, stateful["spec"]["replicas"])
        pod = stateful["spec"]["template"]["spec"]
        self.assertFalse(pod["automountServiceAccountToken"])
        container = pod["containers"][0]
        self.assertRegex(container["image"], r"@sha256:[0-9a-f]{64}$")
        self.assertTrue(container["securityContext"]["readOnlyRootFilesystem"])
        claims = stateful["spec"]["volumeClaimTemplates"]
        self.assertEqual("10Gi", claims[0]["spec"]["resources"]["requests"]["storage"])

        secret = yaml.safe_load(
            (ROOT / self.contract["collector"]["credential"]).read_text(encoding="utf-8")
        )
        self.assertEqual("ExternalSecret", secret["kind"])
        self.assertEqual(
            "traffic-device-log-collector",
            secret["spec"]["target"]["template"]["data"]["username"],
        )


if __name__ == "__main__":
    unittest.main()
