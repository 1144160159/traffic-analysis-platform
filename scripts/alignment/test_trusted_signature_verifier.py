#!/usr/bin/env python3
"""Run the immutable eleven-case protected-verifier contract matrix."""

from __future__ import annotations

import argparse
import hashlib
import json
import sys
from pathlib import Path
from typing import Sequence

from trusted_signature_service import SignatureVerificationRequest
from verify_trusted_signature import TrustedSignatureClientError, verify_exact_payload


NEGATIVE_ROOT = Path(
    "scripts/alignment/fixtures/topic1/t1-m01-n010/"
    "trusted-signature-negative-run/tst-pre"
)
POSITIVE_ROOT = Path(
    "scripts/alignment/fixtures/topic1/t1-m01-n010/"
    "trusted-signature-positive-run/tst-post"
)
CASE_PATHS = (
    NEGATIVE_ROOT / "payload-substitution.json",
    NEGATIVE_ROOT / "wrong-authority-role.json",
    NEGATIVE_ROOT / "certificate-time-revocation-eku.json",
    NEGATIVE_ROOT / "policy-fingerprint-drift.json",
    NEGATIVE_ROOT / "chain-root-algorithm-invalid.json",
    NEGATIVE_ROOT / "attestation-candidate-profile-environment-mismatch.json",
    NEGATIVE_ROOT / "cnas-scope-mismatch.json",
    NEGATIVE_ROOT / "verifier-transport-or-shape-failure.json",
    NEGATIVE_ROOT / "self-reported-pass-random-signature.json",
    NEGATIVE_ROOT / "attestation-replay.json",
    POSITIVE_ROOT / "protected-positive-attestation.json",
)
EXPECTED_CASE_IDS = (
    "payload-substitution",
    "wrong-authority-role",
    "certificate-time-revocation-eku",
    "policy-fingerprint-drift",
    "chain-root-algorithm-invalid",
    "attestation-candidate-profile-environment-mismatch",
    "cnas-scope-mismatch",
    "verifier-transport-or-shape-failure",
    "self-reported-pass-random-signature",
    "attestation-replay",
    "protected-positive-attestation",
)


def main(argv: Sequence[str] | None = None) -> int:
    parser = argparse.ArgumentParser(
        description="Run the fixed protected signature verifier case matrix"
    )
    parser.add_argument("--repo-root", type=Path, required=True)
    parser.add_argument("--endpoint", required=True)
    parser.add_argument("--case-manifest", type=Path, required=True)
    parser.add_argument("--policy-fingerprint", required=True)
    parser.add_argument("--timeout-seconds", type=float, default=5.0)
    parser.add_argument("--max-response-bytes", type=int, default=262144)
    args = parser.parse_args(argv)
    try:
        repo_root = _canonical_repo_root(args.repo_root)
        manifest = _load_json_object(
            _resolve_file(repo_root, args.case_manifest, "case manifest")
        )
        _validate_manifest(manifest)
        observed: list[tuple[str, str, str]] = []
        for relative_path, expected_id in zip(CASE_PATHS, EXPECTED_CASE_IDS, strict=True):
            fixture = _load_json_object(_resolve_file(repo_root, relative_path, expected_id))
            _validate_fixture(fixture, expected_id, relative_path, manifest)
            request = SignatureVerificationRequest.from_dict(fixture["request"])
            try:
                attestation = verify_exact_payload(
                    request,
                    endpoint=args.endpoint,
                    policy_fingerprint=args.policy_fingerprint,
                    timeout_seconds=args.timeout_seconds,
                    max_response_bytes=args.max_response_bytes,
                )
                actual_decision = attestation.decision
                actual_code = attestation.decision_code
            except TrustedSignatureClientError as exc:
                actual_decision = "BLOCKED"
                actual_code = _client_error_code(exc)
            expected_decision = fixture["expected_decision"]
            expected_code = fixture["expected_code"]
            observed.append((expected_id, actual_decision, actual_code))
            if (actual_decision, actual_code) != (expected_decision, expected_code):
                raise ValueError(
                    f"{expected_id} expected {expected_decision}/{expected_code}, "
                    f"got {actual_decision}/{actual_code}"
                )
        if len(observed) != 11 or len({item[0] for item in observed}) != 11:
            raise ValueError("the verifier matrix did not evaluate exactly eleven unique cases")
        if sum(item[1] == "PASS" for item in observed) != 1:
            raise ValueError("the verifier matrix requires exactly one protected PASS")
        if any(item[1] == "PASS" for item in observed[:-1]) or observed[-1][1] != "PASS":
            raise ValueError("only protected-positive-attestation may produce PASS")
    except (OSError, ValueError, TypeError, json.JSONDecodeError) as exc:
        print(f"ERROR: {exc}", file=sys.stderr)
        return 1
    return 0


def _canonical_repo_root(path: Path) -> Path:
    if not path.is_absolute():
        raise ValueError("--repo-root must be absolute")
    resolved = path.resolve(strict=True)
    if resolved != path or path.is_symlink() or not path.is_dir():
        raise ValueError("--repo-root must be a canonical non-symlink directory")
    return resolved


def _resolve_file(repo_root: Path, path: Path, label: str) -> Path:
    relative = path
    if relative.is_absolute():
        try:
            relative = relative.relative_to(repo_root)
        except ValueError as exc:
            raise ValueError(f"{label} is outside the repository") from exc
    if not relative.parts or ".." in relative.parts:
        raise ValueError(f"{label} path is not canonical repo-relative")
    candidate = repo_root / relative
    resolved = candidate.resolve(strict=True)
    if resolved != candidate or candidate.is_symlink() or not resolved.is_file():
        raise ValueError(f"{label} must be a canonical non-symlink file")
    return resolved


def _load_json_object(path: Path) -> dict[str, object]:
    def unique_object(pairs: list[tuple[str, object]]) -> dict[str, object]:
        result: dict[str, object] = {}
        for key, value in pairs:
            if key in result:
                raise ValueError(f"duplicate JSON property in {path}: {key}")
            result[key] = value
        return result

    value = json.loads(path.read_text(encoding="utf-8"), object_pairs_hook=unique_object)
    if not isinstance(value, dict):
        raise ValueError(f"JSON root must be an object: {path}")
    return value


def _validate_manifest(manifest: dict[str, object]) -> None:
    allowed = {"schema_version", "case_ids", "fixtures"}
    if set(manifest) != allowed or manifest["schema_version"] != "1.0.0":
        raise ValueError("case manifest shape or version is invalid")
    if manifest["case_ids"] != list(EXPECTED_CASE_IDS):
        raise ValueError("case manifest IDs or order differ from the fixed matrix")
    fixtures = manifest["fixtures"]
    if not isinstance(fixtures, list) or len(fixtures) != 11:
        raise ValueError("case manifest must pin exactly eleven fixture files")


def _validate_fixture(
    fixture: dict[str, object],
    expected_id: str,
    relative_path: Path,
    manifest: dict[str, object],
) -> None:
    allowed = {
        "schema_version", "case_id", "request", "expected_decision", "expected_code",
        "fixture_sha256", "secret_absence_assertion",
    }
    if set(fixture) != allowed or fixture["schema_version"] != "1.0.0":
        raise ValueError(f"{expected_id} fixture shape or version is invalid")
    if fixture["case_id"] != expected_id:
        raise ValueError(f"{expected_id} fixture identity mismatch")
    if fixture["secret_absence_assertion"] is not True:
        raise ValueError(f"{expected_id} does not assert secret absence")
    expected_decision = "PASS" if expected_id == EXPECTED_CASE_IDS[-1] else "BLOCKED"
    if fixture["expected_decision"] != expected_decision:
        raise ValueError(f"{expected_id} expected decision is not fixed")
    fixtures = manifest["fixtures"]
    assert isinstance(fixtures, list)
    manifest_rows = [row for row in fixtures if isinstance(row, dict) and row.get("case_id") == expected_id]
    if len(manifest_rows) != 1:
        raise ValueError(f"{expected_id} lacks one manifest row")
    row = manifest_rows[0]
    if row.get("path") != relative_path.as_posix() or row.get("sha256") != fixture["fixture_sha256"]:
        raise ValueError(f"{expected_id} fixture path/hash differs from the manifest pin")
    request = fixture["request"]
    if not isinstance(request, dict):
        raise ValueError(f"{expected_id} request is not an object")
    forbidden = {"private_key", "secret", "password", "token", "bearer_token"}
    serialized_keys = _all_object_keys(request)
    if forbidden.intersection(serialized_keys):
        raise ValueError(f"{expected_id} request contains a forbidden secret field")


def _all_object_keys(value: object) -> set[str]:
    if isinstance(value, dict):
        return set(value).union(*( _all_object_keys(item) for item in value.values()), set())
    if isinstance(value, list):
        return set().union(*( _all_object_keys(item) for item in value), set())
    return set()


def _client_error_code(error: TrustedSignatureClientError) -> str:
    message = str(error)
    if "transport" in message or "HTTP" in message or "response" in message:
        return "VERIFIER_TRANSPORT_OR_SHAPE_FAILURE"
    if "binding" in message or "policy" in message:
        return "ATTESTATION_BINDING_REJECTED"
    if "cryptographic signature" in message or "signature digest" in message:
        return "ATTESTATION_SIGNATURE_REJECTED"
    return "VERIFIER_REJECTED"


if __name__ == "__main__":
    raise SystemExit(main())

