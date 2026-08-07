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

from build_dr_recovery_catalog import build_catalog  # noqa: E402
import verify_dr_recovery_catalog as verifier  # noqa: E402


class DRRecoveryCatalogTests(unittest.TestCase):
    def _verify_mutation(self, catalog: dict) -> dict:
        content = dict(catalog)
        content.pop("catalog_sha256", None)
        catalog["catalog_sha256"] = verifier._canonical_sha256(content)
        with tempfile.TemporaryDirectory(prefix="dr-recovery-catalog-") as directory:
            output = Path(directory) / "catalog.json"
            output.write_text(json.dumps(catalog), encoding="utf-8")
            with mock.patch.object(verifier, "OUTPUT", output), mock.patch.object(
                verifier, "build_catalog", return_value=catalog
            ):
                return verifier.verify()

    def test_checked_in_catalog_is_current_and_explicitly_partial(self) -> None:
        result = verifier.verify()
        self.assertEqual("PASS", result["status"], result["errors"])
        self.assertEqual("PARTIAL", result["dr_readiness"])
        self.assertEqual(8, result["counts"]["domains"])
        self.assertEqual(8, result["counts"]["domains_without_restore_evidence"])

    def test_domain_cannot_be_hidden(self) -> None:
        catalog = copy.deepcopy(build_catalog())
        catalog["domains"] = catalog["domains"][:-1]
        result = self._verify_mutation(catalog)
        self.assertEqual("FAIL", result["status"])
        self.assertTrue(any("domain inventory" in error for error in result["errors"]))

    def test_domain_cannot_claim_pass_without_isolated_restore(self) -> None:
        catalog = copy.deepcopy(build_catalog())
        catalog["domains"][0]["repository_state"] = "PASS"
        result = self._verify_mutation(catalog)
        self.assertEqual("FAIL", result["status"])
        self.assertTrue(any("claim PASS" in error for error in result["errors"]))

    def test_backup_success_cannot_substitute_for_restore(self) -> None:
        catalog = copy.deepcopy(build_catalog())
        catalog["policy"]["backup_success_is_restore_success"] = True
        result = self._verify_mutation(catalog)
        self.assertEqual("FAIL", result["status"])
        self.assertTrue(any("backup success" in error for error in result["errors"]))

    def test_restore_evidence_cannot_be_invented(self) -> None:
        catalog = copy.deepcopy(build_catalog())
        catalog["domains"][0]["restore"]["last_successful_evidence"] = "fake.json"
        result = self._verify_mutation(catalog)
        self.assertEqual("FAIL", result["status"])
        self.assertTrue(any("invent successful restore" in error for error in result["errors"]))

    def test_postgres_fencing_and_pitr_gaps_cannot_be_hidden(self) -> None:
        catalog = copy.deepcopy(build_catalog())
        pg = next(item for item in catalog["domains"] if item["domain_id"] == "postgresql_authority")
        pg["blocking_gaps"].remove("isolated_pitr_restore_missing")
        pg["fencing"]["old_primary_fenced_before_endpoint_publication"] = True
        result = self._verify_mutation(catalog)
        self.assertEqual("FAIL", result["status"])
        self.assertTrue(any("fencing gap" in error for error in result["errors"]))
        self.assertTrue(any("PITR gap" in error for error in result["errors"]))

    def test_clickhouse_failure_domain_gap_cannot_be_hidden(self) -> None:
        catalog = copy.deepcopy(build_catalog())
        ch = next(item for item in catalog["domains"] if item["domain_id"] == "clickhouse_facts")
        ch["blocking_gaps"].remove("failure_domain_proof_missing")
        result = self._verify_mutation(catalog)
        self.assertEqual("FAIL", result["status"])
        self.assertTrue(any("ClickHouse failure-domain" in error for error in result["errors"]))

    def test_cross_store_recovery_order_cannot_be_reversed(self) -> None:
        catalog = copy.deepcopy(build_catalog())
        catalog["recovery_order"][1], catalog["recovery_order"][2] = (
            catalog["recovery_order"][2],
            catalog["recovery_order"][1],
        )
        result = self._verify_mutation(catalog)
        self.assertEqual("FAIL", result["status"])
        self.assertTrue(any("recovery order" in error for error in result["errors"]))

    def test_destructive_execution_cannot_be_auto_authorized(self) -> None:
        catalog = copy.deepcopy(build_catalog())
        catalog["destructive_execution_authorized"] = True
        result = self._verify_mutation(catalog)
        self.assertEqual("FAIL", result["status"])
        self.assertTrue(any("destructive DR" in error for error in result["errors"]))


if __name__ == "__main__":
    unittest.main()
