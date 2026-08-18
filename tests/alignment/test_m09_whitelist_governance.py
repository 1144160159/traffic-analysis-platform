import copy
import importlib.util
import unittest
from pathlib import Path


ROOT = Path(__file__).resolve().parents[2]
SPEC = importlib.util.spec_from_file_location(
    "verify_m09_whitelist_governance",
    ROOT / "scripts/alignment/verify_m09_whitelist_governance.py",
)
VERIFY = importlib.util.module_from_spec(SPEC)
assert SPEC.loader is not None
SPEC.loader.exec_module(VERIFY)


class M09WhitelistGovernanceVerifierMutationTests(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        cls.texts = VERIFY.load_texts()
        cls.contract = VERIFY.load_json(VERIFY.CONTRACT)
        cls.feature = VERIFY.load_json(VERIFY.FEATURE)
        cls.evidence = VERIFY.load_json(VERIFY.EVIDENCE)
        cls.topics = VERIFY.load_json(VERIFY.TOPICS)
        cls.schema = VERIFY.load_json(VERIFY.SCHEMA)
        cls.openapi = VERIFY.load_json(VERIFY.OPENAPI)

    def validate(self, *, texts=None, evidence=None, topics=None):
        return VERIFY.validate_snapshot(
            texts or self.texts, self.contract, self.feature, evidence or self.evidence,
            topics or self.topics, self.schema, self.openapi,
        )

    def test_current_snapshot_passes(self):
        self.assertEqual([], self.validate())

    def test_missing_projection_join_is_detected(self):
        texts = dict(self.texts)
        path = "go/control-plane/internal/alert/whitelist/producer_readiness.go"
        texts[path] = texts[path].replace("JOIN whitelist_rule_projection", "JOIN removed_projection")
        self.assertTrue(any("JOIN whitelist_rule_projection" in error for error in self.validate(texts=texts)))

    def test_deprecated_flag_authority_is_detected(self):
        texts = dict(self.texts)
        path = "go/control-plane/cmd/alert-service/main.go"
        texts[path] += "\nvar _ = cfg.Kafka.WhitelistEventPipelineEnabled\n"
        self.assertTrue(any("deprecated combined" in error for error in self.validate(texts=texts)))

    def test_ui_ack_omission_is_detected(self):
        texts = dict(self.texts)
        path = "web/ui/src/services/whitelistGovernanceApi.ts"
        texts[path] = texts[path].replace("rule_ack_event_id", "removed_ack_event_id")
        self.assertTrue(any("rule_ack_event_id" in error for error in self.validate(texts=texts)))

    def test_network_action_is_detected(self):
        texts = dict(self.texts)
        path = "go/control-plane/internal/alert/whitelist/command_atomic.go"
        texts[path] += '\n// exec.Command("iptables", "-F")\n'
        self.assertTrue(any("forbidden network action" in error for error in self.validate(texts=texts)))

    def test_false_production_claim_is_detected(self):
        evidence = copy.deepcopy(self.evidence)
        evidence["production_applied"] = True
        self.assertTrue(any("production_applied=false" in error for error in self.validate(evidence=evidence)))

    def test_catalog_promotion_is_detected(self):
        topics = copy.deepcopy(self.topics)
        next(item for item in topics["topics"] if item["name"] == "whitelist.events.v2")["readiness"] = "active"
        self.assertTrue(any("Kafka catalog readiness" in error for error in self.validate(topics=topics)))


if __name__ == "__main__":
    unittest.main()
