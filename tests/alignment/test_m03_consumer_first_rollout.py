import copy
import hashlib
import json
import sys
import tempfile
import unittest
from pathlib import Path

import yaml


ROOT = Path(__file__).resolve().parents[2]
sys.path.insert(0, str(ROOT / "scripts/alignment"))

from run_m03_consumer_first_rollout import (  # noqa: E402
    ROLLBACK_STEPS,
    STAGES,
    RolloutBlocked,
    evaluate,
    load_plan,
    render_next_action,
    sha256_path,
    validate_pass_evidence,
)


CANDIDATE = "a" * 64
SESSION_INITIAL = "b" * 64
FEATURE_INITIAL = "c" * 64
SESSION_CUTOVER = "d" * 64
FEATURE_CUTOVER = "e" * 64


class M03ConsumerFirstRolloutTest(unittest.TestCase):
    def setUp(self) -> None:
        self.temp = tempfile.TemporaryDirectory(prefix="m03-rollout-", dir=ROOT)
        self.temp_path = Path(self.temp.name)
        self.addCleanup(self.temp.cleanup)
        self.manifest_path = self.temp_path / "savepoints.json"
        self.manifest = {
            "schema_version": 1,
            "source_cluster_id": "flink-traffic",
            "savepoints": {
                "flink-session-job": {
                    "uri": "s3://flink-checkpoints/savepoints/m03/session-initial",
                    "sha256": SESSION_INITIAL,
                    "source_job_id": "1" * 32,
                },
                "flink-feature-job": {
                    "uri": "s3://flink-checkpoints/savepoints/m03/feature-initial",
                    "sha256": FEATURE_INITIAL,
                    "source_job_id": "2" * 32,
                },
            },
        }
        self.manifest_path.write_text(json.dumps(self.manifest), encoding="utf-8")
        self.plan_path = self.temp_path / "plan.json"
        self.plan = {
            "schema_version": 1,
            "rollout_id": "m03-candidate-20260814",
            "candidate_sha256": CANDIDATE,
            "state_recovery_contract_sha256": sha256_path(
                ROOT / "contracts/flink/state-recovery.v1.json"
            ),
            "savepoint_manifest_path": str(self.manifest_path.relative_to(ROOT)),
            "jobs": [
                {
                    "job_id": "flink-session-job",
                    "image": "registry.local/flink-session@sha256:" + "3" * 64,
                    "previous_image": "registry.local/flink-session@sha256:" + "8" * 64,
                    "source_job_id": "1" * 32,
                    "expected_tasks": 24,
                },
                {
                    "job_id": "flink-feature-job",
                    "image": "registry.local/flink-feature@sha256:" + "4" * 64,
                    "previous_image": "registry.local/flink-feature@sha256:" + "9" * 64,
                    "source_job_id": "2" * 32,
                    "expected_tasks": 18,
                },
            ],
            "producer_scope": {
                "tenant_id": "tenant-canary",
                "probe_ids": ["probe-a", "probe-b"],
                "event_contract_sha256": "5" * 64,
                "initially_enabled": False,
            },
            "stages": STAGES,
            "rollback_steps": ROLLBACK_STEPS,
        }
        self.plan_path.write_text(json.dumps(self.plan), encoding="utf-8")

    def receipt(self, sequence: int, status: str = "PASS") -> dict:
        stage = STAGES[sequence - 1]
        evidence: dict = {}
        if stage == "STATIC_COMPATIBILITY_VERIFIED":
            evidence = {
                "state_recovery_result": "PASS",
                "state_recovery_contract_sha256": self.plan["state_recovery_contract_sha256"],
                "savepoint_manifest_sha256": hashlib.sha256(
                    self.manifest_path.read_bytes()
                ).hexdigest(),
            }
        elif stage in {"SESSION_SHADOW_VERIFIED", "FEATURE_SHADOW_VERIFIED"}:
            session = stage.startswith("SESSION")
            job_id = "flink-session-job" if session else "flink-feature-job"
            job = self.plan["jobs"][0 if session else 1]
            evidence = {
                "job_id": job_id,
                "image": job["image"],
                "activation": "shadow",
                "consumer_group": job_id + "-shadow-" + CANDIDATE[:12],
                "running_tasks": job["expected_tasks"],
                "expected_tasks": job["expected_tasks"],
                "restored_from_savepoint": True,
                "restored_savepoint_sha256": SESSION_INITIAL if session else FEATURE_INITIAL,
                "completed_checkpoint_id": 101 if session else 201,
                "failed_checkpoints": 0,
                "external_writes": 0,
            }
        elif stage in {"OLD_SESSION_STOPPED_WITH_SAVEPOINT", "OLD_FEATURE_STOPPED_WITH_SAVEPOINT"}:
            session = "SESSION" in stage
            job_id = "flink-session-job" if session else "flink-feature-job"
            evidence = {
                "job_id": job_id,
                "old_job_stopped": True,
                "shadow_job_stopped": True,
                "cutover_savepoint_uri": f"s3://flink-checkpoints/savepoints/m03-cutover/{job_id}/sp-1",
                "cutover_savepoint_sha256": SESSION_CUTOVER if session else FEATURE_CUTOVER,
                "cutover_source_job_id": "6" * 32 if session else "7" * 32,
            }
        elif stage in {"SESSION_PRODUCTION_VERIFIED", "FEATURE_PRODUCTION_VERIFIED"}:
            session = stage.startswith("SESSION")
            job_id = "flink-session-job" if session else "flink-feature-job"
            job = self.plan["jobs"][0 if session else 1]
            evidence = {
                "job_id": job_id,
                "image": job["image"],
                "activation": "production",
                "consumer_group": job_id,
                "running_tasks": job["expected_tasks"],
                "expected_tasks": job["expected_tasks"],
                "restored_from_savepoint": True,
                "restored_savepoint_sha256": SESSION_CUTOVER if session else FEATURE_CUTOVER,
                "completed_checkpoint_id": 301 if session else 401,
                "failed_checkpoints": 0,
                "external_writes": 10,
            }
        elif stage == "PRODUCER_CANARY_VERIFIED":
            evidence = {
                "producer_enabled": True,
                "tenant_id": "tenant-canary",
                "probe_ids": ["probe-a", "probe-b"],
                "event_contract_sha256": "5" * 64,
                "session_checkpoint_id": 301,
                "feature_checkpoint_id": 401,
            }
        elif stage == "RECONCILIATION_PASSED":
            evidence = {
                "event_reconciliation_result": "PASS",
                "online_offline_parity_result": "PASS",
                "conflicting_event_ids": 0,
                "unexplained_field_differences": 0,
            }
        if status == "FAIL":
            evidence = {"failure_code": "CHECKPOINT_FAILED", "failure_detail": "injected"}
        return {
            "schema_version": 1,
            "rollout_id": self.plan["rollout_id"],
            "candidate_sha256": CANDIDATE,
            "sequence": sequence,
            "stage": stage,
            "status": status,
            "observed_at": f"2026-08-14T01:{sequence:02d}:00Z",
            "evidence": evidence,
        }

    def test_plan_binds_current_state_contract_savepoints_and_task_counts(self) -> None:
        loaded = load_plan(self.plan_path)
        self.assertEqual(self.plan, loaded)

    def test_plan_rejects_feature_before_its_session_dependency(self) -> None:
        candidate = copy.deepcopy(self.plan)
        candidate["jobs"].reverse()
        candidate_path = self.temp_path / "reordered-plan.json"
        candidate_path.write_text(json.dumps(candidate), encoding="utf-8")
        with self.assertRaisesRegex(RolloutBlocked, "BLOCK_JOB_ORDER"):
            load_plan(candidate_path)

    def test_static_receipt_renders_session_shadow_application_manifest(self) -> None:
        payload = render_next_action(self.plan, [self.receipt(1)])
        docs = list(yaml.safe_load_all(payload))
        launcher = next(doc for doc in docs if doc["metadata"]["name"] == "migrate-flink-session-job-shadow-" + CANDIDATE[:12])
        command = launcher["spec"]["template"]["spec"]["containers"][0]["command"]
        self.assertEqual("shadow", command[command.index("--deployment.activation.mode") + 1])
        self.assertEqual(
            "flink-session-job-shadow-" + CANDIDATE[:12],
            command[command.index("--consumer.group") + 1],
        )

    def test_production_manifest_restores_cutover_not_initial_savepoint(self) -> None:
        receipts = [self.receipt(index) for index in range(1, 5)]
        payload = render_next_action(self.plan, receipts)
        docs = list(yaml.safe_load_all(payload))
        launcher = next(doc for doc in docs if doc["metadata"]["name"] == "migrate-flink-session-job-production-" + CANDIDATE[:12])
        command = launcher["spec"]["template"]["spec"]["containers"][0]["command"]
        restored = command[command.index("-s") + 1]
        self.assertIn("m03-cutover/flink-session-job", restored)
        self.assertNotEqual(self.manifest["savepoints"]["flink-session-job"]["uri"], restored)
        self.assertEqual("production", command[command.index("--deployment.activation.mode") + 1])

    def test_shadow_external_write_is_rejected(self) -> None:
        receipt = self.receipt(2)
        receipt["evidence"]["external_writes"] = 1
        with self.assertRaisesRegex(RolloutBlocked, "shadow_writes"):
            validate_pass_evidence(self.plan, receipt, [self.receipt(1)])

    def test_receipts_cannot_skip_feature_shadow_or_reorder_cutover(self) -> None:
        receipts = [self.receipt(1), self.receipt(3)]
        with self.assertRaisesRegex(RolloutBlocked, "BLOCK_RECEIPT_ORDER"):
            evaluate(self.plan, receipts)

    def test_failed_receipt_stops_progress_and_returns_fixed_rollback(self) -> None:
        state = evaluate(self.plan, [self.receipt(1), self.receipt(2, "FAIL")])
        self.assertEqual("rollback_required", state["result"])
        self.assertEqual("DISABLE_PRODUCER_CANARY", state["next_action"])
        self.assertEqual(ROLLBACK_STEPS, state["rollback_steps"])
        self.assertFalse(state["production_applied"])

    def test_full_sequence_requires_zero_conflicts_and_unexplained_diffs(self) -> None:
        receipts = [self.receipt(index) for index in range(1, 10)]
        state = evaluate(self.plan, receipts)
        self.assertEqual("complete", state["result"])
        self.assertTrue(state["producer_canary_enabled"])
        bad = copy.deepcopy(receipts)
        bad[-1]["evidence"]["conflicting_event_ids"] = 1
        with self.assertRaisesRegex(RolloutBlocked, "BLOCK_RECONCILIATION"):
            evaluate(self.plan, bad)


if __name__ == "__main__":
    unittest.main()
