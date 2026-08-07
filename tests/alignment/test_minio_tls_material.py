from __future__ import annotations

import copy
import json
import shutil
import sys
import tempfile
import unittest
from pathlib import Path


ROOT = Path(__file__).resolve().parents[2]
SCRIPTS = ROOT / "scripts/alignment"
if str(SCRIPTS) not in sys.path:
    sys.path.insert(0, str(SCRIPTS))

import verify_minio_tls_material as verifier  # noqa: E402
from capture_minio_object_governance import resolve_artifact  # noqa: E402


class MinIOTLSMaterialTests(unittest.TestCase):
    def _mutated_root(self, mutate) -> tuple[tempfile.TemporaryDirectory, Path]:
        temporary = tempfile.TemporaryDirectory(prefix="minio-tls-material-")
        root = Path(temporary.name)
        for relative in (verifier.CONTRACT, verifier.MATERIAL, verifier.BASE_SERVER):
            destination = root / relative
            destination.parent.mkdir(parents=True, exist_ok=True)
            shutil.copy2(ROOT / relative, destination)
        contract_path = root / verifier.CONTRACT
        contract = json.loads(contract_path.read_text(encoding="utf-8"))
        mutate(contract, root)
        contract_path.write_text(json.dumps(contract), encoding="utf-8")
        return temporary, root

    def test_checked_in_candidate_material_is_valid_and_not_cutover_ready(self) -> None:
        result = verifier.verify()
        self.assertEqual("PASS", result["status"], result["errors"])
        self.assertFalse(result["production_applied"])
        self.assertFalse(result["cutover_ready"])
        self.assertEqual(8, result["server_san_count"])
        self.assertEqual(3, result["client_ca_namespace_count"])

    def test_capture_resolves_relative_tls_contract_without_crashing(self) -> None:
        absolute, relative = resolve_artifact(verifier.CONTRACT)
        self.assertTrue(absolute.is_file())
        self.assertEqual(verifier.CONTRACT.as_posix(), relative)

    def test_false_cutover_claim_is_rejected(self) -> None:
        temporary, root = self._mutated_root(lambda contract, _root: contract.update({"cutover_ready": True}))
        with temporary:
            result = verifier.verify(root)
        self.assertEqual("FAIL", result["status"])
        self.assertTrue(any("cutover readiness" in error for error in result["errors"]))

    def test_missing_server_san_is_rejected(self) -> None:
        def mutate(contract, _root):
            contract["server_identity"]["required_dns_sans"].pop()

        temporary, root = self._mutated_root(mutate)
        with temporary:
            result = verifier.verify(root)
        self.assertEqual("FAIL", result["status"])
        self.assertTrue(any("DNS SAN" in error for error in result["errors"]))

    def test_missing_client_ca_namespace_is_rejected(self) -> None:
        def mutate(contract, _root):
            contract["client_ca_distribution"].pop()

        temporary, root = self._mutated_root(mutate)
        with temporary:
            result = verifier.verify(root)
        self.assertEqual("FAIL", result["status"])
        self.assertTrue(any("namespace inventory" in error for error in result["errors"]))

    def test_accidental_server_activation_is_rejected(self) -> None:
        def mutate(_contract, root):
            path = root / verifier.BASE_SERVER
            path.write_text(path.read_text(encoding="utf-8") + "\n# --certs-dir minio-server-tls\n", encoding="utf-8")

        temporary, root = self._mutated_root(mutate)
        with temporary:
            result = verifier.verify(root)
        self.assertEqual("FAIL", result["status"])
        self.assertTrue(any("must not activate" in error for error in result["errors"]))


if __name__ == "__main__":
    unittest.main()
