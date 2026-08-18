from __future__ import annotations

import copy
import hashlib
import importlib.util
import json
import tempfile
import unittest
from pathlib import Path
from typing import Any


ROOT = Path(__file__).resolve().parents[2]
SCRIPT = ROOT / "scripts/alignment/evaluate_m08_promotion.py"
SPEC = importlib.util.spec_from_file_location("evaluate_m08_promotion", SCRIPT)
MODULE = importlib.util.module_from_spec(SPEC)
assert SPEC.loader is not None
SPEC.loader.exec_module(MODULE)
CONTRACT = json.loads(MODULE.CONTRACT_PATH.read_text(encoding="utf-8"))
RUNNER_PATH = ROOT / "scripts/alignment/run_m08_promotion_index_k8s.py"
RUNNER_SPEC = importlib.util.spec_from_file_location(
    "run_m08_promotion_index_k8s", RUNNER_PATH
)
RUNNER = importlib.util.module_from_spec(RUNNER_SPEC)
assert RUNNER_SPEC.loader is not None
RUNNER_SPEC.loader.exec_module(RUNNER)


def write_json(root: Path, name: str, body: dict[str, Any]) -> dict[str, str]:
    path = root / name
    path.write_text(
        json.dumps(body, sort_keys=True, indent=2) + "\n", encoding="utf-8"
    )
    return {
        "path": str(path.relative_to(ROOT)),
        "sha256": hashlib.sha256(path.read_bytes()).hexdigest(),
    }


def set_pointer(body: dict[str, Any], pointer: str, value: Any) -> None:
    target = body
    tokens = pointer[1:].split("/")
    for token in tokens[:-1]:
        target = target.setdefault(token, {})
    target[tokens[-1]] = value


def valid_manifest(root: Path) -> dict[str, Any]:
    profile = "M08_PRODUCTION_PROMOTION_V1"
    environment = "k8s-production"
    candidate_binding = write_json(
        root,
        "candidate.json",
        {
            "schema_version": 1,
            "artifact_kind": "M08_IMPLEMENTATION_CANDIDATE_MANIFEST",
            "status": "FROZEN",
            "profile_id": profile,
            "environment_id": environment,
        },
    )
    candidate_id = candidate_binding["sha256"]
    evidence = {}
    for index, (evidence_id, rule) in enumerate(
        CONTRACT["required_evidence"].items()
    ):
        body: dict[str, Any] = {
            "schema_version": 1,
            "artifact_kind": rule["artifact_kind"],
            "candidate_manifest_sha256": candidate_id,
            "profile_id": profile,
            "environment_id": environment,
        }
        for assertion in (
            rule["engineering_assertions"] + rule["promotion_assertions"]
        ):
            expected = assertion["expected"]
            if assertion["operator"] == "greater_than":
                expected += 0.5
            set_pointer(body, assertion["pointer"], expected)
        evidence[evidence_id] = write_json(root, f"evidence-{index:02d}.json", body)
    return {
        "schema_version": 1,
        "artifact_kind": "M08_PROMOTION_MANIFEST",
        "candidate_id": candidate_id,
        "profile_id": profile,
        "environment_id": environment,
        "candidate_manifest": candidate_binding,
        "evidence": evidence,
        "allowed_claims": CONTRACT["allowed_claims"],
        "forbidden_claims": CONTRACT["forbidden_claims"],
        "production_applied": True,
    }


class M08PromotionTest(unittest.TestCase):
    def setUp(self) -> None:
        self.temp = tempfile.TemporaryDirectory(prefix=".tmp-m08-promotion-", dir=ROOT)
        self.root = Path(self.temp.name)
        self.manifest = valid_manifest(self.root)

    def tearDown(self) -> None:
        self.temp.cleanup()

    def assertPromotionBlockedWith(self, manifest: dict[str, Any], code: str) -> None:
        result = MODULE.evaluate(manifest, CONTRACT)
        self.assertEqual("BLOCKED", result["promotion_status"])
        self.assertFalse(result["promotion_allowed"])
        self.assertTrue(
            any(item["code"] == code for item in result["promotion_blockers"]),
            result["promotion_blockers"],
        )

    def assertEngineeringBlockedWith(self, manifest: dict[str, Any], code: str) -> None:
        result = MODULE.evaluate(manifest, CONTRACT)
        self.assertEqual("BLOCKED", result["engineering_status"])
        self.assertFalse(result["promotion_allowed"])
        self.assertTrue(
            any(item["code"] == code for item in result["engineering_errors"]),
            result["engineering_errors"],
        )

    def test_exact_same_candidate_production_receipts_can_promote(self) -> None:
        result = MODULE.evaluate(self.manifest, CONTRACT)
        self.assertEqual("PASS", result["engineering_status"])
        self.assertEqual("PASS", result["promotion_status"])
        self.assertTrue(result["promotion_allowed"])
        self.assertFalse(result["production_applied"])
        self.assertFalse(result["automatic_repair"])

    def test_missing_candidate_can_keep_engineering_index_but_never_promotes(self) -> None:
        manifest = copy.deepcopy(self.manifest)
        manifest["candidate_id"] = None
        manifest["candidate_manifest"] = None
        manifest["production_applied"] = False
        result = MODULE.evaluate(manifest, CONTRACT)
        self.assertEqual("PASS", result["engineering_status"])
        self.assertEqual(CONTRACT["allowed_claims"], result["allowed_claims"])
        self.assertPromotionBlockedWith(manifest, "CANDIDATE_MANIFEST_REQUIRED")
        self.assertPromotionBlockedWith(manifest, "PRODUCTION_APPLIED_REQUIRED")

    def test_unknown_recall_zero_is_a_targeted_promotion_blocker(self) -> None:
        manifest = copy.deepcopy(self.manifest)
        binding = manifest["evidence"]["threshold_evaluation"]
        body = json.loads((ROOT / binding["path"]).read_text(encoding="utf-8"))
        body["oracles"]["unknown_recall"] = 0
        manifest["evidence"]["threshold_evaluation"] = write_json(
            self.root, "zero-unknown-recall.json", body
        )
        self.assertPromotionBlockedWith(manifest, "PROMOTION_ASSERTION")

    def test_cross_candidate_receipt_never_aggregates(self) -> None:
        manifest = copy.deepcopy(self.manifest)
        binding = manifest["evidence"]["rollback"]
        body = json.loads((ROOT / binding["path"]).read_text(encoding="utf-8"))
        body["candidate_manifest_sha256"] = "f" * 64
        manifest["evidence"]["rollback"] = write_json(
            self.root, "cross-candidate-rollback.json", body
        )
        self.assertPromotionBlockedWith(manifest, "EVIDENCE_SCOPE")

    def test_hash_drift_blocks_the_engineering_index(self) -> None:
        manifest = copy.deepcopy(self.manifest)
        binding = manifest["evidence"]["inference_parity"]
        with (ROOT / binding["path"]).open("a", encoding="utf-8") as stream:
            stream.write(" \n")
        self.assertEngineeringBlockedWith(manifest, "BINDING_HASH")

    def test_missing_evidence_is_not_silently_accepted(self) -> None:
        manifest = copy.deepcopy(self.manifest)
        del manifest["evidence"]["online_consumer_ack"]
        self.assertEngineeringBlockedWith(manifest, "EVIDENCE_EXACT_SET")

    def test_claim_scope_cannot_be_broadened(self) -> None:
        manifest = copy.deepcopy(self.manifest)
        manifest["allowed_claims"].append("CNAS passed")
        manifest["forbidden_claims"] = []
        self.assertEngineeringBlockedWith(manifest, "ALLOWED_CLAIMS")
        self.assertEngineeringBlockedWith(manifest, "FORBIDDEN_CLAIMS")

    def test_hidden_manifest_field_is_rejected(self) -> None:
        manifest = copy.deepcopy(self.manifest)
        manifest["status"] = "PASS"
        self.assertEngineeringBlockedWith(manifest, "MANIFEST_EXACT_FIELDS")

    def test_repository_no_go_pointer_matches_the_current_exact_index(self) -> None:
        input_path = (
            ROOT
            / "doc/02_acceptance/topic1/work-orders/t1-m08-p038-idx-n018-s1/promotion-input.json"
        )
        index_path = input_path.with_name("current-index.json")
        pointer_path = (
            ROOT / "contracts/releases/topic1/t1-m08-release-pointer.json"
        )
        current_input = json.loads(input_path.read_text(encoding="utf-8"))
        expected_index = MODULE.evaluate(current_input, CONTRACT)
        persisted_index = json.loads(index_path.read_text(encoding="utf-8"))
        pointer = json.loads(pointer_path.read_text(encoding="utf-8"))
        self.assertEqual(expected_index, persisted_index)
        self.assertEqual("PASS", persisted_index["engineering_status"])
        self.assertEqual("BLOCKED", persisted_index["promotion_status"])
        self.assertFalse(persisted_index["promotion_allowed"])
        self.assertEqual("NO_GO", pointer["status"])
        self.assertFalse(pointer["promotion_allowed"])
        self.assertIsNone(pointer["candidate_id"])
        self.assertEqual(
            hashlib.sha256(input_path.read_bytes()).hexdigest(),
            pointer["promotion_input"]["sha256"],
        )
        self.assertEqual(
            hashlib.sha256(index_path.read_bytes()).hexdigest(),
            pointer["current_index"]["sha256"],
        )
        self.assertEqual(CONTRACT["forbidden_claims"], pointer["forbidden_claims"])

    def test_kubernetes_validator_is_read_only_and_has_the_exact_workspace(self) -> None:
        run_id = "33333333-3333-4333-8333-333333333333"
        suffix = RUNNER.validate_inputs(
            "traffic/mlops-trainer:m08-governed-export-w2c-20260815-r10",
            run_id,
            "8-2tb",
        )
        objects = RUNNER.build_objects(
            RUNNER.names(suffix),
            "traffic/mlops-trainer:m08-governed-export-w2c-20260815-r10",
            run_id,
            "8-2tb",
        )
        configmap, job = objects
        self.assertEqual(2, len(objects))
        annotations = job["metadata"]["annotations"]
        for store in ("postgres", "clickhouse", "kafka", "minio", "flink"):
            self.assertEqual(
                "false", annotations[f"traffic.analysis/shared-{store}-touched"]
            )
        spec = job["spec"]["template"]["spec"]
        self.assertFalse(spec["automountServiceAccountToken"])
        container = spec["containers"][0]
        self.assertTrue(container["securityContext"]["readOnlyRootFilesystem"])
        self.assertEqual(["python3"], container["command"])
        mounted_paths = {
            item["path"]
            for item in spec["volumes"][0]["configMap"]["items"]
        }
        self.assertEqual(
            {str(path) for path in RUNNER.required_files()}, mounted_paths
        )
        self.assertEqual(len(configmap["data"]), len(mounted_paths))


if __name__ == "__main__":
    unittest.main()
