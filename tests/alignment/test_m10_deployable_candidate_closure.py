from __future__ import annotations

import copy
import unittest

from scripts.alignment import build_m10_deployable_candidate_closure as builder
from scripts.alignment import verify_m10_deployable_candidate_closure as verifier


class M10DeployableCandidateClosureMutationTests(unittest.TestCase):
    @classmethod
    def setUpClass(cls) -> None:
        cls.expected = builder.build(builder.ROOT)
        cls.actual = verifier.load(verifier.OUTPUT)

    def validate(self, actual=None) -> list[str]:
        return verifier.validate(self.expected, actual or self.actual)

    def assert_error(self, actual: dict, text: str) -> None:
        errors = self.validate(actual)
        self.assertTrue(any(text in item for item in errors), errors)

    def test_current_blocked_closure_passes(self) -> None:
        self.assertEqual([], self.validate())

    def test_false_frozen_candidate_is_rejected(self) -> None:
        actual = copy.deepcopy(self.actual)
        actual["status"] = "FROZEN"
        actual["candidate_id"] = "1" * 64
        self.assert_error(actual, "frozen candidate lacks complete bound inputs")

    def test_candidate_id_on_incomplete_closure_is_rejected(self) -> None:
        actual = copy.deepcopy(self.actual)
        actual["candidate_id"] = "2" * 64
        self.assert_error(actual, "falsely carries candidate identity")

    def test_dimension_omission_is_rejected(self) -> None:
        actual = copy.deepcopy(self.actual)
        actual["dimensions"].pop()
        self.assert_error(actual, "eight-dimension identity/order drifted")

    def test_dimension_reorder_is_rejected(self) -> None:
        actual = copy.deepcopy(self.actual)
        actual["dimensions"][0], actual["dimensions"][1] = actual["dimensions"][1], actual["dimensions"][0]
        self.assert_error(actual, "eight-dimension identity/order drifted")

    def test_existing_ref_without_hash_is_rejected(self) -> None:
        actual = copy.deepcopy(self.actual)
        actual["dimensions"][0]["refs"][0]["sha256"] = None
        self.assert_error(actual, "lacks exact hash")

    def test_m01_blocker_removal_is_rejected(self) -> None:
        actual = copy.deepcopy(self.actual)
        actual["blocking_codes"].remove("UPSTREAM_M01_RELEASE_POINTER_REQUIRED")
        self.assert_error(actual, "missing M01 pointer blocker was removed")

    def test_m09_blocker_removal_is_rejected(self) -> None:
        actual = copy.deepcopy(self.actual)
        actual["blocking_codes"].remove("UPSTREAM_M09_SAME_CANDIDATE_REQUIRED")
        self.assert_error(actual, "non-GO M09 candidate blocker was removed")

    def test_production_overclaim_is_rejected(self) -> None:
        actual = copy.deepcopy(self.actual)
        actual["production_applied"] = True
        self.assert_error(actual, "falsely claims production application")

    def test_unsorted_blockers_are_rejected(self) -> None:
        actual = copy.deepcopy(self.actual)
        actual["blocking_codes"] = list(reversed(actual["blocking_codes"]))
        self.assert_error(actual, "sorted unique list")


if __name__ == "__main__":
    unittest.main()
