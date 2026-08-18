from __future__ import annotations

import copy
import unittest

from scripts.alignment import build_m09_integrated_bom as builder
from scripts.alignment import verify_m09_integrated_bom as verifier


class M09IntegratedBomMutationTests(unittest.TestCase):
    def setUp(self) -> None:
        self.expected = builder.build(builder.ROOT)
        self.bom = verifier.load(verifier.BOM_PATH)
        self.index = verifier.load(verifier.INDEX_PATH)
        self.pointer = verifier.load(verifier.POINTER_PATH)

    def validate(self, bom=None, index=None, pointer=None) -> list[str]:
        return verifier.validate(
            self.expected,
            bom if bom is not None else self.bom,
            index if index is not None else self.index,
            pointer if pointer is not None else self.pointer,
        )

    def assert_error(self, errors: list[str], expected: str) -> None:
        self.assertTrue(any(expected in error for error in errors), errors)

    def test_current_assembled_no_go_snapshot_passes(self) -> None:
        self.assertEqual([], self.validate())

    def test_component_omission_is_rejected(self) -> None:
        bom = copy.deepcopy(self.bom)
        bom["components"].pop()
        self.assert_error(self.validate(bom=bom), "deterministic builder output")

    def test_component_evidence_hash_drift_is_rejected(self) -> None:
        bom = copy.deepcopy(self.bom)
        bom["components"][0]["evidence"]["sha256"] = "0" * 64
        self.assert_error(self.validate(bom=bom), "deterministic builder output")

    def test_same_candidate_overclaim_is_rejected(self) -> None:
        bom = copy.deepcopy(self.bom)
        bom["candidate_id"] = "1" * 64
        bom["closure"]["same_candidate_manifest"] = True
        self.assert_error(self.validate(bom=bom), "falsely claims one same candidate")

    def test_windows_journey_overclaim_is_rejected(self) -> None:
        bom = copy.deepcopy(self.bom)
        bom["closure"]["verified_windows_journeys"] = 7
        self.assert_error(self.validate(bom=bom), "Windows journey verification")

    def test_cross_storage_trace_overclaim_is_rejected(self) -> None:
        bom = copy.deepcopy(self.bom)
        bom["closure"]["complete_cross_storage_traces"] = 7
        self.assert_error(self.validate(bom=bom), "complete cross-storage traces")

    def test_blocker_removal_is_rejected(self) -> None:
        bom = copy.deepcopy(self.bom)
        bom["blocking_codes"].remove("PRODUCTION_APPLIED_REQUIRED")
        self.assert_error(self.validate(bom=bom), "blocking code set drifted")

    def test_index_bom_hash_drift_is_rejected(self) -> None:
        index = copy.deepcopy(self.index)
        index["bom"]["sha256"] = "0" * 64
        self.assert_error(self.validate(index=index), "BOM binding mismatch")

    def test_go_pointer_is_rejected(self) -> None:
        pointer = copy.deepcopy(self.pointer)
        pointer["status"] = "GO"
        pointer["promotion_allowed"] = True
        self.assert_error(self.validate(pointer=pointer), "must remain NO_GO")

    def test_pointer_index_hash_drift_is_rejected(self) -> None:
        pointer = copy.deepcopy(self.pointer)
        pointer["evidence_index"]["sha256"] = "0" * 64
        self.assert_error(self.validate(pointer=pointer), "evidence-index binding mismatch")


if __name__ == "__main__":
    unittest.main()
