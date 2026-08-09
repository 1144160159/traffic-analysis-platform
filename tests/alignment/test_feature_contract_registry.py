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

    def test_checked_in_registry_has_complete_standard_scope_and_explicit_backlog(self) -> None:
        result = verifier.verify()
        self.assertEqual("PASS", result["status"], result["errors"])
        self.assertEqual("STANDARD_SCOPE_COMPLETE_BACKLOG_PARTIAL", result["contract_coverage"])
        self.assertEqual(54, result["coverage"]["canonical_feature_ids"])
        self.assertEqual(38, result["coverage"]["formal_contracts"])
        self.assertEqual(38, result["coverage"]["standard_scope_features"])
        self.assertEqual([], result["coverage"]["missing_standard_scope_contracts"])
        self.assertEqual(16, len(result["coverage"]["missing_backlog_contracts"]))
        self.assertEqual([], result["coverage"]["non_draft_openapi_binding_gaps"])

    def test_alert_and_probe_contracts_bind_exactly_to_openapi(self) -> None:
        registry = build_registry()
        by_id = {item["feature_id"]: item for item in registry["features"]}
        for feature_id in ("F-ALERT-003", "F-ALERT-005", "F-ALERT-006", "F-AUDIT-001", "F-PROBE-001"):
            self.assertEqual(
                "EXACT",
                by_id[feature_id]["formal_contract"]["openapi_binding_status"],
                feature_id,
            )
        self.assertEqual(
            "PROFILED",
            by_id["F-COMMON-003"]["formal_contract"]["openapi_binding_status"],
        )

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

    def test_standard_scope_p1_contract_cannot_be_removed(self) -> None:
        registry = copy.deepcopy(build_registry())
        standard_p1 = next(
            item
            for item in registry["features"]
            if item["priority"] == "P1" and item["standard_24w_scope"]
        )
        standard_p1["formal_contract_present"] = False
        standard_p1["formal_contract"] = None
        standard_p1["blocking_gaps"] = ["versioned_feature_contract_missing"]
        result = self._verify_mutation(registry)
        self.assertEqual("FAIL", result["status"])
        self.assertTrue(any("all 38 standard-scope" in error for error in result["errors"]))

    def test_backlog_gap_count_cannot_be_hidden(self) -> None:
        registry = copy.deepcopy(build_registry())
        registry["coverage"]["missing_backlog_contracts"] = registry["coverage"][
            "missing_backlog_contracts"
        ][1:]
        result = self._verify_mutation(registry)
        self.assertEqual("FAIL", result["status"])
        self.assertTrue(any("backlog contract gaps" in error for error in result["errors"]))

    def test_formal_contract_validation_error_fails_closed(self) -> None:
        registry = copy.deepcopy(build_registry())
        formal = next(item for item in registry["features"] if item["formal_contract_present"])
        formal["formal_contract"]["validation_errors"] = ["permissions missing"]
        result = self._verify_mutation(registry)
        self.assertEqual("FAIL", result["status"])
        self.assertTrue(any("validation errors" in error for error in result["errors"]))

    def test_non_draft_openapi_binding_regression_fails_closed(self) -> None:
        registry = copy.deepcopy(build_registry())
        audit = next(item for item in registry["features"] if item["feature_id"] == "F-AUDIT-001")
        audit["formal_contract"]["openapi_binding_status"] = "MISSING"
        audit["formal_contract"]["openapi_bound_operation_id"] = None
        audit["formal_contract"]["openapi_bound_feature_id"] = None
        audit["blocking_gaps"].append("openapi_operation_binding_missing")
        registry["coverage"]["formal_contracts_openapi_bound"] -= 1
        registry["coverage"]["non_draft_openapi_binding_gaps"] = ["F-AUDIT-001"]
        result = self._verify_mutation(registry)
        self.assertEqual("FAIL", result["status"])
        self.assertTrue(any("binding gaps changed" in error for error in result["errors"]))

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
