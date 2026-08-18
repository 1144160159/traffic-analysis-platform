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

from build_service_identity_catalog import GO_SERVICE_NAMES, _relative, build_catalog  # noqa: E402
import verify_service_identity_catalog as verifier  # noqa: E402


class ServiceIdentityCatalogTests(unittest.TestCase):
    def _seal(self, catalog: dict) -> dict:
        content = dict(catalog)
        content.pop("catalog_sha256", None)
        catalog["catalog_sha256"] = verifier._canonical_sha256(content)
        return catalog

    def _verify_mutation(self, catalog: dict) -> dict:
        self._seal(catalog)
        with tempfile.TemporaryDirectory(prefix="service-identity-catalog-") as directory:
            output = Path(directory) / "catalog.json"
            output.write_text(json.dumps(catalog), encoding="utf-8")
            with mock.patch.object(verifier, "OUTPUT", output), mock.patch.object(
                verifier, "build_catalog", return_value=catalog
            ):
                return verifier.verify()

    def test_checked_in_catalog_is_current_and_explicitly_partial(self) -> None:
        result = verifier.verify()
        self.assertEqual("PASS", result["status"], result["errors"])
        self.assertEqual("PARTIAL", result["security_compliance"])
        self.assertEqual(12, result["counts"]["workloads"])
        self.assertEqual(0, result["counts"]["literal_secret_findings"])

    def test_external_diagnostic_output_path_is_rendered_without_crashing(self) -> None:
        path = Path("/tmp/traffic-service-identity-diagnostic.json")
        self.assertEqual(path.as_posix(), _relative(path))

    def test_all_go_workloads_have_dedicated_identity_and_hardening(self) -> None:
        catalog = build_catalog()
        workloads = {item["name"]: item for item in catalog["workloads"]}
        self.assertTrue(GO_SERVICE_NAMES <= set(workloads))
        for name in GO_SERVICE_NAMES:
            workload = workloads[name]
            self.assertNotEqual("default", workload["service_account"]["name"])
            self.assertTrue(workload["service_account"]["declared"])
            self.assertIs(False, workload["service_account"]["pod_token_automount"])
            self.assertTrue(all(item["hardened"] for item in workload["containers"]))

    def test_default_service_account_fails_closed(self) -> None:
        catalog = copy.deepcopy(build_catalog())
        workload = next(item for item in catalog["workloads"] if item["name"] == "asset-service")
        workload["service_account"].update({"name": "default", "declared": False})
        result = self._verify_mutation(catalog)
        self.assertEqual("FAIL", result["status"])
        self.assertTrue(any("dedicated ServiceAccount" in error for error in result["errors"]))

    def test_service_account_token_automount_fails_closed(self) -> None:
        catalog = copy.deepcopy(build_catalog())
        workload = next(item for item in catalog["workloads"] if item["name"] == "alert-service")
        workload["service_account"]["pod_token_automount"] = True
        result = self._verify_mutation(catalog)
        self.assertEqual("FAIL", result["status"])
        self.assertTrue(any("token automount" in error for error in result["errors"]))

    def test_incomplete_container_hardening_fails_closed(self) -> None:
        catalog = copy.deepcopy(build_catalog())
        workload = next(item for item in catalog["workloads"] if item["name"] == "auth-service")
        workload["containers"][0]["hardened"] = False
        workload["containers"][0]["allow_privilege_escalation"] = True
        result = self._verify_mutation(catalog)
        self.assertEqual("FAIL", result["status"])
        self.assertTrue(any("hardening is incomplete" in error for error in result["errors"]))

    def test_shared_sensitive_credential_cannot_be_hidden(self) -> None:
        catalog = copy.deepcopy(build_catalog())
        catalog["shared_sensitive_credentials"].pop()
        result = self._verify_mutation(catalog)
        self.assertEqual("FAIL", result["status"])
        self.assertTrue(any("shared sensitive credential was hidden" in error for error in result["errors"]))

    def test_tenant_fallback_site_cannot_be_hidden(self) -> None:
        catalog = copy.deepcopy(build_catalog())
        catalog["tenant_authority_findings"]["untrusted_fallback_sites"].pop()
        result = self._verify_mutation(catalog)
        self.assertEqual("FAIL", result["status"])
        self.assertTrue(any("tenant authority fallback inventory" in error for error in result["errors"]))

    def test_wildcard_kafka_principal_fails_closed(self) -> None:
        catalog = copy.deepcopy(build_catalog())
        catalog["kafka_principals"][0]["principal"] = "User:*"
        result = self._verify_mutation(catalog)
        self.assertEqual("FAIL", result["status"])
        self.assertTrue(any("wildcard or anonymous" in error for error in result["errors"]))

    def test_probe_exception_must_remain_explicit(self) -> None:
        catalog = copy.deepcopy(build_catalog())
        workload = next(item for item in catalog["workloads"] if item["name"] == "probe-agent")
        workload["privileged_exception"]["service_account_token_required"] = False
        result = self._verify_mutation(catalog)
        self.assertEqual("FAIL", result["status"])
        self.assertTrue(any("privileged token exception" in error for error in result["errors"]))


if __name__ == "__main__":
    unittest.main()
