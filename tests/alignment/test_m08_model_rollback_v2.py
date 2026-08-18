import json
import re
import unittest
from pathlib import Path

import yaml


ROOT = Path(__file__).resolve().parents[2]


class M08ModelRollbackV2Test(unittest.TestCase):
    def test_writer_is_default_off_and_timeout_is_explicit(self) -> None:
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
        env = {
            item["name"]: item.get("value")
            for item in deployment["spec"]["template"]["spec"]["containers"][0]["env"]
        }
        self.assertEqual(env["MODEL_ROLLBACK_V2_ENABLED"], "false")
        self.assertEqual(env["MODEL_ROLLBACK_ACK_TIMEOUT"], "2m")

    def test_database_forbids_false_recovery_and_identity_rewrite(self) -> None:
        migration_path = (
            ROOT
            / "deployments/postgres/migrations/202608151900_m08_model_rollback_v2.sql"
        )
        migration = migration_path.read_text()
        for marker in (
            "active_switched = (state = 'RECOVERED')",
            "state <> 'RECOVERED' OR applied_subtasks = expected_parallelism",
            "state <> 'FAILED_RESTORED' OR compensation_applied_subtasks = expected_parallelism",
            "model rollback immutable identity changed",
            "model rollback receipts are append-only",
            "uq_model_rollback_inflight",
        ):
            self.assertIn(marker, migration)

        init_job = (
            ROOT / "deployments/kubernetes/init-jobs/02-postgres-schema.yaml"
        ).read_text()
        block = re.search(
            r"  # BEGIN GENERATED T1-M08 MODEL ROLLBACK V2\n"
            r"  47-m08-model-rollback-v2.sql: \|\n(?P<body>.*?)"
            r"\n  # END GENERATED T1-M08 MODEL ROLLBACK V2",
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
            "46-m08-model-drift-candidate-v1.sql 47-m08-model-rollback-v2.sql; do",
            init_job,
        )

    def test_go_control_plane_switches_only_at_exact_ack_quorum(self) -> None:
        source = (
            ROOT
            / "go/control-plane/internal/rules/service/model_rollback_v2.go"
        ).read_text()
        ack_source = (
            ROOT / "go/control-plane/internal/rules/service/model_service.go"
        ).read_text()
        for marker in (
            "prepareModelRollbackV2",
            "previous immutable version",
            "requireExactConsumerReadyReceiptTx",
            "advanceModelRollbackFromAckTx",
            "commitRecoveredModelRollbackTx",
            "startModelRollbackCompensationTx",
            "finishModelRollbackCompensationTx",
            "ROLLBACK_COMPENSATION_ACK_TIMEOUT",
            "active_switched=true",
        ):
            self.assertIn(marker, source)
        self.assertIn("allExpectedSubtasksApplied", ack_source)
        self.assertIn("has no durable broker publication receipt", ack_source)
        prepare = source[source.index("func (s *ModelService) prepareModelRollbackV2") :]
        prepare = prepare[: prepare.index("func governedModelArtifactSHA256")]
        self.assertNotIn("UPDATE model_versions", prepare)
        self.assertNotIn("ModelStatusActive", prepare)

    def test_flink_savepoint_replay_reemits_identity_bound_ack(self) -> None:
        handler = (
            ROOT
            / "java/flink-jobs/flink-behavior-job/src/main/java/com/traffic/flink/behavior/detector/ModelUpdateBroadcastHandler.java"
        ).read_text()
        ack = (
            ROOT
            / "java/flink-jobs/flink-behavior-job/src/main/java/com/traffic/flink/behavior/model/ModelUpdateAppliedAck.java"
        ).read_text()
        test = (
            ROOT
            / "java/flink-jobs/flink-behavior-job/src/test/java/com/traffic/flink/behavior/detector/ModelUpdateBroadcastHandlerTest.java"
        ).read_text()
        for marker in (
            "rollback-compensate",
            "isExactActivationReplay",
            "rollbackConsumerIdentityMatches",
            "processed activation identity differs from restored state",
        ):
            self.assertIn(marker, handler)
        self.assertIn("withConsumerIdentity", ack)
        self.assertIn("savepointRestoreReemitsExactRollbackAcknowledgement", test)
        self.assertIn("initializeState(savepoint)", test)
        self.assertIn("rollbackForAnotherConsumerIdentityFailsClosedBeforeRuntimeSwap", test)

    def test_http_and_openapi_expose_strict_command_receipt_and_final_pointer(self) -> None:
        handler = (
            ROOT / "go/control-plane/internal/rules/api/handler_models.go"
        ).read_text()
        self.assertIn("decodeJSONStrict(r, &request)", handler)
        self.assertIn("type ModelRollbackRequest struct", handler)

        contract = json.loads(
            (ROOT / "contracts/openapi/alignment-v1.openapi.json").read_text()
        )
        paths = contract["paths"]
        command = paths["/v1/models/{id}/versions/{version}/rollback"]["post"]
        self.assertEqual(command["x-rollout-flag"], "MODEL_ROLLBACK_V2_ENABLED")
        request_schema = contract["components"]["schemas"]["ModelRollbackRequest"]
        self.assertFalse(request_schema["additionalProperties"])
        self.assertEqual(
            set(request_schema["required"]),
            {"reason", "expected_active_version", "expected_active_revision"},
        )
        self.assertIn("/v1/models/{id}/rollbacks/{job_id}", paths)
        self.assertIn("/v1/models/{id}/versions/active", paths)

    def test_kubernetes_canary_is_isolated_from_shared_services(self) -> None:
        job = yaml.safe_load(
            (
                ROOT
                / "deployments/kubernetes/jobs/m08-model-rollback-postgres-canary.yaml"
            ).read_text()
        )
        annotations = job["metadata"]["annotations"]
        self.assertEqual(annotations["traffic.analysis/shared-postgres-touched"], "false")
        self.assertEqual(annotations["traffic.analysis/shared-kafka-touched"], "false")
        self.assertEqual(annotations["traffic.analysis/shared-flink-touched"], "false")
        pod_spec = job["spec"]["template"]["spec"]
        self.assertFalse(pod_spec["automountServiceAccountToken"])
        self.assertTrue(any("emptyDir" in volume for volume in pod_spec["volumes"]))


if __name__ == "__main__":
    unittest.main()
