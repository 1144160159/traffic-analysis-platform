#!/usr/bin/env python3
"""Positive and adversarial tests for the one-way function-design artifact chain."""

from __future__ import annotations

import copy
import hashlib
import json
import tempfile
from pathlib import Path
from typing import Any, Callable

from validate_function_design_contracts import (
    CATALOG_PATH,
    REPO_ROOT,
    canonical_sha256,
    load_json,
    validate_catalog,
    validate_code_unit_contract,
    validate_debate,
    validate_function_review,
    validate_pattern_decision,
    validate_pattern_proposal,
)


ATOMIC = "T1-M02-P001-WRT-n001-s1"
DECISION = f"PAT-{ATOMIC}"
PROPOSAL = f"PROP-{ATOMIC}-r1"
H40 = "1" * 40


def write_json(root: Path, name: str, value: dict[str, Any]) -> dict[str, str]:
    path = root / name
    path.write_text(json.dumps(value, ensure_ascii=False, indent=2) + "\n", encoding="utf-8")
    return {
        "path": str(path.relative_to(REPO_ROOT)),
        "sha256": hashlib.sha256(path.read_bytes()).hexdigest(),
    }


def candidate(manifest_ref: dict[str, str]) -> dict[str, str]:
    return {"commit": H40, "manifest_path": manifest_ref["path"], "manifest_sha256": manifest_ref["sha256"]}


def typed_ref(kind: str, ref: dict[str, str], candidate_sha: str | None = None, **identity: str) -> dict[str, str]:
    value = {"artifact_kind": kind, **identity, **ref}
    if candidate_sha is not None:
        value["candidate_manifest_sha256"] = candidate_sha
    return value


def must_reject(name: str, validator: Callable[[dict[str, Any]], None], payload: dict[str, Any]) -> None:
    try:
        validator(payload)
    except (ValueError, KeyError):
        print(f"PASS negative {name}")
        return
    raise AssertionError(f"negative case was accepted: {name}")


def option(option_id: str, form: str, pattern_ids: list[str], constraints: list[str]) -> dict[str, Any]:
    selected_designs = []
    for index, pattern_id in enumerate(pattern_ids):
        selected_designs.append({
            "pattern_id": pattern_id,
            "implementation_form": form if index == 0 else "NATIVE_LANGUAGE",
            "selection_role": "PRIMARY" if index == 0 else "AUXILIARY",
            "participant_bindings": [{"participant_role": "Command" if index == 0 else "State", "exact_symbol": "repository.(*AssetRepository).UpsertAtomic", "locator_id": "LOC-F1"}],
        })
    primary_constraint = constraints[0] if form == "PROJECT_ADAPTATION" else None
    bindings = []
    if pattern_ids:
        bindings = [{"participant_role": "Command", "exact_symbol": "repository.(*AssetRepository).UpsertAtomic", "locator_id": "LOC-F1"}]
    return {
        "option_id": option_id,
        "implementation_form": form,
        "selected_designs": selected_designs,
        "distributed_constraint_ids": constraints,
        "primary_distributed_constraint_id": primary_constraint,
        "participant_bindings": [],
        "invariants": ["accepted只在权威事务和outbox原子提交后成立"],
        "failure_semantics": "提交前回滚，提交未知按原幂等键查询receipt",
        "complexity": {"new_files": 1, "new_types": 2, "additional_call_hops": 1, "hot_path_allocations": "最多两个小对象", "cognitive_cost": "一个命令边界和一个事务副作用边界"},
        "compatibility_and_rollback": "旧入口通过适配器调用且新路由默认关闭",
        "negative_test_ids": ["NEG-SAME-KEY-DIFFERENT-PAYLOAD"],
        "adoption_predicate": "命令变化轴与事务副作用必须由同一稳定合同约束",
        "rejection_predicate": "直接函数已足够或新增间接层超过复杂度预算",
    }


def code_unit_payload(cand: dict[str, str], decision_ref: dict[str, str], generic_ref: dict[str, str]) -> dict[str, Any]:
    locator = {
        "locator_id": "LOC-F1", "locator_kind": "FUNCTION", "target_state": "EXISTING", "language": "go",
        "path": "go/control-plane/internal/policy/decide.go", "qualified_symbol": "policy.decide",
        "signature_before": "func decide(Input) Output", "signature_after": "func decide(Input) Output",
        "candidate_blob_sha256": "3" * 64, "ast_node_sha256": "4" * 64,
        "resolver_receipt_ref": generic_ref, "created_by_atomic_pr_id": None, "creation_reason": None,
        "compatibility_entrypoint": None, "activation_guard": None,
    }
    value = {"name": "result", "type": "Decision", "unit_or_format": "typed", "range": "valid decision", "identity_semantics": "tenant scoped", "ownership": "owned", "secret_classification": "INTERNAL"}
    aspect = lambda applicability, reason: {"applicability": applicability, "reason": reason, "details": {"contract": "显式边界和失败合同由定向测试覆盖"} if applicability == "APPLICABLE" else {}}
    unit = {
        "unit_id": "CU-F1", "profile_ref": "PURE@1", "kind": "function", "change_kind": "modify", "primary": True,
        "locator": locator, "purpose": "纯函数根据输入产生确定性决策", "non_goals": ["不访问数据库或网络"],
        "callers": [], "callees": [], "inputs": [dict(value, name="input", type="Input")], "outputs": [value],
        "preconditions": ["输入已通过schema校验"], "postconditions": ["结果只由输入决定"],
        "pattern_roles": [{"decision_id": DECISION, "pattern_id": "GOF-BEH-02", "participant_role": "Command"}],
        "body_steps": [{"step_id": "B01", "op": "decide", "guard": "始终执行", "reads": ["input"], "writes": [], "invokes": [], "invariant_before": ["无外部副作用"], "invariant_after": ["无外部副作用"], "error_ids": [], "cancel_point": "NONE", "oracle_ids": ["O-DECISION"]}],
        "side_effects": [], "profile_contract": {"profile": "PURE", "details": {"determinism": "相同规范化输入总是产生相同Decision", "input_output_invariants": "输出只由输入决定且不读取环境"}},
        "atomicity": aspect("NOT_APPLICABLE", "纯函数没有事务副作用"), "idempotency": aspect("DETERMINISTIC", "相同输入产生相同结果"),
        "concurrency": aspect("IMMUTABLE", "函数不共享可变状态"), "timeout_cancel": aspect("NOT_APPLICABLE", "函数内没有阻塞取消点"),
        "errors": [], "observability": {"trace": [], "metrics": [], "logs": [], "receipts": [], "forbidden_labels": ["tenant_id"]},
        "security": aspect("APPLICABLE", "输入必须保持租户身份不变量"),
        "performance": {"time_complexity": "O(1)", "space_complexity": "O(1)", "io_bound": "false", "allocation_budget": "zero on hot path", "batch_page_depth_limit": "single item", "profile_requirement": "unit benchmark when changed"},
        "compatibility": aspect("APPLICABLE", "签名保持调用方兼容"),
        "rollback": {"entrypoint": "restore previous pure function", "trigger": "contract test regression", "steps": ["revert authorized AST node"], "irreversible_boundary": "none", "data_policy": "no data effects", "oracle_ids": ["O-DECISION"]},
        "tests": [{"case_id": "TC-DECIDE", "kind": "success", "design_status": "PLANNED_CHANGE", "execution_status": "NOT_RUN", "covers_steps": ["B01"], "fixture_ref": "fixtures/policy.json", "oracle_ids": ["O-DECISION"], "evidence_ref": None, "limitation": "synthetic design-contract fixture; it is not runtime acceptance evidence"}],
    }
    return {
        "schema_version": "2.0.0", "artifact_kind": "CODE_UNIT_CONTRACT", "artifact_status": "DESIGN_CANDIDATE",
        "atomic_pr_id": ATOMIC, "parent_work_id": "T1-M02-N001", "candidate": cand, "design_scope": "FUNCTION_SET",
        "outcome": {"single_result": "纯决策函数满足冻结业务合同", "non_goals": ["不改变持久化和运行路由"]},
        "pattern_decision_ref": decision_ref, "context_locators": [], "code_units": [unit], "sql_migration_design": None,
        "call_flow": {"nodes": ["LOC-F1"], "edges": [], "generated_diagram_ref": None}, "companions": [], "contract_impact_refs": [],
        "plan_refs": {"test": generic_ref, "evidence": generic_ref, "rollback": generic_ref, "observation": None},
        "claims": {"allowed": ["CODE_UNIT_DESIGN_CANDIDATE"], "forbidden": ["READY_BINDING", "EXECUTION_AUTHORIZED", "IMPLEMENTATION_COMPLETE", "PRODUCTION_ACCEPTED"]},
        "review_status": "PENDING_FUNCTION_REVIEW",
        "readiness": {"status": "DESIGN_CANDIDATE", "blockers": [], "proof_ceiling": "CODE_UNIT_DESIGN_CANDIDATE_ONLY_NOT_FUNCTION_COMPLETE_OR_EXECUTION_AUTHORIZATION"},
    }


def main() -> int:
    validate_catalog(load_json(CATALOG_PATH))
    with tempfile.TemporaryDirectory(prefix="function-design-test-", dir=REPO_ROOT / "contracts" / "alignment") as temp_name:
        root = Path(temp_name)
        manifest_ref = write_json(root, "candidate.json", {"artifact_kind": "IMPLEMENTATION_CANDIDATE", "candidate": "fixture"})
        generic_ref = write_json(root, "generic.json", {"artifact_kind": "TEST_ARTIFACT", "result": "PASS"})
        negative_ref = write_json(root, "negative-tests.json", {"artifact_kind": "NEGATIVE_TEST_MANIFEST", "case_ids": ["NEG-SAME-KEY-DIFFERENT-PAYLOAD"]})
        cand = candidate(manifest_ref)
        catalog_ref = {"artifact_kind": "GOF_PATTERN_CATALOG", "path": str(CATALOG_PATH.relative_to(REPO_ROOT)), "sha256": hashlib.sha256(CATALOG_PATH.read_bytes()).hexdigest()}

        options = [
            option("OPT-DIRECT", "DIRECT", [], []),
            option("OPT-COMPOSITE", "GOF", ["GOF-BEH-02"], ["PROJECT-TRANSACTIONAL-OUTBOX"]),
        ]
        options[1]["selected_designs"][0]["participant_bindings"][0]["exact_symbol"] = "policy.decide"
        proposal = {
            "schema_version": "1.0.0", "artifact_kind": "PATTERN_DECISION_PROPOSAL", "artifact_status": "FROZEN_CANDIDATE",
            "proposal_id": PROPOSAL, "decision_id": DECISION, "atomic_pr_id": ATOMIC, "candidate": cand, "catalog_ref": catalog_ref,
            "design_scope": "FUNCTION_SET", "applicability": "APPLICABLE", "trigger_evidence": ["TRANSACTION_BOUNDARY", "CROSS_STORE_SIDE_EFFECT"],
            "problem_evidence": [{"path": CATALOG_PATH.relative_to(REPO_ROOT).as_posix(), "symbol_or_pointer": "patterns.GOF-BEH-02", "sha256": catalog_ref["sha256"]}],
            "change_axis": "命令执行和事务副作用的协作方式变化", "stable_contract": "受理receipt和原子事务边界保持稳定", "invariants": ["accepted仅在权威事务提交后成立"],
            "options": options, "option_set_sha256": canonical_sha256(sorted(options, key=lambda item: item["option_id"])),
            "negative_test_manifest_ref": {"artifact_kind": "NEGATIVE_TEST_MANIFEST", **negative_ref},
            "readiness": {"status": "FROZEN_CANDIDATE", "blockers": [], "proof_ceiling": "PATTERN_PROPOSAL_ONLY_NOT_REVIEW_OR_EXECUTION"},
        }
        validate_pattern_proposal(proposal)
        proposal_ref_raw = write_json(root, "proposal.json", proposal)
        proposal_ref = typed_ref("PATTERN_DECISION_PROPOSAL", proposal_ref_raw, cand["manifest_sha256"], proposal_id=PROPOSAL, decision_id=DECISION)

        selected = options[1]
        outcome = {"selected_option_id": selected["option_id"], "selected_option_sha256": canonical_sha256(selected)}
        canonical = {
            "candidate_manifest_sha256": cand["manifest_sha256"], "proposal_sha256": proposal_ref["sha256"],
            "option_set_sha256": proposal["option_set_sha256"], "catalog_sha256": catalog_ref["sha256"],
            "selected_option_id": selected["option_id"], "selected_option_sha256": canonical_sha256(selected),
            "negative_test_manifest_sha256": negative_ref["sha256"], "decision_outcome_sha256": canonical_sha256(outcome),
        }
        canonical_hash = canonical_sha256(canonical)
        roles = ["DOMAIN_OWNER", "LANGUAGE_OWNER", "RELIABILITY_DATA_EXPERT", "SECURITY_TENANT_EXPERT", "QA_SRE_PERFORMANCE_EXPERT", "MAINTAINABILITY_RED_TEAM"]
        submissions = {role: {"round": 2, "role": role, "reviewer_identity": f"expert-{index}", "recommended_option_id": selected["option_id"], "review_disposition": "UNIFIED", "body_ref": generic_ref, "p0": [], "p1": []} for index, role in enumerate(roles, 1)}
        attestations = {role: {"role": role, "reviewer_identity": submissions[role]["reviewer_identity"], "payload_sha256": canonical_hash, "purpose": "PATTERN_PROPOSAL_UNIFICATION", "policy_id": "TRUST-POLICY-1", "signature_artifact_ref": generic_ref} for role in roles}
        attestations["ADJUDICATOR"] = {"role": "ADJUDICATOR", "reviewer_identity": "adjudicator-1", "payload_sha256": canonical_hash, "purpose": "PATTERN_PROPOSAL_UNIFICATION", "policy_id": "TRUST-POLICY-1", "signature_artifact_ref": generic_ref}
        debate = {
            "schema_version": "2.0.0", "artifact_kind": "PATTERN_DEBATE_RECEIPT", "receipt_id": "PDR-001", "proposal_ref": proposal_ref,
            "decision_id": DECISION, "atomic_pr_id": ATOMIC, "candidate": cand, "rounds": 2, "final_round": 2, "final_submissions": submissions,
            "adjudication": {"role": "ADJUDICATOR", "reviewer_identity": "adjudicator-1", "review_disposition": "UNIFIED", "selected_option_id": selected["option_id"], "reason": "P0和veto清零且六席选择同一组合方案", "body_ref": generic_ref},
            "canonical_review_payload": canonical, "canonical_review_payload_sha256": canonical_hash,
            "negative_test_results": [{"case_id": "NEG-001", "run_id": "RUN-001", "result": "PASS", "artifact_ref": generic_ref}],
            "unresolved_p0": [], "unresolved_p1": [], "vetoes": [], "selected_option_id": selected["option_id"], "review_disposition": "UNIFIED",
            "signed_at": "2026-08-12T12:00:00Z", "attestations": attestations,
            "proof_ceiling": "PATTERN_PROPOSAL_UNIFICATION_ONLY_NOT_FUNCTION_OR_EXECUTION_ACCEPTANCE",
        }
        validate_debate(debate)
        debate_ref_raw = write_json(root, "debate.json", debate)
        debate_ref = typed_ref("PATTERN_DEBATE_RECEIPT", debate_ref_raw, cand["manifest_sha256"], receipt_id="PDR-001")

        dispositions = [
            {"option_id": "OPT-DIRECT", "status": "REJECTED", "reason": "无法显式绑定跨存储事务副作用", "reopen_condition": "跨存储副作用被移除时重新评估"},
            {"option_id": "OPT-COMPOSITE", "status": "SELECTED", "reason": "以Command角色配合事务outbox约束", "reopen_condition": "复杂度预算或失败语义变化时重审"},
        ]
        final = {
            "schema_version": "2.0.0", "artifact_kind": "FINAL_PATTERN_DECISION", "artifact_status": "READY", "decision_id": DECISION,
            "atomic_pr_id": ATOMIC, "candidate": cand, "proposal_ref": proposal_ref, "debate_receipt_ref": debate_ref,
            "selected_option_id": selected["option_id"], "selected_option_sha256": canonical_sha256(selected), "option_dispositions": dispositions,
            "decision_outcome_sha256": canonical_sha256(outcome),
            "readiness": {"status": "READY", "blockers": [], "proof_ceiling": "FINAL_PATTERN_DECISION_ONLY_NOT_FUNCTION_OR_EXECUTION_ACCEPTANCE"},
        }
        validate_pattern_decision(final)
        final_ref_raw = write_json(root, "final-decision.json", final)
        final_ref = typed_ref("FINAL_PATTERN_DECISION", final_ref_raw, cand["manifest_sha256"], decision_id=DECISION)

        code_unit = code_unit_payload(cand, final_ref, generic_ref)
        validate_code_unit_contract(code_unit)
        code_ref_raw = write_json(root, "code-unit.json", code_unit)
        code_ref = typed_ref("CODE_UNIT_CONTRACT", code_ref_raw, cand["manifest_sha256"])
        exact_set = sorted({(unit["locator"]["path"], unit["locator"]["qualified_symbol"], unit["locator"]["signature_after"], unit["locator"]["ast_node_sha256"]) for unit in code_unit["code_units"]})
        oracle_set = sorted({oracle for unit in code_unit["code_units"] for test in unit["tests"] for oracle in test["oracle_ids"]})
        review_hash = canonical_sha256({
            "candidate_manifest_sha256": cand["manifest_sha256"],
            "pattern_decision_sha256": final_ref["sha256"],
            "code_unit_contract_sha256": code_ref["sha256"],
            "function_exact_set_sha256": canonical_sha256(exact_set),
            "test_oracle_exact_set_sha256": canonical_sha256(oracle_set),
            "negative_test_manifest_sha256": negative_ref["sha256"],
            "review_disposition": "UNIFIED",
        })
        function_review = {
            "schema_version": "1.0.0", "artifact_kind": "FUNCTION_DESIGN_REVIEW_RECEIPT", "receipt_id": "FDR-001", "atomic_pr_id": ATOMIC,
            "candidate": cand, "pattern_decision_ref": final_ref, "code_unit_contract_ref": code_ref,
            "function_exact_set_sha256": canonical_sha256(exact_set), "test_oracle_exact_set_sha256": canonical_sha256(oracle_set),
            "negative_test_manifest_ref": {"artifact_kind": "NEGATIVE_TEST_MANIFEST", **negative_ref, "candidate_manifest_sha256": cand["manifest_sha256"]}, "review_disposition": "UNIFIED",
            "unresolved_p0": [], "vetoes": [], "signed_payload_sha256": review_hash,
            "attestations": [
                {"role": "LANGUAGE_OWNER", "reviewer_identity": "function-expert-1", "payload_sha256": review_hash, "policy_id": "TRUST-POLICY-1", "signature_artifact_ref": generic_ref},
                {"role": "MAINTAINABILITY_RED_TEAM", "reviewer_identity": "function-expert-2", "payload_sha256": review_hash, "policy_id": "TRUST-POLICY-1", "signature_artifact_ref": generic_ref},
            ],
            "proof_ceiling": "FUNCTION_DESIGN_REVIEW_ONLY_NOT_EXECUTION_OR_IMPLEMENTATION_ACCEPTANCE",
        }
        validate_function_review(function_review)
        print("PASS positive proposal -> debate -> final ADR -> code-unit -> function-review chain")

        bad = copy.deepcopy(proposal); bad["option_set_sha256"] = "0" * 64
        must_reject("proposal option-set swap", validate_pattern_proposal, bad)
        bad = copy.deepcopy(proposal); bad["options"][1]["selected_designs"][0]["selection_role"] = "AUXILIARY"; bad["option_set_sha256"] = canonical_sha256(sorted(bad["options"], key=lambda item: item["option_id"]))
        must_reject("GOF option without primary", validate_pattern_proposal, bad)
        bad = copy.deepcopy(debate); bad["code_unit_contract_sha256"] = "0" * 64
        must_reject("debate backlink to future code-unit", validate_debate, bad)
        bad = copy.deepcopy(final); bad["proposal_ref"] = typed_ref("PATTERN_DECISION_PROPOSAL", catalog_ref, cand["manifest_sha256"], proposal_id=PROPOSAL, decision_id=DECISION)
        must_reject("typed ref swap", validate_pattern_decision, bad)
        bad = copy.deepcopy(final); bad["candidate"]["manifest_sha256"] = "9" * 64
        must_reject("cross-candidate final ADR", validate_pattern_decision, bad)
        revise_debate = copy.deepcopy(debate); revise_debate["review_disposition"] = "REVISE"; revise_debate["selected_option_id"] = None
        revise_debate["adjudication"]["review_disposition"] = "REVISE"; revise_debate["adjudication"]["selected_option_id"] = None
        revise_ref_raw = write_json(root, "revise-debate.json", revise_debate)
        bad = copy.deepcopy(final); bad["debate_receipt_ref"] = typed_ref("PATTERN_DEBATE_RECEIPT", revise_ref_raw, cand["manifest_sha256"], receipt_id="PDR-REVISE")
        must_reject("final ADR points to REVISE debate", validate_pattern_decision, bad)
        bad_debate = copy.deepcopy(debate); bad_debate["canonical_review_payload"]["decision_outcome_sha256"] = "6" * 64
        bad_debate["canonical_review_payload_sha256"] = canonical_sha256(bad_debate["canonical_review_payload"])
        for attestation in bad_debate["attestations"].values():
            attestation["payload_sha256"] = bad_debate["canonical_review_payload_sha256"]
        must_reject("arbitrary decision outcome digest", validate_debate, bad_debate)
        alternate_options = copy.deepcopy(options)
        alternate_options[1]["failure_semantics"] = "替代revision改变了失败合同但沿用相同option ID"
        alternate_proposal = copy.deepcopy(proposal)
        alternate_proposal["proposal_id"] = f"PROP-{ATOMIC}-r2"
        alternate_proposal["options"] = alternate_options
        alternate_proposal["option_set_sha256"] = canonical_sha256(sorted(alternate_options, key=lambda item: item["option_id"]))
        validate_pattern_proposal(alternate_proposal)
        alternate_ref_raw = write_json(root, "alternate-proposal.json", alternate_proposal)
        alternate_ref = typed_ref("PATTERN_DECISION_PROPOSAL", alternate_ref_raw, cand["manifest_sha256"], proposal_id=alternate_proposal["proposal_id"], decision_id=DECISION)
        bad = copy.deepcopy(final); bad["proposal_ref"] = alternate_ref
        bad["selected_option_sha256"] = canonical_sha256(alternate_options[1])
        bad["decision_outcome_sha256"] = canonical_sha256({"selected_option_id": selected["option_id"], "selected_option_sha256": bad["selected_option_sha256"]})
        must_reject("final ADR mixes proposal revisions", validate_pattern_decision, bad)
        bad = copy.deepcopy(code_unit); bad["review_receipt_ref"] = generic_ref
        must_reject("code-unit backlink to future review", validate_code_unit_contract, bad)
        bad = copy.deepcopy(code_unit); bad["code_units"][0]["pattern_roles"][0]["participant_role"] = "Receiver"
        must_reject("code-unit participant drift", validate_code_unit_contract, bad)
        bad = copy.deepcopy(function_review); bad["function_exact_set_sha256"] = "8" * 64
        must_reject("signed function exact-set swap", validate_function_review, bad)
        bad = copy.deepcopy(function_review); bad["review_disposition"] = "REVISE"
        must_reject("signed review disposition swap", validate_function_review, bad)
        bad = copy.deepcopy(function_review); bad["signed_payload_sha256"] = "7" * 64
        for attestation in bad["attestations"]:
            attestation["payload_sha256"] = "7" * 64
        must_reject("arbitrary unanimous signature hash", validate_function_review, bad)
        bad = copy.deepcopy(function_review); bad["pattern_decision_ref"]["sha256"] = proposal_ref["sha256"]
        must_reject("function review alternate decision ref", validate_function_review, bad)

    print("PROOF_CEILING DESIGN_CONTRACT_TEST_ONLY_NOT_EXECUTION_AUTHORIZATION")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
