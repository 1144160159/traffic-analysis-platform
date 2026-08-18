from __future__ import annotations

import copy
import importlib.util
import json
import unittest
from pathlib import Path


ROOT = Path(__file__).resolve().parents[2]
SCRIPT = ROOT / "scripts/alignment/evaluate_m06_phase_acceptance.py"
SPEC = importlib.util.spec_from_file_location("evaluate_m06_phase_acceptance", SCRIPT)
MODULE = importlib.util.module_from_spec(SPEC)
assert SPEC.loader is not None
SPEC.loader.exec_module(MODULE)
CONTRACT = json.loads(MODULE.CONTRACT_PATH.read_text(encoding="utf-8"))


def valid_manifest(phase: str) -> dict:
    candidate_id = "a" * 64
    profile_id = "M06_K8S_PRODUCER_RAIL_CANARY_V1"
    environment_id = "k8s-shared"
    source_tuple_sha256 = "b" * 64
    phase_contract = CONTRACT["phases"][phase]
    return {
        "schema_version": 1,
        "artifact_kind": "M06_PHASE_ACCEPTANCE_MANIFEST",
        "phase": phase,
        "candidate_id": candidate_id,
        "profile_id": profile_id,
        "environment_id": environment_id,
        "consumer_readiness": {
            "state": "RUNNING",
            "observed_before_producer": True,
            "candidate_id": candidate_id,
            "profile_id": profile_id,
            "environment_id": environment_id,
            "receipt_sha256": "c" * 64,
        },
        "canary_results": [
            {
                "phase": canary_phase,
                "status": "PASS",
                "candidate_id": candidate_id,
                "profile_id": profile_id,
                "environment_id": environment_id,
                "activation_retained_for_acceptance": True,
                "receipt_sha256": f"{index + 1:064x}",
            }
            for index, canary_phase in enumerate(phase_contract["canary_phases"])
        ],
        "source_authority": {
            "kind": "network_device_syslog_connector" if phase == "device-logs" else "runtime_service",
            "real_source": True,
            "fixture": False,
            "postgres_seed": False,
            "candidate_id": candidate_id,
            "profile_id": profile_id,
            "environment_id": environment_id,
            "source_tuple_sha256": source_tuple_sha256,
            "receipt_sha256": "d" * 64,
        },
        "source_tuple_sha256": source_tuple_sha256,
        "oracle_receipts": {
            oracle: {
                "status": "PASS",
                "candidate_id": candidate_id,
                "profile_id": profile_id,
                "environment_id": environment_id,
                "source_tuple_sha256": source_tuple_sha256,
                "receipt_sha256": f"{index + 20:064x}",
            }
            for index, oracle in enumerate(phase_contract["oracles"])
        },
        "production_applied": True,
    }


class M06PhaseAcceptanceTest(unittest.TestCase):
    def assertFailsWith(self, manifest: dict, code: str) -> None:
        result = MODULE.evaluate(manifest, CONTRACT)
        self.assertEqual("FAIL", result["status"])
        self.assertTrue(any(item["code"] == code for item in result["errors"]), result["errors"])

    def test_each_real_k8s_phase_has_an_exact_pass_contract(self) -> None:
        for phase in CONTRACT["phases"]:
            with self.subTest(phase=phase):
                result = MODULE.evaluate(valid_manifest(phase), CONTRACT)
                self.assertEqual("PASS", result["status"])
                self.assertTrue(result["production_applied"])
                self.assertEqual(CONTRACT["phases"][phase]["oracles"], result["oracles"])

    def test_consumer_must_be_running_before_producer(self) -> None:
        manifest = valid_manifest("asset-events")
        manifest["consumer_readiness"]["observed_before_producer"] = False
        self.assertFailsWith(manifest, "CONSUMER_NOT_READY_FIRST")

    def test_canary_order_and_profile_binding_are_exact(self) -> None:
        manifest = valid_manifest("asset-bindings")
        manifest["canary_results"].reverse()
        manifest["canary_results"][0]["profile_id"] = "cross-profile"
        self.assertFailsWith(manifest, "CANARY_PHASE_ORDER")
        self.assertFailsWith(manifest, "CANARY_RESULT")

    def test_oracle_set_and_receipt_binding_are_exact(self) -> None:
        manifest = valid_manifest("asset-events")
        first = next(iter(manifest["oracle_receipts"]))
        manifest["oracle_receipts"][first]["candidate_id"] = "f" * 64
        manifest["oracle_receipts"]["UNEXPECTED"] = copy.deepcopy(manifest["oracle_receipts"][first])
        self.assertFailsWith(manifest, "ORACLE_EXACT_SET")
        self.assertFailsWith(manifest, "ORACLE_RECEIPT")

    def test_device_phase_rejects_fixture_seed_and_wrong_source_kind(self) -> None:
        manifest = valid_manifest("device-logs")
        manifest["source_authority"]["fixture"] = True
        manifest["source_authority"]["postgres_seed"] = True
        manifest["source_authority"]["kind"] = "fixture_file"
        self.assertFailsWith(manifest, "SOURCE_REALITY")
        self.assertFailsWith(manifest, "DEVICE_SOURCE_AUTHORITY")

    def test_source_authority_and_activation_cannot_cross_candidate_or_be_dry_run(self) -> None:
        manifest = valid_manifest("asset-bindings")
        manifest["source_authority"]["candidate_id"] = "e" * 64
        manifest["production_applied"] = False
        self.assertFailsWith(manifest, "SOURCE_AUTHORITY_BINDING")
        self.assertFailsWith(manifest, "PRODUCTION_APPLIED_REQUIRED")

    def test_malformed_receipt_collections_fail_without_traceback(self) -> None:
        manifest = valid_manifest("device-logs")
        manifest["canary_results"] = {"not": "a list"}
        manifest["oracle_receipts"] = ["not", "an", "object"]
        self.assertFailsWith(manifest, "CANARY_RESULT_SHAPE")
        self.assertFailsWith(manifest, "ORACLE_RECEIPT_SHAPE")


if __name__ == "__main__":
    unittest.main()
