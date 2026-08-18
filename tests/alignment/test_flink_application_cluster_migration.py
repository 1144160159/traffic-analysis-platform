import copy
import json
import sys
import unittest
from pathlib import Path

import yaml


ROOT = Path(__file__).resolve().parents[2]
sys.path.insert(0, str(ROOT / "scripts/alignment"))

from render_flink_application_cluster import render, validate_contract  # noqa: E402
from verify_flink_nine_jobs import EXPECTED_TASKS  # noqa: E402


IMAGE = "docker.io/traffic/flink-session@sha256:" + "1" * 64
CANDIDATE_SHA256 = "4" * 64


class FlinkApplicationClusterMigrationTest(unittest.TestCase):
    def setUp(self) -> None:
        self.contract = json.loads(
            (ROOT / "contracts/flink/application-cluster-migration.v1.json").read_text(
                encoding="utf-8"
            )
        )
        self.acl = json.loads(
            (ROOT / "contracts/events/kafka-acl-catalog.v1.json").read_text(
                encoding="utf-8"
            )
        )
        self.job_id = "flink-session-job"
        self.savepoints = {
            "schema_version": 1,
            "source_cluster_id": "flink-traffic",
            "savepoints": {
                self.job_id: {
                    "uri": "s3://flink-checkpoints/savepoints/application-clusters/flink-session-job/savepoint-012345",
                    "sha256": "2" * 64,
                    "source_job_id": "3" * 32,
                }
            },
        }

    def test_contract_covers_canonical_nine_jobs_and_128_tasks(self) -> None:
        result = validate_contract(self.contract, self.acl)
        self.assertEqual("pass", result["result"], result)
        self.assertEqual(9, result["jobs"])
        self.assertEqual(128, result["expected_tasks"])
        self.assertLessEqual(result["max_serial_cpu_request"], 8)
        self.assertLessEqual(result["max_serial_memory_request_gib"], 22)
        by_name = {job["job_name"]: job["expected_tasks"] for job in self.contract["jobs"]}
        self.assertEqual(EXPECTED_TASKS, by_name)
        self.assertEqual(list(range(1, 10)), sorted(job["migration_order"] for job in self.contract["jobs"]))

    def test_each_flink_job_has_unique_expanded_identity_in_flink_namespace(self) -> None:
        principal_by_id = {item["id"]: item for item in self.acl["principals"]}
        secrets = set()
        for job in self.contract["jobs"]:
            principal = principal_by_id[job["principal_id"]]
            self.assertEqual("flink_job", principal["kind"])
            self.assertEqual("expand", principal["rollout_state"])
            credential = principal["credential"]
            self.assertEqual("flink", credential["namespace"])
            self.assertEqual(job["id"], credential["workload"])
            self.assertNotIn(credential["secret_name"], secrets)
            secrets.add(credential["secret_name"])
        self.assertEqual(9, len(secrets))

    def test_render_is_single_job_application_mode_with_restore_and_rollback(self) -> None:
        docs = list(yaml.safe_load_all(render(self.job_id, IMAGE, self.savepoints)))
        self.assertEqual(
            ["ServiceAccount", "Role", "RoleBinding", "ConfigMap", "ConfigMap", "Job", "Job"],
            [doc["kind"] for doc in docs],
        )
        launcher = next(doc for doc in docs if doc["metadata"]["name"] == "migrate-flink-session-job-v1")
        pod_spec = launcher["spec"]["template"]["spec"]
        self.assertEqual("flink-app-session-runtime", pod_spec["serviceAccountName"])
        container = pod_spec["containers"][0]
        self.assertEqual(IMAGE, container["image"])
        command = container["command"]
        self.assertIn("run-application", command)
        self.assertIn("kubernetes-application", command)
        self.assertIn("-s", command)
        self.assertIn(self.savepoints["savepoints"][self.job_id]["uri"], command)
        self.assertIn("local:///opt/flink/usrlib/flink-session-job.jar", command)
        self.assertIn("-Dkubernetes.jobmanager.replicas=2", command)
        self.assertIn(
            "-Djob-result-store.storage-path=s3://flink-checkpoints/job-result-store/application-clusters/flink-app-session",
            command,
        )
        self.assertIn("-Djob-result-store.delete-on-commit=false", command)
        self.assertIn("-Dpipeline.max-parallelism=128", command)
        self.assertFalse(any("allowNonRestoredState" in arg for arg in command))
        rollback = next(doc for doc in docs if doc["metadata"]["name"] == "flink-app-session-rollback-v1")
        self.assertTrue(rollback["immutable"])
        self.assertEqual("false", rollback["data"]["allow-non-restored-state"])
        self.assertEqual("flink-traffic", rollback["data"]["source-cluster-id"])
        rollback_job = next(doc for doc in docs if doc["metadata"]["name"] == "rollback-flink-session-job-v1")
        self.assertTrue(rollback_job["spec"]["suspend"])
        rollback_command = rollback_job["spec"]["template"]["spec"]["containers"][0]["command"]
        self.assertIn("kubernetes-session", rollback_command)
        self.assertIn(self.savepoints["savepoints"][self.job_id]["uri"], rollback_command)
        self.assertIn("-Dpipeline.max-parallelism=128", rollback_command)
        self.assertFalse(any("allowNonRestoredState" in arg for arg in rollback_command))

    def test_pod_template_exposes_only_selected_kafka_identity(self) -> None:
        docs = list(yaml.safe_load_all(render(self.job_id, IMAGE, self.savepoints)))
        template = yaml.safe_load(docs[3]["data"]["pod-template.yaml"])
        container = template["spec"]["containers"][0]
        self.assertEqual("flink-main-container", container["name"])
        env = {item["name"]: item for item in container["env"]}
        selected = "kafka-flink-session-job-credentials"
        self.assertEqual(selected, env["KAFKA_SASL_USERNAME"]["valueFrom"]["secretKeyRef"]["name"])
        self.assertEqual(selected, env["KAFKA_SASL_PASSWORD"]["valueFrom"]["secretKeyRef"]["name"])
        serialized = yaml.safe_dump(template)
        self.assertNotIn("KAFKA_CLIENT_USERNAME", serialized)
        self.assertNotIn("KAFKA_CLIENT_PASSWORD", serialized)
        for job in self.contract["jobs"]:
            other = f"kafka-{job['id']}-credentials"
            if other != selected:
                self.assertNotIn(other, serialized)

    def test_m03_shadow_uses_candidate_group_and_disables_external_writes(self) -> None:
        docs = list(yaml.safe_load_all(render(
            self.job_id,
            IMAGE,
            self.savepoints,
            activation_mode="shadow",
            candidate_sha256=CANDIDATE_SHA256,
        )))
        launcher = next(doc for doc in docs if doc["metadata"]["name"] == "migrate-flink-session-job-shadow-" + CANDIDATE_SHA256[:12])
        self.assertEqual(
            "shadow", launcher["metadata"]["annotations"]["traffic.openai.com/deployment-activation"]
        )
        self.assertEqual(
            CANDIDATE_SHA256,
            launcher["metadata"]["annotations"]["traffic.openai.com/candidate-sha256"],
        )
        command = launcher["spec"]["template"]["spec"]["containers"][0]["command"]
        self.assertEqual("shadow", command[command.index("--deployment.activation.mode") + 1])
        self.assertEqual(
            CANDIDATE_SHA256,
            command[command.index("--deployment.candidate.sha256") + 1],
        )
        self.assertEqual(
            "flink-session-job-shadow-" + CANDIDATE_SHA256[:12],
            command[command.index("--consumer.group") + 1],
        )
        self.assertIn(
            "-Dkubernetes.cluster-id=flink-app-session-shadow-" + CANDIDATE_SHA256[:12],
            command,
        )
        self.assertIn(
            "-Dstate.checkpoints.dir=s3://flink-checkpoints/checkpoints/application-clusters/flink-session-job/shadow-" + CANDIDATE_SHA256[:12],
            command,
        )

    def test_m03_production_uses_canonical_consumer_group(self) -> None:
        docs = list(yaml.safe_load_all(render(
            self.job_id,
            IMAGE,
            self.savepoints,
            activation_mode="production",
            candidate_sha256=CANDIDATE_SHA256,
        )))
        command = docs[-2]["spec"]["template"]["spec"]["containers"][0]["command"]
        self.assertEqual("flink-session-job", command[command.index("--consumer.group") + 1])

    def test_m03_rollback_job_uses_pinned_previous_image_and_unique_revision(self) -> None:
        previous = "docker.io/traffic/flink-session@sha256:" + "9" * 64
        docs = list(yaml.safe_load_all(render(
            self.job_id,
            IMAGE,
            self.savepoints,
            activation_mode="production",
            candidate_sha256=CANDIDATE_SHA256,
            rollback_image=previous,
        )))
        rollback = next(doc for doc in docs if doc["metadata"]["name"] == "rollback-flink-session-job-production-" + CANDIDATE_SHA256[:12])
        self.assertEqual(
            previous,
            rollback["spec"]["template"]["spec"]["containers"][0]["image"],
        )

    def test_activation_rejects_missing_digest_legacy_digest_and_non_m03_shadow(self) -> None:
        with self.assertRaisesRegex(ValueError, "require a lowercase candidate"):
            render(self.job_id, IMAGE, self.savepoints, activation_mode="shadow")
        with self.assertRaisesRegex(ValueError, "must not carry"):
            render(
                self.job_id,
                IMAGE,
                self.savepoints,
                activation_mode="legacy",
                candidate_sha256=CANDIDATE_SHA256,
            )
        non_m03 = "flink-rule-job"
        savepoints = copy.deepcopy(self.savepoints)
        savepoints["savepoints"][non_m03] = savepoints["savepoints"].pop(self.job_id)
        with self.assertRaisesRegex(ValueError, "limited to the M03"):
            render(
                non_m03,
                IMAGE,
                savepoints,
                activation_mode="shadow",
                candidate_sha256=CANDIDATE_SHA256,
            )

    def test_renderer_rejects_mutable_image_and_untrusted_savepoint(self) -> None:
        for image in ("traffic/flink:latest", "${FLINK_IMAGE}", "traffic/flink@sha256:ABC"):
            with self.subTest(image=image), self.assertRaises(ValueError):
                render(self.job_id, image, self.savepoints)
        bad = copy.deepcopy(self.savepoints)
        bad["savepoints"][self.job_id]["uri"] = "file:///tmp/savepoint"
        with self.assertRaises(ValueError):
            render(self.job_id, IMAGE, bad)
        bad = copy.deepcopy(self.savepoints)
        bad["savepoints"][self.job_id]["sha256"] = "unknown"
        with self.assertRaises(ValueError):
            render(self.job_id, IMAGE, bad)

    def test_contract_rejects_parallel_cutover_or_resource_budget_violation(self) -> None:
        candidate = copy.deepcopy(self.contract)
        candidate["migration"]["max_parallel_applications_during_migration"] = 2
        candidate["online_expand_budget"]["max_additional_cpu_requests"] = 1
        result = validate_contract(candidate, self.acl)
        self.assertEqual("blocked", result["result"])
        self.assertTrue(any("exactly one" in error for error in result["errors"]))
        self.assertTrue(any("CPU expand budget" in error for error in result["errors"]))

    def test_contract_rejects_max_parallelism_below_current_parallelism(self) -> None:
        candidate = copy.deepcopy(self.contract)
        candidate["jobs"][2]["max_parallelism"] = 8
        result = validate_contract(candidate, self.acl)
        self.assertEqual("blocked", result["result"])
        self.assertTrue(any("max_parallelism" in error for error in result["errors"]))

    def test_application_dockerfile_builds_exactly_one_job_jar(self) -> None:
        source = (ROOT / "java/flink-jobs/deployments/Dockerfile.application").read_text(
            encoding="utf-8"
        )
        self.assertIn("ARG JOB_MODULE", source)
        self.assertEqual(1, source.count("COPY "))
        self.assertIn("/opt/flink/usrlib/${JOB_MODULE}.jar", source)
        self.assertNotIn(":latest", source)


if __name__ == "__main__":
    unittest.main()
