import json
import unittest
from pathlib import Path

import yaml


ROOT = Path(__file__).resolve().parents[2]


class M06AssetBindingProducerContractTest(unittest.TestCase):
    def setUp(self) -> None:
        self.contract = json.loads(
            (ROOT / "contracts/events/asset-bindings-producer.v1.json").read_text(
                encoding="utf-8"
            )
        )

    def test_wire_contract_is_single_gateway_owned_protobuf_rail(self) -> None:
        self.assertEqual("contract_ready_producer_and_consumer_default_off", self.contract["state"])
        self.assertFalse(self.contract["production_applied"])
        self.assertTrue(self.contract["transport"]["gateway_is_only_kafka_producer"])
        self.assertTrue(self.contract["transport"]["probe_direct_kafka_forbidden"])
        self.assertEqual("traffic.v1.MacIpBinding", self.contract["payload"]["message"])
        self.assertEqual(
            ["tenant_id", "probe_id", "observation_id"],
            self.contract["identity"]["idempotency_scope"],
        )

    def test_proto_gateway_and_consumer_implement_exact_contract(self) -> None:
        ingest_proto = (ROOT / "proto/traffic/v1/ingest.proto").read_text(encoding="utf-8")
        asset_proto = (ROOT / "proto/traffic/v1/asset.proto").read_text(encoding="utf-8")
        producer = (ROOT / "go/control-plane/internal/ingest/queue/producer.go").read_text(
            encoding="utf-8"
        )
        handler = (ROOT / "go/control-plane/internal/ingest/server/handler.go").read_text(
            encoding="utf-8"
        )
        consumer = (ROOT / "go/control-plane/internal/asset/consumer/binding_consumer.go").read_text(
            encoding="utf-8"
        )
        self.assertIn("rpc UploadAssetBindings", ingest_proto)
        for field_number in range(6, 11):
            self.assertIn(f"= {field_number};", asset_proto)
        self.assertIn("func (p *Producer) WriteAssetBindings", producer)
        self.assertIn("KAFKA_REQUIRED_ACKS_ALL", producer)
        self.assertIn("func (h *IngestHandler) UploadAssetBindings", handler)
        self.assertIn("AcceptedResponseRevision < 1", handler)
        self.assertIn("SetDLQAcknowledgementBarrier", consumer)
        for header in self.contract["payload"]["required_headers"]:
            self.assertIn(f'"{header}"', producer)
            self.assertIn(f'"{header}"', consumer)

    def test_catalog_acl_and_kubernetes_are_additive_but_default_off(self) -> None:
        topics = json.loads(
            (ROOT / "contracts/events/kafka-topic-catalog.v1.json").read_text(encoding="utf-8")
        )
        binding_topic = next(item for item in topics["topics"] if item["name"] == "asset.bindings.v1")
        self.assertEqual("producer_candidate_default_off", binding_topic["readiness"])
        self.assertEqual(
            ["go/control-plane/internal/ingest/queue/producer.go"],
            binding_topic["producers"],
        )
        self.assertEqual(
            "contracts/events/asset-bindings-producer.v1.json",
            binding_topic["producer_contract"],
        )
        acl = json.loads(
            (ROOT / "contracts/events/kafka-acl-catalog.v1.json").read_text(encoding="utf-8")
        )
        binding_acl = next(item for item in acl["topic_bindings"] if item["topic"] == "asset.bindings.v1")
        self.assertEqual(["ingest-gateway"], binding_acl["producers"])

        documents = list(
            yaml.safe_load_all(
                (ROOT / "deployments/kubernetes/applications/go-services.yaml").read_text(
                    encoding="utf-8"
                )
            )
        )
        ingest = next(
            item for item in documents
            if item and item.get("kind") == "Deployment" and item["metadata"]["name"] == "ingest-gateway"
        )
        env = {
            item["name"]: item.get("value")
            for item in ingest["spec"]["template"]["spec"]["containers"][0]["env"]
        }
        self.assertEqual("asset.bindings.v1", env["KAFKA_ASSET_BINDING_TOPIC"])
        self.assertEqual("false", env["M06_ASSET_BINDING_WRITER_V1_ENABLED"])

        probe_documents = list(
            yaml.safe_load_all(
                (ROOT / "deployments/kubernetes/applications/probe-agent.yaml").read_text(
                    encoding="utf-8"
                )
            )
        )
        probe = next(item for item in probe_documents if item and item.get("kind") == "DaemonSet")
        probe_env = {
            item["name"]: item.get("value")
            for item in probe["spec"]["template"]["spec"]["containers"][0]["env"]
        }
        self.assertEqual("false", probe_env["M06_ASSET_BINDING_UPLOAD_V1_ENABLED"])
        self.assertEqual("", probe_env["M06_ASSET_BINDING_CANARY_TENANT_ID"])
        self.assertEqual("", probe_env["M06_ASSET_BINDING_CANARY_PROBE_IDS"])

    def test_probe_uses_durable_grpc_spool_and_has_no_kafka_client(self) -> None:
        sender = (ROOT / "rust/probe-agent/probe-agent/src/sender/grpc.rs").read_text(
            encoding="utf-8"
        )
        spool = (ROOT / "rust/probe-agent/probe-agent/src/sender/asset_binding.rs").read_text(
            encoding="utf-8"
        )
        main = (ROOT / "rust/probe-agent/probe-agent/src/main.rs").read_text(encoding="utf-8")
        cargo = (ROOT / "rust/probe-agent/probe-agent/Cargo.toml").read_text(encoding="utf-8")
        self.assertIn("pub async fn send_asset_bindings", sender)
        self.assertIn("accepted_response_revision: 1", sender)
        self.assertIn("pub struct AssetBindingSpool", spool)
        self.assertIn("asset binding response cardinality mismatch", spool)
        self.assertIn("run_asset_binding_sender", main)
        self.assertNotIn("rdkafka", cargo)
        self.assertNotRegex(cargo, r"(?m)^kafka\s*=")


if __name__ == "__main__":
    unittest.main()
