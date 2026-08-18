#!/usr/bin/env python3
"""Generate the blocked M01 verifier image external-activity work order."""

from __future__ import annotations

import argparse
import copy
import hashlib
import json
from pathlib import Path
from typing import Any, Callable

from build_topic1_task_registry import validate_against_schema


REPO = Path(__file__).resolve().parents[2]
ALLOCATION = REPO / "contracts/alignment/m01-early-trust-train-allocation.v2.json"
CATALOG = REPO / "contracts/alignment/m01-early-trust-train-catalog.v2.json"
DESIGN = REPO / "contracts/alignment/m01-early-trust-function-design.v1.json"
RECEIPT_SCHEMA = REPO / "contracts/alignment/m01-verifier-image-build-sign-receipt.schema.json"
RECEIPT_VALIDATOR = REPO / "scripts/alignment/validate_m01_verifier_image_build_sign_receipt.py"
CANDIDATE_WORK_ORDER = REPO / "contracts/alignment/m01-early-trust-candidate-freeze-work-order.v1.json"
SCHEMA = REPO / "contracts/alignment/m01-verifier-image-build-sign-work-order.schema.json"
OUTPUT = REPO / "contracts/alignment/m01-verifier-image-build-sign-work-order.v1.json"
MARKDOWN = REPO / "doc/07_alignment/generated/M01受信验证器镜像构建签名发布工作单.md"
ACTIVITY_ID = "EXT-T1-M01-N015-VERIFIER-IMAGE-BUILD-AND-SIGN"
RECEIPT = "doc/02_acceptance/topic1/m01/external-activities/verifier-image-build-sign-publish/receipt.json"
CANDIDATE = "doc/02_acceptance/topic1/m01/candidates/m01-early-trust-v2/design-candidate-manifest.json"
SOURCE_PATHS = {
    "V2_ALLOCATION": ALLOCATION,
    "V2_CATALOG": CATALOG,
    "FUNCTION_DESIGN": DESIGN,
    "RECEIPT_SCHEMA": RECEIPT_SCHEMA,
    "RECEIPT_VALIDATOR": RECEIPT_VALIDATOR,
    "CANDIDATE_FREEZE_WORK_ORDER": CANDIDATE_WORK_ORDER,
}
SOURCE_INPUTS = [
    ("VERIFIER_IMAGE_BUILD_RECIPE", "T1-M01-P079-OPS-n015-s22", "deployments/security/topic1-trusted-signature-verifier.Dockerfile"),
    ("VERIFIER_REQUIREMENTS_LOCK", "T1-M01-P079-OPS-n015-s22", "deployments/security/topic1-trusted-signature-verifier.requirements.lock"),
    ("VERIFIER_SERVICE_SOURCE", "T1-M01-P094-REF-n015-s37", "scripts/alignment/trusted_signature_service.py"),
    ("TRUST_POLICY_SCHEMA", "T1-M01-P057-CTR-n015-s1", "contracts/alignment/signature-trust-policy.schema.json"),
    ("VERIFICATION_REQUEST_SCHEMA", "T1-M01-P057-CTR-n015-s1", "contracts/alignment/signature-verification-request.schema.json"),
    ("VERIFICATION_ATTESTATION_SCHEMA", "T1-M01-P057-CTR-n015-s1", "contracts/alignment/signature-verification-attestation.schema.json"),
]
EXTERNAL_INPUTS = [
    ("BUILD_ENVIRONMENT_MANIFEST", "build-environment-manifest.json"),
    ("IMAGE_MANIFEST", "image-manifest.json"),
    ("CYCLONEDX_SBOM", "verifier.cdx.json"),
    ("SLSA_PROVENANCE_BUNDLE", "slsa-provenance.bundle"),
    ("IMAGE_SIGNATURE_BUNDLE", "image-signature.bundle"),
    ("SIGNATURE_VERIFICATION_RECEIPT", "signature-verification.json"),
    ("SIGNED_RECEIPT_PAYLOAD", "signed-receipt-payload.json"),
]
BASE = "doc/02_acceptance/topic1/m01/external-activities/verifier-image-build-sign-publish"


def load(path: Path) -> dict[str, Any]:
    return json.loads(path.read_text(encoding="utf-8"))


def sha256(path: Path) -> str:
    return hashlib.sha256(path.read_bytes()).hexdigest()


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


def build() -> dict[str, Any]:
    allocation = load(ALLOCATION)
    activity = allocation["external_activities"][0]
    candidate_work_order = load(CANDIDATE_WORK_ORDER)
    candidate = probe(CANDIDATE)
    sources = [
        {
            "artifact_id": artifact_id,
            "owner_atomic_pr_id": owner,
            "path": path,
            "exists": safe_path(path).is_file(),
            "status": "PRESENT_UNVALIDATED" if safe_path(path).is_file() else "MISSING",
        }
        for artifact_id, owner, path in SOURCE_INPUTS
    ]
    external = [
        {
            "artifact_id": artifact_id,
            "expected_path": f"{BASE}/{name}",
            "status": "PRESENT_UNVALIDATED" if safe_path(f"{BASE}/{name}").is_file() else "MISSING",
        }
        for artifact_id, name in EXTERNAL_INPUTS
    ]
    return {
        "schema_version": "1.0.0",
        "artifact_kind": "M01_VERIFIER_IMAGE_BUILD_SIGN_AND_PUBLISH_WORK_ORDER",
        "artifact_status": "BLOCKED_SOURCE_AND_EXTERNAL_AUTHORITY_INPUTS",
        "work_order_id": "M01-EI-VERIFIER-IMAGE-BUILD-SIGN-PUBLISH",
        "activity_id": ACTIVITY_ID,
        "source_refs": [
            {"role": role, "path": path.relative_to(REPO).as_posix(), "sha256": sha256(path)}
            for role, path in SOURCE_PATHS.items()
        ],
        "depends_on_atomic_pr_ids": activity["depends_on_atomic_pr_ids"],
        "candidate_manifest": candidate,
        "required_source_inputs": sources,
        "required_external_inputs": external,
        "owner_slots": [
            {"role": "SUPPLY_CHAIN_OWNER", "assignment_status": "UNASSIGNED", "identity": None},
            {"role": "SECURITY_OWNER", "assignment_status": "UNASSIGNED", "identity": None},
        ],
        "required_outputs": activity["required_outputs"],
        "expected_receipt_path": RECEIPT,
        "command_plan": [
            {
                "step_id": "validate-design-candidate-freeze",
                "run_when": "before handing this work order to an authorized external build system",
                "working_directory": ".",
                "argv": ["python3", "scripts/alignment/generate_m01_early_trust_candidate_freeze_work_order.py", "--verify"],
                "expected_effect": "validate the two-stage candidate boundary and require an immutable clean design candidate before external build",
            },
            {
                "step_id": "validate-v2-activity-binding",
                "run_when": "before handing this work order to an authorized external build system",
                "working_directory": ".",
                "argv": ["python3", "scripts/alignment/generate_m01_early_trust_train_v2.py", "--verify"],
                "expected_effect": "validate the exact P079 dependency output identities and pending external activity declaration",
            },
            {
                "step_id": "validate-static-source-contracts",
                "run_when": "after P057 P079 P094 source implementations and their reviews exist on one candidate",
                "working_directory": ".",
                "argv": ["python3", "scripts/alignment/validate_m01_early_trust_function_design.py", "--check-generated"],
                "expected_effect": "validate static function type deployment package and rollback contracts without claiming implementation",
            },
            {
                "step_id": "validate-receipt-contract",
                "run_when": "before external execution and after any receipt contract revision",
                "working_directory": ".",
                "argv": ["python3", "scripts/alignment/validate_m01_verifier_image_build_sign_receipt.py", "--self-test"],
                "expected_effect": "validate receipt structure hash closure bootstrap quorum and targeted rejection cases",
            },
            {
                "step_id": "validate-real-receipt",
                "run_when": "only after protected external CI builds signs publishes verifies and two real owners sign the immutable receipt",
                "working_directory": ".",
                "argv": ["python3", "scripts/alignment/validate_m01_verifier_image_build_sign_receipt.py", "--check", RECEIPT],
                "expected_effect": "validate the authored receipt candidate image SBOM provenance signature and 2-of-2 hash closure without asserting cryptographic trust",
            },
        ],
        "status": "BLOCKED_SOURCE_AND_EXTERNAL_AUTHORITY_INPUTS",
        "blocking_reasons": [
            f"candidate manifest is {candidate['status']}",
            f"candidate freeze work order is {candidate_work_order['artifact_status']}",
            f"required candidate source inputs present {sum(item['exists'] for item in sources)}/6",
            f"required external build outputs present {sum(item['status'] != 'MISSING' for item in external)}/7",
            "P079 package P094 service and P057 trust schemas are planned but not implemented",
            "supply-chain owner and security owner identities are unassigned",
            "protected external CI build publish and independent signature verification have not run",
            "2-of-2 bootstrap signatures and immutable receipt are absent",
            "M01 v2 remains outside the four active registries",
        ],
        "validation": {
            "schema": "PASS",
            "source_hashes_exact": "PASS",
            "activity_binding_exact": "PASS",
            "candidate_freeze_work_order_blocked": "PASS_NO_CANDIDATE",
            "source_input_exact_set": "PASS_6",
            "external_input_exact_set": "PASS_7",
            "owner_role_exact_set": "PASS_2_OF_2",
            "all_owner_identities_unassigned": True,
            "receipt_absent": True,
            "no_build_push_signature_or_receipt_created": True,
            "mutation_guards": {
                "source_hash_drift": "PASS",
                "candidate_freeze_false_ready": "PASS",
                "dependency_drift": "PASS",
                "source_input_omission": "PASS",
                "external_input_omission": "PASS",
                "owner_role_drift": "PASS",
                "owner_identity_fabrication": "PASS",
                "receipt_false_present": "PASS",
                "command_drift": "PASS",
                "false_ready": "PASS",
            },
        },
        "allowed_claim": "external activity inputs roles outputs receipt path and validation commands are derived",
        "forbidden_claim": "source implementation candidate build image publish signer identity signature receipt deployment execution registry switch authorization or acceptance exists",
        "proof_ceiling": "BLOCKED_EXTERNAL_IMAGE_ACTIVITY_WORK_ORDER_ONLY",
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
    if payload["depends_on_atomic_pr_ids"] != ["T1-M01-P079-OPS-n015-s22"]:
        fail("E_DEPENDENCY", "activity must depend exactly on P079")
    candidate_work_order = load(CANDIDATE_WORK_ORDER)
    if (
        candidate_work_order["artifact_status"]
        != "BLOCKED_IMPLEMENTATION_SOURCES_AND_CLEAN_WORKTREE"
        or candidate_work_order["design_candidate_stage"]["manifest_probe"]["exists"]
    ):
        fail("E_CANDIDATE_FREEZE_STATE", "candidate freeze work order is not blocked without a manifest")
    if payload["required_source_inputs"] != expected["required_source_inputs"]:
        fail("E_SOURCE_INPUT_SET", "source input exact-set or probe state drifted")
    if payload["required_external_inputs"] != expected["required_external_inputs"]:
        fail("E_EXTERNAL_INPUT_SET", "external input exact-set or probe state drifted")
    roles = [item["role"] for item in payload["owner_slots"]]
    if roles != ["SUPPLY_CHAIN_OWNER", "SECURITY_OWNER"]:
        fail("E_OWNER_ROLE", "2-of-2 role exact-set or order drifted")
    if any(item["assignment_status"] != "UNASSIGNED" or item["identity"] is not None for item in payload["owner_slots"]):
        fail("E_OWNER_IDENTITY_FABRICATION", "owner identity was populated without external assignment")
    receipt = safe_path(payload["expected_receipt_path"])
    if receipt.exists() or payload["validation"]["receipt_absent"] is not True:
        fail("E_RECEIPT_FALSE_PRESENT", payload["expected_receipt_path"])
    if payload["command_plan"] != expected["command_plan"]:
        fail("E_COMMAND", "command plan drifted")
    if payload["status"] != "BLOCKED_SOURCE_AND_EXTERNAL_AUTHORITY_INPUTS":
        fail("E_FALSE_READY", payload["status"])
    if payload != expected:
        fail("E_DERIVATION", "work order differs from deterministic derived state")


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
        ("candidate freeze false ready", lambda p: p["validation"].update({"candidate_freeze_work_order_blocked": "READY"}), "E_SCHEMA"),
        ("dependency drift", lambda p: p.update({"depends_on_atomic_pr_ids": ["T1-M01-P078-REF-n015-s21"]}), "E_SCHEMA"),
        ("source input omission", lambda p: p["required_source_inputs"].pop(), "schema minItems failed at $.required_source_inputs"),
        ("external input omission", lambda p: p["required_external_inputs"].pop(), "schema minItems failed at $.required_external_inputs"),
        ("owner role drift", lambda p: p["owner_slots"].reverse(), "E_OWNER_ROLE"),
        ("owner identity fabrication", lambda p: p["owner_slots"][0].update({"assignment_status": "ASSIGNED", "identity": "invented@example.invalid"}), "E_OWNER_IDENTITY_FABRICATION"),
        ("receipt false present", lambda p: p["validation"].update({"receipt_absent": False}), "E_SCHEMA"),
        ("command drift", lambda p: p["command_plan"][3]["argv"].append("--trust-without-verification"), "E_COMMAND"),
        ("false ready", lambda p: p.update({"status": "READY_FOR_AUTHORIZED_EXTERNAL_EXECUTION"}), "E_FALSE_READY"),
    ]
    for label, mutate, error in tests:
        expect_failure(label, payload, mutate, error)


def render_markdown(payload: dict[str, Any]) -> str:
    lines = [
        "# M01 受信验证器镜像构建、签名与发布工作单",
        "",
        "> 状态：`BLOCKED_SOURCE_AND_EXTERNAL_AUTHORITY_INPUTS`。本工作单不执行构建、push、签名或发布，也不创建身份与 receipt。",
        "",
        "## 活动边界",
        "",
        f"- Activity：`{payload['activity_id']}`，唯一 PR 前驱为 `T1-M01-P079-OPS-n015-s22`。",
        "- 首次 trust bootstrap 需要仓外受保护 CI/分支控制，以及供应链负责人和安全负责人 2-of-2 独立签署；不能由尚未建立信任的验证器自行授权。",
        "- 四个正式输出：`IMAGE_DIGEST`、`SBOM_SHA256`、`PROVENANCE_ATTESTATION`、`SIGNATURE_VERIFICATION_RECEIPT`。",
        "",
        "## 候选源输入",
        "",
        "| Artifact | Owner | Path | 状态 |",
        "|---|---|---|---|",
    ]
    for item in payload["required_source_inputs"]:
        lines.append(f"| `{item['artifact_id']}` | `{item['owner_atomic_pr_id']}` | `{item['path']}` | `{item['status']}` |")
    lines += ["", "## 外部输入", "", "| Artifact | Expected path | 状态 |", "|---|---|---|"]
    for item in payload["required_external_inputs"]:
        lines.append(f"| `{item['artifact_id']}` | `{item['expected_path']}` | `{item['status']}` |")
    lines += [
        "", "## 执行规则", "",
        "1. 先完成并评审 P057、P079、P094 及其依赖，冻结 clean same-commit candidate。",
        "2. 由授权的仓外受保护 CI 按候选 Git blob exact-set 构建，输出 digest、CycloneDX SBOM、SLSA provenance、signature bundle 与独立验签回执。",
        "3. 供应链负责人和安全负责人以不同身份对同一 signed payload 做 2-of-2 签署并附各自 protected verification evidence。",
        "4. 最后运行 `validate-real-receipt`；结构与哈希闭包通过仍不等于全局切换、部署、gate PASS 或执行授权。",
        "",
        f"Receipt：`{payload['expected_receipt_path']}`（当前不存在）",
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
    body = json.dumps(expected, ensure_ascii=False, indent=2) + "\n"
    markdown = render_markdown(expected)
    if args.write:
        OUTPUT.write_text(body, encoding="utf-8")
        MARKDOWN.write_text(markdown, encoding="utf-8")
        print(f"WROTE {OUTPUT.relative_to(REPO)}")
        print(f"WROTE {MARKDOWN.relative_to(REPO)}")
        return 0
    if not OUTPUT.is_file() or OUTPUT.read_text(encoding="utf-8") != body:
        raise ValueError("persisted M01 verifier image activity work order is stale")
    if not MARKDOWN.is_file() or MARKDOWN.read_text(encoding="utf-8") != markdown:
        raise ValueError("persisted M01 verifier image activity markdown is stale")
    validate(load(OUTPUT))
    if args.verify:
        self_test()
        print("PASS M01 verifier image activity work order: 6 candidate inputs, 7 external inputs, 2 unassigned owners, 10 targeted mutation guards")
    else:
        print("PASS M01 verifier image activity work-order generation is deterministic")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
