#!/usr/bin/env python3
"""Validate the P917 domain approval exact-set without granting execution.

Shape validation alone cannot prove quorum: two rows may reuse one signer or
sign a stale body.  This validator recomputes every local hash and the exact
approval payload, requires one attestation per required role and two distinct
signers, and resolves the signature receipt hashes.  Cryptographic trust is
still delegated to the M01 trusted-verifier contract referenced by P917.
"""

from __future__ import annotations

import argparse
import copy
import hashlib
import json
from pathlib import Path
from typing import Any

from build_topic1_task_registry import (
    require_trusted_signature_verifier,
    validate_against_schema,
)


ROOT = Path(__file__).resolve().parents[2]
SCHEMA = ROOT / "contracts/alignment/asset-upsert-source-precedence-approval.schema.json"
SIGNATURE_SCHEMA = ROOT / "contracts/alignment/asset-upsert-source-precedence-signature-receipt.schema.json"
SEMANTIC_RESULT_SCHEMA = ROOT / "contracts/alignment/asset-upsert-source-precedence-test-result.schema.json"
SUBJECT = ROOT / "contracts/alignment/asset-upsert-source-precedence.v1.json"
REQUIRED_ROLES = {"multi-source-data-owner", "asset-service-owner"}


def canonical_bytes(value: Any) -> bytes:
    return json.dumps(value, ensure_ascii=False, sort_keys=True, separators=(",", ":")).encode("utf-8")


def sha(path: Path) -> str:
    return hashlib.sha256(path.read_bytes()).hexdigest()


def digest(value: Any) -> str:
    return hashlib.sha256(canonical_bytes(value)).hexdigest()


def load(path: Path) -> dict[str, Any]:
    value = json.loads(path.read_text(encoding="utf-8"))
    if not isinstance(value, dict):
        raise ValueError(f"expected JSON object: {path}")
    return value


def approval_payload(value: dict[str, Any]) -> dict[str, Any]:
    return {
        "candidate_manifest_sha256": value["candidate_manifest_sha256"],
        "profile_id": value["profile_id"],
        "environment_id": value["environment_id"],
        "subject": value["subject"],
        "semantic_validation_result": value["semantic_validation_result"],
        "decision": value["decision"],
        "required_roles": sorted(value["required_roles"]),
        "quorum": value["quorum"],
        "proof_ceiling": value["proof_ceiling"],
    }


def verify_typed_signature_receipt(
    signature_receipt: dict[str, Any],
    approval: dict[str, Any],
    attestation: dict[str, Any],
) -> None:
    """Validate the typed binding, then require the protected M01 verifier.

    A JSON object whose hashes are internally consistent is not cryptographic
    evidence.  The current M01 adapter is intentionally fail-closed, so this
    call blocks every real P917 instance until the protected verifier is
    installed and verifies the exact receipt payload.
    """
    validate_against_schema(signature_receipt, SIGNATURE_SCHEMA)
    if (
        signature_receipt["candidate_manifest_sha256"] != approval["candidate_manifest_sha256"]
        or signature_receipt["profile_id"] != approval["profile_id"]
        or signature_receipt["environment_id"] != approval["environment_id"]
        or signature_receipt["approval_id"] != approval["approval_id"]
        or signature_receipt["role"] != attestation["role"]
        or signature_receipt["signer_identity"] != attestation["signer_identity"]
        or signature_receipt["payload_sha256"] != attestation["payload_sha256"]
        or signature_receipt["policy_id"] != attestation["policy_id"]
        or signature_receipt["verification_status"] != "PASS"
    ):
        raise ValueError("typed trusted signature receipt differs from approval attestation")
    require_trusted_signature_verifier(
        f"{approval['approval_id']} {attestation['role']} exact signature receipt"
    )


def validate_semantics(
    value: dict[str, Any], *, resolve_files: bool = True,
    subject_value: dict[str, Any] | None = None,
    semantic_result_value: dict[str, Any] | None = None,
) -> None:
    validate_against_schema(value, SCHEMA)
    roles = [item["role"] for item in value["attestations"]]
    signers = [item["signer_identity"] for item in value["attestations"]]
    if set(value["required_roles"]) != REQUIRED_ROLES or set(roles) != REQUIRED_ROLES:
        raise ValueError("approval role exact-set mismatch")
    if len(roles) != len(set(roles)) or len(signers) != len(set(signers)):
        raise ValueError("approval requires one attestation per role and distinct signers")

    subject = subject_value
    semantic_result = semantic_result_value
    if resolve_files:
        subject_path = (ROOT / value["subject"]["path"]).resolve()
        semantic_path = (ROOT / value["semantic_validation_result"]["path"]).resolve()
        if not subject_path.is_relative_to(ROOT) or not semantic_path.is_relative_to(ROOT):
            raise ValueError("approval reference escapes repository")
        if not subject_path.is_file() or sha(subject_path) != value["subject"]["sha256"]:
            raise ValueError("approval subject hash is stale")
        if not semantic_path.is_file() or sha(semantic_path) != value["semantic_validation_result"]["sha256"]:
            raise ValueError("approval semantic result hash is stale")
        subject = load(subject_path)
        semantic_result = load(semantic_path)
        validate_against_schema(semantic_result, SEMANTIC_RESULT_SCHEMA)
        for attestation in value["attestations"]:
            receipt_path = (ROOT / attestation["signature_receipt"]["path"]).resolve()
            if (
                not receipt_path.is_relative_to(ROOT)
                or not receipt_path.is_file()
                or sha(receipt_path) != attestation["signature_receipt"]["sha256"]
            ):
                raise ValueError("approval signature receipt is missing or stale")
            signature_receipt = load(receipt_path)
            verify_typed_signature_receipt(signature_receipt, value, attestation)
    if subject is None or semantic_result is None:
        raise ValueError("approval subject and semantic result are required")
    if subject["candidate_manifest_sha256"] != value["candidate_manifest_sha256"]:
        raise ValueError("approval crosses candidate identity")
    if set(subject["actions"]) != set(value["subject"]["action_exact_set"]):
        raise ValueError("approval action exact-set differs from subject")
    expected_field_hash = digest(sorted(item["field"] for item in subject["field_rules"]))
    if value["subject"]["field_exact_set_sha256"] != expected_field_hash:
        raise ValueError("approval field exact-set digest mismatch")
    if (
        semantic_result.get("result") != "PASS"
        or semantic_result.get("contract_sha256") != value["subject"]["sha256"]
        or not semantic_result.get("self_test")
    ):
        raise ValueError("approval semantic result is not an exact PASS for this subject")
    if (
        semantic_result.get("candidate_manifest_sha256") != value["candidate_manifest_sha256"]
        or semantic_result.get("profile_id") != value["profile_id"]
        or semantic_result.get("environment_id") != value["environment_id"]
    ):
        raise ValueError("approval semantic result crosses candidate/profile/environment identity")
    semantic_sources = semantic_result.get("source_blob_sha256")
    if not isinstance(semantic_sources, dict):
        raise ValueError("approval semantic result lacks candidate-bound source hashes")
    # Only implementation sources belong to the candidate manifest.  The
    # subject contract and validator are downstream reviewed artifacts and
    # are checked through their dedicated hashes to keep the graph acyclic.
    required_semantic_sources = {
        semantic_result.get("asset_record_path"): semantic_result.get("asset_record_sha256"),
    }
    if semantic_sources != required_semantic_sources:
        raise ValueError("approval semantic result source exact-set is not candidate-bound")
    validator_path = (ROOT / semantic_result.get("validator_path", "")).resolve()
    if (
        not validator_path.is_relative_to(ROOT)
        or not validator_path.is_file()
        or sha(validator_path) != semantic_result.get("validator_sha256")
    ):
        raise ValueError("approval semantic validator artifact is missing or stale")
    expected_payload = digest(approval_payload(value))
    if any(item["payload_sha256"] != expected_payload for item in value["attestations"]):
        raise ValueError("approval attestation does not bind the canonical approval payload")


def synthetic() -> tuple[dict[str, Any], dict[str, Any], dict[str, Any]]:
    subject = load(SUBJECT)
    subject_sha = sha(SUBJECT)
    field_hash = digest(sorted(item["field"] for item in subject["field_rules"]))
    value: dict[str, Any] = {
        "schema_version": "1.0.0",
        "artifact_kind": "ASSET_UPSERT_SOURCE_PRECEDENCE_APPROVAL",
        "approval_id": "APPROVAL-T1-M06-P917-SELFTEST",
        "atomic_pr_id": "T1-M06-P917-IDX-n004-source-precedence-approval",
        "candidate_manifest_sha256": subject["candidate_manifest_sha256"],
        "profile_id": "T1-ASSET-SOURCE-PRECEDENCE-V1",
        "environment_id": "selftest-protected-verifier",
        "subject": {
            "path": "contracts/alignment/asset-upsert-source-precedence.v1.json",
            "sha256": subject_sha,
            "contract_id": subject["contract_id"],
            "action_exact_set": subject["actions"],
            "field_exact_set_sha256": field_hash,
        },
        "semantic_validation_result": {
            "path": "doc/02_acceptance/topic1/work-orders/t1-m06-p916-tst-pre-n004-source-precedence-verification/test-result.json",
            "sha256": "1" * 64,
            "status": "PASS",
        },
        "decision": {
            "observation_create_asset_type": "unknown",
            "observation_create_status": "active",
            "observation_create_criticality": 0,
            "stale_observation_revision_policy": "ADVANCE_REVISION_PRESERVE_FACTS_AND_EMIT_ACCEPTED_AUDIT_EVENT",
            "provenance_scope": "ACTION_CLASS_V1_NO_PERSISTED_PER_FIELD_PROVENANCE",
        },
        "required_roles": sorted(REQUIRED_ROLES),
        "quorum": {"required": 2, "distinct_signers": 2},
        "attestations": [
            {"role": "multi-source-data-owner", "signer_identity": "owner-a", "payload_sha256": "0" * 64, "policy_id": "TOPIC1-DOMAIN-APPROVAL-V1", "signature_receipt": {"path": "doc/selftest-a.json", "sha256": "2" * 64}},
            {"role": "asset-service-owner", "signer_identity": "owner-b", "payload_sha256": "0" * 64, "policy_id": "TOPIC1-DOMAIN-APPROVAL-V1", "signature_receipt": {"path": "doc/selftest-b.json", "sha256": "3" * 64}},
        ],
        "status": "APPROVED",
        "blockers": [],
        "proof_ceiling": "DOMAIN_POLICY_APPROVAL_ONLY_NOT_IMPLEMENTATION_OR_EXECUTION_AUTHORIZATION",
    }
    payload_hash = digest(approval_payload(value))
    for item in value["attestations"]:
        item["payload_sha256"] = payload_hash
    asset_sha = sha(ROOT / "go/control-plane/internal/asset/config/config.go")
    validator_path = "scripts/alignment/validate_asset_upsert_source_precedence.py"
    validator_sha = sha(ROOT / validator_path)
    semantic = {
        "result": "PASS",
        "contract_sha256": subject_sha,
        "self_test": True,
        "candidate_manifest_sha256": subject["candidate_manifest_sha256"],
        "profile_id": value["profile_id"],
        "environment_id": value["environment_id"],
        "asset_record_path": "go/control-plane/internal/asset/config/config.go",
        "asset_record_sha256": asset_sha,
        "validator_path": validator_path,
        "validator_sha256": validator_sha,
        "source_blob_sha256": {
            "go/control-plane/internal/asset/config/config.go": asset_sha,
        },
    }
    return value, subject, semantic


def self_test() -> None:
    good, subject, semantic = synthetic()
    validate_semantics(good, resolve_files=False, subject_value=subject, semantic_result_value=semantic)
    mutations = {
        "same-signer": lambda x: x["attestations"][1].update(signer_identity="owner-a"),
        "duplicate-role": lambda x: x["attestations"][1].update(role="multi-source-data-owner"),
        "stale-payload": lambda x: x["decision"].update(observation_create_status="inactive"),
        "wrong-field-set": lambda x: x["subject"].update(field_exact_set_sha256="f" * 64),
        "wrong-semantic-subject": lambda x: None,
        "cross-candidate-semantic-result": lambda x: None,
    }
    for name, mutate in mutations.items():
        bad = copy.deepcopy(good)
        bad_semantic = copy.deepcopy(semantic)
        mutate(bad)
        if name == "wrong-semantic-subject":
            bad_semantic["contract_sha256"] = "f" * 64
        if name == "cross-candidate-semantic-result":
            bad_semantic["candidate_manifest_sha256"] = "e" * 64
        try:
            validate_semantics(bad, resolve_files=False, subject_value=subject, semantic_result_value=bad_semantic)
        except ValueError:
            continue
        raise AssertionError(f"approval negative accepted: {name}")

    # Even a perfectly self-consistent JSON receipt with arbitrary 64-hex
    # verifier fields must not be accepted as cryptographic evidence.
    attestation = good["attestations"][0]
    fake_receipt = {
        "schema_version": "1.0.0",
        "artifact_kind": "TRUSTED_DOMAIN_APPROVAL_SIGNATURE_RECEIPT",
        "receipt_id": "SIG-P917-SELF-REPORTED-PASS",
        "candidate_manifest_sha256": good["candidate_manifest_sha256"],
        "profile_id": good["profile_id"],
        "environment_id": good["environment_id"],
        "approval_id": good["approval_id"],
        "role": attestation["role"],
        "signer_identity": attestation["signer_identity"],
        "payload_sha256": attestation["payload_sha256"],
        "purpose": "ASSET_SOURCE_PRECEDENCE_DOMAIN_APPROVAL",
        "policy_id": attestation["policy_id"],
        "verifier": {
            "service_identity": "self-reported-verifier",
            "deployment_manifest_sha256": "a" * 64,
            "trust_policy_sha256": "b" * 64,
            "request_sha256": "c" * 64,
            "attestation_sha256": "d" * 64,
        },
        "verification_status": "PASS",
        "verified_at": "2026-08-12T00:00:00Z",
        "proof_ceiling": "ONE_TRUSTED_DOMAIN_SIGNATURE_ONLY_NOT_QUORUM_OR_EXECUTION_AUTHORIZATION",
    }
    try:
        verify_typed_signature_receipt(fake_receipt, good, attestation)
    except ValueError as exc:
        if "trusted cryptographic signature verification is not installed" not in str(exc):
            raise
    else:
        raise AssertionError("approval negative accepted: self-reported fake PASS receipt")


def main() -> int:
    parser = argparse.ArgumentParser()
    parser.add_argument("--instance", type=Path)
    parser.add_argument("--self-test", action="store_true")
    args = parser.parse_args()
    if args.self_test:
        self_test()
        print("PASS approval negatives: same signer, duplicate role, stale payload, wrong field set, wrong semantic subject, cross-candidate semantic result, self-reported fake PASS receipt")
    if args.instance is not None:
        validate_semantics(load(args.instance.resolve()))
        print("PASS P917 domain approval exact-set, quorum, payload and local hash closure")
    if not args.self_test and args.instance is None:
        parser.error("--self-test or --instance is required")
    print("PROOF_CEILING DOMAIN_POLICY_APPROVAL_ONLY_NOT_IMPLEMENTATION_OR_EXECUTION_AUTHORIZATION")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
