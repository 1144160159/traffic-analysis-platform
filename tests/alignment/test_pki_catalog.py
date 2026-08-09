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

from build_pki_catalog import build_catalog  # noqa: E402
import verify_pki_catalog as verifier  # noqa: E402


class PKICatalogTests(unittest.TestCase):
    def _verify_mutation(self, catalog: dict) -> dict:
        content = dict(catalog)
        content.pop("catalog_sha256", None)
        catalog["catalog_sha256"] = verifier._canonical_sha256(content)
        with tempfile.TemporaryDirectory(prefix="pki-catalog-") as directory:
            output = Path(directory) / "catalog.json"
            output.write_text(json.dumps(catalog), encoding="utf-8")
            with mock.patch.object(verifier, "OUTPUT", output), mock.patch.object(
                verifier, "build_catalog", return_value=catalog
            ):
                return verifier.verify()

    def test_checked_in_catalog_is_current_and_explicitly_partial(self) -> None:
        result = verifier.verify()
        self.assertEqual("PASS", result["status"], result["errors"])
        self.assertEqual("PARTIAL", result["pki_compliance"])
        self.assertEqual(5, result["counts"]["certificate_domains"])
        self.assertEqual(
            result["counts"]["transport_guards"],
            result["counts"]["transport_guards_passing"],
        )

    def test_remote_transport_guard_cannot_be_disabled(self) -> None:
        catalog = copy.deepcopy(build_catalog())
        catalog["transport_guards"]["probe_remote_plaintext_rejected"] = False
        result = self._verify_mutation(catalog)
        self.assertEqual("FAIL", result["status"])
        self.assertTrue(any("transport guards" in error for error in result["errors"]))

    def test_excessive_leaf_validity_gap_cannot_be_hidden(self) -> None:
        catalog = copy.deepcopy(build_catalog())
        kafka = next(item for item in catalog["certificate_domains"] if item["domain_id"] == "kafka_tls_and_scram")
        kafka["blocking_gaps"].remove("leaf_validity_exceeds_90_days")
        result = self._verify_mutation(catalog)
        self.assertEqual("FAIL", result["status"])
        self.assertTrue(any("excessive leaf validity" in error for error in result["errors"]))

    def test_shared_probe_identity_gap_cannot_be_hidden(self) -> None:
        catalog = copy.deepcopy(build_catalog())
        probe = next(item for item in catalog["certificate_domains"] if item["domain_id"] == "probe_ingest_mtls")
        probe["blocking_gaps"].remove("per_probe_certificate_identity_missing")
        result = self._verify_mutation(catalog)
        self.assertEqual("FAIL", result["status"])
        self.assertTrue(any("per-probe identity gap" in error for error in result["errors"]))

    def test_production_plaintext_inventory_cannot_be_hidden(self) -> None:
        catalog = copy.deepcopy(build_catalog())
        catalog["plaintext_dependencies"] = catalog["plaintext_dependencies"][-1:]
        result = self._verify_mutation(catalog)
        self.assertEqual("FAIL", result["status"])
        self.assertTrue(any("plaintext dependency inventory" in error for error in result["errors"]))

    def test_domain_cannot_claim_pass_without_live_rotation(self) -> None:
        catalog = copy.deepcopy(build_catalog())
        catalog["certificate_domains"][0]["status"] = "PASS"
        result = self._verify_mutation(catalog)
        self.assertEqual("FAIL", result["status"])
        self.assertTrue(any("claim PASS" in error for error in result["errors"]))

    def test_private_key_material_is_rejected(self) -> None:
        catalog = copy.deepcopy(build_catalog())
        catalog["certificate_domains"][0]["private_key_material"] = "-----BEGIN PRIVATE KEY-----"
        result = self._verify_mutation(catalog)
        self.assertEqual("FAIL", result["status"])
        self.assertTrue(any("private key material" in error for error in result["errors"]))

    def test_minio_tls_san_inventory_cannot_be_reduced(self) -> None:
        catalog = copy.deepcopy(build_catalog())
        minio = next(item for item in catalog["certificate_domains"] if item["domain_id"] == "minio_server_tls_target")
        minio["server_identity"]["dns_names"].pop()
        result = self._verify_mutation(catalog)
        self.assertEqual("FAIL", result["status"])
        self.assertTrue(any("MinIO TLS domain SAN" in error for error in result["errors"]))

    def test_minio_cutover_gap_cannot_be_hidden(self) -> None:
        catalog = copy.deepcopy(build_catalog())
        minio = next(item for item in catalog["certificate_domains"] if item["domain_id"] == "minio_server_tls_target")
        minio["blocking_gaps"].remove("candidate_images_and_live_cutover_evidence_incomplete")
        result = self._verify_mutation(catalog)
        self.assertEqual("FAIL", result["status"])
        self.assertTrue(any("MinIO TLS blocking gap" in error for error in result["errors"]))


if __name__ == "__main__":
    unittest.main()
