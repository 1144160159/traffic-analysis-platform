from __future__ import annotations

import copy
import json
import unittest

from scripts.alignment import verify_m09_journey_evidence as verifier


class M09JourneyEvidenceMutationTests(unittest.TestCase):
    def setUp(self) -> None:
        self.contract = verifier.load_json(verifier.ROOT / verifier.CONTRACT_RELATIVE)
        self.manifest = verifier.load_json(verifier.ROOT / verifier.INPUT_RELATIVE)

    def validate(self, contract=None, manifest=None) -> list[str]:
        errors, _ = verifier.validate_manifest(
            verifier.ROOT,
            contract if contract is not None else self.contract,
            manifest if manifest is not None else self.manifest,
        )
        return errors

    def assert_error(self, errors: list[str], expected: str) -> None:
        self.assertTrue(any(expected in error for error in errors), errors)

    def verified_journey(self, index: int = 0) -> dict:
        required = self.contract["required_journeys"][index]
        current = self.manifest["journeys"][index]
        candidate_hash = self.contract["candidate_binding"]["app_image_id"]
        receipts = [
            {
                "store": store,
                "receipt_id": f"receipt-{index}-{store}",
                "trace_id": f"trace-{index}",
                "candidate_hash": candidate_hash,
                "observed_at": "2026-08-16T00:00:00Z",
            }
            for store in required["required_stores"]
        ]
        return {
            **current,
            "status": "VERIFIED",
            "browser": {
                "name": "Chrome",
                "os": "Windows",
                "version": "140.0.0.0",
                "backend": "chrome_extension",
                "viewport": "1366x900",
                "url": "http://127.0.0.1:25173/alerts",
                "route_pattern": required["route"],
                "captured_at": "2026-08-16T00:00:00Z",
            },
            "candidate": {
                "app_image": self.contract["candidate_binding"]["app_image"],
                "app_image_id": candidate_hash,
                "config_sha256": self.contract["candidate_binding"]["config_sha256"],
                "route_config_sha256": self.contract["candidate_binding"][
                    "route_config_sha256"
                ],
            },
            "checks": {
                kind: {"status": "PASS", "oracle": f"{kind}-oracle"}
                for kind in self.contract["required_check_kinds"]
            },
            "network": {
                "artifact_sha256": "1" * 64,
                "request_failed_count": 0,
                "http_4xx_count": 0,
                "http_5xx_count": 0,
            },
            "console": {
                "artifact_sha256": "2" * 64,
                "error_count": 0,
                "page_error_count": 0,
                "runtime_exception_count": 0,
            },
            "cross_storage_trace": {
                "trace_id": f"trace-{index}",
                "receipts": receipts,
                "final_fact": {
                    "store": required["required_stores"][0],
                    "fact_id": f"fact-{index}",
                    "trace_id": f"trace-{index}",
                    "candidate_hash": candidate_hash,
                    "observed_at": "2026-08-16T00:00:01Z",
                },
            },
            "dirty": False,
            "source_hash_match": True,
            "blockers": [],
        }

    def test_current_partial_snapshot_passes(self) -> None:
        self.assertEqual([], self.validate())

    def test_false_complete_claim_is_rejected(self) -> None:
        manifest = copy.deepcopy(self.manifest)
        manifest["candidate_status"] = "COMPLETE"
        self.assert_error(self.validate(manifest=manifest), "candidate_status")

    def test_missing_seventh_journey_is_rejected(self) -> None:
        manifest = copy.deepcopy(self.manifest)
        manifest["journeys"].pop()
        self.assert_error(self.validate(manifest=manifest), "exact seven journeys")

    def test_blocked_journey_with_screenshot_state_is_rejected(self) -> None:
        manifest = copy.deepcopy(self.manifest)
        manifest["journeys"][0]["dirty"] = False
        self.assert_error(self.validate(manifest=manifest), "carries unusable evidence")

    def test_wrong_windows_browser_identity_is_rejected(self) -> None:
        journey = self.verified_journey()
        journey["browser"]["os"] = "Linux"
        errors = verifier.validate_journey(
            journey, self.contract["required_journeys"][0], self.contract
        )
        self.assert_error(errors, "browser os")

    def test_candidate_image_mismatch_is_rejected(self) -> None:
        journey = self.verified_journey()
        journey["candidate"]["app_image_id"] = "sha256:" + "0" * 64
        errors = verifier.validate_journey(
            journey, self.contract["required_journeys"][0], self.contract
        )
        self.assert_error(errors, "candidate binding mismatch")

    def test_missing_mutation_check_is_rejected(self) -> None:
        journey = self.verified_journey()
        del journey["checks"]["mutation"]
        errors = verifier.validate_journey(
            journey, self.contract["required_journeys"][0], self.contract
        )
        self.assert_error(errors, "exact required kinds")

    def test_network_failure_is_rejected(self) -> None:
        journey = self.verified_journey()
        journey["network"]["http_5xx_count"] = 1
        errors = verifier.validate_journey(
            journey, self.contract["required_journeys"][0], self.contract
        )
        self.assert_error(errors, "http_5xx_count")

    def test_missing_required_store_receipt_is_rejected(self) -> None:
        journey = self.verified_journey()
        journey["cross_storage_trace"]["receipts"].pop()
        errors = verifier.validate_journey(
            journey, self.contract["required_journeys"][0], self.contract
        )
        self.assert_error(errors, "required trace stores are absent")

    def test_final_fact_trace_mismatch_is_rejected(self) -> None:
        journey = self.verified_journey()
        journey["cross_storage_trace"]["final_fact"]["trace_id"] = "other-trace"
        errors = verifier.validate_journey(
            journey, self.contract["required_journeys"][0], self.contract
        )
        self.assert_error(errors, "final fact trace identity mismatch")

    def test_source_evidence_hash_drift_is_rejected(self) -> None:
        contract = copy.deepcopy(self.contract)
        contract["source_evidence"]["n012_alert_evidence"]["sha256"] = "0" * 64
        self.assert_error(self.validate(contract=contract), "source evidence hash drifted")

    def test_kubernetes_config_hash_drift_is_rejected(self) -> None:
        contract = copy.deepcopy(self.contract)
        contract["candidate_binding"]["config_sha256"] = "0" * 64
        self.assert_error(
            self.validate(contract=contract), "candidate Kubernetes config hash drifted"
        )

    def test_current_kubernetes_evidence_passes(self) -> None:
        errors, summary = verifier.validate_manifest(
            verifier.ROOT, self.contract, self.manifest
        )
        self.assertEqual([], errors)
        evidence = verifier.load_json(verifier.ROOT / verifier.KUBERNETES_RELATIVE)
        self.assertEqual(
            [],
            verifier.validate_kubernetes_evidence(
                verifier.ROOT, self.contract, summary, evidence
            ),
        )

    def test_shared_storage_touch_is_rejected(self) -> None:
        _, summary = verifier.validate_manifest(
            verifier.ROOT, self.contract, self.manifest
        )
        evidence = verifier.load_json(verifier.ROOT / verifier.KUBERNETES_RELATIVE)
        evidence["shared_kafka_touched"] = True
        self.assert_error(
            verifier.validate_kubernetes_evidence(
                verifier.ROOT, self.contract, summary, evidence
            ),
            "shared_kafka_touched",
        )

    def test_kubernetes_validation_overclaim_is_rejected(self) -> None:
        _, summary = verifier.validate_manifest(
            verifier.ROOT, self.contract, self.manifest
        )
        evidence = verifier.load_json(verifier.ROOT / verifier.KUBERNETES_RELATIVE)
        evidence["validation"]["promotion_eligible"] = True
        self.assert_error(
            verifier.validate_kubernetes_evidence(
                verifier.ROOT, self.contract, summary, evidence
            ),
            "does not match current aggregation",
        )


if __name__ == "__main__":
    unittest.main()
