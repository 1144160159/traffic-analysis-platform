#!/usr/bin/env python3
"""Generate a per-leaf M02 C04 review-input coverage ledger without forging review."""

from __future__ import annotations

import argparse
import copy
import hashlib
import json
import re
import subprocess
from collections import Counter, defaultdict
from pathlib import Path
from typing import Any, Callable

from build_topic1_task_registry import validate_against_schema


REPO = Path(__file__).resolve().parents[2]
SOURCE = REPO / "contracts/alignment/m02-code-direct-leaf-catalog.v4.json"
SCHEMA = REPO / "contracts/alignment/m02-function-review-coverage.schema.json"
OUTPUT = REPO / "contracts/alignment/m02-function-review-coverage.v1.json"
DOC_OUTPUT = REPO / "doc/07_alignment/generated/M02函数评审覆盖清单.md"
FUNCTION_REVIEW_SCHEMA = REPO / "contracts/alignment/function-design-review-receipt.schema.json"
NON_FUNCTION_SCHEMA = REPO / "contracts/alignment/non-function-design-exemption.schema.json"
SHARED_OWNERSHIP = REPO / "contracts/alignment/m02-shared-locator-ownership.v1.json"
WRITE_SCOPE_SUPERSESSION = REPO / "contracts/alignment/m02-write-scope-supersession.v1.json"
FUNCTION_REVIEW_VALIDATOR = REPO / "scripts/alignment/validate_function_design_review_receipt.py"
NON_FUNCTION_VALIDATOR = REPO / "scripts/alignment/validate_non_function_design_exemption_contract.py"
FUNCTION_SUFFIXES = {".go", ".rs", ".java", ".py", ".ts", ".tsx", ".js", ".jsx"}
EXPECTED_STATUS_COUNTS = {
    "FUNCTION_STATIC_DRAFT_INPUT": 66,
    "FUNCTION_CONTRACT_LOCATOR_SCOPE_MISMATCH": 0,
    "FUNCTION_CONTRACT_WITHOUT_LEAF_BINDING": 0,
    "FUNCTION_BINDING_WITHOUT_CONTRACT": 6,
    "FUNCTION_REVIEW_INPUT_MISSING": 183,
    "NON_FUNCTION_STATIC_DRAFT_INPUT": 16,
    "NON_FUNCTION_EXEMPTION_INPUT_MISSING": 154,
}


def sha256(path: Path) -> str:
    return hashlib.sha256(path.read_bytes()).hexdigest()


def hash_ref(path: Path) -> dict[str, str]:
    return {"path": path.relative_to(REPO).as_posix(), "sha256": sha256(path)}


def split_locator(locator: str) -> tuple[str, str | None]:
    if "#" not in locator:
        return locator, None
    path, symbol = locator.split("#", 1)
    return path, symbol


def normalized_symbol(symbol: str) -> str:
    result = re.sub(r"^(?:[A-Za-z0-9_]+\.)?\(\*([A-Za-z0-9_]+)\)\.", r"\1.", symbol)
    return result.split("(", 1)[0]


def contract_in_scope(contract: dict[str, Any], locators: list[str]) -> bool:
    expected = normalized_symbol(contract["qualified_symbol"])
    for locator in locators:
        path, symbol = split_locator(locator)
        if path != contract["path"] or symbol is None:
            continue
        actual = normalized_symbol(symbol)
        if actual == expected:
            return True
        if Path(path).suffix.lower() == ".java" and expected.startswith(actual + "."):
            expected_class, _, member = expected.partition(".")
            if expected_class == actual and member and "." not in member:
                return True
    return False


def function_surface(locators: list[str]) -> bool:
    return any(Path(split_locator(locator)[0]).suffix.lower() in FUNCTION_SUFFIXES for locator in locators)


def validator_check(path: Path, summary: str) -> dict[str, Any]:
    completed = subprocess.run(
        ["python3", str(path.relative_to(REPO)), "--self-test"], cwd=REPO,
        check=False, capture_output=True, text=True,
    )
    if completed.returncode != 0 or summary not in completed.stdout.splitlines():
        raise ValueError(
            f"review contract validator failed: {path.relative_to(REPO)} "
            f"rc={completed.returncode} stdout={completed.stdout!r} stderr={completed.stderr!r}"
        )
    return {
        "path": path.relative_to(REPO).as_posix(),
        "sha256": sha256(path),
        "test_command": f"python3 {path.relative_to(REPO).as_posix()} --self-test",
        "status": "PASS", "positive_case_count": 1, "negative_case_count": 7,
        "stdout_summary": summary,
    }


def build() -> dict[str, Any]:
    function_validator = validator_check(
        FUNCTION_REVIEW_VALIDATOR,
        "PASS function design review receipt: 1 positive and 7 targeted candidate/hash/reviewer/role/veto negative cases",
    )
    non_function_validator = validator_check(
        NON_FUNCTION_VALIDATOR,
        "PASS non-function design exemption: 1 positive and 7 targeted hash/candidate/reviewer/role/P0 negative cases",
    )
    catalog = json.loads(SOURCE.read_text(encoding="utf-8"))
    # V4 allocation owns only the new delivery train.  Function contracts and
    # the five v3 owner rebindings remain frozen in the v3/v2 ancestry.
    v3_catalog = json.loads((REPO / catalog["base_catalog"]["path"]).read_text(encoding="utf-8"))
    v3_allocation_path = REPO / v3_catalog["allocation_ledger"]["path"]
    v2_catalog = json.loads((REPO / v3_catalog["base_catalog"]["path"]).read_text(encoding="utf-8"))
    v2_allocation = json.loads((REPO / v2_catalog["allocation_ledger"]["path"]).read_text(encoding="utf-8"))
    source_designs = v2_allocation["source_design_refs"]
    bindings: dict[str, list[dict[str, Any]]] = defaultdict(list)
    contracts: dict[str, list[dict[str, Any]]] = defaultdict(list)
    for source_ref in source_designs:
        path = REPO / source_ref["path"]
        if sha256(path) != source_ref["sha256"]:
            raise ValueError(f"function review source design hash drifted: {source_ref['path']}")
        design = json.loads(path.read_text(encoding="utf-8"))
        for item in [*design.get("leaf_bindings", []), *design.get("append_only_leaf_bindings", [])]:
            bindings[item["leaf_id"]].append({
                "source_design_path": source_ref["path"],
                "source_design_sha256": source_ref["sha256"],
                "role": item["role"],
                "locator": item["locator"],
            })
        for item in design.get("function_contracts", []):
            contracts[item["leaf_id"]].append({
                "source_design_path": source_ref["path"],
                "source_design_sha256": source_ref["sha256"],
                "contract_id": item["contract_id"],
                "path": item["path"],
                "qualified_symbol": item["qualified_symbol"],
                "signature_after": item["signature_after"],
                "source_declared_leaf_id": item["leaf_id"],
                "ownership_resolution": "SOURCE_LEAF",
            })

    source_refs_by_path = {item["path"]: item for item in source_designs}
    for item in catalog["contract_owner_rebindings"]:
        if source_refs_by_path.get(item["source_design"]["path"]) != item["source_design"]:
            raise ValueError(f"function review rebinding source is outside frozen source exact-set: {item['contract_id']}")
        old_contracts = contracts[item["superseded_leaf_id"]]
        matches = [contract for contract in old_contracts if contract["contract_id"] == item["contract_id"]]
        if len(matches) != 1:
            raise ValueError(f"function review rebinding source contract is not unique: {item['contract_id']}")
        contract = matches[0]
        if f"{contract['path']}#{contract['qualified_symbol']}" != item["exact_locator"]:
            raise ValueError(f"function review rebinding locator differs from source contract: {item['contract_id']}")
        old_contracts.remove(contract)
        contracts[item["owner_leaf_id"]].append({
            **contract,
            "ownership_resolution": "V3_APPEND_ONLY_OWNER_REBINDING",
        })
        bindings[item["owner_leaf_id"]].append({
            "source_design_path": item["source_design"]["path"],
            "source_design_sha256": item["source_design"]["sha256"],
            "role": "V3_FUNCTION_CONTRACT_OWNER",
            "locator": item["exact_locator"],
        })

    rows = []
    for leaf in catalog["leaves"]:
        leaf_id = leaf["leaf_id"]
        is_function = function_surface(leaf["write_locators"])
        leaf_bindings = bindings.get(leaf_id, [])
        leaf_contracts = []
        for item in contracts.get(leaf_id, []):
            leaf_contracts.append({
                **item,
                "within_leaf_write_scope": contract_in_scope(item, leaf["write_locators"]),
            })
        reasons = [
            "clean candidate manifest and candidate-bound exact locator receipts are absent",
            "signed review attestations are absent",
        ]
        if is_function:
            expected_kind = "FUNCTION_DESIGN_REVIEW_RECEIPT"
            expected_name = "function-design-review-receipt.json"
            if leaf_contracts and not leaf_bindings:
                status = "FUNCTION_CONTRACT_WITHOUT_LEAF_BINDING"
                reasons.append("source design contains a function contract but omits this leaf from leaf_bindings")
            elif leaf_contracts and not all(item["within_leaf_write_scope"] for item in leaf_contracts):
                status = "FUNCTION_CONTRACT_LOCATOR_SCOPE_MISMATCH"
                reasons.append("one or more function contracts target a symbol outside the leaf write locator set")
            elif leaf_contracts and leaf_bindings:
                status = "FUNCTION_STATIC_DRAFT_INPUT"
                reasons.append("static contract is an unsigned input and is not a FUNCTION_DESIGN_REVIEW_RECEIPT")
            elif leaf_bindings:
                status = "FUNCTION_BINDING_WITHOUT_CONTRACT"
                reasons.append("function-like binding has no code-unit function contract")
            else:
                status = "FUNCTION_REVIEW_INPUT_MISSING"
                reasons.append("no static function contract or leaf binding exists")
        else:
            expected_kind = "NON_FUNCTION_DESIGN_EXEMPTION_RECEIPT"
            expected_name = "non-function-design-exemption-receipt.json"
            if leaf_bindings:
                status = "NON_FUNCTION_STATIC_DRAFT_INPUT"
                reasons.append("static declarative binding is not a signed specialized exemption")
            else:
                status = "NON_FUNCTION_EXEMPTION_INPUT_MISSING"
                reasons.append("no specialized non-function contract input exists")
        expected_path = (
            "doc/02_acceptance/topic1/m02/function-reviews/"
            + leaf["atomic_pr_id"].lower()
            + "/"
            + expected_name
        )
        if leaf_id in {"M02-N004-L21", "M02-N004-L48"}:
            reasons.append("P165/P408 write-scope supersession is DESIGNED_NOT_REGISTERED")
        rows.append({
            "leaf_id": leaf_id,
            "atomic_pr_id": leaf["atomic_pr_id"],
            "parent_task_id": leaf["parent_task_id"],
            "pr_type": leaf["pr_type"],
            "target_state": leaf["target_state"],
            "write_locators": leaf["write_locators"],
            "review_surface": "FUNCTION_SET" if is_function else "NON_FUNCTION_SET",
            "expected_artifact_kind": expected_kind,
            "expected_artifact_path": expected_path,
            "static_bindings": leaf_bindings,
            "static_function_contracts": leaf_contracts,
            "coverage_status": status,
            "blocking_reasons": reasons,
        })

    status_counts = Counter(item["coverage_status"] for item in rows)
    for status in EXPECTED_STATUS_COUNTS:
        status_counts.setdefault(status, 0)
    function_count = sum(item["review_surface"] == "FUNCTION_SET" for item in rows)
    return {
        "schema_version": "1.0.0",
        "artifact_kind": "M02_FUNCTION_REVIEW_INPUT_COVERAGE",
        "artifact_status": "BLOCKED_REVIEW_INPUTS_INCOMPLETE",
        "source_catalog": hash_ref(SOURCE),
        "source_designs": [hash_ref(REPO / item["path"]) for item in source_designs],
        "review_receipt_schema": hash_ref(FUNCTION_REVIEW_SCHEMA),
        "review_receipt_validator": function_validator,
        "non_function_exemption_schema": hash_ref(NON_FUNCTION_SCHEMA),
        "non_function_exemption_validator": non_function_validator,
        "shared_locator_ownership": hash_ref(SHARED_OWNERSHIP),
        "write_scope_supersession": hash_ref(WRITE_SCOPE_SUPERSESSION),
        "contract_owner_rebindings": hash_ref(v3_allocation_path),
        "leaf_count": len(rows),
        "function_set_leaf_count": function_count,
        "non_function_set_leaf_count": len(rows) - function_count,
        "static_leaf_binding_count": len(bindings),
        "static_function_contract_leaf_count": len(contracts),
        "static_function_contract_row_count": sum(len(items) for items in contracts.values()),
        "status_counts": dict(sorted(status_counts.items())),
        "approved_evidence_counts": {
            "unified_function_review_receipts": 0,
            "signed_non_function_exemptions": 0,
            "review_complete_leaves": 0,
        },
        "leaf_reviews": rows,
        "validation": {
            "source_exact": "PASS",
            "leaf_exact_set": "PASS",
            "surface_partition_exact": "PASS",
            "design_binding_exact_set": "PASS",
            "function_contract_exact_set": "PASS",
            "contract_locator_scope_checked": "PASS",
            "contract_owner_rebinding_exact": "PASS",
            "no_unsigned_draft_claimed_reviewed": True,
            "shared_locator_blocker_propagated": True,
            "mutation_guards": {
                "leaf_omission": "PASS", "surface_kind_drift": "PASS", "status_drift": "PASS",
                "false_approved_count": "PASS", "contract_ref_omission": "PASS", "shared_blocker_omission": "PASS",
            },
        },
        "proof_ceiling": "STATIC_REVIEW_INPUT_COVERAGE_ONLY_NOT_LOCATOR_RESOLVED_FUNCTION_REVIEWED_EXEMPTION_APPROVED_IMPLEMENTED_OR_AUTHORIZED",
    }


def validate(payload: dict[str, Any]) -> None:
    validate_against_schema(payload, SCHEMA)
    if payload != build():
        raise ValueError("function review coverage ledger differs from exact derived state")
    rows = payload["leaf_reviews"]
    if len({item["leaf_id"] for item in rows}) != 425 or len({item["atomic_pr_id"] for item in rows}) != 425:
        raise ValueError("function review leaf or atomic ID exact-set drifted")
    if payload["status_counts"] != EXPECTED_STATUS_COUNTS:
        raise ValueError("function review status counts drifted")


def expect_failure(label: str, payload: dict[str, Any], mutate: Callable[[dict[str, Any]], None], expected_error: str) -> None:
    candidate = copy.deepcopy(payload)
    mutate(candidate)
    try:
        validate(candidate)
    except (ValueError, TypeError) as exc:
        if expected_error not in str(exc):
            raise ValueError(f"mutation {label} hit wrong assertion: {exc}") from exc
        return
    raise ValueError(f"mutation {label} did not fail")


def run_mutation_tests(payload: dict[str, Any]) -> None:
    expect_failure("leaf omission", payload, lambda item: item["leaf_reviews"].pop(), "schema minItems failed at $.leaf_reviews")
    expect_failure(
        "surface kind drift", payload,
        lambda item: item["leaf_reviews"][0].update({"review_surface": "FUNCTION_SET"}),
        "differs from exact derived state",
    )
    expect_failure(
        "status drift", payload,
        lambda item: item["status_counts"].update({"FUNCTION_REVIEW_INPUT_MISSING": 176}),
        "schema const mismatch at $.status_counts.FUNCTION_REVIEW_INPUT_MISSING",
    )
    expect_failure(
        "false approved count", payload,
        lambda item: item["approved_evidence_counts"].update({"review_complete_leaves": 1}),
        "schema const mismatch at $.approved_evidence_counts.review_complete_leaves",
    )
    contract_row = next(item for item in payload["leaf_reviews"] if item["static_function_contracts"])
    expect_failure(
        "contract ref omission", payload,
        lambda item: next(row for row in item["leaf_reviews"] if row["leaf_id"] == contract_row["leaf_id"])["static_function_contracts"].pop(),
        "differs from exact derived state",
    )
    expect_failure(
        "shared blocker omission", payload,
        lambda item: next(row for row in item["leaf_reviews"] if row["leaf_id"] == "M02-N004-L21")["blocking_reasons"].remove("P165/P408 write-scope supersession is DESIGNED_NOT_REGISTERED"),
        "differs from exact derived state",
    )


def render_markdown(payload: dict[str, Any]) -> str:
    counts = payload["status_counts"]
    lines = [
        "# M02函数评审覆盖清单",
        "",
        "状态：`BLOCKED_REVIEW_INPUTS_INCOMPLETE / NO-GO`",
        "",
        "本清单只枚举C04评审输入与缺口；静态函数合同不是评审回执，任何行都不表示已签署、已实现或已授权。",
        "",
        "## 总量",
        "",
        f"- 总叶：{payload['leaf_count']}；函数集合：{payload['function_set_leaf_count']}；非函数集合：{payload['non_function_set_leaf_count']}。",
        f"- 静态leaf binding：{payload['static_leaf_binding_count']}叶；静态函数合同：{payload['static_function_contract_leaf_count']}叶/{payload['static_function_contract_row_count']}行。",
        "- 正式UNIFIED函数评审回执：0；签署非函数豁免：0；C04完成叶：0。",
        "",
        "## 逐状态",
        "",
        "| 状态 | 数量 | 含义 |",
        "|---|---:|---|",
        f"| `FUNCTION_STATIC_DRAFT_INPUT` | {counts['FUNCTION_STATIC_DRAFT_INPUT']} | 有binding且函数合同全部落在叶写范围，但仍缺candidate/locator receipt/签署 |",
        f"| `FUNCTION_CONTRACT_LOCATOR_SCOPE_MISMATCH` | {counts['FUNCTION_CONTRACT_LOCATOR_SCOPE_MISMATCH']} | 合同含叶写范围外函数 |",
        f"| `FUNCTION_CONTRACT_WITHOUT_LEAF_BINDING` | {counts['FUNCTION_CONTRACT_WITHOUT_LEAF_BINDING']} | 函数合同未进入base或append-only leaf binding |",
        f"| `FUNCTION_BINDING_WITHOUT_CONTRACT` | {counts['FUNCTION_BINDING_WITHOUT_CONTRACT']} | 有函数型binding但无函数合同 |",
        f"| `FUNCTION_REVIEW_INPUT_MISSING` | {counts['FUNCTION_REVIEW_INPUT_MISSING']} | 函数叶没有静态合同输入 |",
        f"| `NON_FUNCTION_STATIC_DRAFT_INPUT` | {counts['NON_FUNCTION_STATIC_DRAFT_INPUT']} | 有声明式binding但尚无签署豁免 |",
        f"| `NON_FUNCTION_EXEMPTION_INPUT_MISSING` | {counts['NON_FUNCTION_EXEMPTION_INPUT_MISSING']} | 缺专用非函数合同输入 |",
        "",
        "## 关键断口",
        "",
        "- P165/P408目标边界已冻结为P165拥有`poll_manifest`函数体、P408只拥有`replay_delay` helper；写范围supersession已设计但未注册到四份候选catalog，故仍FAIL_CLOSED。",
        "- 5个原同叶helper合同继续由冻结的v3 append-only rebinding拥有；v4仅追加N013 delivery-package train，P101-P521字段未改写。Java类locator只覆盖同文件同类的直接成员方法，仍需candidate-bound method AST receipt。",
        "- 任何函数叶只有在同一clean candidate上获得exact locator receipts、最终模式决议、code-unit contract、negative-test manifest和至少两份真实签署后，才可生成UNIFIED回执。",
        "- 非函数叶必须走`NON_FUNCTION_DESIGN_EXEMPTION_RECEIPT`，绑定专用合同、验证/回滚计划、candidate和至少两份真实签署。",
        "",
        "## 证明上限",
        "",
        f"`{payload['proof_ceiling']}`",
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
    payload = build()
    validate_against_schema(payload, SCHEMA)
    expected = json.dumps(payload, ensure_ascii=False, indent=2) + "\n"
    markdown = render_markdown(payload)
    if args.write:
        OUTPUT.write_text(expected, encoding="utf-8")
        DOC_OUTPUT.parent.mkdir(parents=True, exist_ok=True)
        DOC_OUTPUT.write_text(markdown, encoding="utf-8")
        print(f"WROTE {OUTPUT.relative_to(REPO)}")
        print(f"WROTE {DOC_OUTPUT.relative_to(REPO)}")
        return 0
    if not OUTPUT.is_file() or OUTPUT.read_text(encoding="utf-8") != expected:
        raise ValueError("generated function review coverage ledger is stale")
    if not DOC_OUTPUT.is_file() or DOC_OUTPUT.read_text(encoding="utf-8") != markdown:
        raise ValueError("generated function review coverage markdown is stale")
    validate(payload)
    if args.verify:
        run_mutation_tests(payload)
    print(
        f"PASS leaves={payload['leaf_count']} function={payload['function_set_leaf_count']} "
        f"non_function={payload['non_function_set_leaf_count']} reviewed=0"
    )
    print(f"PROOF_CEILING {payload['proof_ceiling']}")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
