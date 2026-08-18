#!/usr/bin/env python3
"""Generate the blocked M01 two-stage candidate-freeze work order."""

from __future__ import annotations

import argparse
import copy
from collections import defaultdict
import hashlib
import json
from pathlib import Path
from typing import Any, Callable

from build_topic1_task_registry import validate_against_schema


REPO = Path(__file__).resolve().parents[2]
ALLOCATION = REPO / "contracts/alignment/m01-early-trust-train-allocation.v2.json"
CATALOG = REPO / "contracts/alignment/m01-early-trust-train-catalog.v2.json"
DESIGN = REPO / "contracts/alignment/m01-early-trust-function-design.v1.json"
MANIFEST_SCHEMA = REPO / "contracts/alignment/design-candidate-manifest.schema.json"
FREEZER = REPO / "scripts/alignment/freeze_m01_early_trust_design_candidate.py"
SCHEMA = REPO / "contracts/alignment/m01-early-trust-candidate-freeze-work-order.schema.json"
OUTPUT = REPO / "contracts/alignment/m01-early-trust-candidate-freeze-work-order.v1.json"
MARKDOWN = REPO / "doc/07_alignment/generated/M01早期受信验证两阶段候选冻结工作单.md"
DESIGN_MANIFEST = "doc/02_acceptance/topic1/m01/candidates/m01-early-trust-v2/design-candidate-manifest.json"
IMPLEMENTATION_MANIFEST = "doc/02_acceptance/topic1/m01/candidates/m01-early-trust-v2/implementation-candidate.json"
IMAGE_RECEIPT = "doc/02_acceptance/topic1/m01/external-activities/verifier-image-build-sign-publish/receipt.json"
SOURCE_PATHS = {
    "V2_ALLOCATION": ALLOCATION,
    "V2_CATALOG": CATALOG,
    "FUNCTION_DESIGN": DESIGN,
    "DESIGN_MANIFEST_SCHEMA": MANIFEST_SCHEMA,
    "DESIGN_CANDIDATE_FREEZER": FREEZER,
}
PLANNED_ID_HASH = "258379b7b06d81f4a022d02a204e698603502285ea91373c91764bccd9f07ef0"
PLANNED_OUTPUT_ID_HASH = "0e8dd8d5b6ff68b06725fea96424ff63f4b02ed8f2c439478126b296f89c2639"
PLANNED_PATH_HASH = "f8425368b56964bab900b492b0842fa7a4f2f7ab0ebe75abd3e67267a0241d5a"


def load(path: Path) -> dict[str, Any]:
    return json.loads(path.read_text(encoding="utf-8"))


def sha256(path: Path) -> str:
    return hashlib.sha256(path.read_bytes()).hexdigest()


def digest(value: Any) -> str:
    body = json.dumps(value, ensure_ascii=False, sort_keys=True, separators=(",", ":")).encode()
    return hashlib.sha256(body).hexdigest()


def safe_path(relative: str) -> Path:
    path = Path(relative)
    resolved = (REPO / path).resolve()
    if path.is_absolute() or ".." in path.parts or not resolved.is_relative_to(REPO):
        raise ValueError(f"E_PATH_ESCAPE: {relative}")
    cursor = REPO
    for part in path.parts:
        cursor /= part
        if cursor.is_symlink():
            raise ValueError(f"E_PATH_SYMLINK: {relative}")
    return resolved


def probe(relative: str) -> dict[str, Any]:
    path = safe_path(relative)
    return {
        "path": relative,
        "exists": path.is_file(),
        "sha256": sha256(path) if path.is_file() else None,
        "status": "PRESENT_UNVALIDATED" if path.is_file() else "MISSING",
    }


def candidate_partition(catalog: dict[str, Any]) -> tuple[list[dict[str, Any]], list[str], list[str]]:
    leaves = catalog["candidate_leaves"]
    planned = sorted(item["atomic_pr_id"] for item in leaves if item["target_state"] == "PLANNED")
    planned_output = sorted(
        item["atomic_pr_id"] for item in leaves if item["target_state"] == "PLANNED_OUTPUT"
    )
    owners: dict[str, set[str]] = defaultdict(set)
    output_paths: set[str] = set()
    for leaf in leaves:
        target = owners if leaf["target_state"] == "PLANNED" else None
        for locator in leaf["write_locators"]:
            path = locator.split("#", 1)[0]
            if target is None:
                output_paths.add(path)
            else:
                target[path].add(leaf["atomic_pr_id"])
    source_rows = [
        {
            "path": path,
            "owner_atomic_pr_ids": sorted(atomic_ids),
            "exists": safe_path(path).is_file(),
            "status": "PRESENT_UNVALIDATED" if safe_path(path).is_file() else "MISSING",
        }
        for path, atomic_ids in sorted(owners.items())
    ]
    if len(leaves) != 84 or len(planned) != 55 or len(planned_output) != 29:
        raise ValueError("E_LEAF_PARTITION: candidate leaf partition is not 55 PLANNED plus 29 PLANNED_OUTPUT")
    if set(owners) & output_paths or len(source_rows) != 81 or len(output_paths) != 54:
        raise ValueError("E_PATH_PARTITION: planned and planned-output path partition drifted")
    if digest(planned) != PLANNED_ID_HASH or digest(planned_output) != PLANNED_OUTPUT_ID_HASH:
        raise ValueError("E_LEAF_PARTITION_HASH: candidate leaf exact-set hash drifted")
    if digest(sorted(owners)) != PLANNED_PATH_HASH:
        raise ValueError("E_SOURCE_PATH_HASH: planned source exact-set hash drifted")
    return source_rows, planned, planned_output


def build() -> dict[str, Any]:
    catalog = load(CATALOG)
    source_rows, planned, planned_output = candidate_partition(catalog)
    design_probe = probe(DESIGN_MANIFEST)
    implementation_probe = probe(IMPLEMENTATION_MANIFEST)
    present = sum(item["exists"] for item in source_rows)
    return {
        "schema_version": "1.0.0",
        "artifact_kind": "M01_EARLY_TRUST_TWO_STAGE_CANDIDATE_FREEZE_WORK_ORDER",
        "artifact_status": "BLOCKED_IMPLEMENTATION_SOURCES_AND_CLEAN_WORKTREE",
        "work_order_id": "M01-EARLY-TRUST-V2-CANDIDATE-FREEZE",
        "source_refs": [
            {"role": role, "path": path.relative_to(REPO).as_posix(), "sha256": sha256(path)}
            for role, path in SOURCE_PATHS.items()
        ],
        "leaf_partition": {
            "candidate_leaf_count": 84,
            "planned_source_leaf_count": len(planned),
            "planned_output_leaf_count": len(planned_output),
            "planned_source_atomic_id_exact_set_sha256": digest(planned),
            "planned_output_atomic_id_exact_set_sha256": digest(planned_output),
            "planned_source_path_count": len(source_rows),
            "planned_output_path_count": 54,
            "path_overlap_count": 0,
            "planned_source_path_exact_set_sha256": digest([item["path"] for item in source_rows]),
            "partition_rule": "DESIGN_CANDIDATE_CONTAINS_ONLY_TARGET_STATE_PLANNED_GIT_BLOBS_PLANNED_OUTPUT_EVIDENCE_REMAINS_FOR_IMPLEMENTATION_CANDIDATE",
        },
        "design_candidate_stage": {
            "stage_order": 1,
            "candidate_id": "DESIGN-T1-M01-EARLY-TRUST-V2",
            "expected_manifest_path": DESIGN_MANIFEST,
            "manifest_probe": design_probe,
            "required_source_inputs": source_rows,
            "required_source_count": 81,
            "present_source_count": present,
            "missing_source_count": 81 - present,
            "freeze_preconditions": [
                "all 81 planned source paths exist as regular Git blobs",
                "candidate commit is exactly the isolated worktree HEAD commit",
                "worktree has no tracked or untracked changes outside the target manifest",
                "v2 catalog validates and the 55 plus 29 leaf partition remains exact",
                "immutable target manifest is absent or already has byte-identical content",
            ],
            "status": "BLOCKED_SOURCE_AND_CLEAN_COMMIT_MISSING",
        },
        "implementation_candidate_stage": {
            "stage_order": 2,
            "expected_manifest_path": IMPLEMENTATION_MANIFEST,
            "manifest_probe": implementation_probe,
            "depends_on": [
                "DESIGN_CANDIDATE_FROZEN",
                "PLANNED_OUTPUT_EVIDENCE_COMPLETE",
                "EXTERNAL_VERIFIER_IMAGE_RECEIPT_VALID",
            ],
            "same_commit_rule": "IMPLEMENTATION_CANDIDATE_COMMIT_MUST_EQUAL_DESIGN_CANDIDATE_IMPLEMENTATION_COMMIT",
            "required_schema_path": "contracts/alignment/implementation-candidate.schema.json",
            "semantic_validator_path": "scripts/alignment/build_topic1_task_registry.py#validate_implementation_candidate",
            "status": "BLOCKED_UPSTREAM_DESIGN_EVIDENCE_AND_IMAGE_RECEIPT",
        },
        "current_workspace": {
            "eligibility_rule": "ISOLATED_CLEAN_GIT_WORKTREE_ONLY",
            "required_dirty_count": 0,
            "candidate_commit_must_equal_head": True,
            "shared_worktree_not_designated": True,
            "candidate_outputs_absent": not design_probe["exists"] and not implementation_probe["exists"],
        },
        "command_plan": [
            {
                "step_id": "verify-static-candidate-contract",
                "run_when": "before candidate implementation starts and after any v2 catalog revision",
                "working_directory": ".",
                "argv": ["python3", "scripts/alignment/generate_m01_early_trust_candidate_freeze_work_order.py", "--verify"],
                "expected_effect": "recompute the exact two-stage scope and keep candidate creation blocked",
            },
            {
                "step_id": "freeze-design-candidate",
                "run_when": "only inside an isolated clean worktree after all 81 planned source blobs are committed",
                "working_directory": ".",
                "argv": ["python3", "scripts/alignment/freeze_m01_early_trust_design_candidate.py", "--repo-root", ".", "--candidate-commit", "<40-char-clean-head>", "--write"],
                "expected_effect": "create one immutable 81-source design candidate manifest or fail closed",
            },
            {
                "step_id": "check-design-candidate",
                "run_when": "after immutable design candidate creation and before review or external image execution",
                "working_directory": ".",
                "argv": ["python3", "scripts/alignment/freeze_m01_early_trust_design_candidate.py", "--repo-root", ".", "--candidate-commit", "<40-char-clean-head>", "--check"],
                "expected_effect": "recompute all candidate Git blob hashes and require byte-identical manifest content",
            },
            {
                "step_id": "validate-final-implementation-candidate",
                "run_when": "only after reviews planned-output evidence and external image receipt are complete on the same commit",
                "working_directory": ".",
                "argv": ["python3", "scripts/alignment/build_topic1_task_registry.py", "--validate-only"],
                "expected_effect": "invoke the registered implementation-candidate semantic validator without authorizing execution or registry switching",
            },
        ],
        "blocking_reasons": [
            f"planned source inputs present {present}/81 and missing {81 - present}/81",
            "candidate execution requires a separately designated isolated clean Git worktree",
            "candidate commit must equal that clean worktree HEAD with zero tracked or untracked changes",
            "design candidate manifest is absent",
            "all 36 candidate-bound design reviews and signatures are absent",
            "planned-output evidence and external verifier image receipt are absent",
            "implementation candidate manifest and execution authorization are absent",
        ],
        "validation": {
            "schema": "PASS",
            "source_hashes_exact": "PASS",
            "leaf_partition_exact": "PASS_55_PLANNED_29_PLANNED_OUTPUT",
            "source_path_exact_set": "PASS_81",
            "owner_mapping_exact": "PASS",
            "candidate_outputs_absent": True,
            "candidate_workspace_policy_exact": True,
            "stage_order_acyclic": "PASS_DESIGN_THEN_REVIEW_IMAGE_EVIDENCE_THEN_IMPLEMENTATION",
            "no_candidate_review_receipt_or_authority_created": True,
            "mutation_guards": {
                "source_hash_drift": "PASS",
                "leaf_partition_drift": "PASS",
                "source_input_omission": "PASS",
                "owner_mapping_drift": "PASS",
                "workspace_policy_drift": "PASS",
                "candidate_false_present": "PASS",
                "stage_order_drift": "PASS",
                "dependency_drift": "PASS",
                "command_drift": "PASS",
                "false_ready": "PASS",
            },
        },
        "allowed_claim": "two-stage candidate scope source owners blockers and clean-worktree commands are derived",
        "forbidden_claim": "candidate implementation review image receipt identity signature execution registry switch authorization or acceptance exists",
        "proof_ceiling": "BLOCKED_TWO_STAGE_CANDIDATE_FREEZE_WORK_ORDER_ONLY",
    }


def fail(code: str, detail: str) -> None:
    raise ValueError(f"{code}: {detail}")


def validate(payload: dict[str, Any]) -> None:
    try:
        validate_against_schema(payload, SCHEMA)
    except (ValueError, KeyError, TypeError) as exc:
        fail("E_SCHEMA", str(exc))
    expected = build()
    source_by_role = {item["role"]: item for item in payload["source_refs"]}
    expected_sources = {item["role"]: item for item in expected["source_refs"]}
    if set(source_by_role) != set(expected_sources):
        fail("E_SOURCE_SET", "source roles drifted")
    for role, item in source_by_role.items():
        if item != expected_sources[role]:
            fail("E_SOURCE_HASH", role)
    if payload["leaf_partition"] != expected["leaf_partition"]:
        fail("E_LEAF_PARTITION", "leaf or path partition drifted")
    rows = payload["design_candidate_stage"]["required_source_inputs"]
    if len(rows) != 81 or [item["path"] for item in rows] != [item["path"] for item in expected["design_candidate_stage"]["required_source_inputs"]]:
        fail("E_SOURCE_INPUT_SET", "planned source input exact-set drifted")
    if rows != expected["design_candidate_stage"]["required_source_inputs"]:
        fail("E_OWNER_MAPPING", "source owner or probe mapping drifted")
    if payload["current_workspace"] != expected["current_workspace"]:
        fail("E_WORKSPACE_POLICY", "candidate workspace policy drifted")
    if payload["design_candidate_stage"]["manifest_probe"]["exists"] or payload["implementation_candidate_stage"]["manifest_probe"]["exists"]:
        fail("E_CANDIDATE_FALSE_PRESENT", "candidate output exists in blocked work-order state")
    if payload["design_candidate_stage"]["stage_order"] != 1 or payload["implementation_candidate_stage"]["stage_order"] != 2:
        fail("E_STAGE_ORDER", "candidate stage order drifted")
    if payload["implementation_candidate_stage"]["depends_on"] != expected["implementation_candidate_stage"]["depends_on"]:
        fail("E_DEPENDENCY", "implementation candidate dependencies drifted")
    if payload["command_plan"] != expected["command_plan"]:
        fail("E_COMMAND", "candidate command plan drifted")
    if payload["artifact_status"] != "BLOCKED_IMPLEMENTATION_SOURCES_AND_CLEAN_WORKTREE":
        fail("E_FALSE_READY", payload["artifact_status"])
    if payload != expected:
        fail("E_DERIVATION", "candidate freeze work order differs from deterministic derived state")


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
        ("leaf partition drift", lambda p: p["leaf_partition"].update({"planned_source_leaf_count": 54}), "E_SCHEMA"),
        ("source input omission", lambda p: p["design_candidate_stage"]["required_source_inputs"].pop(), "schema minItems failed"),
        ("owner mapping drift", lambda p: p["design_candidate_stage"]["required_source_inputs"][0]["owner_atomic_pr_ids"].append("T1-M01-P999-WRT-n999-s1"), "E_OWNER_MAPPING"),
        ("workspace policy drift", lambda p: p["current_workspace"].update({"required_dirty_count": 1}), "E_SCHEMA"),
        ("candidate false present", lambda p: p["design_candidate_stage"]["manifest_probe"].update({"exists": True, "status": "PRESENT_UNVALIDATED", "sha256": "0" * 64}), "E_CANDIDATE_FALSE_PRESENT"),
        ("stage order drift", lambda p: p["implementation_candidate_stage"].update({"stage_order": 1}), "E_SCHEMA"),
        ("dependency drift", lambda p: p["implementation_candidate_stage"]["depends_on"].pop(), "schema minItems failed"),
        ("command drift", lambda p: p["command_plan"][1]["argv"].append("--allow-dirty"), "E_COMMAND"),
        ("false ready", lambda p: p.update({"artifact_status": "READY"}), "E_SCHEMA"),
    ]
    for label, mutate, error in tests:
        expect_failure(label, payload, mutate, error)


def render_markdown(payload: dict[str, Any]) -> str:
    stage = payload["design_candidate_stage"]
    lines = [
        "# M01 早期受信验证两阶段候选冻结工作单",
        "",
        "> 状态：`BLOCKED_IMPLEMENTATION_SOURCES_AND_CLEAN_WORKTREE`。本工作单不提交、不清理当前工作树，也不创建候选、评审、镜像回执或授权。",
        "",
        "## 两阶段边界",
        "",
        "1. Design candidate：只冻结 55 个 `PLANNED` 叶的 81 个唯一 Git blob，供 36 个设计评审和外部镜像构建绑定。",
        "2. Implementation candidate：等待 29 个 `PLANNED_OUTPUT` 叶、有效镜像回执及其他完整 artifact closure 后，在同一 commit 上生成并接受语义校验。",
        "",
        "该分层消除了‘镜像活动先要求 implementation candidate、implementation candidate 又先要求镜像回执’的循环。",
        "",
        "## 当前探针",
        "",
        f"- Design source：存在 {stage['present_source_count']}/81，缺失 {stage['missing_source_count']}/81。",
        "- 候选工作区规则：只能使用另行指定的隔离 clean Git worktree，dirty_count 必须为 0，candidate commit 必须等于 HEAD。",
        f"- Design manifest：`{stage['manifest_probe']['status']}`。",
        f"- Implementation manifest：`{payload['implementation_candidate_stage']['manifest_probe']['status']}`。",
        "",
        "## Source exact-set",
        "",
        "| Path | Owner atomic PRs | 状态 |",
        "|---|---|---|",
    ]
    for item in stage["required_source_inputs"]:
        owners = ", ".join(f"`{atomic_id}`" for atomic_id in item["owner_atomic_pr_ids"])
        lines.append(f"| `{item['path']}` | {owners} | `{item['status']}` |")
    lines.extend([
        "",
        "## 执行约束",
        "",
        "- 只能在隔离 clean worktree、81 个源码均已提交、HEAD 与传入 commit 完全一致时运行冻结命令。",
        "- 冻结器拒绝 symlink、缺失/非 blob 路径、dirty tracked/untracked 输入、moving HEAD 和覆盖不同字节的既有 manifest。",
        "- Design manifest 的结构/hash 通过不代表 implementation candidate、评审通过、测试通过、执行授权或 registry 切换。",
        "",
        f"Proof ceiling: `{payload['proof_ceiling']}`",
        "",
    ])
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
    body = json.dumps(expected, ensure_ascii=False, indent=2) + "\n"
    markdown = render_markdown(expected)
    if args.write:
        OUTPUT.write_text(body, encoding="utf-8")
        MARKDOWN.write_text(markdown, encoding="utf-8")
        print(f"WROTE {OUTPUT.relative_to(REPO)}")
        print(f"WROTE {MARKDOWN.relative_to(REPO)}")
        return 0
    if not OUTPUT.is_file() or OUTPUT.read_text(encoding="utf-8") != body:
        raise ValueError("persisted M01 candidate freeze work order is stale")
    if not MARKDOWN.is_file() or MARKDOWN.read_text(encoding="utf-8") != markdown:
        raise ValueError("persisted M01 candidate freeze work-order markdown is stale")
    validate(load(OUTPUT))
    if args.verify:
        self_test()
        print(
            "PASS M01 candidate freeze work order: 55 planned leaves, 29 planned-output "
            "leaves, 81 design sources, 10 targeted mutation guards"
        )
    else:
        print("PASS M01 candidate freeze work-order generation is deterministic")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
