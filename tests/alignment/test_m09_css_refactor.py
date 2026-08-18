from __future__ import annotations

import copy
import json
import unittest

from scripts.alignment import verify_m09_css_refactor as verifier


class M09CssRefactorVerifierMutationTests(unittest.TestCase):
    def setUp(self) -> None:
        self.texts = verifier.load_texts()
        self.contract = json.loads(verifier.CONTRACT_PATH.read_text(encoding="utf-8"))
        self.evidence = json.loads(verifier.EVIDENCE_PATH.read_text(encoding="utf-8"))

    def validate(self, texts=None, contract=None, evidence=None) -> list[str]:
        return verifier.validate_snapshot(
            texts if texts is not None else self.texts,
            contract if contract is not None else self.contract,
            evidence if evidence is not None else self.evidence,
        )

    def assert_error(self, errors: list[str], expected: str) -> None:
        self.assertTrue(any(expected in error for error in errors), errors)

    def test_current_snapshot_passes(self) -> None:
        self.assertEqual([], self.validate())

    def test_old_selector_reintroduction_is_detected(self) -> None:
        texts = dict(self.texts)
        texts["web/ui/src/styles/pages.css"] += "\n.taf-alert-detail-response { display: grid; }\n"
        self.assert_error(self.validate(texts=texts), "moved selector remains")

    def test_import_order_drift_is_detected(self) -> None:
        texts = dict(self.texts)
        main = texts["web/ui/src/main.tsx"]
        main = main.replace("import '@/styles/pages.css';\nimport '@/styles/alert-detail.css';", "import '@/styles/alert-detail.css';\nimport '@/styles/pages.css';")
        texts["web/ui/src/main.tsx"] = main
        self.assert_error(self.validate(texts=texts), "must load after")

    def test_media_query_drift_is_detected(self) -> None:
        texts = dict(self.texts)
        texts["web/ui/src/styles/alert-detail.css"] = texts["web/ui/src/styles/alert-detail.css"].replace("@media (max-width: 1440px)", "@media (min-width: 1441px)")
        self.assert_error(self.validate(texts=texts), "@media (max-width: 1440px)")

    def test_viewport_collapse_is_detected(self) -> None:
        evidence = copy.deepcopy(self.evidence)
        evidence["viewports"] = ["1366x900"]
        self.assert_error(self.validate(evidence=evidence), "two exact declared viewports")

    def test_screenshot_hash_mismatch_is_detected(self) -> None:
        evidence = copy.deepcopy(self.evidence)
        evidence["visual_result"]["results"][0]["candidate"]["screenshot_sha256"] = "0" * 64
        self.assert_error(self.validate(evidence=evidence), "screenshot hashes differ")

    def test_false_production_claim_is_detected(self) -> None:
        evidence = copy.deepcopy(self.evidence)
        evidence["production_applied"] = True
        self.assert_error(self.validate(evidence=evidence), "production_applied")

    def test_route_wide_overclaim_is_detected(self) -> None:
        contract = copy.deepcopy(self.contract)
        contract["scope"]["route_wide_extraction_complete"] = True
        self.assert_error(self.validate(contract=contract), "must not overclaim")

    def test_viewport_api_regression_is_detected(self) -> None:
        texts = dict(self.texts)
        texts["web/ui/deployments/css-visual-diff.mjs"] = texts["web/ui/deployments/css-visual-diff.mjs"].replace("browser.newPage({ viewport, deviceScaleFactor: 1 })", "browser.newPage({ viewportSize: viewport, deviceScaleFactor: 1 })")
        self.assert_error(self.validate(texts=texts), "browser.newPage")


if __name__ == "__main__":
    unittest.main()
