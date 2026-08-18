from __future__ import annotations

import copy
import importlib.util
import json
import unittest
from pathlib import Path


ROOT = Path(__file__).resolve().parents[2]
SCRIPT = ROOT / "scripts/alignment/validate_m03_file_restoration_golden.py"
SPEC = importlib.util.spec_from_file_location("validate_m03_file_restoration_golden", SCRIPT)
assert SPEC and SPEC.loader
MODULE = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(MODULE)


class FileRestorationGoldenValidatorTests(unittest.TestCase):
    @classmethod
    def setUpClass(cls) -> None:
        cls.corpus = json.loads(MODULE.CORPUS.read_text(encoding="utf-8"))

    def test_current_corpus_passes(self) -> None:
        MODULE.validate(copy.deepcopy(self.corpus))

    def test_exact_case_set_is_required(self) -> None:
        value = copy.deepcopy(self.corpus)
        value["cases"][0]["case_id"] = "replacement"
        with self.assertRaisesRegex(ValueError, "^FILE_RESTORE_GOLDEN_CASE_SET$"):
            MODULE.validate(value)

    def test_unsupported_object_write_is_rejected(self) -> None:
        value = copy.deepcopy(self.corpus)
        row = next(item for item in value["cases"] if item["expected"]["status"] == "unsupported")
        row["expected"]["object_policy"] = "required"
        with self.assertRaisesRegex(ValueError, "^FILE_RESTORE_GOLDEN_UNSUPPORTED_OBJECT$"):
            MODULE.validate(value)

    def test_inert_flag_is_required(self) -> None:
        value = copy.deepcopy(self.corpus)
        row = next(item for item in value["cases"] if item["stage"] == "extractor")
        row["expected"]["inert"] = False
        with self.assertRaisesRegex(ValueError, "^FILE_RESTORE_GOLDEN_INERT_REQUIRED$"):
            MODULE.validate(value)


if __name__ == "__main__":
    unittest.main()
