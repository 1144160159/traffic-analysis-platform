import copy
import importlib.util
import unittest
from pathlib import Path


ROOT = Path(__file__).resolve().parents[2]
SPEC = importlib.util.spec_from_file_location(
    "verify_m09_rule_model_review",
    ROOT / "scripts/alignment/verify_m09_rule_model_review.py",
)
VERIFY = importlib.util.module_from_spec(SPEC)
assert SPEC.loader is not None
SPEC.loader.exec_module(VERIFY)


class M09RuleModelReviewVerifierMutationTests(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        cls.texts = VERIFY.load_texts()
        cls.contract = VERIFY.load_json(VERIFY.CONTRACT)
        cls.evidence = VERIFY.load_json(VERIFY.EVIDENCE)

    def validate(self, *, texts=None, contract=None, evidence=None):
        return VERIFY.validate_snapshot(
            texts or self.texts,
            contract or self.contract,
            evidence or self.evidence,
        )

    def test_current_snapshot_passes(self):
        self.assertEqual([], self.validate())

    def test_exact_event_binding_omission_is_detected(self):
        texts = dict(self.texts)
        path = "go/control-plane/internal/rules/service/deployment_runtime_gate.go"
        texts[path] = texts[path].replace("sameDeploymentRuntimeReceipt", "removedExactReceiptBinding")
        self.assertTrue(any("sameDeploymentRuntimeReceipt" in error for error in self.validate(texts=texts)))

    def test_gray_projection_gate_omission_is_detected(self):
        texts = dict(self.texts)
        path = "go/control-plane/internal/rules/service/deployment_service.go"
        texts[path] = texts[path].replace("lockedApprovedConfiguration, true", "lockedApprovedConfiguration, false")
        self.assertTrue(any("lockedApprovedConfiguration, true" in error for error in self.validate(texts=texts)))

    def test_default_on_manifest_is_detected(self):
        texts = dict(self.texts)
        path = "go/control-plane/deployments/kubernetes/rule-manager.yaml"
        at = texts[path].index("DEPLOYMENT_RUNTIME_ACK_GATE_V1_ENABLED")
        texts[path] = texts[path][:at] + texts[path][at:].replace('value: "false"', 'value: "true"', 1)
        self.assertTrue(any("default-off" in error for error in self.validate(texts=texts)))

    def test_ui_blocking_omission_is_detected(self):
        texts = dict(self.texts)
        path = "web/ui/src/pages/DeploymentManagementWorkspace.tsx"
        texts[path] = texts[path].replace("ACK 不完整，已停止灰度扩展", "removed blocking state")
        self.assertTrue(any("ACK 不完整" in error for error in self.validate(texts=texts)))

    def test_partial_ack_false_claim_is_detected(self):
        evidence = copy.deepcopy(self.evidence)
        evidence["partial_ack_stops_expansion"] = False
        self.assertTrue(any("partial_ack_stops_expansion=true" in error for error in self.validate(evidence=evidence)))

    def test_false_production_claim_is_detected(self):
        evidence = copy.deepcopy(self.evidence)
        evidence["production_applied"] = True
        self.assertTrue(any("production_applied=false" in error for error in self.validate(evidence=evidence)))


if __name__ == "__main__":
    unittest.main()
