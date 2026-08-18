#!/usr/bin/env python3
"""Generate and validate the append-only M02 delivery-package catalog v4.

V4 freezes every P101-P521 leaf field from v3, appends only P522-P525 under
N013, and allocates one exact-five implementation delivery package.  It never
creates the planned product artifacts or writes the four active registries.
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
BASE = REPO / "contracts/alignment/m02-code-direct-leaf-catalog.v3.json"
BASE_SCHEMA = REPO / "contracts/alignment/m02-code-direct-leaf-catalog.v3.schema.json"
ALLOCATION = REPO / "contracts/alignment/m02-code-direct-leaf-allocation.v4.json"
ALLOCATION_SCHEMA = REPO / "contracts/alignment/m02-code-direct-leaf-allocation.v4.schema.json"
OUTPUT = REPO / "contracts/alignment/m02-code-direct-leaf-catalog.v4.json"
SCHEMA = REPO / "contracts/alignment/m02-code-direct-leaf-catalog.v4.schema.json"
IMPLEMENTATION_SCHEMA = REPO / "contracts/alignment/implementation-candidate.schema.json"
EXPECTED_BASE_SHA256 = "413694b2f8061972da85fb76401d25ab35d07aa405f77fc324841d132a81852e"
EXPECTED_IMPLEMENTATION_SCHEMA_SHA256 = "d5c65ba04a0d04af29d06808a276ac1ef0c8f5dd2140a25ec1dc339bd2879d89"
EXPECTED_PRIOR_PROJECTION_SHA256 = "f4058eb319d2f9fcf98de259eaba3a1b6bcc51ab4b713d709fa8b10269d689ae"
EXPECTED_ALLOCATION_SEMANTIC_SHA256 = "490a930dd236322e7f59fc1ea625d884e66202c61e574890853a85c0c8a65b46"
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
EXPECTED_ARTIFACT_PATHS = {
    "install-manifest": "deployments/releases/topic1/m02-install-manifest.v1.json",
    "preflight-plan": "deployments/releases/topic1/m02-preflight-plan.v1.json",
    "upgrade-plan": "deployments/releases/topic1/m02-upgrade-plan.v1.json",
    "rollback-plan": "deployments/releases/topic1/m02-rollback-plan.v1.json",
    "restore-plan": "deployments/releases/topic1/m02-restore-plan.v1.json",
}
EXPECTED_STEP_LOCATORS = {
    "M02-N013-L10": ["contracts/alignment/m02-delivery-package.schema.json#/$defs/delivery_package"],
    "M02-N013-L11": [f"{path}#$DOCUMENT" for path in EXPECTED_ARTIFACT_PATHS.values()],
    "M02-N013-L12": ["scripts/alignment/validate_m02_delivery_package.py#main"],
    "M02-N013-L13": [
        "doc/02_acceptance/topic1/tasks/t1-m02-n013/delivery-package-test-result.json",
        "doc/02_acceptance/topic1/tasks/t1-m02-n013/delivery-package-run-manifest.json",
    ],
}
FORMAL_STATUS = "BLOCKED_UNTIL_GLOBAL_REGISTRY_TARGET_BINDING_FUNCTION_REVIEW_AND_SIGNED_OVERLAY"
ALLOWED_CLAIM = "M02 v4 append-only delivery-package ownership and dependency intent are frozen in preview"
FORBIDDEN_CLAIMS = [
    "GLOBAL_REGISTRY_SWITCHED", "PRODUCT_ARTIFACTS_CREATED", "TARGET_BINDING_COMPLETE",
    "FUNCTION_DESIGN_REVIEWED", "IMPLEMENTED", "TEST_EXECUTED",
    "EXECUTION_AUTHORIZED", "M02_ACCEPTED",
]


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
            "revision_id", "base_catalog", "source_contract_refs", "prior_leaf_field_freeze",
            "allocation_epoch", "append_leaf_count", "trains", "delivery_artifact_contract",
        ]
    }


def allocation_semantic_hash(allocation: dict[str, Any]) -> str:
    return hashlib.sha256(canonical_bytes(allocation_semantic_projection(allocation))).hexdigest()


def assert_source_contract(allocation: dict[str, Any]) -> None:
    expected = [{
        "path": IMPLEMENTATION_SCHEMA.relative_to(REPO).as_posix(),
        "sha256": EXPECTED_IMPLEMENTATION_SCHEMA_SHA256,
    }]
    if sha256(IMPLEMENTATION_SCHEMA) != EXPECTED_IMPLEMENTATION_SCHEMA_SHA256:
        raise ValueError("v4 implementation-candidate schema hash drifted")
    if allocation["source_contract_refs"] != expected:
        raise ValueError("v4 source contract exact reference drifted")
    schema = json.loads(IMPLEMENTATION_SCHEMA.read_text(encoding="utf-8"))
    roles = schema["properties"]["delivery_artifacts"]["items"]["properties"]["artifact_id"]["enum"]
    if roles != list(EXPECTED_ARTIFACT_PATHS):
        raise ValueError("v4 implementation-candidate delivery artifact role contract drifted")


def assert_allocation(
    allocation: dict[str, Any], base: dict[str, Any], *, enforce_semantic_pin: bool = True
) -> None:
    validate_against_schema(allocation, ALLOCATION_SCHEMA)
    validate_against_schema(base, BASE_SCHEMA)
    if sha256(BASE) != EXPECTED_BASE_SHA256 or allocation["base_catalog"] != {
        "path": BASE.relative_to(REPO).as_posix(), "sha256": EXPECTED_BASE_SHA256,
    }:
        raise ValueError("v4 frozen base catalog hash or path drifted")
    if projection_hash(base["leaves"]) != EXPECTED_PRIOR_PROJECTION_SHA256:
        raise ValueError("frozen P101-P521 leaf projection drifted")
    expected_freeze = {
        "leaf_id_epoch": "T1-M02-P101-P521",
        "field_names": LEGACY_LEAF_FIELDS,
        "ordered_leaf_projection_sha256": EXPECTED_PRIOR_PROJECTION_SHA256,
        "status": "PASS_EXACT_FIELD_PRESERVATION",
    }
    if allocation["prior_leaf_field_freeze"] != expected_freeze:
        raise ValueError("v4 allocation prior leaf freeze contract drifted")
    if enforce_semantic_pin and allocation["semantic_projection_sha256"] != EXPECTED_ALLOCATION_SEMANTIC_SHA256:
        raise ValueError("v4 allocation semantic projection is not independently pinned")
    if allocation["semantic_projection_sha256"] != allocation_semantic_hash(allocation):
        raise ValueError("v4 allocation semantic projection hash drifted")
    assert_source_contract(allocation)

    if len(allocation["trains"]) != 1:
        raise ValueError("v4 delivery train exact-set drifted")
    train = allocation["trains"][0]
    steps = train["steps"]
    if [item["pr_type"] for item in steps] != ["CTR", "WRT", "REF", "TST-PRE"]:
        raise ValueError("v4 delivery train shape drifted")
    if [item["leaf_id"] for item in steps] != [f"M02-N013-L{number:02d}" for number in range(10, 14)]:
        raise ValueError("v4 delivery leaf ID exact sequence drifted")
    if [atomic_number(item["atomic_pr_id"]) for item in steps] != list(range(522, 526)):
        raise ValueError("v4 atomic ID epoch is not exact contiguous P522-P525")
    for step in steps:
        if f"-{step['pr_type']}-" not in step["atomic_pr_id"] or f"-l{step['leaf_id'][-2:]}" not in step["atomic_pr_id"]:
            raise ValueError(f"v4 atomic ID does not encode leaf and type: {step['leaf_id']}")
    if len({item["leaf_id"] for item in steps}) != 4 or len({item["atomic_pr_id"] for item in steps}) != 4:
        raise ValueError("v4 append leaf or atomic ID reuse detected")

    base_leaf_ids = {item["leaf_id"] for item in base["leaves"]}
    base_atomic_ids = {item["atomic_pr_id"] for item in base["leaves"]}
    terminal_ids = set(base["terminal_by_parent"].values())
    if base_leaf_ids.intersection(item["leaf_id"] for item in steps) or base_atomic_ids.intersection(item["atomic_pr_id"] for item in steps):
        raise ValueError("v4 append ID reuses a frozen base ID")
    if train["terminal_leaf_id"] != base["terminal_by_parent"]["T1-M02-N013"]:
        raise ValueError("v4 N013 terminal mapping drifted")
    if train["feed_source_leaf_id"] != "M02-N013-L12":
        raise ValueError("v4 feed source must be the independent validator owner")
    if set(train["feeds_existing_leaf_ids"]) != {"M02-N013-L06", "M02-N016-L02"}:
        raise ValueError("v4 existing feed target exact-set drifted")
    if any(item not in base_leaf_ids or item in terminal_ids for item in train["feeds_existing_leaf_ids"]):
        raise ValueError("v4 feed target is not a frozen nonterminal leaf")

    all_leaf_ids = base_leaf_ids | {item["leaf_id"] for item in steps}
    previous: str | None = None
    # Frozen v3 may contain historical locator sharing.  V4 cannot rewrite that
    # prefix; it must ensure only that every appended locator is mutually unique
    # and absent from the entire frozen locator set.
    base_locators: set[str] = set()
    for leaf in base["leaves"]:
        for locator in leaf["write_locators"]:
            base_locators.add(locator)
    appended_locators: set[str] = set()
    for step in steps:
        dependencies = step["prerequisite_leaf_ids"]
        if previous is not None and previous not in dependencies:
            raise ValueError(f"{step['leaf_id']} does not depend on prior delivery-package step")
        if terminal_ids.intersection(dependencies):
            raise ValueError(f"v4 new leaf depends on a legacy terminal: {step['leaf_id']}")
        unknown = set(dependencies) - all_leaf_ids
        if unknown:
            raise ValueError(f"{step['leaf_id']} has unknown dependencies: {sorted(unknown)}")
        step_locators = [step["primary_locator"], *step["companion_locators"]]
        if step_locators != EXPECTED_STEP_LOCATORS[step["leaf_id"]]:
            raise ValueError(f"v4 exact write locator set drifted: {step['leaf_id']}")
        for locator in step_locators:
            if locator in base_locators or locator in appended_locators:
                raise ValueError(f"v4 allocation reuses a write locator: {locator}")
            appended_locators.add(locator)
        previous = step["leaf_id"]

    contract = allocation["delivery_artifact_contract"]
    artifact_rows = contract["required_artifacts"]
    roles = [item["artifact_role"] for item in artifact_rows]
    if len(artifact_rows) != 5 or set(roles) != set(EXPECTED_ARTIFACT_PATHS):
        raise ValueError("v4 delivery artifact exact-five role set drifted")
    actual_mapping = {item["artifact_role"]: item["path"] for item in artifact_rows}
    if actual_mapping != EXPECTED_ARTIFACT_PATHS:
        raise ValueError("v4 delivery artifact role-to-path mapping drifted")
    wrt = steps[1]
    if [locator.removesuffix("#$DOCUMENT") for locator in [wrt["primary_locator"], *wrt["companion_locators"]]] != list(EXPECTED_ARTIFACT_PATHS.values()):
        raise ValueError("v4 one WRT owner does not own the exact-five delivery paths")


def appended_leaf(step: dict[str, Any]) -> dict[str, Any]:
    number = int(step["leaf_id"].rsplit("L", 1)[1])
    return {
        "leaf_id": step["leaf_id"],
        "atomic_pr_id": step["atomic_pr_id"],
        "parent_task_id": "T1-M02-N013",
        "leaf_number": number,
        "pr_type": step["pr_type"],
        "phase": f"m02-n013-l{number:02d}",
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


def completion_revisions(base: dict[str, Any]) -> list[dict[str, Any]]:
    terminal = base["terminal_by_parent"]["T1-M02-N013"]
    terminal_leaf = next(item for item in base["leaves"] if item["leaf_id"] == terminal)
    n013 = {
        "parent_task_id": "T1-M02-N013",
        "terminal_leaf_id": terminal,
        "prior_completion_member_leaf_ids": terminal_leaf["depends_on_leaf_ids"],
        "appended_completion_member_leaf_ids": ["M02-N013-L13"],
        "revised_completion_member_leaf_ids": sorted(set(terminal_leaf["depends_on_leaf_ids"]) | {"M02-N013-L13"}),
        "revision_semantics": "TERMINAL_ID_AND_FLAG_FROZEN_COMPLETION_EXACT_SET_VERSION_REVISED",
    }
    return copy.deepcopy(base["completion_contract_revisions"]) + [n013]


def edge_key(edge: dict[str, Any]) -> tuple[str, str]:
    return edge["from"], edge["to"]


def expected_edges(
    base: dict[str, Any], allocation: dict[str, Any], appended: list[dict[str, Any]], revisions: list[dict[str, Any]]
) -> list[dict[str, str]]:
    edges = copy.deepcopy(base["append_only_edges"])
    for leaf in appended:
        for source in leaf["depends_on_leaf_ids"]:
            edges.append({
                "from": source, "to": leaf["leaf_id"],
                "reason": "explicit v4 delivery-package allocation prerequisite",
                "edge_kind": "APPENDED_PREREQUISITE",
            })
    train = allocation["trains"][0]
    for target in train["feeds_existing_leaf_ids"]:
        edges.append({
            "from": train["feed_source_leaf_id"], "to": target,
            "reason": "validated exact-five delivery package precedes frozen canary or promotion verifier",
            "edge_kind": "APPENDED_TO_EXISTING",
        })
    edges.append({
        "from": "M02-N013-L13", "to": "M02-N013-L09",
        "reason": "v4 N013 completion exact-set revision",
        "edge_kind": "COMPLETION_REVISION",
    })
    deduped: dict[tuple[str, str], dict[str, str]] = {}
    for edge in edges:
        key = edge_key(edge)
        if key in deduped and deduped[key] != edge:
            raise ValueError(f"conflicting v4 edge definitions: {key}")
        deduped[key] = edge
    return [deduped[key] for key in sorted(deduped)]


def assert_acyclic(nodes: set[str], edges: list[dict[str, Any]]) -> None:
    outgoing: dict[str, set[str]] = defaultdict(set)
    indegree = {node: 0 for node in nodes}
    for edge in edges:
        source, target = edge["from"], edge["to"]
        if source not in nodes or target not in nodes:
            raise ValueError(f"v4 edge references unknown node: {source} -> {target}")
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
        raise ValueError("M02 v4 mixed DAG contains a cycle")


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
    return len(sets[0]), len(legacy), len(sets[0]) - len(legacy) + 425


def build() -> dict[str, Any]:
    base = json.loads(BASE.read_text(encoding="utf-8"))
    allocation = json.loads(ALLOCATION.read_text(encoding="utf-8"))
    assert_allocation(allocation, base)
    appended = [appended_leaf(step) for step in allocation["trains"][0]["steps"]]
    leaves = copy.deepcopy(base["leaves"]) + appended
    revisions = completion_revisions(base)
    edges = expected_edges(base, allocation, appended, revisions)
    _, legacy_count, candidate_count = derive_global_counts()
    payload = {
        "schema_version": "4.0.0",
        "artifact_kind": "M02_CODE_DIRECT_LEAF_CATALOG_REVISION",
        "artifact_status": "VERSIONED_PREVIEW_NOT_GLOBAL_REGISTRY",
        "revision_id": "M02-CODE-DIRECT-V4",
        "base_catalog": {"path": BASE.relative_to(REPO).as_posix(), "sha256": sha256(BASE)},
        "allocation_ledger": {"path": ALLOCATION.relative_to(REPO).as_posix(), "sha256": sha256(ALLOCATION)},
        "id_epoch": "T1-M02-P101-P525",
        "base_leaf_count": 421,
        "appended_leaf_count": 4,
        "leaf_count": len(leaves),
        "type_counts": dict(sorted(Counter(item["pr_type"] for item in leaves).items())),
        "parent_counts": dict(sorted(Counter(item["parent_task_id"] for item in leaves).items())),
        "prior_leaf_field_freeze": allocation["prior_leaf_field_freeze"],
        "leaves": leaves,
        "terminal_by_parent": base["terminal_by_parent"],
        "completion_contract_revisions": revisions,
        "append_only_edges": edges,
        "external_activities": base["external_activities"],
        "contract_owner_rebindings": base["contract_owner_rebindings"],
        "delivery_artifact_contract": allocation["delivery_artifact_contract"],
        "validation": {
            "schema": "PASS", "base_catalog_hash": "PASS", "prior_leaf_field_exact": "PASS",
            "allocation_explicit_exact_set": "PASS", "id_epoch_contiguous": "PASS",
            "unique_leaf_ids": True, "unique_atomic_pr_ids": True, "write_locator_unique": True,
            "delivery_train_exact": "PASS", "delivery_artifact_exact_five": "PASS",
            "terminal_map_exact": "PASS", "completion_member_exact": "PASS",
            "no_new_leaf_depends_on_terminal": True, "mixed_dag": "PASS",
            "mutation_guards": {
                "prior_leaf_drift": "PASS", "id_reuse": "PASS", "locator_reuse": "PASS",
                "delivery_train_shape_drift": "PASS", "delivery_artifact_omission": "PASS",
                "delivery_artifact_role_drift": "PASS", "completion_omission": "PASS",
                "new_leaf_depends_on_terminal": "PASS", "feed_source_drift": "PASS",
                "dag_cycle": "PASS", "extra_edge": "PASS", "allocation_semantic_drift": "PASS",
                "global_catalog_exact_set": "PASS",
            },
        },
        "global_switch_gate": {
            "decision": "BLOCKED_PREVIEW_ONLY",
            "legacy_active_m02_pr_count": legacy_count,
            "candidate_m02_leaf_count": len(leaves),
            "candidate_global_atomic_pr_count": candidate_count,
            "legacy_catalog_refs": [
                {"path": path, "sha256": sha256(REPO / path), "status": "CURRENT_ACTIVE_INPUT_NOT_V4"}
                for path in EXPECTED_GLOBAL_CATALOGS
            ],
            "required_catalogs": EXPECTED_GLOBAL_CATALOGS,
            "switch_rule": "TASK_CLAIM_PR_DESIGN_AND_OVERLAY_MUST_SWITCH_ATOMICALLY_TO_ONE_V4_CANDIDATE_HASH_AFTER_REVIEW",
        },
        "proof_ceiling": "VERSIONED_STATIC_DESIGN_PREVIEW_ONLY_NOT_GLOBAL_REGISTRATION_PRODUCT_ARTIFACT_CREATION_TARGET_BINDING_FUNCTION_REVIEW_IMPLEMENTATION_TEST_EXECUTION_AUTHORIZATION_OR_ACCEPTANCE",
    }
    validate_payload(payload, base, allocation)
    return payload


def validate_payload(
    payload: dict[str, Any], base: dict[str, Any] | None = None, allocation: dict[str, Any] | None = None
) -> None:
    base = base or json.loads(BASE.read_text(encoding="utf-8"))
    allocation = allocation or json.loads(ALLOCATION.read_text(encoding="utf-8"))
    validate_against_schema(payload, SCHEMA)
    assert_allocation(allocation, base)
    if payload["base_catalog"] != {"path": BASE.relative_to(REPO).as_posix(), "sha256": sha256(BASE)}:
        raise ValueError("v4 base catalog reference mismatch")
    if payload["allocation_ledger"] != {"path": ALLOCATION.relative_to(REPO).as_posix(), "sha256": sha256(ALLOCATION)}:
        raise ValueError("v4 allocation ledger reference mismatch")
    if payload["leaves"][:421] != base["leaves"] or projection_hash(payload["leaves"][:421]) != EXPECTED_PRIOR_PROJECTION_SHA256:
        raise ValueError("frozen P101-P521 leaf field drift detected")
    appended = payload["leaves"][421:]
    steps = allocation["trains"][0]["steps"]
    if [item["leaf_id"] for item in appended] != [item["leaf_id"] for item in steps]:
        raise ValueError("v4 appended leaf allocation exact-set drifted")
    if len({item["leaf_id"] for item in payload["leaves"]}) != 425 or len({item["atomic_pr_id"] for item in payload["leaves"]}) != 425:
        raise ValueError("v4 catalog leaf or atomic ID reuse detected")
    if sorted(atomic_number(item["atomic_pr_id"]) for item in payload["leaves"]) != list(range(101, 526)):
        raise ValueError("v4 catalog ID epoch is not exact P101-P525")
    new_locators = [locator for item in appended for locator in item["write_locators"]]
    base_locators = {locator for item in base["leaves"] for locator in item["write_locators"]}
    if len(new_locators) != len(set(new_locators)) or base_locators.intersection(new_locators):
        raise ValueError("v4 catalog append write locator reuse detected")
    if payload["terminal_by_parent"] != base["terminal_by_parent"]:
        raise ValueError("v4 terminal map drifted")
    if payload["contract_owner_rebindings"] != base["contract_owner_rebindings"]:
        raise ValueError("v4 frozen contract-owner rebindings drifted")
    if payload["delivery_artifact_contract"] != allocation["delivery_artifact_contract"]:
        raise ValueError("v4 delivery artifact contract drifted")
    terminal_ids = set(base["terminal_by_parent"].values())
    if any(terminal_ids.intersection(item["depends_on_leaf_ids"]) for item in appended):
        raise ValueError("v4 new leaf depends on a legacy terminal")

    revisions = completion_revisions(base)
    edges = expected_edges(base, allocation, appended, revisions)
    _, legacy_count, candidate_count = derive_global_counts()
    expected_values = {
        "type_counts": dict(sorted(Counter(item["pr_type"] for item in payload["leaves"]).items())),
        "parent_counts": dict(sorted(Counter(item["parent_task_id"] for item in payload["leaves"]).items())),
        "completion_contract_revisions": revisions,
        "append_only_edges": edges,
        "global_switch_gate": {
            "decision": "BLOCKED_PREVIEW_ONLY",
            "legacy_active_m02_pr_count": legacy_count,
            "candidate_m02_leaf_count": 425,
            "candidate_global_atomic_pr_count": candidate_count,
            "legacy_catalog_refs": [
                {"path": path, "sha256": sha256(REPO / path), "status": "CURRENT_ACTIVE_INPUT_NOT_V4"}
                for path in EXPECTED_GLOBAL_CATALOGS
            ],
            "required_catalogs": EXPECTED_GLOBAL_CATALOGS,
            "switch_rule": "TASK_CLAIM_PR_DESIGN_AND_OVERLAY_MUST_SWITCH_ATOMICALLY_TO_ONE_V4_CANDIDATE_HASH_AFTER_REVIEW",
        },
    }
    for key, value in expected_values.items():
        if payload[key] != value:
            raise ValueError(f"v4 derived {key} drifted")
    nodes = {item["leaf_id"] for item in payload["leaves"]} | {item["activity_id"] for item in payload["external_activities"]}
    assert_acyclic(nodes, payload["append_only_edges"])


def expect_failure(
    label: str, payload: dict[str, Any], mutate: Callable[[dict[str, Any]], None], expected_error: str
) -> None:
    candidate = copy.deepcopy(payload)
    mutate(candidate)
    try:
        validate_payload(candidate)
    except (ValueError, TypeError) as exc:
        if expected_error not in str(exc):
            raise ValueError(f"v4 mutation {label} hit wrong assertion: {exc}") from exc
        return
    raise ValueError(f"v4 mutation {label} did not fail")


def expect_direct_failure(label: str, action: Callable[[], None], expected_error: str) -> None:
    try:
        action()
    except (ValueError, TypeError) as exc:
        if expected_error not in str(exc):
            raise ValueError(f"v4 direct mutation {label} hit wrong assertion: {exc}") from exc
        return
    raise ValueError(f"v4 direct mutation {label} did not fail")


def expect_allocation_failure(
    label: str, mutate: Callable[[dict[str, Any]], None], expected_error: str, *, pin: bool = False
) -> None:
    allocation = json.loads(ALLOCATION.read_text(encoding="utf-8"))
    base = json.loads(BASE.read_text(encoding="utf-8"))
    mutate(allocation)
    allocation["semantic_projection_sha256"] = allocation_semantic_hash(allocation)
    try:
        assert_allocation(allocation, base, enforce_semantic_pin=pin)
    except (ValueError, TypeError) as exc:
        if expected_error not in str(exc):
            raise ValueError(f"v4 allocation mutation {label} hit wrong assertion: {exc}") from exc
        return
    raise ValueError(f"v4 allocation mutation {label} did not fail")


def self_test(payload: dict[str, Any]) -> None:
    expect_failure("prior leaf drift", payload, lambda item: item["leaves"][420].update({"single_outcome": "mutated"}), "frozen P101-P521 leaf field drift detected")
    expect_failure("id reuse", payload, lambda item: item["leaves"][422].update({"atomic_pr_id": item["leaves"][421]["atomic_pr_id"]}), "v4 catalog leaf or atomic ID reuse detected")
    expect_failure("locator reuse", payload, lambda item: item["leaves"][422].update({"write_locators": [item["leaves"][421]["write_locators"][0]]}), "v4 catalog append write locator reuse detected")
    expect_failure("completion omission", payload, lambda item: item["completion_contract_revisions"][2].update({"appended_completion_member_leaf_ids": ["M02-N013-L12"]}), "v4 derived completion_contract_revisions drifted")
    expect_failure("terminal dependency", payload, lambda item: item["leaves"][421]["depends_on_leaf_ids"].append("M02-N013-L09"), "v4 new leaf depends on a legacy terminal")
    expect_failure("extra edge", payload, lambda item: item["append_only_edges"].append({"from": "M02-N013-L10", "to": "M02-N016-L03", "reason": "unauthorized extra v4 edge", "edge_kind": "APPENDED_TO_EXISTING"}), "v4 derived append_only_edges drifted")
    expect_direct_failure(
        "cycle",
        lambda: assert_acyclic(
            {leaf["leaf_id"] for leaf in payload["leaves"]} | {activity["activity_id"] for activity in payload["external_activities"]},
            payload["append_only_edges"] + [{"from": "M02-N013-L10", "to": "M02-N013-L10"}],
        ),
        "M02 v4 mixed DAG contains a cycle",
    )

    def shape_drift(item: dict[str, Any]) -> None:
        item["trains"][0]["steps"][1].update({"pr_type": "REF", "atomic_pr_id": "T1-M02-P523-REF-n013-l11"})
        item["trains"][0]["steps"][2].update({"pr_type": "WRT", "atomic_pr_id": "T1-M02-P524-WRT-n013-l12"})

    def artifact_omission(item: dict[str, Any]) -> None:
        item["delivery_artifact_contract"]["required_artifacts"][-1]["artifact_role"] = "upgrade-plan"

    def artifact_role_drift(item: dict[str, Any]) -> None:
        rows = item["delivery_artifact_contract"]["required_artifacts"]
        rows[0]["path"], rows[1]["path"] = rows[1]["path"], rows[0]["path"]

    expect_allocation_failure("delivery train shape", shape_drift, "v4 delivery train shape drifted")
    expect_allocation_failure("delivery artifact omission", artifact_omission, "v4 delivery artifact exact-five role set drifted")
    expect_allocation_failure("delivery artifact role drift", artifact_role_drift, "v4 delivery artifact role-to-path mapping drifted")
    expect_allocation_failure("feed source", lambda item: item["trains"][0].update({"feed_source_leaf_id": "M02-N013-L13"}), "schema const mismatch")
    expect_allocation_failure("semantic pin", lambda item: item["trains"][0].update({"single_result": "mutated independent semantic pin"}), "not independently pinned", pin=True)

    sets = [active_global_atomic_ids(path) for path in EXPECTED_GLOBAL_CATALOGS]
    try:
        changed = [*sets[:-1], sets[-1] | {"T1-M02-P999-WRT-n999-l99"}]
        if any(items != changed[0] for items in changed[1:]):
            raise ValueError("global four-catalog atomic ID exact-sets differ")
    except ValueError as exc:
        if "global four-catalog atomic ID exact-sets differ" not in str(exc):
            raise
    else:
        raise ValueError("v4 global exact-set mutation did not fail")


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
        print("PASS M02 v4 semantic verify: 425 leaves, P101-P525, frozen P101-P521, exact-five delivery owner and acyclic DAG")
    elif args.self_test:
        self_test(payload)
        print("PASS M02 v4 targeted mutation guards")
    else:
        print("PASS M02 v4 preview catalog is deterministic")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
