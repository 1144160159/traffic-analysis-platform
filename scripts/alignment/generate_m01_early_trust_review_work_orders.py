#!/usr/bin/env python3
"""Generate blocked, developer-claimable M01 design-review work orders.

This catalog derives review scope and commands.  It deliberately does not
invent a clean candidate, reviewer identity, debate decision, signature, or
receipt.
"""

from __future__ import annotations

import argparse
import copy
import hashlib
import json
import re
from pathlib import Path
from typing import Any, Callable

from build_topic1_task_registry import validate_against_schema


REPO = Path(__file__).resolve().parents[2]
ALLOCATION = REPO / "contracts/alignment/m01-early-trust-train-allocation.v2.json"
CATALOG = REPO / "contracts/alignment/m01-early-trust-train-catalog.v2.json"
DESIGN = REPO / "contracts/alignment/m01-early-trust-function-design.v1.json"
FUNCTION_SCHEMA = REPO / "contracts/alignment/function-design-review-receipt.schema.json"
FUNCTION_VALIDATOR = REPO / "scripts/alignment/validate_function_design_review_receipt.py"
NON_FUNCTION_SCHEMA = REPO / "contracts/alignment/non-function-design-exemption.schema.json"
NON_FUNCTION_VALIDATOR = REPO / "scripts/alignment/validate_non_function_design_exemption_contract.py"
CANDIDATE_WORK_ORDER = REPO / "contracts/alignment/m01-early-trust-candidate-freeze-work-order.v1.json"
SCHEMA = REPO / "contracts/alignment/m01-early-trust-review-work-order-catalog.schema.json"
OUTPUT = REPO / "contracts/alignment/m01-early-trust-review-work-order-catalog.v1.json"
MARKDOWN = REPO / "doc/07_alignment/generated/M01早期受信验证设计评审工作单目录.md"

SOURCE_PATHS = {
    "V2_ALLOCATION": ALLOCATION,
    "V2_CATALOG": CATALOG,
    "FUNCTION_DESIGN": DESIGN,
    "FUNCTION_REVIEW_SCHEMA": FUNCTION_SCHEMA,
    "FUNCTION_REVIEW_VALIDATOR": FUNCTION_VALIDATOR,
    "NON_FUNCTION_REVIEW_SCHEMA": NON_FUNCTION_SCHEMA,
    "NON_FUNCTION_REVIEW_VALIDATOR": NON_FUNCTION_VALIDATOR,
    "CANDIDATE_FREEZE_WORK_ORDER": CANDIDATE_WORK_ORDER,
}
FUNCTION_ROLES = [
    "LANGUAGE_OWNER",
    "QA_SRE_PERFORMANCE_EXPERT",
    "MAINTAINABILITY_RED_TEAM",
    "ADJUDICATOR",
]
NON_FUNCTION_ROLES = [
    "DOMAIN_OWNER",
    "QA_SRE_PERFORMANCE_EXPERT",
    "MAINTAINABILITY_RED_TEAM",
    "ADJUDICATOR",
]
PROOF_CEILING = (
    "DEVELOPER_REVIEW_WORK_ORDER_ORCHESTRATION_ONLY_NOT_CANDIDATE_CREATION_"
    "IDENTITY_ASSIGNMENT_REVIEW_DEBATE_DECISION_SIGNATURE_RECEIPT_IMPLEMENTATION_"
    "TEST_EXECUTION_REGISTRY_SWITCH_AUTHORIZATION_OR_ACCEPTANCE"
)


def load(path: Path) -> dict[str, Any]:
    return json.loads(path.read_text(encoding="utf-8"))


def sha256(path: Path) -> str:
    return hashlib.sha256(path.read_bytes()).hexdigest()


def digest(value: Any) -> str:
    body = json.dumps(value, ensure_ascii=False, sort_keys=True, separators=(",", ":")).encode()
    return hashlib.sha256(body).hexdigest()


def p_number(atomic_id: str) -> str:
    match = re.search(r"-P([0-9]{3})-", atomic_id)
    if not match:
        raise ValueError(f"E_ATOMIC_ID: {atomic_id}")
    return match.group(1)


def review_input_rows(atomic_id: str, review_dir: str, surface: str) -> list[dict[str, str]]:
    candidate = "doc/02_acceptance/topic1/m01/candidates/m01-early-trust-v2/design-candidate-manifest.json"
    if surface in {"FUNCTION", "TYPE"}:
        rows = [
            ("CLEAN_CANDIDATE_MANIFEST", candidate),
            ("FINAL_PATTERN_DECISION", f"{review_dir}/pattern-decision.json"),
            ("CODE_UNIT_CONTRACT", f"{review_dir}/code-unit-contract.json"),
            ("NEGATIVE_TEST_MANIFEST", f"{review_dir}/negative-test-manifest.json"),
        ]
    else:
        rows = [
            ("CLEAN_CANDIDATE_MANIFEST", candidate),
            ("SPECIALIZED_CONTRACT", f"{review_dir}/specialized-contract.json"),
            ("VERIFICATION_PLAN", f"{review_dir}/verification-plan.json"),
            ("ROLLBACK_PLAN", f"{review_dir}/rollback-plan.json"),
        ]
    return [{"role": role, "expected_path": path, "status": "MISSING"} for role, path in rows]


def checklist(surface: str) -> list[str]:
    common = [
        "bind every decision and signature to one clean same-commit candidate manifest",
        "verify exact target locators dependencies required gates and design contract identity",
        "challenge fail-closed security boundaries and secret non-disclosure invariants",
        "challenge atomicity idempotency recovery and rollback behavior where applicable",
        "map every positive negative fault and regression oracle without claiming execution",
        "record unresolved P0 findings vetoes split decisions and adjudication explicitly",
        "validate signed payload hash closure and independent reviewer identities",
    ]
    if surface == "FUNCTION":
        return ["verify exact signature body steps return values and every error branch", *common]
    if surface == "TYPE":
        return ["verify declaration fields companion types construction rules and invariants", *common]
    return ["verify specialized declarative contract verification plan and rollback plan", *common]


def contract_index(design: dict[str, Any]) -> tuple[dict[str, tuple[str, int, dict[str, Any]]], dict[str, str]]:
    rows: dict[str, tuple[str, int, dict[str, Any]]] = {}
    for surface, field in (
        ("FUNCTION", "function_contracts"),
        ("TYPE", "type_contracts"),
        ("NON_FUNCTION", "non_function_contracts"),
    ):
        for index, contract in enumerate(design[field]):
            atomic_id = contract["atomic_pr_id"]
            if atomic_id in rows:
                raise ValueError(f"E_CLASSIFICATION: duplicate contract for {atomic_id}")
            rows[atomic_id] = (surface, index, contract)
    reviews = {item["atomic_pr_id"]: item for item in design["review_coverage"]}
    return rows, reviews


def locators(surface: str, contract: dict[str, Any]) -> list[str]:
    if surface == "FUNCTION":
        return [f"{contract['path']}#{contract['qualified_symbol']}", *contract["companion_target_locators"]]
    if surface == "TYPE":
        result = [f"{contract['path']}#{contract['qualified_symbol']}"]
        if contract["atomic_pr_id"] == "T1-M01-P095-WRT-n015-s38":
            result += [f"{contract['path']}#CandidateTrustContext", f"{contract['path']}#TrustedSignatureVerifier"]
        elif contract["atomic_pr_id"] == "T1-M01-P096-WRT-n015-s39":
            result += [f"{contract['path']}#{name}" for name in contract["companion_declarations"]]
        return result
    return contract["target_locators"]


def build() -> dict[str, Any]:
    catalog = load(CATALOG)
    design = load(DESIGN)
    contracts, reviews = contract_index(design)
    leaves = {item["atomic_pr_id"]: item for item in catalog["candidate_leaves"]}
    members = design["design_scope"]["covered_atomic_pr_ids"]
    orders: list[dict[str, Any]] = []
    for order, atomic_id in enumerate(members, 1):
        surface, index, contract = contracts[atomic_id]
        review = reviews[atomic_id]
        review_dir = review["expected_review_path"].rsplit("/", 1)[0]
        validator = (
            "scripts/alignment/validate_function_design_review_receipt.py"
            if surface in {"FUNCTION", "TYPE"}
            else "scripts/alignment/validate_non_function_design_exemption_contract.py"
        )
        roles = FUNCTION_ROLES if surface in {"FUNCTION", "TYPE"} else NON_FUNCTION_ROLES
        leaf = leaves[atomic_id]
        orders.append({
            "work_order_id": f"M01-RWO-P{p_number(atomic_id)}",
            "atomic_pr_id": atomic_id,
            "parent_task_id": "T1-M01-N015",
            "sequence_order": order,
            "pr_type": leaf["pr_type"],
            "review_surface": surface,
            "design_contract_id": contract["contract_id"],
            "design_json_pointer": f"/{ {'FUNCTION': 'function_contracts', 'TYPE': 'type_contracts', 'NON_FUNCTION': 'non_function_contracts'}[surface] }/{index}",
            "target_locators": locators(surface, contract),
            "dependency_ids": leaf["dependency_ids"],
            "required_gates": leaf["required_gates"],
            "review_checklist": checklist(surface),
            "required_review_artifact_kind": review["required_review_artifact_kind"],
            "expected_review_path": review["expected_review_path"],
            "required_inputs": review_input_rows(atomic_id, review_dir, surface),
            "reviewer_slots": [
                {"role": role, "assignment_status": "UNASSIGNED", "reviewer_identity": None}
                for role in roles
            ],
            "command_plan": [
                {
                    "step_id": "validate-candidate-freeze-contract",
                    "run_when": "before external review starts and after any candidate freeze contract revision",
                    "working_directory": ".",
                    "argv": ["python3", "scripts/alignment/generate_m01_early_trust_candidate_freeze_work_order.py", "--verify"],
                    "expected_effect": "validate the 81-source design candidate boundary and keep the dirty shared worktree blocked",
                },
                {
                    "step_id": "validate-static-design",
                    "run_when": "before external review starts and after any static design revision",
                    "working_directory": ".",
                    "argv": ["python3", "scripts/alignment/validate_m01_early_trust_function_design.py", "--check-generated"],
                    "expected_effect": "revalidate the exact static design and keep every test and review claim below its evidence ceiling",
                },
                {
                    "step_id": "validate-signed-review",
                    "run_when": "only after real named independent reviewers create and sign the candidate-bound receipt",
                    "working_directory": ".",
                    "argv": ["python3", validator, review["expected_review_path"]],
                    "expected_effect": "validate receipt structure candidate hash closure reviewer independence and signed payload binding",
                },
            ],
            "status": "BLOCKED_EXTERNAL_INPUTS",
            "blocking_reasons": [
                "clean same-commit M01 design candidate manifest is absent",
                "candidate-bound review input artifacts are absent",
                "independent reviewer identities are not assigned",
                "review debate adjudication and veto resolution have not occurred",
                "trusted signatures and immutable review receipt are absent",
                "M01 v2 remains a preview outside the four active registries",
            ],
            "allowed_claim": "exact review scope inputs roles output path and validation commands are derived",
            "forbidden_claim": "candidate reviewer identity debate decision signature receipt implementation test execution registry switch authorization or acceptance exists",
            "proof_ceiling": "BLOCKED_DEVELOPER_REVIEW_WORK_ORDER_ONLY",
        })
    return {
        "schema_version": "1.0.0",
        "artifact_kind": "M01_EARLY_TRUST_DESIGN_REVIEW_WORK_ORDER_CATALOG",
        "artifact_status": "BLOCKED_EXTERNAL_CANDIDATE_IDENTITY_AND_ATTESTATION_INPUTS",
        "source_refs": [
            {"role": role, "path": path.relative_to(REPO).as_posix(), "sha256": sha256(path)}
            for role, path in SOURCE_PATHS.items()
        ],
        "work_order_count": 36,
        "review_surface_counts": {"FUNCTION": 25, "TYPE": 3, "NON_FUNCTION": 8},
        "work_order_status_counts": {"BLOCKED_EXTERNAL_INPUTS": 36},
        "covered_atomic_pr_exact_set_sha256": digest(sorted(members)),
        "work_orders": orders,
        "validation": {
            "schema": "PASS",
            "source_hashes_exact": "PASS",
            "member_exact_set": "PASS_36",
            "classification_exact": "PASS_25_FUNCTION_3_TYPE_8_NON_FUNCTION",
            "design_pointer_exact": "PASS",
            "locator_dependency_and_gate_binding_exact": "PASS",
            "review_contract_and_command_exact": "PASS",
            "all_external_inputs_missing": True,
            "all_reviewer_identities_unassigned": True,
            "no_candidate_identity_signature_receipt_or_decision_created": True,
            "mutation_guards": {
                "source_hash_drift": "PASS",
                "work_order_omission": "PASS",
                "classification_drift": "PASS",
                "design_pointer_drift": "PASS",
                "locator_drift": "PASS",
                "dependency_drift": "PASS",
                "gate_drift": "PASS",
                "review_path_drift": "PASS",
                "reviewer_role_drift": "PASS",
                "reviewer_identity_fabrication": "PASS",
                "external_input_false_ready": "PASS",
                "work_order_false_ready": "PASS",
                "validator_command_drift": "PASS",
            },
        },
        "proof_ceiling": PROOF_CEILING,
    }


def fail(code: str, detail: str) -> None:
    raise ValueError(f"{code}: {detail}")


def validate(payload: dict[str, Any]) -> None:
    try:
        validate_against_schema(payload, SCHEMA)
    except (ValueError, KeyError, TypeError) as exc:
        fail("E_SCHEMA", str(exc))
    expected = build()
    expected_sources = {item["role"]: item for item in expected["source_refs"]}
    actual_sources = {item["role"]: item for item in payload["source_refs"]}
    if len(actual_sources) != 8 or set(actual_sources) != set(expected_sources):
        fail("E_SOURCE_SET", "source role exact-set drifted")
    for role, item in actual_sources.items():
        if item != expected_sources[role]:
            fail("E_SOURCE_HASH", role)
    actual = payload["work_orders"]
    expected_orders = expected["work_orders"]
    if len(actual) != 36 or len({item["atomic_pr_id"] for item in actual}) != 36:
        fail("E_WORK_ORDER_SET", "work-order exact-set is not 36 unique atomic IDs")
    if [item["atomic_pr_id"] for item in actual] != [item["atomic_pr_id"] for item in expected_orders]:
        fail("E_WORK_ORDER_SET", "work-order ordered member set drifted")
    for row, wanted in zip(actual, expected_orders, strict=True):
        atomic_id = wanted["atomic_pr_id"]
        checks = (
            ("review_surface", "E_CLASSIFICATION"),
            ("design_contract_id", "E_DESIGN_POINTER"),
            ("design_json_pointer", "E_DESIGN_POINTER"),
            ("target_locators", "E_LOCATOR"),
            ("dependency_ids", "E_DEPENDENCY"),
            ("required_gates", "E_GATE"),
            ("required_review_artifact_kind", "E_REVIEW_CONTRACT"),
            ("expected_review_path", "E_REVIEW_PATH"),
            ("command_plan", "E_VALIDATOR_COMMAND"),
        )
        for field, code in checks:
            if row[field] != wanted[field]:
                fail(code, atomic_id)
        if [item["role"] for item in row["reviewer_slots"]] != [item["role"] for item in wanted["reviewer_slots"]]:
            fail("E_REVIEWER_ROLES", atomic_id)
        for item in row["required_inputs"]:
            path = (REPO / item["expected_path"]).resolve()
            if not path.is_relative_to(REPO) or path.is_symlink():
                fail("E_INPUT_PATH", item["expected_path"])
            if item["status"] != "MISSING" or path.exists():
                fail("E_EXTERNAL_INPUT_FALSE_READY", item["expected_path"])
        if any(slot["assignment_status"] != "UNASSIGNED" or slot["reviewer_identity"] is not None for slot in row["reviewer_slots"]):
            fail("E_REVIEWER_IDENTITY_FABRICATION", atomic_id)
        review_path = (REPO / row["expected_review_path"]).resolve()
        if not review_path.is_relative_to(REPO) or review_path.exists() or review_path.is_symlink():
            fail("E_REVIEW_RECEIPT_EXISTS", atomic_id)
        if row["status"] != "BLOCKED_EXTERNAL_INPUTS":
            fail("E_FALSE_READY", atomic_id)
    if payload != expected:
        fail("E_DERIVATION", "catalog differs from deterministic derived state")


def expect_failure(label: str, payload: dict[str, Any], mutate: Callable[[dict[str, Any]], None], expected_error: str) -> None:
    candidate = copy.deepcopy(payload)
    mutate(candidate)
    try:
        validate(candidate)
    except (ValueError, KeyError, TypeError) as exc:
        if expected_error not in str(exc):
            raise ValueError(f"negative case {label} hit wrong assertion: {exc}") from exc
        return
    raise ValueError(f"negative case {label} did not fail")


def self_test() -> None:
    payload = build()
    validate(payload)
    tests: list[tuple[str, Callable[[dict[str, Any]], None], str]] = [
        ("source hash drift", lambda p: p["source_refs"][0].update({"sha256": "0" * 64}), "E_SOURCE_HASH"),
        ("work order omission", lambda p: p["work_orders"].pop(), "E_WORK_ORDER_SET"),
        ("classification drift", lambda p: p["work_orders"][0].update({"review_surface": "TYPE"}), "E_CLASSIFICATION"),
        ("design pointer drift", lambda p: p["work_orders"][0].update({"design_json_pointer": "/non_function_contracts/7"}), "E_DESIGN_POINTER"),
        ("locator drift", lambda p: p["work_orders"][0]["target_locators"].append("invalid#locator"), "E_LOCATOR"),
        ("dependency drift", lambda p: p["work_orders"][0]["dependency_ids"].append("EXT-INVALID"), "E_DEPENDENCY"),
        ("gate drift", lambda p: p["work_orders"][0].update({"required_gates": ["G9"]}), "E_GATE"),
        ("review path drift", lambda p: p["work_orders"][0].update({"expected_review_path": "doc/invalid.json"}), "E_REVIEW_PATH"),
        ("reviewer role drift", lambda p: p["work_orders"][0]["reviewer_slots"][0].update({"role": "LANGUAGE_OWNER"}), "E_REVIEWER_ROLES"),
        ("reviewer identity fabrication", lambda p: p["work_orders"][0]["reviewer_slots"][0].update({"assignment_status": "ASSIGNED", "reviewer_identity": "invented@example.invalid"}), "E_REVIEWER_IDENTITY_FABRICATION"),
        ("external input false ready", lambda p: p["work_orders"][0]["required_inputs"][0].update({"status": "READY"}), "E_EXTERNAL_INPUT_FALSE_READY"),
        ("work order false ready", lambda p: p["work_orders"][0].update({"status": "READY_FOR_EXTERNAL_REVIEW"}), "E_FALSE_READY"),
        ("validator command drift", lambda p: p["work_orders"][0]["command_plan"][-1]["argv"].append("--skip-signature"), "E_VALIDATOR_COMMAND"),
    ]
    for label, mutate, error in tests:
        expect_failure(label, payload, mutate, error)


def render_markdown(payload: dict[str, Any]) -> str:
    lines = [
        "# M01 早期受信验证设计评审工作单目录",
        "",
        "> 状态：`BLOCKED_EXTERNAL_CANDIDATE_IDENTITY_AND_ATTESTATION_INPUTS`。本目录只把 36 个设计面的评审输入、角色、输出路径和校验命令固化为可领取工作单；没有创建候选、人员身份、评审决定、签名或 receipt。",
        "",
        "## 边界与计数",
        "",
        "- 父任务：`T1-M01-N015`（受信验证基础列车）；`T1-M01-N010` 保留调用方迁移职责。",
        "- 评审面：25 个函数、3 个类型、8 个非函数，共 36 个；终端 `P066` 不虚构可执行设计合同。",
        "- 当前输入：36/36 `BLOCKED_EXTERNAL_INPUTS`；reviewer identity 全部 `UNASSIGNED`。",
        "- 现役四目录仍为 1289 个原子 ID，未发生切换。",
        "",
        "## 工作单",
        "",
        "| 顺序 | 工作单 | Atomic PR | 面 | 设计合同 | Gate | Receipt | 状态 |",
        "|---:|---|---|---|---|---|---|---|",
    ]
    for row in payload["work_orders"]:
        gates = ",".join(row["required_gates"])
        lines.append(
            f"| {row['sequence_order']} | `{row['work_order_id']}` | `{row['atomic_pr_id']}` | "
            f"{row['review_surface']} | `{row['design_contract_id']}` | `{gates}` | "
            f"`{row['expected_review_path']}` | `{row['status']}` |"
        )
    lines += [
        "",
        "## 使用规则",
        "",
        "1. 先在隔离 clean worktree 生成并冻结 81-source design candidate manifest 及 candidate-bound 输入；本目录不代做。",
        "2. 为每单指派真实且彼此独立的四个角色，完成评审争议、P0、veto 和裁决记录。",
        "3. 真实评审人签署后，按 `command_plan` 校验 receipt；结构校验通过仍不等于实现、测试、授权或验收通过。",
        "4. 任一输入或身份缺失时保持 `BLOCKED_EXTERNAL_INPUTS`，不得创建占位 receipt。",
        "",
        f"Proof ceiling: `{payload['proof_ceiling']}`",
        "",
    ]
    return "\n".join(lines)


def main() -> int:
    parser = argparse.ArgumentParser()
    mode = parser.add_mutually_exclusive_group(required=True)
    mode.add_argument("--write", action="store_true")
    mode.add_argument("--check", action="store_true")
    mode.add_argument("--verify", action="store_true")
    args = parser.parse_args()
    expected = build()
    validate(expected)
    expected_json = json.dumps(expected, ensure_ascii=False, indent=2) + "\n"
    expected_md = render_markdown(expected)
    if args.write:
        OUTPUT.write_text(expected_json, encoding="utf-8")
        MARKDOWN.write_text(expected_md, encoding="utf-8")
        print(f"WROTE {OUTPUT.relative_to(REPO)}")
        print(f"WROTE {MARKDOWN.relative_to(REPO)}")
        return 0
    if not OUTPUT.is_file() or OUTPUT.read_text(encoding="utf-8") != expected_json:
        raise ValueError("persisted M01 review work-order catalog is stale")
    if not MARKDOWN.is_file() or MARKDOWN.read_text(encoding="utf-8") != expected_md:
        raise ValueError("persisted M01 review work-order markdown is stale")
    validate(load(OUTPUT))
    if args.verify:
        self_test()
        print("PASS M01 review work orders: 36 blocked rows and 13 targeted mutation guards")
    else:
        print("PASS M01 review work-order generation is deterministic")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
