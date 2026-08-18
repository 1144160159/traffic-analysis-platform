#!/usr/bin/env python3
"""Generate the append-only effective write-scope supersession for P408."""

from __future__ import annotations

import argparse
import copy
import hashlib
import json
from pathlib import Path
from typing import Any, Callable

from build_topic1_task_registry import validate_against_schema


REPO = Path(__file__).resolve().parents[2]
CATALOG = REPO / "contracts/alignment/m02-code-direct-leaf-catalog.v2.json"
ALLOCATION = REPO / "contracts/alignment/m02-code-direct-leaf-allocation.v2.json"
OWNERSHIP = REPO / "contracts/alignment/m02-shared-locator-ownership.v1.json"
SCHEMA = REPO / "contracts/alignment/m02-write-scope-supersession.schema.json"
OUTPUT = REPO / "contracts/alignment/m02-write-scope-supersession.v1.json"
DOC_OUTPUT = REPO / "doc/07_alignment/generated/M02写范围Supersession账本.md"
HELPER = "rust/probe-agent/probe-agent/src/capture/pcap_offline.rs#replay_delay"
CONTEXT = "rust/probe-agent/probe-agent/src/capture/pcap_offline.rs#ManifestPcapReplayer::poll_manifest"


def sha256(path: Path) -> str:
    return hashlib.sha256(path.read_bytes()).hexdigest()


def semantic_sha256(value: Any) -> str:
    body = json.dumps(value, ensure_ascii=False, sort_keys=True, separators=(",", ":")).encode("utf-8")
    return hashlib.sha256(body).hexdigest()


def hash_ref(path: Path) -> dict[str, str]:
    return {"path": path.relative_to(REPO).as_posix(), "sha256": sha256(path)}


def build() -> dict[str, Any]:
    catalog = json.loads(CATALOG.read_text(encoding="utf-8"))
    allocation = json.loads(ALLOCATION.read_text(encoding="utf-8"))
    ownership = json.loads(OWNERSHIP.read_text(encoding="utf-8"))
    leaf = next(item for item in catalog["leaves"] if item["leaf_id"] == "M02-N004-L48")
    step = next(
        item for train in allocation["trains"] for item in train["steps"]
        if item["leaf_id"] == "M02-N004-L48"
    )
    frozen = [step["primary_locator"], *step["companion_locators"]]
    if leaf["write_locators"] != frozen or frozen != [HELPER, CONTEXT]:
        raise ValueError("P408 frozen write scope differs between catalog and allocation")
    edge = next((
        item for item in catalog["append_only_edges"]
        if item["from"] == "M02-N004-L48" and item["to"] == "M02-N004-L21"
    ), None)
    if edge is None or edge["edge_kind"] != "APPENDED_TO_EXISTING":
        raise ValueError("P408 to P165 integration edge drifted")
    boundary = ownership["required_single_writer_boundary"]
    if boundary["function_body_owner"] != "T1-M02-P165-WRT-n004-l21" or boundary["helper_owner"] != leaf["atomic_pr_id"]:
        raise ValueError("P165/P408 ownership boundary drifted")
    return {
        "schema_version": "1.0.0",
        "artifact_kind": "M02_EFFECTIVE_WRITE_SCOPE_SUPERSESSION_LEDGER",
        "artifact_status": "DESIGNED_NOT_REGISTERED",
        "revision_id": "M02-WRITE-SCOPE-SUPERSESSION-V1",
        "source_catalog": hash_ref(CATALOG),
        "source_allocation": hash_ref(ALLOCATION),
        "ownership_boundary": hash_ref(OWNERSHIP),
        "supersessions": [{
            "supersession_id": "M02-P408-POLL-MANIFEST-WRITE-SCOPE-V1",
            "predecessor_leaf_id": leaf["leaf_id"],
            "predecessor_atomic_pr_id": leaf["atomic_pr_id"],
            "frozen_write_locators": frozen,
            "frozen_write_locator_exact_set_sha256": semantic_sha256(sorted(frozen)),
            "effective_write_locators": [HELPER],
            "read_only_context_locators": [CONTEXT],
            "context_function_owner_leaf_id": "M02-N004-L21",
            "context_function_owner_atomic_pr_id": "T1-M02-P165-WRT-n004-l21",
            "integration_edge": {
                "from": edge["from"], "to": edge["to"], "edge_kind": edge["edge_kind"],
            },
            "change_rule": "REMOVE_COMPANION_FROM_EFFECTIVE_WRITE_SCOPE_WITHOUT_MUTATING_FROZEN_P408_RECORD",
            "activation_status": "BLOCKED_NOT_REGISTERED",
        }],
        "activation_policy": {
            "decision": "BLOCKED_NOT_REGISTERED",
            "required_evidence": [
                "ONE_CLEAN_CANDIDATE_MANIFEST",
                "P408_REPLAY_DELAY_EXACT_RUST_AST_RECEIPT",
                "P165_POLL_MANIFEST_EXACT_RUST_AST_RECEIPT",
                "P408_UNIFIED_FUNCTION_REVIEW_RECEIPT",
                "P165_UNIFIED_FUNCTION_REVIEW_RECEIPT",
                "FOUR_CANDIDATE_CATALOGS_APPLY_THE_SAME_SUPERSESSION_HASH",
            ],
            "activation_effect": "FUTURE_CANDIDATE_PACKAGES_USE_EFFECTIVE_WRITE_AND_READ_ONLY_CONTEXT_SETS_WHILE_FROZEN_HISTORY_REMAINS_BYTE_IDENTICAL",
            "failure_rule": "ANY_MISSING_OR_HASH_MISMATCH_KEEPS_P408_UNCLAIMABLE_AND_ACTIVE_REGISTRIES_UNCHANGED",
        },
        "validation": {
            "source_exact": "PASS", "frozen_predecessor_exact": "PASS",
            "scope_strictly_narrows": True, "no_new_locator_introduced": True,
            "context_owner_exact": "PASS", "integration_edge_exact": "PASS",
            "historical_artifacts_not_mutated": True, "premature_activation_rejected": True,
            "mutation_guards": {
                "scope_widening": "PASS", "context_omission": "PASS", "owner_swap": "PASS",
                "edge_reversal": "PASS", "false_activation": "PASS", "source_hash_drift": "PASS",
            },
        },
        "proof_ceiling": "WRITE_SCOPE_SUPERSESSION_DESIGN_ONLY_NOT_REGISTERED_LOCATOR_RESOLVED_FUNCTION_REVIEWED_IMPLEMENTED_OR_AUTHORIZED",
    }


def validate(payload: dict[str, Any]) -> None:
    validate_against_schema(payload, SCHEMA)
    if payload != build():
        raise ValueError("write-scope supersession ledger differs from exact derived state")
    record = payload["supersessions"][0]
    frozen = set(record["frozen_write_locators"])
    effective = set(record["effective_write_locators"])
    context = set(record["read_only_context_locators"])
    if not effective < frozen:
        raise ValueError("write-scope supersession does not strictly narrow")
    if context != frozen - effective:
        raise ValueError("write-scope removed locator is not preserved as exact read-only context")
    if effective | context != frozen:
        raise ValueError("write-scope supersession introduces or loses a locator")


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


def self_test(payload: dict[str, Any]) -> None:
    expect_failure(
        "scope widening", payload,
        lambda item: item["supersessions"][0]["effective_write_locators"].append(CONTEXT),
        "schema maxItems failed at $.supersessions[0].effective_write_locators",
    )
    expect_failure(
        "context omission", payload,
        lambda item: item["supersessions"][0]["read_only_context_locators"].pop(),
        "schema minItems failed at $.supersessions[0].read_only_context_locators",
    )
    expect_failure(
        "owner swap", payload,
        lambda item: item["supersessions"][0].update({"context_function_owner_atomic_pr_id": "T1-M02-P408-WRT-n004-l48"}),
        "schema const mismatch at $.supersessions[0].context_function_owner_atomic_pr_id",
    )
    expect_failure(
        "edge reversal", payload,
        lambda item: item["supersessions"][0]["integration_edge"].update({"from": "M02-N004-L21"}),
        "schema const mismatch at $.supersessions[0].integration_edge.from",
    )
    expect_failure(
        "false activation", payload,
        lambda item: item["activation_policy"].update({"decision": "PASS"}),
        "schema const mismatch at $.activation_policy.decision",
    )
    expect_failure(
        "source hash drift", payload,
        lambda item: item["source_catalog"].update({"sha256": "0" * 64}),
        "differs from exact derived state",
    )


def render_markdown(payload: dict[str, Any]) -> str:
    record = payload["supersessions"][0]
    return "\n".join([
        "# M02写范围Supersession账本", "",
        "状态：`DESIGNED_NOT_REGISTERED / BLOCKED / NO-GO`", "",
        "本账本追加有效执行范围的窄化规则，不修改P408冻结记录，也不使任何active catalog或执行包生效。", "",
        "## 唯一规则", "",
        f"- predecessor：`{record['predecessor_atomic_pr_id']}`。",
        f"- 有效可写：`{record['effective_write_locators'][0]}`。",
        f"- 只读context：`{record['read_only_context_locators'][0]}`。",
        f"- context函数体owner：`{record['context_function_owner_atomic_pr_id']}`。",
        "- 集成方向：P408 helper先于P165 body；P165在自己拥有的body内调用helper。", "",
        "## 激活门", "",
        *[f"- `{item}`" for item in payload["activation_policy"]["required_evidence"]], "",
        f"失败规则：`{payload['activation_policy']['failure_rule']}`。", "",
        "## 证明上限", "", f"`{payload['proof_ceiling']}`", "",
    ])


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
        raise ValueError("generated write-scope supersession ledger is stale")
    if not DOC_OUTPUT.is_file() or DOC_OUTPUT.read_text(encoding="utf-8") != markdown:
        raise ValueError("generated write-scope supersession markdown is stale")
    validate(payload)
    if args.verify:
        self_test(payload)
    print("PASS P408 effective write scope narrows to replay_delay; poll_manifest remains P165-owned read-only context")
    print(f"PROOF_CEILING {payload['proof_ceiling']}")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
