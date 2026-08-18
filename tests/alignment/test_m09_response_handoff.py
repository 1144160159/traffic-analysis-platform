import copy
import importlib.util
import unittest
from pathlib import Path


ROOT = Path(__file__).resolve().parents[2]
SPEC = importlib.util.spec_from_file_location(
    "verify_m09_response_handoff",
    ROOT / "scripts/alignment/verify_m09_response_handoff.py",
)
VERIFY = importlib.util.module_from_spec(SPEC)
assert SPEC.loader is not None
SPEC.loader.exec_module(VERIFY)


class M09ResponseHandoffVerifierMutationTests(unittest.TestCase):
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

    def test_tenant_bound_receipt_lookup_omission_is_detected(self):
        texts = dict(self.texts)
        path = "go/control-plane/internal/alert/api/handler_alert_actions.go"
        texts[path] = texts[path].replace(
            "WHERE tenant_id=$1 AND alert_id=$2 AND job_id=$3",
            "WHERE job_id=$3",
        )
        self.assertTrue(any("tenant_id" in error for error in self.validate(texts=texts)))

    def test_fail_closed_executor_omission_is_detected(self):
        texts = dict(self.texts)
        path = "go/control-plane/internal/alert/consumer/alert_response_event_consumer.go"
        texts[path] = texts[path].replace('"blocked_external_executor", "unconfigured"', '"completed", "unconfigured"')
        self.assertTrue(any("unconfigured" in error for error in self.validate(texts=texts)))

    def test_default_on_manifest_is_detected(self):
        texts = dict(self.texts)
        path = "go/control-plane/deployments/kubernetes/alert-service.yaml"
        at = texts[path].index("ALERT_RESPONSE_EXTERNAL_EXECUTOR_V1_ENABLED")
        texts[path] = texts[path][:at] + texts[path][at:].replace('value: "false"', 'value: "true"', 1)
        self.assertTrue(any("default-off" in error for error in self.validate(texts=texts)))

    def test_receipt_ui_omission_is_detected(self):
        texts = dict(self.texts)
        path = "web/ui/src/pages/AlertDetailPage.tsx"
        texts[path] = texts[path].replace("alert-response-provider-receipt", "removed-provider-receipt")
        self.assertTrue(any("provider-receipt" in error for error in self.validate(texts=texts)))

    def test_direct_effect_boundary_drift_is_detected(self):
        contract = copy.deepcopy(self.contract)
        contract["topic_boundary"]["not_owned"].remove("direct production blackhole routing")
        self.assertTrue(any("direct-effect boundary" in error for error in self.validate(contract=contract)))

    def test_missing_blocked_oracle_is_detected(self):
        evidence = copy.deepcopy(self.evidence)
        evidence["postgres_oracle"]["blocked"] = 0
        self.assertTrue(any("positive blocked" in error for error in self.validate(evidence=evidence)))

    def test_false_production_claim_is_detected(self):
        evidence = copy.deepcopy(self.evidence)
        evidence["production_applied"] = True
        self.assertTrue(any("production_applied=false" in error for error in self.validate(evidence=evidence)))


if __name__ == "__main__":
    unittest.main()
