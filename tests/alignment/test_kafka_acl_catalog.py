import copy
import json
import subprocess
import sys
import tempfile
import unittest
from pathlib import Path

import yaml


ROOT = Path(__file__).resolve().parents[2]
sys.path.insert(0, str(ROOT / "scripts/alignment"))

from generate_kafka_acl_plan import (  # noqa: E402
    render_configmap,
    render_service_identities,
    render_shell,
    split_service_identity_bundle,
    validate_documents,
)
from render_audit_materializer_expand import render as render_audit_materializer  # noqa: E402


class KafkaAclCatalogTest(unittest.TestCase):
    def setUp(self) -> None:
        self.acl = json.loads(
            (ROOT / "contracts/events/kafka-acl-catalog.v1.json").read_text(
                encoding="utf-8"
            )
        )
        self.topics = json.loads(
            (ROOT / "contracts/events/kafka-topic-catalog.v1.json").read_text(
                encoding="utf-8"
            )
        )

    def test_catalog_covers_every_canonical_topic_and_blocks_legacy_extras(self) -> None:
        result = validate_documents(self.acl, self.topics)
        self.assertEqual("pass", result["result"], result)
        self.assertEqual(45, result["counts"]["canonical_topics"])
        self.assertEqual(45, result["counts"]["topic_bindings"])
        self.assertEqual(6, result["counts"]["additional_topics_blocked"])
        self.assertEqual(21, result["counts"]["principals"])
        self.assertEqual(8, result["counts"]["expanded_service_identities"])
        self.assertEqual(19, result["counts"]["expanded_workload_identities"])

    def test_generated_plan_is_literal_and_contains_no_all_or_wildcard_grant(self) -> None:
        shell = render_shell(self.acl)
        grant_lines = [
            line for line in shell.splitlines()
            if line.startswith(("apply_literal_acl ", "apply_prefixed_acl "))
        ]
        self.assertEqual(314, len(grant_lines))
        self.assertTrue(all("--allow-principal User:traffic-" in line for line in grant_lines))
        self.assertTrue(all("--operation All" not in line for line in grant_lines))
        self.assertTrue(all("--topic *" not in line for line in grant_lines))
        self.assertTrue(all("--group *" not in line for line in grant_lines))
        self.assertNotIn("traffic-kafka-replayer", shell)
        self.assertIn(
            "User:traffic-flink-behavior --operation Read --group "
            "flink-behavior-job-champion-challenger-model-updates",
            shell,
        )
        self.assertIn(
            "User:traffic-flink-behavior --operation Read --group "
            "flink-behavior-job-champion-challenger-features",
            shell,
        )
        self.assertIn(
            "User:traffic-flink-behavior --operation Write --topic "
            "model-shadow-observations.v1",
            shell,
        )
        self.assertIn(
            "User:traffic-rule-manager --operation Read --group "
            "rule-manager-model-tenant-canary-v1",
            shell,
        )
        self.assertIn(
            "User:traffic-rule-manager --operation Read --topic "
            "model-shadow-observations.v1",
            shell,
        )
        self.assertIn(
            "User:traffic-rule-manager --operation Read --group "
            "rule-manager-model-feedback-revision-v1",
            shell,
        )
        self.assertIn(
            "User:traffic-rule-manager --operation Read --topic model.feedback.v1",
            shell,
        )
        self.assertNotIn("--operation Write --topic model.feedback.v1", shell)
        self.assertIn(
            "User:traffic-alert-service --operation Write --topic playbook.execution.events.v2",
            shell,
        )
        self.assertIn(
            "User:traffic-alert-service --operation Write --topic alert.saved-view.events.v1",
            shell,
        )
        self.assertIn(
            "User:traffic-alert-service --operation Read --group alert-service-batch-assignment-execution-v1",
            shell,
        )
        self.assertIn(
            "User:traffic-alert-service --operation Write --topic alert.assignment.events.v1",
            shell,
        )
        self.assertIn(
            "User:traffic-alert-service --operation Write --topic notification.governance.events.v1",
            shell,
        )
        self.assertIn(
            "User:traffic-alert-service --operation Write --topic whitelist.events.v2",
            shell,
        )
        self.assertIn(
            "User:traffic-rule-manager --operation Read --group rule-manager-whitelist-rule-effect-v2",
            shell,
        )
        self.assertIn(
            "User:traffic-rule-manager --operation Read --topic whitelist.events.v2",
            shell,
        )
        self.assertIn(
            "User:traffic-auth-service --operation Write --topic user.events.v1",
            shell,
        )
        self.assertIn(
            "User:traffic-ingest-gateway --operation Write --topic asset.bindings.v1",
            shell,
        )
        self.assertIn(
            "User:traffic-device-log-collector --operation Write --topic device.logs.v1",
            shell,
        )
        self.assertIn(
            "User:traffic-device-log-collector --operation IdempotentWrite --cluster",
            shell,
        )
        self.assertIn(
            "User:traffic-alert-service --operation Read --group alert-service-playbook-execution-projection-v2",
            shell,
        )
        self.assertIn(
            "User:traffic-flink-user-behavior --operation IdempotentWrite --cluster",
            shell,
        )
        self.assertIn(
            "User:traffic-flink-user-behavior --operation Describe --topic dlq.v1",
            shell,
        )
        self.assertIn(
            "User:traffic-flink-user-behavior --operation Write --topic dlq.v1",
            shell,
        )
        self.assertIn(
            "User:traffic-alert-service --operation Write --topic baseline.lifecycle.v1",
            shell,
        )
        self.assertIn(
            "User:traffic-flink-user-behavior --operation Read --group flink-user-behavior-baseline-v1",
            shell,
        )
        self.assertIn(
            "User:traffic-flink-user-behavior --operation Write --topic baseline.activation-acks.v1",
            shell,
        )
        self.assertIn(
            "User:traffic-flink-pcap-index --operation IdempotentWrite --cluster",
            shell,
        )
        self.assertIn(
            "User:traffic-flink-pcap-index --operation Describe --topic dlq.v1",
            shell,
        )
        self.assertIn(
            "User:traffic-flink-pcap-index --operation Write --topic dlq.v1",
            shell,
        )
        self.assertIn(
            "apply_prefixed_acl --allow-principal User:traffic-flink-session --operation Read --group flink-session-job-shadow-",
            shell,
        )
        self.assertIn(
            "apply_prefixed_acl --allow-principal User:traffic-flink-feature --operation Read --group flink-feature-job-shadow-",
            shell,
        )
        self.assertIn(
            "apply_prefixed_acl --allow-principal User:traffic-flink-log --operation Read --group flink-log-job-shadow-",
            shell,
        )
        self.assertIn(
            "User:traffic-flink-log --operation IdempotentWrite --cluster",
            shell,
        )
        self.assertIn(
            "User:traffic-flink-log --operation Write --topic dlq.v1",
            shell,
        )
        audit_grants = [
            line for line in grant_lines if "User:traffic-audit-materializer " in line
        ]
        self.assertEqual(
            [
                "apply_literal_acl --allow-principal User:traffic-audit-materializer --operation IdempotentWrite --cluster",
                "apply_literal_acl --allow-principal User:traffic-audit-materializer --operation Describe --group audit-consumer",
                "apply_literal_acl --allow-principal User:traffic-audit-materializer --operation Read --group audit-consumer",
                "apply_literal_acl --allow-principal User:traffic-audit-materializer --operation Describe --topic audit.logs",
                "apply_literal_acl --allow-principal User:traffic-audit-materializer --operation Read --topic audit.logs",
                "apply_literal_acl --allow-principal User:traffic-audit-materializer --operation Describe --topic dlq.v1",
                "apply_literal_acl --allow-principal User:traffic-audit-materializer --operation Write --topic dlq.v1",
            ],
            audit_grants,
        )

    def test_generated_configmap_matches_contract(self) -> None:
        generated = ROOT / "deployments/kubernetes/init-jobs/00-kafka-acl-plan.yaml"
        self.assertEqual(render_configmap(self.acl), generated.read_text(encoding="utf-8"))

    def test_generated_service_identity_manifests_match_contract_and_shell_parses(self) -> None:
        external_secrets, bootstrap = split_service_identity_bundle(
            render_service_identities(self.acl)
        )
        identity_path = (
            ROOT
            / "deployments/kubernetes/security/generated-kafka-service-identities.v1.yaml"
        )
        bootstrap_path = (
            ROOT / "deployments/kubernetes/init-jobs/00-kafka-service-principals.yaml"
        )
        self.assertEqual(external_secrets, identity_path.read_text(encoding="utf-8"))
        self.assertEqual(bootstrap, bootstrap_path.read_text(encoding="utf-8"))
        identity_docs = list(yaml.safe_load_all(external_secrets))
        self.assertEqual(19, len(identity_docs))
        self.assertTrue(all(item["kind"] == "ExternalSecret" for item in identity_docs))
        bootstrap_docs = list(yaml.safe_load_all(bootstrap))
        self.assertEqual(["ConfigMap", "Job"], [item["kind"] for item in bootstrap_docs])
        script = bootstrap_docs[0]["data"]["provision.sh"]
        with tempfile.NamedTemporaryFile(mode="w", encoding="utf-8") as handle:
            handle.write(script)
            handle.flush()
            result = subprocess.run(
                ["bash", "-n", handle.name], capture_output=True, text=True, check=False
            )
        self.assertEqual(0, result.returncode, result.stderr)
        for item in self.acl["principals"]:
            if not isinstance(item.get("credential"), dict):
                continue
            username = item["principal"].removeprefix("User:")
            self.assertIn(
                f'{username}:{item["credential"]["password_env"]}', script
            )
        self.assertNotIn("traffic-kafka-replayer", script)
        self.assertIn(
            "traffic-device-log-collector:KAFKA_DEVICE_LOG_COLLECTOR_PASSWORD",
            script,
        )
        explicit = yaml.safe_load(
            (ROOT / "deployments/log-collectors/device-logs-secret-ref.yaml").read_text(
                encoding="utf-8"
            )
        )
        self.assertEqual("ExternalSecret", explicit["kind"])
        self.assertEqual(
            "kafka-device-log-collector-credentials", explicit["metadata"]["name"]
        )
        self.assertNotIn(
            "name: kafka-device-log-collector-credentials\n  namespace: traffic-analysis",
            external_secrets,
        )

    def test_audit_materializer_expand_manifest_binds_dedicated_identity(self) -> None:
        digest = "1" * 64
        rendered = render_audit_materializer(
            f"docker.io/traffic/audit-materializer@sha256:{digest}"
        )
        manifest = yaml.safe_load(rendered)
        self.assertEqual("audit-materializer", manifest["metadata"]["name"])
        container = manifest["spec"]["template"]["spec"]["containers"][0]
        self.assertEqual(
            f"docker.io/traffic/audit-materializer@sha256:{digest}",
            container["image"],
        )
        env = {item["name"]: item for item in container["env"]}
        for name, key in (
            ("KAFKA_SASL_USERNAME", "username"),
            ("KAFKA_SASL_PASSWORD", "password"),
        ):
            ref = env[name]["valueFrom"]["secretKeyRef"]
            self.assertEqual("kafka-audit-materializer-credentials", ref["name"])
            self.assertEqual(key, ref["key"])
        self.assertEqual("audit.logs", env["AUDIT_TOPIC"]["value"])
        self.assertEqual("dlq.v1", env["AUDIT_DLQ_TOPIC"]["value"])
        self.assertEqual("audit-consumer", env["AUDIT_CONSUMER_GROUP"]["value"])
        self.assertEqual("/readyz", container["readinessProbe"]["httpGet"]["path"])

    def test_audit_materializer_renderer_rejects_mutable_or_unresolved_image(self) -> None:
        for image in (
            "traffic/audit-materializer:latest",
            "traffic/audit-materializer@sha256:ABC",
            "${AUDIT_MATERIALIZER_IMAGE}",
        ):
            with self.subTest(image=image), self.assertRaises(ValueError):
                render_audit_materializer(image)

    def test_go_workloads_use_their_own_service_identity_secret(self) -> None:
        catalog_services = {
            item["credential"]["workload"]: item["credential"]["secret_name"]
            for item in self.acl["principals"]
            if item.get("kind") == "service"
        }
        manifests = list(
            yaml.safe_load_all(
                (ROOT / "deployments/kubernetes/applications/go-services.yaml").read_text(
                    encoding="utf-8"
                )
            )
        )
        deployments = {
            item["metadata"]["name"]: item
            for item in manifests
            if item and item.get("kind") == "Deployment"
        }
        self.assertEqual(set(catalog_services), set(deployments))
        for workload, secret_name in catalog_services.items():
            env = deployments[workload]["spec"]["template"]["spec"]["containers"][0]["env"]
            by_name = {item["name"]: item for item in env}
            self.assertEqual(
                secret_name,
                by_name["KAFKA_SASL_USERNAME"]["valueFrom"]["secretKeyRef"]["name"],
            )
            self.assertEqual(
                secret_name,
                by_name["KAFKA_SASL_PASSWORD"]["valueFrom"]["secretKeyRef"]["name"],
            )
            self.assertEqual(
                "username",
                by_name["KAFKA_SASL_USERNAME"]["valueFrom"]["secretKeyRef"]["key"],
            )
            self.assertEqual(
                "password",
                by_name["KAFKA_SASL_PASSWORD"]["valueFrom"]["secretKeyRef"]["key"],
            )
        source = (ROOT / "deployments/kubernetes/applications/go-services.yaml").read_text(
            encoding="utf-8"
        )
        self.assertNotIn("key: KAFKA_CLIENT_USERNAME", source)
        self.assertNotIn("key: KAFKA_CLIENT_PASSWORD", source)

    def test_duplicate_service_secret_and_password_property_are_rejected(self) -> None:
        candidate = copy.deepcopy(self.acl)
        first = candidate["principals"][0]["credential"]
        second = candidate["principals"][1]["credential"]
        second["secret_name"] = first["secret_name"]
        second["password_env"] = first["password_env"]
        second["remote_password_property"] = first["remote_password_property"]
        result = validate_documents(candidate, self.topics)
        self.assertEqual("blocked", result["result"])
        self.assertTrue(any("duplicate workload credential secret" in item for item in result["errors"]))
        self.assertTrue(any("duplicate workload password env" in item for item in result["errors"]))
        self.assertTrue(any("duplicate remote password property" in item for item in result["errors"]))

    def test_wildcard_principal_is_rejected(self) -> None:
        candidate = copy.deepcopy(self.acl)
        candidate["principals"][0]["principal"] = "User:*"
        result = validate_documents(candidate, self.topics)
        self.assertEqual("blocked", result["result"])
        self.assertTrue(any("unsafe Kafka principal" in item for item in result["errors"]))

    def test_consumer_without_explicit_group_is_rejected(self) -> None:
        candidate = copy.deepcopy(self.acl)
        candidate["topic_bindings"][0]["consumers"][0]["groups"] = []
        result = validate_documents(candidate, self.topics)
        self.assertEqual("blocked", result["result"])
        self.assertTrue(any("requires groups" in item for item in result["errors"]))

    def test_shadow_group_prefix_is_exactly_principal_scoped(self) -> None:
        candidate = copy.deepcopy(self.acl)
        candidate["topic_bindings"][0]["consumers"][0]["group_prefixes"] = ["flink-"]
        result = validate_documents(candidate, self.topics)
        self.assertEqual("blocked", result["result"])
        self.assertTrue(any("unauthorized shadow group prefix" in item for item in result["errors"]))
        candidate = copy.deepcopy(self.acl)
        candidate["policy"]["consumer_group_prefix_pattern_type"] = "literal"
        result = validate_documents(candidate, self.topics)
        self.assertTrue(any("must be prefixed" in item for item in result["errors"]))

    def test_baseline_cannot_enable_replayer(self) -> None:
        candidate = copy.deepcopy(self.acl)
        candidate["replay_policy"]["enabled"] = True
        result = validate_documents(candidate, self.topics)
        self.assertEqual("blocked", result["result"])
        self.assertIn("baseline replay principal must be disabled", result["errors"])

    def test_init_job_only_removes_legacy_wildcard_acl_at_cutover(self) -> None:
        source = (ROOT / "deployments/kubernetes/init-jobs/01-kafka-topics.yaml").read_text(
            encoding="utf-8"
        )
        self.assertIn("kafka-acl-plan-v1", source)
        self.assertIn('KAFKA_ACL_MIGRATION_PHASE', source)
        self.assertNotRegex(source, r"--add[^\n]*--operation All")
        self.assertEqual(3, source.count("--remove --force --allow-principal"))
        deploy = (ROOT / "deployments/kubernetes/deploy.sh").read_text(encoding="utf-8")
        self.assertIn("sync_kafka_service_identity_secrets", deploy)
        self.assertIn("job/init-kafka-principals", deploy)
        self.assertIn("refusing to continue with application ACL rollout", deploy)


if __name__ == "__main__":
    unittest.main()
