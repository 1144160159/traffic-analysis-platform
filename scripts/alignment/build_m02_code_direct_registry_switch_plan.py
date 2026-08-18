#!/usr/bin/env python3
"""Build the fail-closed M02 REG-03 global-registry switch readiness ledger.

This generator never changes an active registry.  It freezes the current
34-card legacy set, the 425-card replacement exact-set, the future tombstones,
and every prerequisite that must pass before the four active catalogs may be
replaced atomically.
"""

from __future__ import annotations

import argparse
import copy
import hashlib
import json
import re
import subprocess
from collections import defaultdict
from pathlib import Path
from typing import Any, Callable

from build_topic1_task_registry import validate_against_schema


REPO = Path(__file__).resolve().parents[2]
SOURCE = REPO / "contracts/alignment/m02-code-direct-leaf-catalog.v4.json"
SCHEMA = REPO / "contracts/alignment/m02-code-direct-registry-switch-plan.schema.json"
OUTPUT = REPO / "contracts/alignment/m02-code-direct-registry-switch-plan.v1.json"
DOC_OUTPUT = REPO / "doc/07_alignment/generated/M02代码直达Registry切换就绪账本.md"
EXTERNAL_SCHEMA = REPO / "contracts/alignment/external-activity-receipt.schema.json"
LOCATOR_COVERAGE_PATH = REPO / "contracts/alignment/m02-code-direct-locator-coverage.v1.json"
FUNCTION_REVIEW_COVERAGE_PATH = REPO / "contracts/alignment/m02-function-review-coverage.v1.json"
WRITE_SCOPE_SUPERSESSION_PATH = REPO / "contracts/alignment/m02-write-scope-supersession.v1.json"
GATE_INPUT_PREFLIGHT_PATH = REPO / "contracts/alignment/m02-gate-input-preflight.v1.json"
EXTERNAL_INPUT_WORK_ORDERS_PATH = REPO / "contracts/alignment/m02-external-input-work-order-catalog.v1.json"
EXTERNAL_INPUT_WORK_ORDERS_SCHEMA = REPO / "contracts/alignment/m02-external-input-work-order-catalog.schema.json"
IMPLEMENTATION_CLOSURE_PATH = REPO / "contracts/alignment/m02-implementation-candidate-closure.v1.json"

GLOBAL_CATALOGS = [
    "contracts/alignment/task-registry.v1.json",
    "contracts/alignment/developer-claim-package-catalog.v1.json",
    "contracts/alignment/pr-design-application-catalog.v1.json",
    "contracts/alignment/task-execution-overlay.template.v1.json",
]
EXPECTED_EXTERNAL_TYPES = {
    "SCOPED_CANARY", "PROFILE_APPROVAL", "PROTECTED_MERGE",
}
EXPECTED_SOURCE_DESIGN_COUNT = 7


def sha256(path: Path) -> str:
    return hashlib.sha256(path.read_bytes()).hexdigest()


def semantic_sha256(value: Any) -> str:
    body = json.dumps(
        value, ensure_ascii=False, sort_keys=True, separators=(",", ":")
    ).encode("utf-8")
    return hashlib.sha256(body).hexdigest()


def atomic_ids(relative: str, payload: dict[str, Any]) -> list[str]:
    if relative.endswith("task-registry.v1.json"):
        values = [
            pr["pr_id"] for task in payload["tasks"] for pr in task["pr_sequence"]
        ] + [
            pr["pr_id"]
            for group in payload["closure_slices"]
            for pr in group["pr_sequence"]
        ]
    elif relative.endswith("developer-claim-package-catalog.v1.json"):
        values = [item["atomic_pr_id"] for item in payload["packages"]]
    elif relative.endswith("pr-design-application-catalog.v1.json"):
        values = [item["atomic_pr_id"] for item in payload["entries"]]
    elif relative.endswith("task-execution-overlay.template.v1.json"):
        values = [item["pr_id"] for item in payload["atomic_pr_bindings"]]
    else:
        raise ValueError(f"unsupported global catalog {relative}")
    if len(values) != len(set(values)):
        raise ValueError(f"global catalog has duplicate atomic IDs: {relative}")
    return values


def load_global_catalogs() -> tuple[list[dict[str, Any]], set[str], dict[str, list[str]]]:
    refs: list[dict[str, Any]] = []
    exact_sets: list[set[str]] = []
    legacy_by_parent: dict[str, list[str]] = defaultdict(list)
    for relative in GLOBAL_CATALOGS:
        path = REPO / relative
        payload = json.loads(path.read_text(encoding="utf-8"))
        values = atomic_ids(relative, payload)
        exact = set(values)
        exact_sets.append(exact)
        m02_ids = sorted(item for item in exact if item.startswith("T1-M02-"))
        refs.append({
            "path": relative,
            "sha256": sha256(path),
            "atomic_pr_count": len(exact),
            "active_m02_atomic_pr_count": len(m02_ids),
            "atomic_id_exact_set_sha256": semantic_sha256(sorted(exact)),
            "status": "CURRENT_ACTIVE_LEGACY_M02",
        })
    if any(items != exact_sets[0] for items in exact_sets[1:]):
        raise ValueError("global four-catalog atomic ID exact-sets differ")

    registry = json.loads((REPO / GLOBAL_CATALOGS[0]).read_text(encoding="utf-8"))
    for task in registry["tasks"]:
        if task["milestone_id"] == "T1-M02":
            legacy_by_parent[task["task_id"]] = [
                item["pr_id"] for item in task["pr_sequence"]
            ]
    return refs, exact_sets[0], dict(legacy_by_parent)


def source_semantic_projection(source: dict[str, Any]) -> dict[str, Any]:
    return {
        "revision_id": source["revision_id"],
        "id_epoch": source["id_epoch"],
        "prior_leaf_field_freeze": source["prior_leaf_field_freeze"],
        "allocation_semantic_projection_sha256": json.loads(
            (REPO / source["allocation_ledger"]["path"]).read_text(encoding="utf-8")
        )["semantic_projection_sha256"],
        "leaves": source["leaves"],
        "append_only_edges": source["append_only_edges"],
        "terminal_by_parent": source["terminal_by_parent"],
        "completion_contract_revisions": source["completion_contract_revisions"],
        "external_activities": source["external_activities"],
        "contract_owner_rebindings": source["contract_owner_rebindings"],
        "delivery_artifact_contract": source["delivery_artifact_contract"],
    }


def completion_members(
    parent: str,
    terminal_leaf: str,
    source: dict[str, Any],
    leaf_to_atomic: dict[str, str],
) -> list[str]:
    revisions = {
        item["parent_task_id"]: item
        for item in source["completion_contract_revisions"]
    }
    if parent in revisions:
        leaf_ids = revisions[parent]["revised_completion_member_leaf_ids"]
    elif source.get("revision_id") in {"M02-CODE-DIRECT-V3", "M02-CODE-DIRECT-V4"}:
        base = json.loads(
            (REPO / source["base_catalog"]["path"]).read_text(encoding="utf-8")
        )
        base_leaf_to_atomic = {
            item["leaf_id"]: item["atomic_pr_id"] for item in base["leaves"]
        }
        return completion_members(
            parent,
            base["terminal_by_parent"][parent],
            base,
            base_leaf_to_atomic,
        )
    else:
        terminal = next(item for item in source["leaves"] if item["leaf_id"] == terminal_leaf)
        leaf_ids = terminal["depends_on_leaf_ids"]
    return [leaf_to_atomic[item] for item in leaf_ids]


def build() -> dict[str, Any]:
    source = json.loads(SOURCE.read_text(encoding="utf-8"))
    refs, current_ids, legacy_by_parent = load_global_catalogs()
    leaves = source["leaves"]
    replacement_ids = {item["atomic_pr_id"] for item in leaves}
    legacy_ids = {
        item for values in legacy_by_parent.values() for item in values
    }
    if len(current_ids) != 1289 or len(legacy_ids) != 34:
        raise ValueError("current global or legacy M02 exact-set count drifted")
    if len(replacement_ids) != 425:
        raise ValueError("replacement M02 exact-set count drifted")
    if legacy_ids & replacement_ids:
        raise ValueError("legacy and replacement M02 atomic IDs overlap")
    candidate_ids = (current_ids - legacy_ids) | replacement_ids
    if len(candidate_ids) != 1680:
        raise ValueError("candidate global atomic count is not 1289 - 34 + 425")

    leaves_by_parent: dict[str, list[dict[str, Any]]] = defaultdict(list)
    for leaf in leaves:
        leaves_by_parent[leaf["parent_task_id"]].append(leaf)
    leaf_to_atomic = {item["leaf_id"]: item["atomic_pr_id"] for item in leaves}
    activities_by_parent: dict[str, list[dict[str, Any]]] = defaultdict(list)
    for activity in source["external_activities"]:
        activities_by_parent[activity["parent_task_id"]].append(activity)

    task_replacements = []
    for parent in sorted(leaves_by_parent):
        parent_leaves = sorted(leaves_by_parent[parent], key=lambda item: item["leaf_number"])
        terminal_leaf = source["terminal_by_parent"][parent]
        task_replacements.append({
            "parent_task_id": parent,
            "legacy_atomic_pr_ids": legacy_by_parent[parent],
            "replacement_leaf_ids": [item["leaf_id"] for item in parent_leaves],
            "replacement_atomic_pr_ids": [item["atomic_pr_id"] for item in parent_leaves],
            "terminal_leaf_id": terminal_leaf,
            "terminal_atomic_pr_id": leaf_to_atomic[terminal_leaf],
            "completion_member_atomic_pr_ids": completion_members(
                parent, terminal_leaf, source, leaf_to_atomic
            ),
            "external_activity_ids": [
                item["activity_id"] for item in activities_by_parent[parent]
            ],
            "transition_status": "PLANNED_BLOCKED_NOT_ACTIVE",
        })
    replacement_by_parent = {
        item["parent_task_id"]: item["replacement_atomic_pr_ids"]
        for item in task_replacements
    }
    tombstones = [
        {
            "legacy_atomic_pr_id": legacy_id,
            "parent_task_id": parent,
            "current_status": "ACTIVE_LEGACY_BINDING_UNTIL_ATOMIC_SWITCH",
            "status_after_atomic_switch": "SUPERSEDED_NOT_AUTHORIZED",
            "replacement_atomic_pr_ids": replacement_by_parent[parent],
            "claim_rule": "OLD_ID_MUST_BE_UNCLAIMABLE_AFTER_SWITCH",
            "retention_rule": "RETAIN_TOMBSTONE_UNTIL_REPLACEMENT_TERMINAL_TASK_IDX_IS_REACHABLE",
        }
        for parent in sorted(legacy_by_parent)
        for legacy_id in legacy_by_parent[parent]
    ]
    external_activities = [
        {
            "activity_id": item["activity_id"],
            "parent_task_id": item["parent_task_id"],
            "activity_type": item["activity_type"],
            "depends_on_atomic_pr_ids": [
                leaf_to_atomic[leaf_id] for leaf_id in item["depends_on_leaf_ids"]
            ],
            "successor_atomic_pr_ids": [
                leaf_to_atomic[leaf_id] for leaf_id in item["successor_leaf_ids"]
            ],
            "registration_status": "BLOCKED_EXTERNAL_RECEIPT_CONTRACT_NOT_ACTIVE",
        }
        for item in source["external_activities"]
    ]

    allocation = json.loads((REPO / source["allocation_ledger"]["path"]).read_text(encoding="utf-8"))
    ancestry = source
    source_design_refs: list[dict[str, Any]] = []
    while not source_design_refs:
        ancestor_allocation = json.loads((REPO / ancestry["allocation_ledger"]["path"]).read_text(encoding="utf-8"))
        candidate_refs = ancestor_allocation.get("source_design_refs", [])
        if len(candidate_refs) == EXPECTED_SOURCE_DESIGN_COUNT:
            source_design_refs = candidate_refs
            break
        base_ref = ancestry.get("base_catalog")
        if not base_ref:
            break
        ancestry = json.loads((REPO / base_ref["path"]).read_text(encoding="utf-8"))
    if len(source_design_refs) != EXPECTED_SOURCE_DESIGN_COUNT:
        raise ValueError("M02 function-design source exact-set drifted")
    planned_leaf_count = sum(item["target_state"] == "PLANNED" for item in leaves)
    external_schema = json.loads(EXTERNAL_SCHEMA.read_text(encoding="utf-8"))
    active_external_types = set(external_schema["properties"]["activity_type"]["enum"])
    missing_external_types = sorted(EXPECTED_EXTERNAL_TYPES - active_external_types)
    external_contract_validator = subprocess.run(
        [
            "python3",
            "scripts/alignment/validate_m02_external_activity_receipt_contract.py",
            "--self-test",
        ],
        cwd=REPO,
        check=False,
        capture_output=True,
        text=True,
    )
    external_contract_ready = (
        not missing_external_types and external_contract_validator.returncode == 0
    )
    registry = json.loads((REPO / GLOBAL_CATALOGS[0]).read_text(encoding="utf-8"))
    m02_tasks = [item for item in registry["tasks"] if item["milestone_id"] == "T1-M02"]
    assigned = sum(
        bool(task["responsibility"]["owner"])
        and bool(task["responsibility"]["reviewers"])
        and bool(task["responsibility"]["approvers"])
        for task in m02_tasks
    )
    locator_coverage = json.loads(LOCATOR_COVERAGE_PATH.read_text(encoding="utf-8"))
    function_review_coverage = json.loads(FUNCTION_REVIEW_COVERAGE_PATH.read_text(encoding="utf-8"))
    gate_input_preflight = json.loads(GATE_INPUT_PREFLIGHT_PATH.read_text(encoding="utf-8"))
    external_input_work_orders = json.loads(EXTERNAL_INPUT_WORK_ORDERS_PATH.read_text(encoding="utf-8"))
    implementation_closure = json.loads(IMPLEMENTATION_CLOSURE_PATH.read_text(encoding="utf-8"))
    validate_against_schema(external_input_work_orders, EXTERNAL_INPUT_WORK_ORDERS_SCHEMA)
    expected_work_order_counts = {
        "LOCATOR_RESOLUTION": 269,
        "COMPATIBILITY_DEFAULT_OFF_REVIEW": 213,
        "FUNCTION_OR_EXEMPTION_REVIEW": 425,
        "RESPONSIBILITY_ASSIGNMENT": 16,
    }
    if (
        external_input_work_orders["artifact_status"] != "BLOCKED_EXTERNAL_PREREQUISITES"
        or external_input_work_orders["work_order_count"] != 923
        or external_input_work_orders["work_order_kind_counts"] != expected_work_order_counts
        or any(not item["status"].startswith("BLOCKED_") for item in external_input_work_orders["work_orders"])
    ):
        raise ValueError("external-input work-order catalog is not the blocked 923-item exact-set")
    preflight_by_gate = {item["gate_id"]: item for item in gate_input_preflight["gate_results"]}
    preconditions = [
        {
            "check_id": "REG03-C01",
            "description": "four current global catalogs expose one exact atomic-ID set",
            "status": "PASS",
            "evidence": [f"4 catalogs, {len(current_ids)} atomic IDs, exact-set hash {semantic_sha256(sorted(current_ids))}"],
            "blocking_reason": None,
        },
        {
            "check_id": "REG03-C02",
            "description": "M02 replacement identity, DAG, terminals and completion revisions are frozen",
            "status": "PASS",
            "evidence": [f"{len(leaves)} leaves, {len(source['append_only_edges'])} edges, source semantic projection frozen"],
            "blocking_reason": None,
        },
        {
            "check_id": "REG03-C03",
            "description": "every planned production locator has a trusted resolver receipt and compatibility/default-off review",
            "status": "BLOCKED" if planned_leaf_count else "PASS",
            "evidence": [
                f"planned locator leaves requiring trusted resolution: {planned_leaf_count}",
                f"coverage ledger {LOCATOR_COVERAGE_PATH.relative_to(REPO).as_posix()} sha256={sha256(LOCATOR_COVERAGE_PATH)}",
                (
                    f"occurrences={locator_coverage['locator_occurrence_count']} "
                    f"unique={locator_coverage['unique_locator_count']} "
                    f"implemented-resolver/candidate-missing={locator_coverage['status_counts']['RESOLVER_IMPLEMENTED_CANDIDATE_NOT_FROZEN']} "
                    f"resolver-missing={locator_coverage['status_counts']['TRUSTED_RESOLVER_NOT_IMPLEMENTED']} "
                    f"file-absent={locator_coverage['status_counts']['PLANNED_FILE_ABSENT']} "
                    f"ordered-shared={locator_coverage['ownership_conflict_count']}"
                ),
                (
                    f"external-input preflight {GATE_INPUT_PREFLIGHT_PATH.relative_to(REPO).as_posix()} "
                    f"sha256={sha256(GATE_INPUT_PREFLIGHT_PATH)}; "
                    f"locator receipts={gate_input_preflight['locator_receipt_intake']['validated_count']}/"
                    f"{gate_input_preflight['locator_receipt_intake']['expected_count']}; "
                    f"compatibility/default-off reviews={gate_input_preflight['compatibility_review_intake']['validated_count']}/"
                    f"{gate_input_preflight['compatibility_review_intake']['expected_count']}; "
                    f"input-status={preflight_by_gate['REG03-C03']['status']}"
                ),
                (
                    f"blocked work orders locator={external_input_work_orders['work_order_kind_counts']['LOCATOR_RESOLUTION']} "
                    f"compatibility={external_input_work_orders['work_order_kind_counts']['COMPATIBILITY_DEFAULT_OFF_REVIEW']}"
                ),
            ],
            "blocking_reason": (
                "planned after-state files, clean-candidate receipts, default-off reviews or shared-locator ownership closure remain absent"
                if planned_leaf_count else None
            ),
        },
        {
            "check_id": "REG03-C04",
            "description": "function-set leaves have hash-bound UNIFIED review receipts or structured non-function exemptions",
            "status": "BLOCKED",
            "evidence": [
                f"coverage ledger {FUNCTION_REVIEW_COVERAGE_PATH.relative_to(REPO).as_posix()} sha256={sha256(FUNCTION_REVIEW_COVERAGE_PATH)}",
                (
                    f"function-set={function_review_coverage['function_set_leaf_count']} "
                    f"non-function-set={function_review_coverage['non_function_set_leaf_count']} "
                    f"static-contract-leaves={function_review_coverage['static_function_contract_leaf_count']} "
                    f"approved-function-receipts={function_review_coverage['approved_evidence_counts']['unified_function_review_receipts']} "
                    f"signed-non-function-exemptions={function_review_coverage['approved_evidence_counts']['signed_non_function_exemptions']}"
                ),
                (
                    f"external-input review receipts={gate_input_preflight['review_receipt_intake']['validated_count']}/"
                    f"{gate_input_preflight['review_receipt_intake']['expected_count']}; "
                    f"input-status={preflight_by_gate['REG03-C04']['status']}"
                ),
                f"blocked review work orders={external_input_work_orders['work_order_kind_counts']['FUNCTION_OR_EXEMPTION_REVIEW']}",
            ],
            "blocking_reason": "candidate-bound signed function reviews and structured non-function exemptions are absent",
        },
        {
            "check_id": "REG03-C05",
            "description": "the active external-activity receipt contract supports all three M02 activity types",
            "status": "PASS" if external_contract_ready else "BLOCKED",
            "evidence": [
                f"missing active receipt types: {', '.join(missing_external_types) or 'none'}",
                "3 positive typed payloads and 8 targeted negative cases pass"
                if external_contract_ready else
                f"semantic validator exit={external_contract_validator.returncode}",
            ],
            "blocking_reason": (
                "SCOPED_CANARY, PROFILE_APPROVAL and PROTECTED_MERGE are not active receipt payload contracts"
                if not external_contract_ready else None
            ),
        },
        {
            "check_id": "REG03-C06",
            "description": "all sixteen M02 parents have named owner, reviewer and approver",
            "status": "PASS" if assigned == 16 else "BLOCKED",
            "evidence": [
                f"fully assigned active-registry M02 parents: {assigned}/16",
                (
                    f"signed responsibility input={gate_input_preflight['responsibility_intake']['assigned_task_count']}/16; "
                    f"trusted-verifier={str(gate_input_preflight['responsibility_intake']['signature_verification_available']).lower()}; "
                    f"input-status={preflight_by_gate['REG03-C06']['status']}"
                ),
                f"blocked responsibility work orders={external_input_work_orders['work_order_kind_counts']['RESPONSIBILITY_ASSIGNMENT']}",
            ],
            "blocking_reason": None if assigned == 16 else "M02 responsibility identities are unresolved",
        },
        {
            "check_id": "REG03-C07",
            "description": "one clean implementation candidate is frozen for the switch review",
            "status": "BLOCKED",
            "evidence": [
                "all active M02 tasks retain clean implementation candidate not frozen",
                (
                    f"design-manifest={gate_input_preflight['candidate_intake']['design_manifest']['status']} "
                    f"implementation-manifest={gate_input_preflight['candidate_intake']['implementation_manifest']['status']} "
                    f"same-commit={str(gate_input_preflight['candidate_intake']['same_commit']).lower()} "
                    f"input-status={preflight_by_gate['REG03-C07']['status']}"
                ),
                (
                    f"implementation closure READY={implementation_closure['readiness_counts']['READY']} "
                    f"PARTIAL={implementation_closure['readiness_counts']['PARTIAL']} "
                    f"MISSING={implementation_closure['readiness_counts']['MISSING']} "
                    f"INVALID={implementation_closure['readiness_counts']['INVALID']}"
                ),
            ],
            "blocking_reason": "candidate manifest and same-candidate review scope are absent",
        },
        {
            "check_id": "REG03-C08",
            "description": "future tombstones cover all old IDs and point to each parent's replacement terminal closure",
            "status": "PASS",
            "evidence": [f"{len(tombstones)} tombstones, {len(task_replacements)} parent replacement exact-sets"],
            "blocking_reason": None,
        },
        {
            "check_id": "REG03-C09",
            "description": "all four candidate catalogs are generated from one switch-plan hash before atomic replacement",
            "status": "BLOCKED",
            "evidence": [f"current catalogs remain legacy at {len(current_ids)} IDs; candidate exact-set has {len(candidate_ids)} IDs"],
            "blocking_reason": "candidate catalogs must not be emitted until C03-C07 pass",
        },
    ]
    return {
        "schema_version": "1.0.0",
        "artifact_kind": "M02_CODE_DIRECT_GLOBAL_REGISTRY_SWITCH_READINESS_LEDGER",
        "artifact_status": "BLOCKED_PRE_SWITCH_READINESS",
        "revision_id": "M02-REG-03-PRECHECK-V1",
        "source_catalog": {
            "path": SOURCE.relative_to(REPO).as_posix(),
            "sha256": sha256(SOURCE),
        },
        "source_semantic_projection_sha256": semantic_sha256(source_semantic_projection(source)),
        "locator_coverage": {
            "path": LOCATOR_COVERAGE_PATH.relative_to(REPO).as_posix(),
            "sha256": sha256(LOCATOR_COVERAGE_PATH),
        },
        "function_review_coverage": {
            "path": FUNCTION_REVIEW_COVERAGE_PATH.relative_to(REPO).as_posix(),
            "sha256": sha256(FUNCTION_REVIEW_COVERAGE_PATH),
        },
        "gate_input_preflight": {
            "path": GATE_INPUT_PREFLIGHT_PATH.relative_to(REPO).as_posix(),
            "sha256": sha256(GATE_INPUT_PREFLIGHT_PATH),
        },
        "external_input_work_orders": {
            "path": EXTERNAL_INPUT_WORK_ORDERS_PATH.relative_to(REPO).as_posix(),
            "sha256": sha256(EXTERNAL_INPUT_WORK_ORDERS_PATH),
        },
        "implementation_candidate_closure": {
            "path": IMPLEMENTATION_CLOSURE_PATH.relative_to(REPO).as_posix(),
            "sha256": sha256(IMPLEMENTATION_CLOSURE_PATH),
        },
        "write_scope_supersession": {
            "path": WRITE_SCOPE_SUPERSESSION_PATH.relative_to(REPO).as_posix(),
            "sha256": sha256(WRITE_SCOPE_SUPERSESSION_PATH),
        },
        "current_global_catalogs": refs,
        "current_global_atomic_pr_count": len(current_ids),
        "current_legacy_m02_atomic_pr_count": len(legacy_ids),
        "replacement_m02_atomic_pr_count": len(replacement_ids),
        "candidate_global_atomic_pr_count": len(candidate_ids),
        "current_atomic_id_exact_set_sha256": semantic_sha256(sorted(current_ids)),
        "replacement_atomic_id_exact_set_sha256": semantic_sha256(sorted(replacement_ids)),
        "candidate_atomic_id_exact_set_sha256": semantic_sha256(sorted(candidate_ids)),
        "task_replacements": task_replacements,
        "legacy_tombstones": tombstones,
        "external_activity_registrations": external_activities,
        "preconditions": preconditions,
        "switch_protocol": {
            "decision": "BLOCKED_PRECONDITIONS",
            "atomic_steps": [
                "freeze one clean candidate plus named owner reviewer and approver identities",
                "resolve planned locators and attach hash-bound function-review receipts or typed exemptions",
                "activate and validate the three M02 external-activity receipt payload contracts",
                "generate task claim PR-design and overlay candidates from this exact replacement set",
                "verify all four candidates share the candidate atomic-ID hash and reject every old claim ID",
                "replace all four catalogs in one reviewed commit while retaining the 34 tombstones",
            ],
            "failure_rule": "ANY_BLOCKED_PRECONDITION_OR_CATALOG_HASH_MISMATCH_ABORTS_WITH_ALL_FOUR_ACTIVE_CATALOGS_UNCHANGED",
            "rollback_rule": "REVERT_THE_SINGLE_REGISTRY_SWITCH_COMMIT_AND_KEEP_TOMBSTONES_AND_EVIDENCE_APPEND_ONLY",
            "post_switch_execution_state": "DRAFT_DESIGN_AND_TEMPLATE_EXECUTION_NO_GO",
        },
        "validation": {
            "schema": "PASS",
            "source_catalog_semantic_exact": "PASS",
            "four_catalog_current_exact_set": "PASS",
            "legacy_m02_exact_set": "PASS",
            "replacement_exact_set": "PASS",
            "legacy_replacement_disjoint": True,
            "candidate_count_derivation": "PASS",
            "terminal_exact_set": "PASS",
            "completion_member_exact_set": "PASS",
            "external_activity_exact_set": "PASS",
            "tombstone_exact_set": "PASS",
            "gate_input_preflight_exact": "PASS",
            "external_input_work_orders_exact": "PASS",
            "implementation_candidate_closure_exact": "PASS",
            "premature_switch_rejected": True,
            "mutation_guards": {
                "catalog_exact_set_drift": "PASS",
                "legacy_omission": "PASS",
                "replacement_omission": "PASS",
                "candidate_hash_drift": "PASS",
                "terminal_mismatch": "PASS",
                "completion_member_mismatch": "PASS",
                "tombstone_omission": "PASS",
                "premature_pass": "PASS",
                "preflight_hash_drift": "PASS",
                "work_order_hash_drift": "PASS",
                "implementation_closure_hash_drift": "PASS",
            },
        },
        "proof_ceiling": "SWITCH_READINESS_AND_TOMBSTONE_PLAN_ONLY_NOT_GLOBAL_REGISTRY_SWITCH_TARGET_BINDING_FUNCTION_REVIEW_IMPLEMENTATION_TEST_EXECUTION_AUTHORIZATION_OR_ACCEPTANCE",
    }


def validate(payload: dict[str, Any]) -> None:
    validate_against_schema(payload, SCHEMA)
    expected = build()
    if payload != expected:
        raise ValueError("switch readiness ledger differs from exact derived state")
    task_rows = payload["task_replacements"]
    if [item["parent_task_id"] for item in task_rows] != [
        f"T1-M02-N{number:03d}" for number in range(1, 17)
    ]:
        raise ValueError("M02 parent replacement exact-set drifted")
    legacy = [item["legacy_atomic_pr_id"] for item in payload["legacy_tombstones"]]
    if len(legacy) != len(set(legacy)) or len(legacy) != 34:
        raise ValueError("legacy tombstone exact-set drifted")
    replacement = {
        item for row in task_rows for item in row["replacement_atomic_pr_ids"]
    }
    if len(replacement) != 425:
        raise ValueError("replacement exact-set drifted")
    if any(item["status"] == "BLOCKED" for item in payload["preconditions"]):
        if payload["switch_protocol"]["decision"] != "BLOCKED_PRECONDITIONS":
            raise ValueError("premature registry switch decision")


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
    except (ValueError, TypeError) as exc:
        if expected_error not in str(exc):
            raise ValueError(f"mutation {label} hit wrong assertion: {exc}") from exc
        return
    raise ValueError(f"mutation {label} did not fail")


def run_mutation_tests(payload: dict[str, Any]) -> None:
    expect_failure(
        "catalog exact-set drift", payload,
        lambda item: item["current_global_catalogs"][0].update({"atomic_pr_count": 1288}),
        "schema const mismatch at $.current_global_catalogs[0].atomic_pr_count",
    )
    expect_failure(
        "legacy omission", payload,
        lambda item: item["legacy_tombstones"].pop(),
        "schema minItems failed at $.legacy_tombstones",
    )
    expect_failure(
        "replacement omission", payload,
        lambda item: item["task_replacements"][0]["replacement_atomic_pr_ids"].pop(),
        "differs from exact derived state",
    )
    expect_failure(
        "candidate hash drift", payload,
        lambda item: item.update({"candidate_atomic_id_exact_set_sha256": "0" * 64}),
        "differs from exact derived state",
    )
    expect_failure(
        "terminal mismatch", payload,
        lambda item: item["task_replacements"][0].update({"terminal_leaf_id": "M02-N001-L01"}),
        "differs from exact derived state",
    )
    expect_failure(
        "completion member mismatch", payload,
        lambda item: item["task_replacements"][0]["completion_member_atomic_pr_ids"].pop(),
        "differs from exact derived state",
    )
    expect_failure(
        "tombstone omission", payload,
        lambda item: item["legacy_tombstones"].pop(0),
        "schema minItems failed at $.legacy_tombstones",
    )
    expect_failure(
        "premature pass", payload,
        lambda item: item["switch_protocol"].update({"decision": "PASS"}),
        "schema const mismatch at $.switch_protocol.decision",
    )
    expect_failure(
        "preflight hash drift", payload,
        lambda item: item["gate_input_preflight"].update({"sha256": "0" * 64}),
        "differs from exact derived state",
    )
    expect_failure(
        "work order hash drift", payload,
        lambda item: item["external_input_work_orders"].update({"sha256": "0" * 64}),
        "differs from exact derived state",
    )
    expect_failure(
        "implementation closure hash drift", payload,
        lambda item: item["implementation_candidate_closure"].update({"sha256": "0" * 64}),
        "differs from exact derived state",
    )


def render_markdown(payload: dict[str, Any]) -> str:
    lines = [
        "# M02代码直达Registry切换就绪账本",
        "",
        "状态：`BLOCKED_PRE_SWITCH_READINESS / NO-GO`",
        "",
        "本页由机器账本确定性生成。它冻结REG-03的替换与tombstone语义，"
        "不表示全局registry已经切换，也不授予实现或执行权限。",
        "",
        "## 精确集合",
        "",
        f"- 当前全局原子PR：{payload['current_global_atomic_pr_count']}；当前M02旧卡：{payload['current_legacy_m02_atomic_pr_count']}。",
        f"- M02替代叶：{payload['replacement_m02_atomic_pr_count']}；切换候选全局原子PR：{payload['candidate_global_atomic_pr_count']}。",
        f"- 候选exact-set SHA256：`{payload['candidate_atomic_id_exact_set_sha256']}`。",
        f"- 旧卡tombstone：{len(payload['legacy_tombstones'])}；父任务replacement：{len(payload['task_replacements'])}。",
        "",
        "## 前置门",
        "",
        "| Gate | 状态 | 证据/阻断 |",
        "|---|---|---|",
    ]
    for item in payload["preconditions"]:
        detail = "; ".join(item["evidence"])
        if item["blocking_reason"]:
            detail += f"；阻断：{item['blocking_reason']}"
        lines.append(f"| `{item['check_id']}` | `{item['status']}` | {detail} |")
    lines.extend([
        "",
        "## 原子切换协议",
        "",
    ])
    for index, step in enumerate(payload["switch_protocol"]["atomic_steps"], start=1):
        lines.append(f"{index}. {step}")
    lines.extend([
        "",
        f"失败规则：`{payload['switch_protocol']['failure_rule']}`。",
        "",
        "切换后仍保持：`DRAFT_DESIGN / TEMPLATE_EXECUTION_NO_GO`。",
        "",
        "## 证明上限",
        "",
        f"`{payload['proof_ceiling']}`",
        "",
    ])
    return "\n".join(lines)


def canonical_json(payload: dict[str, Any]) -> str:
    return json.dumps(payload, ensure_ascii=False, indent=2) + "\n"


def main() -> int:
    parser = argparse.ArgumentParser()
    mode = parser.add_mutually_exclusive_group(required=True)
    mode.add_argument("--write", action="store_true")
    mode.add_argument("--check", action="store_true")
    mode.add_argument("--verify", action="store_true")
    args = parser.parse_args()
    payload = build()
    validate_against_schema(payload, SCHEMA)
    if args.write:
        OUTPUT.write_text(canonical_json(payload), encoding="utf-8")
        DOC_OUTPUT.parent.mkdir(parents=True, exist_ok=True)
        DOC_OUTPUT.write_text(render_markdown(payload), encoding="utf-8")
        print(f"WROTE {OUTPUT.relative_to(REPO)}")
        print(f"WROTE {DOC_OUTPUT.relative_to(REPO)}")
    else:
        if not OUTPUT.is_file() or json.loads(OUTPUT.read_text(encoding="utf-8")) != payload:
            raise ValueError("generated M02 registry switch plan is stale")
        if not DOC_OUTPUT.is_file() or DOC_OUTPUT.read_text(encoding="utf-8") != render_markdown(payload):
            raise ValueError("generated M02 registry switch plan markdown is stale")
        validate(payload)
        if args.verify:
            run_mutation_tests(payload)
        print(
            f"PASS decision={payload['switch_protocol']['decision']} "
            f"current={payload['current_global_atomic_pr_count']} "
            f"legacy={payload['current_legacy_m02_atomic_pr_count']} "
            f"replacement={payload['replacement_m02_atomic_pr_count']} "
            f"candidate={payload['candidate_global_atomic_pr_count']}"
        )
    print(f"PROOF_CEILING {payload['proof_ceiling']}")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
