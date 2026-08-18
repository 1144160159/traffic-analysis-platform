#!/usr/bin/env python3
"""Bounded HTTPS client for the independently protected signature verifier."""

from __future__ import annotations

import hashlib
import http.client
import json
import ssl
from urllib.parse import urlsplit

from trusted_signature_service import (
    SignatureVerificationAttestation,
    SignatureVerificationRequest,
    VerificationRejected,
    _canonical_json_bytes,
    _certificate_chain_sha256,
    _sha256,
    _verify_public_key_signature,
)
from cryptography import x509


class TrustedSignatureClientError(RuntimeError):
    """No transport, shape, signature, or binding error may become a trust decision."""


def verify_exact_payload(
    request: SignatureVerificationRequest,
    *,
    endpoint: str,
    policy_fingerprint: str,
    timeout_seconds: float = 5.0,
    max_response_bytes: int = 262144,
) -> SignatureVerificationAttestation:
    """Submit one exact request and accept only a signed, fully bound PASS attestation."""

    if request.signed_payload.policy_fingerprint_sha256 != policy_fingerprint:
        raise TrustedSignatureClientError("request policy fingerprint differs from the pinned client value")
    if timeout_seconds <= 0 or timeout_seconds > 30:
        raise TrustedSignatureClientError("timeout must be within (0, 30] seconds")
    if max_response_bytes < 1024 or max_response_bytes > 4_194_304:
        raise TrustedSignatureClientError("response bound must be within [1024, 4194304] bytes")
    target = urlsplit(endpoint)
    if target.scheme != "https" or not target.hostname or target.username or target.password:
        raise TrustedSignatureClientError("protected verifier endpoint must be an HTTPS origin without credentials")
    if target.query or target.fragment or target.path not in {"", "/", "/v1/verify"}:
        raise TrustedSignatureClientError("protected verifier endpoint must use the fixed /v1/verify route")
    request_body = _canonical_json_bytes(request.to_dict())
    if len(request_body) > 4_194_304:
        raise TrustedSignatureClientError("serialized verification request exceeds the absolute client bound")
    context = ssl.create_default_context(purpose=ssl.Purpose.SERVER_AUTH)
    context.minimum_version = ssl.TLSVersion.TLSv1_2
    connection = http.client.HTTPSConnection(
        target.hostname,
        port=target.port or 443,
        timeout=timeout_seconds,
        context=context,
    )
    try:
        connection.request(
            "POST",
            "/v1/verify",
            body=request_body,
            headers={"Content-Type": "application/json", "Accept": "application/json"},
        )
        response = connection.getresponse()
        if response.status != 200:
            response.read(min(max_response_bytes + 1, 65536))
            raise TrustedSignatureClientError(f"protected verifier returned HTTP {response.status}")
        declared_length = response.getheader("Content-Length")
        if declared_length is not None:
            try:
                length = int(declared_length)
            except ValueError as exc:
                raise TrustedSignatureClientError("verifier Content-Length is invalid") from exc
            if length < 0 or length > max_response_bytes:
                raise TrustedSignatureClientError("verifier response exceeds the configured bound")
        body = response.read(max_response_bytes + 1)
        if len(body) > max_response_bytes:
            raise TrustedSignatureClientError("verifier response exceeds the configured bound")
    except (OSError, TimeoutError, http.client.HTTPException, ssl.SSLError) as exc:
        raise TrustedSignatureClientError("protected verifier transport failed") from exc
    finally:
        connection.close()
    try:
        raw = _load_unique_json(body)
        attestation = SignatureVerificationAttestation.from_dict(raw)
    except (UnicodeDecodeError, json.JSONDecodeError, ValueError, VerificationRejected) as exc:
        raise TrustedSignatureClientError("protected verifier returned an invalid attestation") from exc
    _verify_attestation_signature(attestation)
    _require_exact_binding(request, attestation, policy_fingerprint)
    if attestation.decision != "PASS" or attestation.decision_code != "VERIFIED":
        raise TrustedSignatureClientError(
            f"protected verifier rejected the request with {attestation.decision_code}"
        )
    if attestation.signer is None or attestation.replay_claim is None:
        raise TrustedSignatureClientError("PASS attestation lacks signer or replay binding")
    return attestation


def _verify_attestation_signature(attestation: SignatureVerificationAttestation) -> None:
    signature = attestation.attestation_signature
    if _sha256(signature.certificate_der) != signature.certificate_sha256:
        raise TrustedSignatureClientError("attestation certificate digest mismatch")
    if _sha256(signature.signature) != signature.signature_sha256:
        raise TrustedSignatureClientError("attestation signature digest mismatch")
    unsigned = attestation.to_dict()
    unsigned.pop("attestation_signature")
    body = _canonical_json_bytes(unsigned)
    if _sha256(body) != signature.attestation_body_sha256:
        raise TrustedSignatureClientError("attestation body digest mismatch")
    try:
        certificate = x509.load_der_x509_certificate(signature.certificate_der)
        _verify_public_key_signature(
            certificate.public_key(), body, signature.signature, signature.algorithm
        )
    except Exception as exc:
        raise TrustedSignatureClientError("attestation cryptographic signature is invalid") from exc


def _require_exact_binding(
    request: SignatureVerificationRequest,
    attestation: SignatureVerificationAttestation,
    policy_fingerprint: str,
) -> None:
    signed = request.signed_payload
    material = request.verification_material
    binding = attestation.request_binding
    canonical_envelope = {
        "schema_version": request.schema_version,
        "canonicalization_version": request.canonicalization_version,
        "request_id": request.request_id,
        "signed_payload": signed.to_dict(),
    }
    expected = {
        "request_id": request.request_id,
        "canonical_request_sha256": _sha256(_canonical_json_bytes(canonical_envelope)),
        "subject_type": signed.subject_type,
        "subject_id": signed.subject_id,
        "subject_payload_sha256": signed.subject_payload.content_sha256,
        "candidate_commit": signed.candidate_commit,
        "profile_id": signed.profile_id,
        "environment_id": signed.environment_id,
        "purpose": signed.purpose,
        "signature_algorithm": signed.signature_algorithm,
        "signer_certificate_sha256": signed.signer_certificate_sha256,
        "claimed_authority_ids": [item.authority_id for item in signed.claimed_authorities],
        "required_authority_roles": list(signed.required_authority_roles),
        "required_scopes": list(signed.required_scopes),
        "request_issued_at": signed.issued_at,
        "request_expires_at": signed.expires_at,
        "evaluation_time": signed.evaluation_time,
        "policy_id": signed.policy_id,
        "policy_version": signed.policy_version,
        "policy_fingerprint_sha256": policy_fingerprint,
        "nonce": signed.nonce,
        "cnas_context": signed.cnas_context.to_dict() if signed.cnas_context else None,
        "signature_sha256": material.signature_sha256,
        "certificate_chain_sha256": _certificate_chain_sha256(material.certificate_chain),
    }
    if binding.to_dict() != expected:
        raise TrustedSignatureClientError("attestation does not exactly bind the submitted request")
    if attestation.verifier.policy_bytes_sha256 != policy_fingerprint:
        raise TrustedSignatureClientError("attestation verifier policy bytes differ from the client pin")
    if attestation.attestation_signature.certificate_sha256 == signed.signer_certificate_sha256:
        raise TrustedSignatureClientError("attestation signer must be distinct from the subject signer")


def _load_unique_json(payload: bytes) -> dict[str, object]:
    def unique_object(pairs: list[tuple[str, object]]) -> dict[str, object]:
        result: dict[str, object] = {}
        for key, value in pairs:
            if key in result:
                raise ValueError(f"duplicate JSON property: {key}")
            result[key] = value
        return result

    value = json.loads(payload.decode("utf-8"), object_pairs_hook=unique_object)
    if not isinstance(value, dict):
        raise ValueError("attestation JSON root must be an object")
    return value
