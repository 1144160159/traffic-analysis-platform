from __future__ import annotations

import copy
import json
import sys
import tempfile
import unittest
from pathlib import Path
from unittest import mock


ROOT = Path(__file__).resolve().parents[2]
SCRIPTS = ROOT / "scripts/alignment"
if str(SCRIPTS) not in sys.path:
    sys.path.insert(0, str(SCRIPTS))

from build_feature_contract_registry import build_registry  # noqa: E402
import verify_feature_contract_registry as verifier  # noqa: E402


class FeatureContractRegistryTests(unittest.TestCase):
    def _verify_mutation(self, registry: dict) -> dict:
        content = dict(registry)
        content.pop("catalog_sha256", None)
        registry["catalog_sha256"] = verifier._canonical_sha256(content)
        with tempfile.TemporaryDirectory(prefix="feature-contract-registry-") as directory:
            output = Path(directory) / "registry.json"
            output.write_text(json.dumps(registry), encoding="utf-8")
            with mock.patch.object(verifier, "OUTPUT", output), mock.patch.object(
                verifier, "build_registry", return_value=registry
            ):
                return verifier.verify()

    def test_checked_in_registry_is_current_and_explicitly_partial(self) -> None:
        result = verifier.verify()
        self.assertEqual("PASS", result["status"], result["errors"])
        self.assertEqual("PARTIAL", result["contract_coverage"])
        self.assertEqual(54, result["coverage"]["canonical_feature_ids"])
        self.assertEqual(27, result["coverage"]["formal_contracts"])
        self.assertEqual(38, result["coverage"]["standard_scope_features"])
        self.assertEqual(11, len(result["coverage"]["missing_standard_scope_contracts"]))

    def test_canonical_feature_cannot_be_hidden(self) -> None:
        registry = copy.deepcopy(build_registry())
        registry["features"] = registry["features"][:-1]
        result = self._verify_mutation(registry)
        self.assertEqual("FAIL", result["status"])
        self.assertTrue(any("54 unique" in error for error in result["errors"]))

    def test_owner_cannot_be_removed(self) -> None:
        registry = copy.deepcopy(build_registry())
        registry["features"][0]["accountable"] = ""
        result = self._verify_mutation(registry)
        self.assertEqual("FAIL", result["status"])
        self.assertTrue(any("accountable owner" in error for error in result["errors"]))

    def test_missing_contract_gap_cannot_be_hidden(self) -> None:
        registry = copy.deepcopy(build_registry())
        missing = next(item for item in registry["features"] if not item["formal_contract_present"])
        missing["blocking_gaps"] = []
        result = self._verify_mutation(registry)
        self.assertEqual("FAIL", result["status"])
        self.assertTrue(any("gap was hidden" in error for error in result["errors"]))

    def test_p0_contract_cannot_be_removed(self) -> None:
        registry = copy.deepcopy(build_registry())
        p0 = next(item for item in registry["features"] if item["priority"] == "P0")
        p0["formal_contract_present"] = False
        p0["formal_contract"] = None
        p0["blocking_gaps"] = ["versioned_feature_contract_missing"]
        result = self._verify_mutation(registry)
        self.assertEqual("FAIL", result["status"])
        self.assertTrue(any("all P0" in error for error in result["errors"]))

    def test_formal_contract_validation_error_fails_closed(self) -> None:
        registry = copy.deepcopy(build_registry())
        formal = next(item for item in registry["features"] if item["formal_contract_present"])
        formal["formal_contract"]["validation_errors"] = ["permissions missing"]
        result = self._verify_mutation(registry)
        self.assertEqual("FAIL", result["status"])
        self.assertTrue(any("validation errors" in error for error in result["errors"]))

    def test_duplicate_operation_id_fails_closed(self) -> None:
        registry = copy.deepcopy(build_registry())
        registry["integrity"]["duplicate_operation_ids"] = {"duplicate": ["F-A", "F-B"]}
        result = self._verify_mutation(registry)
        self.assertEqual("FAIL", result["status"])
        self.assertTrue(any("operation_id" in error for error in result["errors"]))

    def test_compatibility_removal_cannot_be_enabled(self) -> None:
        registry = copy.deepcopy(build_registry())
        registry["policy"]["compatibility_removal_allowed_in_current_program"] = True
        result = self._verify_mutation(registry)
        self.assertEqual("FAIL", result["status"])
        self.assertTrue(any("compatibility removals" in error for error in result["errors"]))

    def test_registry_cannot_become_runtime_dependency(self) -> None:
        registry = copy.deepcopy(build_registry())
        registry["production_runtime_dependency"] = True
        result = self._verify_mutation(registry)
        self.assertEqual("FAIL", result["status"])
        self.assertTrue(any("runtime dependency" in error for error in result["errors"]))


if __name__ == "__main__":
    unittest.main()
