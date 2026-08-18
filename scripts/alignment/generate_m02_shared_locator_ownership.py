#!/usr/bin/env python3
"""Generate the P165/P408 poll_manifest single-writer boundary, fail closed."""

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
SCHEMA = REPO / "contracts/alignment/m02-shared-locator-ownership.schema.json"
OUTPUT = REPO / "contracts/alignment/m02-shared-locator-ownership.v1.json"
SHARED = "rust/probe-agent/probe-agent/src/capture/pcap_offline.rs#ManifestPcapReplayer::poll_manifest"


def sha256(path: Path) -> str:
    return hashlib.sha256(path.read_bytes()).hexdigest()


def ref(path: Path) -> dict[str, str]:
    return {"path": path.relative_to(REPO).as_posix(), "sha256": sha256(path)}


def transitive_dependencies(leaves: dict[str, dict[str, Any]], leaf_id: str) -> set[str]:
    result: set[str] = set()
    stack = [leaf_id]
    while stack:
        current = stack.pop()
        for dependency in leaves[current]["depends_on_leaf_ids"]:
            if dependency not in result:
                result.add(dependency)
                stack.append(dependency)
    return result


def build() -> dict[str, Any]:
    catalog = json.loads(CATALOG.read_text(encoding="utf-8"))
    allocation = json.loads(ALLOCATION.read_text(encoding="utf-8"))
    leaves = {item["leaf_id"]: item for item in catalog["leaves"]}
    steps = {
        item["leaf_id"]: item
        for train in allocation["trains"]
        for item in train["steps"]
    }
    owner = leaves["M02-N004-L21"]
    helper = leaves["M02-N004-L48"]
    if owner["write_locators"] != [SHARED]:
        raise ValueError("P165 poll_manifest primary ownership drifted")
    if steps["M02-N004-L48"]["primary_locator"] != "rust/probe-agent/probe-agent/src/capture/pcap_offline.rs#replay_delay":
        raise ValueError("P408 replay_delay primary ownership drifted")
    if SHARED not in steps["M02-N004-L48"]["companion_locators"]:
        raise ValueError("P408 poll_manifest companion occurrence is absent")
    edge = next((item for item in catalog["append_only_edges"] if item["from"] == "M02-N004-L48" and item["to"] == "M02-N004-L21"), None)
    if edge is None or edge["edge_kind"] != "APPENDED_TO_EXISTING":
        raise ValueError("P408 to P165 ordered edge drifted")
    depends_on_creator = "M02-N004-L17" in transitive_dependencies(leaves, "M02-N004-L48")
    if depends_on_creator:
        raise ValueError("P408 unexpectedly depends on the manifest replayer creator")
    return {
        "schema_version": "1.0.0",
        "artifact_kind": "M02_SHARED_LOCATOR_SINGLE_WRITER_BOUNDARY",
        "artifact_status": "BLOCKED_FROZEN_ALLOCATION_CONFORMANCE",
        "decision_id": "M02-N004-P165-P408-POLL-MANIFEST-OWNER-V1",
        "source_catalog": ref(CATALOG),
        "source_allocation": ref(ALLOCATION),
        "shared_locator": SHARED,
        "current_occurrences": [
            {
                "leaf_id": owner["leaf_id"],
                "atomic_pr_id": owner["atomic_pr_id"],
                "locator_role": "PRIMARY",
                "single_outcome": owner["single_outcome"],
            },
            {
                "leaf_id": helper["leaf_id"],
                "atomic_pr_id": helper["atomic_pr_id"],
                "locator_role": "COMPANION",
                "single_outcome": helper["single_outcome"],
            },
        ],
        "dependency_evidence": {
            "ordered_edge": {
                "from": edge["from"], "to": edge["to"],
                "edge_kind": edge["edge_kind"], "present": True,
            },
            "helper_owner_depends_on_state_creator": depends_on_creator,
            "finding": "ORDERING_EXISTS_BUT_DOES_NOT_GRANT_A_SECOND_FUNCTION_BODY_WRITER",
        },
        "required_single_writer_boundary": {
            "function_body_owner": owner["atomic_pr_id"],
            "helper_owner": helper["atomic_pr_id"],
            "helper_integration_mode": "P165_CALLS_REPLAY_DELAY_INSIDE_ITS_OWN_POLL_MANIFEST_BODY",
            "forbidden_for_helper_owner": [
                "WRITE_POLL_MANIFEST_BODY", "CREATE_MANIFEST_REPLAYER", "ALTER_ENTRY_SWITCHING_OR_IDENTITY",
            ],
        },
        "allocation_conformance": {
            "status": "FAIL_CLOSED",
            "blocking_facts": [
                "P408_CURRENTLY_LISTS_POLL_MANIFEST_AS_A_WRITABLE_COMPANION",
                "P408_DEPENDENCY_CLOSURE_DOES_NOT_CONTAIN_L17_MANIFEST_REPLAYER_CREATOR",
            ],
        },
        "required_closure": [
            "PRESERVE_FROZEN_P308_P485_PREFIX_AND_RECORD_A_NEW_APPEND_ONLY_SUPERSESSION_CONTRACT",
            "REMOVE_POLL_MANIFEST_FROM_P408_WRITABLE_SCOPE_WITHOUT_REWRITING_FROZEN_HISTORY",
            "BIND_P165_AND_P408_TO_ONE_CLEAN_CANDIDATE_WITH_EXACT_RUST_AST_RECEIPTS",
            "OBTAIN_SIGNED_FUNCTION_REVIEWS_FOR_BOTH_ATOMIC_PRS",
        ],
        "validation": {
            "source_exact": "PASS",
            "occurrence_roles_exact": "PASS",
            "ordered_edge_exact": "PASS",
            "missing_dependency_proven": True,
            "false_closure_rejected": True,
            "mutation_guards": {
                "owner_swap": "PASS", "companion_omission": "PASS", "edge_drift": "PASS",
                "false_conformance": "PASS", "missing_dependency_drift": "PASS",
            },
        },
        "proof_ceiling": "STATIC_SINGLE_WRITER_BOUNDARY_ONLY_NOT_ALLOCATION_REWRITTEN_LOCATOR_RESOLVED_FUNCTION_REVIEWED_IMPLEMENTED_OR_AUTHORIZED",
    }


def validate(payload: dict[str, Any]) -> None:
    validate_against_schema(payload, SCHEMA)
    if payload != build():
        raise ValueError("shared locator ownership ledger differs from exact derived state")


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
        "owner swap", payload,
        lambda item: item["required_single_writer_boundary"].update({"function_body_owner": "T1-M02-P408-WRT-n004-l48"}),
        "schema const mismatch at $.required_single_writer_boundary.function_body_owner",
    )
    expect_failure(
        "companion omission", payload,
        lambda item: item["current_occurrences"].pop(),
        "schema minItems failed at $.current_occurrences",
    )
    expect_failure(
        "edge drift", payload,
        lambda item: item["dependency_evidence"]["ordered_edge"].update({"from": "M02-N004-L21"}),
        "schema const mismatch at $.dependency_evidence.ordered_edge.from",
    )
    expect_failure(
        "false conformance", payload,
        lambda item: item["allocation_conformance"].update({"status": "PASS"}),
        "schema const mismatch at $.allocation_conformance.status",
    )
    expect_failure(
        "missing dependency drift", payload,
        lambda item: item["dependency_evidence"].update({"helper_owner_depends_on_state_creator": True}),
        "schema const mismatch at $.dependency_evidence.helper_owner_depends_on_state_creator",
    )


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
    if args.write:
        OUTPUT.write_text(expected, encoding="utf-8")
        print(f"WROTE {OUTPUT.relative_to(REPO)}")
        return 0
    if not OUTPUT.is_file() or OUTPUT.read_text(encoding="utf-8") != expected:
        raise ValueError("generated shared locator ownership ledger is stale")
    validate(payload)
    if args.verify:
        self_test(payload)
    print("PASS P165 primary owner / P408 helper owner; frozen allocation conformance remains FAIL_CLOSED")
    print(f"PROOF_CEILING {payload['proof_ceiling']}")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
