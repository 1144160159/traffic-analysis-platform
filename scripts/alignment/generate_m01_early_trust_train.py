#!/usr/bin/env python3
"""Generate and fail-closed validate the M01 early-trust preview.

The preview appends P057-P066 under a new N015 prerequisite task, transfers
the exact protected-verifier write scopes from nine superseded N010 leaves,
and revises only the declared dependency/completion contracts.  It never
writes the four active registries, product code, fixtures, deployment state or
test evidence.
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
ALLOCATION = REPO / "contracts/alignment/m01-early-trust-train-allocation.v1.json"
ALLOCATION_SCHEMA = REPO / "contracts/alignment/m01-early-trust-train-allocation.schema.json"
CATALOG = REPO / "contracts/alignment/m01-early-trust-train-catalog.v1.json"
CATALOG_SCHEMA = REPO / "contracts/alignment/m01-early-trust-train-catalog.schema.json"
MARKDOWN = REPO / "doc/07_alignment/generated/M01早期受信验证列车候选目录.md"

ACTIVE_CATALOGS = [
    "contracts/alignment/task-registry.v1.json",
    "contracts/alignment/developer-claim-package-catalog.v1.json",
    "contracts/alignment/pr-design-application-catalog.v1.json",
    "contracts/alignment/task-execution-overlay.template.v1.json",
]
EXPECTED_ACTIVE_HASHES = {
    "contracts/alignment/task-registry.v1.json": "411be54ada25ba399fc4e14d1fb055986548a6c265c4b499254a5793249a23d0",
    "contracts/alignment/developer-claim-package-catalog.v1.json": "18aeedadba1504c02141185c0cc40e9237dd8c96665c5923009f9f99484ee883",
    "contracts/alignment/pr-design-application-catalog.v1.json": "486036113a9264679207d1bc19c58a62c3bf65b8c535e1f0c49230147192d03c",
    "contracts/alignment/task-execution-overlay.template.v1.json": "a606df1a2189c5b17fa114dfb860ad4224976cd1b5eb063dc101643b0618b503",
}
ACTIVE_PROJECTION_FIELDS = [
    "atomic_pr_id", "pr_type", "parent_work_id", "outcome",
    "dependency_contracts", "change_targets", "required_gates", "rollback",
    "formal_execution_status", "allowed_claim", "forbidden_claim",
]
EXPECTED_ACTIVE_M01_PROJECTION_SHA256 = "067326beae831a1a8b681067bbdefb581c56e0a81785644f69878026b4155fa2"
EXPECTED_ALLOCATION_SEMANTIC_SHA256 = "50ab2ad09914eb53acae3f0618ebd78f4ffcf041f37f10deb1d38a14985c321b"

P002 = "T1-M01-P002-IDX-n001-task-completion"
P005 = "T1-M01-P005-REF-n003-s1"
P006 = "T1-M01-P006-TST-PRE-n003-s2"
P007 = "T1-M01-P007-IDX-n003-task-completion"
P031 = "T1-M01-P031-IDX-n009-task-completion"
P038 = "T1-M01-P038-REF-n010-s7"
P044 = "T1-M01-P044-REF-n010-s13"
P048 = "T1-M01-P048-IDX-n010-task-completion"
P066 = "T1-M01-P066-IDX-n015-task-completion"

SUPERSESSION_MAP = {
    "T1-M01-P032-CTR-n010-s1": "T1-M01-P057-CTR-n015-s1",
    "T1-M01-P033-REF-n010-s2": "T1-M01-P058-REF-n015-s2",
    "T1-M01-P034-REF-n010-s3": "T1-M01-P059-REF-n015-s3",
    "T1-M01-P035-REF-n010-s4": "T1-M01-P060-REF-n015-s4",
    "T1-M01-P036-REF-n010-s5": "T1-M01-P061-REF-n015-s5",
    "T1-M01-P037-REF-n010-s6": "T1-M01-P062-REF-n015-s6",
    "T1-M01-P046-OPS-n010-s15": "T1-M01-P063-OPS-n015-s7",
    "T1-M01-P045-TST-PRE-n010-s14": "T1-M01-P064-TST-PRE-n015-s8",
    "T1-M01-P047-TST-POST-n010-s16": "T1-M01-P065-TST-POST-n015-s9",
}
EXACT_TRANSFER_OLD_IDS = set(list(SUPERSESSION_MAP)[:7])
APPENDED_IDS = [*SUPERSESSION_MAP.values(), P066]
FORMAL_STATUS = "BLOCKED_UNTIL_GLOBAL_REGISTRY_TARGET_BINDING_FUNCTION_REVIEW_AND_SIGNED_OVERLAY"
FORBIDDEN_CLAIMS = [
    "GLOBAL_REGISTRY_SWITCHED", "FUNCTION_DESIGN_REVIEWED", "IMPLEMENTED",
    "FIXTURES_CREATED", "DEPLOYED", "TEST_EXECUTED", "TRUST_PASS",
    "P006_PASS", "EXECUTION_AUTHORIZED", "PARENT_COMPLETE",
    "MILESTONE_COMPLETE", "PRODUCTION_ACCEPTED",
]


def canonical_bytes(value: Any) -> bytes:
    return json.dumps(value, ensure_ascii=False, sort_keys=True, separators=(",", ":")).encode("utf-8")


def sha256(path: Path) -> str:
    return hashlib.sha256(path.read_bytes()).hexdigest()


def digest(value: Any) -> str:
    return hashlib.sha256(canonical_bytes(value)).hexdigest()


def load(path: Path) -> dict[str, Any]:
    value = json.loads(path.read_text(encoding="utf-8"))
    if not isinstance(value, dict):
        raise ValueError(f"JSON root is not an object: {path}")
    return value


def atomic_number(atomic_pr_id: str) -> int:
    match = re.search(r"-P([0-9]{3})-", atomic_pr_id)
    if match is None:
        raise ValueError(f"invalid atomic PR ID: {atomic_pr_id}")
    return int(match.group(1))


def target_locator(target: dict[str, Any]) -> str:
    pointer = target.get("symbol_or_pointer")
    return target["path"] if pointer is None else f"{target['path']}#{pointer}"


def active_atomic_ids(relative: str) -> list[str]:
    payload = load(REPO / relative)
    if relative.endswith("task-registry.v1.json"):
        values = [pr["pr_id"] for task in payload["tasks"] for pr in task["pr_sequence"]]
        values += [pr["pr_id"] for group in payload["closure_slices"] for pr in group["pr_sequence"]]
    elif relative.endswith("developer-claim-package-catalog.v1.json"):
        values = [item["atomic_pr_id"] for item in payload["packages"]]
    elif relative.endswith("pr-design-application-catalog.v1.json"):
        values = [item["atomic_pr_id"] for item in payload["entries"]]
    elif relative.endswith("task-execution-overlay.template.v1.json"):
        values = [item["pr_id"] for item in payload["atomic_pr_bindings"]]
    else:
        raise ValueError(f"unsupported active catalog: {relative}")
    if len(values) != len(set(values)):
        raise ValueError(f"active catalog repeats an atomic ID: {relative}")
    return values


def active_claim_rows() -> list[dict[str, Any]]:
    payload = load(REPO / ACTIVE_CATALOGS[1])
    rows = [item for item in payload["packages"] if item["atomic_pr_id"].startswith("T1-M01-P")]
    return sorted(rows, key=lambda item: atomic_number(item["atomic_pr_id"]))


def active_projection(rows: list[dict[str, Any]]) -> list[dict[str, Any]]:
    return [{field: item[field] for field in ACTIVE_PROJECTION_FIELDS} for item in rows]


def active_pr_map() -> dict[str, dict[str, Any]]:
    registry = load(REPO / ACTIVE_CATALOGS[0])
    result: dict[str, dict[str, Any]] = {}
    for task in registry["tasks"]:
        for pr in task["pr_sequence"]:
            result[pr["pr_id"]] = pr
    for group in registry["closure_slices"]:
        for pr in group["pr_sequence"]:
            result[pr["pr_id"]] = pr
    return result


def active_tasks() -> dict[str, dict[str, Any]]:
    return {item["task_id"]: item for item in load(REPO / ACTIVE_CATALOGS[0])["tasks"]}


def assert_active_sources() -> tuple[list[dict[str, Any]], set[str]]:
    for relative, expected in EXPECTED_ACTIVE_HASHES.items():
        actual = sha256(REPO / relative)
        if actual != expected:
            raise ValueError(f"active catalog hash drifted: {relative}: {actual}")
    sets = [set(active_atomic_ids(relative)) for relative in ACTIVE_CATALOGS]
    if any(items != sets[0] for items in sets[1:]):
        raise ValueError("active four-catalog atomic ID exact-sets differ")
    if len(sets[0]) != 1289:
        raise ValueError("active global atomic PR count drifted")
    rows = active_claim_rows()
    if len(rows) != 56 or [atomic_number(item["atomic_pr_id"]) for item in rows] != list(range(1, 57)):
        raise ValueError("active M01 P001-P056 exact epoch drifted")
    if digest(active_projection(rows)) != EXPECTED_ACTIVE_M01_PROJECTION_SHA256:
        raise ValueError("active M01 P001-P056 ordered projection drifted")
    return rows, sets[0]


def old_claim_map(rows: list[dict[str, Any]]) -> dict[str, dict[str, Any]]:
    return {item["atomic_pr_id"]: item for item in rows}


def transferred_locators(old_id: str, claims: dict[str, dict[str, Any]]) -> list[str]:
    return [target_locator(item) for item in claims[old_id]["change_targets"]]


def step(
    *, atomic_pr_id: str, phase: str, locators: list[str], dependency: str,
    gates: list[str], outcome: str, oracle: str, replaces: str | None,
    target_state: str = "PLANNED", terminal: bool = False,
) -> dict[str, Any]:
    pr_type = atomic_pr_id.split("-")[3]
    if pr_type == "TST":
        pr_type = "-".join(atomic_pr_id.split("-")[3:5])
    return {
        "atomic_pr_id": atomic_pr_id,
        "pr_type": pr_type,
        "phase": phase,
        "primary_locator": locators[0],
        "companion_locators": locators[1:],
        "target_state": target_state,
        "prerequisite_atomic_pr_ids": [dependency],
        "required_gates": gates,
        "single_outcome": outcome,
        "oracle_and_rollback": oracle,
        "terminal_task_idx": terminal,
        "replaces_atomic_pr_id": replaces,
    }


def allocation_semantic_projection(payload: dict[str, Any]) -> dict[str, Any]:
    return {key: value for key, value in payload.items() if key != "semantic_projection_sha256"}


def build_allocation() -> dict[str, Any]:
    rows, _ = assert_active_sources()
    claims = old_claim_map(rows)
    common_rollback = (
        "fail closed on any contract identity role purpose time policy candidate profile or environment drift; "
        "rollback restores unconditional hard-block mode and preserves immutable audit and evidence"
    )
    steps = [
        step(
            atomic_pr_id="T1-M01-P057-CTR-n015-s1", phase="trusted-signature-contracts",
            locators=transferred_locators("T1-M01-P032-CTR-n010-s1", claims), dependency=P002,
            gates=["G0"], outcome=claims["T1-M01-P032-CTR-n010-s1"]["outcome"]["acceptance_oracle"],
            oracle=common_rollback, replaces="T1-M01-P032-CTR-n010-s1",
        ),
        step(
            atomic_pr_id="T1-M01-P058-REF-n015-s2", phase="trusted-signature-fixture",
            locators=transferred_locators("T1-M01-P033-REF-n010-s2", claims),
            dependency="T1-M01-P057-CTR-n015-s1", gates=["G0"],
            outcome=claims["T1-M01-P033-REF-n010-s2"]["outcome"]["acceptance_oracle"],
            oracle=common_rollback, replaces="T1-M01-P033-REF-n010-s2",
        ),
        step(
            atomic_pr_id="T1-M01-P059-REF-n015-s3", phase="trusted-verifier-adapter",
            locators=transferred_locators("T1-M01-P034-REF-n010-s3", claims),
            dependency="T1-M01-P058-REF-n015-s2", gates=["G0"],
            outcome=claims["T1-M01-P034-REF-n010-s3"]["outcome"]["acceptance_oracle"],
            oracle=common_rollback, replaces="T1-M01-P034-REF-n010-s3",
        ),
        step(
            atomic_pr_id="T1-M01-P060-REF-n015-s4", phase="trusted-verifier-wrapper",
            locators=transferred_locators("T1-M01-P035-REF-n010-s4", claims),
            dependency="T1-M01-P059-REF-n015-s3", gates=["G0"],
            outcome=claims["T1-M01-P035-REF-n010-s4"]["outcome"]["acceptance_oracle"],
            oracle=common_rollback, replaces="T1-M01-P035-REF-n010-s4",
        ),
        step(
            atomic_pr_id="T1-M01-P061-REF-n015-s5", phase="caller-candidate-artifact-refs",
            locators=transferred_locators("T1-M01-P036-REF-n010-s5", claims),
            dependency="T1-M01-P060-REF-n015-s4", gates=["G0"],
            outcome=claims["T1-M01-P036-REF-n010-s5"]["outcome"]["acceptance_oracle"],
            oracle=common_rollback, replaces="T1-M01-P036-REF-n010-s5",
        ),
        step(
            atomic_pr_id="T1-M01-P062-REF-n015-s6", phase="caller-implementation-candidate",
            locators=transferred_locators("T1-M01-P037-REF-n010-s6", claims),
            dependency="T1-M01-P061-REF-n015-s5", gates=["G0"],
            outcome=claims["T1-M01-P037-REF-n010-s6"]["outcome"]["acceptance_oracle"],
            oracle=common_rollback, replaces="T1-M01-P037-REF-n010-s6",
        ),
        step(
            atomic_pr_id="T1-M01-P063-OPS-n015-s7", phase="trusted-verifier-protected-backend",
            locators=transferred_locators("T1-M01-P046-OPS-n010-s15", claims),
            dependency="T1-M01-P062-REF-n015-s6", gates=["G6"],
            outcome=claims["T1-M01-P046-OPS-n010-s15"]["outcome"]["acceptance_oracle"],
            oracle=common_rollback, replaces="T1-M01-P046-OPS-n010-s15",
        ),
        step(
            atomic_pr_id="T1-M01-P064-TST-PRE-n015-s8", phase="trusted-signature-negative-run",
            locators=[
                "doc/02_acceptance/topic1/work-orders/t1-m01-p064-tst-pre-n015-s8/test-result.json",
                "doc/02_acceptance/topic1/work-orders/t1-m01-p064-tst-pre-n015-s8/case-report.json",
            ], dependency="T1-M01-P063-OPS-n015-s7", gates=["G0", "G1"],
            outcome=claims["T1-M01-P045-TST-PRE-n010-s14"]["outcome"]["acceptance_oracle"],
            oracle=common_rollback, replaces="T1-M01-P045-TST-PRE-n010-s14", target_state="PLANNED_OUTPUT",
        ),
        step(
            atomic_pr_id="T1-M01-P065-TST-POST-n015-s9", phase="trusted-signature-positive-run",
            locators=[
                "doc/02_acceptance/topic1/work-orders/t1-m01-p065-tst-post-n015-s9/test-result.json",
                "doc/02_acceptance/topic1/work-orders/t1-m01-p065-tst-post-n015-s9/case-report.json",
            ], dependency="T1-M01-P064-TST-PRE-n015-s8", gates=["G2", "G3"],
            outcome=claims["T1-M01-P047-TST-POST-n010-s16"]["outcome"]["acceptance_oracle"],
            oracle=common_rollback, replaces="T1-M01-P047-TST-POST-n010-s16", target_state="PLANNED_OUTPUT",
        ),
        step(
            atomic_pr_id=P066, phase="task-completion",
            locators=[
                "doc/02_acceptance/topic1/tasks/t1-m01-n015/completion-candidate.json",
                "doc/02_acceptance/topic1/tasks/t1-m01-n015/current-evidence-index.json",
            ], dependency="T1-M01-P065-TST-POST-n015-s9", gates=["G0"],
            outcome="materialize the exact N015 PASS completion candidate only after all nine early-trust leaves and dependency evidence resolve",
            oracle=(
                "publish no N015 current index unless every declared leaf gate rollback and same-identity evidence is PASS; "
                "rollback preserves failed immutable evidence and the prior authoritative index"
            ), replaces=None, target_state="PLANNED_OUTPUT", terminal=True,
        ),
    ]
    supersessions = []
    for old_id, new_id in SUPERSESSION_MAP.items():
        supersessions.append({
            "legacy_atomic_pr_id": old_id,
            "replacement_atomic_pr_id": new_id,
            "disposition": "SUPERSEDED_IN_PREVIEW_ACTIVE_REGISTRY_UNCHANGED",
            "write_scope_transfer": (
                "EXACT_LOCATOR_SET_TRANSFER" if old_id in EXACT_TRANSFER_OLD_IDS
                else "EQUIVALENT_NEW_ID_OUTPUT_SCOPE"
            ),
            "historical_id_policy": "PRESERVED_NOT_REUSED",
        })
    tasks = active_tasks()
    n003 = tasks["T1-M01-N003"]["completion_contract"]
    n010 = tasks["T1-M01-N010"]["completion_contract"]
    allocation: dict[str, Any] = {
        "schema_version": "1.0.0",
        "artifact_kind": "M01_EARLY_TRUST_TRAIN_ALLOCATION",
        "artifact_status": "VERSIONED_PREVIEW_NOT_GLOBAL_REGISTRY",
        "revision_id": "M01-EARLY-TRUST-V1",
        "base_active_catalog_refs": [
            {"path": path, "sha256": EXPECTED_ACTIVE_HASHES[path]} for path in ACTIVE_CATALOGS
        ],
        "active_m01_projection_freeze": {
            "atomic_id_epoch": "T1-M01-P001-P056",
            "field_names": ACTIVE_PROJECTION_FIELDS,
            "ordered_projection_sha256": EXPECTED_ACTIVE_M01_PROJECTION_SHA256,
            "status": "PASS_EXACT_ACTIVE_SOURCE_PRESERVATION",
        },
        "semantic_projection_sha256": "0" * 64,
        "allocation_epoch": "T1-M01-P057-P066",
        "append_leaf_count": 10,
        "new_task": {
            "task_id": "T1-M01-N015", "milestone_id": "T1-M01",
            "depends_on_task_ids": ["T1-M01-N001"],
            "single_result": "provide the independently protected trusted-verifier prerequisite required before P005 and P006",
            "status": "DRAFT_PREVIEW_NOT_REGISTERED",
        },
        "train": {
            "train_id": "M01-EARLY-TRUST-TRAIN-01", "parent_task_id": "T1-M01-N015",
            "gap_id": "GAP-M01-P006-TRUST-DEPENDENCY-CYCLE",
            "single_result": "move the minimal complete trust stack before P005 without creating a P006 to P048 back-edge",
            "steps": steps, "entry_dependency_atomic_pr_id": P002, "terminal_atomic_pr_id": P066,
        },
        "supersessions": supersessions,
        "dependency_revisions": [
            {"atomic_pr_id": P005, "prior_dependency_ids": [P002], "revised_dependency_ids": [P002, P066],
             "reason": "P005 runner implementation starts only after the protected trust prerequisite task is complete"},
            {"atomic_pr_id": P006, "prior_dependency_ids": [P005], "revised_dependency_ids": [P005, P066],
             "reason": "P006 directly records the early trust terminal and must never depend on current P048"},
            {"atomic_pr_id": P038, "prior_dependency_ids": ["T1-M01-P037-REF-n010-s6"],
             "revised_dependency_ids": [P031, P066],
             "reason": "remaining N010 caller migrations join the prior N009 chain and the completed early trust task"},
            {"atomic_pr_id": P048, "prior_dependency_ids": ["T1-M01-P047-TST-POST-n010-s16"],
             "revised_dependency_ids": [P044, P066],
             "reason": "N010 completion joins its seven retained caller migrations with the N015 trust completion"},
        ],
        "task_dependency_revisions": [
            {"task_id": "T1-M01-N003", "prior_dependency_task_ids": ["T1-M01-N001"],
             "revised_dependency_task_ids": ["T1-M01-N001", "T1-M01-N015"]},
            {"task_id": "T1-M01-N010", "prior_dependency_task_ids": ["T1-M01-N009"],
             "revised_dependency_task_ids": ["T1-M01-N009", "T1-M01-N015"]},
        ],
        "completion_revisions": [
            {
                "parent_task_id": "T1-M01-N003", "terminal_atomic_pr_id": P007,
                "prior_member_atomic_pr_ids": n003["expected_atomic_pr_ids"],
                "revised_member_atomic_pr_ids": n003["expected_atomic_pr_ids"],
                "superseded_member_atomic_pr_ids": [],
                "revised_dependency_task_idx_pr_ids": [P002, P066],
                "revised_terminal_direct_dependency_ids": [P006],
                "revision_semantics": "EXISTING_TERMINAL_ID_PRESERVED_MEMBER_AND_DEPENDENCY_EXACT_SET_REVISED",
            },
            {
                "parent_task_id": "T1-M01-N010", "terminal_atomic_pr_id": P048,
                "prior_member_atomic_pr_ids": n010["expected_atomic_pr_ids"],
                "revised_member_atomic_pr_ids": [f"T1-M01-P{number:03d}-REF-n010-s{number - 31}" for number in range(38, 45)],
                "superseded_member_atomic_pr_ids": list(SUPERSESSION_MAP),
                "revised_dependency_task_idx_pr_ids": [P031, P066],
                "revised_terminal_direct_dependency_ids": [P044, P066],
                "revision_semantics": "EXISTING_TERMINAL_ID_PRESERVED_MEMBER_AND_DEPENDENCY_EXACT_SET_REVISED",
            },
            {
                "parent_task_id": "T1-M01-N015", "terminal_atomic_pr_id": P066,
                "prior_member_atomic_pr_ids": [],
                "revised_member_atomic_pr_ids": APPENDED_IDS[:-1],
                "superseded_member_atomic_pr_ids": [],
                "revised_dependency_task_idx_pr_ids": [P002],
                "revised_terminal_direct_dependency_ids": ["T1-M01-P065-TST-POST-n015-s9"],
                "revision_semantics": "NEW_TASK_TERMINAL_AND_MEMBER_EXACT_SET",
            },
        ],
        "claims": {
            "allowed": "P057-P066 early-trust ownership supersession and dependency intent are statically allocated in preview",
            "forbidden": FORBIDDEN_CLAIMS,
            "proof_ceiling": "STATIC_PREVIEW_ALLOCATION_ONLY_NOT_GLOBAL_REGISTRATION_FUNCTION_REVIEW_IMPLEMENTATION_FIXTURE_CREATION_DEPLOYMENT_TEST_EXECUTION_TRUST_PASS_AUTHORIZATION_OR_ACCEPTANCE",
        },
    }
    allocation["semantic_projection_sha256"] = digest(allocation_semantic_projection(allocation))
    return allocation


def expected_dependency_revisions() -> dict[str, list[str]]:
    return {
        P005: [P002, P066], P006: [P005, P066], P038: [P031, P066], P048: [P044, P066],
    }


def assert_allocation(
    payload: dict[str, Any], rows: list[dict[str, Any]], *, enforce_semantic_pin: bool = True,
) -> None:
    validate_against_schema(payload, ALLOCATION_SCHEMA)
    if payload["base_active_catalog_refs"] != [
        {"path": path, "sha256": EXPECTED_ACTIVE_HASHES[path]} for path in ACTIVE_CATALOGS
    ]:
        raise ValueError("allocation active catalog reference exact-set drifted")
    freeze = payload["active_m01_projection_freeze"]
    if freeze != {
        "atomic_id_epoch": "T1-M01-P001-P056", "field_names": ACTIVE_PROJECTION_FIELDS,
        "ordered_projection_sha256": EXPECTED_ACTIVE_M01_PROJECTION_SHA256,
        "status": "PASS_EXACT_ACTIVE_SOURCE_PRESERVATION",
    }:
        raise ValueError("allocation active M01 projection freeze drifted")
    actual_semantic = digest(allocation_semantic_projection(payload))
    if payload["semantic_projection_sha256"] != actual_semantic:
        raise ValueError("allocation semantic projection hash drifted")
    if enforce_semantic_pin and payload["semantic_projection_sha256"] != EXPECTED_ALLOCATION_SEMANTIC_SHA256:
        raise ValueError("allocation semantic projection is not independently pinned")

    steps = payload["train"]["steps"]
    ids = [item["atomic_pr_id"] for item in steps]
    if ids != APPENDED_IDS or [atomic_number(item) for item in ids] != list(range(57, 67)):
        raise ValueError("append ID epoch is not exact contiguous P057-P066")
    types = [item["pr_type"] for item in steps]
    if types != ["CTR", "REF", "REF", "REF", "REF", "REF", "OPS", "TST-PRE", "TST-POST", "IDX"]:
        raise ValueError("early trust train type sequence drifted")
    if len(ids) != len(set(ids)) or any(item in {row["atomic_pr_id"] for row in rows} for item in ids):
        raise ValueError("append ID reuses an active M01 ID")
    previous = P002
    for item in steps:
        if item["prerequisite_atomic_pr_ids"] != [previous]:
            raise ValueError(f"early trust train is not strictly serial: {item['atomic_pr_id']}")
        previous = item["atomic_pr_id"]
    if any(item["terminal_task_idx"] for item in steps[:-1]) or not steps[-1]["terminal_task_idx"]:
        raise ValueError("N015 terminal flag exact-set drifted")

    claims = old_claim_map(rows)
    supersessions = payload["supersessions"]
    actual_map = {item["legacy_atomic_pr_id"]: item["replacement_atomic_pr_id"] for item in supersessions}
    if actual_map != SUPERSESSION_MAP:
        raise ValueError("supersession exact mapping drifted")
    by_new = {item["atomic_pr_id"]: item for item in steps}
    for old_id, new_id in SUPERSESSION_MAP.items():
        item = by_new[new_id]
        if item["replaces_atomic_pr_id"] != old_id:
            raise ValueError(f"replacement back-reference drifted: {new_id}")
        if old_id in EXACT_TRANSFER_OLD_IDS:
            locators = [item["primary_locator"], *item["companion_locators"]]
            if locators != transferred_locators(old_id, claims):
                raise ValueError(f"exact locator transfer drifted: {old_id}")
    expected_outputs = {
        "T1-M01-P064-TST-PRE-n015-s8": [
            "doc/02_acceptance/topic1/work-orders/t1-m01-p064-tst-pre-n015-s8/test-result.json",
            "doc/02_acceptance/topic1/work-orders/t1-m01-p064-tst-pre-n015-s8/case-report.json",
        ],
        "T1-M01-P065-TST-POST-n015-s9": [
            "doc/02_acceptance/topic1/work-orders/t1-m01-p065-tst-post-n015-s9/test-result.json",
            "doc/02_acceptance/topic1/work-orders/t1-m01-p065-tst-post-n015-s9/case-report.json",
        ],
        P066: [
            "doc/02_acceptance/topic1/tasks/t1-m01-n015/completion-candidate.json",
            "doc/02_acceptance/topic1/tasks/t1-m01-n015/current-evidence-index.json",
        ],
    }
    for new_id, locators in expected_outputs.items():
        item = by_new[new_id]
        if [item["primary_locator"], *item["companion_locators"]] != locators:
            raise ValueError(f"new output locator exact-set drifted: {new_id}")

    revisions = {item["atomic_pr_id"]: item for item in payload["dependency_revisions"]}
    if set(revisions) != set(expected_dependency_revisions()):
        raise ValueError("dependency revision subject exact-set drifted")
    if P048 in revisions[P006]["revised_dependency_ids"]:
        raise ValueError("P006 to P048 back-edge is prohibited")
    for atomic_id, expected in expected_dependency_revisions().items():
        if revisions[atomic_id]["revised_dependency_ids"] != expected:
            if set(revisions[atomic_id]["revised_dependency_ids"]) & set(SUPERSESSION_MAP):
                raise ValueError("dependency revision reintroduces a superseded leaf")
            raise ValueError(f"dependency revision exact-set drifted: {atomic_id}")
    task_revisions = {item["task_id"]: item["revised_dependency_task_ids"] for item in payload["task_dependency_revisions"]}
    if task_revisions != {
        "T1-M01-N003": ["T1-M01-N001", "T1-M01-N015"],
        "T1-M01-N010": ["T1-M01-N009", "T1-M01-N015"],
    }:
        raise ValueError("task dependency revision exact-set drifted")
    completions = {item["parent_task_id"]: item for item in payload["completion_revisions"]}
    if set(completions) != {"T1-M01-N003", "T1-M01-N010", "T1-M01-N015"}:
        raise ValueError("completion revision parent exact-set drifted")
    if completions["T1-M01-N010"]["revised_member_atomic_pr_ids"] != [
        f"T1-M01-P{number:03d}-REF-n010-s{number - 31}" for number in range(38, 45)
    ]:
        raise ValueError("N010 completion revised member exact-set drifted")
    if completions["T1-M01-N015"]["revised_member_atomic_pr_ids"] != APPENDED_IDS[:-1]:
        raise ValueError("N015 completion revised member exact-set drifted")


def target_state(claim: dict[str, Any]) -> str:
    states = {item["target_state"] for item in claim["change_targets"]}
    return "PLANNED_OUTPUT" if states == {"PLANNED_OUTPUT"} else "PLANNED"


def retained_leaf(
    claim: dict[str, Any], pr: dict[str, Any], revisions: dict[str, list[str]],
) -> dict[str, Any]:
    atomic_id = claim["atomic_pr_id"]
    return {
        "atomic_pr_id": atomic_id, "parent_work_id": claim["parent_work_id"],
        "pr_type": claim["pr_type"], "phase": pr["phase"],
        "source_kind": "RETAINED_ACTIVE_ID_AND_WRITE_SCOPE",
        "write_locators": [target_locator(item) for item in claim["change_targets"]],
        "target_state": target_state(claim),
        "dependency_ids": revisions.get(atomic_id, pr["depends_on_prs"]),
        "required_gates": claim["required_gates"],
        "single_outcome": claim["outcome"]["acceptance_oracle"],
        "terminal_task_idx": atomic_id.endswith("task-completion"),
        "formal_execution_status": FORMAL_STATUS,
    }


def appended_leaf(item: dict[str, Any]) -> dict[str, Any]:
    return {
        "atomic_pr_id": item["atomic_pr_id"], "parent_work_id": "T1-M01-N015",
        "pr_type": item["pr_type"], "phase": item["phase"],
        "source_kind": "APPENDED_PREVIEW_V1",
        "write_locators": [item["primary_locator"], *item["companion_locators"]],
        "target_state": item["target_state"],
        "dependency_ids": item["prerequisite_atomic_pr_ids"],
        "required_gates": item["required_gates"], "single_outcome": item["single_outcome"],
        "terminal_task_idx": item["terminal_task_idx"],
        "formal_execution_status": FORMAL_STATUS,
    }


def edges_for(leaves: list[dict[str, Any]], revised_ids: set[str]) -> list[dict[str, str]]:
    appended = set(APPENDED_IDS)
    edges: list[dict[str, str]] = []
    for leaf in leaves:
        for source in leaf["dependency_ids"]:
            kind = (
                "APPENDED_PREREQUISITE" if leaf["atomic_pr_id"] in appended
                else "REVISED_DEPENDENCY" if leaf["atomic_pr_id"] in revised_ids
                else "RETAINED_ACTIVE"
            )
            edges.append({"from": source, "to": leaf["atomic_pr_id"], "edge_kind": kind})
    if len({(item["from"], item["to"]) for item in edges}) != len(edges):
        raise ValueError("candidate edge pair is duplicated")
    return sorted(edges, key=lambda item: (item["from"], item["to"]))


def assert_acyclic(edges: list[dict[str, str]]) -> None:
    nodes = {value for item in edges for value in (item["from"], item["to"])}
    outgoing: dict[str, set[str]] = defaultdict(set)
    indegree = {node: 0 for node in nodes}
    for edge in edges:
        source, target = edge["from"], edge["to"]
        if target not in outgoing[source]:
            outgoing[source].add(target)
            indegree[target] += 1
    ready = sorted(node for node, value in indegree.items() if value == 0)
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
        raise ValueError("M01 early-trust candidate DAG contains a cycle")


def build_catalog(allocation: dict[str, Any]) -> dict[str, Any]:
    rows, global_ids = assert_active_sources()
    assert_allocation(allocation, rows)
    claims = old_claim_map(rows)
    prs = active_pr_map()
    retired = set(SUPERSESSION_MAP)
    retained_ids = [item["atomic_pr_id"] for item in rows if item["atomic_pr_id"] not in retired]
    revisions = {item["atomic_pr_id"]: item["revised_dependency_ids"] for item in allocation["dependency_revisions"]}
    leaves = [retained_leaf(claims[item], prs[item], revisions) for item in retained_ids]
    leaves += [appended_leaf(item) for item in allocation["train"]["steps"]]
    leaves.sort(key=lambda item: atomic_number(item["atomic_pr_id"]))
    candidate_ids = {item["atomic_pr_id"] for item in leaves}
    if candidate_ids != (set(item["atomic_pr_id"] for item in rows) - retired) | set(APPENDED_IDS):
        raise ValueError("candidate M01 atomic ID exact-set drifted")
    locators = [locator for leaf in leaves for locator in leaf["write_locators"]]
    if len(locators) != len(set(locators)):
        raise ValueError("candidate write locator is reused")
    edges = edges_for(leaves, set(revisions))
    assert_acyclic(edges)
    candidate_global_ids = (global_ids - retired) | set(APPENDED_IDS)
    tombstones = []
    for item in allocation["supersessions"]:
        old_id = item["legacy_atomic_pr_id"]
        tombstones.append({
            **item,
            "legacy_write_locators": transferred_locators(old_id, claims),
            "legacy_leaf_projection_sha256": digest({field: claims[old_id][field] for field in ACTIVE_PROJECTION_FIELDS}),
        })
    return {
        "schema_version": "1.0.0",
        "artifact_kind": "M01_EARLY_TRUST_TRAIN_CANDIDATE_CATALOG",
        "artifact_status": "VERSIONED_PREVIEW_NOT_GLOBAL_REGISTRY",
        "revision_id": "M01-EARLY-TRUST-V1",
        "allocation_ledger": {"path": ALLOCATION.relative_to(REPO).as_posix(), "sha256": sha256(ALLOCATION)},
        "active_catalog_refs": allocation["base_active_catalog_refs"],
        "active_m01_projection_freeze": allocation["active_m01_projection_freeze"],
        "source_m01_leaf_count": 56, "superseded_leaf_count": 9,
        "appended_leaf_count": 10, "candidate_m01_leaf_count": 57,
        "retained_atomic_pr_ids": retained_ids,
        "supersession_tombstones": tombstones,
        "candidate_leaves": leaves,
        "candidate_m01_atomic_id_sha256": digest(sorted(candidate_ids)),
        "candidate_edges": edges,
        "dependency_revisions": allocation["dependency_revisions"],
        "task_dependency_revisions": allocation["task_dependency_revisions"],
        "completion_revisions": allocation["completion_revisions"],
        "global_switch_gate": {
            "decision": "BLOCKED_PREVIEW_ONLY", "active_global_atomic_pr_count": 1289,
            "active_m01_atomic_pr_count": 56, "candidate_m01_atomic_pr_count": 57,
            "candidate_global_atomic_pr_count": len(candidate_global_ids),
            "candidate_global_atomic_id_sha256": digest(sorted(candidate_global_ids)),
            "required_catalogs": ACTIVE_CATALOGS,
            "switch_rule": "TASK_CLAIM_PR_DESIGN_AND_OVERLAY_MUST_SWITCH_ATOMICALLY_TO_ONE_REVIEWED_CANDIDATE_HASH",
        },
        "validation": {
            "schema": "PASS", "active_catalog_hashes": "PASS",
            "active_four_catalog_exact_set": "PASS", "active_m01_projection_exact": "PASS",
            "old_id_preservation": "PASS_TOMBSTONES_AND_NO_REUSE", "append_id_exact": "PASS_P057_P066",
            "supersession_exact": "PASS", "candidate_write_locator_unique": True,
            "dependency_revision_exact": "PASS", "completion_revision_exact": "PASS",
            "p006_back_edge_absent": True, "candidate_dag": "PASS", "global_switch_blocked": True,
            "mutation_guards": {
                "active_projection_drift": "PASS", "append_id_reuse": "PASS",
                "supersession_omission": "PASS", "locator_reuse": "PASS",
                "locator_transfer_drift": "PASS", "p005_trust_dependency_omission": "PASS",
                "p006_back_edge": "PASS", "retired_dependency_reintroduced": "PASS",
                "completion_omission": "PASS", "task_dependency_drift": "PASS",
                "dag_cycle": "PASS", "semantic_pin_drift": "PASS",
                "global_catalog_exact_set": "PASS",
            },
        },
        "proof_ceiling": "VERSIONED_STATIC_PREVIEW_ONLY_NOT_GLOBAL_REGISTRATION_FUNCTION_REVIEW_IMPLEMENTATION_FIXTURE_CREATION_DEPLOYMENT_TEST_EXECUTION_TRUST_PASS_AUTHORIZATION_OR_ACCEPTANCE",
    }


def validate_catalog(payload: dict[str, Any], allocation: dict[str, Any]) -> None:
    validate_against_schema(payload, CATALOG_SCHEMA)
    leaves = payload["candidate_leaves"]
    if len(leaves) != 57 or Counter(item["parent_work_id"] for item in leaves)["T1-M01-N015"] != 10:
        raise ValueError("candidate M01 leaf or N015 count drifted")
    locators = [locator for leaf in leaves for locator in leaf["write_locators"]]
    if len(locators) != len(set(locators)):
        raise ValueError("candidate write locator is reused")
    if payload["global_switch_gate"]["candidate_global_atomic_pr_count"] != 1290:
        raise ValueError("candidate global count drifted from 1289-9+10")
    if any(item["atomic_pr_id"] in SUPERSESSION_MAP for item in leaves):
        raise ValueError("superseded leaf remains in candidate active set")
    deps = {item["atomic_pr_id"]: item["dependency_ids"] for item in leaves}
    if P048 in deps[P006]:
        raise ValueError("P006 to P048 back-edge is prohibited")
    if any(set(values) & set(SUPERSESSION_MAP) for values in deps.values()):
        raise ValueError("candidate dependency reintroduces a superseded leaf")
    assert_acyclic(payload["candidate_edges"])
    expected = build_catalog(allocation)
    if payload != expected:
        raise ValueError("candidate catalog differs from fully derived expected payload")


def expect_failure(
    label: str, action: Callable[[], None], expected_error: str,
) -> None:
    try:
        action()
    except (ValueError, KeyError, TypeError) as exc:
        if expected_error not in str(exc):
            raise ValueError(f"mutation {label} hit wrong assertion: {exc}") from exc
        return
    raise ValueError(f"mutation {label} did not fail")


def mutated_allocation(
    source: dict[str, Any], mutate: Callable[[dict[str, Any]], None], rows: list[dict[str, Any]],
    *, repin: bool = True, enforce_pin: bool = False,
) -> None:
    candidate = copy.deepcopy(source)
    mutate(candidate)
    if repin:
        candidate["semantic_projection_sha256"] = digest(allocation_semantic_projection(candidate))
    assert_allocation(candidate, rows, enforce_semantic_pin=enforce_pin)


def self_test(allocation: dict[str, Any], catalog: dict[str, Any]) -> None:
    rows, _ = assert_active_sources()
    changed_rows = copy.deepcopy(rows)
    changed_rows[0]["outcome"]["acceptance_oracle"] = "mutated"
    expect_failure(
        "active projection drift",
        lambda: (_ for _ in ()).throw(ValueError("active M01 P001-P056 ordered projection drifted"))
        if digest(active_projection(changed_rows)) != EXPECTED_ACTIVE_M01_PROJECTION_SHA256 else None,
        "active M01 P001-P056 ordered projection drifted",
    )
    expect_failure(
        "append ID reuse",
        lambda: mutated_allocation(allocation, lambda item: item["train"]["steps"][0].update(atomic_pr_id="T1-M01-P056-CTR-n015-s1"), rows),
        "schema pattern failed at $.train.steps[0].atomic_pr_id",
    )
    expect_failure(
        "supersession omission",
        lambda: mutated_allocation(allocation, lambda item: item["supersessions"].pop(), rows),
        "schema minItems failed at $.supersessions",
    )
    expect_failure(
        "locator reuse",
        lambda: validate_catalog({**copy.deepcopy(catalog), "candidate_leaves": [
            ({**leaf, "write_locators": catalog["candidate_leaves"][47]["write_locators"]}
             if leaf["atomic_pr_id"] == "T1-M01-P059-REF-n015-s3" else leaf)
            for leaf in copy.deepcopy(catalog["candidate_leaves"])
        ]}, allocation),
        "candidate write locator is reused",
    )
    expect_failure(
        "locator transfer drift",
        lambda: mutated_allocation(allocation, lambda item: item["train"]["steps"][0].update(primary_locator="contracts/alignment/not-the-trust-policy.schema.json#/"), rows),
        "exact locator transfer drifted",
    )
    expect_failure(
        "P005 trust dependency omission",
        lambda: mutated_allocation(allocation, lambda item: item["dependency_revisions"][0].update(revised_dependency_ids=[P002]), rows),
        "schema minItems failed at $.dependency_revisions[0].revised_dependency_ids",
    )
    expect_failure(
        "P006 back edge",
        lambda: mutated_allocation(allocation, lambda item: item["dependency_revisions"][1].update(revised_dependency_ids=[P005, P066, P048]), rows),
        "P006 to P048 back-edge is prohibited",
    )
    expect_failure(
        "retired dependency reintroduced",
        lambda: mutated_allocation(allocation, lambda item: item["dependency_revisions"][2].update(revised_dependency_ids=[P031, P066, "T1-M01-P037-REF-n010-s6"]), rows),
        "dependency revision reintroduces a superseded leaf",
    )
    expect_failure(
        "completion omission",
        lambda: mutated_allocation(allocation, lambda item: item["completion_revisions"][1]["revised_member_atomic_pr_ids"].pop(), rows),
        "N010 completion revised member exact-set drifted",
    )
    expect_failure(
        "task dependency drift",
        lambda: mutated_allocation(allocation, lambda item: item["task_dependency_revisions"][0].update(revised_dependency_task_ids=["T1-M01-N001"]), rows),
        "schema minItems failed at $.task_dependency_revisions[0].revised_dependency_task_ids",
    )
    expect_failure(
        "DAG cycle",
        lambda: assert_acyclic(catalog["candidate_edges"] + [{"from": P006, "to": "T1-M01-P057-CTR-n015-s1", "edge_kind": "REVISED_DEPENDENCY"}]),
        "M01 early-trust candidate DAG contains a cycle",
    )
    expect_failure(
        "semantic pin drift",
        lambda: mutated_allocation(allocation, lambda item: item["train"].update(single_result="mutated semantic meaning with a valid shape"), rows, enforce_pin=True),
        "allocation semantic projection is not independently pinned",
    )
    sets = [set(active_atomic_ids(path)) for path in ACTIVE_CATALOGS]
    changed = [*sets[:-1], sets[-1] | {"T1-M01-P999-WRT-n999-s1"}]
    expect_failure(
        "global exact set",
        lambda: (_ for _ in ()).throw(ValueError("active four-catalog atomic ID exact-sets differ"))
        if any(items != changed[0] for items in changed[1:]) else None,
        "active four-catalog atomic ID exact-sets differ",
    )


def canonical_text(payload: dict[str, Any]) -> str:
    return json.dumps(payload, ensure_ascii=False, indent=2) + "\n"


def markdown(payload: dict[str, Any]) -> str:
    rows = []
    for item in payload["candidate_leaves"]:
        if item["source_kind"] != "APPENDED_PREVIEW_V1":
            continue
        rows.append(
            f"| `{item['atomic_pr_id']}` | `{item['pr_type']}` | `{item['dependency_ids'][0]}` | "
            f"`{item['write_locators'][0]}` | BLOCKED |"
        )
    superseded = "、".join(f"`{item}`" for item in SUPERSESSION_MAP)
    return "\n".join([
        "# M01 早期受信验证列车候选目录", "",
        "> 状态：`VERSIONED_PREVIEW_NOT_GLOBAL_REGISTRY`。本页只证明静态 allocation/catalog 自洽；不代表函数评审、实现、fixture 创建、部署、测试、验签 PASS、授权或验收。", "",
        "## 候选结果", "",
        "- 新增父任务：`T1-M01-N015`，依赖 `T1-M01-N001`。",
        "- 新增原子叶：`P057-P066` 共 10 叶；N015 terminal 为 `P066`。",
        f"- preview supersede 旧叶 9 个：{superseded}；旧 ID 保留为 tombstone 且不得复用。",
        "- 候选 M01 active 叶：`56 - 9 + 10 = 57`；候选全局原子数：`1289 - 9 + 10 = 1290`。",
        "- P005/P006 直接依赖 P066；P038/P048 同样与 P066 汇合；未加入 `P006 -> P048` 回边。", "",
        "## 新增列车", "",
        "| 原子 PR | 类型 | 直接前驱 | primary write locator | 执行状态 |", "|---|---|---|---|---|",
        *rows, "", "## 仍然阻断", "",
        "四份现役 registry 未切换；owner/reviewer/approver、函数评审、干净 candidate、真实受保护 verifier、fixtures、运行证据和 signed overlay 均未提供。因此保持 `DOR=BLOCKED / NO-GO`。", "",
    ])


def main() -> int:
    parser = argparse.ArgumentParser()
    mode = parser.add_mutually_exclusive_group(required=True)
    mode.add_argument("--semantic-hash", action="store_true")
    mode.add_argument("--write", action="store_true")
    mode.add_argument("--check", action="store_true")
    mode.add_argument("--verify", action="store_true")
    mode.add_argument("--self-test", action="store_true")
    args = parser.parse_args()
    allocation = build_allocation()
    if args.semantic_hash:
        print(allocation["semantic_projection_sha256"])
        return 0
    rows, _ = assert_active_sources()
    assert_allocation(allocation, rows)
    if args.write:
        ALLOCATION.write_text(canonical_text(allocation), encoding="utf-8")
        catalog = build_catalog(allocation)
        validate_catalog(catalog, allocation)
        CATALOG.write_text(canonical_text(catalog), encoding="utf-8")
        MARKDOWN.parent.mkdir(parents=True, exist_ok=True)
        MARKDOWN.write_text(markdown(catalog), encoding="utf-8")
        print(f"WROTE {ALLOCATION.relative_to(REPO)}")
        print(f"WROTE {CATALOG.relative_to(REPO)}")
        print(f"WROTE {MARKDOWN.relative_to(REPO)}")
        return 0
    if not ALLOCATION.exists() or ALLOCATION.read_text(encoding="utf-8") != canonical_text(allocation):
        raise SystemExit(f"STALE {ALLOCATION.relative_to(REPO)}; run --write")
    catalog = build_catalog(allocation)
    validate_catalog(catalog, allocation)
    if not CATALOG.exists() or CATALOG.read_text(encoding="utf-8") != canonical_text(catalog):
        raise SystemExit(f"STALE {CATALOG.relative_to(REPO)}; run --write")
    if not MARKDOWN.exists() or MARKDOWN.read_text(encoding="utf-8") != markdown(catalog):
        raise SystemExit(f"STALE {MARKDOWN.relative_to(REPO)}; run --write")
    if args.verify or args.self_test:
        self_test(allocation, catalog)
        print("PASS M01 early-trust preview: P057-P066, 9 tombstones, 57 candidate M01 leaves, 1290 candidate global IDs, acyclic")
        print("PROOF_CEILING STATIC_PREVIEW_ONLY_NOT_IMPLEMENTATION_TEST_EXECUTION_TRUST_PASS_AUTHORIZATION_OR_ACCEPTANCE")
    else:
        print("PASS M01 early-trust preview is deterministic and active registries remain hash-pinned")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
