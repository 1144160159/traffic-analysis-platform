import copy
import hashlib
import json
import unittest
from pathlib import Path

import yaml


ROOT = Path(__file__).resolve().parents[2]
POLICY = ROOT / "deployments/releases/topic1/m08-tenant-model-canary.v1.json"
SCHEMA = ROOT / "contracts/mlops/tenant-model-canary-policy.schema.json"
MANIFEST = ROOT / "deployments/kubernetes/jobs/m08-model-tenant-canary.yaml"
ACL = ROOT / "contracts/events/kafka-acl-catalog.v1.json"
TOPICS = ROOT / "contracts/events/kafka-topic-catalog.v1.json"
CONTROLLER = ROOT / "go/control-plane/cmd/model-canary-controller/main.go"


def sha256(payload: bytes) -> str:
    return hashlib.sha256(payload).hexdigest()


def default_off_errors(policy: dict, job: dict) -> list[str]:
    errors: list[str] = []
    container = job["spec"]["template"]["spec"]["containers"][0]
    env = {item["name"]: item.get("value") for item in container["env"]}
    if job["spec"].get("suspend") is not True:
        errors.append("job_not_suspended")
    if policy.get("enabled") is not False:
        errors.append("policy_enabled")
    if env.get("MODEL_CANARY_EXECUTION_AUTHORIZED") != "false":
        errors.append("execution_authorized")
    if not container["image"].endswith("@sha256:" + "0" * 64):
        errors.append("sentinel_image_replaced")
    return errors


class M08TenantModelCanaryTest(unittest.TestCase):
    def setUp(self) -> None:
        self.policy_bytes = POLICY.read_bytes()
        self.policy = json.loads(self.policy_bytes)
        self.schema = json.loads(SCHEMA.read_text(encoding="utf-8"))
        self.documents = list(yaml.safe_load_all(MANIFEST.read_text(encoding="utf-8")))

    def test_policy_is_schema_shaped_and_cross_field_bounded(self) -> None:
        self.assertEqual(1, self.policy["schema_version"])
        self.assertEqual(
            set(self.schema["required"]),
            set(self.policy),
            "top-level policy must remain an exact additionalProperties=false object",
        )
        self.assertFalse(self.policy["enabled"])
        self.assertLessEqual(self.policy["rollout_percentage"], 10)
        self.assertGreaterEqual(self.policy["minimum_samples"], 100)
        self.assertGreaterEqual(
            self.policy["maximum_samples"], self.policy["minimum_samples"]
        )
        self.assertGreaterEqual(self.policy["observation_window_seconds"], 300)
        self.assertNotEqual(
            self.policy["deployment_id"], self.policy["rollback_deployment_id"]
        )
        self.assertGreaterEqual(
            self.policy["shadow_evidence"]["minimum_samples"],
            self.policy["minimum_samples"],
        )
        self.assertGreaterEqual(
            self.policy["shadow_evidence"]["minimum_window_seconds"],
            self.policy["observation_window_seconds"],
        )

    def test_kubernetes_submission_is_exact_default_off_policy(self) -> None:
        self.assertEqual(2, len(self.documents))
        configmap, job = self.documents
        self.assertEqual("ConfigMap", configmap["kind"])
        self.assertEqual("Job", job["kind"])
        embedded = configmap["data"]["policy.json"].encode()
        self.assertEqual(self.policy_bytes, embedded)
        policy_hash = sha256(self.policy_bytes)
        self.assertEqual(
            policy_hash,
            configmap["metadata"]["annotations"]["traffic.analysis/policy-sha256"],
        )
        self.assertTrue(job["spec"]["suspend"])
        container = job["spec"]["template"]["spec"]["containers"][0]
        env = {item["name"]: item.get("value") for item in container["env"]}
        self.assertEqual(policy_hash, env["MODEL_CANARY_POLICY_SHA256"])
        self.assertEqual("false", env["MODEL_CANARY_EXECUTION_AUTHORIZED"])
        self.assertTrue(container["image"].endswith("@sha256:" + "0" * 64))
        self.assertFalse(job["spec"]["template"]["spec"]["automountServiceAccountToken"])
        api_endpoint = next(
            item
            for item in container["env"]
            if item["name"] == "MODEL_CANARY_API_BASE_URL"
        )
        self.assertNotIn("value", api_endpoint)
        self.assertEqual(
            {
                "name": "model-canary-control-plane-credentials",
                "key": "base_url",
            },
            api_endpoint["valueFrom"]["secretKeyRef"],
        )
        self.assertEqual([], default_off_errors(self.policy, job))

    def test_current_n012_component_receipt_cannot_authorize_n013(self) -> None:
        evidence = ROOT / self.policy["shadow_evidence"]["path"]
        self.assertEqual(self.policy["shadow_evidence"]["sha256"], sha256(evidence.read_bytes()))
        receipt = json.loads(evidence.read_text(encoding="utf-8"))
        self.assertEqual("PASS", receipt["scoped_evidence_status"])
        self.assertNotIn(
            "shadow_observation_window",
            receipt,
            "the isolated component canary must not be upgraded into a full shadow window",
        )

    def test_acl_and_topic_catalog_bind_exact_controller_group(self) -> None:
        acl = json.loads(ACL.read_text(encoding="utf-8"))
        binding = next(
            item
            for item in acl["topic_bindings"]
            if item["topic"] == "model-shadow-observations.v1"
        )
        self.assertIn(
            {
                "principal": "rule-manager",
                "groups": ["rule-manager-model-tenant-canary-v1"],
            },
            binding["consumers"],
        )
        topics = json.loads(TOPICS.read_text(encoding="utf-8"))
        topic = next(
            item for item in topics["topics"] if item["name"] == binding["topic"]
        )
        self.assertEqual("consumer_candidate_default_off", topic["readiness"])
        self.assertEqual(
            ["go/control-plane/cmd/model-canary-controller/main.go"],
            topic["consumers"],
        )

    def test_controller_has_rollback_but_no_automatic_activation_call(self) -> None:
        source = CONTROLLER.read_text(encoding="utf-8")
        self.assertIn('policy.DeploymentID+"/rollback"', source)
        self.assertNotIn('policy.DeploymentID+"/activate"', source)
        self.assertIn("Deliberately no POST /activate", source)
        self.assertIn("observation.ObservedAtMS < canaryStartedAt.UnixMilli()", source)
        self.assertIn("candidate deployment percentage does not match", source)

    def test_mutated_manifest_enablement_is_detectable(self) -> None:
        _, job = copy.deepcopy(self.documents)
        job["spec"]["suspend"] = False
        container = job["spec"]["template"]["spec"]["containers"][0]
        env = {item["name"]: item for item in container["env"]}
        env["MODEL_CANARY_EXECUTION_AUTHORIZED"]["value"] = "true"
        self.assertEqual(
            ["job_not_suspended", "execution_authorized"],
            default_off_errors(self.policy, job),
        )


if __name__ == "__main__":
    unittest.main()
