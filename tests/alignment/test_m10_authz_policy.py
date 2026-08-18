from __future__ import annotations

import copy
import unittest

from scripts.alignment import build_m10_authz_policy as builder
from scripts.alignment import verify_m10_authz_policy as verifier


class M10AuthzPolicyTests(unittest.TestCase):
    @classmethod
    def setUpClass(cls) -> None:
        cls.expected = builder.build()

    def test_current_policy_is_exact(self) -> None:
        actual = verifier.load(verifier.OUTPUT)
        self.assertEqual([], verifier.validate(self.expected, actual))

    def test_role_and_operation_closure_is_exact(self) -> None:
        policy = self.expected
        self.assertEqual(["admin", "analyst", "operator", "viewer"], [r["role_id"] for r in policy["roles"]])
        self.assertEqual(143, policy["counts"]["operations"])
        self.assertEqual(0, policy["counts"]["operations_without_scope"])
        self.assertEqual(0, policy["counts"]["operations_without_authorized_role"])
        self.assertEqual(0, policy["counts"]["roles_with_wildcards"])
        self.assertEqual(0, policy["counts"]["roles_with_cross_tenant"])

    def test_runtime_sources_are_bound(self) -> None:
        bound = set(self.expected["source_sha256"])
        for required in (
            "go/control-plane/internal/common/httpx/authorization.go",
            "go/control-plane/internal/common/httpx/auth.go",
            "go/control-plane/internal/common/httpx/tenant.go",
            "go/control-plane/internal/auth/middleware/auth_middleware.go",
            "go/control-plane/internal/asset/api/auth.go",
        ):
            self.assertIn(required, bound)

    def test_wildcard_role_mutation_is_rejected(self) -> None:
        actual = copy.deepcopy(self.expected)
        actual["roles"][0]["scopes"].append("*")
        self.assertTrue(any("wildcard" in error for error in verifier.validate(self.expected, actual)))

    def test_cross_tenant_role_mutation_is_rejected(self) -> None:
        actual = copy.deepcopy(self.expected)
        actual["roles"][0]["cross_tenant"] = True
        self.assertTrue(any("cross-tenant" in error for error in verifier.validate(self.expected, actual)))

    def test_tenant_policy_relaxation_is_rejected(self) -> None:
        actual = copy.deepcopy(self.expected)
        actual["operations"][0]["tenant_policy"]["identity_source"] = "REQUEST_HEADER"
        self.assertTrue(any("tenant policy" in error for error in verifier.validate(self.expected, actual)))

    def test_object_existence_leak_is_rejected(self) -> None:
        actual = copy.deepcopy(self.expected)
        actual["operations"][0]["object_policy"]["missing_or_cross_tenant_status"] = 403
        self.assertTrue(any("leaks existence" in error for error in verifier.validate(self.expected, actual)))

    def test_field_allowlist_relaxation_is_rejected(self) -> None:
        actual = copy.deepcopy(self.expected)
        actual["operations"][0]["field_policy"]["enforcement"] = "PASS_THROUGH"
        self.assertTrue(any("allowlist-only" in error for error in verifier.validate(self.expected, actual)))

    def test_secret_field_denylist_removal_is_rejected(self) -> None:
        actual = copy.deepcopy(self.expected)
        actual["operations"][0]["field_policy"]["never_expose_fields"].remove("private_key")
        self.assertTrue(any("secret field" in error for error in verifier.validate(self.expected, actual)))

    def test_missing_authorized_role_is_rejected(self) -> None:
        actual = copy.deepcopy(self.expected)
        actual["operations"][0]["authorized_roles"] = []
        self.assertTrue(any("lacks a scope or role" in error for error in verifier.validate(self.expected, actual)))

    def test_policy_hash_mutation_is_rejected(self) -> None:
        actual = copy.deepcopy(self.expected)
        actual["policy_sha256"] = "0" * 64
        self.assertTrue(any("policy_sha256" in error for error in verifier.validate(self.expected, actual)))


if __name__ == "__main__":
    unittest.main()
