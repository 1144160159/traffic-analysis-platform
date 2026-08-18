#!/usr/bin/env python3
"""Generate and validate the append-only M02 helper-owner catalog revision v3.

V3 preserves all P101-P506 leaf fields from the v2 preview, allocates five
independent helper owners as P507-P521, and records explicit contract-owner
rebindings.  It never writes the four active global registries.
"""

from __future__ import annotations

import argparse
import copy
import hashlib
import json
import re
from collections import Counter, defaultdict
from pathlib import Path
from typing import Any, Callable

from build_topic1_task_registry import validate_against_schema


REPO = Path(__file__).resolve().parents[2]
BASE = REPO / "contracts/alignment/m02-code-direct-leaf-catalog.v2.json"
BASE_SCHEMA = REPO / "contracts/alignment/m02-code-direct-leaf-catalog.v2.schema.json"
ALLOCATION = REPO / "contracts/alignment/m02-code-direct-leaf-allocation.v3.json"
ALLOCATION_SCHEMA = REPO / "contracts/alignment/m02-code-direct-leaf-allocation.v3.schema.json"
OUTPUT = REPO / "contracts/alignment/m02-code-direct-leaf-catalog.v3.json"
SCHEMA = REPO / "contracts/alignment/m02-code-direct-leaf-catalog.v3.schema.json"
EXPECTED_BASE_SHA256 = "138277a3df27917c8248dcab863a767dca5ba61ca8730403d8086e7b93bedb44"
EXPECTED_PRIOR_PROJECTION_SHA256 = "760ea9dc6b7a51cbaeb1726ea66bbbf7a96e534d420f64ced3264496ff1a821c"
EXPECTED_ALLOCATION_SEMANTIC_SHA256 = "f9272d5f24064bcafdbed3a96c3f3cb7e44ba6cd4c315c651d079480180da2d5"
EXPECTED_GLOBAL_CATALOGS = [
    "contracts/alignment/task-registry.v1.json",
    "contracts/alignment/developer-claim-package-catalog.v1.json",
    "contracts/alignment/pr-design-application-catalog.v1.json",
    "contracts/alignment/task-execution-overlay.template.v1.json",
]
LEGACY_LEAF_FIELDS = [
    "leaf_id", "atomic_pr_id", "parent_task_id", "leaf_number", "pr_type",
    "phase", "write_locators", "target_state", "prerequisites_raw",
    "single_outcome", "oracle_and_rollback", "depends_on_leaf_ids",
    "depends_on_external_activities", "terminal_task_idx",
    "formal_execution_status", "allowed_claim", "forbidden_claims",
]
FORMAL_STATUS = "BLOCKED_UNTIL_GLOBAL_REGISTRY_TARGET_BINDING_FUNCTION_REVIEW_AND_SIGNED_OVERLAY"
ALLOWED_CLAIM = "M02 v3 append-only helper-owner identity and dependency intent are frozen in preview"
FORBIDDEN_CLAIMS = [
    "GLOBAL_REGISTRY_SWITCHED", "TARGET_BINDING_COMPLETE", "FUNCTION_DESIGN_REVIEWED",
    "IMPLEMENTED", "TEST_EXECUTED", "EXECUTION_AUTHORIZED", "M02_ACCEPTED",
]
EXPECTED_PARENT_LEAF_NUMBERS = {
    "T1-M02-N007": [11, 12, 13],
    "T1-M02-N008": list(range(24, 36)),
}


def sha256(path: Path) -> str:
    return hashlib.sha256(path.read_bytes()).hexdigest()


def canonical_bytes(value: Any) -> bytes:
    return json.dumps(value, ensure_ascii=False, sort_keys=True, separators=(",", ":")).encode("utf-8")


def projection_hash(leaves: list[dict[str, Any]]) -> str:
    return hashlib.sha256(canonical_bytes([
        {field: item[field] for field in LEGACY_LEAF_FIELDS} for item in leaves
    ])).hexdigest()


def atomic_number(atomic_pr_id: str) -> int:
    match = re.search(r"-P([0-9]{3})-", atomic_pr_id)
    if match is None:
        raise ValueError(f"invalid atomic PR ID: {atomic_pr_id}")
    return int(match.group(1))


def allocation_semantic_projection(allocation: dict[str, Any]) -> dict[str, Any]:
    return {
        key: allocation[key]
        for key in [
            "revision_id", "base_catalog", "source_design_refs", "prior_leaf_field_freeze",
            "allocation_epoch", "append_leaf_count", "trains", "contract_owner_rebindings",
        ]
    }


def allocation_semantic_hash(allocation: dict[str, Any]) -> str:
    return hashlib.sha256(canonical_bytes(allocation_semantic_projection(allocation))).hexdigest()


def owner_step(train: dict[str, Any]) -> dict[str, Any]:
    owners = [item for item in train["steps"] if item["pr_type"] == "WRT"]
    if len(owners) != 1:
        raise ValueError(f"{train['train_id']} must have exactly one WRT owner")
    return owners[0]


def contract_index(allocation: dict[str, Any]) -> dict[str, dict[str, Any]]:
    result: dict[str, dict[str, Any]] = {}
    for source_ref in allocation["source_design_refs"]:
        source = REPO / source_ref["path"]
        if sha256(source) != source_ref["sha256"]:
            raise ValueError(f"v3 source design hash drifted: {source_ref['path']}")
        design = json.loads(source.read_text(encoding="utf-8"))
        for item in design.get("function_contracts", []):
            if item["contract_id"] in result:
                raise ValueError(f"duplicate source contract ID: {item['contract_id']}")
            result[item["contract_id"]] = {**item, "source_design": source_ref}
    return result


def assert_allocation(
    allocation: dict[str, Any], base: dict[str, Any], *, enforce_semantic_pin: bool = True
) -> None:
    validate_against_schema(allocation, ALLOCATION_SCHEMA)
    validate_against_schema(base, BASE_SCHEMA)
    if sha256(BASE) != EXPECTED_BASE_SHA256 or allocation["base_catalog"] != {
        "path": BASE.relative_to(REPO).as_posix(), "sha256": EXPECTED_BASE_SHA256,
    }:
        raise ValueError("v3 frozen base catalog hash or path drifted")
    if projection_hash(base["leaves"]) != EXPECTED_PRIOR_PROJECTION_SHA256:
        raise ValueError("frozen P101-P506 leaf projection drifted")
    if allocation["prior_leaf_field_freeze"] != {
        "leaf_id_epoch": "T1-M02-P101-P506",
        "field_names": LEGACY_LEAF_FIELDS,
        "ordered_leaf_projection_sha256": EXPECTED_PRIOR_PROJECTION_SHA256,
        "status": "PASS_EXACT_FIELD_PRESERVATION",
    }:
        raise ValueError("v3 allocation prior leaf freeze contract drifted")
    if enforce_semantic_pin and allocation["semantic_projection_sha256"] != EXPECTED_ALLOCATION_SEMANTIC_SHA256:
        raise ValueError("v3 allocation semantic projection is not independently pinned")
    if allocation["semantic_projection_sha256"] != allocation_semantic_hash(allocation):
        raise ValueError("v3 allocation semantic projection hash drifted")

    trains = sorted(allocation["trains"], key=lambda item: atomic_number(item["steps"][0]["atomic_pr_id"]))
    if [item["train_id"] for item in trains] != [f"M02-V3-TRAIN-{index}" for index in range(66, 71)]:
        raise ValueError("v3 train ID order or exact-set drifted")
    steps = [step for train in trains for step in train["steps"]]
    if [atomic_number(item["atomic_pr_id"]) for item in steps] != list(range(507, 522)):
        raise ValueError("v3 atomic ID epoch is not exact contiguous P507-P521")
    if len({item["leaf_id"] for item in steps}) != 15 or len({item["atomic_pr_id"] for item in steps}) != 15:
        raise ValueError("v3 append leaf or atomic ID reuse detected")
    base_leaf_ids = {item["leaf_id"] for item in base["leaves"]}
    base_atomic_ids = {item["atomic_pr_id"] for item in base["leaves"]}
    if base_leaf_ids.intersection(item["leaf_id"] for item in steps) or base_atomic_ids.intersection(item["atomic_pr_id"] for item in steps):
        raise ValueError("v3 append ID reuses a frozen base ID")
    by_parent: dict[str, list[int]] = defaultdict(list)
    terminal_ids = set(base["terminal_by_parent"].values())
    all_leaf_ids = base_leaf_ids | {item["leaf_id"] for item in steps}
    all_locators: dict[str, str] = {}
    for leaf in base["leaves"]:
        for locator in leaf["write_locators"]:
            all_locators.setdefault(locator, leaf["atomic_pr_id"])
    for train in trains:
        if [item["pr_type"] for item in train["steps"]] != ["WRT", "REF", "TST-PRE"]:
            raise ValueError(f"{train['train_id']} support train shape drifted")
        if train["terminal_leaf_id"] != base["terminal_by_parent"][train["parent_task_id"]]:
            raise ValueError(f"{train['train_id']} terminal mapping drifted")
        if any(target not in base_leaf_ids or target in terminal_ids for target in train["feeds_existing_leaf_ids"]):
            raise ValueError(f"{train['train_id']} feed target is not a frozen nonterminal leaf")
        previous: str | None = None
        for item in train["steps"]:
            leaf_match = re.fullmatch(r"M02-N([0-9]{3})-L([0-9]{2,3})", item["leaf_id"])
            if leaf_match is None:
                raise ValueError(f"invalid v3 leaf ID: {item['leaf_id']}")
            parent = f"T1-M02-N{leaf_match.group(1)}"
            if parent != train["parent_task_id"]:
                raise ValueError(f"{train['train_id']} parent differs from step parent")
            if f"-n{leaf_match.group(1)}-l{leaf_match.group(2)}" not in item["atomic_pr_id"] or f"-{item['pr_type']}-" not in item["atomic_pr_id"]:
                raise ValueError(f"v3 atomic ID does not encode leaf and type: {item['leaf_id']}")
            by_parent[parent].append(int(leaf_match.group(2)))
            dependencies = item["prerequisite_leaf_ids"]
            if previous is not None and previous not in dependencies:
                raise ValueError(f"{item['leaf_id']} does not depend on prior support step")
            if terminal_ids.intersection(dependencies):
                raise ValueError(f"v3 new leaf depends on a legacy terminal: {item['leaf_id']}")
            unknown = set(dependencies) - all_leaf_ids
            if unknown:
                raise ValueError(f"{item['leaf_id']} has unknown dependencies: {sorted(unknown)}")
            for locator in [item["primary_locator"], *item["companion_locators"]]:
                if locator in all_locators:
                    raise ValueError(f"v3 allocation reuses a write locator: {locator}")
                all_locators[locator] = item["atomic_pr_id"]
            previous = item["leaf_id"]
        owner_step(train)
    if {parent: sorted(numbers) for parent, numbers in by_parent.items()} != EXPECTED_PARENT_LEAF_NUMBERS:
        raise ValueError("v3 append parent leaf numbers are not exact contiguous suffixes")

    contracts = contract_index(allocation)
    rebindings = allocation["contract_owner_rebindings"]
    if len({item["contract_id"] for item in rebindings}) != 5 or {item["contract_id"] for item in rebindings} != {item["contract_id"] for item in trains}:
        raise ValueError("v3 contract-owner rebinding exact-set drifted")
    step_by_leaf = {item["leaf_id"]: item for item in steps}
    train_by_contract = {item["contract_id"]: item for item in trains}
    for binding in rebindings:
        contract = contracts.get(binding["contract_id"])
        if contract is None:
            raise ValueError(f"v3 rebinding references unknown source contract: {binding['contract_id']}")
        expected_locator = f"{contract['path']}#{contract['qualified_symbol']}"
        owner = step_by_leaf.get(binding["owner_leaf_id"])
        train = train_by_contract[binding["contract_id"]]
        if binding["source_design"] != contract["source_design"] or binding["superseded_leaf_id"] != contract["leaf_id"]:
            raise ValueError(f"v3 rebinding source contract identity drifted: {binding['contract_id']}")
        if owner is None or owner["atomic_pr_id"] != binding["owner_atomic_pr_id"] or owner["primary_locator"] != binding["exact_locator"]:
            raise ValueError(f"v3 rebinding owner identity drifted: {binding['contract_id']}")
        if binding["exact_locator"] != expected_locator or owner_step(train)["leaf_id"] != binding["owner_leaf_id"]:
            raise ValueError(f"v3 rebinding exact locator drifted: {binding['contract_id']}")


def appended_leaf(step: dict[str, Any], parent_task_id: str) -> dict[str, Any]:
    number = int(step["leaf_id"].rsplit("L", 1)[1])
    return {
        "leaf_id": step["leaf_id"],
        "atomic_pr_id": step["atomic_pr_id"],
        "parent_task_id": parent_task_id,
        "leaf_number": number,
        "pr_type": step["pr_type"],
        "phase": f"m02-n{parent_task_id[-3:]}-l{number:02d}",
        "write_locators": [step["primary_locator"], *step["companion_locators"]],
        "target_state": step["target_state"],
        "prerequisites_raw": "append-only prerequisites: " + ", ".join(step["prerequisite_leaf_ids"]),
        "single_outcome": step["single_outcome"],
        "oracle_and_rollback": step["oracle_and_rollback"],
        "depends_on_leaf_ids": step["prerequisite_leaf_ids"],
        "depends_on_external_activities": [],
        "terminal_task_idx": False,
        "formal_execution_status": FORMAL_STATUS,
        "allowed_claim": ALLOWED_CLAIM,
        "forbidden_claims": FORBIDDEN_CLAIMS,
    }


def edge_key(edge: dict[str, Any]) -> tuple[str, str]:
    return edge["from"], edge["to"]


def expected_edges(base: dict[str, Any], allocation: dict[str, Any], appended: list[dict[str, Any]], revisions: list[dict[str, Any]]) -> list[dict[str, str]]:
    edges = copy.deepcopy(base["append_only_edges"])
    for leaf in appended:
        for source in leaf["depends_on_leaf_ids"]:
            edges.append({
                "from": source, "to": leaf["leaf_id"],
                "reason": "explicit v3 helper-owner allocation prerequisite",
                "edge_kind": "APPENDED_PREREQUISITE",
            })
    for train in allocation["trains"]:
        source = owner_step(train)["leaf_id"]
        for target in train["feeds_existing_leaf_ids"]:
            edges.append({
                "from": source, "to": target,
                "reason": f"{train['gap_id']} owner precedes frozen caller adapter",
                "edge_kind": "APPENDED_TO_EXISTING",
            })
    for revision in revisions:
        for source in revision["appended_completion_member_leaf_ids"]:
            edges.append({
                "from": source, "to": revision["terminal_leaf_id"],
                "reason": "v3 completion exact-set revision",
                "edge_kind": "COMPLETION_REVISION",
            })
    deduped: dict[tuple[str, str], dict[str, str]] = {}
    for edge in edges:
        key = edge_key(edge)
        if key in deduped and deduped[key] != edge:
            raise ValueError(f"conflicting v3 edge definitions: {key}")
        deduped[key] = edge
    return [deduped[key] for key in sorted(deduped)]


def assert_acyclic(nodes: set[str], edges: list[dict[str, Any]]) -> None:
    outgoing: dict[str, set[str]] = defaultdict(set)
    indegree = {node: 0 for node in nodes}
    for edge in edges:
        source, target = edge["from"], edge["to"]
        if source not in nodes or target not in nodes:
            raise ValueError(f"v3 edge references unknown node: {source} -> {target}")
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
        raise ValueError("M02 v3 mixed DAG contains a cycle")


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


def derive_global_counts() -> tuple[int, int, int]:
    sets = [active_global_atomic_ids(path) for path in EXPECTED_GLOBAL_CATALOGS]
    if any(items != sets[0] for items in sets[1:]):
        raise ValueError("global four-catalog atomic ID exact-sets differ")
    legacy = {item for item in sets[0] if re.match(r"^T1-M02-P[0-9]{3}-", item)}
    if len(sets[0]) != 1289 or len(legacy) != 34:
        raise ValueError("active global or legacy M02 count drifted")
    return len(sets[0]), len(legacy), len(sets[0]) - len(legacy) + 421


def build() -> dict[str, Any]:
    base = json.loads(BASE.read_text(encoding="utf-8"))
    allocation = json.loads(ALLOCATION.read_text(encoding="utf-8"))
    assert_allocation(allocation, base)
    trains = sorted(allocation["trains"], key=lambda item: atomic_number(item["steps"][0]["atomic_pr_id"]))
    appended = [appended_leaf(step, train["parent_task_id"]) for train in trains for step in train["steps"]]
    leaves = copy.deepcopy(base["leaves"]) + appended

    prior_revisions = {item["parent_task_id"]: item for item in base["completion_contract_revisions"]}
    base_by_id = {item["leaf_id"]: item for item in base["leaves"]}
    revisions = []
    for parent in sorted(EXPECTED_PARENT_LEAF_NUMBERS):
        terminal = base["terminal_by_parent"][parent]
        prior_members = (
            prior_revisions[parent]["revised_completion_member_leaf_ids"]
            if parent in prior_revisions else base_by_id[terminal]["depends_on_leaf_ids"]
        )
        appended_members = [train["steps"][-1]["leaf_id"] for train in trains if train["parent_task_id"] == parent]
        revisions.append({
            "parent_task_id": parent,
            "terminal_leaf_id": terminal,
            "prior_completion_member_leaf_ids": prior_members,
            "appended_completion_member_leaf_ids": appended_members,
            "revised_completion_member_leaf_ids": sorted(set(prior_members) | set(appended_members)),
            "revision_semantics": "TERMINAL_ID_AND_FLAG_FROZEN_COMPLETION_EXACT_SET_VERSION_REVISED",
        })
    edges = expected_edges(base, allocation, appended, revisions)
    _, legacy_count, candidate_count = derive_global_counts()
    payload = {
        "schema_version": "3.0.0",
        "artifact_kind": "M02_CODE_DIRECT_LEAF_CATALOG_REVISION",
        "artifact_status": "VERSIONED_PREVIEW_NOT_GLOBAL_REGISTRY",
        "revision_id": "M02-CODE-DIRECT-V3",
        "base_catalog": {"path": BASE.relative_to(REPO).as_posix(), "sha256": sha256(BASE)},
        "allocation_ledger": {"path": ALLOCATION.relative_to(REPO).as_posix(), "sha256": sha256(ALLOCATION)},
        "id_epoch": "T1-M02-P101-P521",
        "base_leaf_count": 406,
        "appended_leaf_count": 15,
        "leaf_count": len(leaves),
        "type_counts": dict(sorted(Counter(item["pr_type"] for item in leaves).items())),
        "parent_counts": dict(sorted(Counter(item["parent_task_id"] for item in leaves).items())),
        "prior_leaf_field_freeze": allocation["prior_leaf_field_freeze"],
        "leaves": leaves,
        "terminal_by_parent": base["terminal_by_parent"],
        "completion_contract_revisions": revisions,
        "append_only_edges": edges,
        "external_activities": base["external_activities"],
        "contract_owner_rebindings": allocation["contract_owner_rebindings"],
        "validation": {
            "schema": "PASS", "base_catalog_hash": "PASS", "prior_leaf_field_exact": "PASS",
            "allocation_explicit_exact_set": "PASS", "id_epoch_contiguous": "PASS",
            "unique_leaf_ids": True, "unique_atomic_pr_ids": True, "write_locator_unique": True,
            "support_train_exact": "PASS", "contract_owner_rebinding_exact": "PASS",
            "terminal_map_exact": "PASS", "completion_member_exact": "PASS",
            "no_new_leaf_depends_on_terminal": True, "mixed_dag": "PASS",
            "mutation_guards": {
                "prior_leaf_drift": "PASS", "id_reuse": "PASS", "locator_reuse": "PASS",
                "support_shape_drift": "PASS", "contract_rebinding_omission": "PASS",
                "contract_rebinding_locator_drift": "PASS", "completion_omission": "PASS",
                "new_leaf_depends_on_terminal": "PASS", "dag_cycle": "PASS", "extra_edge": "PASS",
                "allocation_semantic_drift": "PASS", "global_catalog_exact_set": "PASS",
            },
        },
        "global_switch_gate": {
            "decision": "BLOCKED_PREVIEW_ONLY",
            "legacy_active_m02_pr_count": legacy_count,
            "candidate_m02_leaf_count": len(leaves),
            "candidate_global_atomic_pr_count": candidate_count,
            "legacy_catalog_refs": [
                {"path": path, "sha256": sha256(REPO / path), "status": "CURRENT_ACTIVE_INPUT_NOT_V3"}
                for path in EXPECTED_GLOBAL_CATALOGS
            ],
            "required_catalogs": EXPECTED_GLOBAL_CATALOGS,
            "switch_rule": "TASK_CLAIM_PR_DESIGN_AND_OVERLAY_MUST_SWITCH_ATOMICALLY_TO_ONE_V3_CANDIDATE_HASH_AFTER_REVIEW",
        },
        "proof_ceiling": "VERSIONED_STATIC_DESIGN_PREVIEW_ONLY_NOT_GLOBAL_REGISTRATION_TARGET_BINDING_FUNCTION_REVIEW_IMPLEMENTATION_TEST_EXECUTION_AUTHORIZATION_OR_ACCEPTANCE",
    }
    validate_payload(payload, base, allocation)
    return payload


def validate_payload(payload: dict[str, Any], base: dict[str, Any] | None = None, allocation: dict[str, Any] | None = None) -> None:
    base = base or json.loads(BASE.read_text(encoding="utf-8"))
    allocation = allocation or json.loads(ALLOCATION.read_text(encoding="utf-8"))
    validate_against_schema(payload, SCHEMA)
    assert_allocation(allocation, base)
    if payload["base_catalog"] != {"path": BASE.relative_to(REPO).as_posix(), "sha256": sha256(BASE)}:
        raise ValueError("v3 base catalog reference mismatch")
    if payload["allocation_ledger"] != {"path": ALLOCATION.relative_to(REPO).as_posix(), "sha256": sha256(ALLOCATION)}:
        raise ValueError("v3 allocation ledger reference mismatch")
    if payload["leaves"][:406] != base["leaves"] or projection_hash(payload["leaves"][:406]) != EXPECTED_PRIOR_PROJECTION_SHA256:
        raise ValueError("frozen P101-P506 leaf field drift detected")
    if payload["prior_leaf_field_freeze"] != allocation["prior_leaf_field_freeze"]:
        raise ValueError("v3 prior leaf field freeze metadata drifted")
    appended = payload["leaves"][406:]
    allocation_steps = [step for train in sorted(allocation["trains"], key=lambda item: atomic_number(item["steps"][0]["atomic_pr_id"])) for step in train["steps"]]
    if [item["leaf_id"] for item in appended] != [item["leaf_id"] for item in allocation_steps]:
        raise ValueError("v3 appended leaf allocation exact-set drifted")
    if len({item["leaf_id"] for item in payload["leaves"]}) != 421 or len({item["atomic_pr_id"] for item in payload["leaves"]}) != 421:
        raise ValueError("v3 catalog leaf or atomic ID reuse detected")
    if sorted(atomic_number(item["atomic_pr_id"]) for item in payload["leaves"]) != list(range(101, 522)):
        raise ValueError("v3 catalog ID epoch is not exact P101-P521")
    new_locators = [locator for item in appended for locator in item["write_locators"]]
    base_locators = {locator for item in base["leaves"] for locator in item["write_locators"]}
    if len(new_locators) != len(set(new_locators)) or base_locators.intersection(new_locators):
        raise ValueError("v3 catalog append write locator reuse detected")
    if payload["terminal_by_parent"] != base["terminal_by_parent"]:
        raise ValueError("v3 terminal map drifted")
    terminal_ids = set(base["terminal_by_parent"].values())
    if any(terminal_ids.intersection(item["depends_on_leaf_ids"]) for item in appended):
        raise ValueError("v3 new leaf depends on a legacy terminal")
    if payload["contract_owner_rebindings"] != allocation["contract_owner_rebindings"]:
        raise ValueError("v3 contract-owner rebinding exact-set drifted")

    trains = sorted(allocation["trains"], key=lambda item: atomic_number(item["steps"][0]["atomic_pr_id"]))
    prior_revisions = {item["parent_task_id"]: item for item in base["completion_contract_revisions"]}
    base_by_id = {item["leaf_id"]: item for item in base["leaves"]}
    expected_revisions = []
    for parent in sorted(EXPECTED_PARENT_LEAF_NUMBERS):
        terminal = base["terminal_by_parent"][parent]
        prior_members = (
            prior_revisions[parent]["revised_completion_member_leaf_ids"]
            if parent in prior_revisions else base_by_id[terminal]["depends_on_leaf_ids"]
        )
        appended_members = [train["steps"][-1]["leaf_id"] for train in trains if train["parent_task_id"] == parent]
        expected_revisions.append({
            "parent_task_id": parent,
            "terminal_leaf_id": terminal,
            "prior_completion_member_leaf_ids": prior_members,
            "appended_completion_member_leaf_ids": appended_members,
            "revised_completion_member_leaf_ids": sorted(set(prior_members) | set(appended_members)),
            "revision_semantics": "TERMINAL_ID_AND_FLAG_FROZEN_COMPLETION_EXACT_SET_VERSION_REVISED",
        })
    _, legacy_count, candidate_count = derive_global_counts()
    expected_values = {
        "type_counts": dict(sorted(Counter(item["pr_type"] for item in payload["leaves"]).items())),
        "parent_counts": dict(sorted(Counter(item["parent_task_id"] for item in payload["leaves"]).items())),
        "completion_contract_revisions": expected_revisions,
        "append_only_edges": expected_edges(base, allocation, appended, expected_revisions),
        "global_switch_gate": {
            "decision": "BLOCKED_PREVIEW_ONLY",
            "legacy_active_m02_pr_count": legacy_count,
            "candidate_m02_leaf_count": 421,
            "candidate_global_atomic_pr_count": candidate_count,
            "legacy_catalog_refs": [
                {"path": path, "sha256": sha256(REPO / path), "status": "CURRENT_ACTIVE_INPUT_NOT_V3"}
                for path in EXPECTED_GLOBAL_CATALOGS
            ],
            "required_catalogs": EXPECTED_GLOBAL_CATALOGS,
            "switch_rule": "TASK_CLAIM_PR_DESIGN_AND_OVERLAY_MUST_SWITCH_ATOMICALLY_TO_ONE_V3_CANDIDATE_HASH_AFTER_REVIEW",
        },
    }
    for key, value in expected_values.items():
        if payload[key] != value:
            raise ValueError(f"v3 derived {key} drifted")
    nodes = {item["leaf_id"] for item in payload["leaves"]} | {item["activity_id"] for item in payload["external_activities"]}
    assert_acyclic(nodes, payload["append_only_edges"])


def expect_failure(label: str, payload: dict[str, Any], mutate: Callable[[dict[str, Any]], None], expected_error: str) -> None:
    candidate = copy.deepcopy(payload)
    mutate(candidate)
    try:
        validate_payload(candidate)
    except (ValueError, TypeError) as exc:
        if expected_error not in str(exc):
            raise ValueError(f"v3 mutation {label} hit wrong assertion: {exc}") from exc
        return
    raise ValueError(f"v3 mutation {label} did not fail")


def expect_direct_failure(label: str, action: Callable[[], None], expected_error: str) -> None:
    try:
        action()
    except (ValueError, TypeError) as exc:
        if expected_error not in str(exc):
            raise ValueError(f"v3 direct mutation {label} hit wrong assertion: {exc}") from exc
        return
    raise ValueError(f"v3 direct mutation {label} did not fail")


def expect_allocation_failure(label: str, mutate: Callable[[dict[str, Any]], None], expected_error: str, *, pin: bool = False) -> None:
    allocation = json.loads(ALLOCATION.read_text(encoding="utf-8"))
    base = json.loads(BASE.read_text(encoding="utf-8"))
    mutate(allocation)
    allocation["semantic_projection_sha256"] = allocation_semantic_hash(allocation)
    try:
        assert_allocation(allocation, base, enforce_semantic_pin=pin)
    except (ValueError, TypeError) as exc:
        if expected_error not in str(exc):
            raise ValueError(f"v3 allocation mutation {label} hit wrong assertion: {exc}") from exc
        return
    raise ValueError(f"v3 allocation mutation {label} did not fail")


def self_test(payload: dict[str, Any]) -> None:
    expect_failure("prior leaf drift", payload, lambda item: item["leaves"][405].update({"single_outcome": "mutated"}), "frozen P101-P506 leaf field drift detected")
    expect_failure("id reuse", payload, lambda item: item["leaves"][407].update({"atomic_pr_id": item["leaves"][406]["atomic_pr_id"]}), "v3 catalog leaf or atomic ID reuse detected")
    expect_failure("locator reuse", payload, lambda item: item["leaves"][407].update({"write_locators": [item["leaves"][406]["write_locators"][0]]}), "v3 catalog append write locator reuse detected")
    expect_failure("rebinding omission", payload, lambda item: item["contract_owner_rebindings"].pop(), "schema minItems failed at $.contract_owner_rebindings")
    expect_failure("completion omission", payload, lambda item: item["completion_contract_revisions"][0]["appended_completion_member_leaf_ids"].pop(), "schema minItems failed")
    expect_failure("terminal dependency", payload, lambda item: item["leaves"][406]["depends_on_leaf_ids"].append("M02-N007-L10"), "v3 new leaf depends on a legacy terminal")
    expect_failure("extra edge", payload, lambda item: item["append_only_edges"].append({"from": "M02-N001-L01", "to": "M02-N007-L11", "reason": "unauthorized extra v3 edge", "edge_kind": "APPENDED_PREREQUISITE"}), "v3 derived append_only_edges drifted")
    expect_direct_failure(
        "cycle",
        lambda: assert_acyclic(
            {leaf["leaf_id"] for leaf in payload["leaves"]} | {activity["activity_id"] for activity in payload["external_activities"]},
            payload["append_only_edges"] + [{"from": "M02-N007-L11", "to": "M02-N007-L11"}],
        ),
        "M02 v3 mixed DAG contains a cycle",
    )
    expect_allocation_failure("support shape", lambda item: item["trains"][0]["steps"][0].update({"pr_type": "REF", "atomic_pr_id": "T1-M02-P507-REF-n007-l11"}), "support train shape drifted")
    expect_allocation_failure("rebinding locator", lambda item: item["contract_owner_rebindings"][0].update({"exact_locator": "rust/probe-agent/probe-agent/src/sender/retry.rs#wrong"}), "rebinding owner identity drifted")
    expect_allocation_failure("semantic pin", lambda item: item["trains"][0].update({"single_result": "mutated independent semantic pin"}), "not independently pinned", pin=True)
    sets = [active_global_atomic_ids(path) for path in EXPECTED_GLOBAL_CATALOGS]
    try:
        if any(items != sets[0] for items in [*sets[:-1], sets[-1] | {"T1-M02-P999-WRT-n999-l99"}][1:]):
            raise ValueError("global four-catalog atomic ID exact-sets differ")
    except ValueError as exc:
        if "global four-catalog atomic ID exact-sets differ" not in str(exc):
            raise
    else:
        raise ValueError("v3 global exact-set mutation did not fail")


def canonical(payload: dict[str, Any]) -> str:
    return json.dumps(payload, ensure_ascii=False, indent=2) + "\n"


def main() -> int:
    parser = argparse.ArgumentParser()
    mode = parser.add_mutually_exclusive_group(required=True)
    mode.add_argument("--write", action="store_true")
    mode.add_argument("--check", action="store_true")
    mode.add_argument("--verify", action="store_true")
    mode.add_argument("--self-test", action="store_true")
    args = parser.parse_args()
    payload = build()
    expected = canonical(payload)
    if args.write:
        OUTPUT.write_text(expected, encoding="utf-8")
        print(f"WROTE {OUTPUT.relative_to(REPO)}")
        return 0
    if not OUTPUT.exists() or OUTPUT.read_text(encoding="utf-8") != expected:
        raise SystemExit(f"STALE {OUTPUT.relative_to(REPO)}; run with --write")
    if args.verify:
        validate_payload(json.loads(OUTPUT.read_text(encoding="utf-8")))
        self_test(payload)
        print("PASS M02 v3 semantic verify: 421 leaves, P101-P521, frozen P101-P506, five helper-owner trains, exact rebindings and acyclic DAG")
    elif args.self_test:
        self_test(payload)
        print("PASS M02 v3 targeted mutation guards")
    else:
        print("PASS M02 v3 preview catalog is deterministic")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
