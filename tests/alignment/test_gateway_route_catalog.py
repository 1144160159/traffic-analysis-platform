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

from build_gateway_route_catalog import build_catalog  # noqa: E402
import verify_gateway_route_catalog as verifier  # noqa: E402


class GatewayRouteCatalogTests(unittest.TestCase):
    def _verify_mutation(self, catalog: dict) -> dict:
        with tempfile.TemporaryDirectory(prefix="gateway-route-catalog-") as directory:
            output = Path(directory) / "catalog.json"
            output.write_text(json.dumps(catalog), encoding="utf-8")
            with mock.patch.object(verifier, "OUTPUT", output), mock.patch.object(
                verifier, "build_catalog", return_value=catalog
            ):
                return verifier.verify()

    def test_checked_in_catalog_matches_apisix_openapi_and_services(self) -> None:
        result = verifier.verify()
        self.assertEqual("PASS", result["status"], result["errors"])
        self.assertEqual("PASS", result["security_compliance"])
        self.assertEqual([], result["openapi_coverage"]["uncovered_operation_ids"])
        self.assertEqual([], result["openapi_coverage"]["operations_missing_required_scope"])
        self.assertGreaterEqual(result["counts"]["routes"], 50)

    def test_every_route_has_request_id_and_explicit_upstream_policy(self) -> None:
        catalog = build_catalog()
        for route in catalog["routes"]:
            self.assertEqual("implemented", route["trace"]["status"], route["route_id"])
            if route["upstream"]:
                self.assertIsInstance(route["upstream"]["timeout"], dict, route["route_id"])
                self.assertEqual(0, route["upstream"]["retries"], route["route_id"])

    def test_protected_routes_have_oidc_body_and_request_validation(self) -> None:
        catalog = build_catalog()
        for route in catalog["routes"]:
            if not route["authentication"]["required"]:
                continue
            self.assertEqual(["openid-connect"], route["authentication"]["observed_plugins"], route["route_id"])
            self.assertGreater(route["limits"]["body_bytes"], 0, route["route_id"])
            self.assertIn("request-validation", route["plugins"], route["route_id"])
            self.assertEqual("$ENV://APISIX_OIDC_CLIENT_SECRET", route["plugins"]["openid-connect"]["client_secret"])

    def test_every_upstream_has_a_declared_service(self) -> None:
        catalog = build_catalog()
        undeclared = [
            node["endpoint"]
            for route in catalog["routes"]
            for node in ((route.get("upstream") or {}).get("nodes") or [])
            if not node["service_declared"]
        ]
        self.assertEqual([], undeclared)

    def test_protected_route_cannot_hide_missing_gateway_auth(self) -> None:
        catalog = build_catalog()
        mutated = copy.deepcopy(catalog)
        route = next(item for item in mutated["routes"] if item["authentication"]["required"])
        route["authentication"]["observed_plugins"] = []
        route["authentication"]["status"] = "missing"
        content = dict(mutated)
        content.pop("catalog_sha256")
        mutated["catalog_sha256"] = verifier._canonical_sha256(content)
        result = self._verify_mutation(mutated)
        self.assertEqual("FAIL", result["status"])
        self.assertTrue(any("not fail-closed" in error for error in result["errors"]))

    def test_uncovered_openapi_operation_fails_closed(self) -> None:
        catalog = build_catalog()
        mutated = copy.deepcopy(catalog)
        mutated["openapi_coverage"]["uncovered_operation_ids"] = ["mustRemainRouted"]
        content = dict(mutated)
        content.pop("catalog_sha256")
        mutated["catalog_sha256"] = verifier._canonical_sha256(content)
        result = self._verify_mutation(mutated)
        self.assertEqual("FAIL", result["status"])
        self.assertTrue(any("not exposed" in error for error in result["errors"]))

    def test_missing_openapi_scope_fails_closed(self) -> None:
        catalog = build_catalog()
        mutated = copy.deepcopy(catalog)
        mutated["openapi_coverage"]["operations_missing_required_scope"] = ["mustHaveScope"]
        content = dict(mutated)
        content.pop("catalog_sha256")
        mutated["catalog_sha256"] = verifier._canonical_sha256(content)
        result = self._verify_mutation(mutated)
        self.assertEqual("FAIL", result["status"])
        self.assertTrue(any("missing x-required-scope" in error for error in result["errors"]))

    def test_undeclared_upstream_service_fails_closed(self) -> None:
        catalog = build_catalog()
        mutated = copy.deepcopy(catalog)
        route = next(item for item in mutated["routes"] if item.get("upstream"))
        route["upstream"]["nodes"][0]["service_declared"] = False
        content = dict(mutated)
        content.pop("catalog_sha256")
        mutated["catalog_sha256"] = verifier._canonical_sha256(content)
        result = self._verify_mutation(mutated)
        self.assertEqual("FAIL", result["status"])
        self.assertTrue(any("has no declared Service" in error for error in result["errors"]))

    def test_admin_api_external_exposure_fails_closed(self) -> None:
        catalog = build_catalog()
        with tempfile.TemporaryDirectory(prefix="gateway-admin-exposure-") as directory:
            output = Path(directory) / "catalog.json"
            output.write_text(json.dumps(catalog), encoding="utf-8")
            services = {
                "apisix": {"ports": [{"port": 9080}]},
                "apisix-admin": {"type": "NodePort", "ports": [{"port": 9180, "nodePort": 30181}]},
            }
            with mock.patch.object(verifier, "OUTPUT", output), mock.patch.object(
                verifier, "build_catalog", return_value=catalog
            ), mock.patch.object(verifier, "_gateway_services", return_value=services):
                result = verifier.verify()
        self.assertEqual("FAIL", result["status"])
        self.assertTrue(any("admin API" in error for error in result["errors"]))


if __name__ == "__main__":
    unittest.main()
