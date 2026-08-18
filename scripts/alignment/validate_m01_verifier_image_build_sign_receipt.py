#!/usr/bin/env python3
"""Validate M01 verifier image build/sign receipt structure and hash closure.

This validator proves candidate and artifact bindings. It does not perform the
external build, contact a registry, or establish cryptographic trust itself.
"""

from __future__ import annotations

import argparse
import copy
import hashlib
import json
import subprocess
from datetime import datetime
from pathlib import Path
from typing import Any, Callable

from build_topic1_task_registry import validate_against_schema


REPO = Path(__file__).resolve().parents[2]
SCHEMA = REPO / "contracts/alignment/m01-verifier-image-build-sign-receipt.schema.json"
ACTIVITY_ID = "EXT-T1-M01-N015-VERIFIER-IMAGE-BUILD-AND-SIGN"
INPUT_IDS = [
    "VERIFIER_IMAGE_BUILD_RECIPE",
    "VERIFIER_REQUIREMENTS_LOCK",
    "VERIFIER_SERVICE_SOURCE",
    "TRUST_POLICY_SCHEMA",
    "VERIFICATION_REQUEST_SCHEMA",
    "VERIFICATION_ATTESTATION_SCHEMA",
]
INPUT_PATHS = {
    "VERIFIER_IMAGE_BUILD_RECIPE": "deployments/security/topic1-trusted-signature-verifier.Dockerfile",
    "VERIFIER_REQUIREMENTS_LOCK": "deployments/security/topic1-trusted-signature-verifier.requirements.lock",
    "VERIFIER_SERVICE_SOURCE": "scripts/alignment/trusted_signature_service.py",
    "TRUST_POLICY_SCHEMA": "contracts/alignment/signature-trust-policy.schema.json",
    "VERIFICATION_REQUEST_SCHEMA": "contracts/alignment/signature-verification-request.schema.json",
    "VERIFICATION_ATTESTATION_SCHEMA": "contracts/alignment/signature-verification-attestation.schema.json",
}
OUTPUT_IDS = [
    "IMAGE_DIGEST",
    "SBOM_SHA256",
    "PROVENANCE_ATTESTATION",
    "SIGNATURE_VERIFICATION_RECEIPT",
]
ROLES = ["SUPPLY_CHAIN_OWNER", "SECURITY_OWNER"]


def semantic_sha256(value: Any) -> str:
    body = json.dumps(value, ensure_ascii=False, sort_keys=True, separators=(",", ":")).encode()
    return hashlib.sha256(body).hexdigest()


def signed_projection(payload: dict[str, Any]) -> dict[str, Any]:
    return {key: value for key, value in payload.items() if key not in {"signed_payload_sha256", "attestations"}}


def parse_time(value: str) -> datetime:
    try:
        return datetime.fromisoformat(value.replace("Z", "+00:00"))
    except ValueError as exc:
        raise ValueError("M01 verifier image receipt timestamp is not RFC3339") from exc


def validate_path(relative: str) -> None:
    path = Path(relative)
    resolved = (REPO / path).resolve()
    if path.is_absolute() or ".." in path.parts or not resolved.is_relative_to(REPO):
        raise ValueError("M01 verifier image receipt path escapes repository")


def load_exact_file(relative: str, expected_sha256: str) -> bytes:
    validate_path(relative)
    path = REPO / relative
    if not path.is_file() or path.is_symlink():
        raise ValueError(f"M01 verifier image receipt artifact is absent or unsafe: {relative}")
    body = path.read_bytes()
    if hashlib.sha256(body).hexdigest() != expected_sha256:
        raise ValueError(f"M01 verifier image receipt artifact hash mismatch: {relative}")
    return body


def load_candidate_blob(commit: str, relative: str) -> bytes:
    validate_path(relative)
    completed = subprocess.run(
        ["git", "show", f"{commit}:{relative}"], cwd=REPO,
        stdout=subprocess.PIPE, stderr=subprocess.PIPE, check=False,
    )
    if completed.returncode:
        raise ValueError(f"M01 verifier image receipt candidate blob is absent: {relative}")
    return completed.stdout


def validate_semantics(payload: dict[str, Any]) -> None:
    validate_against_schema(payload, SCHEMA)
    if payload["activity_id"] != ACTIVITY_ID:
        raise ValueError("M01 verifier image receipt activity identity mismatch")
    candidate_hash = payload["candidate"]["manifest_sha256"]
    inputs = payload["source_inputs"]
    input_ids = [item["artifact_id"] for item in inputs]
    if input_ids != INPUT_IDS or len(input_ids) != len(set(input_ids)):
        raise ValueError("M01 verifier image receipt input exact-set or order mismatch")
    for item in inputs:
        if item["path"] != INPUT_PATHS[item["artifact_id"]]:
            raise ValueError("M01 verifier image receipt input path mapping mismatch")
        if item["candidate_manifest_sha256"] != candidate_hash:
            raise ValueError("M01 verifier image receipt crosses candidate manifests")
        validate_path(item["path"])
    build_recipe_hash = inputs[0]["sha256"]
    build = payload["build"]
    if parse_time(build["started_at"]) >= parse_time(build["finished_at"]):
        raise ValueError("M01 verifier image receipt build window is not increasing")
    image = payload["image"]
    digest = image["digest"]
    if payload["published_image_ref"] != f"{image['repository']}@{digest}":
        raise ValueError("M01 verifier image receipt published reference is not the exact digest")
    if payload["sbom"]["subject_digest"] != digest:
        raise ValueError("M01 verifier image receipt SBOM subject mismatch")
    if payload["provenance"]["subject_digest"] != digest:
        raise ValueError("M01 verifier image receipt provenance subject mismatch")
    if payload["provenance"]["builder_identity"] != build["builder_identity"]:
        raise ValueError("M01 verifier image receipt provenance builder mismatch")
    if payload["provenance"]["build_recipe_sha256"] != build_recipe_hash:
        raise ValueError("M01 verifier image receipt provenance recipe mismatch")
    if payload["image_signature"]["subject_digest"] != digest:
        raise ValueError("M01 verifier image receipt signature subject mismatch")
    verification = payload["signature_verification_receipt"]
    if verification["subject_digest"] != digest:
        raise ValueError("M01 verifier image receipt verification subject mismatch")
    if parse_time(verification["verified_at"]) < parse_time(build["finished_at"]):
        raise ValueError("M01 verifier image receipt verification predates the build")
    outputs = payload["output_hashes"]
    output_ids = [item["artifact_id"] for item in outputs]
    if output_ids != OUTPUT_IDS or len(output_ids) != len(set(output_ids)):
        raise ValueError("M01 verifier image receipt output exact-set or order mismatch")
    output_map = {item["artifact_id"]: item["sha256"] for item in outputs}
    bindings = {
        "IMAGE_DIGEST": digest.removeprefix("sha256:"),
        "SBOM_SHA256": payload["sbom"]["artifact"]["sha256"],
        "PROVENANCE_ATTESTATION": payload["provenance"]["artifact"]["sha256"],
        "SIGNATURE_VERIFICATION_RECEIPT": verification["artifact"]["sha256"],
    }
    if output_map != bindings:
        raise ValueError("M01 verifier image receipt output hash binding mismatch")
    for field in ("manifest",):
        validate_path(image[field]["path"])
    for row in (
        build["environment_manifest"], payload["sbom"]["artifact"], payload["provenance"]["artifact"],
        payload["image_signature"]["bundle"], verification["artifact"],
    ):
        validate_path(row["path"])
    expected_signed = semantic_sha256(signed_projection(payload))
    if payload["signed_payload_sha256"] != expected_signed:
        raise ValueError("M01 verifier image receipt signed payload hash drifted")
    roles = [item["role"] for item in payload["attestations"]]
    identities = [item["signer_identity"] for item in payload["attestations"]]
    signatures = [item["signature_artifact"]["path"] for item in payload["attestations"]]
    protected_refs = [item["protected_verification_ref"]["path"] for item in payload["attestations"]]
    if roles != ROLES:
        raise ValueError("M01 verifier image receipt 2-of-2 role exact-set or order mismatch")
    if len(set(identities)) != 2 or len(set(signatures)) != 2 or len(set(protected_refs)) != 2:
        raise ValueError("M01 verifier image receipt signer or protected evidence is not independent")
    for item in payload["attestations"]:
        if item["signed_payload_sha256"] != expected_signed:
            raise ValueError("M01 verifier image receipt attestation payload hash mismatch")
        validate_path(item["signature_artifact"]["path"])
        validate_path(item["protected_verification_ref"]["path"])


def validate_instance_files(payload: dict[str, Any]) -> None:
    candidate = payload["candidate"]
    load_exact_file(candidate["manifest_path"], candidate["manifest_sha256"])
    commit_check = subprocess.run(
        ["git", "cat-file", "-e", f"{candidate['implementation_commit']}^{{commit}}"],
        cwd=REPO, stdout=subprocess.PIPE, stderr=subprocess.PIPE, check=False,
    )
    if commit_check.returncode:
        raise ValueError("M01 verifier image receipt candidate commit does not exist")
    for item in payload["source_inputs"]:
        blob = load_candidate_blob(candidate["implementation_commit"], item["path"])
        if hashlib.sha256(blob).hexdigest() != item["sha256"]:
            raise ValueError(f"M01 verifier image receipt candidate blob hash mismatch: {item['path']}")
    refs = [
        payload["build"]["environment_manifest"], payload["image"]["manifest"],
        payload["sbom"]["artifact"], payload["provenance"]["artifact"],
        payload["image_signature"]["bundle"], payload["signature_verification_receipt"]["artifact"],
    ]
    refs += [item["signature_artifact"] for item in payload["attestations"]]
    refs += [item["protected_verification_ref"] for item in payload["attestations"]]
    for ref in refs:
        load_exact_file(ref["path"], ref["sha256"])
    signed_body = load_exact_file(payload["signed_payload_artifact"], payload["signed_payload_sha256"])
    try:
        signed_value = json.loads(signed_body)
    except json.JSONDecodeError as exc:
        raise ValueError("M01 verifier image receipt signed payload artifact is not JSON") from exc
    if signed_value != signed_projection(payload):
        raise ValueError("M01 verifier image receipt signed payload artifact differs from receipt projection")


def fixture() -> dict[str, Any]:
    candidate_hash = "a" * 64
    digest = "sha256:" + "b" * 64
    inputs = [
        {"artifact_id": artifact_id, "source_kind": "CANDIDATE_GIT_BLOB", "path": INPUT_PATHS[artifact_id], "sha256": f"{index:064x}", "candidate_manifest_sha256": candidate_hash}
        for index, artifact_id in enumerate(INPUT_IDS, start=1)
    ]
    sbom_hash, provenance_hash, verification_hash = "c" * 64, "d" * 64, "e" * 64
    payload: dict[str, Any] = {
        "schema_version": "1.0.0",
        "artifact_kind": "M01_VERIFIER_IMAGE_BUILD_SIGN_AND_PUBLISH_RECEIPT",
        "receipt_id": "M01-VIBSP-selftest-0001",
        "activity_id": ACTIVITY_ID,
        "activity_kind": "EXTERNAL_BUILD_SIGN_AND_PUBLISH",
        "run_id": "run-selftest-0001",
        "candidate": {"implementation_commit": "1" * 40, "manifest_path": "doc/02_acceptance/topic1/m01/candidate.json", "manifest_sha256": candidate_hash},
        "profile_id": "M01-TRUST-BOOTSTRAP",
        "environment_id": "protected-ci-selftest",
        "source_inputs": inputs,
        "build": {
            "builder_identity": "protected-ci-builder",
            "builder_version": "1",
            "environment_manifest": {"path": "doc/02_acceptance/topic1/m01/environment.json", "sha256": "2" * 64},
            "invocation_sha256": "3" * 64,
            "network_mode": "NONE",
            "reproducible": True,
            "started_at": "2026-08-13T00:00:00Z",
            "finished_at": "2026-08-13T00:10:00Z",
        },
        "image": {
            "repository": "registry.example.invalid/topic1/trusted-signature-verifier",
            "digest": digest,
            "manifest": {"path": "doc/02_acceptance/topic1/m01/image-manifest.json", "sha256": "4" * 64},
            "config_digest": "sha256:" + "5" * 64,
            "runtime_binary_sha256": "6" * 64,
            "platforms": [{"os": "linux", "architecture": "amd64"}],
        },
        "sbom": {"format": "CycloneDX", "spec_version": "1.5", "artifact": {"path": "doc/02_acceptance/topic1/m01/verifier.cdx.json", "sha256": sbom_hash}, "subject_digest": digest},
        "provenance": {"predicate_type": "https://slsa.dev/provenance/v1", "artifact": {"path": "doc/02_acceptance/topic1/m01/provenance.bundle", "sha256": provenance_hash}, "subject_digest": digest, "builder_identity": "protected-ci-builder", "build_recipe_sha256": inputs[0]["sha256"]},
        "image_signature": {"bundle": {"path": "doc/02_acceptance/topic1/m01/signature.bundle", "sha256": "7" * 64}, "subject_digest": digest, "signature_algorithm": "ECDSA-P256-SHA256", "certificate_identity": "protected-ci@example.invalid", "certificate_issuer": "https://issuer.example.invalid", "transparency_log_verified": True},
        "signature_verification_receipt": {"artifact": {"path": "doc/02_acceptance/topic1/m01/signature-verification.json", "sha256": verification_hash}, "status": "PASS", "subject_digest": digest, "verifier_identity": "external-bootstrap-verifier", "verifier_version": "1", "verified_at": "2026-08-13T00:11:00Z", "revocation_checked": True},
        "output_hashes": [
            {"artifact_id": "IMAGE_DIGEST", "sha256": digest.removeprefix("sha256:")},
            {"artifact_id": "SBOM_SHA256", "sha256": sbom_hash},
            {"artifact_id": "PROVENANCE_ATTESTATION", "sha256": provenance_hash},
            {"artifact_id": "SIGNATURE_VERIFICATION_RECEIPT", "sha256": verification_hash},
        ],
        "result": "PASS",
        "published_image_ref": "registry.example.invalid/topic1/trusted-signature-verifier@" + digest,
        "signed_payload_artifact": "doc/02_acceptance/topic1/m01/signed-receipt-payload.json",
        "signed_payload_sha256": "",
        "attestations": [],
        "proof_ceiling": "EXTERNAL_VERIFIER_IMAGE_BUILD_SIGN_PUBLISH_RECEIPT_ONLY_NOT_GLOBAL_REGISTRY_SWITCH_RUNTIME_DEPLOYMENT_GATE_PASS_EXECUTION_AUTHORIZATION_OR_ACCEPTANCE",
    }
    payload["signed_payload_sha256"] = semantic_sha256(signed_projection(payload))
    payload["attestations"] = [
        {
            "role": role,
            "signer_identity": f"signer-{index}@example.invalid",
            "signed_payload_sha256": payload["signed_payload_sha256"],
            "signature_artifact": {"path": f"doc/02_acceptance/topic1/m01/{role.lower()}.sig", "sha256": f"{20 + index:064x}"},
            "bootstrap_policy_id": "M01-VERIFIER-BOOTSTRAP-PROTECTED-CI-2-OF-2-V1",
            "protected_verification_ref": {"path": f"doc/02_acceptance/topic1/m01/{role.lower()}-protected-verification.json", "sha256": f"{30 + index:064x}"},
        }
        for index, role in enumerate(ROLES, start=1)
    ]
    return payload


def expect_failure(label: str, payload: dict[str, Any], mutate: Callable[[dict[str, Any]], None], expected_error: str) -> None:
    candidate = copy.deepcopy(payload)
    mutate(candidate)
    try:
        validate_semantics(candidate)
    except (ValueError, KeyError, TypeError) as exc:
        if expected_error not in str(exc):
            raise ValueError(f"negative case {label} hit wrong assertion: {exc}") from exc
        return
    raise ValueError(f"negative case {label} did not fail")


def self_test() -> None:
    payload = fixture()
    validate_semantics(payload)
    tests: list[tuple[str, Callable[[dict[str, Any]], None], str]] = [
        ("input omission", lambda p: p["source_inputs"].pop(), "input exact-set or order mismatch"),
        ("input path drift", lambda p: p["source_inputs"][0].update({"path": "deployments/security/wrong.Dockerfile"}), "input path mapping mismatch"),
        ("cross candidate", lambda p: p["source_inputs"][0].update({"candidate_manifest_sha256": "0" * 64}), "crosses candidate manifests"),
        ("build window", lambda p: p["build"].update({"finished_at": p["build"]["started_at"]}), "build window is not increasing"),
        ("published digest", lambda p: p.update({"published_image_ref": "registry.example.invalid/wrong@sha256:" + "b" * 64}), "published reference is not the exact digest"),
        ("sbom subject", lambda p: p["sbom"].update({"subject_digest": "sha256:" + "0" * 64}), "SBOM subject mismatch"),
        ("provenance builder", lambda p: p["provenance"].update({"builder_identity": "wrong-builder"}), "provenance builder mismatch"),
        ("provenance recipe", lambda p: p["provenance"].update({"build_recipe_sha256": "0" * 64}), "provenance recipe mismatch"),
        ("signature subject", lambda p: p["image_signature"].update({"subject_digest": "sha256:" + "0" * 64}), "signature subject mismatch"),
        ("verification subject", lambda p: p["signature_verification_receipt"].update({"subject_digest": "sha256:" + "0" * 64}), "verification subject mismatch"),
        ("output omission", lambda p: p["output_hashes"].pop(), "output exact-set or order mismatch"),
        ("output binding", lambda p: p["output_hashes"][1].update({"sha256": "0" * 64}), "output hash binding mismatch"),
        ("signed payload drift", lambda p: p.update({"signed_payload_sha256": "0" * 64}), "signed payload hash drifted"),
        ("role drift", lambda p: p["attestations"].reverse(), "2-of-2 role exact-set or order mismatch"),
        ("signer reuse", lambda p: p["attestations"][1].update({"signer_identity": p["attestations"][0]["signer_identity"]}), "signer or protected evidence is not independent"),
        ("attestation payload drift", lambda p: p["attestations"][0].update({"signed_payload_sha256": "0" * 64}), "attestation payload hash mismatch"),
    ]
    for label, mutate, error in tests:
        expect_failure(label, payload, mutate, error)


def main() -> int:
    parser = argparse.ArgumentParser()
    parser.add_argument("--self-test", action="store_true")
    parser.add_argument("--check", type=Path)
    args = parser.parse_args()
    if args.self_test == bool(args.check):
        parser.error("choose exactly one of --self-test or --check PATH")
    if args.self_test:
        self_test()
        print("PASS M01 verifier image build-sign receipt: 1 structural positive and 16 targeted negative cases")
    else:
        path = args.check if args.check.is_absolute() else REPO / args.check
        payload = json.loads(path.read_text(encoding="utf-8"))
        validate_semantics(payload)
        validate_instance_files(payload)
        print(f"PASS M01 verifier image build-sign receipt instance: {path}")
    print("PROOF_CEILING STRUCTURE_AND_HASH_CLOSURE_ONLY_NOT_CRYPTOGRAPHIC_TRUST_EXTERNAL_BUILD_PUBLISH_DEPLOYMENT_EXECUTION_OR_ACCEPTANCE")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
