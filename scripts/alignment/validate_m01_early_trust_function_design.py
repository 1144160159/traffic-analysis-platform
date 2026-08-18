#!/usr/bin/env python3
"""Validate the M01 N015 function-complete static design and claim ceiling."""

from __future__ import annotations

import argparse
import ast
import copy
import hashlib
import json
import re
from pathlib import Path
from typing import Any, Callable

from build_topic1_task_registry import validate_against_schema
from generate_m01_early_trust_function_design import (
    ALLOCATION,
    BLOCKED,
    BUILDER,
    CATALOG,
    EXTERNAL_IMAGE,
    FUNCTION_IDS,
    INVARIANTS,
    MEMBER_IDS,
    NON_FUNCTION_IDS,
    OUTPUT as DESIGN,
    SCHEMA,
    SIGNATURES,
    TERMINAL,
    TYPE_IDS,
    build,
    load,
    pr_number,
)


REPO = Path(__file__).resolve().parents[2]
ACTIVE_CATALOGS = [
    REPO / "contracts/alignment/task-registry.v1.json",
    REPO / "contracts/alignment/developer-claim-package-catalog.v1.json",
    REPO / "contracts/alignment/pr-design-application-catalog.v1.json",
    REPO / "contracts/alignment/task-execution-overlay.template.v1.json",
]
EXPECTED_TESTS = {f"TC-M01-TRUST-{index:02d}" for index in range(1, 43)}
EXPECTED_REVIEW_KINDS = {
    **{item: "FUNCTION_DESIGN_REVIEW_RECEIPT" for item in FUNCTION_IDS},
    **{item: "FUNCTION_DESIGN_REVIEW_RECEIPT" for item in TYPE_IDS},
    **{item: "NON_FUNCTION_DESIGN_EXEMPTION_RECEIPT" for item in NON_FUNCTION_IDS},
}
EVIDENCE_GATES = {
    "T1-M01-P080-TST-PRE-n015-s23": "G0",
    "T1-M01-P081-TST-PRE-n015-s24": "G1",
    "T1-M01-P083-OPS-n015-s26": "G6",
    "T1-M01-P084-TST-POST-n015-s27": "G2",
    "T1-M01-P085-TST-POST-n015-s28": "G3",
}


def sha256(path: Path) -> str:
    return hashlib.sha256(path.read_bytes()).hexdigest()


def source_has_exact_signature(source: str, signature: str) -> bool:
    expected_node = ast.parse(signature + ":\n    pass").body[0]
    expected_shape = (ast.dump(expected_node.args, include_attributes=False), ast.dump(expected_node.returns, include_attributes=False))
    for node in ast.parse(source).body:
        if isinstance(node, (ast.FunctionDef, ast.AsyncFunctionDef)) and node.name == expected_node.name:
            actual_shape = (ast.dump(node.args, include_attributes=False), ast.dump(node.returns, include_attributes=False))
            return actual_shape == expected_shape
    return False


def source_has_symbol(path: Path, symbol: str) -> bool:
    if not path.is_file():
        return False
    module = ast.parse(path.read_text(encoding="utf-8"))
    return any(
        isinstance(node, (ast.FunctionDef, ast.AsyncFunctionDef, ast.ClassDef)) and node.name == symbol
        for node in ast.walk(module)
    )


def fail(code: str, detail: str) -> None:
    raise ValueError(f"{code}: {detail}")


def active_atomic_ids(path: Path) -> set[str]:
    payload = load(path)
    result: set[str] = set()

    def walk(value: Any) -> None:
        if isinstance(value, dict):
            for key, child in value.items():
                if key in {"pr_id", "atomic_pr_id"} and isinstance(child, str) and child.startswith("T1-"):
                    result.add(child)
                walk(child)
        elif isinstance(value, list):
            for child in value:
                walk(child)

    walk(payload)
    return result


def validate(payload: dict[str, Any]) -> None:
    try:
        validate_against_schema(payload, SCHEMA)
    except (ValueError, KeyError, TypeError) as exc:
        fail("E_SCHEMA", str(exc))

    roles = [item["role"] for item in payload["source_refs"]]
    paths = [item["path"] for item in payload["source_refs"]]
    expected_roles = {"V2_ALLOCATION", "V2_CATALOG", "ACTIVE_BUILDER", "FUNCTION_REVIEW_SCHEMA", "NON_FUNCTION_REVIEW_SCHEMA"}
    if set(roles) != expected_roles or len(roles) != len(set(roles)) or len(paths) != len(set(paths)):
        fail("E_SOURCE_SET", "source role and path exact-set is not unique")
    for ref in payload["source_refs"]:
        path = (REPO / ref["path"]).resolve()
        if not path.is_relative_to(REPO) or not path.is_file() or path.is_symlink():
            fail("E_SOURCE_UNSAFE", ref["path"])
        if sha256(path) != ref["sha256"]:
            fail("E_SOURCE_HASH", ref["path"])

    allocation = load(ALLOCATION)
    catalog = load(CATALOG)
    completion = catalog["completion_revision"]
    expected_members = completion["revised_member_atomic_pr_ids"]
    if expected_members != MEMBER_IDS or set(expected_members) != set(MEMBER_IDS):
        fail("E_UPSTREAM_MEMBERS", "validator constants differ from v2 completion")
    if completion["terminal_atomic_pr_id"] != TERMINAL or completion["revised_terminal_direct_dependency_ids"] != [NON_FUNCTION_IDS[-1]]:
        fail("E_TERMINAL_CONTRACT", "P066 identity or direct dependency drifted")
    scope_members = payload["design_scope"]["covered_atomic_pr_ids"]
    if scope_members != MEMBER_IDS or TERMINAL in scope_members:
        fail("E_MEMBER_SET", "design does not cover the exact 36 non-terminal members")
    if allocation["completion_revision"]["revised_member_atomic_pr_ids"] != MEMBER_IDS:
        fail("E_ALLOCATION_MEMBERS", "allocation and catalog completion differ")

    leaves = {item["atomic_pr_id"]: item for item in catalog["candidate_leaves"]}
    functions = payload["function_contracts"]
    function_by_id = {item["atomic_pr_id"]: item for item in functions}
    if len(function_by_id) != len(functions) or list(function_by_id) != FUNCTION_IDS:
        fail("E_FUNCTION_SET", "function contract exact ordered set drifted")
    types = payload["type_contracts"]
    type_by_id = {item["atomic_pr_id"]: item for item in types}
    if len(type_by_id) != len(types) or list(type_by_id) != TYPE_IDS:
        fail("E_TYPE_SET", "type contract exact ordered set drifted")
    non_functions = payload["non_function_contracts"]
    non_function_by_id = {item["atomic_pr_id"]: item for item in non_functions}
    if len(non_function_by_id) != len(non_functions) or list(non_function_by_id) != NON_FUNCTION_IDS:
        fail("E_NON_FUNCTION_SET", "non-function contract exact ordered set drifted")
    if set(function_by_id) | set(type_by_id) | set(non_function_by_id) != set(MEMBER_IDS):
        fail("E_CLASSIFICATION", "function type and non-function exact partition drifted")
    if TERMINAL in function_by_id or TERMINAL in type_by_id or TERMINAL in non_function_by_id:
        fail("E_TERMINAL_FABRICATION", "P066 was given an executable design contract")

    builder_text = BUILDER.read_text(encoding="utf-8")
    for atomic_id, contract in function_by_id.items():
        expected_locators = leaves[atomic_id]["write_locators"]
        actual_locators = [f"{contract['path']}#{contract['qualified_symbol']}", *contract["companion_target_locators"]]
        if actual_locators != expected_locators:
            fail("E_FUNCTION_LOCATOR", atomic_id)
        if leaves[atomic_id]["source_kind"] not in {"RETAINED_V1_ID_AND_WRITE_SCOPE", "APPENDED_PREVIEW_V2"}:
            fail("E_FUNCTION_SOURCE", atomic_id)
        before, after = SIGNATURES[atomic_id]
        if contract["signature_before"] != before or contract["signature_after"] != after:
            fail("E_SIGNATURE", atomic_id)
        if before is not None and not source_has_exact_signature(builder_text, before):
            fail("E_BEFORE_ABSENT", atomic_id)
        if before is None and contract["change_kind"] != "planned_create":
            fail("E_CHANGE_KIND", atomic_id)
        if before is None and source_has_symbol(REPO / contract["path"], contract["qualified_symbol"]):
            fail("E_PLANNED_CREATE_EXISTS", atomic_id)
        if before is not None and contract["change_kind"] != "planned_modify":
            fail("E_CHANGE_KIND", atomic_id)
        step_ids = [item.split(" ", 1)[0] for item in contract["body_steps"]]
        if step_ids != [f"B{index:02d}" for index in range(1, len(step_ids) + 1)]:
            fail("E_BODY_STEPS", atomic_id)
        if len(contract["error_branches"]) < 3:
            fail("E_ERROR_BRANCHES", atomic_id)
        if contract["design_review_status"] != "REQUIRED_NOT_PERFORMED":
            fail("E_FALSE_FUNCTION_REVIEW", atomic_id)

    for atomic_id, contract in type_by_id.items():
        expected_locators = leaves[atomic_id]["write_locators"]
        actual_locators = [f"{contract['path']}#{contract['qualified_symbol']}"]
        if atomic_id == TYPE_IDS[1]:
            actual_locators += [f"{contract['path']}#CandidateTrustContext", f"{contract['path']}#TrustedSignatureVerifier"]
        elif atomic_id == TYPE_IDS[2]:
            actual_locators += [f"{contract['path']}#{declaration}" for declaration in contract["companion_declarations"]]
        if expected_locators != actual_locators:
            fail("E_TYPE_LOCATOR", atomic_id)
        expected_primary = {
            TYPE_IDS[0]: ("SignatureVerificationRuntime", "@dataclass(frozen=True, slots=True) class SignatureVerificationRuntime"),
            TYPE_IDS[1]: ("CandidateRepository", "@dataclass(frozen=True, slots=True) class CandidateRepository"),
            TYPE_IDS[2]: ("SignatureVerificationRequest", "@dataclass(frozen=True, slots=True) class SignatureVerificationRequest"),
        }
        if (contract["qualified_symbol"], contract["declaration_after"]) != expected_primary[atomic_id]:
            fail("E_TYPE_DECLARATION", atomic_id)
        if source_has_symbol(REPO / contract["path"], contract["qualified_symbol"]):
            fail("E_PLANNED_TYPE_EXISTS", atomic_id)
        if len(contract["fields"]) < 6 or len(contract["invariants"]) < 5 or len(contract["construction_rules"]) < 4:
            fail("E_TYPE_COMPLETENESS", atomic_id)
        if contract["design_review_status"] != "REQUIRED_NOT_PERFORMED":
            fail("E_FALSE_FUNCTION_REVIEW", atomic_id)

    for atomic_id, contract in non_function_by_id.items():
        if contract["target_locators"] != leaves[atomic_id]["write_locators"]:
            fail("E_NON_FUNCTION_LOCATOR", atomic_id)
        if contract["required_review_artifact_kind"] != "NON_FUNCTION_DESIGN_EXEMPTION_RECEIPT" or contract["design_review_status"] != "REQUIRED_NOT_PERFORMED":
            fail("E_FALSE_NON_FUNCTION_REVIEW", atomic_id)

    tests = payload["test_cases"]
    test_by_id = {item["case_id"]: item for item in tests}
    if len(test_by_id) != len(tests) or set(test_by_id) != EXPECTED_TESTS:
        fail("E_TEST_SET", "test exact-set is not TC01-TC42")
    if any(item["execution_status"] != "NOT_RUN" for item in tests):
        fail("E_FALSE_TEST_EXECUTION", "a static design claims an executed test")
    covered: set[str] = set()
    for test in tests:
        subjects = set(test["subject_atomic_pr_ids"])
        if not subjects.issubset(MEMBER_IDS):
            fail("E_TEST_SUBJECT", test["case_id"])
        covered.update(subjects)
    if covered != set(MEMBER_IDS):
        fail("E_TEST_COVERAGE", "not all 36 members have a test subject")
    for contract in functions + types + non_functions:
        if not contract["tests"] or not set(contract["tests"]).issubset(EXPECTED_TESTS):
            fail("E_CONTRACT_TEST", contract["atomic_pr_id"])
        if any(contract["atomic_pr_id"] not in test_by_id[test_id]["subject_atomic_pr_ids"] for test_id in contract["tests"]):
            fail("E_CONTRACT_TEST_BINDING", contract["atomic_pr_id"])
    for atomic_id, gate in EVIDENCE_GATES.items():
        contract = non_function_by_id[atomic_id]
        if any(test_by_id[test_id]["gate_id"] != gate for test_id in contract["tests"]):
            fail("E_EVIDENCE_GATE", atomic_id)
        if leaves[atomic_id]["required_gates"] != [gate]:
            fail("E_CATALOG_EVIDENCE_GATE", atomic_id)

    sequence = payload["sequencing"]
    if [item["order"] for item in sequence] != list(range(1, 37)):
        fail("E_SEQUENCE_ORDER", "sequence order is not contiguous")
    ordered_ids = [item["atomic_pr_id"] for item in sequence]
    if set(ordered_ids) != set(MEMBER_IDS):
        fail("E_SEQUENCE_SET", "sequence does not cover the exact member set")
    position = {item: index for index, item in enumerate(ordered_ids)}
    for atomic_id in ordered_ids:
        for dependency in leaves[atomic_id]["dependency_ids"]:
            if dependency in position and position[dependency] >= position[atomic_id]:
                fail("E_SEQUENCE_TOPOLOGY", f"{dependency} does not precede {atomic_id}")
    p082 = leaves["T1-M01-P082-OPS-n015-s25"]
    if p082["dependency_ids"] != ["T1-M01-P081-TST-PRE-n015-s24", EXTERNAL_IMAGE]:
        fail("E_EXTERNAL_JOIN", "P082 does not join G1 and the external signed image receipt")

    reviews = payload["review_coverage"]
    review_by_id = {item["atomic_pr_id"]: item for item in reviews}
    if len(review_by_id) != len(reviews) or list(review_by_id) != MEMBER_IDS:
        fail("E_REVIEW_SET", "review coverage exact ordered set drifted")
    for atomic_id, row in review_by_id.items():
        if row["required_review_artifact_kind"] != EXPECTED_REVIEW_KINDS[atomic_id] or row["status"] != "MISSING_NOT_AUTHORED":
            fail("E_FALSE_REVIEW_STATUS", atomic_id)
        path = (REPO / row["expected_review_path"]).resolve()
        if not path.is_relative_to(REPO):
            fail("E_REVIEW_PATH", atomic_id)
        if path.exists():
            fail("E_REVIEW_FABRICATION", row["expected_review_path"])
        if len(row["blocking_reasons"]) < 3:
            fail("E_REVIEW_BLOCKERS", atomic_id)

    if len(payload["cross_cutting_invariants"]) < 12 or payload["cross_cutting_invariants"] != INVARIANTS:
        fail("E_INVARIANTS", "cross-cutting invariant exact-set drifted")
    secret_text = " ".join(item["secret_policy"] for item in payload["data_contracts"])
    required_secret_tokens = {"no credentials", "private keys", "raw resolved secret", "no secret is serialized"}
    if not all(token in secret_text for token in required_secret_tokens):
        fail("E_SECRET_POLICY", "secret non-disclosure policy was weakened")

    forbidden = set(payload["claims"]["forbidden"])
    required_forbidden = {"GLOBAL_REGISTRY_SWITCHED", "FUNCTION_DESIGN_REVIEWED", "IMPLEMENTED", "IMAGE_BUILT", "IMAGE_SIGNED", "DEPLOYED", "TEST_EXECUTED", "G0_PASS", "G1_PASS", "G2_PASS", "G3_PASS", "G6_PASS", "TRUST_PASS", "EXECUTION_AUTHORIZED", "PARENT_COMPLETE", "PRODUCTION_ACCEPTED"}
    if not required_forbidden.issubset(forbidden):
        fail("E_CLAIM_CEILING", "forbidden claim exact floor was weakened")
    switch = catalog["global_switch_gate"]
    if switch["decision"] != "BLOCKED_PREVIEW_ONLY" or switch["active_global_atomic_pr_count"] != 1289 or switch["candidate_global_atomic_pr_count"] != 1317:
        fail("E_SWITCH_GATE", "v2 preview switch decision or counts drifted")
    appended = set(FUNCTION_IDS[5:]) | set(TYPE_IDS) | set(NON_FUNCTION_IDS[1:])
    for path in ACTIVE_CATALOGS:
        if appended & active_atomic_ids(path):
            fail("E_ACTIVE_SWITCH", path.relative_to(REPO).as_posix())


def expect_reject(name: str, code: str, mutate: Callable[[dict[str, Any]], None], source: dict[str, Any]) -> None:
    candidate = copy.deepcopy(source)
    mutate(candidate)
    try:
        validate(candidate)
    except ValueError as exc:
        if str(exc).startswith(code + ":"):
            return
        raise ValueError(f"mutation {name} hit {exc!s}, expected {code}") from exc
    raise ValueError(f"malicious M01 early-trust mutation was accepted: {name}")


def self_test(payload: dict[str, Any]) -> None:
    def invert_sequence(value: dict[str, Any]) -> None:
        sequence = value["sequencing"]
        first = copy.deepcopy(sequence[0])
        second = copy.deepcopy(sequence[1])
        sequence[0] = {**second, "order": 1}
        sequence[1] = {**first, "order": 2}

    tests = [
        ("source_hash_drift", "E_SOURCE_HASH", lambda value: value["source_refs"][0].update(sha256="0" * 64)),
        ("member_omission", "E_SCHEMA", lambda value: value["design_scope"]["covered_atomic_pr_ids"].pop()),
        ("function_omission", "E_SCHEMA", lambda value: value["function_contracts"].pop()),
        ("type_omission", "E_SCHEMA", lambda value: value["type_contracts"].pop()),
        ("non_function_omission", "E_SCHEMA", lambda value: value["non_function_contracts"].pop()),
        ("signature_drift", "E_SIGNATURE", lambda value: value["function_contracts"][0].update(signature_after="def main() -> bool")),
        ("body_step_drift", "E_BODY_STEPS", lambda value: value["function_contracts"][0]["body_steps"].__setitem__(1, "B07 skip required work")),
        ("error_branch_omission", "E_SCHEMA", lambda value: value["function_contracts"][0].update(error_branches=value["function_contracts"][0]["error_branches"][:2])),
        ("test_omission", "E_SCHEMA", lambda value: value["test_cases"].pop()),
        ("uncovered_leaf", "E_TEST_COVERAGE", lambda value: [item.update(subject_atomic_pr_ids=[FUNCTION_IDS[0]]) for item in value["test_cases"] if NON_FUNCTION_IDS[-1] in item["subject_atomic_pr_ids"]]),
        ("wrong_gate", "E_EVIDENCE_GATE", lambda value: next(item for item in value["test_cases"] if item["case_id"] == "TC-M01-TRUST-29").update(gate_id="G0")),
        ("sequence_inversion", "E_SEQUENCE_TOPOLOGY", invert_sequence),
        ("review_false_ready", "E_SCHEMA", lambda value: value["review_coverage"][0].update(status="PASS")),
        ("terminal_function_fabrication", "E_SCHEMA", lambda value: value["function_contracts"].append({**value["function_contracts"][0], "contract_id": "FC-M01-N015-P066", "atomic_pr_id": TERMINAL})),
        ("secret_policy_weakening", "E_SECRET_POLICY", lambda value: [item.update(secret_policy="Secrets may be serialized for debugging under an operator flag.") for item in value["data_contracts"]]),
    ]
    for name, code, mutate in tests:
        expect_reject(name, code, mutate, payload)


def main() -> int:
    parser = argparse.ArgumentParser()
    parser.add_argument("--self-test", action="store_true")
    parser.add_argument("--check-generated", action="store_true")
    args = parser.parse_args()
    payload = load(DESIGN)
    validate(payload)
    if args.check_generated and payload != build():
        fail("E_GENERATED_DRIFT", "persisted design differs from deterministic builder")
    if args.self_test:
        self_test(payload)
    print("PASS M01 early-trust function design: 36 leaves, 25 exact function locators, 3 type owners, 8 non-function surfaces, 42 NOT_RUN tests, reviews 36 MISSING, active registries unchanged")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
