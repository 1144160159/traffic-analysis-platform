from __future__ import annotations

import copy
import hashlib
import json
import sys
import unittest
from pathlib import Path


SCRIPT_ROOT = Path(__file__).resolve().parents[1]
REPO_ROOT = SCRIPT_ROOT.parents[1]
sys.path.insert(0, str(SCRIPT_ROOT))

from learning_episode import (  # noqa: E402
    EpisodeValidationError,
    evaluate_episode,
    load_json,
    validate_episode,
    validate_policy,
)


SPEC_ROOT = REPO_ROOT / "doc" / "04_assets" / "ui_suite_gpt_v1" / "specs"


def passing_episode() -> dict:
    policy_version = "2026-07-12.v1"
    evidence_path = "doc/04_assets/ui_suite_gpt_v1/specs/reinforcement-learning-policy.json"
    evidence_checksum = hashlib.sha256((REPO_ROOT / evidence_path).read_bytes()).hexdigest()
    gates = [
        "business_semantics",
        "functional_realization",
        "tenant_rbac_audit",
        "database_and_seed",
        "runtime_clean",
        "business_roi",
        "required_tests",
        "security_and_secrets",
        "rollout_and_rollback",
    ]
    critic = {"verdict": "pass", "findings": [], "evidence": [evidence_path]}
    return {
        "schema_version": policy_version,
        "run_id": "test-run",
        "task_id": "TEST-001",
        "subject": {"type": "page", "id": "campaigns", "route": "/campaigns", "risk": "medium"},
        "versions": {"code": "abc", "frontend_image": "ui:test", "backend_image": "api:test", "evaluator": policy_version, "policy": policy_version},
        "observation": {"business": {}, "function": {}, "visual": {}, "quality": {}, "production": {}, "data_sources": ["test"]},
        "action_candidates": [{"id": "a", "type": "function", "summary": "complete chain", "allowed_paths": ["web/ui"]}],
        "selected_action": {"id": "a", "reason": "best", "change_intent": ["close API chain"]},
        "gates": {"all_passed": True, "results": [{"id": gate, "status": "pass", "evidence": [evidence_path]} for gate in gates]},
        "reward": {
            "eligible": False,
            "total": None,
            "dimensions": {"business": 90, "function": 90, "quality": 80, "visual": 90, "performance": 80, "maintainability": 80},
            "penalties": {"regression": 0, "cost": 2, "uncertainty": 1},
            "confidence": 0.9,
        },
        "critics": {"design": copy.deepcopy(critic), "business": copy.deepcopy(critic), "quality": copy.deepcopy(critic), "performance": copy.deepcopy(critic)},
        "main_thread_decision": {"decision": "accept", "finding_decisions": [], "reason": "all evidence passed"},
        "production": {"status": "stable", "stable_window": "30m", "rollback_verified": True},
        "learning_status": "ineligible",
        "evidence": [{"type": "test", "path": evidence_path, "checksum": evidence_checksum}],
    }


class LearningEpisodeTest(unittest.TestCase):
    @classmethod
    def setUpClass(cls) -> None:
        cls.policy = load_json(SPEC_ROOT / "reinforcement-learning-policy.json")
        cls.schema = load_json(SPEC_ROOT / "learning-episode.schema.json")

    def test_valid_episode_becomes_positive(self) -> None:
        episode = passing_episode()
        validate_policy(self.policy)
        validate_episode(episode, self.schema, self.policy)
        result = evaluate_episode(episode, self.policy)
        self.assertTrue(result["reward"]["eligible"])
        self.assertEqual(result["reward"]["total"], 83.0)
        self.assertEqual(result["learning_status"], "positive")
        validate_episode(result, self.schema, self.policy)

    def test_failed_hard_gate_cannot_be_compensated_by_reward(self) -> None:
        episode = passing_episode()
        episode["gates"]["results"][0]["status"] = "fail"
        episode["reward"]["dimensions"] = {key: 100 for key in episode["reward"]["dimensions"]}
        result = evaluate_episode(episode, self.policy)
        self.assertFalse(result["reward"]["eligible"])
        self.assertIsNone(result["reward"]["total"])
        self.assertEqual(result["learning_status"], "negative")

    def test_schema_rejects_unknown_property(self) -> None:
        episode = passing_episode()
        episode["unexpected"] = True
        with self.assertRaises(EpisodeValidationError):
            validate_episode(episode, self.schema, self.policy)

    def test_policy_rejects_unfrozen_weight_sum(self) -> None:
        policy = json.loads(json.dumps(self.policy))
        policy["reward"]["weights"]["business"] = 0.5
        with self.assertRaises(EpisodeValidationError):
            validate_policy(policy)

    def test_schema_rejects_duplicate_gate_ids(self) -> None:
        episode = passing_episode()
        episode["gates"]["results"].append(copy.deepcopy(episode["gates"]["results"][0]))
        with self.assertRaises(EpisodeValidationError):
            validate_episode(episode, self.schema, self.policy)

    def test_schema_rejects_tampered_evidence_checksum(self) -> None:
        episode = passing_episode()
        episode["evidence"][0]["checksum"] = "0" * 64
        with self.assertRaises(EpisodeValidationError):
            validate_episode(episode, self.schema, self.policy)


if __name__ == "__main__":
    unittest.main()
