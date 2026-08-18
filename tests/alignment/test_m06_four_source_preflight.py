import copy
import json
import sys
import unittest
from pathlib import Path


ROOT = Path(__file__).resolve().parents[2]
sys.path.insert(0, str(ROOT / "scripts/alignment"))

from run_m06_four_source_preflight import candidate_digest, validate_matrix  # noqa: E402


class M06FourSourcePreflightTest(unittest.TestCase):
    @classmethod
    def setUpClass(cls) -> None:
        cls.matrix = json.loads(
            (ROOT / "tests/fixtures/m06/four-source-matrix.v1.json").read_text()
        )

    def test_exact_matrix_is_bound_to_real_implementation_tests(self) -> None:
        validate_matrix(self.matrix)
        self.assertEqual(64, len(candidate_digest(self.matrix["candidate_files"])))
        for source in self.matrix["sources"].values():
            self.assertEqual(
                {"positive", "permission_negative", "bad_message", "replay"},
                {case["kind"] for case in source["cases"]},
            )

    def test_missing_permission_case_is_rejected(self) -> None:
        changed = copy.deepcopy(self.matrix)
        changed["sources"]["flow"]["cases"] = [
            case for case in changed["sources"]["flow"]["cases"]
            if case["kind"] != "permission_negative"
        ]
        with self.assertRaisesRegex(ValueError, "exact four fixture kinds"):
            validate_matrix(changed)

    def test_promotion_claim_is_rejected(self) -> None:
        changed = copy.deepcopy(self.matrix)
        changed["state"] = "promotion_evidence"
        with self.assertRaisesRegex(ValueError, "must not claim promotion evidence"):
            validate_matrix(changed)

    def test_unbound_selector_is_rejected(self) -> None:
        changed = copy.deepcopy(self.matrix)
        changed["sources"]["asset"]["cases"][0]["selector"] = "doesNotExist"
        with self.assertRaisesRegex(ValueError, "fixture selector is missing"):
            validate_matrix(changed)


if __name__ == "__main__":
    unittest.main()
