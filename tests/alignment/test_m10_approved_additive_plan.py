from __future__ import annotations

import copy
import unittest

from scripts.alignment import build_m10_approved_additive_plan as builder
from scripts.alignment import guard_m10_approved_additive_apply as guard
from scripts.alignment import verify_m10_approved_additive_plan as verifier


class M10ApprovedAdditivePlanMutationTests(unittest.TestCase):
    @classmethod
    def setUpClass(cls) -> None:
        cls.expected = builder.build(builder.ROOT)
        cls.actual = verifier.load(verifier.OUTPUT)

    def validate(self, actual=None) -> list[str]:
        return verifier.validate(self.expected, actual or self.actual)

    def assert_error(self, actual: dict, text: str) -> None:
        errors = self.validate(actual)
        self.assertTrue(any(text in item for item in errors), errors)

    def test_current_blocked_plan_passes(self) -> None:
        self.assertEqual([], self.validate())

    def test_current_apply_guard_blocks_before_mutation(self) -> None:
        decision = guard.evaluate_apply_authorization(builder.ROOT, self.actual)
        self.assertEqual("BLOCKED", decision["decision"])
        self.assertFalse(decision["mutating_client_started"])
        self.assertFalse(decision["shared_infrastructure_touched"])

    def test_sql_comment_does_not_trigger_drop_detection(self) -> None:
        self.assertEqual([], builder.destructive_findings("postgres_migration", "-- DROP TABLE x\nCREATE TABLE x(id int);"))

    def test_drop_statement_is_rejected(self) -> None:
        self.assertEqual(["DROP_STATEMENT"], builder.destructive_findings("postgres_migration", "DROP TABLE x;"))

    def test_artifact_hash_tamper_is_rejected(self) -> None:
        actual = copy.deepcopy(self.actual)
        actual["artifacts"][1]["sha256"] = "0" * 64
        self.assert_error(actual, "deterministic builder output")

    def test_artifact_reorder_is_rejected(self) -> None:
        actual = copy.deepcopy(self.actual)
        actual["artifacts"][0], actual["artifacts"][1] = actual["artifacts"][1], actual["artifacts"][0]
        self.assert_error(actual, "artifact identity/order drifted")

    def test_false_authorization_is_rejected(self) -> None:
        actual = copy.deepcopy(self.actual)
        actual["status"] = "AUTHORIZED"
        actual["apply_allowed"] = True
        self.assert_error(actual, "authorized plan retains blockers")

    def test_guard_rejects_false_authorization(self) -> None:
        actual = copy.deepcopy(self.actual)
        actual["status"] = "AUTHORIZED"
        actual["apply_allowed"] = True
        actual["blocking_codes"] = []
        decision = guard.evaluate_apply_authorization(builder.ROOT, actual)
        self.assertEqual("BLOCKED", decision["decision"])
        self.assertIn("plan does not equal deterministic builder output", decision["reasons"])

    def test_candidate_blocker_removal_is_rejected(self) -> None:
        actual = copy.deepcopy(self.actual)
        actual["blocking_codes"].remove("DEPLOYABLE_CANDIDATE_REQUIRED")
        self.assert_error(actual, "missing deployable-candidate blocker was removed")

    def test_preflight_blocker_removal_is_rejected(self) -> None:
        actual = copy.deepcopy(self.actual)
        actual["blocking_codes"].remove("N004_PREFLIGHT_G6_PASS_REQUIRED")
        self.assert_error(actual, "failed N004/G6 blocker was removed")

    def test_non_additive_blocker_removal_is_rejected(self) -> None:
        actual = copy.deepcopy(self.actual)
        actual["blocking_codes"].remove("NON_ADDITIVE_ARTIFACT_PRESENT")
        self.assert_error(actual, "non-additive artifact blocker was removed")

    def test_replay_policy_drift_is_rejected(self) -> None:
        actual = copy.deepcopy(self.actual)
        actual["replay_policy"]["on_hash_mismatch"] = "CONTINUE"
        self.assert_error(actual, "exact-hash replay policy drifted")

    def test_half_failure_policy_drift_is_rejected(self) -> None:
        actual = copy.deepcopy(self.actual)
        actual["half_failure_policy"]["automatic_destructive_rollback"] = True
        self.assert_error(actual, "half-failure recovery policy drifted")

    def test_legacy_write_removal_is_rejected(self) -> None:
        actual = copy.deepcopy(self.actual)
        actual["compatibility_policy"]["legacy_write"] = "DISABLE"
        self.assert_error(actual, "legacy compatibility policy drifted")

    def test_production_overclaim_is_rejected(self) -> None:
        actual = copy.deepcopy(self.actual)
        actual["production_applied"] = True
        self.assert_error(actual, "falsely claims production application")


if __name__ == "__main__":
    unittest.main()
