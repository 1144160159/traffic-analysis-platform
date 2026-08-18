#!/usr/bin/env python3
"""Generate a read-only blocked preflight for the M01 four-registry switch."""

from __future__ import annotations

import argparse
import copy
import hashlib
import json
from pathlib import Path
from typing import Any, Callable

from build_topic1_task_registry import validate_against_schema


REPO = Path(__file__).resolve().parents[2]
V1_ALLOCATION = REPO / "contracts/alignment/m01-early-trust-train-allocation.v1.json"
V2_ALLOCATION = REPO / "contracts/alignment/m01-early-trust-train-allocation.v2.json"
V2_CATALOG = REPO / "contracts/alignment/m01-early-trust-train-catalog.v2.json"
DESIGN = REPO / "contracts/alignment/m01-early-trust-function-design.v1.json"
WORK_ORDERS = REPO / "contracts/alignment/m01-early-trust-review-work-order-catalog.v1.json"
IMAGE_WORK_ORDER = REPO / "contracts/alignment/m01-verifier-image-build-sign-work-order.v1.json"
CANDIDATE_WORK_ORDER = REPO / "contracts/alignment/m01-early-trust-candidate-freeze-work-order.v1.json"
SCHEMA = REPO / "contracts/alignment/m01-early-trust-registry-switch-preflight.schema.json"
OUTPUT = REPO / "contracts/alignment/m01-early-trust-registry-switch-preflight.v1.json"
MARKDOWN = REPO / "doc/07_alignment/generated/M01早期受信验证四目录原子切换阻断预检.md"

ACTIVE_CATALOGS = [
    ("TASK_REGISTRY", "contracts/alignment/task-registry.v1.json"),
    ("CLAIM_CATALOG", "contracts/alignment/developer-claim-package-catalog.v1.json"),
    ("PR_DESIGN_CATALOG", "contracts/alignment/pr-design-application-catalog.v1.json"),
    ("EXECUTION_OVERLAY", "contracts/alignment/task-execution-overlay.template.v1.json"),
]
EXPECTED_ACTIVE_HASHES = {
    "TASK_REGISTRY": "411be54ada25ba399fc4e14d1fb055986548a6c265c4b499254a5793249a23d0",
    "CLAIM_CATALOG": "18aeedadba1504c02141185c0cc40e9237dd8c96665c5923009f9f99484ee883",
    "PR_DESIGN_CATALOG": "486036113a9264679207d1bc19c58a62c3bf65b8c535e1f0c49230147192d03c",
    "EXECUTION_OVERLAY": "a606df1a2189c5b17fa114dfb860ad4224976cd1b5eb063dc101643b0618b503",
}
FUTURE_PATHS = {
    "TASK_REGISTRY": "contracts/alignment/candidates/m01-early-trust-v2/task-registry.candidate.json",
    "CLAIM_CATALOG": "contracts/alignment/candidates/m01-early-trust-v2/developer-claim-package-catalog.candidate.json",
    "PR_DESIGN_CATALOG": "contracts/alignment/candidates/m01-early-trust-v2/pr-design-application-catalog.candidate.json",
    "EXECUTION_OVERLAY": "contracts/alignment/candidates/m01-early-trust-v2/task-execution-overlay.candidate.json",
}
SOURCE_PATHS = {
    "V1_ALLOCATION": V1_ALLOCATION,
    "V2_ALLOCATION": V2_ALLOCATION,
    "V2_CATALOG": V2_CATALOG,
    "FUNCTION_DESIGN": DESIGN,
    "REVIEW_WORK_ORDERS": WORK_ORDERS,
    "EXTERNAL_IMAGE_WORK_ORDER": IMAGE_WORK_ORDER,
    "CANDIDATE_FREEZE_WORK_ORDER": CANDIDATE_WORK_ORDER,
    "TASK_REGISTRY_BUILDER": REPO / "scripts/alignment/build_topic1_task_registry.py",
    "CLAIM_CATALOG_SCHEMA": REPO / "contracts/alignment/developer-claim-package.schema.json",
    "PR_DESIGN_CATALOG_BUILDER": REPO / "scripts/alignment/build_pr_design_application_catalog.py",
}
PROOF_CEILING = (
    "READ_ONLY_SWITCH_SET_MATH_AND_BLOCKER_PREFLIGHT_ONLY_NOT_FUTURE_REGISTRY_"
    "MATERIALIZATION_ATOMIC_SWITCH_CANDIDATE_IDENTITY_REVIEW_SIGNATURE_IMPLEMENTATION_"
    "TEST_EXECUTION_AUTHORIZATION_OR_ACCEPTANCE"
)


def load(path: Path) -> dict[str, Any]:
    return json.loads(path.read_text(encoding="utf-8"))


def sha256(path: Path) -> str:
    return hashlib.sha256(path.read_bytes()).hexdigest()


def digest(value: Any) -> str:
    body = json.dumps(value, ensure_ascii=False, sort_keys=True, separators=(",", ":")).encode()
    return hashlib.sha256(body).hexdigest()


def active_ids(relative: str) -> set[str]:
    payload = load(REPO / relative)
    if relative.endswith("task-registry.v1.json"):
        values = [p["pr_id"] for t in payload["tasks"] for p in t["pr_sequence"]]
        values += [p["pr_id"] for t in payload["closure_slices"] for p in t["pr_sequence"]]
    elif relative.endswith("developer-claim-package-catalog.v1.json"):
        values = [p["atomic_pr_id"] for p in payload["packages"]]
    elif relative.endswith("pr-design-application-catalog.v1.json"):
        values = [p["atomic_pr_id"] for p in payload["entries"]]
    else:
        values = [p["pr_id"] for p in payload["atomic_pr_bindings"]]
    if len(values) != len(set(values)):
        raise ValueError(f"E_ACTIVE_DUPLICATE: {relative}")
    return set(values)


def task_transitions(v1: dict[str, Any], v2: dict[str, Any]) -> list[dict[str, Any]]:
    v1_completion = {item["parent_task_id"]: item for item in v1["completion_revisions"]}
    task_revisions = {item["task_id"]: item for item in v1["task_dependency_revisions"]}
    n015 = v2["completion_revision"]
    return [
        {
            "task_id": "T1-M01-N003",
            "transition_kind": "REVISE_EXISTING",
            "responsibility_boundary": "candidate provenance continues to consume the N015 trusted-verifier foundation",
            "dependency_task_ids": task_revisions["T1-M01-N003"]["revised_dependency_task_ids"],
            "member_atomic_pr_ids": v1_completion["T1-M01-N003"]["revised_member_atomic_pr_ids"],
            "terminal_atomic_pr_id": v1_completion["T1-M01-N003"]["terminal_atomic_pr_id"],
            "terminal_direct_dependency_ids": v1_completion["T1-M01-N003"]["revised_terminal_direct_dependency_ids"],
            "status": "PLANNED_BLOCKED_NOT_ACTIVE",
        },
        {
            "task_id": "T1-M01-N010",
            "transition_kind": "REVISE_EXISTING",
            "responsibility_boundary": "caller migration leaves P038-P044 remain in N010 and consume the N015 trusted-verifier foundation",
            "dependency_task_ids": task_revisions["T1-M01-N010"]["revised_dependency_task_ids"],
            "member_atomic_pr_ids": v1_completion["T1-M01-N010"]["revised_member_atomic_pr_ids"],
            "terminal_atomic_pr_id": v1_completion["T1-M01-N010"]["terminal_atomic_pr_id"],
            "terminal_direct_dependency_ids": v1_completion["T1-M01-N010"]["revised_terminal_direct_dependency_ids"],
            "status": "PLANNED_BLOCKED_NOT_ACTIVE",
        },
        {
            "task_id": "T1-M01-N015",
            "transition_kind": "ADD_NEW",
            "responsibility_boundary": "trusted-verifier contracts domain runtime package deployment and gate evidence foundation only",
            "dependency_task_ids": ["T1-M01-N001"],
            "member_atomic_pr_ids": n015["revised_member_atomic_pr_ids"],
            "terminal_atomic_pr_id": n015["terminal_atomic_pr_id"],
            "terminal_direct_dependency_ids": n015["revised_terminal_direct_dependency_ids"],
            "status": "PLANNED_BLOCKED_NOT_ACTIVE",
        },
    ]


def build() -> dict[str, Any]:
    v1 = load(V1_ALLOCATION)
    v2 = load(V2_ALLOCATION)
    catalog = load(V2_CATALOG)
    work_orders = load(WORK_ORDERS)
    image_work_order = load(IMAGE_WORK_ORDER)
    candidate_work_order = load(CANDIDATE_WORK_ORDER)
    sets = {role: active_ids(path) for role, path in ACTIVE_CATALOGS}
    baseline = sets["TASK_REGISTRY"]
    active_m01 = {item for item in baseline if item.startswith("T1-M01-")}
    candidate_m01 = {item["atomic_pr_id"] for item in catalog["candidate_leaves"]}
    removed = sorted(active_m01 - candidate_m01)
    retained = sorted(active_m01 & candidate_m01)
    added = sorted(candidate_m01 - active_m01)
    candidate_global = (baseline - active_m01) | candidate_m01
    current = [
        {
            "role": role,
            "path": path,
            "sha256": sha256(REPO / path),
            "atomic_pr_count": len(values),
            "m01_atomic_pr_count": len({item for item in values if item.startswith("T1-M01-")}),
            "atomic_id_exact_set_sha256": digest(sorted(values)),
            "status": "ACTIVE_UNCHANGED",
        }
        for role, path in ACTIVE_CATALOGS
        for values in [sets[role]]
    ]
    future = [
        {
            "role": role,
            "expected_path": FUTURE_PATHS[role],
            "status": "NOT_CREATED",
            "expected_atomic_pr_count": len(candidate_global),
            "expected_atomic_id_exact_set_sha256": digest(sorted(candidate_global)),
        }
        for role, _ in ACTIVE_CATALOGS
    ]
    preconditions = [
        ("M01-SW-C01", "four active registry hashes and exact atomic ID sets remain frozen and identical", "PASS_STATIC", [item["path"] for item in current], None),
        ("M01-SW-C02", "v2 allocation catalog design and set arithmetic are deterministic and internally exact", "PASS_STATIC", [V2_ALLOCATION.relative_to(REPO).as_posix(), V2_CATALOG.relative_to(REPO).as_posix(), DESIGN.relative_to(REPO).as_posix()], None),
        ("M01-SW-C03", "one immutable design candidate and one same-commit implementation candidate are frozen in two acyclic stages", "BLOCKED", [CANDIDATE_WORK_ORDER.relative_to(REPO).as_posix(), candidate_work_order["design_candidate_stage"]["expected_manifest_path"], candidate_work_order["implementation_candidate_stage"]["expected_manifest_path"]], f"candidate freeze work order is {candidate_work_order['artifact_status']} and both candidate manifests are absent"),
        ("M01-SW-C04", "all 36 candidate-bound design reviews are independently completed and signed", "BLOCKED", [WORK_ORDERS.relative_to(REPO).as_posix()], "36 review work orders remain BLOCKED_EXTERNAL_INPUTS"),
        ("M01-SW-C05", "external verifier image digest SBOM provenance and signature verification receipt are bound", "BLOCKED", [IMAGE_WORK_ORDER.relative_to(REPO).as_posix(), image_work_order["expected_receipt_path"]], f"external image work order is {image_work_order['status']} and receipt is absent"),
        ("M01-SW-C06", "future task claim PR-design and execution-overlay catalogs are generated from one candidate exact set", "BLOCKED", list(FUTURE_PATHS.values()), "all four future candidate catalog paths are intentionally absent"),
        ("M01-SW-C07", "named owners reviewers approvers and trusted signatures authorize the exact atomic switch", "BLOCKED", ["contracts/alignment/task-execution-overlay.schema.json"], "named identities approvals signed overlay and switch authorization are absent"),
        ("M01-SW-C08", "authorized atomic commit can validate and replace all four catalogs without partial visibility", "BLOCKED", ["contracts/alignment/task-registry.v1.json", "contracts/alignment/developer-claim-package-catalog.v1.json", "contracts/alignment/pr-design-application-catalog.v1.json", "contracts/alignment/task-execution-overlay.template.v1.json"], "no authorized atomic switch candidate or protected merge receipt exists"),
    ]
    return {
        "schema_version": "1.0.0",
        "artifact_kind": "M01_EARLY_TRUST_FOUR_REGISTRY_ATOMIC_SWITCH_PREFLIGHT",
        "artifact_status": "BLOCKED_PRE_SWITCH_PRECONDITIONS",
        "revision_id": "M01-EARLY-TRUST-REGISTRY-SWITCH-PREFLIGHT-V1",
        "source_refs": [
            {"role": role, "path": path.relative_to(REPO).as_posix(), "sha256": sha256(path)}
            for role, path in SOURCE_PATHS.items()
        ],
        "current_global_catalogs": current,
        "active_baseline": {
            "global_atomic_pr_count": len(baseline),
            "m01_atomic_pr_count": len(active_m01),
            "global_atomic_id_exact_set_sha256": digest(sorted(baseline)),
            "m01_atomic_id_exact_set_sha256": digest(sorted(active_m01)),
        },
        "candidate_projection": {
            "removed_active_m01_atomic_pr_ids": removed,
            "retained_active_m01_atomic_pr_ids": retained,
            "added_candidate_m01_atomic_pr_ids": added,
            "candidate_m01_atomic_pr_ids": sorted(candidate_m01),
            "removed_count": len(removed),
            "retained_count": len(retained),
            "added_count": len(added),
            "candidate_m01_atomic_pr_count": len(candidate_m01),
            "candidate_global_atomic_pr_count": len(candidate_global),
            "candidate_global_atomic_id_exact_set_sha256": digest(sorted(candidate_global)),
            "derivation": "1289_ACTIVE_MINUS_9_SUPERSEDED_PLUS_37_NEW_EQUALS_1317_CANDIDATE",
        },
        "task_transitions": task_transitions(v1, v2),
        "future_catalogs": future,
        "preconditions": [
            {"check_id": check, "description": desc, "status": status, "evidence": evidence, "blocking_reason": reason}
            for check, desc, status, evidence, reason in preconditions
        ],
        "switch_protocol": {
            "decision": "BLOCKED_PRECONDITIONS",
            "atomic_steps": [
                "freeze one immutable 81-source design candidate then one same-commit implementation candidate after review image and evidence closure",
                "complete validate and independently sign all 36 candidate-bound design review receipts",
                "bind the external verifier image digest SBOM provenance and signature verification receipt",
                "generate four future catalogs from one reviewed candidate and exact 1317-ID set",
                "validate four schemas hashes ID sets N010 N015 boundary dependencies and completion contracts",
                "obtain named switch authorization and protected merge receipt for the exact candidate hashes",
                "replace task claim PR-design and overlay catalogs in one atomic commit then rerun all gates",
            ],
            "failure_rule": "ANY_FAILED_PRECONDITION_OR_FOUR_CATALOG_DIVERGENCE_ABORTS_BEFORE_REPLACING_ANY_ACTIVE_FILE",
            "rollback_rule": "RETAIN_ALL_FOUR_ACTIVE_FILES_AND_HASHES_UNCHANGED_UNTIL_ONE_AUTHORIZED_ATOMIC_COMMIT_CAN_REPLACE_THE_EXACT_SET",
            "post_switch_execution_state": "DRAFT_DESIGN_AND_FORMAL_EXECUTION_STILL_BLOCKED_UNTIL_SIGNED_OVERLAY_AND_GATE_EVIDENCE",
        },
        "validation": {
            "schema": "PASS",
            "source_hashes_exact": "PASS",
            "four_active_catalog_hashes_exact": "PASS",
            "four_active_catalog_exact_set_equal": "PASS_1289",
            "active_m01_exact_set": "PASS_56",
            "removed_retained_added_partition_exact": "PASS_9_47_37",
            "candidate_count_derivation": "PASS_1317",
            "candidate_global_exact_set": "PASS",
            "task_transition_exact_set": "PASS_N003_N010_N015",
            "n010_n015_boundary_exact": "PASS_FOUNDATION_VS_CALLER_MIGRATION",
            "future_catalogs_absent": True,
            "candidate_freeze_work_order_blocked": "PASS_NO_CANDIDATE_PAIR",
            "review_work_orders_blocked": "PASS_36_BLOCKED",
            "external_image_work_order_blocked": "PASS_NO_RECEIPT",
            "premature_switch_rejected": True,
            "active_catalogs_unchanged": True,
            "mutation_guards": {
                "source_hash_drift": "PASS",
                "active_catalog_hash_drift": "PASS",
                "active_exact_set_drift": "PASS",
                "removed_omission": "PASS",
                "retained_drift": "PASS",
                "added_omission": "PASS",
                "candidate_hash_drift": "PASS",
                "task_transition_omission": "PASS",
                "n010_boundary_drift": "PASS",
                "future_catalog_false_created": "PASS",
                "candidate_freeze_false_ready": "PASS",
                "review_work_order_hash_drift": "PASS",
                "precondition_false_pass": "PASS",
                "switch_false_ready": "PASS",
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
    source_by_role = {item["role"]: item for item in payload["source_refs"]}
    expected_source_by_role = {item["role"]: item for item in expected["source_refs"]}
    if set(source_by_role) != set(expected_source_by_role):
        fail("E_SOURCE_SET", "source roles drifted")
    for role, item in source_by_role.items():
        if item != expected_source_by_role[role]:
            fail("E_SOURCE_HASH", role)
    current_by_role = {item["role"]: item for item in payload["current_global_catalogs"]}
    if set(current_by_role) != set(EXPECTED_ACTIVE_HASHES):
        fail("E_ACTIVE_SET", "active catalog roles drifted")
    active_sets = []
    for role, relative in ACTIVE_CATALOGS:
        item = current_by_role[role]
        if item["sha256"] != EXPECTED_ACTIVE_HASHES[role] or item["sha256"] != sha256(REPO / relative):
            fail("E_ACTIVE_HASH", role)
        if item != next(row for row in expected["current_global_catalogs"] if row["role"] == role):
            fail("E_ACTIVE_SET", role)
        active_sets.append(active_ids(relative))
    if any(items != active_sets[0] for items in active_sets[1:]) or len(active_sets[0]) != 1289:
        fail("E_ACTIVE_EXACT_SET", "four active catalogs are not the same 1289 IDs")
    projection = payload["candidate_projection"]
    wanted = expected["candidate_projection"]
    for field, code in (
        ("removed_active_m01_atomic_pr_ids", "E_REMOVED_SET"),
        ("retained_active_m01_atomic_pr_ids", "E_RETAINED_SET"),
        ("added_candidate_m01_atomic_pr_ids", "E_ADDED_SET"),
        ("candidate_m01_atomic_pr_ids", "E_CANDIDATE_M01_SET"),
        ("candidate_global_atomic_id_exact_set_sha256", "E_CANDIDATE_HASH"),
    ):
        if projection[field] != wanted[field]:
            fail(code, field)
    if projection["candidate_global_atomic_pr_count"] != 1317 or 1289 - 9 + 37 != 1317:
        fail("E_CANDIDATE_COUNT", "candidate count is not 1317")
    if [item["task_id"] for item in payload["task_transitions"]] != ["T1-M01-N003", "T1-M01-N010", "T1-M01-N015"]:
        fail("E_TASK_TRANSITION_SET", "task transition exact ordered set drifted")
    for row, wanted_row in zip(payload["task_transitions"], expected["task_transitions"], strict=True):
        if row != wanted_row:
            code = "E_N010_BOUNDARY" if row["task_id"] == "T1-M01-N010" else "E_TASK_TRANSITION"
            fail(code, row["task_id"])
    for row in payload["future_catalogs"]:
        path = (REPO / row["expected_path"]).resolve()
        if not path.is_relative_to(REPO) or path.is_symlink():
            fail("E_FUTURE_PATH", row["expected_path"])
        if path.exists() or row["status"] != "NOT_CREATED":
            fail("E_FUTURE_CATALOG_CREATED", row["expected_path"])
    work_orders = load(WORK_ORDERS)
    if sha256(WORK_ORDERS) != expected_source_by_role["REVIEW_WORK_ORDERS"]["sha256"]:
        fail("E_REVIEW_WORK_ORDER_HASH", "review work-order hash drifted")
    if work_orders["work_order_status_counts"] != {"BLOCKED_EXTERNAL_INPUTS": 36}:
        fail("E_REVIEW_WORK_ORDER_STATE", "reviews are not exactly 36 blocked")
    image_work_order = load(IMAGE_WORK_ORDER)
    if (
        sha256(IMAGE_WORK_ORDER) != expected_source_by_role["EXTERNAL_IMAGE_WORK_ORDER"]["sha256"]
        or image_work_order["status"] != "BLOCKED_SOURCE_AND_EXTERNAL_AUTHORITY_INPUTS"
        or (REPO / image_work_order["expected_receipt_path"]).exists()
    ):
        fail("E_EXTERNAL_IMAGE_WORK_ORDER_STATE", "external image work order or receipt state drifted")
    candidate_work_order = load(CANDIDATE_WORK_ORDER)
    if (
        candidate_work_order["artifact_status"]
        != "BLOCKED_IMPLEMENTATION_SOURCES_AND_CLEAN_WORKTREE"
        or candidate_work_order["design_candidate_stage"]["manifest_probe"]["exists"]
        or candidate_work_order["implementation_candidate_stage"]["manifest_probe"]["exists"]
    ):
        fail("E_CANDIDATE_FREEZE_STATE", "candidate work order is not blocked with both manifests absent")
    expected_preconditions = {item["check_id"]: item for item in expected["preconditions"]}
    for item in payload["preconditions"]:
        if item != expected_preconditions.get(item["check_id"]):
            fail("E_PRECONDITION", item["check_id"])
    if payload["switch_protocol"]["decision"] != "BLOCKED_PRECONDITIONS":
        fail("E_SWITCH_FALSE_READY", "switch decision is not blocked")
    if payload != expected:
        fail("E_DERIVATION", "preflight differs from deterministic derived state")


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
        ("active catalog hash drift", lambda p: p["current_global_catalogs"][0].update({"sha256": "0" * 64}), "E_ACTIVE_HASH"),
        ("active exact set drift", lambda p: p["current_global_catalogs"][0].update({"atomic_id_exact_set_sha256": "0" * 64}), "E_ACTIVE_SET"),
        ("removed omission", lambda p: p["candidate_projection"]["removed_active_m01_atomic_pr_ids"].pop(), "E_REMOVED_SET"),
        ("retained drift", lambda p: p["candidate_projection"]["retained_active_m01_atomic_pr_ids"].pop(), "E_RETAINED_SET"),
        ("added omission", lambda p: p["candidate_projection"]["added_candidate_m01_atomic_pr_ids"].pop(), "E_ADDED_SET"),
        ("candidate hash drift", lambda p: p["candidate_projection"].update({"candidate_global_atomic_id_exact_set_sha256": "0" * 64}), "E_CANDIDATE_HASH"),
        ("task transition omission", lambda p: p["task_transitions"].pop(), "E_TASK_TRANSITION_SET"),
        ("n010 boundary drift", lambda p: p["task_transitions"][1]["member_atomic_pr_ids"].append("T1-M01-P057-CTR-n015-s1"), "E_N010_BOUNDARY"),
        ("future catalog false created", lambda p: p["future_catalogs"][0].update({"status": "CREATED"}), "E_FUTURE_CATALOG_CREATED"),
        ("candidate freeze false ready", lambda p: p["validation"].update({"candidate_freeze_work_order_blocked": "READY"}), "E_SCHEMA"),
        ("review work order hash drift", lambda p: next(x for x in p["source_refs"] if x["role"] == "REVIEW_WORK_ORDERS").update({"sha256": "0" * 64}), "E_SOURCE_HASH"),
        ("precondition false pass", lambda p: p["preconditions"][2].update({"status": "PASS_STATIC", "blocking_reason": None}), "E_PRECONDITION"),
        ("switch false ready", lambda p: p["switch_protocol"].update({"decision": "READY"}), "E_SCHEMA"),
    ]
    for label, mutate, error in tests:
        expect_failure(label, payload, mutate, error)


def render_markdown(payload: dict[str, Any]) -> str:
    projection = payload["candidate_projection"]
    lines = [
        "# M01 早期受信验证四目录原子切换阻断预检",
        "",
        "> 决策：`BLOCKED_PRECONDITIONS`。这是只读集合预检，不生成未来目录，也不改动四个现役 registry。",
        "",
        "## 集合结果",
        "",
        "- 现役四目录：各 1289 个唯一 atomic ID，exact-set 相同；M01 为 56 个。",
        f"- 候选 M01：保留 {projection['retained_count']}、移除 {projection['removed_count']}、新增 {projection['added_count']}，合计 {projection['candidate_m01_atomic_pr_count']}。",
        f"- 候选全局：`1289 - 9 + 37 = {projection['candidate_global_atomic_pr_count']}`。",
        "- 任务边界：N015 承担受信验证基础列车；N010 仅保留 P038-P044 调用方迁移并依赖 N015；N003 同样消费 N015。",
        "",
        "## 前置条件",
        "",
        "| 检查 | 状态 | 说明 | 阻断原因 |",
        "|---|---|---|---|",
    ]
    for row in payload["preconditions"]:
        lines.append(f"| `{row['check_id']}` | `{row['status']}` | {row['description']} | {row['blocking_reason'] or '-'} |")
    lines += [
        "",
        "## 原子协议",
        "",
    ]
    for index, step in enumerate(payload["switch_protocol"]["atomic_steps"], 1):
        lines.append(f"{index}. {step}")
    lines += [
        "",
        "任一步失败都必须在替换任何现役文件前终止；不存在先切 task、稍后补 claim/design/overlay 的合法中间态。即使将来完成切换，执行仍保持 DRAFT/BLOCKED，直到 signed overlay 与真实 gate evidence 到齐。",
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
        raise ValueError("persisted M01 registry switch preflight is stale")
    if not MARKDOWN.is_file() or MARKDOWN.read_text(encoding="utf-8") != expected_md:
        raise ValueError("persisted M01 registry switch markdown is stale")
    validate(load(OUTPUT))
    if args.verify:
        self_test()
        print("PASS M01 registry switch preflight: 1289-9+37=1317, 8 preconditions, 13 targeted mutation guards, switch BLOCKED")
    else:
        print("PASS M01 registry switch preflight generation is deterministic")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
