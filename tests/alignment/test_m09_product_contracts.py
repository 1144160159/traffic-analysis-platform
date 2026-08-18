from __future__ import annotations

import hashlib
import importlib.util
import json
import sys
import unittest
from collections import Counter
from pathlib import Path


ROOT = Path(__file__).resolve().parents[2]
ALIGNMENT = ROOT / "scripts/alignment"
sys.path.insert(0, str(ALIGNMENT))

from inventory import build_inventory  # noqa: E402
from validate import _validate_contract  # noqa: E402

RUNNER_PATH = ROOT / "scripts/alignment/run_m09_product_contracts_k8s.py"
RUNNER_SPEC = importlib.util.spec_from_file_location(
    "run_m09_product_contracts_k8s", RUNNER_PATH
)
RUNNER = importlib.util.module_from_spec(RUNNER_SPEC)
assert RUNNER_SPEC.loader is not None
RUNNER_SPEC.loader.exec_module(RUNNER)


FEATURES = {
    "F-ENCRYPTED-001": {
        "owner": "encrypted-traffic-domain-owner",
        "route": "/encrypted-traffic",
        "flag": "ENCRYPTED_TRAFFIC_SNAPSHOT_V1_ENABLED",
        "rollback": "doc/07_alignment/runbooks/F-ENCRYPTED-001-rollback.md",
    },
    "F-ENCRYPTED-002": {
        "owner": "encrypted-traffic-domain-owner",
        "route": "/encrypted-traffic",
        "flag": "ENCRYPTED_ACTION_JOBS_V1_ENABLED",
        "rollback": "doc/07_alignment/runbooks/F-ENCRYPTED-002-rollback.md",
    },
    "F-FORENSICS-001": {
        "owner": "forensics-domain-owner",
        "route": "/forensics",
        "flag": "FORENSICS_PIPELINE_V1_ENABLED",
        "rollback": "doc/07_alignment/runbooks/F-FORENSICS-001-rollback.md",
    },
}


class M09ProductContractTest(unittest.TestCase):
    @classmethod
    def setUpClass(cls) -> None:
        cls.inventory = build_inventory()
        cls.contracts = {
            feature_id: json.loads(
                (
                    ROOT / f"contracts/alignment/features/{feature_id}.json"
                ).read_text(encoding="utf-8")
            )
            for feature_id in FEATURES
        }
        cls.packages = {
            package["id"]: package
            for package in json.loads(
                (ROOT / "contracts/alignment/work-packages.json").read_text(
                    encoding="utf-8"
                )
            )["packages"]
        }
        cls.canonical = {
            item["id"]: item
            for item in json.loads(
                (ROOT / "contracts/alignment/canonical-registry.json").read_text(
                    encoding="utf-8"
                )
            )["items"]
        }

    def test_contracts_use_the_existing_canonical_ids_and_unique_owners(self) -> None:
        all_contract_ids = []
        for path in (ROOT / "contracts/alignment/features").glob("*.json"):
            all_contract_ids.append(json.loads(path.read_text())["feature_id"])
        duplicates = {
            feature_id
            for feature_id, count in Counter(all_contract_ids).items()
            if count != 1
        }
        self.assertFalse(duplicates)
        for feature_id, expected in FEATURES.items():
            self.assertIn(feature_id, self.canonical)
            package_id = self.canonical[feature_id]["work_package"]
            self.assertEqual(expected["owner"], self.packages[package_id]["accountable"])
            self.assertEqual(feature_id, self.contracts[feature_id]["feature_id"])
            self.assertEqual("draft", self.contracts[feature_id]["status"])

    def test_contracts_validate_against_current_route_action_api_and_scope_inventory(self) -> None:
        for feature_id, contract in self.contracts.items():
            self.assertEqual([], _validate_contract(contract, self.inventory), feature_id)
            self.assertIn(FEATURES[feature_id]["route"], contract["compatibility"]["preserved_routes"])
            self.assertGreaterEqual(len(contract["domain"]["invariants"]), 6)
            self.assertTrue(contract["performance"]["baseline_required"])
            self.assertIn("budgets", contract["performance"])

    def test_encrypted_snapshot_has_exact_fact_classes_and_missing_semantics(self) -> None:
        contract = self.contracts["F-ENCRYPTED-001"]
        self.assertEqual(
            {
                "flow_metadata",
                "plaintext_visible",
                "side_channel",
                "raw_reference",
                "randomness_statistics",
            },
            set(contract["domain"]["snapshot_sections"]),
        )
        self.assertEqual(
            {
                "available",
                "zero",
                "no_sample",
                "not_computable",
                "unavailable",
                "forbidden",
            },
            set(contract["domain"]["availability_states"]),
        )
        self.assertTrue(
            any(
                "never" in invariant and "malicious" in invariant
                for invariant in contract["domain"]["invariants"]
            )
        )

    def test_encrypted_actions_bind_authority_final_entity_and_compensation(self) -> None:
        contract = self.contracts["F-ENCRYPTED-002"]
        authorities = contract["domain"]["action_authority"]
        self.assertEqual(
            {
                "create_alert",
                "collect_pcap",
                "verify_hash",
                "export_evidence",
                "preserve_evidence",
                "associate_analysis",
                "response_request",
            },
            set(authorities),
        )
        for authority in authorities.values():
            self.assertEqual(
                {"scope", "final_entity", "executor", "compensation"},
                set(authority),
            )
        self.assertTrue(contract["api"]["async"])
        self.assertEqual("Idempotency-Key", contract["api"]["idempotency_key"])

    def test_forensics_contract_freezes_request_and_reuses_versioned_restoration(self) -> None:
        contract = self.contracts["F-FORENSICS-001"]
        self.assertIn("five_tuple", contract["domain"]["frozen_request"])
        self.assertIn("permission_snapshot", contract["domain"]["frozen_request"])
        self.assertIn("chain_of_custody", contract["domain"]["manifest_fields"])
        self.assertTrue(
            any(
                "M03 versioned restoration interface" in invariant
                and "second reassembly algorithm" in invariant
                for invariant in contract["domain"]["invariants"]
            )
        )
        self.assertTrue(
            any(
                "inert bytes" in invariant and "never executed" in invariant
                for invariant in contract["domain"]["invariants"]
            )
        )

    def test_all_three_rollouts_are_default_off_and_runbooks_preserve_facts(self) -> None:
        for feature_id, expected in FEATURES.items():
            contract = self.contracts[feature_id]
            self.assertEqual(expected["flag"], contract["rollout"]["feature_flag"])
            self.assertFalse(contract["rollout"]["default"])
            self.assertEqual(expected["rollback"], contract["rollout"]["rollback_runbook"])
            runbook = ROOT / expected["rollback"]
            self.assertTrue(runbook.is_file())
            text = runbook.read_text(encoding="utf-8")
            self.assertIn("retain", text.lower())
            self.assertGreaterEqual(len(text.splitlines()), 8)

    def test_contract_hashes_are_distinct_and_stable_inputs(self) -> None:
        hashes = {
            hashlib.sha256(
                (ROOT / f"contracts/alignment/features/{feature_id}.json").read_bytes()
            ).hexdigest()
            for feature_id in FEATURES
        }
        self.assertEqual(3, len(hashes))

    def test_kubernetes_contract_validator_is_read_only_and_isolated(self) -> None:
        run_id = "33333333-3333-4333-8333-333333333333"
        suffix = RUNNER.validate_inputs(
            "traffic/mlops-trainer:m08-governed-export-w2c-20260815-r10",
            run_id,
            "8-2tb",
        )
        configmap, job = RUNNER.build_objects(
            RUNNER.names(suffix),
            "traffic/mlops-trainer:m08-governed-export-w2c-20260815-r10",
            run_id,
            "8-2tb",
        )
        annotations = job["metadata"]["annotations"]
        for store in ("postgres", "clickhouse", "kafka", "minio", "flink"):
            self.assertEqual(
                "false", annotations[f"traffic.analysis/shared-{store}-touched"]
            )
        pod = job["spec"]["template"]["spec"]
        self.assertFalse(pod["automountServiceAccountToken"])
        container = pod["containers"][0]
        self.assertTrue(container["securityContext"]["readOnlyRootFilesystem"])
        self.assertEqual(len(RUNNER.FILES), len(configmap["data"]))
        self.assertEqual(
            {str(path) for path in RUNNER.FILES},
            {item["path"] for item in pod["volumes"][0]["configMap"]["items"]},
        )


if __name__ == "__main__":
    unittest.main()
