from __future__ import annotations

import hashlib
import json
import subprocess
import tempfile
import unittest
import uuid
from pathlib import Path

from scripts.alignment import candidate_snapshot


def digest(value: bytes) -> str:
    return hashlib.sha256(value).hexdigest()


class M10CandidateProvenanceGuardTests(unittest.TestCase):
    def test_current_repository_fails_closed(self) -> None:
        result = candidate_snapshot.scan_candidate_artifact_provenance()
        self.assertEqual("BLOCKED", result["status"])
        self.assertIn("PROVENANCE_REGISTRATION_REQUIRED", result["blocking_codes"])
        self.assertIn(
            "EXCLUDED_PREBUILT_ACTIVE_SELECTOR_UNPROVEN",
            result["blocking_codes"],
        )
        self.assertGreater(len(result["active_first_party_images"]), 0)

    def test_complete_fixture_passes(self) -> None:
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            image = "docker.io/traffic/example@sha256:" + "a" * 64
            manifest = root / candidate_snapshot.ACTIVE_MANIFESTS[0]
            manifest.parent.mkdir(parents=True)
            manifest.write_text(f"image: {image}\n", encoding="utf-8")
            binary_body = b"binary"
            sbom_body = b'{"bomFormat":"CycloneDX"}'
            attestation_body = b'{"predicateType":"https://slsa.dev/provenance/v1"}'
            binary = root / "artifacts/example"
            sbom = root / "artifacts/example.sbom.json"
            attestation = root / "artifacts/example.intoto.json"
            for path, body in ((binary, binary_body), (sbom, sbom_body), (attestation, attestation_body)):
                path.parent.mkdir(parents=True, exist_ok=True)
                path.write_bytes(body)

            registrations = [
                self.registration(image, "artifacts/example", binary_body, sbom_body, attestation_body)
            ]
            selectors = []
            for index, (artifact, recipes) in enumerate(candidate_snapshot.PREBUILT_RECIPES.items()):
                artifact_path = root / artifact
                artifact_path.parent.mkdir(parents=True, exist_ok=True)
                artifact_body = f"prebuilt-{index}".encode()
                artifact_path.write_bytes(artifact_body)
                recipe = root / recipes[0]
                recipe.parent.mkdir(parents=True, exist_ok=True)
                recipe.write_text("COPY alert-service /usr/local/bin/alert-service\n", encoding="utf-8")
                selectors.append(recipes[0])
                registrations.append(
                    self.registration(
                        artifact, artifact, artifact_body, sbom_body, attestation_body
                    )
                )
            (root / candidate_snapshot.BUILD_SELECTORS[0]).write_text(
                "\n".join(selectors), encoding="utf-8"
            )
            index = root / candidate_snapshot.PROVENANCE_INDEX
            index.parent.mkdir(parents=True)
            index.write_text(
                json.dumps({"schema_version": 1, "registrations": registrations}),
                encoding="utf-8",
            )
            result = candidate_snapshot.scan_candidate_artifact_provenance(root)
            self.assertEqual([], result["blocking_codes"])
            self.assertEqual("PASS", result["status"])

    def registration(
        self,
        artifact_ref: str,
        binary_path: str,
        binary_body: bytes,
        sbom_body: bytes,
        attestation_body: bytes,
    ) -> dict[str, str]:
        return {
            "artifact_ref": artifact_ref,
            "binary_path": binary_path,
            "binary_sha256": digest(binary_body),
            "source_or_builder_sha": "b" * 40,
            "recipe_or_toolchain": "go1.25.0 linux/amd64 deterministic",
            "sbom_path": "artifacts/example.sbom.json",
            "sbom_sha256": digest(sbom_body),
            "attestation_path": "artifacts/example.intoto.json",
            "attestation_sha256": digest(attestation_body),
            "image_digest": "docker.io/traffic/example@sha256:" + "a" * 64,
            "image_internal_binary_sha256": digest(binary_body),
        }

    def test_hash_mismatch_is_rejected(self) -> None:
        item = self.registration("artifact", "missing", b"x", b"y", b"z")
        with tempfile.TemporaryDirectory() as directory:
            errors = candidate_snapshot._registration_errors(
                Path(directory), "artifact", item
            )
        self.assertIn("REGISTERED_BINARY_MISSING", errors)
        self.assertIn("REGISTERED_ARTIFACT_MISSING:sbom_path", errors)

    def test_capture_g0_stops_before_commands(self) -> None:
        with tempfile.TemporaryDirectory() as directory:
            run_id = "m10-n002-" + uuid.uuid4().hex
            completed = subprocess.run(
                [
                    "python3", "scripts/alignment/capture_g0.py",
                    "--run-id", run_id,
                    "--profile", "probe-publisher",
                    "--output-root", directory,
                ],
                cwd=candidate_snapshot.ROOT,
                text=True,
                capture_output=True,
                check=False,
            )
            self.assertEqual(2, completed.returncode, completed.stdout + completed.stderr)
            manifest = json.loads(
                (Path(directory) / run_id / "manifest.json").read_text(encoding="utf-8")
            )
            self.assertEqual("BLOCKED", manifest["status"])
            self.assertEqual("candidate-artifact-provenance", manifest["blocking_stage"])
            self.assertEqual([], manifest["commands"])


if __name__ == "__main__":
    unittest.main()
