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

from build_configuration_catalog import build_catalog  # noqa: E402
import verify_configuration_catalog as verifier  # noqa: E402


class ConfigurationCatalogTests(unittest.TestCase):
    def test_checked_in_catalog_matches_all_go_flink_and_kubernetes_sources(self) -> None:
        result = verifier.verify()
        self.assertEqual("PASS", result["status"], result["errors"])
        self.assertGreaterEqual(result["counts"]["entries"], 1000)
        self.assertEqual([], result["errors"])

    def test_secret_material_is_never_serialized(self) -> None:
        catalog = build_catalog()
        secret_entries = [entry for entry in catalog["entries"] if entry["secret"]]
        self.assertGreater(len(secret_entries), 100)
        for entry in secret_entries:
            self.assertIsNone(entry["default"], entry["id"])
            self.assertFalse(entry["secret_default_nonempty"], entry["id"])
            for source in entry["sources"]:
                self.assertNotIn("value", source.get("reference") or {}, entry["id"])
                self.assertNotEqual("kubernetes_literal", source["kind"], entry["id"])

    def test_runtime_declarations_have_no_unmarked_conflicts(self) -> None:
        catalog = build_catalog()
        self.assertEqual([], catalog["conflicting_runtime_bindings"])

    def test_nonempty_secret_default_fails_closed(self) -> None:
        catalog = build_catalog()
        mutated = copy.deepcopy(catalog)
        target = next(entry for entry in mutated["entries"] if entry["secret"])
        target["secret_default_nonempty"] = True
        with tempfile.TemporaryDirectory(prefix="configuration-catalog-negative-") as directory:
            path = Path(directory) / "catalog.json"
            path.write_text(json.dumps(mutated), encoding="utf-8")
            with mock.patch.object(verifier, "OUTPUT", path), mock.patch.object(
                verifier, "build_catalog", return_value=mutated
            ):
                result = verifier.verify()
        self.assertEqual("FAIL", result["status"])
        self.assertTrue(any("non-empty secret default" in error for error in result["errors"]))

    def test_conflicting_runtime_binding_fails_closed(self) -> None:
        catalog = build_catalog()
        mutated = copy.deepcopy(catalog)
        mutated["conflicting_runtime_bindings"] = ["k8s:test/deployment/service/KEY"]
        with tempfile.TemporaryDirectory(prefix="configuration-catalog-conflict-") as directory:
            path = Path(directory) / "catalog.json"
            path.write_text(json.dumps(mutated), encoding="utf-8")
            with mock.patch.object(verifier, "OUTPUT", path), mock.patch.object(
                verifier, "build_catalog", return_value=mutated
            ):
                result = verifier.verify()
        self.assertEqual("FAIL", result["status"])
        self.assertTrue(any("conflicting declarations" in error for error in result["errors"]))


if __name__ == "__main__":
    unittest.main()
