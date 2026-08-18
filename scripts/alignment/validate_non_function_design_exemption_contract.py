#!/usr/bin/env python3
"""Validate signed non-function design exemptions and their hash closure."""

from __future__ import annotations

import argparse
import copy
import hashlib
import json
from pathlib import Path
from typing import Any, Callable

from build_topic1_task_registry import validate_against_schema


REPO = Path(__file__).resolve().parents[2]
SCHEMA = REPO / "contracts/alignment/non-function-design-exemption.schema.json"


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
    if payload["target_locator_exact_set_sha256"] != semantic_sha256(sorted(payload["target_locators"])):
        raise ValueError("non-function target locator exact-set hash drifted")
    candidate_hash = payload["candidate"]["manifest_sha256"]
    for field in ("specialized_contract_ref", "verification_plan_ref", "rollback_plan_ref"):
        if payload[field]["candidate_manifest_sha256"] != candidate_hash:
            raise ValueError(f"non-function {field} crosses candidate manifests")
    expected_signed = semantic_sha256(signed_projection(payload))
    if payload["signed_payload_sha256"] != expected_signed:
        raise ValueError("non-function signed payload hash drifted")
    identities = [item["reviewer_identity"] for item in payload["attestations"]]
    if len(identities) != len(set(identities)):
        raise ValueError("non-function exemption reuses a reviewer identity")
    roles = {item["role"] for item in payload["attestations"]}
    if "DOMAIN_OWNER" not in roles or not roles.intersection({
        "QA_SRE_PERFORMANCE_EXPERT", "MAINTAINABILITY_RED_TEAM", "ADJUDICATOR",
    }):
        raise ValueError("non-function exemption lacks independent review roles")
    if any(item["payload_sha256"] != expected_signed for item in payload["attestations"]):
        raise ValueError("non-function attestation payload hash drifted")


def fixture() -> dict[str, Any]:
    manifest_hash = "1" * 64
    payload: dict[str, Any] = {
        "schema_version": "1.0.0",
        "artifact_kind": "NON_FUNCTION_DESIGN_EXEMPTION_RECEIPT",
        "exemption_id": "NFDE-T1-M02-P101",
        "atomic_pr_id": "T1-M02-P101-CTR-n001-l01",
        "candidate": {
            "commit": "2" * 40,
            "manifest_path": "doc/02_acceptance/topic1/m02/candidate.json",
            "manifest_sha256": manifest_hash,
        },
        "surface_kind": "PROTO_CONTRACT",
        "exemption_reason_code": "DECLARATIVE_CONTRACT_REQUIRES_SPECIALIZED_REVIEW",
        "target_locators": ["proto/traffic/v1/common.proto#traffic.v1.EventHeader"],
        "target_locator_exact_set_sha256": "",
        "specialized_contract_ref": {
            "artifact_kind": "PROTO_COMPATIBILITY_CONTRACT",
            "path": "doc/02_acceptance/topic1/m02/proto-contract.json",
            "sha256": "3" * 64,
            "candidate_manifest_sha256": manifest_hash,
        },
        "verification_plan_ref": {
            "artifact_kind": "ATOMIC_PR_PLAN_MANIFEST",
            "path": "doc/02_acceptance/topic1/m02/verification-plan.json",
            "sha256": "4" * 64,
            "candidate_manifest_sha256": manifest_hash,
        },
        "rollback_plan_ref": {
            "artifact_kind": "ATOMIC_PR_PLAN_MANIFEST",
            "path": "doc/02_acceptance/topic1/m02/rollback-plan.json",
            "sha256": "5" * 64,
            "candidate_manifest_sha256": manifest_hash,
        },
        "review_disposition": "APPROVED",
        "unresolved_p0": [],
        "signed_payload_sha256": "",
        "attestations": [],
        "proof_ceiling": "NON_FUNCTION_DESIGN_EXEMPTION_ONLY_NOT_EXECUTION_IMPLEMENTATION_TEST_OR_ACCEPTANCE",
    }
    payload["target_locator_exact_set_sha256"] = semantic_sha256(sorted(payload["target_locators"]))
    payload["signed_payload_sha256"] = semantic_sha256(signed_projection(payload))
    payload["attestations"] = [
        {
            "role": "DOMAIN_OWNER",
            "reviewer_identity": "owner@example.invalid",
            "payload_sha256": payload["signed_payload_sha256"],
            "policy_id": "M02-NON-FUNCTION-REVIEW-V1",
            "signature_artifact_ref": {"path": "signatures/owner.sig", "sha256": "6" * 64},
        },
        {
            "role": "MAINTAINABILITY_RED_TEAM",
            "reviewer_identity": "reviewer@example.invalid",
            "payload_sha256": payload["signed_payload_sha256"],
            "policy_id": "M02-NON-FUNCTION-REVIEW-V1",
            "signature_artifact_ref": {"path": "signatures/reviewer.sig", "sha256": "7" * 64},
        },
    ]
    return payload


def expect_failure(
    label: str,
    payload: dict[str, Any],
    mutate: Callable[[dict[str, Any]], None],
    expected_error: str,
) -> None:
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
        "locator hash drift", payload,
        lambda item: item.update({"target_locator_exact_set_sha256": "0" * 64}),
        "target locator exact-set hash drifted",
    )
    expect_failure(
        "cross candidate contract", payload,
        lambda item: item["specialized_contract_ref"].update({"candidate_manifest_sha256": "0" * 64}),
        "specialized_contract_ref crosses candidate manifests",
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
        lambda item: item["attestations"][1].update({"role": "DOMAIN_OWNER"}),
        "lacks independent review roles",
    )
    expect_failure(
        "attestation payload drift", payload,
        lambda item: item["attestations"][0].update({"payload_sha256": "0" * 64}),
        "attestation payload hash drifted",
    )
    expect_failure(
        "unresolved p0", payload,
        lambda item: item["unresolved_p0"].append("P0 unresolved"),
        "schema maxItems failed at $.unresolved_p0",
    )


def main() -> int:
    parser = argparse.ArgumentParser()
    parser.add_argument("path", nargs="?")
    parser.add_argument("--self-test", action="store_true")
    args = parser.parse_args()
    if args.self_test:
        self_test()
        print("PASS non-function design exemption: 1 positive and 7 targeted hash/candidate/reviewer/role/P0 negative cases")
        return 0
    if not args.path:
        parser.error("path is required unless --self-test is used")
    validate(json.loads(Path(args.path).read_text(encoding="utf-8")))
    print(f"PASS {args.path}")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
