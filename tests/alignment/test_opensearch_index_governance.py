import json
import shutil
import tempfile
import unittest
from datetime import datetime, timedelta, timezone
from pathlib import Path

import yaml


ROOT = Path(__file__).resolve().parents[2]

import sys
sys.path.insert(0, str(ROOT / "scripts/alignment"))

from verify_opensearch_index_governance import verify  # noqa: E402
from render_opensearch_alerts_v2_expand import render_documents as render_expand_documents, validate_window  # noqa: E402
from plan_opensearch_alerts_v2_backfill import build_plan, validate_scope  # noqa: E402
from render_opensearch_alerts_v2_backfill import (  # noqa: E402
    render_documents as render_backfill_documents,
    validate_plan as validate_backfill_plan,
)


FILES = (
    "contracts/opensearch/index-governance.v1.json",
    "common/opensearch/index-templates.json",
    "common/opensearch/alerts-v2/mappings-component.json",
    "common/opensearch/alerts-v2/settings-component.json",
    "common/opensearch/alerts-v2/index-template.json",
    "common/opensearch/alerts-v2/ism-policy.json",
    "common/opensearch/alerts-v2/bootstrap-index.json",
    "deployments/kubernetes/init-jobs/04-opensearch-templates.yaml",
    "deployments/kubernetes/migrations/opensearch/T-OS-002-alerts-v2-expand.yaml",
    "scripts/alignment/render_opensearch_alerts_v2_expand.py",
    "scripts/alignment/plan_opensearch_alerts_v2_backfill.py",
    "scripts/alignment/render_opensearch_alerts_v2_backfill.py",
    "deployments/kubernetes/applications/go-services.yaml",
    "go/control-plane/internal/alert/config/config.go",
    "go/control-plane/internal/alert/persistence/opensearch.go",
    "go/control-plane/internal/alert/persistence/alert.go",
    "go/control-plane/internal/alert/repository/opensearch.go",
    "java/flink-jobs/flink-alert-generator-job/src/main/java/com/traffic/flink/alert/AlertGeneratorJob.java",
    "java/flink-jobs/flink-alert-generator-job/src/main/java/com/traffic/flink/alert/sink/OpenSearchAlertSinkFactory.java",
    "java/flink-jobs/flink-alert-generator-job/src/main/resources/alert-generator-job.properties",
)


def copy_candidate(parent: Path) -> Path:
    candidate = parent / "repo"
    for relative in FILES:
        target = candidate / relative
        target.parent.mkdir(parents=True, exist_ok=True)
        shutil.copy2(ROOT / relative, target)
    return candidate


class OpenSearchIndexGovernanceTest(unittest.TestCase):
    def test_repository_candidate_passes_without_live_apply_claim(self) -> None:
        result = verify(ROOT)
        self.assertEqual("PASS", result["status"], result)
        self.assertEqual("PARTIAL", result["coverage_status"])
        self.assertFalse(result["production_applied"])
        self.assertFalse(result["v2_traffic_default_enabled"])
        self.assertGreaterEqual(result["mapping_fields"], result["producer_fields"])

    def test_false_closure_and_production_apply_are_rejected(self) -> None:
        with tempfile.TemporaryDirectory() as directory:
            candidate = copy_candidate(Path(directory))
            path = candidate / "contracts/opensearch/index-governance.v1.json"
            contract = json.loads(path.read_text())
            contract["status"] = "closed"
            contract["production_applied"] = True
            path.write_text(json.dumps(contract))
            result = verify(candidate)
            self.assertEqual("FAIL", result["status"])
            self.assertTrue(any("must not claim closure" in error for error in result["errors"]))

    def test_dynamic_mapping_drift_is_rejected(self) -> None:
        with tempfile.TemporaryDirectory() as directory:
            candidate = copy_candidate(Path(directory))
            path = candidate / "common/opensearch/alerts-v2/mappings-component.json"
            payload = json.loads(path.read_text())
            payload["template"]["mappings"]["dynamic"] = True
            path.write_text(json.dumps(payload))
            result = verify(candidate)
            self.assertEqual("FAIL", result["status"])
            self.assertTrue(any("dynamic=strict" in error for error in result["errors"]))

    def test_runtime_template_mutation_is_rejected(self) -> None:
        with tempfile.TemporaryDirectory() as directory:
            candidate = copy_candidate(Path(directory))
            path = candidate / "go/control-plane/internal/alert/persistence/opensearch.go"
            path.write_text(path.read_text() + "\n// IndicesPutIndexTemplateRequest\n")
            result = verify(candidate)
            self.assertEqual("FAIL", result["status"])
            self.assertTrue(any("schema mutation" in error for error in result["errors"]))

    def test_default_v2_traffic_enable_is_rejected(self) -> None:
        with tempfile.TemporaryDirectory() as directory:
            candidate = copy_candidate(Path(directory))
            path = candidate / "java/flink-jobs/flink-alert-generator-job/src/main/resources/alert-generator-job.properties"
            path.write_text(path.read_text().replace(
                "opensearch.alerts.v2.enabled=false", "opensearch.alerts.v2.enabled=true"))
            result = verify(candidate)
            self.assertEqual("FAIL", result["status"])
            self.assertTrue(any("keep alerts v2 traffic disabled" in error for error in result["errors"]))

    def test_routine_init_path_cannot_create_alerts_v2(self) -> None:
        with tempfile.TemporaryDirectory() as directory:
            candidate = copy_candidate(Path(directory))
            path = candidate / "deployments/kubernetes/init-jobs/04-opensearch-templates.yaml"
            path.write_text(path.read_text() + "\n# alerts-v2-000001\n")
            result = verify(candidate)
            self.assertEqual("FAIL", result["status"])
            self.assertTrue(any("routine init path" in error for error in result["errors"]))

    def test_unsuspended_expand_job_is_rejected(self) -> None:
        with tempfile.TemporaryDirectory() as directory:
            candidate = copy_candidate(Path(directory))
            path = candidate / "deployments/kubernetes/migrations/opensearch/T-OS-002-alerts-v2-expand.yaml"
            path.write_text(path.read_text().replace("suspend: true", "suspend: false", 1))
            result = verify(candidate)
            self.assertEqual("FAIL", result["status"])
            self.assertTrue(any("suspended" in error for error in result["errors"]))

    def test_expand_job_without_cluster_binding_is_rejected(self) -> None:
        with tempfile.TemporaryDirectory() as directory:
            candidate = copy_candidate(Path(directory))
            path = candidate / "deployments/kubernetes/migrations/opensearch/T-OS-002-alerts-v2-expand.yaml"
            path.write_text(path.read_text().replace("APPROVED_CLUSTER_UUID", "REMOVED_CLUSTER_UUID"))
            result = verify(candidate)
            self.assertEqual("FAIL", result["status"])
            self.assertTrue(any("APPROVED_CLUSTER_UUID" in error for error in result["errors"]))

    def test_expand_renderer_creates_one_time_immutable_suspended_run(self) -> None:
        path = ROOT / "deployments/kubernetes/migrations/opensearch/T-OS-002-alerts-v2-expand.yaml"
        documents = [item for item in yaml.safe_load_all(path.read_text()) if item]
        rendered = render_expand_documents(
            documents,
            run_id="change-20260805-001",
            approval_id="CHG-20260805-001",
            approved_by="sre,qa,security",
            cluster_uuid="cluster-uuid-1",
            not_before_epoch=1_786_000_000,
            expires_at_epoch=1_786_003_600,
            g0_candidate_sha256="a" * 64,
            g0_manifest_sha256="b" * 64,
            contract_sha256="c" * 64,
        )
        secret = next(item for item in rendered if item["kind"] == "Secret")
        job = next(item for item in rendered if item["kind"] == "Job")
        self.assertTrue(secret["immutable"])
        self.assertEqual("change-20260805-001", secret["stringData"]["approval_nonce"])
        self.assertTrue(job["spec"]["suspend"])
        env = job["spec"]["template"]["spec"]["containers"][0]["env"]
        expected = next(item for item in env if item["name"] == "EXPECTED_APPROVAL_NONCE")
        self.assertEqual("change-20260805-001", expected["value"])
        referenced = {
            item["valueFrom"]["secretKeyRef"]["name"]
            for item in env if "valueFrom" in item and item["valueFrom"].get("secretKeyRef", {}).get("key") == "approval_nonce"
        }
        self.assertEqual({secret["metadata"]["name"]}, referenced)

    def test_expand_approval_window_rejects_expired_and_oversized(self) -> None:
        now = datetime.now(timezone.utc)
        with self.assertRaisesRegex(ValueError, "already expired"):
            validate_window(now - timedelta(hours=2), now - timedelta(hours=1), now)
        with self.assertRaisesRegex(ValueError, "exceeds"):
            validate_window(now, now + timedelta(hours=5), now)

    def test_backfill_plan_is_ready_only_for_one_bounded_write_alias(self) -> None:
        contract = json.loads((ROOT / "contracts/opensearch/index-governance.v1.json").read_text())
        fields = ["tenant_id", "alert_id", "ingest_ts"]
        plan = build_plan(
            observation={
                "cluster_uuid": "cluster-uuid-1",
                "cluster_status": "green",
                "source_mapping_fields": fields,
                "target_contract_fields": fields,
                "target_alias_indices": [{"index": "alerts-v2-000001", "is_write_index": True}],
                "minimum_node_free_bytes": 200_000_000_000,
                "source_count": 14,
            },
            tenant_id="default",
            start_time="2026-08-04T18:47:59Z",
            end_time="2026-08-04T18:48:00Z",
            time_field="ingest_ts",
            max_documents=100,
            slices=1,
            requests_per_second=10,
            min_free_bytes_required=contract["backfill_execution"]["minimum_free_bytes"],
            contract=contract,
        )
        self.assertEqual("READY", plan["execution_readiness"])
        self.assertEqual("create", plan["execution"]["body"]["dest"]["op_type"])
        self.assertEqual([], plan["production_mutations"])

    def test_backfill_plan_blocks_missing_target_and_oversized_scope(self) -> None:
        contract = json.loads((ROOT / "contracts/opensearch/index-governance.v1.json").read_text())
        plan = build_plan(
            observation={
                "cluster_uuid": "cluster-uuid-1",
                "cluster_status": "green",
                "source_mapping_fields": ["tenant_id"],
                "target_contract_fields": ["tenant_id"],
                "target_alias_indices": [],
                "minimum_node_free_bytes": 200_000_000_000,
                "source_count": 101,
            },
            tenant_id="default",
            start_time="2026-08-04T18:47:59Z",
            end_time="2026-08-04T18:48:00Z",
            time_field="ingest_ts",
            max_documents=100,
            slices=1,
            requests_per_second=10,
            min_free_bytes_required=contract["backfill_execution"]["minimum_free_bytes"],
            contract=contract,
        )
        self.assertEqual("BLOCKED", plan["execution_readiness"])
        self.assertTrue(any("write index" in item for item in plan["blockers"]))
        self.assertTrue(any("must be split" in item for item in plan["blockers"]))

    def test_backfill_scope_rejects_wildcard_tenant(self) -> None:
        with self.assertRaisesRegex(ValueError, "non-wildcard"):
            validate_scope("*", "2026-08-04T18:47:59Z", "2026-08-04T18:48:00Z", max_window_seconds=3600)

    def test_backfill_renderer_is_suspended_and_binds_plan_and_recounts(self) -> None:
        contract = json.loads((ROOT / "contracts/opensearch/index-governance.v1.json").read_text())
        fields = ["tenant_id", "alert_id", "ingest_ts"]
        plan = build_plan(
            observation={
                "cluster_uuid": "cluster-uuid-1",
                "cluster_status": "green",
                "source_mapping_fields": fields,
                "target_contract_fields": fields,
                "target_alias_indices": [{"index": "alerts-v2-000001", "is_write_index": True}],
                "minimum_node_free_bytes": 200_000_000_000,
                "source_count": 14,
            },
            tenant_id="default",
            start_time="2026-08-04T18:47:59Z",
            end_time="2026-08-04T18:48:00Z",
            time_field="ingest_ts",
            max_documents=100,
            slices=1,
            requests_per_second=10,
            min_free_bytes_required=contract["backfill_execution"]["minimum_free_bytes"],
            contract=contract,
        )
        now = datetime.now(timezone.utc)
        plan["captured_at"] = now.isoformat()
        plan["observation"] = {"cluster_uuid": "cluster-uuid-1"}
        validate_backfill_plan(plan, now=now)
        rendered = render_backfill_documents(
            plan=plan,
            run_id="backfill-20260805-001",
            approval_id="CHG-20260805-002",
            approved_by="sre,qa,security",
            not_before_epoch=1_786_000_000,
            expires_at_epoch=1_786_003_600,
            g0_candidate_sha256="a" * 64,
            g0_manifest_sha256="b" * 64,
            contract_file_sha256="c" * 64,
        )
        config = next(item for item in rendered if item["kind"] == "ConfigMap")
        secret = next(item for item in rendered if item["kind"] == "Secret")
        job = next(item for item in rendered if item["kind"] == "Job")
        self.assertTrue(config["immutable"])
        self.assertTrue(secret["immutable"])
        self.assertTrue(job["spec"]["suspend"])
        self.assertEqual(0, job["spec"]["backoffLimit"])
        self.assertEqual(plan["plan_sha256"], secret["stringData"]["plan_sha256"])
        script = job["spec"]["template"]["spec"]["containers"][0]["args"][0]
        self.assertIn("TARGET_COUNT_BEFORE", script)
        self.assertIn("TARGET_COUNT_AFTER", script)
        self.assertIn("_tasks/$TASK_ID/_cancel", script)

    def test_backfill_renderer_rejects_stale_or_tampered_plan(self) -> None:
        now = datetime.now(timezone.utc)
        stale = {
            "remediation_id": "T-OS-002",
            "mode": "READ_ONLY_PLAN",
            "scoped_evidence_status": "PASS",
            "execution_readiness": "READY",
            "production_applied": False,
            "production_mutations": [],
            "captured_at": (now - timedelta(hours=1)).isoformat(),
            "binding": {"target_write_index": "alerts-v2-000001", "source_count": 1},
            "plan_sha256": "0" * 64,
        }
        with self.assertRaisesRegex(ValueError, "SHA-256"):
            validate_backfill_plan(stale, now=now)


if __name__ == "__main__":
    unittest.main()
