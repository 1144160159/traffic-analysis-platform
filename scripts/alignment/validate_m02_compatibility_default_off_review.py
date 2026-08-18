#!/usr/bin/env python3
"""Validate M02 compatibility/default-off review hash closure and independence."""

from __future__ import annotations

import argparse
import copy
import hashlib
import json
from pathlib import Path
from typing import Any, Callable

from build_topic1_task_registry import validate_against_schema


REPO = Path(__file__).resolve().parents[2]
SCHEMA = REPO / "contracts/alignment/m02-compatibility-default-off-review.schema.json"


def semantic_sha256(value: Any) -> str:
    return hashlib.sha256(json.dumps(value, ensure_ascii=False, sort_keys=True, separators=(",", ":")).encode()).hexdigest()


def signed_projection(payload: dict[str, Any]) -> dict[str, Any]:
    return {key: value for key, value in payload.items() if key not in {"signed_payload_sha256", "attestations"}}


def validate(payload: dict[str, Any]) -> None:
    validate_against_schema(payload, SCHEMA)
    if payload["target_locator_exact_set_sha256"] != semantic_sha256(sorted(payload["target_locators"])):
        raise ValueError("compatibility target locator exact-set hash drifted")
    candidate_hash = payload["candidate"]["manifest_sha256"]
    for field in ("compatibility_contract_ref", "verification_plan_ref", "rollback_plan_ref"):
        if payload[field]["candidate_manifest_sha256"] != candidate_hash:
            raise ValueError(f"compatibility {field} crosses candidate manifests")
    allowed_states = {
        "ADDITIVE_DEFAULT_OFF": {"DISABLED_UNLESS_EXPLICITLY_ENABLED"},
        "ADDITIVE_BACKWARD_COMPATIBLE": {"LEGACY_BEHAVIOR_PRESERVED", "DISABLED_UNLESS_EXPLICITLY_ENABLED"},
        "NO_RUNTIME_BEHAVIOR_CHANGE": {"NO_RUNTIME_CHANGE"},
        "TEST_OR_EVIDENCE_ONLY": {"NO_RUNTIME_CHANGE"},
    }
    if payload["default_runtime_state"] not in allowed_states[payload["compatibility_mode"]]:
        raise ValueError("compatibility mode/default runtime state mismatch")
    expected_signed = semantic_sha256(signed_projection(payload))
    if payload["signed_payload_sha256"] != expected_signed:
        raise ValueError("compatibility signed payload hash drifted")
    identities = [item["reviewer_identity"] for item in payload["attestations"]]
    if len(identities) != len(set(identities)):
        raise ValueError("compatibility review reuses a reviewer identity")
    roles = {item["role"] for item in payload["attestations"]}
    if "DOMAIN_OWNER" not in roles or "COMPATIBILITY_REVIEWER" not in roles:
        raise ValueError("compatibility review lacks domain-owner and compatibility-reviewer roles")
    if any(item["payload_sha256"] != expected_signed for item in payload["attestations"]):
        raise ValueError("compatibility attestation payload hash drifted")


def fixture() -> dict[str, Any]:
    manifest_hash = "1" * 64
    payload: dict[str, Any] = {
        "schema_version": "1.0.0", "artifact_kind": "M02_COMPATIBILITY_DEFAULT_OFF_REVIEW_RECEIPT",
        "receipt_id": "CDR-T1-M02-P117", "atomic_pr_id": "T1-M02-P117-REF-n001-l17",
        "candidate": {"commit": "2" * 40, "manifest_path": "doc/02_acceptance/topic1/m02/candidate.json", "manifest_sha256": manifest_hash},
        "target_locators": ["scripts/alignment/build_topic1_task_registry.py#M02_EXTERNAL_ACTIVITY_DEFINITIONS"],
        "target_locator_exact_set_sha256": "", "compatibility_mode": "NO_RUNTIME_BEHAVIOR_CHANGE",
        "default_runtime_state": "NO_RUNTIME_CHANGE", "legacy_contract_preserved": True,
        "compatibility_contract_ref": {"artifact_kind": "COMPATIBILITY_CONTRACT", "path": "contracts/compatibility.json", "sha256": "3" * 64, "candidate_manifest_sha256": manifest_hash},
        "verification_plan_ref": {"artifact_kind": "ATOMIC_PR_PLAN_MANIFEST", "path": "plans/verification.json", "sha256": "4" * 64, "candidate_manifest_sha256": manifest_hash},
        "rollback_plan_ref": {"artifact_kind": "ATOMIC_PR_PLAN_MANIFEST", "path": "plans/rollback.json", "sha256": "5" * 64, "candidate_manifest_sha256": manifest_hash},
        "review_disposition": "APPROVED", "unresolved_p0": [], "signed_payload_sha256": "", "attestations": [],
        "proof_ceiling": "COMPATIBILITY_DEFAULT_OFF_REVIEW_ONLY_NOT_LOCATOR_RESOLUTION_IMPLEMENTATION_EXECUTION_SWITCH_OR_ACCEPTANCE",
    }
    payload["target_locator_exact_set_sha256"] = semantic_sha256(sorted(payload["target_locators"]))
    payload["signed_payload_sha256"] = semantic_sha256(signed_projection(payload))
    payload["attestations"] = [
        {"role": "DOMAIN_OWNER", "reviewer_identity": "owner@example.invalid", "payload_sha256": payload["signed_payload_sha256"], "policy_id": "M02-COMPATIBILITY-V1", "signature_artifact_ref": {"path": "signatures/owner.sig", "sha256": "6" * 64}},
        {"role": "COMPATIBILITY_REVIEWER", "reviewer_identity": "reviewer@example.invalid", "payload_sha256": payload["signed_payload_sha256"], "policy_id": "M02-COMPATIBILITY-V1", "signature_artifact_ref": {"path": "signatures/reviewer.sig", "sha256": "7" * 64}},
    ]
    return payload


def expect_failure(label: str, payload: dict[str, Any], mutate: Callable[[dict[str, Any]], None], expected: str) -> None:
    candidate = copy.deepcopy(payload); mutate(candidate)
    try:
        validate(candidate)
    except (ValueError, TypeError) as exc:
        if expected not in str(exc): raise ValueError(f"negative case {label} hit wrong assertion: {exc}") from exc
        return
    raise ValueError(f"negative case {label} did not fail")


def self_test() -> None:
    payload = fixture(); validate(payload)
    expect_failure("locator hash", payload, lambda item: item.update({"target_locator_exact_set_sha256": "0" * 64}), "locator exact-set hash drifted")
    expect_failure("cross candidate", payload, lambda item: item["compatibility_contract_ref"].update({"candidate_manifest_sha256": "0" * 64}), "crosses candidate manifests")
    expect_failure("mode state", payload, lambda item: item.update({"default_runtime_state": "LEGACY_BEHAVIOR_PRESERVED"}), "mode/default runtime state mismatch")
    expect_failure("signed payload", payload, lambda item: item.update({"signed_payload_sha256": "0" * 64}), "signed payload hash drifted")
    expect_failure("identity reuse", payload, lambda item: item["attestations"][1].update({"reviewer_identity": item["attestations"][0]["reviewer_identity"]}), "reuses a reviewer identity")
    expect_failure("missing role", payload, lambda item: item["attestations"][1].update({"role": "QA_SRE_PERFORMANCE_EXPERT"}), "lacks domain-owner")
    expect_failure("attestation drift", payload, lambda item: item["attestations"][0].update({"payload_sha256": "0" * 64}), "attestation payload hash drifted")
    expect_failure("unresolved p0", payload, lambda item: item["unresolved_p0"].append("P0"), "schema maxItems failed")


def main() -> int:
    parser = argparse.ArgumentParser(); parser.add_argument("path", nargs="?"); parser.add_argument("--self-test", action="store_true"); args = parser.parse_args()
    if args.self_test:
        self_test(); print("PASS M02 compatibility/default-off review: 1 positive and 8 targeted locator/candidate/mode/hash/reviewer/P0 negative cases")
        print("PROOF_CEILING COMPATIBILITY_DEFAULT_OFF_REVIEW_ONLY_NOT_LOCATOR_RESOLUTION_IMPLEMENTATION_EXECUTION_SWITCH_OR_ACCEPTANCE"); return 0
    if not args.path: parser.error("path is required unless --self-test is used")
    validate(json.loads(Path(args.path).read_text(encoding="utf-8"))); print(f"PASS {args.path}"); return 0


if __name__ == "__main__":
    raise SystemExit(main())
