import copy
import importlib.util
import unittest
from pathlib import Path


ROOT = Path(__file__).resolve().parents[2]
SPEC = importlib.util.spec_from_file_location(
    "verify_m09_page_state_accessibility",
    ROOT / "scripts/alignment/verify_m09_page_state_accessibility.py",
)
VERIFY = importlib.util.module_from_spec(SPEC)
assert SPEC.loader is not None
SPEC.loader.exec_module(VERIFY)


class M09PageStateAccessibilityVerifierMutationTests(unittest.TestCase):
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

    def test_conflict_collapse_is_detected(self):
        texts = dict(self.texts)
        path = "web/ui/src/components/pageState.ts"
        texts[path] = texts[path].replace("responseStatus(error) === 409", "false")
        self.assertTrue(any("409" in error for error in self.validate(texts=texts)))

    def test_unavailable_fallback_leak_is_detected(self):
        texts = dict(self.texts)
        path = "web/ui/src/components/PageStateBoundary.tsx"
        texts[path] = texts[path].replace("state === 'partial' ? children : null", "children")
        self.assertTrue(any("partial" in error for error in self.validate(texts=texts)))

    def test_focus_return_guard_removal_is_detected(self):
        texts = dict(self.texts)
        path = "web/ui/src/components/useDrawerFocusReturn.ts"
        texts[path] = texts[path].replace("target?.isConnected", "target")
        self.assertTrue(any("isConnected" in error for error in self.validate(texts=texts)))

    def test_drawer_adoption_removal_is_detected(self):
        texts = dict(self.texts)
        path = "web/ui/src/pages/CampaignWorkbenchPage.tsx"
        texts[path] = texts[path].replace("data-campaign-detail-initial-focus", "removed-initial-focus")
        self.assertTrue(any("initial-focus" in error for error in self.validate(texts=texts)))

    def test_1366_contract_removal_is_detected(self):
        texts = dict(self.texts)
        path = "web/ui/src/styles/page-state.css"
        texts[path] = texts[path].replace("@media (max-width: 1366px)", "@media (max-width: 1200px)")
        self.assertTrue(any("1366" in error for error in self.validate(texts=texts)))

    def test_route_wide_overclaim_is_detected(self):
        evidence = copy.deepcopy(self.evidence)
        evidence["does_not_prove"] = [item for item in evidence["does_not_prove"] if "every product page migrated" not in item]
        self.assertTrue(any("route-wide" in error for error in self.validate(evidence=evidence)))

    def test_browser_overclaim_is_detected(self):
        evidence = copy.deepcopy(self.evidence)
        evidence["browser_evidence"] = True
        self.assertTrue(any("browser_evidence=false" in error for error in self.validate(evidence=evidence)))


if __name__ == "__main__":
    unittest.main()
