import copy
import json
import sys
import unittest
from pathlib import Path

ROOT = Path(__file__).resolve().parents[2]
sys.path.insert(0, str(ROOT / "scripts/alignment"))

from verify_m06_event_time_policy import ContractError, validate_contract  # noqa: E402


class M06EventTimePolicyTest(unittest.TestCase):
    @classmethod
    def setUpClass(cls) -> None:
        cls.contract = json.loads(
            (ROOT / "contracts/events/event-time-policy.v1.json").read_text(encoding="utf-8")
        )
        cls.schema = json.loads(
            (ROOT / "contracts/events/event-time-policy.schema.json").read_text(encoding="utf-8")
        )

    def test_contract_is_strict_and_shared(self) -> None:
        result = validate_contract(self.contract, self.schema)
        self.assertEqual("PASS", result["status"])
        self.assertEqual(4, result["java_consumers"])
        self.assertEqual(
            "event_time < watermark - allowed_lateness_ms",
            self.contract["boundaries"]["late"],
        )

    def test_boundary_and_classification_mutations_fail_closed(self) -> None:
        mutations = (
            lambda value: value["boundaries"].update(
                {"late": "event_time <= watermark - allowed_lateness_ms"}
            ),
            lambda value: value["classification_order"].reverse(),
            lambda value: value["implementations"]["java_consumers"].remove(
                "flink-session-job"
            ),
        )
        for mutation in mutations:
            candidate = copy.deepcopy(self.contract)
            mutation(candidate)
            with self.subTest(candidate=candidate), self.assertRaises(ContractError):
                validate_contract(candidate, self.schema)

    def test_java_consumers_call_the_shared_policy(self) -> None:
        files = (
            "java/flink-jobs/flink-log-job/src/main/java/com/traffic/flink/log/LogJob.java",
            "java/flink-jobs/flink-session-job/src/main/java/com/traffic/flink/session/source/FlowLatenessFunction.java",
            "java/flink-jobs/flink-session-job/src/main/java/com/traffic/flink/session/processor/SessionizeProcessFunction.java",
            "java/flink-jobs/flink-feature-job/src/main/java/com/traffic/flink/feature/processor/FeatureProcessFunctionV3.java",
            "java/flink-jobs/flink-user-behavior-job/src/main/java/com/traffic/flink/behavior/user/LateUserEventRouter.java",
        )
        for relative in files:
            source = (ROOT / relative).read_text(encoding="utf-8")
            with self.subTest(file=relative):
                self.assertIn("EventTimePolicy", source)
        shared = (ROOT / self.contract["implementations"]["java"]).read_text(encoding="utf-8")
        self.assertIn("eventTimeMs < saturatingSubtract(watermarkMs, allowedLatenessMs)", shared)
        self.assertNotIn("System.currentTimeMillis", shared)

    def test_go_entity_resolution_uses_explicit_as_of_boundary(self) -> None:
        policy = (ROOT / self.contract["implementations"]["go"]).read_text(encoding="utf-8")
        normalization = (
            ROOT / "go/control-plane/internal/entityresolution/normalization.go"
        ).read_text(encoding="utf-8")
        self.assertIn("func EffectiveAsOf(", policy)
        self.assertIn("func ObservedWithinAsOf(", policy)
        self.assertIn("eventtime.ObservedWithinAsOf", normalization)


if __name__ == "__main__":
    unittest.main()
