#!/usr/bin/env python3
"""Validate M02 external-activity receipt v1.1 semantics without trusting signatures."""

from __future__ import annotations

import argparse
import copy
import json
from pathlib import Path
from typing import Any, Callable

from build_topic1_task_registry import validate_against_schema


REPO = Path(__file__).resolve().parents[2]
SCHEMA = REPO / "contracts/alignment/external-activity-receipt.schema.json"
M02_TYPES = {"SCOPED_CANARY", "PROFILE_APPROVAL", "PROTECTED_MERGE"}
ROLES = ["PROJECT_OWNER", "TEST_OWNER", "ACCEPTANCE_AUTHORITY"]
INPUT_IDS = {
    "SCOPED_CANARY": [
        "implementation-candidate-identity-manifest", "rollout-body",
        "n001-n012-current-task-index-set", "consumer-receipt-set",
        "rollback-observation-plan",
    ],
    "PROFILE_APPROVAL": [
        "implementation-candidate-identity-manifest", "capture-profile-contract",
        "counter-contract", "offline-source-manifest", "runtime-environment-manifest",
        "m02-external-activity-enabling-index",
    ],
    "PROTECTED_MERGE": [
        "implementation-candidate-identity-manifest", "current-premerge-pass-run",
        "promotion-intent", "target-branch-policy",
    ],
}
OUTPUT_IDS = {
    "SCOPED_CANARY": [
        "deployed-runtime-manifest", "canary-result", "rollback-result", "observation-result",
    ],
    "PROFILE_APPROVAL": ["signed-profile-approval"],
    "PROTECTED_MERGE": ["protected-merge-receipt"],
}


def hashes(ids: list[str]) -> list[dict[str, str]]:
    return [
        {"artifact_id": artifact_id, "sha256": f"{index:064x}"}
        for index, artifact_id in enumerate(ids, start=1)
    ]


def sample(activity_type: str) -> dict[str, Any]:
    inputs = hashes(INPUT_IDS[activity_type])
    outputs = hashes(OUTPUT_IDS[activity_type])
    input_map = {item["artifact_id"]: item["sha256"] for item in inputs}
    output_map = {item["artifact_id"]: item["sha256"] for item in outputs}
    if activity_type == "SCOPED_CANARY":
        payload = {
            "payload_type": activity_type,
            "rollout_body_sha256": input_map["rollout-body"],
            "scope_sha256": f"{21:064x}",
            "deployed_digests_sha256": output_map["deployed-runtime-manifest"],
            "canary_started_at": "2026-08-13T00:00:00Z",
            "canary_finished_at": "2026-08-13T00:10:00Z",
            "stop_reason": "bounded observation window completed",
            "canary_result_sha256": output_map["canary-result"],
            "rollback_result_sha256": output_map["rollback-result"],
            "observation_result_sha256": output_map["observation-result"],
        }
    elif activity_type == "PROFILE_APPROVAL":
        payload = {
            "payload_type": activity_type,
            "profile_contract_sha256": input_map["capture-profile-contract"],
            "counter_contract_sha256": input_map["counter-contract"],
            "offline_source_manifest_sha256": input_map["offline-source-manifest"],
            "environment_manifest_sha256": input_map["runtime-environment-manifest"],
            "profile_id": "M02-COVERED-PROFILE",
            "required_authority_roles": ROLES,
            "quorum": 3,
            "distinct_signer_count": 3,
            "authority_signatures": [
                {
                    "role": role,
                    "signer_identity": f"signer-{index}",
                    "signature_artifact": f"doc/02_acceptance/external/signature-{index}.sig",
                    "signature_sha256": f"{30 + index:064x}",
                    "trust_policy_role": role,
                }
                for index, role in enumerate(ROLES, start=1)
            ],
            "decision": "APPROVE",
            "valid_from": "2026-08-13T00:00:00Z",
            "valid_until": "2026-08-14T00:00:00Z",
            "signed_profile_approval_sha256": output_map["signed-profile-approval"],
        }
    else:
        payload = {
            "payload_type": activity_type,
            "implementation_commit": "1" * 40,
            "implementation_tree_sha256": f"{41:064x}",
            "merge_commit": "2" * 40,
            "merge_tree_sha256": f"{42:064x}",
            "allowed_path_diff_sha256": f"{43:064x}",
            "target_branch": "main",
            "target_branch_policy_sha256": input_map["target-branch-policy"],
            "premerge_run_sha256": input_map["current-premerge-pass-run"],
            "promotion_intent_sha256": input_map["promotion-intent"],
            "protected_merge_receipt_sha256": output_map["protected-merge-receipt"],
        }
    return {
        "schema_version": "1.1.0",
        "activity_id": f"EXT-SELFTEST-{activity_type}",
        "activity_type": activity_type,
        "run_id": "run-00000001",
        "instance_id": f"EXT-SELFTEST-{activity_type}-run-00000001",
        "authority": "external self-test authority",
        "authority_scope": "contract semantics only; no trusted signature or execution claim",
        "candidate_manifest_sha256": input_map["implementation-candidate-identity-manifest"],
        "profile_id": "M02-COVERED-PROFILE",
        "input_hashes": inputs,
        "output_hashes": outputs,
        "activity_payload": payload,
        "started_at": "2026-08-13T00:00:00Z",
        "finished_at": "2026-08-13T00:10:00Z",
        "result": "BLOCKED",
        "signed_payload_artifact": "doc/02_acceptance/external/selftest/signed-payload.json",
        "signed_payload_sha256": "b" * 64,
        "signature_artifact": "doc/02_acceptance/external/selftest/signature.sig",
        "signature_sha256": "c" * 64,
        "signature_verification": {
            "status": "FAIL",
            "signature_algorithm": "SELFTEST-NOT-TRUSTED",
            "signer_identity": "selftest-only",
            "certificate_or_key_id": "selftest-only",
            "verifier": "selftest-only",
            "verifier_version": "0",
            "verified_at": "2026-08-13T00:10:00Z",
            "revocation_checked": True,
        },
    }


def validate_semantics(receipt: dict[str, Any]) -> None:
    validate_against_schema(receipt, SCHEMA)
    activity_type = receipt["activity_type"]
    if activity_type not in M02_TYPES:
        raise ValueError("M02 receipt validator received a non-M02 activity type")
    input_map = {item["artifact_id"]: item["sha256"] for item in receipt["input_hashes"]}
    output_map = {item["artifact_id"]: item["sha256"] for item in receipt["output_hashes"]}
    if list(input_map) != INPUT_IDS[activity_type] or len(input_map) != len(receipt["input_hashes"]):
        raise ValueError("M02 external receipt input exact-set or order mismatch")
    if list(output_map) != OUTPUT_IDS[activity_type] or len(output_map) != len(receipt["output_hashes"]):
        raise ValueError("M02 external receipt output exact-set or order mismatch")
    if (
        receipt["candidate_manifest_sha256"]
        != input_map["implementation-candidate-identity-manifest"]
    ):
        raise ValueError("M02 external receipt candidate input hash mismatch")
    payload = receipt["activity_payload"]
    if payload["payload_type"] != activity_type:
        raise ValueError("M02 external receipt payload type mismatch")
    if activity_type == "SCOPED_CANARY":
        pairs = {
            "rollout_body_sha256": input_map["rollout-body"],
            "deployed_digests_sha256": output_map["deployed-runtime-manifest"],
            "canary_result_sha256": output_map["canary-result"],
            "rollback_result_sha256": output_map["rollback-result"],
            "observation_result_sha256": output_map["observation-result"],
        }
        if payload["canary_started_at"] >= payload["canary_finished_at"]:
            raise ValueError("SCOPED_CANARY time window is not increasing")
    elif activity_type == "PROFILE_APPROVAL":
        pairs = {
            "profile_contract_sha256": input_map["capture-profile-contract"],
            "counter_contract_sha256": input_map["counter-contract"],
            "offline_source_manifest_sha256": input_map["offline-source-manifest"],
            "environment_manifest_sha256": input_map["runtime-environment-manifest"],
            "signed_profile_approval_sha256": output_map["signed-profile-approval"],
        }
        if payload["profile_id"] != receipt["profile_id"]:
            raise ValueError("PROFILE_APPROVAL profile mismatch")
        if payload["required_authority_roles"] != ROLES:
            raise ValueError("PROFILE_APPROVAL required role exact-set or order mismatch")
        signatures = payload["authority_signatures"]
        roles = [item["role"] for item in signatures]
        signers = [item["signer_identity"] for item in signatures]
        signature_artifacts = [item["signature_artifact"] for item in signatures]
        signature_hashes = [item["signature_sha256"] for item in signatures]
        if (
            roles != ROLES
            or any(item["trust_policy_role"] != item["role"] for item in signatures)
            or len(set(signers)) != 3
            or len(set(signature_artifacts)) != 3
            or len(set(signature_hashes)) != 3
            or payload["distinct_signer_count"] != len(set(signers))
            or payload["quorum"] != 3
        ):
            raise ValueError("PROFILE_APPROVAL 3-of-3 role and signer quorum mismatch")
        if payload["valid_from"] >= payload["valid_until"]:
            raise ValueError("PROFILE_APPROVAL validity window is not increasing")
    else:
        pairs = {
            "target_branch_policy_sha256": input_map["target-branch-policy"],
            "premerge_run_sha256": input_map["current-premerge-pass-run"],
            "promotion_intent_sha256": input_map["promotion-intent"],
            "protected_merge_receipt_sha256": output_map["protected-merge-receipt"],
        }
    if any(payload[field] != expected for field, expected in pairs.items()):
        raise ValueError("M02 external receipt payload/input/output hash mismatch")


def expect_failure(
    label: str,
    receipt: dict[str, Any],
    mutate: Callable[[dict[str, Any]], None],
    expected_error: str,
) -> None:
    candidate = copy.deepcopy(receipt)
    mutate(candidate)
    try:
        validate_semantics(candidate)
    except (ValueError, TypeError) as exc:
        if expected_error not in str(exc):
            raise ValueError(f"{label} hit wrong assertion: {exc}") from exc
        return
    raise ValueError(f"{label} did not fail")


def self_test() -> None:
    receipts = {activity_type: sample(activity_type) for activity_type in sorted(M02_TYPES)}
    for receipt in receipts.values():
        validate_semantics(receipt)
    expect_failure(
        "old schema version on M02 type", receipts["SCOPED_CANARY"],
        lambda item: item.update({"schema_version": "1.0.0"}),
        "schema const mismatch at $.schema_version",
    )
    expect_failure(
        "payload type mismatch", receipts["SCOPED_CANARY"],
        lambda item: item["activity_payload"].update({"payload_type": "PROTECTED_MERGE"}),
        "schema const mismatch at $.activity_payload.payload_type",
    )
    expect_failure(
        "input omission", receipts["PROTECTED_MERGE"],
        lambda item: item["input_hashes"].pop(),
        "M02 external receipt input exact-set or order mismatch",
    )
    expect_failure(
        "candidate input hash mismatch", receipts["PROTECTED_MERGE"],
        lambda item: item.update({"candidate_manifest_sha256": "f" * 64}),
        "M02 external receipt candidate input hash mismatch",
    )
    expect_failure(
        "output hash mismatch", receipts["SCOPED_CANARY"],
        lambda item: item["activity_payload"].update({"canary_result_sha256": "f" * 64}),
        "M02 external receipt payload/input/output hash mismatch",
    )
    expect_failure(
        "duplicate approval signer", receipts["PROFILE_APPROVAL"],
        lambda item: item["activity_payload"]["authority_signatures"][1].update({"signer_identity": "signer-1"}),
        "PROFILE_APPROVAL 3-of-3 role and signer quorum mismatch",
    )
    expect_failure(
        "wrong trust role", receipts["PROFILE_APPROVAL"],
        lambda item: item["activity_payload"]["authority_signatures"][1].update({"trust_policy_role": "PROJECT_OWNER"}),
        "PROFILE_APPROVAL 3-of-3 role and signer quorum mismatch",
    )
    expect_failure(
        "profile mismatch", receipts["PROFILE_APPROVAL"],
        lambda item: item["activity_payload"].update({"profile_id": "WRONG"}),
        "PROFILE_APPROVAL profile mismatch",
    )
    expect_failure(
        "premerge hash mismatch", receipts["PROTECTED_MERGE"],
        lambda item: item["activity_payload"].update({"premerge_run_sha256": "e" * 64}),
        "M02 external receipt payload/input/output hash mismatch",
    )


def main() -> int:
    parser = argparse.ArgumentParser()
    parser.add_argument("--self-test", action="store_true")
    parser.add_argument("--check", type=Path)
    args = parser.parse_args()
    if args.self_test == bool(args.check):
        parser.error("choose exactly one of --self-test or --check PATH")
    if args.self_test:
        self_test()
        print("PASS M02 external receipt contract: 3 positive types and 9 targeted negative cases")
    else:
        path = args.check if args.check.is_absolute() else REPO / args.check
        validate_semantics(json.loads(path.read_text(encoding="utf-8")))
        print(f"PASS M02 external receipt contract instance: {path}")
    print("PROOF_CEILING CONTRACT_SEMANTICS_ONLY_NOT_SIGNATURE_TRUST_EXTERNAL_EXECUTION_OR_ACCEPTANCE")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
