from __future__ import annotations

import copy
import hashlib
import importlib.util
import json
import tempfile
import unittest
from pathlib import Path


ROOT = Path(__file__).resolve().parents[2]
SCRIPT = ROOT / "scripts/alignment/evaluate_m06_promotion.py"
SPEC = importlib.util.spec_from_file_location("evaluate_m06_promotion", SCRIPT)
MODULE = importlib.util.module_from_spec(SPEC)
assert SPEC.loader is not None
SPEC.loader.exec_module(MODULE)
CONTRACT = json.loads(MODULE.CONTRACT_PATH.read_text(encoding="utf-8"))


def write_json(root: Path, name: str, body: dict) -> dict[str, str]:
    path = root / name
    path.write_text(json.dumps(body, sort_keys=True, indent=2) + "\n", encoding="utf-8")
    return {
        "path": str(path.relative_to(ROOT)),
        "sha256": hashlib.sha256(path.read_bytes()).hexdigest(),
    }


def valid_manifest(root: Path) -> dict:
    profile = "M06_K8S_PRODUCER_RAIL_CANARY_V1"
    environment = "k8s-shared"
    candidate_binding = write_json(root, "candidate.json", {
        "schema_version": 1,
        "artifact_kind": "M06_IMPLEMENTATION_CANDIDATE_MANIFEST",
        "status": "FROZEN",
        "profile_id": profile,
        "environment_id": environment,
        "source_tree_sha256": "f" * 64,
    })
    candidate = candidate_binding["sha256"]
    scope = {"candidate_id": candidate, "profile_id": profile, "environment_id": environment}
    tasks = {}
    for index, task in enumerate(CONTRACT["required_task_indexes"]):
        tasks[task] = write_json(root, f"task-{index:02d}.json", {
            "schema_version": 1,
            "artifact_kind": "M06_TASK_CURRENT_EVIDENCE_INDEX",
            "task_id": task,
            "status": "PASS",
            **scope,
            "evidence_receipts": [{"sha256": f"{index + 1:064x}"}],
        })
    phases = {}
    for index, phase in enumerate(CONTRACT["required_phase_acceptance"]):
        phases[phase] = write_json(root, f"phase-{index:02d}.json", {
            "schema_version": 1,
            "artifact_kind": "M06_PHASE_ACCEPTANCE_RECEIPT",
            "phase": phase,
            "status": "PASS",
            **scope,
            "production_applied": True,
        })
    reconcile = write_json(root, "four-source.json", {
        "schema_version": 1,
        "artifact_kind": "M06_FOUR_SOURCE_WINDOW_RECONCILIATION",
        "status": "PASS",
        **scope,
        "production_applied": True,
        "rail_results": {rail: {"status": "PASS"} for rail in CONTRACT["required_four_source_rails"]},
    })
    rollbacks = {}
    for index, phase in enumerate(CONTRACT["required_canary_rollbacks"]):
        rollbacks[phase] = write_json(root, f"rollback-{index:02d}.json", {
            "schema_version": 1,
            "artifact_kind": "M06_CANARY_ROLLBACK_RESULT",
            "phase": phase,
            "status": "PASS",
            **scope,
            "production_applied": False,
            "rollback": {"status": "PASS", "restored": True},
        })
    return {
        "schema_version": 1,
        "artifact_kind": "M06_PROMOTION_MANIFEST",
        **scope,
        "candidate_manifest": candidate_binding,
        "task_indexes": tasks,
        "phase_acceptance": phases,
        "four_source_reconciliation": reconcile,
        "canary_rollbacks": rollbacks,
        "allowed_claims": CONTRACT["allowed_claims"],
        "forbidden_claims": CONTRACT["forbidden_claims"],
        "production_applied": True,
    }


class M06PromotionTest(unittest.TestCase):
    def setUp(self) -> None:
        self.temp = tempfile.TemporaryDirectory(prefix=".tmp-m06-promotion-", dir=ROOT)
        self.root = Path(self.temp.name)
        self.manifest = valid_manifest(self.root)

    def tearDown(self) -> None:
        self.temp.cleanup()

    def assertBlockedWith(self, manifest: dict, code: str) -> None:
        result = MODULE.evaluate(manifest, CONTRACT)
        self.assertEqual("BLOCKED", result["status"])
        self.assertFalse(result["promotion_allowed"])
        self.assertTrue(any(item["code"] == code for item in result["errors"]), result["errors"])

    def test_exact_current_evidence_set_allows_only_bounded_m06_claims(self) -> None:
        result = MODULE.evaluate(self.manifest, CONTRACT)
        self.assertEqual("PASS", result["status"])
        self.assertTrue(result["promotion_allowed"])
        self.assertEqual(CONTRACT["allowed_claims"], result["allowed_claims"])
        self.assertEqual(CONTRACT["forbidden_claims"], result["forbidden_claims"])
        self.assertFalse(result["production_applied"])
        self.assertFalse(result["automatic_repair"])

    def test_missing_device_log_acceptance_never_promotes(self) -> None:
        manifest = copy.deepcopy(self.manifest)
        del manifest["phase_acceptance"]["device-logs"]
        self.assertBlockedWith(manifest, "PHASE_ACCEPTANCE_EXACT_SET")

    def test_cross_profile_task_index_never_aggregates(self) -> None:
        manifest = copy.deepcopy(self.manifest)
        task = CONTRACT["required_task_indexes"][0]
        binding = manifest["task_indexes"][task]
        body = json.loads((ROOT / binding["path"]).read_text(encoding="utf-8"))
        body["profile_id"] = "other-profile"
        manifest["task_indexes"][task] = write_json(self.root, "cross-profile-task.json", body)
        self.assertBlockedWith(manifest, "TASK_INDEX_RESULT")

    def test_hash_drift_is_not_silently_accepted(self) -> None:
        manifest = copy.deepcopy(self.manifest)
        binding = manifest["four_source_reconciliation"]
        with (ROOT / binding["path"]).open("a", encoding="utf-8") as stream:
            stream.write(" \n")
        self.assertBlockedWith(manifest, "BINDING_HASH")

    def test_missing_rollback_and_dry_run_are_both_blocking(self) -> None:
        manifest = copy.deepcopy(self.manifest)
        del manifest["canary_rollbacks"]["device-logs"]
        manifest["production_applied"] = False
        self.assertBlockedWith(manifest, "ROLLBACK_EXACT_SET")
        self.assertBlockedWith(manifest, "PRODUCTION_APPLIED_REQUIRED")

    def test_claim_scope_cannot_be_broadened(self) -> None:
        manifest = copy.deepcopy(self.manifest)
        manifest["allowed_claims"].append("fusion complete")
        manifest["forbidden_claims"] = []
        self.assertBlockedWith(manifest, "ALLOWED_CLAIMS")
        self.assertBlockedWith(manifest, "FORBIDDEN_CLAIMS")

    def test_hidden_manifest_field_is_rejected(self) -> None:
        manifest = copy.deepcopy(self.manifest)
        manifest["status"] = "PASS"
        self.assertBlockedWith(manifest, "MANIFEST_EXACT_FIELDS")


if __name__ == "__main__":
    unittest.main()
