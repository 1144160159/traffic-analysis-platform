#!/usr/bin/env python3
"""Fail-closed core for the Topic One protected signature verifier.

This module deliberately contains no in-memory production fallback for secret
signing, revocation checking, replay protection, or the append-only ledger.
Those capabilities cross a trust boundary and must be injected explicitly.
"""

from __future__ import annotations

import base64
import hashlib
import json
import re
from dataclasses import dataclass, replace
from datetime import datetime, timedelta, timezone
from pathlib import Path
from types import MappingProxyType
from typing import Mapping, Protocol, Sequence

from cryptography import x509
from cryptography.exceptions import InvalidSignature
from cryptography.hazmat.primitives import hashes, serialization
from cryptography.hazmat.primitives.asymmetric import ec, ed25519, ed448, padding, rsa
from cryptography.x509.oid import ExtensionOID, ExtendedKeyUsageOID


REPO_ROOT = Path(__file__).resolve().parents[2]
CONTRACT_ROOT = REPO_ROOT / "contracts" / "alignment"
REQUEST_SCHEMA_PATH = CONTRACT_ROOT / "signature-verification-request.schema.json"
POLICY_SCHEMA_PATH = CONTRACT_ROOT / "signature-trust-policy.schema.json"
ATTESTATION_SCHEMA_PATH = CONTRACT_ROOT / "signature-verification-attestation.schema.json"
SOFTWARE_VERSION = "m01-trusted-signature-v1"

_SHA256_RE = re.compile(r"^[0-9a-f]{64}$")
_RFC3339_UTC_RE = re.compile(
    r"^[0-9]{4}-[0-9]{2}-[0-9]{2}T[0-9]{2}:[0-9]{2}:[0-9]{2}(?:\.[0-9]{1,9})?Z$"
)


class VerificationError(RuntimeError):
    """Base class carrying a closed attestation decision code and failed check."""

    def __init__(self, code: str, check: str, message: str) -> None:
        super().__init__(message)
        self.code = code
        self.check = check


class VerificationRejected(VerificationError):
    """A deterministic input, policy, cryptographic, authority, or replay rejection."""


class VerificationDependencyError(VerificationError):
    """A protected dependency failed, so verification cannot safely decide PASS."""


@dataclass(frozen=True, slots=True)
class SecretReference:
    provider: str
    reference: str
    version: str
    purpose: str

    @classmethod
    def from_dict(cls, value: Mapping[str, object]) -> "SecretReference":
        return cls(
            provider=_required_string(value, "provider"),
            reference=_required_string(value, "reference"),
            version=_required_string(value, "version"),
            purpose=_required_string(value, "purpose"),
        )


@dataclass(frozen=True, slots=True)
class SecretHandle:
    """Opaque provider handle. Raw secret bytes are intentionally unrepresentable."""

    provider: str
    reference: str
    version: str
    purpose: str
    handle_id: str
    expires_at: datetime | None = None

    def __repr__(self) -> str:
        return (
            "SecretHandle(provider="
            f"{self.provider!r}, reference={self.reference!r}, version={self.version!r}, "
            f"purpose={self.purpose!r}, handle_id='<opaque>')"
        )


@dataclass(frozen=True, slots=True)
class RevocationEvidence:
    status: str
    checked_certificate_sha256: tuple[str, ...]
    evidence_sha256: str
    produced_at: datetime
    next_update: datetime


class SecretResolver(Protocol):
    """Protected provider boundary; methods never return secret values."""

    def resolve(self, reference: SecretReference) -> SecretHandle: ...

    def sign(self, handle: SecretHandle, payload: bytes, algorithm: str) -> bytes: ...

    def check_revocation(
        self,
        certificate_chain: Sequence[bytes],
        *,
        verification_time: datetime,
        methods: frozenset[str],
        max_evidence_age_seconds: int,
        client_auth_handle: SecretHandle | None,
    ) -> RevocationEvidence: ...


@dataclass(frozen=True, slots=True)
class AuthorityClaim:
    authority_id: str
    role: str

    @classmethod
    def from_dict(cls, value: Mapping[str, object]) -> "AuthorityClaim":
        return cls(_required_string(value, "authority_id"), _required_string(value, "role"))

    def to_dict(self) -> dict[str, object]:
        return {"authority_id": self.authority_id, "role": self.role}


@dataclass(frozen=True, slots=True)
class CnasContext:
    accreditation_id: str
    scope_item_id: str
    method_id: str
    scope_sha256: str

    @classmethod
    def from_dict(cls, value: Mapping[str, object]) -> "CnasContext":
        return cls(
            _required_string(value, "accreditation_id"),
            _required_string(value, "scope_item_id"),
            _required_string(value, "method_id"),
            _required_string(value, "scope_sha256"),
        )

    def to_dict(self) -> dict[str, object]:
        return {
            "accreditation_id": self.accreditation_id,
            "scope_item_id": self.scope_item_id,
            "method_id": self.method_id,
            "scope_sha256": self.scope_sha256,
        }


@dataclass(frozen=True, slots=True)
class SubjectPayload:
    artifact_id: str
    media_type: str
    content: bytes
    content_sha256: str
    size_bytes: int

    @classmethod
    def from_dict(cls, value: Mapping[str, object]) -> "SubjectPayload":
        return cls(
            artifact_id=_required_string(value, "artifact_id"),
            media_type=_required_string(value, "media_type"),
            content=_decode_base64(_required_string(value, "content_base64"), "subject content"),
            content_sha256=_required_string(value, "content_sha256"),
            size_bytes=_required_integer(value, "size_bytes"),
        )

    def to_dict(self) -> dict[str, object]:
        return {
            "artifact_id": self.artifact_id,
            "media_type": self.media_type,
            "content_base64": base64.b64encode(self.content).decode("ascii"),
            "content_sha256": self.content_sha256,
            "size_bytes": self.size_bytes,
        }


@dataclass(frozen=True, slots=True)
class SignedPayload:
    domain: str
    subject_type: str
    subject_id: str
    subject_payload: SubjectPayload
    candidate_commit: str
    profile_id: str
    environment_id: str
    purpose: str
    signature_algorithm: str
    signer_certificate_sha256: str
    certificate_chain_sha256: str
    claimed_authorities: tuple[AuthorityClaim, ...]
    required_authority_roles: tuple[str, ...]
    required_scopes: tuple[str, ...]
    issued_at: str
    expires_at: str
    evaluation_time: str
    policy_id: str
    policy_version: str
    policy_fingerprint_sha256: str
    nonce: str
    cnas_context: CnasContext | None

    @classmethod
    def from_dict(cls, value: Mapping[str, object]) -> "SignedPayload":
        subject_payload = _required_mapping(value, "subject_payload")
        authorities = _required_sequence(value, "claimed_authorities")
        cnas_value = value.get("cnas_context")
        if cnas_value is not None and not isinstance(cnas_value, Mapping):
            raise VerificationRejected("REQUEST_SCHEMA_INVALID", "schema", "cnas_context must be an object or null")
        return cls(
            domain=_required_string(value, "domain"),
            subject_type=_required_string(value, "subject_type"),
            subject_id=_required_string(value, "subject_id"),
            subject_payload=SubjectPayload.from_dict(subject_payload),
            candidate_commit=_required_string(value, "candidate_commit"),
            profile_id=_required_string(value, "profile_id"),
            environment_id=_required_string(value, "environment_id"),
            purpose=_required_string(value, "purpose"),
            signature_algorithm=_required_string(value, "signature_algorithm"),
            signer_certificate_sha256=_required_string(value, "signer_certificate_sha256"),
            certificate_chain_sha256=_required_string(value, "certificate_chain_sha256"),
            claimed_authorities=tuple(
                AuthorityClaim.from_dict(_as_mapping(item, "claimed_authorities item"))
                for item in authorities
            ),
            required_authority_roles=_string_tuple(value, "required_authority_roles"),
            required_scopes=_string_tuple(value, "required_scopes"),
            issued_at=_required_string(value, "issued_at"),
            expires_at=_required_string(value, "expires_at"),
            evaluation_time=_required_string(value, "evaluation_time"),
            policy_id=_required_string(value, "policy_id"),
            policy_version=_required_string(value, "policy_version"),
            policy_fingerprint_sha256=_required_string(value, "policy_fingerprint_sha256"),
            nonce=_required_string(value, "nonce"),
            cnas_context=CnasContext.from_dict(cnas_value) if cnas_value is not None else None,
        )

    def to_dict(self) -> dict[str, object]:
        return {
            "domain": self.domain,
            "subject_type": self.subject_type,
            "subject_id": self.subject_id,
            "subject_payload": self.subject_payload.to_dict(),
            "candidate_commit": self.candidate_commit,
            "profile_id": self.profile_id,
            "environment_id": self.environment_id,
            "purpose": self.purpose,
            "signature_algorithm": self.signature_algorithm,
            "signer_certificate_sha256": self.signer_certificate_sha256,
            "certificate_chain_sha256": self.certificate_chain_sha256,
            "claimed_authorities": [item.to_dict() for item in self.claimed_authorities],
            "required_authority_roles": list(self.required_authority_roles),
            "required_scopes": list(self.required_scopes),
            "issued_at": self.issued_at,
            "expires_at": self.expires_at,
            "evaluation_time": self.evaluation_time,
            "policy_id": self.policy_id,
            "policy_version": self.policy_version,
            "policy_fingerprint_sha256": self.policy_fingerprint_sha256,
            "nonce": self.nonce,
            "cnas_context": self.cnas_context.to_dict() if self.cnas_context else None,
        }


@dataclass(frozen=True, slots=True)
class CertificateMaterial:
    certificate_id: str
    certificate_sha256: str
    certificate_der: bytes

    @classmethod
    def from_dict(cls, value: Mapping[str, object]) -> "CertificateMaterial":
        return cls(
            certificate_id=_required_string(value, "certificate_id"),
            certificate_sha256=_required_string(value, "certificate_sha256"),
            certificate_der=_decode_base64(
                _required_string(value, "certificate_der_base64"), "certificate DER"
            ),
        )

    def to_dict(self) -> dict[str, object]:
        return {
            "certificate_id": self.certificate_id,
            "certificate_sha256": self.certificate_sha256,
            "certificate_der_base64": base64.b64encode(self.certificate_der).decode("ascii"),
        }


@dataclass(frozen=True, slots=True)
class VerificationMaterial:
    detached_signature: bytes
    signature_sha256: str
    certificate_chain: tuple[CertificateMaterial, ...]

    @classmethod
    def from_dict(cls, value: Mapping[str, object]) -> "VerificationMaterial":
        return cls(
            detached_signature=_decode_base64(
                _required_string(value, "detached_signature_base64"), "detached signature"
            ),
            signature_sha256=_required_string(value, "signature_sha256"),
            certificate_chain=tuple(
                CertificateMaterial.from_dict(_as_mapping(item, "certificate_chain item"))
                for item in _required_sequence(value, "certificate_chain")
            ),
        )

    def to_dict(self) -> dict[str, object]:
        return {
            "detached_signature_base64": base64.b64encode(self.detached_signature).decode("ascii"),
            "signature_sha256": self.signature_sha256,
            "certificate_chain": [item.to_dict() for item in self.certificate_chain],
        }


@dataclass(frozen=True, slots=True)
class SignatureVerificationRequest:
    schema_version: str
    canonicalization_version: str
    request_id: str
    signed_payload: SignedPayload
    verification_material: VerificationMaterial

    @classmethod
    def from_dict(cls, value: Mapping[str, object]) -> "SignatureVerificationRequest":
        _validate_against_schema(value, REQUEST_SCHEMA_PATH)
        return cls(
            schema_version=_required_string(value, "schema_version"),
            canonicalization_version=_required_string(value, "canonicalization_version"),
            request_id=_required_string(value, "request_id"),
            signed_payload=SignedPayload.from_dict(_required_mapping(value, "signed_payload")),
            verification_material=VerificationMaterial.from_dict(
                _required_mapping(value, "verification_material")
            ),
        )

    def to_dict(self) -> dict[str, object]:
        return {
            "schema_version": self.schema_version,
            "canonicalization_version": self.canonicalization_version,
            "request_id": self.request_id,
            "signed_payload": self.signed_payload.to_dict(),
            "verification_material": self.verification_material.to_dict(),
        }


@dataclass(frozen=True, slots=True)
class TrustAnchor:
    anchor_id: str
    certificate_sha256: str
    certificate_der: bytes
    allowed_algorithms: frozenset[str]


@dataclass(frozen=True, slots=True)
class RoleRule:
    role: str
    allowed_authority_ids: frozenset[str]
    allowed_purposes: frozenset[str]
    allowed_subject_types: frozenset[str]
    allowed_scopes: frozenset[str]
    allowed_profile_ids: frozenset[str]
    allowed_environment_ids: frozenset[str]
    required_eku_oids: frozenset[str]
    require_cnas_scope: bool
    allowed_cnas_scope_sha256: str | None


@dataclass(frozen=True, slots=True)
class LoadedTrustPolicy:
    schema_version: str
    policy_id: str
    policy_version: str
    fingerprint: str
    canonicalization_version: str
    verifier_service_id: str
    verifier_environment: str
    attestation_algorithm: str
    attestation_certificate_sha256: str
    attestation_certificate_der: bytes
    trust_anchors: tuple[TrustAnchor, ...]
    allowed_algorithms: frozenset[str]
    role_rules: tuple[RoleRule, ...]
    max_clock_skew_seconds: int
    max_request_validity_seconds: int
    max_attestation_age_seconds: int
    revocation_methods: frozenset[str]
    max_revocation_evidence_age_seconds: int
    replay_namespace: str
    replay_claim_ttl_seconds: int
    max_request_bytes: int
    max_payload_bytes: int
    max_signature_bytes: int
    max_certificate_bytes: int
    max_chain_length: int
    attestation_signing_handle: SecretHandle
    revocation_client_handle: SecretHandle | None
    revocation_checker: SecretResolver


@dataclass(frozen=True, slots=True)
class VerifiedSigner:
    signer_fingerprint: str
    chain_fingerprints: tuple[str, ...]
    signer_identities: frozenset[str]
    asserted_roles: frozenset[str]
    asserted_scopes: frozenset[str]
    valid_until: datetime
    revocation_evidence_sha256: str
    allowed_algorithms: frozenset[str]
    max_payload_bytes: int
    max_signature_bytes: int
    public_key: object


@dataclass(frozen=True, slots=True)
class VerifiedAuthority:
    authority_ids: tuple[str, ...]
    verified_roles: frozenset[str]
    verified_scopes: frozenset[str]
    valid_until: datetime
    cnas_scope_sha256: str | None


@dataclass(frozen=True, slots=True)
class ReplayClaim:
    claim_id: str
    claim_key_sha256: str
    canonical_request_sha256: str
    disposition: str
    expires_at: datetime

    @classmethod
    def from_dict(cls, value: Mapping[str, object]) -> "ReplayClaim":
        return cls(
            claim_id=_required_string(value, "claim_id"),
            claim_key_sha256=_required_string(value, "claim_key_sha256"),
            canonical_request_sha256=_required_string(value, "canonical_request_sha256"),
            disposition=_required_string(value, "disposition"),
            expires_at=_parse_rfc3339(_required_string(value, "expires_at"), "expires_at"),
        )

    def to_dict(self) -> dict[str, object]:
        return {
            "claim_id": self.claim_id,
            "claim_key_sha256": self.claim_key_sha256,
            "canonical_request_sha256": self.canonical_request_sha256,
            "disposition": self.disposition,
            "expires_at": _format_rfc3339(self.expires_at),
        }


class ReplayStore(Protocol):
    def compare_and_set(
        self,
        *,
        claim_key_sha256: str,
        canonical_request_sha256: str,
        request_identity_sha256: str,
        expires_at: datetime,
    ) -> ReplayClaim: ...


class AttestationLedger(Protocol):
    def append_and_fsync(self, record_id: str, record: bytes) -> str: ...

    def health(self) -> bool: ...


@dataclass(frozen=True, slots=True)
class RequestBinding:
    request_id: str
    canonical_request_sha256: str
    subject_type: str
    subject_id: str
    subject_payload_sha256: str
    candidate_commit: str
    profile_id: str
    environment_id: str
    purpose: str
    signature_algorithm: str
    signer_certificate_sha256: str
    claimed_authority_ids: tuple[str, ...]
    required_authority_roles: tuple[str, ...]
    required_scopes: tuple[str, ...]
    request_issued_at: str
    request_expires_at: str
    evaluation_time: str
    policy_id: str
    policy_version: str
    policy_fingerprint_sha256: str
    nonce: str
    cnas_context: CnasContext | None
    signature_sha256: str
    certificate_chain_sha256: str

    @classmethod
    def from_dict(cls, value: Mapping[str, object]) -> "RequestBinding":
        cnas_value = value.get("cnas_context")
        if cnas_value is not None and not isinstance(cnas_value, Mapping):
            raise VerificationRejected(
                "REQUEST_SCHEMA_INVALID", "schema", "cnas_context must be an object or null"
            )
        return cls(
            request_id=_required_string(value, "request_id"),
            canonical_request_sha256=_required_string(value, "canonical_request_sha256"),
            subject_type=_required_string(value, "subject_type"),
            subject_id=_required_string(value, "subject_id"),
            subject_payload_sha256=_required_string(value, "subject_payload_sha256"),
            candidate_commit=_required_string(value, "candidate_commit"),
            profile_id=_required_string(value, "profile_id"),
            environment_id=_required_string(value, "environment_id"),
            purpose=_required_string(value, "purpose"),
            signature_algorithm=_required_string(value, "signature_algorithm"),
            signer_certificate_sha256=_required_string(value, "signer_certificate_sha256"),
            claimed_authority_ids=_string_tuple(value, "claimed_authority_ids"),
            required_authority_roles=_string_tuple(value, "required_authority_roles"),
            required_scopes=_string_tuple(value, "required_scopes"),
            request_issued_at=_required_string(value, "request_issued_at"),
            request_expires_at=_required_string(value, "request_expires_at"),
            evaluation_time=_required_string(value, "evaluation_time"),
            policy_id=_required_string(value, "policy_id"),
            policy_version=_required_string(value, "policy_version"),
            policy_fingerprint_sha256=_required_string(value, "policy_fingerprint_sha256"),
            nonce=_required_string(value, "nonce"),
            cnas_context=CnasContext.from_dict(cnas_value) if cnas_value is not None else None,
            signature_sha256=_required_string(value, "signature_sha256"),
            certificate_chain_sha256=_required_string(value, "certificate_chain_sha256"),
        )

    @classmethod
    def from_request(
        cls, request: SignatureVerificationRequest, canonical_request_sha256: str
    ) -> "RequestBinding":
        signed = request.signed_payload
        material = request.verification_material
        return cls(
            request_id=request.request_id,
            canonical_request_sha256=canonical_request_sha256,
            subject_type=signed.subject_type,
            subject_id=signed.subject_id,
            subject_payload_sha256=signed.subject_payload.content_sha256,
            candidate_commit=signed.candidate_commit,
            profile_id=signed.profile_id,
            environment_id=signed.environment_id,
            purpose=signed.purpose,
            signature_algorithm=signed.signature_algorithm,
            signer_certificate_sha256=signed.signer_certificate_sha256,
            claimed_authority_ids=tuple(item.authority_id for item in signed.claimed_authorities),
            required_authority_roles=signed.required_authority_roles,
            required_scopes=signed.required_scopes,
            request_issued_at=signed.issued_at,
            request_expires_at=signed.expires_at,
            evaluation_time=signed.evaluation_time,
            policy_id=signed.policy_id,
            policy_version=signed.policy_version,
            policy_fingerprint_sha256=signed.policy_fingerprint_sha256,
            nonce=signed.nonce,
            cnas_context=signed.cnas_context,
            signature_sha256=material.signature_sha256,
            certificate_chain_sha256=signed.certificate_chain_sha256,
        )

    def to_dict(self) -> dict[str, object]:
        return {
            "request_id": self.request_id,
            "canonical_request_sha256": self.canonical_request_sha256,
            "subject_type": self.subject_type,
            "subject_id": self.subject_id,
            "subject_payload_sha256": self.subject_payload_sha256,
            "candidate_commit": self.candidate_commit,
            "profile_id": self.profile_id,
            "environment_id": self.environment_id,
            "purpose": self.purpose,
            "signature_algorithm": self.signature_algorithm,
            "signer_certificate_sha256": self.signer_certificate_sha256,
            "claimed_authority_ids": list(self.claimed_authority_ids),
            "required_authority_roles": list(self.required_authority_roles),
            "required_scopes": list(self.required_scopes),
            "request_issued_at": self.request_issued_at,
            "request_expires_at": self.request_expires_at,
            "evaluation_time": self.evaluation_time,
            "policy_id": self.policy_id,
            "policy_version": self.policy_version,
            "policy_fingerprint_sha256": self.policy_fingerprint_sha256,
            "nonce": self.nonce,
            "cnas_context": self.cnas_context.to_dict() if self.cnas_context else None,
            "signature_sha256": self.signature_sha256,
            "certificate_chain_sha256": self.certificate_chain_sha256,
        }


_CHECK_NAMES = (
    "schema",
    "policy",
    "certificate_chain",
    "signature",
    "authority_scope",
    "time_window",
    "revocation",
    "replay",
    "request_binding",
    "cnas_scope",
)


@dataclass(frozen=True, slots=True)
class VerificationChecks:
    schema: str = "NOT_RUN"
    policy: str = "NOT_RUN"
    certificate_chain: str = "NOT_RUN"
    signature: str = "NOT_RUN"
    authority_scope: str = "NOT_RUN"
    time_window: str = "NOT_RUN"
    revocation: str = "NOT_RUN"
    replay: str = "NOT_RUN"
    request_binding: str = "NOT_RUN"
    cnas_scope: str = "NOT_RUN"

    @classmethod
    def from_dict(cls, value: Mapping[str, object]) -> "VerificationChecks":
        return cls(**{name: _required_string(value, name) for name in _CHECK_NAMES})

    def mark(self, check: str, status: str) -> "VerificationChecks":
        if check not in _CHECK_NAMES:
            raise VerificationDependencyError("INTERNAL_ERROR", "request_binding", f"unknown check {check}")
        return replace(self, **{check: status})

    def to_dict(self) -> dict[str, object]:
        return {name: getattr(self, name) for name in _CHECK_NAMES}


@dataclass(frozen=True, slots=True)
class AttestationSigner:
    signer_id: str
    signer_certificate_sha256: str
    chain_fingerprints: tuple[str, ...]
    verified_roles: tuple[str, ...]
    verified_scopes: tuple[str, ...]
    valid_until: str
    revocation_evidence_sha256: str

    @classmethod
    def from_dict(cls, value: Mapping[str, object]) -> "AttestationSigner":
        return cls(
            signer_id=_required_string(value, "signer_id"),
            signer_certificate_sha256=_required_string(value, "signer_certificate_sha256"),
            chain_fingerprints=_string_tuple(value, "chain_fingerprints"),
            verified_roles=_string_tuple(value, "verified_roles"),
            verified_scopes=_string_tuple(value, "verified_scopes"),
            valid_until=_required_string(value, "valid_until"),
            revocation_evidence_sha256=_required_string(value, "revocation_evidence_sha256"),
        )

    @classmethod
    def from_verified(
        cls, signer: VerifiedSigner, authority: VerifiedAuthority
    ) -> "AttestationSigner":
        return cls(
            signer_id=authority.authority_ids[0],
            signer_certificate_sha256=signer.signer_fingerprint,
            chain_fingerprints=signer.chain_fingerprints,
            verified_roles=tuple(sorted(authority.verified_roles)),
            verified_scopes=tuple(sorted(authority.verified_scopes)),
            valid_until=_format_rfc3339(authority.valid_until),
            revocation_evidence_sha256=signer.revocation_evidence_sha256,
        )

    def to_dict(self) -> dict[str, object]:
        return {
            "signer_id": self.signer_id,
            "signer_certificate_sha256": self.signer_certificate_sha256,
            "chain_fingerprints": list(self.chain_fingerprints),
            "verified_roles": list(self.verified_roles),
            "verified_scopes": list(self.verified_scopes),
            "valid_until": self.valid_until,
            "revocation_evidence_sha256": self.revocation_evidence_sha256,
        }


@dataclass(frozen=True, slots=True)
class VerifierIdentity:
    service_id: str
    runtime_id: str
    software_version: str
    policy_bytes_sha256: str

    @classmethod
    def from_dict(cls, value: Mapping[str, object]) -> "VerifierIdentity":
        return cls(
            service_id=_required_string(value, "service_id"),
            runtime_id=_required_string(value, "runtime_id"),
            software_version=_required_string(value, "software_version"),
            policy_bytes_sha256=_required_string(value, "policy_bytes_sha256"),
        )

    def to_dict(self) -> dict[str, object]:
        return {
            "service_id": self.service_id,
            "runtime_id": self.runtime_id,
            "software_version": self.software_version,
            "policy_bytes_sha256": self.policy_bytes_sha256,
        }


@dataclass(frozen=True, slots=True)
class AttestationSignature:
    key_id: str
    algorithm: str
    certificate_sha256: str
    certificate_der: bytes
    attestation_body_sha256: str
    signature: bytes
    signature_sha256: str

    @classmethod
    def from_dict(cls, value: Mapping[str, object]) -> "AttestationSignature":
        return cls(
            key_id=_required_string(value, "key_id"),
            algorithm=_required_string(value, "algorithm"),
            certificate_sha256=_required_string(value, "certificate_sha256"),
            certificate_der=_decode_base64(
                _required_string(value, "certificate_der_base64"), "attestation certificate"
            ),
            attestation_body_sha256=_required_string(value, "attestation_body_sha256"),
            signature=_decode_base64(
                _required_string(value, "signature_base64"), "attestation signature"
            ),
            signature_sha256=_required_string(value, "signature_sha256"),
        )

    def to_dict(self) -> dict[str, object]:
        return {
            "key_id": self.key_id,
            "algorithm": self.algorithm,
            "certificate_sha256": self.certificate_sha256,
            "certificate_der_base64": base64.b64encode(self.certificate_der).decode("ascii"),
            "attestation_body_sha256": self.attestation_body_sha256,
            "signature_base64": base64.b64encode(self.signature).decode("ascii"),
            "signature_sha256": self.signature_sha256,
        }


@dataclass(frozen=True, slots=True)
class SignatureVerificationAttestation:
    schema_version: str
    attestation_id: str
    decision: str
    decision_code: str
    request_binding: RequestBinding
    verification_checks: VerificationChecks
    signer: AttestationSigner | None
    replay_claim: ReplayClaim | None
    verifier: VerifierIdentity
    issued_at: str
    attestation_signature: AttestationSignature

    @classmethod
    def from_dict(cls, value: Mapping[str, object]) -> "SignatureVerificationAttestation":
        try:
            _validate_against_schema(value, ATTESTATION_SCHEMA_PATH)
        except (ValueError, TypeError) as exc:
            raise VerificationRejected(
                "REQUEST_SCHEMA_INVALID", "schema", "attestation schema validation failed"
            ) from exc
        signer_value = value.get("signer")
        replay_value = value.get("replay_claim")
        return cls(
            schema_version=_required_string(value, "schema_version"),
            attestation_id=_required_string(value, "attestation_id"),
            decision=_required_string(value, "decision"),
            decision_code=_required_string(value, "decision_code"),
            request_binding=RequestBinding.from_dict(_required_mapping(value, "request_binding")),
            verification_checks=VerificationChecks.from_dict(
                _required_mapping(value, "verification_checks")
            ),
            signer=(
                AttestationSigner.from_dict(_as_mapping(signer_value, "signer"))
                if signer_value is not None
                else None
            ),
            replay_claim=(
                ReplayClaim.from_dict(_as_mapping(replay_value, "replay_claim"))
                if replay_value is not None
                else None
            ),
            verifier=VerifierIdentity.from_dict(_required_mapping(value, "verifier")),
            issued_at=_required_string(value, "issued_at"),
            attestation_signature=AttestationSignature.from_dict(
                _required_mapping(value, "attestation_signature")
            ),
        )

    def to_dict(self) -> dict[str, object]:
        return {
            "schema_version": self.schema_version,
            "attestation_id": self.attestation_id,
            "decision": self.decision,
            "decision_code": self.decision_code,
            "request_binding": self.request_binding.to_dict(),
            "verification_checks": self.verification_checks.to_dict(),
            "signer": self.signer.to_dict() if self.signer else None,
            "replay_claim": self.replay_claim.to_dict() if self.replay_claim else None,
            "verifier": self.verifier.to_dict(),
            "issued_at": self.issued_at,
            "attestation_signature": self.attestation_signature.to_dict(),
        }


@dataclass(frozen=True, slots=True)
class HttpResponse:
    status_code: int
    headers: Mapping[str, str]
    body: bytes


class SSLContext(Protocol):
    def wrap_socket(self, socket: object, *, server_side: bool = True) -> object: ...


@dataclass(frozen=True, slots=True)
class ServerLimits:
    max_header_bytes: int
    max_request_bytes: int
    max_response_bytes: int
    max_connections: int
    request_timeout_seconds: float
    shutdown_timeout_seconds: float


class VerifierServer(Protocol):
    def start(self) -> None: ...

    def stop_admission(self) -> None: ...

    def drain(self, deadline: datetime) -> bool: ...

    def close(self) -> None: ...


@dataclass(frozen=True, slots=True)
class SignatureServiceConfig:
    policy_path: Path
    policy_fingerprint: str
    replay_store_path: Path
    attestation_ledger_path: Path
    listen: str
    runtime_id: str
    limits: ServerLimits


class Clock(Protocol):
    def now(self) -> datetime: ...


class ShutdownSignal(Protocol):
    def wait(self) -> None: ...

    def requested(self) -> bool: ...


@dataclass(frozen=True, slots=True)
class SignatureVerificationRuntime:
    policy: LoadedTrustPolicy
    secret_resolver: SecretResolver
    replay_store: ReplayStore
    attestation_ledger: AttestationLedger
    clock: Clock
    runtime_id: str
    server_limits: ServerLimits


def canonicalize_verification_request(request: SignatureVerificationRequest) -> bytes:
    """Validate and canonicalize exactly the fields authenticated by the signer."""

    wire = request.to_dict()
    try:
        _validate_against_schema(wire, REQUEST_SCHEMA_PATH)
    except (ValueError, TypeError) as exc:
        raise VerificationRejected("REQUEST_SCHEMA_INVALID", "schema", str(exc)) from exc

    signed = request.signed_payload
    material = request.verification_material
    _require_sha256(signed.subject_payload.content_sha256, "subject payload digest")
    if len(signed.subject_payload.content) != signed.subject_payload.size_bytes:
        raise VerificationRejected(
            "CANONICALIZATION_REJECTED", "schema", "subject payload size does not match bytes"
        )
    if _sha256(signed.subject_payload.content) != signed.subject_payload.content_sha256:
        raise VerificationRejected(
            "CANONICALIZATION_REJECTED", "schema", "subject payload digest does not match bytes"
        )
    if _sha256(material.detached_signature) != material.signature_sha256:
        raise VerificationRejected(
            "CANONICALIZATION_REJECTED", "schema", "signature digest does not match bytes"
        )
    if not material.certificate_chain:
        raise VerificationRejected(
            "CANONICALIZATION_REJECTED", "schema", "certificate chain is empty"
        )
    for certificate in material.certificate_chain:
        if _sha256(certificate.certificate_der) != certificate.certificate_sha256:
            raise VerificationRejected(
                "CANONICALIZATION_REJECTED",
                "schema",
                f"certificate digest mismatch for {certificate.certificate_id}",
            )
    if material.certificate_chain[0].certificate_sha256 != signed.signer_certificate_sha256:
        raise VerificationRejected(
            "CANONICALIZATION_REJECTED", "schema", "signed leaf certificate identity mismatch"
        )
    if _certificate_chain_sha256(material.certificate_chain) != signed.certificate_chain_sha256:
        raise VerificationRejected(
            "CANONICALIZATION_REJECTED", "schema", "signed certificate-chain identity mismatch"
        )
    issued_at = _parse_rfc3339(signed.issued_at, "issued_at")
    expires_at = _parse_rfc3339(signed.expires_at, "expires_at")
    _parse_rfc3339(signed.evaluation_time, "evaluation_time")
    if expires_at <= issued_at:
        raise VerificationRejected(
            "CANONICALIZATION_REJECTED", "schema", "request validity interval is not ordered"
        )
    claimed_roles = {item.role for item in signed.claimed_authorities}
    if not set(signed.required_authority_roles).issubset(claimed_roles):
        raise VerificationRejected(
            "CANONICALIZATION_REJECTED", "schema", "required authority role lacks a signed claim"
        )
    envelope = {
        "schema_version": request.schema_version,
        "canonicalization_version": request.canonicalization_version,
        "request_id": request.request_id,
        "signed_payload": signed.to_dict(),
    }
    return _canonical_json_bytes(envelope)


def load_trust_policy(
    policy_path: Path, *, policy_fingerprint: str, secret_resolver: SecretResolver
) -> LoadedTrustPolicy:
    """Load one absolute non-symlinked policy whose exact bytes match the pin."""

    if not policy_path.is_absolute():
        raise VerificationRejected("POLICY_REJECTED", "policy", "policy path must be absolute")
    try:
        resolved = policy_path.resolve(strict=True)
    except OSError as exc:
        raise VerificationRejected("POLICY_REJECTED", "policy", "policy path is unavailable") from exc
    if resolved != policy_path or policy_path.is_symlink() or not policy_path.is_file():
        raise VerificationRejected(
            "POLICY_REJECTED", "policy", "policy path must be canonical regular non-symlink file"
        )
    try:
        policy_bytes = policy_path.read_bytes()
    except OSError as exc:
        raise VerificationDependencyError(
            "DEPENDENCY_UNAVAILABLE", "policy", "policy bytes could not be read"
        ) from exc
    if not policy_bytes or len(policy_bytes) > 1_048_576:
        raise VerificationRejected("POLICY_REJECTED", "policy", "policy byte size is invalid")
    actual_fingerprint = _sha256(policy_bytes)
    if not _SHA256_RE.fullmatch(policy_fingerprint) or actual_fingerprint != policy_fingerprint:
        raise VerificationRejected(
            "POLICY_FINGERPRINT_MISMATCH", "policy", "policy fingerprint does not match exact bytes"
        )
    try:
        raw = _load_unique_json(policy_bytes)
        _validate_against_schema(raw, POLICY_SCHEMA_PATH)
    except (ValueError, TypeError, UnicodeDecodeError, json.JSONDecodeError) as exc:
        raise VerificationRejected("POLICY_REJECTED", "policy", str(exc)) from exc
    if raw["policy_status"] != "ACTIVE":
        raise VerificationRejected("POLICY_REJECTED", "policy", "policy is not active")

    allowed_algorithms = frozenset(_string_sequence(raw["allowed_algorithms"], "allowed_algorithms"))
    anchors: list[TrustAnchor] = []
    for item in _mapping_sequence(raw["trust_anchors"], "trust_anchors"):
        der = _decode_base64(_required_string(item, "certificate_der_base64"), "trust anchor")
        digest = _required_string(item, "certificate_sha256")
        if _sha256(der) != digest:
            raise VerificationRejected("POLICY_REJECTED", "policy", "trust anchor digest mismatch")
        try:
            certificate = x509.load_der_x509_certificate(der)
        except ValueError as exc:
            raise VerificationRejected("POLICY_REJECTED", "policy", "invalid trust anchor DER") from exc
        if certificate.public_bytes(serialization.Encoding.DER) != der:
            raise VerificationRejected("POLICY_REJECTED", "policy", "non-canonical trust anchor DER")
        anchor_algorithms = frozenset(
            _string_sequence(item["allowed_algorithms"], "trust anchor algorithms")
        )
        if not anchor_algorithms.issubset(allowed_algorithms):
            raise VerificationRejected(
                "POLICY_REJECTED", "policy", "trust anchor expands the global algorithm set"
            )
        anchors.append(
            TrustAnchor(
                anchor_id=_required_string(item, "anchor_id"),
                certificate_sha256=digest,
                certificate_der=der,
                allowed_algorithms=anchor_algorithms,
            )
        )

    role_rules = tuple(_parse_role_rule(item) for item in _mapping_sequence(raw["role_rules"], "role_rules"))
    role_keys = [(rule.role, authority_id) for rule in role_rules for authority_id in rule.allowed_authority_ids]
    if len(role_keys) != len(set(role_keys)):
        raise VerificationRejected("POLICY_REJECTED", "policy", "role and authority mapping is ambiguous")

    verifier = _required_mapping(raw, "verifier_identity")
    attestation_certificate_der = _decode_base64(
        _required_string(verifier, "attestation_certificate_der_base64"),
        "attestation certificate",
    )
    attestation_certificate_sha256 = _required_string(verifier, "attestation_certificate_sha256")
    if _sha256(attestation_certificate_der) != attestation_certificate_sha256:
        raise VerificationRejected("POLICY_REJECTED", "policy", "attestation certificate digest mismatch")
    try:
        attestation_certificate = x509.load_der_x509_certificate(attestation_certificate_der)
    except ValueError as exc:
        raise VerificationRejected("POLICY_REJECTED", "policy", "invalid attestation certificate DER") from exc
    attestation_algorithm = _required_string(verifier, "attestation_algorithm")
    _require_key_algorithm_compatible(attestation_certificate.public_key(), attestation_algorithm)

    attestation_reference = SecretReference.from_dict(
        _required_mapping(raw, "attestation_signing_key_ref")
    )
    try:
        attestation_handle = secret_resolver.resolve(attestation_reference)
    except Exception as exc:
        raise VerificationDependencyError(
            "DEPENDENCY_UNAVAILABLE", "policy", "attestation signing handle is unavailable"
        ) from exc
    _validate_secret_handle(attestation_handle, attestation_reference)

    revocation = _required_mapping(raw, "revocation_policy")
    revocation_handle: SecretHandle | None = None
    client_auth_ref = revocation.get("client_auth_ref")
    if client_auth_ref is not None:
        reference = SecretReference.from_dict(_as_mapping(client_auth_ref, "client_auth_ref"))
        try:
            revocation_handle = secret_resolver.resolve(reference)
        except Exception as exc:
            raise VerificationDependencyError(
                "DEPENDENCY_UNAVAILABLE", "policy", "revocation client handle is unavailable"
            ) from exc
        _validate_secret_handle(revocation_handle, reference)

    time_policy = _required_mapping(raw, "time_policy")
    replay_policy = _required_mapping(raw, "replay_policy")
    limits = _required_mapping(raw, "resource_limits")
    return LoadedTrustPolicy(
        schema_version=_required_string(raw, "schema_version"),
        policy_id=_required_string(raw, "policy_id"),
        policy_version=_required_string(raw, "policy_version"),
        fingerprint=actual_fingerprint,
        canonicalization_version=_required_string(raw, "canonicalization_version"),
        verifier_service_id=_required_string(verifier, "service_id"),
        verifier_environment=_required_string(verifier, "deployment_environment"),
        attestation_algorithm=attestation_algorithm,
        attestation_certificate_sha256=attestation_certificate_sha256,
        attestation_certificate_der=attestation_certificate_der,
        trust_anchors=tuple(anchors),
        allowed_algorithms=allowed_algorithms,
        role_rules=role_rules,
        max_clock_skew_seconds=_required_integer(time_policy, "max_clock_skew_seconds"),
        max_request_validity_seconds=_required_integer(time_policy, "max_request_validity_seconds"),
        max_attestation_age_seconds=_required_integer(time_policy, "max_attestation_age_seconds"),
        revocation_methods=frozenset(_string_sequence(revocation["methods"], "revocation methods")),
        max_revocation_evidence_age_seconds=_required_integer(revocation, "max_evidence_age_seconds"),
        replay_namespace=_required_string(replay_policy, "namespace"),
        replay_claim_ttl_seconds=_required_integer(replay_policy, "claim_ttl_seconds"),
        max_request_bytes=_required_integer(limits, "max_request_bytes"),
        max_payload_bytes=_required_integer(limits, "max_payload_bytes"),
        max_signature_bytes=_required_integer(limits, "max_signature_bytes"),
        max_certificate_bytes=_required_integer(limits, "max_certificate_bytes"),
        max_chain_length=_required_integer(limits, "max_chain_length"),
        attestation_signing_handle=attestation_handle,
        revocation_client_handle=revocation_handle,
        revocation_checker=secret_resolver,
    )


def verify_certificate_chain(
    chain: Sequence[bytes], *, policy: LoadedTrustPolicy, verification_time: datetime
) -> VerifiedSigner:
    """Verify an ordered leaf-to-root DER chain against an exact policy anchor."""

    verification_time = _require_aware_datetime(verification_time, "verification_time")
    if not 1 <= len(chain) <= policy.max_chain_length:
        raise VerificationRejected(
            "CERTIFICATE_CHAIN_REJECTED", "certificate_chain", "certificate chain length is invalid"
        )
    certificates: list[x509.Certificate] = []
    fingerprints: list[str] = []
    for der in chain:
        if not 1 <= len(der) <= policy.max_certificate_bytes:
            raise VerificationRejected(
                "CERTIFICATE_CHAIN_REJECTED", "certificate_chain", "certificate size is invalid"
            )
        try:
            certificate = x509.load_der_x509_certificate(der)
        except ValueError as exc:
            raise VerificationRejected(
                "CERTIFICATE_CHAIN_REJECTED", "certificate_chain", "certificate DER is invalid"
            ) from exc
        if certificate.public_bytes(serialization.Encoding.DER) != der:
            raise VerificationRejected(
                "CERTIFICATE_CHAIN_REJECTED", "certificate_chain", "certificate DER is not canonical"
            )
        _reject_unknown_critical_extensions(certificate)
        _require_strong_public_key(certificate.public_key())
        _require_strong_certificate_signature(certificate)
        not_before = _certificate_not_before(certificate)
        not_after = _certificate_not_after(certificate)
        skew = timedelta(seconds=policy.max_clock_skew_seconds)
        if verification_time < not_before - skew or verification_time > not_after + skew:
            raise VerificationRejected(
                "CERTIFICATE_CHAIN_REJECTED", "time_window", "certificate is outside its validity window"
            )
        certificates.append(certificate)
        fingerprints.append(_sha256(der))

    anchor_by_digest = {item.certificate_sha256: item for item in policy.trust_anchors}
    if fingerprints[-1] not in anchor_by_digest or chain[-1] != anchor_by_digest[fingerprints[-1]].certificate_der:
        raise VerificationRejected(
            "CERTIFICATE_CHAIN_REJECTED", "certificate_chain", "chain does not end at a pinned anchor"
        )
    for index in range(len(certificates) - 1):
        child = certificates[index]
        issuer = certificates[index + 1]
        if child.issuer != issuer.subject:
            raise VerificationRejected(
                "CERTIFICATE_CHAIN_REJECTED", "certificate_chain", "certificate issuer ordering mismatch"
            )
        _require_ca_certificate(issuer)
        _verify_certificate_signature(child, issuer.public_key())
    root = certificates[-1]
    if root.issuer != root.subject:
        raise VerificationRejected(
            "CERTIFICATE_CHAIN_REJECTED", "certificate_chain", "pinned root is not self-issued"
        )
    _require_ca_certificate(root)
    _verify_certificate_signature(root, root.public_key())
    _require_leaf_certificate(certificates[0])

    identities = _certificate_identities(certificates[0], fingerprints[0])
    leaf_eku_oids = _certificate_eku_oids(certificates[0])
    matching_rules = tuple(
        rule
        for rule in policy.role_rules
        if identities.intersection(rule.allowed_authority_ids)
        and rule.required_eku_oids.issubset(leaf_eku_oids)
    )
    if not matching_rules:
        raise VerificationRejected(
            "CERTIFICATE_CHAIN_REJECTED", "certificate_chain", "leaf identity and EKU match no policy role"
        )
    try:
        revocation = policy.revocation_checker.check_revocation(
            chain,
            verification_time=verification_time,
            methods=policy.revocation_methods,
            max_evidence_age_seconds=policy.max_revocation_evidence_age_seconds,
            client_auth_handle=policy.revocation_client_handle,
        )
    except VerificationError:
        raise
    except Exception as exc:
        raise VerificationDependencyError(
            "REVOCATION_STATUS_UNAVAILABLE", "revocation", "revocation provider is unavailable"
        ) from exc
    expected_revocation_set = tuple(fingerprints[:-1] or fingerprints)
    if revocation.status == "REVOKED":
        raise VerificationRejected("CERTIFICATE_REVOKED", "revocation", "certificate is revoked")
    if revocation.status != "GOOD" or not set(expected_revocation_set).issubset(
        revocation.checked_certificate_sha256
    ):
        raise VerificationDependencyError(
            "REVOCATION_STATUS_UNAVAILABLE", "revocation", "revocation evidence is incomplete"
        )
    if revocation.produced_at > verification_time or revocation.next_update < verification_time:
        raise VerificationDependencyError(
            "REVOCATION_STATUS_UNAVAILABLE", "revocation", "revocation evidence is stale"
        )
    if verification_time - revocation.produced_at > timedelta(
        seconds=policy.max_revocation_evidence_age_seconds
    ):
        raise VerificationDependencyError(
            "REVOCATION_STATUS_UNAVAILABLE", "revocation", "revocation evidence exceeds policy age"
        )
    _require_sha256(revocation.evidence_sha256, "revocation evidence digest")

    compatible = _compatible_algorithms(certificates[0].public_key()).intersection(
        policy.allowed_algorithms
    ).intersection(anchor_by_digest[fingerprints[-1]].allowed_algorithms)
    if not compatible:
        raise VerificationRejected(
            "CERTIFICATE_CHAIN_REJECTED", "certificate_chain", "leaf key has no allowed algorithm"
        )
    return VerifiedSigner(
        signer_fingerprint=fingerprints[0],
        chain_fingerprints=tuple(fingerprints),
        signer_identities=frozenset(identities),
        asserted_roles=frozenset(rule.role for rule in matching_rules),
        asserted_scopes=frozenset(scope for rule in matching_rules for scope in rule.allowed_scopes),
        valid_until=min(_certificate_not_after(item) for item in certificates),
        revocation_evidence_sha256=revocation.evidence_sha256,
        allowed_algorithms=frozenset(compatible),
        max_payload_bytes=policy.max_payload_bytes,
        max_signature_bytes=policy.max_signature_bytes,
        public_key=certificates[0].public_key(),
    )


def verify_detached_signature(
    payload: bytes, signature: bytes, *, signer: VerifiedSigner, algorithm: str
) -> str:
    """Verify one declared algorithm over the exact supplied bytes without fallback."""

    if algorithm not in signer.allowed_algorithms:
        raise VerificationRejected(
            "SIGNATURE_REJECTED", "signature", "algorithm is not allowed for the verified signer"
        )
    if not 1 <= len(payload) <= signer.max_payload_bytes:
        raise VerificationRejected("SIGNATURE_REJECTED", "signature", "payload size is invalid")
    if not 1 <= len(signature) <= signer.max_signature_bytes:
        raise VerificationRejected("SIGNATURE_REJECTED", "signature", "signature size is invalid")
    try:
        _verify_public_key_signature(signer.public_key, payload, signature, algorithm)
    except (InvalidSignature, TypeError, ValueError) as exc:
        raise VerificationRejected(
            "SIGNATURE_REJECTED", "signature", "detached signature verification failed"
        ) from exc
    return _sha256(payload)


def verify_authority_scope(
    request: SignatureVerificationRequest,
    signer: VerifiedSigner,
    *,
    policy: LoadedTrustPolicy,
    now: datetime,
) -> VerifiedAuthority:
    """Authorize exact signed identities, roles, scopes, time, and optional CNAS scope."""

    now = _require_aware_datetime(now, "now")
    signed = request.signed_payload
    issued_at = _parse_rfc3339(signed.issued_at, "issued_at")
    expires_at = _parse_rfc3339(signed.expires_at, "expires_at")
    evaluation_time = _parse_rfc3339(signed.evaluation_time, "evaluation_time")
    skew = timedelta(seconds=policy.max_clock_skew_seconds)
    if expires_at <= issued_at or expires_at - issued_at > timedelta(
        seconds=policy.max_request_validity_seconds
    ):
        raise VerificationRejected("TIME_WINDOW_REJECTED", "time_window", "request duration is invalid")
    if now < issued_at - skew or now > expires_at + skew:
        raise VerificationRejected("TIME_WINDOW_REJECTED", "time_window", "request is outside its validity window")
    if abs(now - evaluation_time) > skew:
        raise VerificationRejected("TIME_WINDOW_REJECTED", "time_window", "evaluation time drift exceeds policy")
    if expires_at > signer.valid_until + skew:
        raise VerificationRejected("TIME_WINDOW_REJECTED", "time_window", "request outlives signer certificate")
    if signed.policy_id != policy.policy_id or signed.policy_version != policy.policy_version:
        raise VerificationRejected("POLICY_REJECTED", "policy", "request policy identity mismatch")
    if signed.policy_fingerprint_sha256 != policy.fingerprint:
        raise VerificationRejected(
            "POLICY_FINGERPRINT_MISMATCH", "policy", "request policy fingerprint mismatch"
        )
    if signed.environment_id != policy.verifier_environment:
        raise VerificationRejected(
            "AUTHORITY_SCOPE_REJECTED", "authority_scope", "request environment differs from verifier policy"
        )

    claims_by_role: dict[str, set[str]] = {}
    for claim in signed.claimed_authorities:
        claims_by_role.setdefault(claim.role, set()).add(claim.authority_id)
    verified_ids: set[str] = set()
    verified_roles: set[str] = set()
    verified_scopes: set[str] = set()
    cnas_hash: str | None = None
    for required_role in signed.required_authority_roles:
        candidates = [
            rule
            for rule in policy.role_rules
            if rule.role == required_role
            and signed.purpose in rule.allowed_purposes
            and signed.subject_type in rule.allowed_subject_types
            and signed.profile_id in rule.allowed_profile_ids
            and signed.environment_id in rule.allowed_environment_ids
            and set(signed.required_scopes).issubset(rule.allowed_scopes)
            and signer.signer_identities.intersection(rule.allowed_authority_ids)
            and claims_by_role.get(required_role, set()).intersection(rule.allowed_authority_ids)
        ]
        if len(candidates) != 1 or required_role not in signer.asserted_roles:
            raise VerificationRejected(
                "AUTHORITY_SCOPE_REJECTED",
                "authority_scope",
                f"required role {required_role} is not uniquely authorized",
            )
        rule = candidates[0]
        matched_ids = (
            signer.signer_identities
            .intersection(rule.allowed_authority_ids)
            .intersection(claims_by_role[required_role])
        )
        if len(matched_ids) != 1:
            raise VerificationRejected(
                "AUTHORITY_SCOPE_REJECTED", "authority_scope", "authority identity is ambiguous"
            )
        verified_ids.update(matched_ids)
        verified_roles.add(required_role)
        verified_scopes.update(signed.required_scopes)
        if rule.require_cnas_scope:
            if signed.cnas_context is None or "CNAS_ACCREDITATION" not in signed.required_scopes:
                raise VerificationRejected(
                    "CNAS_SCOPE_REJECTED", "cnas_scope", "required CNAS context is missing"
                )
            if rule.allowed_cnas_scope_sha256 != signed.cnas_context.scope_sha256:
                raise VerificationRejected(
                    "CNAS_SCOPE_REJECTED", "cnas_scope", "CNAS scope digest is not authorized"
                )
            cnas_hash = signed.cnas_context.scope_sha256
    if not set(signed.required_scopes).issubset(signer.asserted_scopes):
        raise VerificationRejected(
            "AUTHORITY_SCOPE_REJECTED", "authority_scope", "signer lacks a required scope"
        )
    if signed.cnas_context is not None and "CNAS_ACCREDITATION" not in signed.required_scopes:
        raise VerificationRejected(
            "CNAS_SCOPE_REJECTED", "cnas_scope", "unexpected CNAS context is not permitted"
        )
    return VerifiedAuthority(
        authority_ids=tuple(sorted(verified_ids)),
        verified_roles=frozenset(verified_roles),
        verified_scopes=frozenset(verified_scopes),
        valid_until=min(expires_at, signer.valid_until),
        cnas_scope_sha256=cnas_hash,
    )


def claim_attestation_nonce(
    store: ReplayStore,
    request: SignatureVerificationRequest,
    *,
    payload_sha256: str,
    expires_at: datetime,
) -> ReplayClaim:
    """Atomically bind one policy-purpose-environment nonce to one canonical request."""

    _require_sha256(payload_sha256, "canonical request digest")
    expires_at = _require_aware_datetime(expires_at, "expires_at")
    signed = request.signed_payload
    claim_key_sha256 = _sha256(
        _canonical_json_bytes(
            {
                "domain": "traffic-analysis-platform/topic1/replay-claim/v1",
                "policy_id": signed.policy_id,
                "policy_version": signed.policy_version,
                "purpose": signed.purpose,
                "environment_id": signed.environment_id,
                "nonce": signed.nonce,
            }
        )
    )
    request_identity_sha256 = _sha256(
        _canonical_json_bytes(
            {
                "request_id": request.request_id,
                "candidate_commit": signed.candidate_commit,
                "profile_id": signed.profile_id,
                "environment_id": signed.environment_id,
                "purpose": signed.purpose,
                "policy_fingerprint_sha256": signed.policy_fingerprint_sha256,
            }
        )
    )
    try:
        claim = store.compare_and_set(
            claim_key_sha256=claim_key_sha256,
            canonical_request_sha256=payload_sha256,
            request_identity_sha256=request_identity_sha256,
            expires_at=expires_at,
        )
    except VerificationError:
        raise
    except Exception as exc:
        raise VerificationDependencyError(
            "REPLAY_STORE_UNAVAILABLE", "replay", "replay store compare-and-set failed"
        ) from exc
    if claim.claim_key_sha256 != claim_key_sha256:
        raise VerificationDependencyError(
            "REPLAY_STORE_UNAVAILABLE", "replay", "replay store returned the wrong claim key"
        )
    if claim.disposition == "CONFLICT":
        raise VerificationRejected("REPLAY_CONFLICT", "replay", "nonce is bound to another request")
    if claim.disposition not in {"CREATED", "IDEMPOTENT"}:
        raise VerificationDependencyError(
            "REPLAY_STORE_UNAVAILABLE", "replay", "replay store returned an unknown disposition"
        )
    if claim.canonical_request_sha256 != payload_sha256 or claim.expires_at != expires_at:
        raise VerificationRejected("REPLAY_CONFLICT", "replay", "existing nonce binding differs")
    return claim


def verify_request(
    request: SignatureVerificationRequest,
    *,
    policy_path: Path,
    secret_resolver: SecretResolver,
    replay_store: ReplayStore,
    now: datetime,
) -> SignatureVerificationAttestation:
    """Run the complete fail-closed verification sequence and sign the decision."""

    now = _require_aware_datetime(now, "now")
    checks = VerificationChecks()
    canonical_payload: bytes
    canonical_error: VerificationRejected | None = None
    try:
        canonical_payload = canonicalize_verification_request(request)
        checks = checks.mark("schema", "PASS")
    except VerificationRejected as exc:
        canonical_error = exc
        canonical_payload = _canonical_json_bytes(
            {
                "schema_version": request.schema_version,
                "canonicalization_version": request.canonicalization_version,
                "request_id": request.request_id,
                "signed_payload": request.signed_payload.to_dict(),
            }
        )
        checks = checks.mark("schema", "REJECT")
    canonical_sha256 = _sha256(canonical_payload)
    policy = load_trust_policy(
        policy_path,
        policy_fingerprint=request.signed_payload.policy_fingerprint_sha256,
        secret_resolver=secret_resolver,
    )
    checks = checks.mark("policy", "PASS")
    if request.canonicalization_version != policy.canonicalization_version:
        canonical_error = VerificationRejected(
            "CANONICALIZATION_REJECTED", "schema", "canonicalization version differs from policy"
        )
    if canonical_error is not None:
        return _build_signed_attestation(
            request=request,
            canonical_request_sha256=canonical_sha256,
            decision="REJECT",
            decision_code=canonical_error.code,
            checks=checks,
            signer=None,
            replay_claim=None,
            policy=policy,
            secret_resolver=secret_resolver,
            now=now,
        )

    signer: VerifiedSigner | None = None
    authority: VerifiedAuthority | None = None
    replay_claim: ReplayClaim | None = None
    try:
        signer = verify_certificate_chain(
            tuple(item.certificate_der for item in request.verification_material.certificate_chain),
            policy=policy,
            verification_time=now,
        )
        checks = checks.mark("certificate_chain", "PASS").mark("revocation", "PASS")
        verified_payload_sha256 = verify_detached_signature(
            canonical_payload,
            request.verification_material.detached_signature,
            signer=signer,
            algorithm=request.signed_payload.signature_algorithm,
        )
        if verified_payload_sha256 != canonical_sha256:
            raise VerificationDependencyError(
                "INTERNAL_ERROR", "signature", "verified payload digest changed unexpectedly"
            )
        checks = checks.mark("signature", "PASS")
        authority = verify_authority_scope(request, signer, policy=policy, now=now)
        checks = checks.mark("authority_scope", "PASS").mark("time_window", "PASS")
        checks = checks.mark("cnas_scope", "PASS")
        claim_expiry = min(
            _parse_rfc3339(request.signed_payload.expires_at, "expires_at"),
            now + timedelta(seconds=policy.replay_claim_ttl_seconds),
        )
        replay_claim = claim_attestation_nonce(
            replay_store,
            request,
            payload_sha256=canonical_sha256,
            expires_at=claim_expiry,
        )
        checks = checks.mark("replay", "PASS").mark("request_binding", "PASS")
    except VerificationRejected as exc:
        checks = checks.mark(exc.check, "REJECT")
        return _build_signed_attestation(
            request=request,
            canonical_request_sha256=canonical_sha256,
            decision="REJECT",
            decision_code=exc.code,
            checks=checks,
            signer=None,
            replay_claim=None,
            policy=policy,
            secret_resolver=secret_resolver,
            now=now,
        )
    except VerificationDependencyError as exc:
        checks = checks.mark(exc.check, "ERROR")
        return _build_signed_attestation(
            request=request,
            canonical_request_sha256=canonical_sha256,
            decision="ERROR",
            decision_code=exc.code,
            checks=checks,
            signer=None,
            replay_claim=None,
            policy=policy,
            secret_resolver=secret_resolver,
            now=now,
        )
    if signer is None or authority is None or replay_claim is None:
        raise VerificationDependencyError(
            "INTERNAL_ERROR", "request_binding", "verification completed without required outputs"
        )
    return _build_signed_attestation(
        request=request,
        canonical_request_sha256=canonical_sha256,
        decision="PASS",
        decision_code="VERIFIED",
        checks=checks,
        signer=AttestationSigner.from_verified(signer, authority),
        replay_claim=replay_claim,
        policy=policy,
        secret_resolver=secret_resolver,
        now=now,
    )


def _build_signed_attestation(
    *,
    request: SignatureVerificationRequest,
    canonical_request_sha256: str,
    decision: str,
    decision_code: str,
    checks: VerificationChecks,
    signer: AttestationSigner | None,
    replay_claim: ReplayClaim | None,
    policy: LoadedTrustPolicy,
    secret_resolver: SecretResolver,
    now: datetime,
) -> SignatureVerificationAttestation:
    issued_at = _format_rfc3339(now)
    binding = RequestBinding.from_request(request, canonical_request_sha256)
    verifier = VerifierIdentity(
        service_id=policy.verifier_service_id,
        runtime_id=f"runtime-{_sha256((policy.verifier_service_id + policy.fingerprint).encode())[:24]}",
        software_version=SOFTWARE_VERSION,
        policy_bytes_sha256=policy.fingerprint,
    )
    attestation_id = "T1-SIGATT-" + _sha256(
        _canonical_json_bytes(
            {
                "request_id": request.request_id,
                "canonical_request_sha256": canonical_request_sha256,
                "decision": decision,
                "decision_code": decision_code,
                "issued_at": issued_at,
                "verifier": verifier.to_dict(),
            }
        )
    )
    unsigned = {
        "schema_version": "1.0.0",
        "attestation_id": attestation_id,
        "decision": decision,
        "decision_code": decision_code,
        "request_binding": binding.to_dict(),
        "verification_checks": checks.to_dict(),
        "signer": signer.to_dict() if signer else None,
        "replay_claim": replay_claim.to_dict() if replay_claim else None,
        "verifier": verifier.to_dict(),
        "issued_at": issued_at,
    }
    body = _canonical_json_bytes(unsigned)
    try:
        signature = secret_resolver.sign(
            policy.attestation_signing_handle, body, policy.attestation_algorithm
        )
    except Exception as exc:
        raise VerificationDependencyError(
            "ATTESTATION_SIGNING_FAILED", "request_binding", "attestation signing failed"
        ) from exc
    if not isinstance(signature, bytes) or not signature:
        raise VerificationDependencyError(
            "ATTESTATION_SIGNING_FAILED", "request_binding", "attestation signer returned invalid bytes"
        )
    try:
        certificate = x509.load_der_x509_certificate(policy.attestation_certificate_der)
        _verify_public_key_signature(
            certificate.public_key(), body, signature, policy.attestation_algorithm
        )
    except (ValueError, InvalidSignature, TypeError) as exc:
        raise VerificationDependencyError(
            "ATTESTATION_SIGNING_FAILED", "request_binding", "attestation signature self-check failed"
        ) from exc
    signature_record = AttestationSignature(
        key_id=policy.attestation_signing_handle.reference,
        algorithm=policy.attestation_algorithm,
        certificate_sha256=policy.attestation_certificate_sha256,
        certificate_der=policy.attestation_certificate_der,
        attestation_body_sha256=_sha256(body),
        signature=signature,
        signature_sha256=_sha256(signature),
    )
    attestation = SignatureVerificationAttestation(
        schema_version="1.0.0",
        attestation_id=attestation_id,
        decision=decision,
        decision_code=decision_code,
        request_binding=binding,
        verification_checks=checks,
        signer=signer,
        replay_claim=replay_claim,
        verifier=verifier,
        issued_at=issued_at,
        attestation_signature=signature_record,
    )
    try:
        _validate_against_schema(attestation.to_dict(), ATTESTATION_SCHEMA_PATH)
    except (ValueError, TypeError) as exc:
        raise VerificationDependencyError(
            "INTERNAL_ERROR", "request_binding", "constructed attestation violates its schema"
        ) from exc
    return attestation


def _parse_role_rule(value: Mapping[str, object]) -> RoleRule:
    cnas = value.get("allowed_cnas_scope_sha256")
    if cnas is not None and not isinstance(cnas, str):
        raise VerificationRejected("POLICY_REJECTED", "policy", "CNAS scope digest must be a string or null")
    return RoleRule(
        role=_required_string(value, "role"),
        allowed_authority_ids=frozenset(_string_sequence(value["allowed_authority_ids"], "authority ids")),
        allowed_purposes=frozenset(_string_sequence(value["allowed_purposes"], "purposes")),
        allowed_subject_types=frozenset(_string_sequence(value["allowed_subject_types"], "subject types")),
        allowed_scopes=frozenset(_string_sequence(value["allowed_scopes"], "scopes")),
        allowed_profile_ids=frozenset(_string_sequence(value["allowed_profile_ids"], "profile ids")),
        allowed_environment_ids=frozenset(_string_sequence(value["allowed_environment_ids"], "environment ids")),
        required_eku_oids=frozenset(_string_sequence(value["required_eku_oids"], "EKU OIDs")),
        require_cnas_scope=_required_boolean(value, "require_cnas_scope"),
        allowed_cnas_scope_sha256=cnas,
    )


def _validate_secret_handle(handle: SecretHandle, reference: SecretReference) -> None:
    if not isinstance(handle, SecretHandle):
        raise VerificationDependencyError(
            "DEPENDENCY_UNAVAILABLE", "policy", "secret provider returned a non-opaque handle"
        )
    if (
        handle.provider,
        handle.reference,
        handle.version,
        handle.purpose,
    ) != (reference.provider, reference.reference, reference.version, reference.purpose):
        raise VerificationDependencyError(
            "DEPENDENCY_UNAVAILABLE", "policy", "secret handle identity differs from its reference"
        )
    if not handle.handle_id or len(handle.handle_id) > 256:
        raise VerificationDependencyError(
            "DEPENDENCY_UNAVAILABLE", "policy", "secret handle identity is invalid"
        )


def _certificate_identities(certificate: x509.Certificate, fingerprint: str) -> set[str]:
    identities = {fingerprint, certificate.subject.rfc4514_string()}
    try:
        san = certificate.extensions.get_extension_for_oid(ExtensionOID.SUBJECT_ALTERNATIVE_NAME).value
    except x509.ExtensionNotFound:
        return identities
    identities.update(san.get_values_for_type(x509.RFC822Name))
    identities.update(san.get_values_for_type(x509.DNSName))
    identities.update(str(item) for item in san.get_values_for_type(x509.UniformResourceIdentifier))
    return identities


def _certificate_eku_oids(certificate: x509.Certificate) -> frozenset[str]:
    try:
        eku = certificate.extensions.get_extension_for_oid(ExtensionOID.EXTENDED_KEY_USAGE).value
    except x509.ExtensionNotFound:
        return frozenset()
    return frozenset(item.dotted_string for item in eku)


def _require_leaf_certificate(certificate: x509.Certificate) -> None:
    try:
        basic = certificate.extensions.get_extension_for_oid(ExtensionOID.BASIC_CONSTRAINTS).value
    except x509.ExtensionNotFound as exc:
        raise VerificationRejected(
            "CERTIFICATE_CHAIN_REJECTED", "certificate_chain", "leaf lacks basic constraints"
        ) from exc
    if basic.ca:
        raise VerificationRejected(
            "CERTIFICATE_CHAIN_REJECTED", "certificate_chain", "leaf certificate is a CA"
        )
    try:
        key_usage = certificate.extensions.get_extension_for_oid(ExtensionOID.KEY_USAGE).value
    except x509.ExtensionNotFound as exc:
        raise VerificationRejected(
            "CERTIFICATE_CHAIN_REJECTED", "certificate_chain", "leaf lacks key usage"
        ) from exc
    if not key_usage.digital_signature:
        raise VerificationRejected(
            "CERTIFICATE_CHAIN_REJECTED", "certificate_chain", "leaf cannot sign"
        )
    if ExtendedKeyUsageOID.CODE_SIGNING.dotted_string not in _certificate_eku_oids(certificate):
        raise VerificationRejected(
            "CERTIFICATE_CHAIN_REJECTED", "certificate_chain", "leaf lacks code-signing EKU"
        )


def _require_ca_certificate(certificate: x509.Certificate) -> None:
    try:
        basic = certificate.extensions.get_extension_for_oid(ExtensionOID.BASIC_CONSTRAINTS).value
        key_usage = certificate.extensions.get_extension_for_oid(ExtensionOID.KEY_USAGE).value
    except x509.ExtensionNotFound as exc:
        raise VerificationRejected(
            "CERTIFICATE_CHAIN_REJECTED", "certificate_chain", "issuer lacks CA constraints"
        ) from exc
    if not basic.ca or not key_usage.key_cert_sign:
        raise VerificationRejected(
            "CERTIFICATE_CHAIN_REJECTED", "certificate_chain", "issuer is not an authorized CA"
        )


_KNOWN_CRITICAL_EXTENSION_OIDS = frozenset(
    {
        ExtensionOID.BASIC_CONSTRAINTS.dotted_string,
        ExtensionOID.KEY_USAGE.dotted_string,
        ExtensionOID.EXTENDED_KEY_USAGE.dotted_string,
        ExtensionOID.SUBJECT_ALTERNATIVE_NAME.dotted_string,
        ExtensionOID.AUTHORITY_KEY_IDENTIFIER.dotted_string,
        ExtensionOID.SUBJECT_KEY_IDENTIFIER.dotted_string,
        ExtensionOID.NAME_CONSTRAINTS.dotted_string,
        ExtensionOID.CERTIFICATE_POLICIES.dotted_string,
        ExtensionOID.POLICY_CONSTRAINTS.dotted_string,
        ExtensionOID.INHIBIT_ANY_POLICY.dotted_string,
    }
)


def _reject_unknown_critical_extensions(certificate: x509.Certificate) -> None:
    unknown = [
        extension.oid.dotted_string
        for extension in certificate.extensions
        if extension.critical and extension.oid.dotted_string not in _KNOWN_CRITICAL_EXTENSION_OIDS
    ]
    if unknown:
        raise VerificationRejected(
            "CERTIFICATE_CHAIN_REJECTED",
            "certificate_chain",
            f"unknown critical certificate extensions: {sorted(unknown)}",
        )


def _verify_certificate_signature(certificate: x509.Certificate, issuer_key: object) -> None:
    try:
        if isinstance(issuer_key, rsa.RSAPublicKey):
            issuer_key.verify(
                certificate.signature,
                certificate.tbs_certificate_bytes,
                padding.PKCS1v15(),
                certificate.signature_hash_algorithm,
            )
        elif isinstance(issuer_key, ec.EllipticCurvePublicKey):
            issuer_key.verify(
                certificate.signature,
                certificate.tbs_certificate_bytes,
                ec.ECDSA(certificate.signature_hash_algorithm),
            )
        elif isinstance(issuer_key, (ed25519.Ed25519PublicKey, ed448.Ed448PublicKey)):
            issuer_key.verify(certificate.signature, certificate.tbs_certificate_bytes)
        else:
            raise TypeError("unsupported issuer key")
    except (InvalidSignature, TypeError, ValueError) as exc:
        raise VerificationRejected(
            "CERTIFICATE_CHAIN_REJECTED", "certificate_chain", "certificate signature is invalid"
        ) from exc


def _require_strong_certificate_signature(certificate: x509.Certificate) -> None:
    algorithm = certificate.signature_hash_algorithm
    if algorithm is not None and algorithm.name not in {"sha256", "sha384", "sha512"}:
        raise VerificationRejected(
            "CERTIFICATE_CHAIN_REJECTED", "certificate_chain", "certificate uses a weak signature hash"
        )


def _require_strong_public_key(public_key: object) -> None:
    if isinstance(public_key, rsa.RSAPublicKey):
        if public_key.key_size < 3072:
            raise VerificationRejected(
                "CERTIFICATE_CHAIN_REJECTED", "certificate_chain", "RSA key is smaller than 3072 bits"
            )
        return
    if isinstance(public_key, ec.EllipticCurvePublicKey):
        if public_key.curve.name not in {"secp256r1", "secp384r1"}:
            raise VerificationRejected(
                "CERTIFICATE_CHAIN_REJECTED", "certificate_chain", "elliptic curve is not allowed"
            )
        return
    if isinstance(public_key, (ed25519.Ed25519PublicKey, ed448.Ed448PublicKey)):
        return
    raise VerificationRejected(
        "CERTIFICATE_CHAIN_REJECTED", "certificate_chain", "public key type is not allowed"
    )


def _compatible_algorithms(public_key: object) -> frozenset[str]:
    if isinstance(public_key, ed25519.Ed25519PublicKey):
        return frozenset({"ED25519"})
    if isinstance(public_key, ec.EllipticCurvePublicKey):
        if public_key.curve.name == "secp256r1":
            return frozenset({"ECDSA_P256_SHA256"})
        if public_key.curve.name == "secp384r1":
            return frozenset({"ECDSA_P384_SHA384"})
    if isinstance(public_key, rsa.RSAPublicKey) and public_key.key_size >= 3072:
        return frozenset({"RSA_PSS_SHA256", "RSA_PSS_SHA384"})
    return frozenset()


def _require_key_algorithm_compatible(public_key: object, algorithm: str) -> None:
    if algorithm not in _compatible_algorithms(public_key):
        raise VerificationRejected(
            "POLICY_REJECTED", "policy", "attestation key and algorithm are incompatible"
        )


def _verify_public_key_signature(
    public_key: object, payload: bytes, signature: bytes, algorithm: str
) -> None:
    _require_key_algorithm_compatible_for_signature(public_key, algorithm)
    if algorithm == "ED25519":
        assert isinstance(public_key, ed25519.Ed25519PublicKey)
        public_key.verify(signature, payload)
    elif algorithm == "ECDSA_P256_SHA256":
        assert isinstance(public_key, ec.EllipticCurvePublicKey)
        public_key.verify(signature, payload, ec.ECDSA(hashes.SHA256()))
    elif algorithm == "ECDSA_P384_SHA384":
        assert isinstance(public_key, ec.EllipticCurvePublicKey)
        public_key.verify(signature, payload, ec.ECDSA(hashes.SHA384()))
    elif algorithm in {"RSA_PSS_SHA256", "RSA_PSS_SHA384"}:
        assert isinstance(public_key, rsa.RSAPublicKey)
        digest = hashes.SHA256() if algorithm == "RSA_PSS_SHA256" else hashes.SHA384()
        public_key.verify(
            signature,
            payload,
            padding.PSS(mgf=padding.MGF1(digest), salt_length=digest.digest_size),
            digest,
        )
    else:
        raise ValueError("unsupported signature algorithm")


def _require_key_algorithm_compatible_for_signature(public_key: object, algorithm: str) -> None:
    if algorithm not in _compatible_algorithms(public_key):
        raise ValueError("key and signature algorithm are incompatible")


def _certificate_not_before(certificate: x509.Certificate) -> datetime:
    value = getattr(certificate, "not_valid_before_utc", None)
    if value is None:
        value = certificate.not_valid_before.replace(tzinfo=timezone.utc)
    return value


def _certificate_not_after(certificate: x509.Certificate) -> datetime:
    value = getattr(certificate, "not_valid_after_utc", None)
    if value is None:
        value = certificate.not_valid_after.replace(tzinfo=timezone.utc)
    return value


def _certificate_chain_sha256(chain: Sequence[CertificateMaterial]) -> str:
    return _sha256(
        _canonical_json_bytes(
            [
                {
                    "certificate_id": item.certificate_id,
                    "certificate_sha256": item.certificate_sha256,
                }
                for item in chain
            ]
        )
    )


def _canonical_json_bytes(value: object) -> bytes:
    return json.dumps(
        value, ensure_ascii=False, sort_keys=True, separators=(",", ":"), allow_nan=False
    ).encode("utf-8")


def _sha256(value: bytes) -> str:
    return hashlib.sha256(value).hexdigest()


def _require_sha256(value: str, label: str) -> None:
    if not _SHA256_RE.fullmatch(value):
        raise VerificationRejected(
            "CANONICALIZATION_REJECTED", "schema", f"{label} is not lowercase SHA-256"
        )


def _decode_base64(value: str, label: str) -> bytes:
    try:
        decoded = base64.b64decode(value, validate=True)
    except (ValueError, TypeError) as exc:
        raise VerificationRejected(
            "REQUEST_SCHEMA_INVALID", "schema", f"{label} is not canonical base64"
        ) from exc
    if base64.b64encode(decoded).decode("ascii") != value:
        raise VerificationRejected(
            "REQUEST_SCHEMA_INVALID", "schema", f"{label} is not canonical base64"
        )
    return decoded


def _parse_rfc3339(value: str, label: str) -> datetime:
    if not _RFC3339_UTC_RE.fullmatch(value):
        raise VerificationRejected(
            "CANONICALIZATION_REJECTED", "schema", f"{label} is not canonical RFC3339 UTC"
        )
    try:
        parsed = datetime.fromisoformat(value[:-1] + "+00:00")
    except ValueError as exc:
        raise VerificationRejected(
            "CANONICALIZATION_REJECTED", "schema", f"{label} is invalid"
        ) from exc
    return parsed.astimezone(timezone.utc)


def _format_rfc3339(value: datetime) -> str:
    value = _require_aware_datetime(value, "timestamp").astimezone(timezone.utc)
    if value.microsecond:
        return value.isoformat(timespec="microseconds").replace("+00:00", "Z")
    return value.isoformat(timespec="seconds").replace("+00:00", "Z")


def _require_aware_datetime(value: datetime, label: str) -> datetime:
    if value.tzinfo is None or value.utcoffset() is None:
        raise VerificationRejected(
            "TIME_WINDOW_REJECTED", "time_window", f"{label} must be timezone-aware"
        )
    return value.astimezone(timezone.utc)


def _required_string(value: Mapping[str, object], key: str) -> str:
    item = value.get(key)
    if not isinstance(item, str) or not item:
        raise VerificationRejected("REQUEST_SCHEMA_INVALID", "schema", f"{key} must be a non-empty string")
    return item


def _required_integer(value: Mapping[str, object], key: str) -> int:
    item = value.get(key)
    if not isinstance(item, int) or isinstance(item, bool):
        raise VerificationRejected("REQUEST_SCHEMA_INVALID", "schema", f"{key} must be an integer")
    return item


def _required_boolean(value: Mapping[str, object], key: str) -> bool:
    item = value.get(key)
    if not isinstance(item, bool):
        raise VerificationRejected("REQUEST_SCHEMA_INVALID", "schema", f"{key} must be a boolean")
    return item


def _as_mapping(value: object, label: str) -> Mapping[str, object]:
    if not isinstance(value, Mapping):
        raise VerificationRejected("REQUEST_SCHEMA_INVALID", "schema", f"{label} must be an object")
    return value


def _required_mapping(value: Mapping[str, object], key: str) -> Mapping[str, object]:
    return _as_mapping(value.get(key), key)


def _required_sequence(value: Mapping[str, object], key: str) -> Sequence[object]:
    item = value.get(key)
    if not isinstance(item, list):
        raise VerificationRejected("REQUEST_SCHEMA_INVALID", "schema", f"{key} must be an array")
    return item


def _string_tuple(value: Mapping[str, object], key: str) -> tuple[str, ...]:
    return tuple(_string_sequence(value.get(key), key))


def _string_sequence(value: object, label: str) -> tuple[str, ...]:
    if not isinstance(value, list) or any(not isinstance(item, str) for item in value):
        raise VerificationRejected("REQUEST_SCHEMA_INVALID", "schema", f"{label} must be a string array")
    return tuple(value)


def _mapping_sequence(value: object, label: str) -> tuple[Mapping[str, object], ...]:
    if not isinstance(value, list):
        raise VerificationRejected("REQUEST_SCHEMA_INVALID", "schema", f"{label} must be an object array")
    return tuple(_as_mapping(item, f"{label} item") for item in value)


def _load_unique_json(payload: bytes) -> Mapping[str, object]:
    def unique_object(pairs: list[tuple[str, object]]) -> dict[str, object]:
        result: dict[str, object] = {}
        for key, value in pairs:
            if key in result:
                raise ValueError(f"duplicate JSON property: {key}")
            result[key] = value
        return result

    value = json.loads(payload.decode("utf-8"), object_pairs_hook=unique_object)
    if not isinstance(value, Mapping):
        raise ValueError("JSON document must be an object")
    return value


def _validate_against_schema(instance: object, schema_path: Path) -> None:
    """Strict local validator for the schema subset used by these contracts."""

    root = json.loads(schema_path.read_text(encoding="utf-8"))
    known = {
        "$schema", "$id", "$defs", "$ref", "title", "description", "type", "const", "enum",
        "required", "properties", "additionalProperties", "items", "minItems", "maxItems",
        "uniqueItems", "minLength", "maxLength", "pattern", "minimum", "maximum", "allOf",
        "anyOf", "oneOf", "not", "if", "then", "else",
    }

    def resolve(ref: str) -> Mapping[str, object]:
        if not ref.startswith("#/"):
            raise ValueError(f"unsupported schema reference {ref}")
        current: object = root
        for component in ref[2:].split("/"):
            if not isinstance(current, Mapping):
                raise ValueError(f"invalid schema reference {ref}")
            current = current[component.replace("~1", "/").replace("~0", "~")]
        if not isinstance(current, Mapping):
            raise ValueError(f"schema reference is not an object: {ref}")
        return current

    def matches(value: object, schema: Mapping[str, object], location: str) -> bool:
        try:
            walk(value, schema, location)
            return True
        except ValueError:
            return False

    def walk(value: object, schema: Mapping[str, object], location: str) -> None:
        unknown = sorted(set(schema) - known)
        if unknown:
            raise ValueError(f"unsupported schema keywords at {location}: {unknown}")
        if "$ref" in schema:
            walk(value, resolve(str(schema["$ref"])), location)
            return
        for child in schema.get("allOf", []):
            walk(value, _as_schema(child), location)
        if "anyOf" in schema and not any(
            matches(value, _as_schema(child), location) for child in schema["anyOf"]
        ):
            raise ValueError(f"schema anyOf mismatch at {location}")
        if "oneOf" in schema and sum(
            matches(value, _as_schema(child), location) for child in schema["oneOf"]
        ) != 1:
            raise ValueError(f"schema oneOf mismatch at {location}")
        if "not" in schema and matches(value, _as_schema(schema["not"]), location):
            raise ValueError(f"schema not constraint failed at {location}")
        if "if" in schema:
            branch = "then" if matches(value, _as_schema(schema["if"]), location) else "else"
            if branch in schema:
                walk(value, _as_schema(schema[branch]), location)
        if "const" in schema and value != schema["const"]:
            raise ValueError(f"schema const mismatch at {location}")
        if "enum" in schema and value not in schema["enum"]:
            raise ValueError(f"schema enum mismatch at {location}: {value!r}")
        expected = schema.get("type")
        allowed = expected if isinstance(expected, list) else [expected]
        if expected is not None and not any(_matches_type(value, str(item)) for item in allowed):
            raise ValueError(f"schema type mismatch at {location}: expected {expected}")
        if isinstance(value, Mapping):
            required = set(schema.get("required", []))
            missing = sorted(required - set(value))
            if missing:
                raise ValueError(f"schema missing required fields at {location}: {missing}")
            properties = schema.get("properties", {})
            if not isinstance(properties, Mapping):
                raise ValueError(f"schema properties is invalid at {location}")
            if schema.get("additionalProperties") is False:
                extra = sorted(set(value) - set(properties))
                if extra:
                    raise ValueError(f"schema extra fields at {location}: {extra}")
            for key, child_value in value.items():
                if key in properties:
                    walk(child_value, _as_schema(properties[key]), f"{location}.{key}")
        elif isinstance(value, list):
            if len(value) < int(schema.get("minItems", 0)):
                raise ValueError(f"schema minItems failed at {location}")
            if "maxItems" in schema and len(value) > int(schema["maxItems"]):
                raise ValueError(f"schema maxItems failed at {location}")
            if schema.get("uniqueItems"):
                encoded = [_canonical_json_bytes(item) for item in value]
                if len(encoded) != len(set(encoded)):
                    raise ValueError(f"schema uniqueItems failed at {location}")
            if "items" in schema:
                for index, item in enumerate(value):
                    walk(item, _as_schema(schema["items"]), f"{location}[{index}]")
        elif isinstance(value, str):
            if len(value) < int(schema.get("minLength", 0)):
                raise ValueError(f"schema minLength failed at {location}")
            if "maxLength" in schema and len(value) > int(schema["maxLength"]):
                raise ValueError(f"schema maxLength failed at {location}")
            if "pattern" in schema and not re.search(str(schema["pattern"]), value):
                raise ValueError(f"schema pattern failed at {location}: {value!r}")
        elif isinstance(value, (int, float)) and not isinstance(value, bool):
            if "minimum" in schema and value < schema["minimum"]:
                raise ValueError(f"schema minimum failed at {location}")
            if "maximum" in schema and value > schema["maximum"]:
                raise ValueError(f"schema maximum failed at {location}")

    walk(instance, root, "$")


def _as_schema(value: object) -> Mapping[str, object]:
    if not isinstance(value, Mapping):
        raise ValueError("schema node must be an object")
    return value


def _matches_type(value: object, expected: str) -> bool:
    return (
        (expected == "object" and isinstance(value, Mapping))
        or (expected == "array" and isinstance(value, list))
        or (expected == "string" and isinstance(value, str))
        or (expected == "integer" and isinstance(value, int) and not isinstance(value, bool))
        or (expected == "number" and isinstance(value, (int, float)) and not isinstance(value, bool))
        or (expected == "boolean" and isinstance(value, bool))
        or (expected == "null" and value is None)
    )
