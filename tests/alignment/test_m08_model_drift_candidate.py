import re
import unittest
from pathlib import Path

import yaml


ROOT = Path(__file__).resolve().parents[2]


class M08ModelDriftCandidateTest(unittest.TestCase):
    def test_rule_manager_candidate_writer_is_explicitly_default_off(self) -> None:
        documents = list(
            yaml.safe_load_all(
                (ROOT / "deployments/kubernetes/applications/go-services.yaml").read_text()
            )
        )
        deployment = next(
            document
            for document in documents
            if isinstance(document, dict)
            and document.get("kind") == "Deployment"
            and document.get("metadata", {}).get("name") == "rule-manager"
        )
        container = deployment["spec"]["template"]["spec"]["containers"][0]
        env = {item["name"]: item.get("value") for item in container["env"]}
        self.assertEqual(env["MLOPS_AUTOMATIC_CANDIDATE_V1_ENABLED"], "false")
        self.assertEqual(env["MLOPS_MAX_PSI"], "0.25")
        self.assertEqual(env["MLOPS_MAX_FEATURE_PARTIAL_RATE"], "0.01")
        self.assertEqual(env["MLOPS_MIN_FEATURE_SAMPLES"], "1000")
        self.assertEqual(env["MLOPS_MIN_FEEDBACK_SAMPLES"], "100")

    def test_workflow_is_candidate_only_and_cannot_auto_activate(self) -> None:
        deployed = list(
            yaml.safe_load_all(
                (ROOT / "deployments/kubernetes/argo-events/mlops-training-template.yaml").read_text()
            )
        )
        workflow = next(document for document in deployed if document.get("kind") == "WorkflowTemplate")
        mirror = yaml.safe_load(
            (ROOT / "mlops/workflows/mlops-workflow-template.yaml").read_text()
        )
        self.assertEqual(workflow, mirror)
        parameters = {
            item["name"]: item.get("value")
            for item in workflow["spec"]["arguments"]["parameters"]
        }
        self.assertEqual(parameters["auto-activate"], "false")
        self.assertEqual(parameters["candidate-only"], "false")
        for required in (
            "baseline-model-version",
            "drift-evaluation-id",
            "drift-signal-sha256",
            "drift-policy-sha256",
        ):
            self.assertIn(required, parameters)

        register_source = (ROOT / "mlops/scripts/register_model.py").read_text()
        self.assertIn("cannot activate or notify runtimes", register_source)
        self.assertIn("activation_authorized': False", register_source)

    def test_database_enforces_candidate_and_append_only_guards(self) -> None:
        migration_path = ROOT / "deployments/postgres/migrations/202608151700_m08_model_drift_candidate_v1.sql"
        migration = migration_path.read_text()
        self.assertIn("CHECK (activation_authorized=false)", migration)
        self.assertIn("approval_state='NOT_REQUESTED'", migration)
        self.assertIn("decision_state<>'CANDIDATE'", migration)
        self.assertIn("candidate baseline is not the active model version", migration)
        self.assertIn("model drift evaluation receipts are append-only", migration)
        self.assertIn("UNIQUE (tenant_id,model_id,baseline_model_version)", migration)

        init_job = (ROOT / "deployments/kubernetes/init-jobs/02-postgres-schema.yaml").read_text()
        block = re.search(
            r"  # BEGIN GENERATED T1-M08 MODEL DRIFT CANDIDATE V1\n"
            r"  46-m08-model-drift-candidate-v1.sql: \|\n(?P<body>.*?)"
            r"\n  # END GENERATED T1-M08 MODEL DRIFT CANDIDATE V1",
            init_job,
            re.DOTALL,
        )
        self.assertIsNotNone(block)
        embedded = "\n".join(
            line[4:] if line.startswith("    ") else line
            for line in block.group("body").splitlines()
        )
        self.assertEqual(embedded.rstrip(), migration.rstrip())
        self.assertIn(
            "45-m08-model-feedback-revision-inbox-v1.sql "
            "46-m08-model-drift-candidate-v1.sql "
            "47-m08-model-rollback-v2.sql; do",
            init_job,
        )

    def test_go_policy_fails_closed_and_scheduler_submits_fixed_candidate(self) -> None:
        policy = (ROOT / "go/control-plane/internal/rules/modeldrift/policy.go").read_text()
        service = (ROOT / "go/control-plane/internal/rules/service/mlops_drift_candidate.go").read_text()
        orchestrator = (ROOT / "go/control-plane/internal/rules/service/mlops_orchestrator.go").read_text()
        for marker in (
            "feature_watermark_missing",
            "feedback_watermark_missing",
            "feature_quality_partial",
            "feedback_samples_insufficient",
            "ActivationAuthorized: false",
        ):
            self.assertIn(marker, policy)
        self.assertIn("model_feedback_revision_head", service)
        self.assertIn("adjudication_state = 'ADJUDICATED'", service)
        self.assertIn("ON CONFLICT DO NOTHING", service)
        self.assertIn("candidate-only=true", orchestrator)
        self.assertIn("auto-activate=false", orchestrator)
        self.assertIn("MLOps automatic candidate scheduler is disabled", orchestrator)

    def test_kubernetes_canary_is_isolated_from_shared_services(self) -> None:
        job = yaml.safe_load(
            (ROOT / "deployments/kubernetes/jobs/m08-model-drift-postgres-canary.yaml").read_text()
        )
        annotations = job["metadata"]["annotations"]
        self.assertEqual(annotations["traffic.analysis/shared-postgres-touched"], "false")
        self.assertEqual(annotations["traffic.analysis/shared-clickhouse-touched"], "false")
        self.assertEqual(annotations["traffic.analysis/shared-argo-touched"], "false")
        volumes = job["spec"]["template"]["spec"]["volumes"]
        self.assertTrue(any("emptyDir" in volume for volume in volumes))
        self.assertFalse(job["spec"]["template"]["spec"]["automountServiceAccountToken"])


if __name__ == "__main__":
    unittest.main()
