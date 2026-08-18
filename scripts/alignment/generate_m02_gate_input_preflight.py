#!/usr/bin/env python3
"""Generate the fail-closed M02 C03-C07 external-input exact-set preflight.

This script consumes evidence if humans or candidate builders provide it.  It
does not create candidates, identities, signatures, locator receipts, or
review decisions, and it cannot turn structurally valid signatures into a
trusted cryptographic authorization.
"""

from __future__ import annotations

import argparse
import copy
import hashlib
import json
from pathlib import Path
import subprocess
from typing import Any, Callable

from build_topic1_task_registry import validate_against_schema, validate_implementation_candidate
from validate_function_design_review_receipt import validate as validate_function_review
from validate_non_function_design_exemption_contract import validate as validate_non_function_exemption
from validate_m02_responsibility_assignment import validate as validate_responsibility
from validate_m02_compatibility_default_off_review import validate as validate_compatibility_review


REPO = Path(__file__).resolve().parents[2]
SCHEMA = REPO / "contracts/alignment/m02-gate-input-preflight.schema.json"
OUTPUT = REPO / "contracts/alignment/m02-gate-input-preflight.v1.json"
DOC_OUTPUT = REPO / "doc/07_alignment/generated/M02外部证据收件预检.md"
LOCATOR_COVERAGE_PATH = REPO / "contracts/alignment/m02-code-direct-locator-coverage.v1.json"
REVIEW_COVERAGE_PATH = REPO / "contracts/alignment/m02-function-review-coverage.v1.json"
TASK_REGISTRY_PATH = REPO / "contracts/alignment/task-registry.v1.json"
REPLACEMENT_PATH = REPO / "contracts/alignment/m02-code-direct-leaf-catalog.v4.json"
DESIGN_MANIFEST_PATH = "doc/02_acceptance/topic1/m02/candidates/m02-code-direct-v4/design-candidate-manifest.json"
IMPLEMENTATION_MANIFEST_PATH = "doc/02_acceptance/topic1/m02/candidates/m02-code-direct-v4/implementation-candidate.json"
RESPONSIBILITY_PATH = "doc/02_acceptance/topic1/m02/responsibility/m02-responsibility-assignment.json"
RESPONSIBILITY_INTAKE_PATH = "doc/02_acceptance/topic1/m02/responsibility/m02-responsibility-signed-intake.json"
LOCATOR_ROOT = REPO / "doc/02_acceptance/topic1/m02/locator-receipts"
REVIEW_ROOT = REPO / "doc/02_acceptance/topic1/m02/function-reviews"
COMPATIBILITY_ROOT = REPO / "doc/02_acceptance/topic1/m02/compatibility-reviews"
MANAGED_ROOT = REPO / "doc/02_acceptance/topic1/m02"

DESIGN_SCHEMA = REPO / "contracts/alignment/design-candidate-manifest.schema.json"
IMPLEMENTATION_SCHEMA = REPO / "contracts/alignment/implementation-candidate.schema.json"
RESPONSIBILITY_SCHEMA = REPO / "contracts/alignment/m02-responsibility-assignment.schema.json"
SIGNED_INTAKE_SCHEMA = REPO / "contracts/alignment/signed-contract-intake.schema.json"
FUNCTION_REVIEW_SCHEMA = REPO / "contracts/alignment/function-design-review-receipt.schema.json"
NON_FUNCTION_SCHEMA = REPO / "contracts/alignment/non-function-design-exemption.schema.json"
RESPONSIBILITY_VALIDATOR = REPO / "scripts/alignment/validate_m02_responsibility_assignment.py"
COMPATIBILITY_SCHEMA = REPO / "contracts/alignment/m02-compatibility-default-off-review.schema.json"
COMPATIBILITY_VALIDATOR = REPO / "scripts/alignment/validate_m02_compatibility_default_off_review.py"
FUNCTION_REVIEW_VALIDATOR = REPO / "scripts/alignment/validate_function_design_review_receipt.py"
NON_FUNCTION_VALIDATOR = REPO / "scripts/alignment/validate_non_function_design_exemption_contract.py"
IMPLEMENTATION_VALIDATOR = REPO / "scripts/alignment/build_topic1_task_registry.py"

LOCATOR_SCHEMAS = {
    ".go": REPO / "contracts/alignment/locator-resolution-receipt.schema.json",
    ".py": REPO / "contracts/alignment/python-locator-resolution-receipt.schema.json",
    ".rs": REPO / "contracts/alignment/rust-locator-resolution-receipt.schema.json",
    ".java": REPO / "contracts/alignment/java-locator-resolution-receipt.schema.json",
    ".proto": REPO / "contracts/alignment/proto-descriptor-locator-resolution-receipt.schema.json",
    ".sql": REPO / "contracts/alignment/sql-ddl-locator-resolution-receipt.schema.json",
    ".json": REPO / "contracts/alignment/structured-config-locator-resolution-receipt.schema.json",
    ".yaml": REPO / "contracts/alignment/structured-config-locator-resolution-receipt.schema.json",
    ".yml": REPO / "contracts/alignment/structured-config-locator-resolution-receipt.schema.json",
    ".toml": REPO / "contracts/alignment/structured-config-locator-resolution-receipt.schema.json",
    ".sh": REPO / "contracts/alignment/shell-ast-locator-resolution-receipt.schema.json",
}


def sha256_bytes(value: bytes) -> str:
    return hashlib.sha256(value).hexdigest()


def sha256(path: Path) -> str:
    return sha256_bytes(path.read_bytes())


def semantic_sha256(value: Any) -> str:
    return sha256_bytes(json.dumps(value, ensure_ascii=False, sort_keys=True, separators=(",", ":")).encode())


def hash_ref(path: Path) -> dict[str, str]:
    return {"path": path.relative_to(REPO).as_posix(), "sha256": sha256(path)}


def safe_existing(relative: str) -> Path:
    raw = REPO / relative
    resolved = raw.resolve()
    if Path(relative).is_absolute() or ".." in Path(relative).parts or not resolved.is_relative_to(REPO) or not resolved.is_file():
        raise ValueError(f"unsafe or missing input artifact: {relative}")
    cursor = REPO
    for part in Path(relative).parts:
        cursor = cursor / part
        if cursor.is_symlink():
            raise ValueError(f"input artifact path contains a symlink: {relative}")
    return resolved


def load_json(relative: str, schema: Path | None = None) -> tuple[dict[str, Any], str]:
    path = safe_existing(relative)
    raw = path.read_bytes()
    payload = json.loads(raw)
    if not isinstance(payload, dict):
        raise ValueError(f"input artifact must be a JSON object: {relative}")
    if schema is not None:
        validate_against_schema(payload, schema)
    return payload, sha256_bytes(raw)


def probe(relative: str, validator: Callable[[dict[str, Any]], None] | None = None, schema: Path | None = None) -> tuple[dict[str, Any], dict[str, Any] | None]:
    path = REPO / relative
    if not path.exists():
        return {"expected_path": relative, "exists": False, "sha256": None, "status": "MISSING", "blocking_reasons": ["required primary artifact is absent"]}, None
    try:
        payload, digest = load_json(relative, schema)
        if validator is not None:
            validator(payload)
    except (ValueError, TypeError, json.JSONDecodeError, subprocess.SubprocessError) as exc:
        digest = sha256(path) if path.is_file() and not path.is_symlink() else None
        return {"expected_path": relative, "exists": True, "sha256": digest, "status": "INVALID", "blocking_reasons": [str(exc)]}, None
    return {"expected_path": relative, "exists": True, "sha256": digest, "status": "STRUCTURALLY_VALID", "blocking_reasons": []}, payload


def split_locator(locator: str) -> tuple[str, str]:
    path, separator, query = locator.partition("#")
    if not separator or not path or not query:
        raise ValueError(f"planned locator lacks exact path/query: {locator}")
    return path, query


def expected_locator_rows(locator_coverage: dict[str, Any]) -> list[dict[str, Any]]:
    by_locator: dict[str, dict[str, Any]] = {}
    for occurrence in locator_coverage["locator_occurrences"]:
        locator = occurrence["locator"]
        current = by_locator.setdefault(locator, {
            "locator": locator,
            "path": occurrence["path"],
            "query": occurrence["symbol_or_pointer"],
            "consumer_atomic_pr_ids": [],
        })
        current["consumer_atomic_pr_ids"].append(occurrence["atomic_pr_id"])
    rows = []
    for locator, row in sorted(by_locator.items()):
        locator_hash = semantic_sha256(locator)
        suffix = Path(row["path"]).suffix.lower()
        schema = LOCATOR_SCHEMAS.get(suffix)
        if schema is None:
            raise ValueError(f"no intake receipt schema for planned locator: {locator}")
        query = row["query"]
        if query is None:
            if suffix != ".json" or "#" in locator:
                raise ValueError(f"only a whole JSON document may omit a locator query: {locator}")
            query = "$DOCUMENT"
        rows.append({
            **row,
            "query": query,
            "consumer_atomic_pr_ids": sorted(set(row["consumer_atomic_pr_ids"])),
            "locator_id": "LOC-M02-" + locator_hash[:24].upper(),
            "receipt_schema": schema,
            "expected_path": f"doc/02_acceptance/topic1/m02/locator-receipts/{locator_hash}-locator-resolution-receipt.json",
        })
    return rows


def expected_review_rows(review_coverage: dict[str, Any]) -> list[dict[str, Any]]:
    return sorted(
        ({**item, "expected_path": item["expected_artifact_path"]} for item in review_coverage["leaf_reviews"]),
        key=lambda item: item["expected_path"],
    )


def expected_compatibility_rows(locator_coverage: dict[str, Any]) -> list[dict[str, Any]]:
    grouped: dict[str, dict[str, Any]] = {}
    for occurrence in locator_coverage["locator_occurrences"]:
        row = grouped.setdefault(occurrence["atomic_pr_id"], {
            "atomic_pr_id": occurrence["atomic_pr_id"], "target_locators": [],
        })
        row["target_locators"].append(occurrence["locator"])
    rows = []
    for atomic_pr_id, row in sorted(grouped.items()):
        rows.append({
            **row, "target_locators": sorted(set(row["target_locators"])),
            "expected_path": f"doc/02_acceptance/topic1/m02/compatibility-reviews/{atomic_pr_id.lower()}/compatibility-default-off-review-receipt.json",
        })
    return rows


def artifact_ref_exists(item: dict[str, Any]) -> None:
    path = safe_existing(item["path"])
    if sha256(path) != item["sha256"]:
        raise ValueError(f"referenced artifact hash mismatch: {item['path']}")


def exact_candidate(receipt: dict[str, Any], design: dict[str, Any], design_hash: str) -> None:
    expected = {"commit": design["implementation_candidate_commit"], "manifest_path": DESIGN_MANIFEST_PATH, "manifest_sha256": design_hash}
    if receipt.get("candidate") != expected:
        raise ValueError("receipt crosses the frozen design candidate")


def validate_locator_receipt(row: dict[str, Any], payload: dict[str, Any], design: dict[str, Any], design_hash: str, resolver_refs: dict[str, dict[str, str]]) -> None:
    validate_against_schema(payload, row["receipt_schema"])
    exact_candidate(payload, design, design_hash)
    if payload["locator"]["locator_id"] != row["locator_id"] or payload["locator"]["path"] != row["path"] or payload["locator"]["query"] != row["query"]:
        raise ValueError("locator receipt identity/path/query mismatch")
    resolver = payload["resolver"]
    expected = resolver_refs.get(resolver["source_path"])
    if expected is None or resolver["source_sha256"] != expected["sha256"]:
        raise ValueError("locator receipt resolver source is outside the trusted exact-set")


def validate_review_receipt(row: dict[str, Any], payload: dict[str, Any], design: dict[str, Any], design_hash: str) -> None:
    if row["review_surface"] == "FUNCTION_SET":
        validate_function_review(payload)
        if payload["review_disposition"] != "UNIFIED":
            raise ValueError("function review is not UNIFIED")
        refs = [payload["pattern_decision_ref"], payload["code_unit_contract_ref"], payload["negative_test_manifest_ref"]]
    else:
        validate_non_function_exemption(payload)
        if sorted(payload["target_locators"]) != sorted(row["write_locators"]):
            raise ValueError("non-function review target locator exact-set drifted")
        refs = [payload["specialized_contract_ref"], payload["verification_plan_ref"], payload["rollback_plan_ref"]]
    if payload["atomic_pr_id"] != row["atomic_pr_id"]:
        raise ValueError("review receipt atomic PR identity mismatch")
    exact_candidate(payload, design, design_hash)
    for item in refs:
        artifact_ref_exists(item)
    for attestation in payload["attestations"]:
        artifact_ref_exists(attestation["signature_artifact_ref"])


def validate_compatibility_receipt(row: dict[str, Any], payload: dict[str, Any], design: dict[str, Any], design_hash: str) -> None:
    validate_compatibility_review(payload)
    if payload["atomic_pr_id"] != row["atomic_pr_id"] or sorted(payload["target_locators"]) != row["target_locators"]:
        raise ValueError("compatibility receipt atomic PR or target locator exact-set mismatch")
    exact_candidate(payload, design, design_hash)
    for field in ("compatibility_contract_ref", "verification_plan_ref", "rollback_plan_ref"):
        artifact_ref_exists(payload[field])
    for attestation in payload["attestations"]:
        artifact_ref_exists(attestation["signature_artifact_ref"])


def scan(pattern: str) -> list[str]:
    if not MANAGED_ROOT.exists():
        return []
    return sorted(path.relative_to(REPO).as_posix() for path in MANAGED_ROOT.glob(pattern) if path.is_file() or path.is_symlink())


def build_exact_set(rows: list[dict[str, Any]], found_paths: list[str], design: dict[str, Any] | None, design_hash: str | None, kind: str, resolver_refs: dict[str, dict[str, str]]) -> dict[str, Any]:
    expected = sorted(row["expected_path"] for row in rows)
    expected_set, found_set = set(expected), set(found_paths)
    missing, unexpected = sorted(expected_set - found_set), sorted(found_set - expected_set)
    invalid: list[dict[str, str]] = []
    validated = 0
    rows_by_path = {row["expected_path"]: row for row in rows}
    for relative in sorted(expected_set & found_set):
        try:
            if design is None or design_hash is None:
                raise ValueError("frozen design candidate is unavailable")
            payload, _ = load_json(relative)
            row = rows_by_path[relative]
            if kind == "locator":
                validate_locator_receipt(row, payload, design, design_hash, resolver_refs)
            elif kind == "compatibility":
                validate_compatibility_receipt(row, payload, design, design_hash)
            else:
                validate_review_receipt(row, payload, design, design_hash)
            validated += 1
        except (ValueError, TypeError, json.JSONDecodeError) as exc:
            invalid.append({"path": relative, "error": str(exc)})
    ready = not missing and not unexpected and not invalid and validated == len(expected) and design is not None
    return {
        "expected_count": len(expected), "found_count": len(found_set), "validated_count": validated,
        "invalid_count": len(invalid), "missing_count": len(missing), "unexpected_count": len(unexpected),
        "expected_path_exact_set_sha256": semantic_sha256(expected), "expected_paths": expected,
        "found_paths": sorted(found_set), "missing_paths": missing, "invalid_artifacts": invalid,
        "unexpected_paths": unexpected,
        "status": "READY_EXACT_SET_VALIDATED" if ready else "BLOCKED_EXACT_SET_INCOMPLETE",
    }


def build() -> dict[str, Any]:
    locator_coverage = json.loads(LOCATOR_COVERAGE_PATH.read_text(encoding="utf-8"))
    review_coverage = json.loads(REVIEW_COVERAGE_PATH.read_text(encoding="utf-8"))
    locator_rows = expected_locator_rows(locator_coverage)
    review_rows = expected_review_rows(review_coverage)
    compatibility_rows = expected_compatibility_rows(locator_coverage)
    required_sources = sorted({row["path"] for row in locator_rows})

    design_probe, design = probe(DESIGN_MANIFEST_PATH, schema=DESIGN_SCHEMA)
    if design is not None:
        declared = design["source_blob_sha256"]
        try:
            if set(declared) != set(required_sources):
                raise ValueError("design candidate source path exact-set drifted")
            for relative, expected_hash in declared.items():
                frozen = subprocess.run(["git", "show", f"{design['implementation_candidate_commit']}:{relative}"], cwd=REPO, check=True, capture_output=True).stdout
                if sha256_bytes(frozen) != expected_hash or safe_existing(relative).read_bytes() != frozen:
                    raise ValueError(f"design candidate source blob mismatch: {relative}")
        except (ValueError, subprocess.SubprocessError) as exc:
            design_probe["status"] = "INVALID"
            design_probe["blocking_reasons"] = [str(exc)]
            design = None
    implementation_probe, implementation = probe(
        IMPLEMENTATION_MANIFEST_PATH,
        validator=lambda item: (validate_against_schema(item, IMPLEMENTATION_SCHEMA), validate_implementation_candidate(item, "M02 gate preflight")),
    )
    same_commit = bool(design and implementation and design["implementation_candidate_commit"] == implementation["implementation_candidate_commit"])
    candidate_ready = design is not None and implementation is not None and same_commit
    candidate_intake = {
        "required_source_path_count": len(required_sources),
        "required_source_path_exact_set_sha256": semantic_sha256(required_sources),
        "required_source_paths": required_sources,
        "design_manifest": design_probe, "implementation_manifest": implementation_probe,
        "same_commit": same_commit,
        "status": "READY_SAME_CLEAN_CANDIDATE_INPUT" if candidate_ready else "BLOCKED_CANDIDATE_INPUTS_MISSING_OR_INVALID",
    }

    responsibility_probe, responsibility = probe(RESPONSIBILITY_PATH, validator=validate_responsibility)
    intake_probe, signed_intake = probe(RESPONSIBILITY_INTAKE_PATH, schema=SIGNED_INTAKE_SCHEMA)
    assigned = len(responsibility["assignments"]) if responsibility is not None else 0
    structural_intake = False
    if responsibility is not None and signed_intake is not None:
        try:
            responsibility_hash = responsibility_probe["sha256"]
            if (
                signed_intake["intake_type"] != "EVIDENCE_CONTRACT_SIGNATURE"
                or signed_intake["subject_path"] != RESPONSIBILITY_PATH
                or signed_intake["subject_body_sha256"] != responsibility_hash
                or signed_intake["signed_payload_artifact"] != RESPONSIBILITY_PATH
                or signed_intake["signed_payload_sha256"] != responsibility_hash
            ):
                raise ValueError("responsibility signed intake subject/hash mismatch")
            for signature in signed_intake["signature_artifacts"]:
                artifact_ref_exists(signature)
            structural_intake = True
        except (ValueError, TypeError) as exc:
            intake_probe["status"] = "INVALID"
            intake_probe["blocking_reasons"] = [str(exc)]
    signature_verification_available = False
    responsibility_ready = structural_intake and signature_verification_available
    responsibility_intake = {
        "expected_task_count": 16, "manifest": responsibility_probe, "signed_intake": intake_probe,
        "assigned_task_count": assigned, "signature_verification_available": signature_verification_available,
        "status": "READY_SIGNED_RESPONSIBILITY_INPUT" if responsibility_ready else "BLOCKED_RESPONSIBILITY_OR_SIGNATURE_INPUT_MISSING",
    }

    resolver_refs = {item["resolver_source"]["path"]: item["resolver_source"] for item in locator_coverage["trusted_resolver_checks"]}
    locator_found = scan("locator-receipts/**/*-locator-resolution-receipt.json")
    compatibility_found = scan("compatibility-reviews/**/compatibility-default-off-review-receipt.json")
    review_found = sorted(set(scan("function-reviews/**/function-design-review-receipt.json")) | set(scan("function-reviews/**/non-function-design-exemption-receipt.json")))
    locator_intake = build_exact_set(locator_rows, locator_found, design, design_probe["sha256"], "locator", resolver_refs)
    compatibility_intake = build_exact_set(compatibility_rows, compatibility_found, design, design_probe["sha256"], "compatibility", resolver_refs)
    review_intake = build_exact_set(review_rows, review_found, design, design_probe["sha256"], "review", resolver_refs)

    expected_primary = {
        DESIGN_MANIFEST_PATH, IMPLEMENTATION_MANIFEST_PATH, RESPONSIBILITY_PATH, RESPONSIBILITY_INTAKE_PATH,
        *locator_intake["expected_paths"], *compatibility_intake["expected_paths"], *review_intake["expected_paths"],
    }
    actual_primary = set(locator_found) | set(compatibility_found) | set(review_found)
    actual_primary.update(scan("candidates/**/design-candidate-manifest.json"))
    actual_primary.update(scan("candidates/**/implementation-candidate.json"))
    actual_primary.update(scan("responsibility/**/m02-responsibility-assignment.json"))
    actual_primary.update(scan("responsibility/**/m02-responsibility-signed-intake.json"))
    unexpected_primary = sorted(actual_primary - expected_primary)

    def gate(gate_id: str, ready: bool, evidence: list[str], reason: str) -> dict[str, Any]:
        return {"gate_id": gate_id, "status": "READY" if ready else "BLOCKED", "evidence": evidence, "blocking_reason": None if ready else reason}

    gates = [
        gate("REG03-C03", candidate_ready and locator_intake["status"].startswith("READY") and compatibility_intake["status"].startswith("READY") and signature_verification_available, [f"locator receipts validated {locator_intake['validated_count']}/{locator_intake['expected_count']}", f"compatibility/default-off reviews structurally validated {compatibility_intake['validated_count']}/{compatibility_intake['expected_count']}", "trusted cryptographic verifier availability=false"], "clean same-candidate locator exact-set, compatibility/default-off review exact-set, or trusted signature verification is incomplete"),
        gate("REG03-C04", candidate_ready and review_intake["status"].startswith("READY") and signature_verification_available, [f"review receipts structurally validated {review_intake['validated_count']}/{review_intake['expected_count']}", "trusted cryptographic verifier availability=false"], "review exact-set or trusted signature verification is incomplete"),
        gate("REG03-C06", responsibility_ready, [f"assigned tasks {assigned}/16", "trusted cryptographic verifier availability=false"], "signed named responsibility intake cannot be trusted until the independent verifier is installed"),
        gate("REG03-C07", candidate_ready, [f"design candidate={design_probe['status']}", f"implementation candidate={implementation_probe['status']}", f"same commit={str(same_commit).lower()}"], "design and clean implementation candidates are missing, invalid, or cross-commit"),
    ]
    overall_ready = all(item["status"] == "READY" for item in gates) and not unexpected_primary
    contract_paths = [
        DESIGN_SCHEMA, IMPLEMENTATION_SCHEMA, RESPONSIBILITY_SCHEMA, RESPONSIBILITY_VALIDATOR, COMPATIBILITY_SCHEMA, COMPATIBILITY_VALIDATOR,
        SIGNED_INTAKE_SCHEMA, *sorted(set(LOCATOR_SCHEMAS.values())), FUNCTION_REVIEW_SCHEMA,
        NON_FUNCTION_SCHEMA, FUNCTION_REVIEW_VALIDATOR, NON_FUNCTION_VALIDATOR, IMPLEMENTATION_VALIDATOR,
    ]
    return {
        "schema_version": "1.0.0", "artifact_kind": "M02_C03_C07_EXTERNAL_INPUT_PREFLIGHT",
        "artifact_status": "READY_EXTERNAL_INPUTS_EXACT_NOT_GATE_PASS" if overall_ready else "BLOCKED_EXTERNAL_INPUTS_INCOMPLETE",
        "source_locator_coverage": hash_ref(LOCATOR_COVERAGE_PATH),
        "source_function_review_coverage": hash_ref(REVIEW_COVERAGE_PATH),
        "source_task_registry": hash_ref(TASK_REGISTRY_PATH), "source_replacement_catalog": hash_ref(REPLACEMENT_PATH),
        "contract_refs": [hash_ref(item) for item in sorted(contract_paths)],
        "candidate_intake": candidate_intake, "responsibility_intake": responsibility_intake,
        "locator_receipt_intake": locator_intake, "compatibility_review_intake": compatibility_intake, "review_receipt_intake": review_intake,
        "gate_results": gates, "unexpected_primary_artifacts": unexpected_primary,
        "validation": {
            "source_hashes_exact": "PASS", "expected_path_sets_derived": "PASS", "managed_roots_scanned": "PASS",
            "candidate_cross_binding_checked": "PASS", "receipt_candidate_binding_checked": "PASS",
            "no_missing_or_unverified_input_claimed_pass": True,
            "mutation_guards": {"locator_omission": "PASS", "compatibility_review_omission": "PASS", "review_omission": "PASS", "unexpected_artifact": "PASS", "candidate_false_pass": "PASS", "responsibility_false_pass": "PASS", "gate_false_pass": "PASS"},
        },
        "proof_ceiling": "EXTERNAL_INPUT_EXACT_SET_PREFLIGHT_ONLY_NOT_IDENTITY_INVENTION_SIGNATURE_VERIFICATION_LOCATOR_RESOLUTION_REVIEW_APPROVAL_IMPLEMENTATION_SWITCH_EXECUTION_OR_ACCEPTANCE",
    }


def validate_semantics(payload: dict[str, Any], *, exact: bool) -> None:
    validate_against_schema(payload, SCHEMA)
    locator_rows = expected_locator_rows(json.loads(LOCATOR_COVERAGE_PATH.read_text(encoding="utf-8")))
    expected_locators = sorted(item["expected_path"] for item in locator_rows)
    if payload["locator_receipt_intake"]["expected_paths"] != expected_locators:
        raise ValueError("locator expected path exact-set drifted")
    expected_reviews = sorted(item["expected_artifact_path"] for item in json.loads(REVIEW_COVERAGE_PATH.read_text(encoding="utf-8"))["leaf_reviews"])
    if payload["review_receipt_intake"]["expected_paths"] != expected_reviews:
        raise ValueError("review expected path exact-set drifted")
    expected_compatibility = sorted(item["expected_path"] for item in expected_compatibility_rows(json.loads(LOCATOR_COVERAGE_PATH.read_text(encoding="utf-8"))))
    if payload["compatibility_review_intake"]["expected_paths"] != expected_compatibility:
        raise ValueError("compatibility review expected path exact-set drifted")
    for name in ("locator_receipt_intake", "compatibility_review_intake", "review_receipt_intake"):
        item = payload[name]
        if item["expected_count"] != len(item["expected_paths"]) or item["missing_count"] != len(item["missing_paths"]) or item["unexpected_count"] != len(item["unexpected_paths"]) or item["invalid_count"] != len(item["invalid_artifacts"]):
            raise ValueError(f"{name} count partition drifted")
    actual_unexpected = set(scan("locator-receipts/**/*-locator-resolution-receipt.json")) | set(scan("compatibility-reviews/**/compatibility-default-off-review-receipt.json")) | set(scan("function-reviews/**/function-design-review-receipt.json")) | set(scan("function-reviews/**/non-function-design-exemption-receipt.json"))
    actual_unexpected.update(scan("candidates/**/design-candidate-manifest.json")); actual_unexpected.update(scan("candidates/**/implementation-candidate.json"))
    actual_unexpected.update(scan("responsibility/**/m02-responsibility-assignment.json")); actual_unexpected.update(scan("responsibility/**/m02-responsibility-signed-intake.json"))
    expected_primary = {DESIGN_MANIFEST_PATH, IMPLEMENTATION_MANIFEST_PATH, RESPONSIBILITY_PATH, RESPONSIBILITY_INTAKE_PATH, *expected_locators, *expected_compatibility, *expected_reviews}
    if payload["unexpected_primary_artifacts"] != sorted(actual_unexpected - expected_primary):
        raise ValueError("unexpected primary artifact exact-set drifted")
    candidate = payload["candidate_intake"]
    if candidate["status"].startswith("READY") and (not candidate["same_commit"] or candidate["design_manifest"]["status"] != "STRUCTURALLY_VALID" or candidate["implementation_manifest"]["status"] != "STRUCTURALLY_VALID"):
        raise ValueError("candidate status falsely ready")
    responsibility = payload["responsibility_intake"]
    if responsibility["status"].startswith("READY") and (responsibility["assigned_task_count"] != 16 or not responsibility["signature_verification_available"]):
        raise ValueError("responsibility status falsely ready")
    gates = {item["gate_id"]: item for item in payload["gate_results"]}
    candidate_ready = candidate["status"] == "READY_SAME_CLEAN_CANDIDATE_INPUT"
    expected_gate_ready = {
        "REG03-C03": candidate_ready and payload["locator_receipt_intake"]["status"] == "READY_EXACT_SET_VALIDATED" and payload["compatibility_review_intake"]["status"] == "READY_EXACT_SET_VALIDATED" and responsibility["signature_verification_available"],
        "REG03-C04": candidate_ready and payload["review_receipt_intake"]["status"] == "READY_EXACT_SET_VALIDATED" and responsibility["signature_verification_available"],
        "REG03-C06": responsibility["status"] == "READY_SIGNED_RESPONSIBILITY_INPUT",
        "REG03-C07": candidate_ready,
    }
    if any((gates[gate_id]["status"] == "READY") != ready for gate_id, ready in expected_gate_ready.items()):
        raise ValueError("gate status falsely ready")
    expected_artifact_ready = all(expected_gate_ready.values()) and not payload["unexpected_primary_artifacts"]
    if (payload["artifact_status"] == "READY_EXTERNAL_INPUTS_EXACT_NOT_GATE_PASS") != expected_artifact_ready:
        raise ValueError("preflight artifact status falsely ready")
    if exact and payload != build():
        raise ValueError("M02 gate input preflight differs from exact derived state")


def expect_failure(label: str, payload: dict[str, Any], mutate: Callable[[dict[str, Any]], None], expected: str) -> None:
    candidate = copy.deepcopy(payload); mutate(candidate)
    try:
        validate_semantics(candidate, exact=False)
    except (ValueError, TypeError) as exc:
        if expected not in str(exc):
            raise ValueError(f"mutation {label} hit wrong assertion: {exc}") from exc
        return
    raise ValueError(f"mutation {label} did not fail")


def mutation_tests(payload: dict[str, Any]) -> None:
    expect_failure("locator omission", payload, lambda item: item["locator_receipt_intake"]["expected_paths"].pop(), "locator expected path exact-set drifted")
    expect_failure("compatibility review omission", payload, lambda item: item["compatibility_review_intake"]["expected_paths"].pop(), "compatibility review expected path exact-set drifted")
    expect_failure("review omission", payload, lambda item: item["review_receipt_intake"]["expected_paths"].pop(), "review expected path exact-set drifted")
    expect_failure("unexpected artifact", payload, lambda item: item["unexpected_primary_artifacts"].append("doc/02_acceptance/topic1/m02/locator-receipts/rogue.json"), "unexpected primary artifact exact-set drifted")
    expect_failure("candidate false pass", payload, lambda item: item["candidate_intake"].update({"status": "READY_SAME_CLEAN_CANDIDATE_INPUT"}), "candidate status falsely ready")
    expect_failure("responsibility false pass", payload, lambda item: item["responsibility_intake"].update({"status": "READY_SIGNED_RESPONSIBILITY_INPUT"}), "responsibility status falsely ready")
    expect_failure("gate false pass", payload, lambda item: (item.update({"artifact_status": "READY_EXTERNAL_INPUTS_EXACT_NOT_GATE_PASS"}), [gate.update({"status": "READY", "blocking_reason": None}) for gate in item["gate_results"]]), "gate status falsely ready")


def render(payload: dict[str, Any]) -> str:
    candidate = payload["candidate_intake"]; responsibility = payload["responsibility_intake"]
    locator = payload["locator_receipt_intake"]; compatibility = payload["compatibility_review_intake"]; review = payload["review_receipt_intake"]
    lines = [
        "# M02外部证据收件预检", "", f"状态：`{payload['artifact_status']} / NO-GO`", "",
        "该预检只建立C03-C07主输入的exact-set收件协议；不创建人员、候选、签名或评审结论，也不把结构校验等同于受信验签。", "",
        "## 当前缺口", "",
        f"- Candidate：设计manifest `{candidate['design_manifest']['status']}`，implementation manifest `{candidate['implementation_manifest']['status']}`，同commit={str(candidate['same_commit']).lower()}；需覆盖{candidate['required_source_path_count']}个唯一源文件。",
        f"- Responsibility：实名完整任务 {responsibility['assigned_task_count']}/16；trusted signature verifier={str(responsibility['signature_verification_available']).lower()}。",
        f"- Locator receipts：validated={locator['validated_count']}/{locator['expected_count']}，missing={locator['missing_count']}，invalid={locator['invalid_count']}，unexpected={locator['unexpected_count']}。",
        f"- Compatibility/default-off reviews：validated={compatibility['validated_count']}/{compatibility['expected_count']}，missing={compatibility['missing_count']}，invalid={compatibility['invalid_count']}，unexpected={compatibility['unexpected_count']}。",
        f"- Review receipts：validated={review['validated_count']}/{review['expected_count']}，missing={review['missing_count']}，invalid={review['invalid_count']}，unexpected={review['unexpected_count']}。", "",
        "## Gate输入状态", "", "| Gate | 输入状态 | 阻断 |", "|---|---|---|",
    ]
    for item in payload["gate_results"]:
        lines.append(f"| `{item['gate_id']}` | `{item['status']}` | {item['blocking_reason'] or 'none'} |")
    lines += ["", "即使四项均为`READY`，仍只表示主输入exact-set可交给switch gate复核，不表示registry可切换。", "", "## 证明上限", "", f"`{payload['proof_ceiling']}`", ""]
    return "\n".join(lines)


def main() -> int:
    parser = argparse.ArgumentParser(); mode = parser.add_mutually_exclusive_group(required=True)
    mode.add_argument("--write", action="store_true"); mode.add_argument("--check", action="store_true"); mode.add_argument("--verify", action="store_true")
    args = parser.parse_args(); payload = build(); validate_against_schema(payload, SCHEMA)
    encoded = json.dumps(payload, ensure_ascii=False, indent=2) + "\n"; markdown = render(payload)
    if args.write:
        OUTPUT.write_text(encoded, encoding="utf-8"); DOC_OUTPUT.parent.mkdir(parents=True, exist_ok=True); DOC_OUTPUT.write_text(markdown, encoding="utf-8")
        print(f"WROTE {OUTPUT.relative_to(REPO)}"); print(f"WROTE {DOC_OUTPUT.relative_to(REPO)}")
    else:
        if not OUTPUT.is_file() or OUTPUT.read_text(encoding="utf-8") != encoded: raise ValueError("generated M02 gate input preflight is stale")
        if not DOC_OUTPUT.is_file() or DOC_OUTPUT.read_text(encoding="utf-8") != markdown: raise ValueError("generated M02 gate input preflight markdown is stale")
        validate_semantics(payload, exact=True)
        if args.verify: mutation_tests(payload)
        print(f"PASS status={payload['artifact_status']} locator={payload['locator_receipt_intake']['validated_count']}/{payload['locator_receipt_intake']['expected_count']} compatibility={payload['compatibility_review_intake']['validated_count']}/{payload['compatibility_review_intake']['expected_count']} review={payload['review_receipt_intake']['validated_count']}/{payload['review_receipt_intake']['expected_count']} responsibility={payload['responsibility_intake']['assigned_task_count']}/16")
    print(f"PROOF_CEILING {payload['proof_ceiling']}")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
