from __future__ import annotations

import copy
import unittest

from scripts.alignment import validate_m10_site_values as validator


class M10SiteValuesMutationTests(unittest.TestCase):
    @classmethod
    def setUpClass(cls) -> None:
        cls.valid = validator.load(validator.DEFAULT_INPUT)

    def errors(self, mutation) -> list[str]:
        value = copy.deepcopy(self.valid)
        mutation(value)
        return validator.validate_site_values(value)

    def assert_error(self, mutation, text: str) -> None:
        errors = self.errors(mutation)
        self.assertTrue(any(text in item for item in errors), errors)

    def test_current_template_passes(self) -> None:
        self.assertEqual([], validator.validate_site_values(self.valid))

    def test_unknown_top_level_field_is_rejected(self) -> None:
        self.assert_error(lambda value: value.update({"extra": {}}), "unknown fields: extra")

    def test_unknown_nested_field_is_rejected(self) -> None:
        self.assert_error(lambda value: value["site"]["quota"].update({"burst": 1}), "unknown fields: burst")

    def test_plaintext_secret_is_rejected(self) -> None:
        self.assert_error(lambda value: value["site"].update({"password": "bad"}), "plaintext secret field")

    def test_inline_private_key_is_rejected(self) -> None:
        self.assert_error(lambda value: value["site"]["storage"].update({"minio": "-----BEGIN PRIVATE KEY-----"}), "inline key/certificate")

    def test_default_tenant_is_rejected(self) -> None:
        self.assert_error(lambda value: value["tenants"][0].update({"tenantId": "default"}), "tenantId is default")

    def test_default_site_is_rejected(self) -> None:
        self.assert_error(lambda value: value["site"].update({"siteId": "default"}), "site.siteId")

    def test_invalid_port_is_rejected(self) -> None:
        self.assert_error(lambda value: value["site"]["network"]["ports"].update({"kafkaBootstrap": 70000}), "kafkaBootstrap is invalid")

    def test_tls_dependency_without_ca_is_rejected(self) -> None:
        self.assert_error(lambda value: value["site"]["externalDependencies"][0].update({"caSecretRef": None}), "caSecretRef must be an object")

    def test_non_tls_dependency_with_ca_is_rejected(self) -> None:
        self.assert_error(lambda value: value["site"]["externalDependencies"][1].update({"caSecretRef": {"namespace": "x", "name": "y", "key": "z"}}), "must be null when tls=false")

    def test_unknown_tenant_secret_ref_is_rejected(self) -> None:
        self.assert_error(lambda value: value["tenants"][0]["secretRefNames"].append("missing"), "unknown reference")

    def test_duplicate_tenant_is_rejected(self) -> None:
        def mutate(value):
            value["tenants"].append(copy.deepcopy(value["tenants"][0]))
        self.assert_error(mutate, "tenant ids must be unique")

    def test_fail_closed_flag_cannot_be_disabled(self) -> None:
        self.assert_error(lambda value: value["verification"].update({"rejectPlaintextSecrets": False}), "fail-closed flags")


if __name__ == "__main__":
    unittest.main()
