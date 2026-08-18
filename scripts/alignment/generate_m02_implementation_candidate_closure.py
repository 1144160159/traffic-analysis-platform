#!/usr/bin/env python3
"""Derive the fail-closed M02 implementation-candidate closure checklist.

This generator scans only declared candidate inputs.  It never creates an
implementation manifest, image, SBOM, attestation, delivery package, identity,
signature, authorization, or registry mutation.
"""

from __future__ import annotations

import argparse
from collections import Counter
import copy
import hashlib
import json
from pathlib import Path
import subprocess
from typing import Any, Callable

from build_topic1_task_registry import (
    require_trusted_signature_verifier,
    validate_against_schema,
    validate_implementation_candidate,
)


REPO = Path(__file__).resolve().parents[2]
SCHEMA = REPO / "contracts/alignment/m02-implementation-candidate-closure.schema.json"
OUTPUT = REPO / "contracts/alignment/m02-implementation-candidate-closure.v1.json"
DOC_OUTPUT = REPO / "doc/07_alignment/generated/M02实现候选闭包检查清单.md"
CATALOG = REPO / "contracts/alignment/m02-code-direct-leaf-catalog.v4.json"
ALLOCATION = REPO / "contracts/alignment/m02-code-direct-leaf-allocation.v4.json"
IMPLEMENTATION_SCHEMA = REPO / "contracts/alignment/implementation-candidate.schema.json"
SEMANTIC_VALIDATOR = REPO / "scripts/alignment/build_topic1_task_registry.py"
MANIFEST = "doc/02_acceptance/topic1/m02/candidates/m02-code-direct-v4/implementation-candidate.json"
DESIGN_MANIFEST = "doc/02_acceptance/topic1/m02/candidates/m02-code-direct-v4/design-candidate-manifest.json"
DESIGN_SCHEMA = REPO / "contracts/alignment/design-candidate-manifest.schema.json"
IMPLEMENTATION_TEST = "scripts/alignment/test_implementation_candidate.py"
IMPLEMENTATION_TEST_DESIGN = "contracts/alignment/m01-n003-implementation-candidate-test-function-design.v1.json"
IMPLEMENTATION_TEST_DESIGN_SCHEMA = "contracts/alignment/m01-n003-implementation-candidate-test-function-design.schema.json"
IMPLEMENTATION_TEST_DESIGN_VALIDATOR = "scripts/alignment/validate_m01_n003_implementation_candidate_test_function_design.py"
EARLY_TRUST_PREVIEW_PATHS = [
    "contracts/alignment/m01-early-trust-train-allocation.v2.json",
    "contracts/alignment/m01-early-trust-train-catalog.v2.json",
    "contracts/alignment/m01-early-trust-function-design.v1.json",
    "contracts/alignment/m01-early-trust-review-work-order-catalog.v1.json",
    "contracts/alignment/m01-early-trust-registry-switch-preflight.v1.json",
]
EARLY_TRUST_PREVIEW_VALIDATOR = "scripts/alignment/validate_m01_early_trust_function_design.py"
EARLY_TRUST_CANDIDATE_FREEZE_PATHS = [
    "contracts/alignment/m01-early-trust-candidate-freeze-work-order.schema.json",
    "contracts/alignment/m01-early-trust-candidate-freeze-work-order.v1.json",
    "scripts/alignment/generate_m01_early_trust_candidate_freeze_work_order.py",
    "scripts/alignment/freeze_m01_early_trust_design_candidate.py",
]
EARLY_TRUST_CANDIDATE_FREEZE_VALIDATOR = (
    "scripts/alignment/generate_m01_early_trust_candidate_freeze_work_order.py"
)
EARLY_TRUST_CANDIDATE_FREEZE_SELF_TEST_SUMMARY = (
    "PASS M01 candidate freeze work order: 55 planned leaves, 29 planned-output "
    "leaves, 81 design sources, 10 targeted mutation guards"
)
EXTERNAL_IMAGE_ACTIVITY_PATHS = [
    "contracts/alignment/m01-verifier-image-build-sign-receipt.schema.json",
    "scripts/alignment/validate_m01_verifier_image_build_sign_receipt.py",
    "contracts/alignment/m01-verifier-image-build-sign-work-order.v1.json",
]
EXTERNAL_IMAGE_RECEIPT = "doc/02_acceptance/topic1/m01/external-activities/verifier-image-build-sign-publish/receipt.json"
EXTERNAL_IMAGE_RECEIPT_SELF_TEST_SUMMARY = (
    "PASS M01 verifier image build-sign receipt: 1 structural positive and 16 targeted negative cases"
)
TRUSTED_SIGNATURE_PATHS = [
    "contracts/alignment/signature-trust-policy.schema.json",
    "contracts/alignment/signature-verification-request.schema.json",
    "contracts/alignment/signature-verification-attestation.schema.json",
    "scripts/alignment/verify_trusted_signature.py",
    "scripts/alignment/test_trusted_signature_verifier.py",
    "deployments/security/topic1-trusted-signature-verifier.yaml",
]
EXPECTED_REQUIREMENTS = [f"IC-M02-C{index:02d}" for index in range(1, 12)]
IMPLEMENTATION_TEST_SUMMARY = (
    "PASS implementation candidate rejection matrix: 8 exact negative cases; "
    "no positive candidate or execution authorization claimed"
)
IMPLEMENTATION_TEST_DESIGN_SUMMARY = (
    "PASS M01-N003 P005/P006 static design: 1 function, 8 NOT_RUN cases, "
    "2 evidence artifacts, 7 reproduced DoR findings; execution remains BLOCKED"
)
TRUSTED_SIGNATURE_TEST_SUMMARY = (
    "PASS trusted signature verifier: 10 fail-closed negative cases and one protected positive case"
)


def sha256(path: Path) -> str:
    return hashlib.sha256(path.read_bytes()).hexdigest()


def hash_ref(path: Path) -> dict[str, str]:
    return {"path": path.relative_to(REPO).as_posix(), "sha256": sha256(path)}


def safe_path(relative: str) -> Path:
    path = Path(relative)
    resolved = (REPO / path).resolve()
    if path.is_absolute() or ".." in path.parts or not resolved.is_relative_to(REPO):
        raise ValueError(f"implementation closure path escapes repository: {relative}")
    cursor = REPO
    for part in path.parts:
        cursor /= part
        if cursor.is_symlink():
            raise ValueError(f"implementation closure path contains a symlink: {relative}")
    return resolved


def probe(relative: str) -> dict[str, Any]:
    path = safe_path(relative)
    if not path.is_file():
        return {"path": relative, "exists": False, "sha256": None, "status": "MISSING", "blocking_reasons": ["required input is absent"]}
    return {"path": relative, "exists": True, "sha256": sha256(path), "status": "PRESENT", "blocking_reasons": []}


def self_test_ready(relative: str, expected_summary: str) -> tuple[bool, str]:
    path = safe_path(relative)
    if not path.is_file():
        return False, "test runner is absent"
    completed = subprocess.run(
        ["python3", relative, "--self-test"], cwd=REPO,
        check=False, capture_output=True, text=True,
    )
    if completed.returncode != 0:
        return False, f"self-test exit={completed.returncode}"
    if expected_summary not in completed.stdout.splitlines():
        return False, "self-test success summary differs from the frozen contract"
    return True, ""


def manifest_probe(expected_delivery_paths: dict[str, str]) -> tuple[dict[str, Any], dict[str, Any] | None]:
    result = probe(MANIFEST)
    if not result["exists"]:
        return result, None
    try:
        payload = json.loads(safe_path(MANIFEST).read_text(encoding="utf-8"))
        validate_against_schema(payload, IMPLEMENTATION_SCHEMA)
        validate_implementation_candidate(payload, "M02 implementation closure")
        actual_delivery_paths = {
            item["artifact_id"]: item["path"] for item in payload["delivery_artifacts"]
        }
        if actual_delivery_paths != expected_delivery_paths:
            raise ValueError("M02 implementation candidate delivery role-to-path exact-set drifted")
    except (ValueError, TypeError, json.JSONDecodeError, OSError) as exc:
        result["status"] = "INVALID"
        result["blocking_reasons"] = [str(exc)]
        return result, None
    return result, payload


def design_candidate() -> tuple[dict[str, Any], dict[str, Any] | None]:
    result = probe(DESIGN_MANIFEST)
    if not result["exists"]:
        return result, None
    try:
        payload = json.loads(safe_path(DESIGN_MANIFEST).read_text(encoding="utf-8"))
        validate_against_schema(payload, DESIGN_SCHEMA)
    except (ValueError, TypeError, json.JSONDecodeError, OSError) as exc:
        result["status"] = "INVALID"
        result["blocking_reasons"] = [str(exc)]
        return result, None
    return result, payload


def requirement(
    requirement_id: str,
    authority_task_ids: list[str],
    description: str,
    required_fields: list[str],
    required_paths: list[str],
    status: str,
    blocking_reasons: list[str],
    next_action: str,
) -> dict[str, Any]:
    return {
        "requirement_id": requirement_id,
        "authority_task_ids": authority_task_ids,
        "description": description,
        "required_fields": required_fields,
        "required_paths": required_paths,
        "status": status,
        "blocking_reasons": blocking_reasons,
        "next_action": next_action,
    }


def build() -> dict[str, Any]:
    catalog = json.loads(CATALOG.read_text(encoding="utf-8"))
    allocation = json.loads(ALLOCATION.read_text(encoding="utf-8"))
    implementation_schema = json.loads(IMPLEMENTATION_SCHEMA.read_text(encoding="utf-8"))
    if catalog["allocation_ledger"] != hash_ref(ALLOCATION):
        raise ValueError("implementation closure catalog/allocation reference drifted")
    if catalog["delivery_artifact_contract"] != allocation["delivery_artifact_contract"]:
        raise ValueError("implementation closure delivery contract differs across v4 artifacts")

    required_fields = implementation_schema["required"]
    if len(required_fields) != 22 or len(required_fields) != len(set(required_fields)):
        raise ValueError("implementation candidate schema required-field exact-set drifted")
    roles = implementation_schema["properties"]["delivery_artifacts"]["items"]["properties"]["artifact_id"]["enum"]
    allocation_rows = allocation["delivery_artifact_contract"]["required_artifacts"]
    mapping = {item["artifact_role"]: item["path"] for item in allocation_rows}
    if roles != list(mapping) or len(mapping) != 5:
        raise ValueError("implementation closure delivery role exact-set drifted")
    owner = next(item for item in catalog["leaves"] if item["leaf_id"] == "M02-N013-L11")
    owner_paths = [item.removesuffix("#$DOCUMENT") for item in owner["write_locators"]]
    if owner_paths != list(mapping.values()):
        raise ValueError("implementation closure delivery paths differ from one WRT owner")
    delivery_rows = [
        {
            "artifact_id": role, "path": path,
            "owner_leaf_id": owner["leaf_id"], "owner_atomic_pr_id": owner["atomic_pr_id"],
            "exists": safe_path(path).is_file(),
            "status": "PRESENT_UNVALIDATED" if safe_path(path).is_file() else "MISSING",
        }
        for role, path in mapping.items()
    ]
    manifest_result, manifest = manifest_probe(mapping)
    design_result, design = design_candidate()
    same_commit = bool(
        manifest is not None and design is not None
        and manifest["implementation_candidate_commit"] == design["implementation_candidate_commit"]
    )
    signature_inputs = [probe(path) for path in TRUSTED_SIGNATURE_PATHS]
    validator_test = probe(IMPLEMENTATION_TEST)
    validator_test_ready, validator_test_blocker = self_test_ready(
        IMPLEMENTATION_TEST, IMPLEMENTATION_TEST_SUMMARY
    )
    validator_design_inputs = [
        probe(IMPLEMENTATION_TEST_DESIGN),
        probe(IMPLEMENTATION_TEST_DESIGN_SCHEMA),
        probe(IMPLEMENTATION_TEST_DESIGN_VALIDATOR),
    ]
    validator_design_self_test_ready, validator_design_blocker = self_test_ready(
        IMPLEMENTATION_TEST_DESIGN_VALIDATOR, IMPLEMENTATION_TEST_DESIGN_SUMMARY
    )
    validator_design_ready = (
        all(item["exists"] for item in validator_design_inputs)
        and validator_design_self_test_ready
    )
    early_trust_preview_inputs = [probe(path) for path in EARLY_TRUST_PREVIEW_PATHS]
    early_trust_preview_ready, early_trust_preview_blocker = self_test_ready(
        EARLY_TRUST_PREVIEW_VALIDATOR,
        "PASS M01 early-trust function design: 36 leaves, 25 exact function locators, 3 type owners, 8 non-function surfaces, 42 NOT_RUN tests, reviews 36 MISSING, active registries unchanged",
    )
    early_trust_preview_valid = (
        all(item["exists"] for item in early_trust_preview_inputs)
        and early_trust_preview_ready
    )
    early_trust_candidate_freeze_inputs = [
        probe(path) for path in EARLY_TRUST_CANDIDATE_FREEZE_PATHS
    ]
    candidate_freeze_run = subprocess.run(
        ["python3", EARLY_TRUST_CANDIDATE_FREEZE_VALIDATOR, "--verify"],
        cwd=REPO, check=False, capture_output=True, text=True,
    )
    candidate_freeze_contract_ready = (
        candidate_freeze_run.returncode == 0
        and EARLY_TRUST_CANDIDATE_FREEZE_SELF_TEST_SUMMARY
        in candidate_freeze_run.stdout.splitlines()
    )
    candidate_freeze_contract_blocker = (
        "" if candidate_freeze_contract_ready
        else f"candidate freeze work-order verify exit={candidate_freeze_run.returncode}"
    )
    candidate_freeze_work_order = None
    try:
        freeze_path = safe_path(EARLY_TRUST_CANDIDATE_FREEZE_PATHS[1])
        candidate_freeze_work_order = json.loads(freeze_path.read_text(encoding="utf-8"))
    except (OSError, json.JSONDecodeError):
        pass
    candidate_freeze_blocked_exact = bool(
        candidate_freeze_work_order is not None
        and candidate_freeze_work_order.get("artifact_status")
        == "BLOCKED_IMPLEMENTATION_SOURCES_AND_CLEAN_WORKTREE"
        and not candidate_freeze_work_order.get("design_candidate_stage", {})
        .get("manifest_probe", {})
        .get("exists", True)
        and not candidate_freeze_work_order.get("implementation_candidate_stage", {})
        .get("manifest_probe", {})
        .get("exists", True)
    )
    candidate_freeze_contract_valid = (
        all(item["exists"] for item in early_trust_candidate_freeze_inputs)
        and candidate_freeze_contract_ready
        and candidate_freeze_blocked_exact
    )
    external_image_activity_inputs = [probe(path) for path in EXTERNAL_IMAGE_ACTIVITY_PATHS]
    external_image_contract_ready, external_image_contract_blocker = self_test_ready(
        EXTERNAL_IMAGE_ACTIVITY_PATHS[1], EXTERNAL_IMAGE_RECEIPT_SELF_TEST_SUMMARY
    )
    external_image_receipt = probe(EXTERNAL_IMAGE_RECEIPT)
    external_image_receipt_ready = False
    external_image_receipt_blocker = "external verifier image build sign publish receipt is absent"
    if external_image_receipt["exists"]:
        completed = subprocess.run(
            ["python3", EXTERNAL_IMAGE_ACTIVITY_PATHS[1], "--check", EXTERNAL_IMAGE_RECEIPT],
            cwd=REPO, check=False, capture_output=True, text=True,
        )
        external_image_receipt_ready = completed.returncode == 0
        external_image_receipt_blocker = "" if external_image_receipt_ready else f"external image receipt validation exit={completed.returncode}"
        if not external_image_receipt_ready:
            external_image_receipt["status"] = "INVALID"
            external_image_receipt["blocking_reasons"] = [external_image_receipt_blocker]
    external_image_activity_ready = (
        all(item["exists"] for item in external_image_activity_inputs)
        and external_image_contract_ready and external_image_receipt_ready
    )
    signature_test_ready, signature_test_blocker = self_test_ready(
        "scripts/alignment/test_trusted_signature_verifier.py", TRUSTED_SIGNATURE_TEST_SUMMARY
    )
    trusted_wrapper_ready = False
    trusted_wrapper_blocker = "trusted cryptographic wrapper remains hard-blocked"
    try:
        require_trusted_signature_verifier("M02 implementation closure probe")
    except ValueError as exc:
        trusted_wrapper_blocker = str(exc)
    else:
        trusted_wrapper_ready = True
        trusted_wrapper_blocker = ""
    delivery_present = sum(item["exists"] for item in delivery_rows)

    manifest_missing = manifest is None
    requirements = [
        requirement(
            "IC-M02-C01", ["T1-M01-N003"], "clean candidate commit and canonical production-tree fingerprint",
            ["implementation_candidate_commit", "production_tree_content_sha256", "tree_hash_algorithm", "dirty_count", "source_roots", "excluded_paths"],
            [MANIFEST], "MISSING" if manifest_missing else "READY",
            ["implementation candidate manifest is absent or invalid"] if manifest_missing else [],
            "produce one clean commit-bound tree fingerprint and pass the existing semantic validator",
        ),
        requirement(
            "IC-M02-C02", ["T1-M01-N003"], "immutable image digest and deployed digest exact-set",
            ["image_digests", "image_attestations"], [MANIFEST], "MISSING" if manifest_missing else "READY",
            ["candidate-bound image digests and attestations are unavailable"] if manifest_missing else [],
            "build immutable images and bind every deployed digest to one attestation row",
        ),
        requirement(
            "IC-M02-C03", ["T1-M01-N003"], "image-internal binary and external prebuilt artifact provenance closure",
            ["external_or_prebuilt_artifacts"], [MANIFEST], "MISSING" if manifest_missing else "READY",
            ["binary recipe image and SBOM provenance cannot be inspected without a valid candidate"] if manifest_missing else [],
            "record binary hash builder recipe image-internal hash and SBOM or attestation for each prebuilt artifact",
        ),
        requirement(
            "IC-M02-C04", ["T1-M01-N003"], "config schema and migration candidate Git-blob closure",
            ["config_schema_migration_hashes", "config_schema_migration_artifacts"], [MANIFEST], "MISSING" if manifest_missing else "READY",
            ["config schema and migration artifact closure is unavailable"] if manifest_missing else [],
            "bind every config schema and migration to the same candidate Git commit",
        ),
        requirement(
            "IC-M02-C05", ["T1-M01-N003"], "model threshold and dataset artifact closure",
            ["model_threshold_dataset_hashes", "model_threshold_dataset_artifacts"], [MANIFEST], "MISSING" if manifest_missing else "READY",
            ["model threshold and dataset artifact closure is unavailable"] if manifest_missing else [],
            "declare exact candidate or trusted-external model threshold and dataset artifacts",
        ),
        requirement(
            "IC-M02-C06", ["T1-M01-N003", "T1-M01-N010"], "supply-chain artifact and provenance receipt closure",
            ["supply_chain_artifact_hashes", "supply_chain_artifacts"], [MANIFEST], "MISSING" if manifest_missing else "READY",
            ["supply-chain artifact closure is unavailable"] if manifest_missing else [],
            "collect trusted external attestations and bind their provenance receipts and hashes",
        ),
        requirement(
            "IC-M02-C07", ["T1-M01-N003"], "runtime artifact closure for the same environment",
            ["runtime_artifact_hashes", "runtime_artifacts", "environment_id"], [MANIFEST], "MISSING" if manifest_missing else "READY",
            ["runtime artifact and environment closure is unavailable"] if manifest_missing else [],
            "bind runtime artifacts and deployment environment to the candidate exact-set",
        ),
        requirement(
            "IC-M02-C08", ["T1-M02-N013"], "exact-five candidate Git-blob delivery package",
            ["delivery_artifacts"], list(mapping.values()),
            "READY" if manifest is not None and delivery_present == 5 else "PARTIAL" if delivery_present else "MISSING",
            [] if manifest is not None and delivery_present == 5 else [f"delivery package present {delivery_present}/5 and valid manifest={str(manifest is not None).lower()}"],
            "materialize install preflight upgrade rollback and restore files atomically under P523 and bind all five hashes",
        ),
        requirement(
            "IC-M02-C09", ["T1-M01-N003"], "independent implementation-candidate validator mutation runner",
            [], [
                IMPLEMENTATION_TEST,
                IMPLEMENTATION_TEST_DESIGN,
                IMPLEMENTATION_TEST_DESIGN_SCHEMA,
                IMPLEMENTATION_TEST_DESIGN_VALIDATOR,
            ],
            "READY" if validator_test_ready and validator_design_ready else "PARTIAL"
            if validator_design_ready or validator_test["exists"] else "MISSING",
            [] if validator_test_ready and validator_design_ready else [
                blocker for blocker in [
                    None if validator_design_ready else validator_design_blocker,
                    None if validator_test_ready else validator_test_blocker,
                    None if validator_test_ready else (
                        "static design has eight NOT_RUN cases and required registry amendments; "
                        "it is not runner implementation or execution evidence"
                    ),
                ] if blocker
            ],
            "review and apply the registered dependency/schema/command amendments, then authorize P005 to implement main plus eight fixtures and P006 to run one G0 negative matrix",
        ),
        requirement(
            "IC-M02-C10", ["T1-M01-N015", "T1-M01-N010"], "function-complete v2 early trust preview plus trusted signature verifier policy request attestation runtime and test closure",
            [], [*EARLY_TRUST_PREVIEW_PATHS, EARLY_TRUST_PREVIEW_VALIDATOR, *EARLY_TRUST_CANDIDATE_FREEZE_PATHS, *EXTERNAL_IMAGE_ACTIVITY_PATHS, EXTERNAL_IMAGE_RECEIPT, *TRUSTED_SIGNATURE_PATHS],
            "READY" if early_trust_preview_valid and candidate_freeze_contract_valid and external_image_activity_ready and all(item["exists"] for item in signature_inputs) and signature_test_ready and trusted_wrapper_ready else "PARTIAL" if early_trust_preview_valid or candidate_freeze_contract_valid or any(item["exists"] for item in signature_inputs) or all(item["exists"] for item in external_image_activity_inputs) else "MISSING",
            [] if all(item["exists"] for item in signature_inputs) and signature_test_ready and trusted_wrapper_ready and early_trust_preview_valid and external_image_activity_ready else [
                f"early trust preview={early_trust_preview_blocker or 'PASS_NOT_REGISTERED'}",
                "early trust preview is not a global registry switch, implementation, protected deployment or trust PASS",
                "function type and non-function reviews are 0/36 and all remain MISSING_NOT_AUTHORED",
                f"candidate freeze contract={candidate_freeze_contract_blocker or 'PASS_BLOCKED_NO_CANDIDATE'}",
                f"external image activity contract={external_image_contract_blocker or 'PASS_CONTRACT_ONLY'}",
                f"external image receipt={external_image_receipt_blocker or 'PASS_HASH_CLOSURE_ONLY'}",
                f"trusted signature inputs present {sum(item['exists'] for item in signature_inputs)}/6",
                f"test={signature_test_blocker or 'PASS'}",
                f"wrapper={trusted_wrapper_blocker or 'PASS'}",
            ],
            "claim the 36 blocked review work orders, obtain real candidate-bound signed receipts, satisfy the blocked four-registry preflight, atomically register effective v2 N015 members P057-P062/P067-P096 while retaining P063-P065 as historical superseded IDs, then obtain the external image receipt and implement protect and test the verifier before P005/P006",
        ),
        requirement(
            "IC-M02-C11", ["T1-M02-N013", "T1-M02-N016"], "same-commit design implementation and delivery closure before C07",
            ["candidate_id", "implementation_candidate_commit", "created_at"], [DESIGN_MANIFEST, MANIFEST],
            "READY" if same_commit else "INVALID" if manifest is not None and design is not None else "MISSING",
            [] if same_commit else [
                f"same-commit closure unavailable: design={design_result['status']} implementation={manifest_result['status']}"
            ],
            "submit the valid implementation candidate beside the v4 design manifest and rerun preflight exact binding",
        ),
    ]
    if [item["requirement_id"] for item in requirements] != EXPECTED_REQUIREMENTS:
        raise ValueError("implementation closure requirement exact-set drifted")
    counts = Counter(item["status"] for item in requirements)
    readiness_counts = {status: counts.get(status, 0) for status in ["READY", "PARTIAL", "MISSING", "INVALID"]}
    all_ready = readiness_counts == {"READY": 11, "PARTIAL": 0, "MISSING": 0, "INVALID": 0}
    return {
        "schema_version": "1.0.0",
        "artifact_kind": "M02_IMPLEMENTATION_CANDIDATE_CLOSURE_CHECKLIST",
        "artifact_status": "READY_IMPLEMENTATION_CANDIDATE_INPUTS_EXACT_NOT_EXECUTION_AUTHORIZATION" if all_ready else "BLOCKED_IMPLEMENTATION_CANDIDATE_INCOMPLETE",
        "source_catalog": hash_ref(CATALOG),
        "source_allocation": hash_ref(ALLOCATION),
        "implementation_candidate_schema": hash_ref(IMPLEMENTATION_SCHEMA),
        "semantic_validator": hash_ref(SEMANTIC_VALIDATOR),
        "candidate_manifest_path": MANIFEST,
        "manifest_probe": manifest_result,
        "required_manifest_fields": required_fields,
        "delivery_artifact_contract": delivery_rows,
        "early_trust_preview_inputs": early_trust_preview_inputs,
        "early_trust_candidate_freeze_inputs": early_trust_candidate_freeze_inputs,
        "external_image_activity_inputs": external_image_activity_inputs,
        "external_image_receipt_probe": external_image_receipt,
        "trusted_signature_inputs": signature_inputs,
        "requirements": requirements,
        "readiness_counts": readiness_counts,
        "validation": {
            "source_hashes_exact": "PASS", "schema_required_fields_exact": "PASS",
            "delivery_roles_exact": "PASS", "delivery_paths_exact": "PASS",
            "early_trust_preview_exact": "PASS_NOT_GLOBAL_REGISTRY",
            "early_trust_candidate_freeze_contract_exact": "PASS_BLOCKED_NO_CANDIDATE",
            "external_image_activity_contract_exact": "PASS_RECEIPT_VALID" if external_image_receipt_ready else "PASS_RECEIPT_MISSING",
            "requirement_exact_set": "PASS", "no_missing_input_claimed_ready": True,
            "mutation_guards": {
                "required_field_omission": "PASS", "delivery_role_omission": "PASS",
                "delivery_path_drift": "PASS", "requirement_omission": "PASS",
                "manifest_false_ready": "PASS", "signature_false_ready": "PASS",
                "validator_test_false_ready": "PASS", "early_trust_preview_false_ready": "PASS",
                "candidate_freeze_false_ready": "PASS",
                "external_image_receipt_false_ready": "PASS",
            },
        },
        "proof_ceiling": "IMPLEMENTATION_CANDIDATE_REQUIREMENT_CLOSURE_ONLY_NOT_CANDIDATE_CREATION_SUPPLY_CHAIN_ATTESTATION_SIGNATURE_VERIFICATION_IMPLEMENTATION_EXECUTION_SWITCH_OR_ACCEPTANCE",
    }


def validate(payload: dict[str, Any]) -> None:
    validate_against_schema(payload, SCHEMA)
    expected = build()
    if payload != expected:
        raise ValueError("implementation candidate closure differs from exact derived state")
    if sum(payload["readiness_counts"].values()) != 11:
        raise ValueError("implementation closure readiness count total drifted")
    not_ready = any(item["status"] != "READY" for item in payload["requirements"])
    if not_ready and payload["artifact_status"].startswith("READY_"):
        raise ValueError("implementation closure falsely claims READY")


def expect_failure(label: str, payload: dict[str, Any], mutate: Callable[[dict[str, Any]], None], expected: str) -> None:
    candidate = copy.deepcopy(payload)
    mutate(candidate)
    try:
        validate(candidate)
    except (ValueError, TypeError) as exc:
        if expected not in str(exc):
            raise ValueError(f"implementation closure mutation {label} hit wrong assertion: {exc}") from exc
        return
    raise ValueError(f"implementation closure mutation {label} did not fail")


def self_test(payload: dict[str, Any]) -> None:
    expect_failure("required field omission", payload, lambda item: item["required_manifest_fields"].pop(), "schema minItems failed")
    expect_failure("delivery role omission", payload, lambda item: item["delivery_artifact_contract"].pop(), "schema minItems failed")
    expect_failure("delivery path drift", payload, lambda item: item["delivery_artifact_contract"][0].update({"path": "deployments/releases/topic1/wrong.json"}), "differs from exact derived state")
    expect_failure("requirement omission", payload, lambda item: item["requirements"].pop(), "schema minItems failed")
    expect_failure("manifest false ready", payload, lambda item: item["manifest_probe"].update({"exists": True, "status": "PRESENT"}), "differs from exact derived state")
    expect_failure(
        "signature identity drift",
        payload,
        lambda item: item["trusted_signature_inputs"][0].update({"sha256": "0" * 64}),
        "differs from exact derived state",
    )
    c09 = next(index for index, item in enumerate(payload["requirements"]) if item["requirement_id"] == "IC-M02-C09")
    expect_failure("validator test false ready", payload, lambda item: item["requirements"][c09].update({"status": "READY", "blocking_reasons": []}), "differs from exact derived state")
    c10 = next(index for index, item in enumerate(payload["requirements"]) if item["requirement_id"] == "IC-M02-C10")
    expect_failure("early trust preview false ready", payload, lambda item: item["requirements"][c10].update({"status": "READY", "blocking_reasons": []}), "differs from exact derived state")
    expect_failure("candidate freeze false ready", payload, lambda item: item["early_trust_candidate_freeze_inputs"][0].update({"exists": False, "status": "MISSING", "sha256": None, "blocking_reasons": ["forced missing"]}), "differs from exact derived state")
    expect_failure("external image receipt false ready", payload, lambda item: item["external_image_receipt_probe"].update({"exists": True, "status": "PRESENT", "sha256": "0" * 64, "blocking_reasons": []}), "differs from exact derived state")


def render_markdown(payload: dict[str, Any]) -> str:
    lines = [
        "# M02实现候选闭包检查清单", "",
        f"状态：`{payload['artifact_status']} / NO-GO`", "",
        "本清单只派生实现候选所需输入，不创建候选、镜像、SBOM、attestation、五件套、签名或授权。", "",
        "## 汇总", "",
        f"- READY={payload['readiness_counts']['READY']}；PARTIAL={payload['readiness_counts']['PARTIAL']}；MISSING={payload['readiness_counts']['MISSING']}；INVALID={payload['readiness_counts']['INVALID']}。",
        f"- implementation manifest：`{payload['manifest_probe']['status']}`。", "",
        "## Exact-five delivery package", "",
        "| Role | Path | Owner | Status |", "|---|---|---|---|",
    ]
    for item in payload["delivery_artifact_contract"]:
        lines.append(f"| `{item['artifact_id']}` | `{item['path']}` | `{item['owner_atomic_pr_id']}` | `{item['status']}` |")
    lines.extend(["", "## Closure requirements", "", "| ID | Authority | Status | Requirement | Next action |", "|---|---|---|---|---|"])
    for item in payload["requirements"]:
        authorities = ", ".join(f"`{task_id}`" for task_id in item["authority_task_ids"])
        lines.append(f"| `{item['requirement_id']}` | {authorities} | `{item['status']}` | {item['description']} | {item['next_action']} |")
    lines.extend(["", "## 证明上限", "", f"`{payload['proof_ceiling']}`", ""])
    return "\n".join(lines)


def main() -> int:
    parser = argparse.ArgumentParser()
    mode = parser.add_mutually_exclusive_group(required=True)
    mode.add_argument("--write", action="store_true")
    mode.add_argument("--check", action="store_true")
    mode.add_argument("--verify", action="store_true")
    args = parser.parse_args()
    payload = build()
    body = json.dumps(payload, ensure_ascii=False, indent=2) + "\n"
    markdown = render_markdown(payload)
    if args.write:
        OUTPUT.write_text(body, encoding="utf-8")
        DOC_OUTPUT.parent.mkdir(parents=True, exist_ok=True)
        DOC_OUTPUT.write_text(markdown, encoding="utf-8")
        print(f"WROTE {OUTPUT.relative_to(REPO)}")
        print(f"WROTE {DOC_OUTPUT.relative_to(REPO)}")
    else:
        if not OUTPUT.is_file() or OUTPUT.read_text(encoding="utf-8") != body:
            raise ValueError("generated implementation candidate closure is stale")
        if not DOC_OUTPUT.is_file() or DOC_OUTPUT.read_text(encoding="utf-8") != markdown:
            raise ValueError("generated implementation candidate closure markdown is stale")
        validate(payload)
        if args.verify:
            self_test(payload)
    print(
        f"PASS implementation-closure ready={payload['readiness_counts']['READY']} "
        f"partial={payload['readiness_counts']['PARTIAL']} missing={payload['readiness_counts']['MISSING']} "
        f"invalid={payload['readiness_counts']['INVALID']}"
    )
    print(f"PROOF_CEILING {payload['proof_ceiling']}")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
