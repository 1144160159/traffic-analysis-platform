import copy
import json
import sys
import unittest
from pathlib import Path

ROOT = Path(__file__).resolve().parents[2]
sys.path.insert(0, str(ROOT / "scripts/alignment"))

from verify_m06_entity_resolution_contract import (  # noqa: E402
    ContractError,
    validate_contract,
)


CONTRACT_PATH = ROOT / "contracts/entity/entity-resolution.v1.json"
SCHEMA_PATH = ROOT / "contracts/entity/entity-resolution.schema.json"
GO_PACKAGE = ROOT / "go/control-plane/internal/entityresolution"


class M06EntityResolutionContractTest(unittest.TestCase):
    @classmethod
    def setUpClass(cls) -> None:
        cls.contract = json.loads(CONTRACT_PATH.read_text(encoding="utf-8"))
        cls.schema = json.loads(SCHEMA_PATH.read_text(encoding="utf-8"))

    def test_frozen_contract_validates_and_remains_default_off(self) -> None:
        result = validate_contract(self.contract, self.schema)
        self.assertEqual("PASS", result["status"])
        self.assertEqual("frozen_default_off", self.contract["status"])
        self.assertFalse(self.contract["rollout"]["default_enabled"])
        self.assertFalse(self.contract["rollout"]["writes_external_store"])
        self.assertEqual(
            "reject", self.contract["tenant_boundary"]["cross_tenant_action"]
        )

    def test_all_six_identifiers_have_one_rule_and_fixed_point_confidence(self) -> None:
        rules = self.contract["identifier_rules"]
        by_identifier = {rule["identifier"]: rule for rule in rules}
        self.assertEqual(
            {"asset_id", "user_id", "ip", "mac", "probe_id", "community_id"},
            set(by_identifier),
        )
        self.assertTrue(
            all(isinstance(rule["confidence_ppm"], int) for rule in rules)
        )
        self.assertFalse(by_identifier["ip"]["may_create_entity"])
        self.assertFalse(by_identifier["mac"]["may_create_entity"])
        self.assertFalse(by_identifier["community_id"]["may_create_entity"])
        self.assertEqual("correlation", by_identifier["community_id"]["scope"])
        self.assertTrue(
            all(
                rule["ambiguity_action"]
                == "preserve_candidates_without_merge"
                for rule in rules
            )
        )

    def test_source_rail_identifier_allowlists_are_exact_and_tenant_scoped(self) -> None:
        rails = {item["rail"]: item for item in self.contract["source_rails"]}
        self.assertEqual(
            {
                "flow",
                "asset_authority",
                "asset_binding",
                "device_log",
                "user_behavior",
                "probe_ingest",
            },
            set(rails),
        )
        self.assertEqual(["ip"], rails["device_log"]["allowed_identifiers"])
        self.assertEqual([], rails["device_log"]["anchor_authority"])
        self.assertEqual(["user"], rails["user_behavior"]["anchor_authority"])
        self.assertEqual(["asset"], rails["asset_authority"]["anchor_authority"])

    def test_schema_rejects_cross_tenant_merge_and_runtime_enablement(self) -> None:
        for mutate in (
            lambda value: value["tenant_boundary"].update(
                {"cross_tenant_action": "merge"}
            ),
            lambda value: value["rollout"].update({"default_enabled": True}),
            lambda value: value["identifier_rules"][3].update(
                {"ambiguity_action": "choose_highest"}
            ),
        ):
            candidate = copy.deepcopy(self.contract)
            mutate(candidate)
            with self.subTest(candidate=candidate), self.assertRaises(ContractError):
                validate_contract(candidate, self.schema)

    def test_go_projector_is_pure_deterministic_and_has_four_source_adapters(self) -> None:
        resolver = (GO_PACKAGE / "resolver.go").read_text(encoding="utf-8")
        adapters = (GO_PACKAGE / "adapters.go").read_text(encoding="utf-8")
        types = (GO_PACKAGE / "types.go").read_text(encoding="utf-8")
        self.assertIn("func Resolve(", resolver)
        self.assertIn("source tuple identity collision", resolver)
        self.assertIn("AMBIGUOUS_IP_ASSET", resolver)
        self.assertIn("MAC_IP_TARGET_CONFLICT", resolver)
        self.assertIn("DecisionSHA256", types)
        for function in (
            "ObservationFromFlow",
            "ObservationFromAsset",
            "ObservationFromMacIPBinding",
            "ObservationFromDeviceLog",
            "ObservationFromUserEvent",
        ):
            self.assertIn("func " + function + "(", adapters)
        for forbidden in ("database/sql", "kafka-go", "os.WriteFile", "http.Client"):
            self.assertNotIn(forbidden, resolver + adapters)


if __name__ == "__main__":
    unittest.main()
