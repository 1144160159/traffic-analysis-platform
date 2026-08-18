#!/usr/bin/env python3
"""Validate function-design review receipt hash closure and reviewer independence."""

from __future__ import annotations

import argparse
import copy
import hashlib
import json
from pathlib import Path
from typing import Any, Callable

from build_topic1_task_registry import validate_against_schema


REPO = Path(__file__).resolve().parents[2]
SCHEMA = REPO / "contracts/alignment/function-design-review-receipt.schema.json"


def semantic_sha256(value: Any) -> str:
    body = json.dumps(value, ensure_ascii=False, sort_keys=True, separators=(",", ":")).encode("utf-8")
    return hashlib.sha256(body).hexdigest()


def signed_projection(payload: dict[str, Any]) -> dict[str, Any]:
    return {
        key: value
        for key, value in payload.items()
        if key not in {"signed_payload_sha256", "attestations"}
    }


def validate(payload: dict[str, Any]) -> None:
    validate_against_schema(payload, SCHEMA)
    candidate_hash = payload["candidate"]["manifest_sha256"]
    for field in ("pattern_decision_ref", "code_unit_contract_ref", "negative_test_manifest_ref"):
        if payload[field]["candidate_manifest_sha256"] != candidate_hash:
            raise ValueError(f"function review {field} crosses candidate manifests")
    expected_signed = semantic_sha256(signed_projection(payload))
    if payload["signed_payload_sha256"] != expected_signed:
        raise ValueError("function review signed payload hash drifted")
    identities = [item["reviewer_identity"] for item in payload["attestations"]]
    if len(identities) != len(set(identities)):
        raise ValueError("function review reuses a reviewer identity")
    roles = {item["role"] for item in payload["attestations"]}
    if "LANGUAGE_OWNER" not in roles or not roles.intersection({
        "QA_SRE_PERFORMANCE_EXPERT", "MAINTAINABILITY_RED_TEAM", "ADJUDICATOR",
    }):
        raise ValueError("function review lacks language-owner and independent roles")
    if any(item["payload_sha256"] != expected_signed for item in payload["attestations"]):
        raise ValueError("function review attestation payload hash drifted")


def fixture() -> dict[str, Any]:
    manifest_hash = "1" * 64
    payload: dict[str, Any] = {
        "schema_version": "1.0.0",
        "artifact_kind": "FUNCTION_DESIGN_REVIEW_RECEIPT",
        "receipt_id": "FDR-T1-M02-P165",
        "atomic_pr_id": "T1-M02-P165-WRT-n004-l21",
        "candidate": {
            "commit": "2" * 40,
            "manifest_path": "doc/02_acceptance/topic1/m02/candidate.json",
            "manifest_sha256": manifest_hash,
        },
        "pattern_decision_ref": {
            "artifact_kind": "FINAL_PATTERN_DECISION",
            "decision_id": "PD-M02-P165",
            "path": "doc/02_acceptance/topic1/m02/pattern-decision.json",
            "sha256": "3" * 64,
            "candidate_manifest_sha256": manifest_hash,
        },
        "code_unit_contract_ref": {
            "artifact_kind": "CODE_UNIT_CONTRACT",
            "path": "doc/02_acceptance/topic1/m02/code-unit-contract.json",
            "sha256": "4" * 64,
            "candidate_manifest_sha256": manifest_hash,
        },
        "function_exact_set_sha256": "5" * 64,
        "test_oracle_exact_set_sha256": "6" * 64,
        "negative_test_manifest_ref": {
            "artifact_kind": "NEGATIVE_TEST_MANIFEST",
            "path": "doc/02_acceptance/topic1/m02/negative-tests.json",
            "sha256": "7" * 64,
            "candidate_manifest_sha256": manifest_hash,
        },
        "review_disposition": "UNIFIED",
        "unresolved_p0": [],
        "vetoes": [],
        "signed_payload_sha256": "",
        "attestations": [],
        "proof_ceiling": "FUNCTION_DESIGN_REVIEW_ONLY_NOT_EXECUTION_OR_IMPLEMENTATION_ACCEPTANCE",
    }
    payload["signed_payload_sha256"] = semantic_sha256(signed_projection(payload))
    payload["attestations"] = [
        {
            "role": "LANGUAGE_OWNER",
            "reviewer_identity": "owner@example.invalid",
            "payload_sha256": payload["signed_payload_sha256"],
            "policy_id": "M02-FUNCTION-REVIEW-V1",
            "signature_artifact_ref": {"path": "signatures/owner.sig", "sha256": "8" * 64},
        },
        {
            "role": "MAINTAINABILITY_RED_TEAM",
            "reviewer_identity": "reviewer@example.invalid",
            "payload_sha256": payload["signed_payload_sha256"],
            "policy_id": "M02-FUNCTION-REVIEW-V1",
            "signature_artifact_ref": {"path": "signatures/reviewer.sig", "sha256": "9" * 64},
        },
    ]
    return payload


def expect_failure(label: str, payload: dict[str, Any], mutate: Callable[[dict[str, Any]], None], expected_error: str) -> None:
    candidate = copy.deepcopy(payload)
    mutate(candidate)
    try:
        validate(candidate)
    except (ValueError, TypeError) as exc:
        if expected_error not in str(exc):
            raise ValueError(f"negative case {label} hit wrong assertion: {exc}") from exc
        return
    raise ValueError(f"negative case {label} did not fail")


def self_test() -> None:
    payload = fixture()
    validate(payload)
    expect_failure(
        "cross candidate pattern", payload,
        lambda item: item["pattern_decision_ref"].update({"candidate_manifest_sha256": "0" * 64}),
        "pattern_decision_ref crosses candidate manifests",
    )
    expect_failure(
        "cross candidate code unit", payload,
        lambda item: item["code_unit_contract_ref"].update({"candidate_manifest_sha256": "0" * 64}),
        "code_unit_contract_ref crosses candidate manifests",
    )
    expect_failure(
        "signed payload drift", payload,
        lambda item: item.update({"signed_payload_sha256": "0" * 64}),
        "signed payload hash drifted",
    )
    expect_failure(
        "reviewer reuse", payload,
        lambda item: item["attestations"][1].update({"reviewer_identity": item["attestations"][0]["reviewer_identity"]}),
        "reuses a reviewer identity",
    )
    expect_failure(
        "missing independent role", payload,
        lambda item: item["attestations"][1].update({"role": "LANGUAGE_OWNER"}),
        "lacks language-owner and independent roles",
    )
    expect_failure(
        "attestation payload drift", payload,
        lambda item: item["attestations"][0].update({"payload_sha256": "0" * 64}),
        "attestation payload hash drifted",
    )
    expect_failure(
        "unified veto", payload,
        lambda item: item["vetoes"].append("veto remains"),
        "schema maxItems failed at $.vetoes",
    )


def main() -> int:
    parser = argparse.ArgumentParser()
    parser.add_argument("path", nargs="?")
    parser.add_argument("--self-test", action="store_true")
    args = parser.parse_args()
    if args.self_test:
        self_test()
        print("PASS function design review receipt: 1 positive and 7 targeted candidate/hash/reviewer/role/veto negative cases")
        return 0
    if not args.path:
        parser.error("path is required unless --self-test is used")
    validate(json.loads(Path(args.path).read_text(encoding="utf-8")))
    print(f"PASS {args.path}")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
