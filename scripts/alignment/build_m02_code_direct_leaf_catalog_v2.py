#!/usr/bin/env python3
"""Build and validate the append-only M02 code-direct catalog revision v2.

The v2 preview preserves every field of P101-P307, allocates P308-P506 from
an explicit ledger, and version-revises completion exact sets without changing
the legacy terminal leaf identities.  It does not switch the global catalogs.
"""

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
BASE = REPO / "contracts/alignment/m02-code-direct-leaf-catalog.v1.json"
BASE_SCHEMA = REPO / "contracts/alignment/m02-code-direct-leaf-catalog.schema.json"
ALLOCATION = REPO / "contracts/alignment/m02-code-direct-leaf-allocation.v2.json"
ALLOCATION_SCHEMA = REPO / "contracts/alignment/m02-code-direct-leaf-allocation.v2.schema.json"
OUTPUT = REPO / "contracts/alignment/m02-code-direct-leaf-catalog.v2.json"
SCHEMA = REPO / "contracts/alignment/m02-code-direct-leaf-catalog.v2.schema.json"

EXPECTED_BASE_SHA256 = "29b7584696440bb70a8203f4ecbe4ae6574f1a785743562b94e87efbde73a703"
EXPECTED_ALLOCATION_SEMANTIC_SHA256 = "7cf8b202c8004654e54d5ed52a5d085b2b1e20e30d85ef521604b70c79e0b621"
EXPECTED_LEGACY_LEAF_PROJECTION_SHA256 = "2199097bdcf70c496bdf9f5ba868f43313c3c7ac0b6d9e9200ee2e66ca83dbec"
EXPECTED_PRIOR_APPEND_PREFIX_PROJECTION_SHA256 = "3f407f257b303a8b09992db3a34b3bbfef0f8145a804e4e33129c4eabac8ca1d"
EXPECTED_TERMINAL_BY_PARENT = {
    "T1-M02-N001": "M02-N001-L24",
    "T1-M02-N002": "M02-N002-L08",
    "T1-M02-N003": "M02-N003-L12",
    "T1-M02-N004": "M02-N004-L26",
    "T1-M02-N005": "M02-N005-L09",
    "T1-M02-N006": "M02-N006-L20",
    "T1-M02-N007": "M02-N007-L10",
    "T1-M02-N008": "M02-N008-L10",
    "T1-M02-N009": "M02-N009-L13",
    "T1-M02-N010": "M02-N010-L07",
    "T1-M02-N011": "M02-N011-L10",
    "T1-M02-N012": "M02-N012-L23",
    "T1-M02-N013": "M02-N013-L09",
    "T1-M02-N014": "M02-N014-L09",
    "T1-M02-N015": "M02-N015-L11",
    "T1-M02-N016": "M02-N016-L06",
}
EXPECTED_APPEND_PARENT_COUNTS = {
    "T1-M02-N001": 6,
    "T1-M02-N004": 27,
    "T1-M02-N005": 36,
    "T1-M02-N006": 21,
    "T1-M02-N008": 13,
    "T1-M02-N012": 96,
}
EXPECTED_SOURCE_DESIGNS = [
    "contracts/alignment/m02-partial-ack-function-design.v1.json",
    "contracts/alignment/m02-pcap-spool-function-design.v1.json",
    "contracts/alignment/m02-pcap-consumer-function-design.v1.json",
    "contracts/alignment/m02-pcap-projection-function-design.v1.json",
    "contracts/alignment/m02-pcap-metadata-receipt-function-design.v1.json",
    "contracts/alignment/m02-capture-flow-identity-function-design.v1.json",
    "contracts/alignment/m02-probe-control-ack-function-design.v1.json",
]
EXPECTED_SOURCE_VALIDATORS = [
    "scripts/alignment/validate_m02_partial_ack_function_design.py",
    "scripts/alignment/validate_m02_pcap_spool_function_design.py",
    "scripts/alignment/validate_m02_pcap_consumer_function_design.py",
    "scripts/alignment/validate_m02_pcap_projection_function_design.py",
    "scripts/alignment/validate_m02_pcap_metadata_receipt_function_design.py",
    "scripts/alignment/validate_m02_capture_flow_identity_function_design.py",
    "scripts/alignment/validate_m02_probe_control_ack_function_design.py",
]
EXPECTED_GLOBAL_CATALOGS = [
    "contracts/alignment/task-registry.v1.json",
    "contracts/alignment/developer-claim-package-catalog.v1.json",
    "contracts/alignment/pr-design-application-catalog.v1.json",
    "contracts/alignment/task-execution-overlay.template.v1.json",
]
LEGACY_LEAF_FIELDS = [
    "leaf_id",
    "atomic_pr_id",
    "parent_task_id",
    "leaf_number",
    "pr_type",
    "phase",
    "write_locators",
    "target_state",
    "prerequisites_raw",
    "single_outcome",
    "oracle_and_rollback",
    "depends_on_leaf_ids",
    "depends_on_external_activities",
    "terminal_task_idx",
    "formal_execution_status",
    "allowed_claim",
    "forbidden_claims",
]
FORMAL_STATUS = "BLOCKED_UNTIL_GLOBAL_REGISTRY_TARGET_BINDING_FUNCTION_REVIEW_AND_SIGNED_OVERLAY"
ALLOWED_CLAIM = "M02 v2 append-only leaf identity and dependency intent are frozen in preview"
FORBIDDEN_CLAIMS = [
    "GLOBAL_REGISTRY_SWITCHED",
    "TARGET_BINDING_COMPLETE",
    "FUNCTION_DESIGN_REVIEWED",
    "IMPLEMENTED",
    "TEST_EXECUTED",
    "EXECUTION_AUTHORIZED",
    "M02_ACCEPTED",
]


def sha256(path: Path) -> str:
    return hashlib.sha256(path.read_bytes()).hexdigest()


def canonical_bytes(value: Any) -> bytes:
    return json.dumps(value, ensure_ascii=False, sort_keys=True, separators=(",", ":")).encode("utf-8")


def projection_hash(leaves: list[dict[str, Any]]) -> str:
    projection = [{field: item[field] for field in LEGACY_LEAF_FIELDS} for item in leaves]
    return hashlib.sha256(canonical_bytes(projection)).hexdigest()


def atomic_number(atomic_pr_id: str) -> int:
    match = re.search(r"-P([0-9]{3})-", atomic_pr_id)
    if match is None:
        raise ValueError(f"invalid atomic PR ID: {atomic_pr_id}")
    return int(match.group(1))


def owner_leaf(train: dict[str, Any]) -> str:
    owners = [
        step["leaf_id"]
        for step in train["steps"]
        if step["pr_type"] in {"CTR", "WRT"}
    ]
    if len(owners) != 1:
        raise ValueError(f"{train['train_id']} must contain exactly one CTR or WRT owner")
    return owners[0]


def support_types(train: dict[str, Any]) -> list[str]:
    return [step["pr_type"] for step in train["steps"]]


def ordered_trains(allocation: dict[str, Any]) -> list[dict[str, Any]]:
    return sorted(
        allocation["trains"],
        key=lambda train: atomic_number(train["steps"][0]["atomic_pr_id"]),
    )


def allocation_semantic_projection(allocation: dict[str, Any]) -> dict[str, Any]:
    return {
        "revision_id": allocation["revision_id"],
        "base_catalog": allocation["base_catalog"],
        "source_design_refs": allocation["source_design_refs"],
        "allocation_epoch": allocation["allocation_epoch"],
        "append_leaf_count": allocation["append_leaf_count"],
        "trains": allocation["trains"],
    }


def allocation_semantic_hash(allocation: dict[str, Any]) -> str:
    return hashlib.sha256(canonical_bytes(allocation_semantic_projection(allocation))).hexdigest()


def assert_allocation(
    allocation: dict[str, Any],
    base: dict[str, Any],
    *,
    enforce_semantic_pin: bool = True,
) -> None:
    validate_against_schema(allocation, ALLOCATION_SCHEMA)
    if allocation["base_catalog"] != {
        "path": BASE.relative_to(REPO).as_posix(),
        "sha256": EXPECTED_BASE_SHA256,
    }:
        raise ValueError("allocation base catalog hash or path drifted")
    if sha256(BASE) != EXPECTED_BASE_SHA256:
        raise ValueError("frozen v1 base catalog hash drifted")
    validate_against_schema(base, BASE_SCHEMA)
    if projection_hash(base["leaves"]) != EXPECTED_LEGACY_LEAF_PROJECTION_SHA256:
        raise ValueError("frozen P101-P307 legacy leaf projection drifted")
    expected_source_refs = [
        {"path": relative, "sha256": sha256(REPO / relative)}
        for relative in EXPECTED_SOURCE_DESIGNS
    ]
    if allocation["source_design_refs"] != expected_source_refs:
        raise ValueError("allocation source function-design hash set drifted")
    if enforce_semantic_pin and allocation["semantic_projection_sha256"] != EXPECTED_ALLOCATION_SEMANTIC_SHA256:
        raise ValueError("allocation semantic projection is not the independently pinned revision")
    if allocation["semantic_projection_sha256"] != allocation_semantic_hash(allocation):
        raise ValueError("allocation semantic projection hash drifted")

    trains = allocation["trains"]
    expected_train_ids = {f"M02-V2-TRAIN-{index:02d}" for index in range(1, 66)}
    if {item["train_id"] for item in trains} != expected_train_ids:
        raise ValueError("allocation train ID exact-set drifted")
    if len({item["gap_id"] for item in trains}) != 65:
        raise ValueError("allocation gap IDs must be unique")
    steps = [step for train in trains for step in train["steps"]]
    if len(steps) != 199:
        raise ValueError("append allocation must contain exactly 199 explicit steps")
    if sorted(atomic_number(item["atomic_pr_id"]) for item in steps) != list(range(308, 507)):
        raise ValueError("append atomic IDs must be explicit contiguous P308-P506")

    base_leaf_ids = {item["leaf_id"] for item in base["leaves"]}
    base_atomic_ids = {item["atomic_pr_id"] for item in base["leaves"]}
    leaf_ids = [item["leaf_id"] for item in steps]
    atomic_ids = [item["atomic_pr_id"] for item in steps]
    if len(set(leaf_ids)) != 199 or base_leaf_ids.intersection(leaf_ids):
        raise ValueError("append leaf ID is duplicated or reuses a base leaf ID")
    if len(set(atomic_ids)) != 199 or base_atomic_ids.intersection(atomic_ids):
        raise ValueError("append atomic PR ID is duplicated or reuses a base PR ID")

    by_parent: dict[str, list[dict[str, Any]]] = defaultdict(list)
    for step in steps:
        match = re.fullmatch(r"M02-N([0-9]{3})-L([0-9]{2,3})", step["leaf_id"])
        if match is None:
            raise ValueError(f"invalid leaf ID: {step['leaf_id']}")
        parent = f"T1-M02-N{match.group(1)}"
        if f"-n{match.group(1)}-l{match.group(2)}" not in step["atomic_pr_id"]:
            raise ValueError(f"atomic ID does not encode leaf identity: {step['leaf_id']}")
        if f"-{step['pr_type']}-" not in step["atomic_pr_id"]:
            raise ValueError(f"atomic ID type token differs from pr_type: {step['leaf_id']}")
        by_parent[parent].append(step)
    if {parent: len(items) for parent, items in by_parent.items()} != EXPECTED_APPEND_PARENT_COUNTS:
        raise ValueError("append parent allocation exact-set drifted")
    for parent, items in by_parent.items():
        base_count = base["parent_counts"][parent]
        numbers = sorted(int(item["leaf_id"].rsplit("L", 1)[1]) for item in items)
        if numbers != list(range(base_count + 1, base_count + len(items) + 1)):
            raise ValueError(f"{parent} append leaf numbers are not contiguous after v1")

    all_leaf_ids = base_leaf_ids | set(leaf_ids)
    terminal_ids = set(EXPECTED_TERMINAL_BY_PARENT.values())
    for train in trains:
        expected_parent = train["parent_task_id"].removeprefix("T1-")
        if any(
            not item["leaf_id"].startswith(f"{expected_parent}-L")
            for item in train["steps"]
        ):
            raise ValueError(f"{train['train_id']} step parent differs from train parent")
        types = support_types(train)
        expected = (
            ["EXP", "WRT", "REF", "TST-PRE"]
            if train["owner_kind"] == "DATABASE_MIGRATION_AND_WRITER"
            else ["CTR", "REF", "TST-PRE"]
            if train["owner_kind"] == "CONTRACT"
            else ["WRT", "REF", "TST-PRE"]
        )
        if types != expected:
            raise ValueError(f"{train['train_id']} support train drifted: {types}")
        if train["terminal_leaf_id"] != EXPECTED_TERMINAL_BY_PARENT[train["parent_task_id"]]:
            raise ValueError(f"{train['train_id']} terminal mapping drifted")
        previous: str | None = None
        for item in train["steps"]:
            dependencies = item["prerequisite_leaf_ids"]
            if previous is not None and previous not in dependencies:
                raise ValueError(f"{item['leaf_id']} does not depend on prior support step {previous}")
            if terminal_ids.intersection(dependencies):
                raise ValueError(f"new leaf depends on a legacy terminal: {item['leaf_id']}")
            unknown = set(dependencies) - all_leaf_ids
            if unknown:
                raise ValueError(f"{item['leaf_id']} has unknown dependencies: {sorted(unknown)}")
            previous = item["leaf_id"]
        if set(train["feeds_existing_leaf_ids"]) - base_leaf_ids:
            raise ValueError(f"{train['train_id']} feeds an unknown existing leaf")
        if train["terminal_leaf_id"] in train["feeds_existing_leaf_ids"]:
            raise ValueError(f"{train['train_id']} must use completion revision, not a normal terminal feed")
        owner_leaf(train)
    locator_owners: dict[str, list[str]] = defaultdict(list)
    for step in steps:
        for locator in [step["primary_locator"], *step["companion_locators"]]:
            locator_owners[locator].append(step["atomic_pr_id"])
    duplicate_locators = {
        locator: owners for locator, owners in locator_owners.items() if len(owners) != 1
    }
    if duplicate_locators:
        raise ValueError(f"append allocation reuses a write locator: {duplicate_locators}")


def appended_leaf(step: dict[str, Any], parent_task_id: str) -> dict[str, Any]:
    leaf_number = int(step["leaf_id"].rsplit("L", 1)[1])
    locators = [step["primary_locator"], *step["companion_locators"]]
    return {
        "leaf_id": step["leaf_id"],
        "atomic_pr_id": step["atomic_pr_id"],
        "parent_task_id": parent_task_id,
        "leaf_number": leaf_number,
        "pr_type": step["pr_type"],
        "phase": f"m02-n{parent_task_id[-3:]}-l{leaf_number:02d}",
        "write_locators": locators,
        "target_state": step["target_state"],
        "prerequisites_raw": (
            "append-only prerequisites: " + ", ".join(step["prerequisite_leaf_ids"])
            if step["prerequisite_leaf_ids"]
            else "append-only revision root"
        ),
        "single_outcome": step["single_outcome"],
        "oracle_and_rollback": step["oracle_and_rollback"],
        "depends_on_leaf_ids": step["prerequisite_leaf_ids"],
        "depends_on_external_activities": [],
        "terminal_task_idx": False,
        "formal_execution_status": FORMAL_STATUS,
        "allowed_claim": ALLOWED_CLAIM,
        "forbidden_claims": FORBIDDEN_CLAIMS,
    }


def assert_acyclic(nodes: set[str], edges: list[dict[str, str]]) -> None:
    indegree = {node: 0 for node in nodes}
    outgoing: dict[str, set[str]] = defaultdict(set)
    for edge in edges:
        source, target = edge["from"], edge["to"]
        if source not in nodes or target not in nodes:
            raise ValueError(f"mixed DAG edge references unknown node: {source} -> {target}")
        if target not in outgoing[source]:
            outgoing[source].add(target)
            indegree[target] += 1
    ready = sorted(node for node, degree in indegree.items() if degree == 0)
    visited = 0
    while ready:
        source = ready.pop(0)
        visited += 1
        for target in sorted(outgoing[source]):
            indegree[target] -= 1
            if indegree[target] == 0:
                ready.append(target)
                ready.sort()
    if visited != len(nodes):
        cyclic = sorted(node for node, degree in indegree.items() if degree > 0)
        raise ValueError(f"M02 v2 mixed DAG contains a cycle involving {cyclic[:12]}")


def assert_unique_ids(leaves: list[dict[str, Any]]) -> None:
    leaf_ids = [item["leaf_id"] for item in leaves]
    atomic_ids = [item["atomic_pr_id"] for item in leaves]
    if len(set(leaf_ids)) != len(leaves) or len(set(atomic_ids)) != len(leaves):
        raise ValueError("v2 leaf or atomic ID reuse detected")


def active_global_atomic_ids(relative: str) -> set[str]:
    payload = json.loads((REPO / relative).read_text(encoding="utf-8"))
    if relative.endswith("task-registry.v1.json"):
        values = [item["pr_id"] for task in payload["tasks"] for item in task["pr_sequence"]]
        values += [item["pr_id"] for group in payload["closure_slices"] for item in group["pr_sequence"]]
    elif relative.endswith("developer-claim-package-catalog.v1.json"):
        values = [item["atomic_pr_id"] for item in payload["packages"]]
    elif relative.endswith("pr-design-application-catalog.v1.json"):
        values = [item["atomic_pr_id"] for item in payload["entries"]]
    elif relative.endswith("task-execution-overlay.template.v1.json"):
        values = [item["pr_id"] for item in payload["atomic_pr_bindings"]]
    else:
        raise ValueError(f"unsupported global catalog: {relative}")
    if len(values) != len(set(values)):
        raise ValueError(f"global catalog contains duplicate atomic IDs: {relative}")
    return set(values)


def assert_global_atomic_id_sets(sets: list[set[str]]) -> set[str]:
    if any(items != sets[0] for items in sets[1:]):
        raise ValueError("global four-catalog atomic ID exact-sets differ")
    return sets[0]


def derive_global_switch_counts() -> tuple[int, int, int]:
    sets = [active_global_atomic_ids(relative) for relative in EXPECTED_GLOBAL_CATALOGS]
    active_ids = assert_global_atomic_id_sets(sets)
    legacy_m02 = {item for item in active_ids if re.match(r"^T1-M02-P[0-9]{3}-", item)}
    if len(active_ids) != 1289 or len(legacy_m02) != 34:
        raise ValueError("global active or legacy M02 derived count drifted")
    return len(active_ids), len(legacy_m02), len(active_ids) - len(legacy_m02) + 406


def edge_key(edge: dict[str, Any]) -> tuple[str, str]:
    return edge["from"], edge["to"]


def build_expected_edges(
    base: dict[str, Any],
    appended: list[dict[str, Any]],
    allocation: dict[str, Any],
    completion_revisions: list[dict[str, Any]],
) -> list[dict[str, str]]:
    edges: list[dict[str, str]] = [
        {**item, "edge_kind": "BASE_FROZEN"}
        for item in base["cross_leaf_edges"]
    ]
    for item in appended:
        for source in item["depends_on_leaf_ids"]:
            edges.append({
                "from": source,
                "to": item["leaf_id"],
                "reason": "explicit v2 allocation prerequisite",
                "edge_kind": "APPENDED_PREREQUISITE",
            })
    for train in ordered_trains(allocation):
        source = owner_leaf(train)
        for target in train["feeds_existing_leaf_ids"]:
            edges.append({
                "from": source,
                "to": target,
                "reason": f"{train['gap_id']} owner precedes frozen adapter",
                "edge_kind": "APPENDED_TO_EXISTING",
            })
    for revision in completion_revisions:
        for source in revision["appended_completion_member_leaf_ids"]:
            edges.append({
                "from": source,
                "to": revision["terminal_leaf_id"],
                "reason": "v2 completion exact-set revision",
                "edge_kind": "COMPLETION_REVISION",
            })
    deduped: dict[tuple[str, str], dict[str, str]] = {}
    for edge in edges:
        key = edge_key(edge)
        previous = deduped.get(key)
        if previous is not None and previous != edge:
            if edge["edge_kind"] == "COMPLETION_REVISION":
                deduped[key] = edge
            elif previous["edge_kind"] != "COMPLETION_REVISION":
                raise ValueError(f"conflicting v2 edge definitions: {key}")
        else:
            deduped[key] = edge
    return [deduped[key] for key in sorted(deduped)]


def build() -> dict[str, Any]:
    base = json.loads(BASE.read_text(encoding="utf-8"))
    allocation = json.loads(ALLOCATION.read_text(encoding="utf-8"))
    assert_allocation(allocation, base)

    appended: list[dict[str, Any]] = []
    for train in ordered_trains(allocation):
        for item in train["steps"]:
            appended.append(appended_leaf(item, train["parent_task_id"]))
    leaves = copy.deepcopy(base["leaves"]) + appended

    append_by_parent: dict[str, list[str]] = defaultdict(list)
    for item in appended:
        append_by_parent[item["parent_task_id"]].append(item["leaf_id"])

    completion_revisions = []
    base_by_id = {item["leaf_id"]: item for item in base["leaves"]}
    for parent in sorted(EXPECTED_APPEND_PARENT_COUNTS):
        terminal_id = EXPECTED_TERMINAL_BY_PARENT[parent]
        base_members = base_by_id[terminal_id]["depends_on_leaf_ids"]
        appended_members = [
            train["steps"][-1]["leaf_id"]
            for train in ordered_trains(allocation)
            if train["parent_task_id"] == parent
        ]
        completion_revisions.append({
            "parent_task_id": parent,
            "terminal_leaf_id": terminal_id,
            "base_completion_member_leaf_ids": base_members,
            "appended_completion_member_leaf_ids": appended_members,
            "revised_completion_member_leaf_ids": sorted(set(base_members) | set(appended_members)),
            "revision_semantics": "TERMINAL_ID_AND_FLAG_FROZEN_COMPLETION_EXACT_SET_VERSION_REVISED",
        })

    edges = build_expected_edges(base, appended, allocation, completion_revisions)

    type_counts = dict(sorted(Counter(item["pr_type"] for item in leaves).items()))
    parent_counts = dict(sorted(Counter(item["parent_task_id"] for item in leaves).items()))
    global_atomic_count, legacy_m02_count, candidate_global_count = derive_global_switch_counts()
    payload = {
        "schema_version": "2.0.0",
        "artifact_kind": "M02_CODE_DIRECT_LEAF_CATALOG_REVISION",
        "artifact_status": "VERSIONED_PREVIEW_NOT_GLOBAL_REGISTRY",
        "revision_id": "M02-CODE-DIRECT-V2",
        "base_catalog": {
            "path": BASE.relative_to(REPO).as_posix(),
            "sha256": sha256(BASE),
        },
        "allocation_ledger": {
            "path": ALLOCATION.relative_to(REPO).as_posix(),
            "sha256": sha256(ALLOCATION),
        },
        "id_epoch": "T1-M02-P101-P506",
        "base_leaf_count": 207,
        "appended_leaf_count": 199,
        "leaf_count": len(leaves),
        "type_counts": type_counts,
        "parent_counts": parent_counts,
        "legacy_leaf_field_freeze": {
            "leaf_id_epoch": "T1-M02-P101-P307",
            "field_names": LEGACY_LEAF_FIELDS,
            "ordered_leaf_projection_sha256": projection_hash(base["leaves"]),
            "status": "PASS_EXACT_FIELD_PRESERVATION",
        },
        "prior_append_prefix_freeze": {
            "leaf_id_epoch": "T1-M02-P308-P485",
            "field_names": LEGACY_LEAF_FIELDS,
            "ordered_leaf_projection_sha256": projection_hash(appended[:178]),
            "status": "PASS_EXACT_PREFIX_PRESERVATION",
        },
        "leaves": leaves,
        "terminal_by_parent": EXPECTED_TERMINAL_BY_PARENT,
        "completion_contract_revisions": completion_revisions,
        "append_only_edges": edges,
        "external_activities": base["external_activities"],
        "validation": {
            "schema": "PASS",
            "base_catalog_hash": "PASS",
            "base_leaf_field_exact": "PASS",
            "prior_append_prefix_exact": "PASS",
            "allocation_explicit_exact_set": "PASS",
            "id_epoch_contiguous": "PASS",
            "unique_leaf_ids": True,
            "unique_atomic_pr_ids": True,
            "support_train_exact": "PASS",
            "terminal_map_exact": "PASS",
            "completion_member_exact": "PASS",
            "no_new_leaf_depends_on_terminal": True,
            "mixed_dag": "PASS",
            "mutation_guards": {
                "old_leaf_drift": "PASS",
                "prior_append_prefix_drift": "PASS",
                "id_reuse": "PASS",
                "support_shape_drift": "PASS",
                "second_terminal": "PASS",
                "new_leaf_depends_on_terminal": "PASS",
                "completion_omission": "PASS",
                "dag_cycle": "PASS",
                "catalog_hash_mismatch": "PASS",
                "allocation_semantic_drift": "PASS",
                "extra_edge": "PASS",
                "wrong_edge_kind": "PASS",
                "unrelated_feed": "PASS",
                "legacy_field_list_drift": "PASS",
                "atomic_type_token_mismatch": "PASS",
                "parent_mismatch": "PASS",
                "locator_reuse": "PASS",
                "global_catalog_exact_set": "PASS",
            },
        },
        "global_switch_gate": {
            "decision": "BLOCKED_PREVIEW_ONLY",
            "legacy_active_m02_pr_count": legacy_m02_count,
            "candidate_m02_leaf_count": 406,
            "candidate_global_atomic_pr_count": candidate_global_count,
            "legacy_catalog_refs": [
                {
                    "path": relative,
                    "sha256": sha256(REPO / relative),
                    "status": "CURRENT_ACTIVE_INPUT_NOT_V2",
                }
                for relative in EXPECTED_GLOBAL_CATALOGS
            ],
            "required_catalogs": EXPECTED_GLOBAL_CATALOGS,
            "switch_rule": "TASK_CLAIM_PR_DESIGN_AND_OVERLAY_MUST_SWITCH_ATOMICALLY_TO_ONE_CANDIDATE_HASH_AFTER_REVIEW",
        },
        "proof_ceiling": "VERSIONED_STATIC_DESIGN_PREVIEW_ONLY_NOT_GLOBAL_REGISTRATION_TARGET_BINDING_FUNCTION_REVIEW_IMPLEMENTATION_TEST_EXECUTION_AUTHORIZATION_OR_ACCEPTANCE",
    }
    validate_payload(payload, base, allocation)
    return payload


def validate_payload(
    payload: dict[str, Any],
    base: dict[str, Any] | None = None,
    allocation: dict[str, Any] | None = None,
) -> None:
    if base is None:
        base = json.loads(BASE.read_text(encoding="utf-8"))
    if allocation is None:
        allocation = json.loads(ALLOCATION.read_text(encoding="utf-8"))
    validate_against_schema(payload, SCHEMA)
    assert_allocation(allocation, base)
    if payload["base_catalog"] != {
        "path": BASE.relative_to(REPO).as_posix(),
        "sha256": sha256(BASE),
    }:
        raise ValueError("v2 base catalog reference mismatch")
    if payload["allocation_ledger"] != {
        "path": ALLOCATION.relative_to(REPO).as_posix(),
        "sha256": sha256(ALLOCATION),
    }:
        raise ValueError("v2 allocation ledger reference mismatch")

    leaves = payload["leaves"]
    if leaves[:207] != base["leaves"]:
        raise ValueError("legacy P101-P307 leaf field drift detected")
    if payload["legacy_leaf_field_freeze"]["ordered_leaf_projection_sha256"] != projection_hash(base["leaves"]):
        raise ValueError("legacy leaf projection hash mismatch")
    if payload["legacy_leaf_field_freeze"]["field_names"] != LEGACY_LEAF_FIELDS:
        raise ValueError("legacy leaf field-name exact-set drifted")
    appended = leaves[207:]
    allocation_steps = [step for train in ordered_trains(allocation) for step in train["steps"]]
    if len(appended) != 199 or [item["leaf_id"] for item in appended] != [item["leaf_id"] for item in allocation_steps]:
        raise ValueError("v2 appended leaf allocation exact-set drifted")
    prior_prefix = appended[:178]
    if payload["prior_append_prefix_freeze"] != {
        "leaf_id_epoch": "T1-M02-P308-P485",
        "field_names": LEGACY_LEAF_FIELDS,
        "ordered_leaf_projection_sha256": EXPECTED_PRIOR_APPEND_PREFIX_PROJECTION_SHA256,
        "status": "PASS_EXACT_PREFIX_PRESERVATION",
    } or projection_hash(prior_prefix) != EXPECTED_PRIOR_APPEND_PREFIX_PROJECTION_SHA256:
        raise ValueError("prior P308-P485 append prefix field drift detected")
    actual_atomic = [atomic_number(item["atomic_pr_id"]) for item in leaves]
    if actual_atomic[:207] != list(range(101, 308)) or sorted(actual_atomic[207:]) != list(range(308, 507)):
        raise ValueError("v2 atomic ID epoch is not exact P101-P506")
    assert_unique_ids(leaves)
    leaf_ids = [item["leaf_id"] for item in leaves]

    if payload["terminal_by_parent"] != EXPECTED_TERMINAL_BY_PARENT:
        raise ValueError("v2 terminal map drifted")
    terminals = {item["leaf_id"] for item in leaves if item["terminal_task_idx"]}
    if terminals != set(EXPECTED_TERMINAL_BY_PARENT.values()):
        raise ValueError("v2 must preserve exactly one legacy terminal per parent")
    terminal_ids = set(EXPECTED_TERMINAL_BY_PARENT.values())
    if any(terminal_ids.intersection(item["depends_on_leaf_ids"]) for item in appended):
        raise ValueError("new leaf depends on a legacy terminal")

    revisions = {item["parent_task_id"]: item for item in payload["completion_contract_revisions"]}
    if set(revisions) != set(EXPECTED_APPEND_PARENT_COUNTS):
        raise ValueError("completion revision parent exact-set drifted")
    base_by_id = {item["leaf_id"]: item for item in base["leaves"]}
    for parent, revision in revisions.items():
        terminal = EXPECTED_TERMINAL_BY_PARENT[parent]
        base_members = base_by_id[terminal]["depends_on_leaf_ids"]
        append_members = [
            train["steps"][-1]["leaf_id"]
            for train in ordered_trains(allocation)
            if train["parent_task_id"] == parent
        ]
        if (
            revision["terminal_leaf_id"] != terminal
            or revision["base_completion_member_leaf_ids"] != base_members
            or revision["appended_completion_member_leaf_ids"] != append_members
            or revision["revised_completion_member_leaf_ids"] != sorted(set(base_members) | set(append_members))
        ):
            raise ValueError(f"{parent} completion exact-set revision drifted")

    edges = payload["append_only_edges"]
    expected_edges = build_expected_edges(base, appended, allocation, list(revisions.values()))
    if edges != expected_edges:
        raise ValueError("v2 edge map differs from exact derived edge set")
    if len({edge_key(item) for item in edges}) != len(edges):
        raise ValueError("duplicate ordered edge in v2 mixed DAG")
    edge_map = {edge_key(item): item for item in edges}
    for base_edge in base["cross_leaf_edges"]:
        item = edge_map.get(edge_key(base_edge))
        if item is None or item["edge_kind"] != "BASE_FROZEN" or item["reason"] != base_edge["reason"]:
            raise ValueError(f"base edge drifted or omitted: {edge_key(base_edge)}")
    for item in appended:
        for source in item["depends_on_leaf_ids"]:
            if edge_map.get((source, item["leaf_id"]), {}).get("edge_kind") != "APPENDED_PREREQUISITE":
                raise ValueError(f"append prerequisite edge omitted: {source} -> {item['leaf_id']}")
    for train in ordered_trains(allocation):
        source = owner_leaf(train)
        for target in train["feeds_existing_leaf_ids"]:
            if edge_map.get((source, target), {}).get("edge_kind") != "APPENDED_TO_EXISTING":
                raise ValueError(f"append-to-existing edge omitted: {source} -> {target}")
    for revision in revisions.values():
        for source in revision["appended_completion_member_leaf_ids"]:
            if edge_map.get((source, revision["terminal_leaf_id"]), {}).get("edge_kind") != "COMPLETION_REVISION":
                raise ValueError(f"completion edge omitted: {source} -> {revision['terminal_leaf_id']}")

    activity_ids = {item["activity_id"] for item in payload["external_activities"]}
    assert_acyclic(set(leaf_ids) | activity_ids, edges)
    expected_counts = dict(sorted(Counter(item["pr_type"] for item in leaves).items()))
    expected_parents = dict(sorted(Counter(item["parent_task_id"] for item in leaves).items()))
    if payload["type_counts"] != expected_counts or payload["parent_counts"] != expected_parents:
        raise ValueError("v2 derived type or parent counts drifted")
    if payload["global_switch_gate"]["required_catalogs"] != EXPECTED_GLOBAL_CATALOGS:
        raise ValueError("v2 global four-catalog switch set drifted")
    expected_global_refs = [
        {
            "path": relative,
            "sha256": sha256(REPO / relative),
            "status": "CURRENT_ACTIVE_INPUT_NOT_V2",
        }
        for relative in EXPECTED_GLOBAL_CATALOGS
    ]
    if payload["global_switch_gate"]["legacy_catalog_refs"] != expected_global_refs:
        raise ValueError("v2 global four-catalog hash set drifted")
    global_atomic_count, legacy_m02_count, candidate_global_count = derive_global_switch_counts()
    if payload["global_switch_gate"]["legacy_active_m02_pr_count"] != legacy_m02_count:
        raise ValueError("v2 legacy active M02 count differs from global catalogs")
    if payload["global_switch_gate"]["candidate_m02_leaf_count"] != len(leaves):
        raise ValueError("v2 candidate M02 leaf count is not derived from catalog leaves")
    if payload["global_switch_gate"]["candidate_global_atomic_pr_count"] != candidate_global_count:
        raise ValueError("v2 candidate global count is not derived from active exact-set replacement")


def expect_failure(
    label: str,
    mutate: Callable[[dict[str, Any]], None],
    payload: dict[str, Any],
    expected_error: str,
) -> None:
    candidate = copy.deepcopy(payload)
    try:
        mutate(candidate)
        validate_payload(candidate)
    except (ValueError, TypeError) as exc:
        if expected_error not in str(exc):
            raise ValueError(f"mutation guard hit wrong assertion: {label}: {exc}") from exc
        return
    raise ValueError(f"mutation guard did not fail: {label}")


def expect_allocation_failure(
    label: str,
    mutate: Callable[[dict[str, Any]], None],
    *,
    enforce_semantic_pin: bool = True,
    expected_error: str,
) -> None:
    candidate = json.loads(ALLOCATION.read_text(encoding="utf-8"))
    base = json.loads(BASE.read_text(encoding="utf-8"))
    try:
        mutate(candidate)
        candidate["semantic_projection_sha256"] = allocation_semantic_hash(candidate)
        assert_allocation(candidate, base, enforce_semantic_pin=enforce_semantic_pin)
    except (ValueError, TypeError) as exc:
        if expected_error not in str(exc):
            raise ValueError(f"allocation mutation hit wrong assertion: {label}: {exc}") from exc
        return
    raise ValueError(f"allocation mutation guard did not fail: {label}")


def self_test(payload: dict[str, Any]) -> None:
    expect_failure(
        "old_leaf_drift",
        lambda item: item["leaves"][0].update({"single_outcome": "mutated"}),
        payload,
        "legacy P101-P307 leaf field drift detected",
    )
    expect_failure(
        "prior_append_prefix_drift",
        lambda item: item["leaves"][207].update({"single_outcome": "mutated prior append prefix"}),
        payload,
        "prior P308-P485 append prefix field drift detected",
    )
    expect_failure(
        "id_reuse",
        lambda item: assert_unique_ids(item["leaves"][:208] + [dict(item["leaves"][208], atomic_pr_id=item["leaves"][207]["atomic_pr_id"])] + item["leaves"][209:]),
        payload,
        "v2 leaf or atomic ID reuse detected",
    )
    expect_failure(
        "second_terminal",
        lambda item: item["leaves"][385].update({"terminal_task_idx": True}),
        payload,
        "v2 must preserve exactly one legacy terminal per parent",
    )
    expect_failure(
        "new_leaf_depends_on_terminal",
        lambda item: item["leaves"][385]["depends_on_leaf_ids"].append("M02-N006-L20"),
        payload,
        "new leaf depends on a legacy terminal",
    )
    expect_failure(
        "completion_omission",
        lambda item: item["completion_contract_revisions"][0]["appended_completion_member_leaf_ids"].pop(),
        payload,
        "completion exact-set revision drifted",
    )
    expect_failure(
        "dag_cycle",
        lambda item: assert_acyclic(
            {leaf["leaf_id"] for leaf in item["leaves"]} | {activity["activity_id"] for activity in item["external_activities"]},
            item["append_only_edges"] + [{"from": "M02-N006-L21", "to": "M02-N006-L21"}],
        ),
        payload,
        "M02 v2 mixed DAG contains a cycle",
    )
    expect_failure(
        "catalog_hash_mismatch",
        lambda item: item["allocation_ledger"].update({"sha256": "0" * 64}),
        payload,
        "v2 allocation ledger reference mismatch",
    )
    expect_failure(
        "extra_edge",
        lambda item: item["append_only_edges"].append({
            "from": "M02-N001-L01",
            "to": "M02-N006-L21",
            "reason": "unauthorized mutation edge",
            "edge_kind": "APPENDED_PREREQUISITE",
        }),
        payload,
        "v2 edge map differs from exact derived edge set",
    )
    expect_failure(
        "wrong_edge_kind",
        lambda item: item["append_only_edges"][0].update({"edge_kind": "APPENDED_PREREQUISITE"}),
        payload,
        "v2 edge map differs from exact derived edge set",
    )
    expect_failure(
        "legacy_field_list_drift",
        lambda item: item["legacy_leaf_field_freeze"].update({"field_names": [f"fake-{index}" for index in range(17)]}),
        payload,
        "legacy leaf field-name exact-set drifted",
    )
    expect_failure(
        "global_catalog_exact_set",
        lambda item: assert_global_atomic_id_sets([
            {"T1-M00-P001-REF-n001-l01"},
            {"T1-M00-P001-REF-n001-l01"},
            {"T1-M00-P001-REF-n001-l01"},
            {"T1-M00-P002-REF-n001-l02"},
        ]),
        payload,
        "global four-catalog atomic ID exact-sets differ",
    )
    expect_failure(
        "unrelated_feed",
        lambda item: next(
            edge for edge in item["append_only_edges"]
            if edge["edge_kind"] == "APPENDED_TO_EXISTING"
        ).update({"to": "M02-N002-L01"}),
        payload,
        "v2 edge map differs from exact derived edge set",
    )
    expect_allocation_failure(
        "allocation_semantic_drift",
        lambda item: item["trains"][0]["steps"][0].update({"single_outcome": "mutated semantic result"}),
        expected_error="allocation semantic projection is not the independently pinned revision",
    )
    expect_allocation_failure(
        "atomic_type_token_mismatch",
        lambda item: item["trains"][0]["steps"][0].update({
            "atomic_pr_id": item["trains"][0]["steps"][0]["atomic_pr_id"].replace("-CTR-", "-REF-")
        }),
        enforce_semantic_pin=False,
        expected_error="atomic ID type token differs from pr_type",
    )
    expect_allocation_failure(
        "parent_mismatch",
        lambda item: item["trains"][0].update({"parent_task_id": "T1-M02-N005"}),
        enforce_semantic_pin=False,
        expected_error="step parent differs from train parent",
    )
    expect_allocation_failure(
        "support_shape_drift",
        lambda item: item["trains"][0]["steps"][0].update({
            "pr_type": "WRT",
            "atomic_pr_id": item["trains"][0]["steps"][0]["atomic_pr_id"].replace("-CTR-", "-WRT-"),
        }),
        enforce_semantic_pin=False,
        expected_error="support train drifted",
    )
    expect_allocation_failure(
        "locator_reuse",
        lambda item: item["trains"][1]["steps"][0]["companion_locators"].append(
            item["trains"][0]["steps"][0]["primary_locator"]
        ),
        enforce_semantic_pin=False,
        expected_error="append allocation reuses a write locator",
    )


def verify_source_designs() -> None:
    for relative in EXPECTED_SOURCE_VALIDATORS:
        completed = subprocess.run(
            ["python3", str(REPO / relative), "--self-test"],
            cwd=REPO,
            text=True,
            stdout=subprocess.PIPE,
            stderr=subprocess.STDOUT,
            check=False,
        )
        if completed.returncode != 0:
            raise ValueError(f"source design semantic validator failed: {relative}\n{completed.stdout}")


def canonical(payload: dict[str, Any]) -> str:
    return json.dumps(payload, ensure_ascii=False, indent=2) + "\n"


def main() -> int:
    parser = argparse.ArgumentParser()
    mode = parser.add_mutually_exclusive_group(required=True)
    mode.add_argument("--write", action="store_true")
    mode.add_argument("--check", action="store_true")
    mode.add_argument("--self-test", action="store_true")
    mode.add_argument("--verify", action="store_true")
    args = parser.parse_args()
    payload = build()
    expected = canonical(payload)
    if args.write:
        OUTPUT.write_text(expected, encoding="utf-8")
        print(f"WROTE {OUTPUT.relative_to(REPO)}")
        return 0
    if args.check:
        if not OUTPUT.exists() or OUTPUT.read_text(encoding="utf-8") != expected:
            raise SystemExit(f"STALE {OUTPUT.relative_to(REPO)}; run with --write")
        print("PASS M02 v2 preview: 406 leaves, P101-P506, 199 append-only leaves, 65 support trains, frozen legacy and prior append prefix fields, exact acyclic mixed DAG")
        return 0
    if args.verify:
        if not OUTPUT.exists() or OUTPUT.read_text(encoding="utf-8") != expected:
            raise SystemExit(f"STALE {OUTPUT.relative_to(REPO)}; run with --write")
        persisted = json.loads(OUTPUT.read_text(encoding="utf-8"))
        validate_payload(persisted)
        self_test(persisted)
        verify_source_designs()
        print("PASS M02 v2 semantic verify: persisted deterministic catalog, complete mutation guards, and seven source-design semantic validators")
        return 0
    self_test(payload)
    print("PASS M02 v2 mutation guards: legacy drift, prior append prefix drift, allocation semantic drift, atomic type mismatch, ID reuse, locator reuse, support shape, second terminal, terminal dependency, completion omission, cycle, catalog hash, global exact-set, extra/wrong/unrelated edges")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
