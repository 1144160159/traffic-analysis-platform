#!/usr/bin/env python3
"""Fail-closed semantic checks for DESIGN_SCHEMA_ONLY function-design artifacts.

This validator does not grant execution authorization.  It validates shape and
cross-field invariants for the GoF catalog, PR-level pattern decisions, debate
receipts, and code-unit contracts.  Formal execution remains governed by the
existing execution-package/overlay validator.
"""

from __future__ import annotations

import argparse
from datetime import datetime
import hashlib
import json
import re
from pathlib import Path
from typing import Any

from build_topic1_task_registry import validate_against_schema


REPO_ROOT = Path(__file__).resolve().parents[2]
CONTRACT_ROOT = REPO_ROOT / "contracts" / "alignment"
CATALOG_PATH = CONTRACT_ROOT / "gof-pattern-catalog.v1.json"
CATALOG_SCHEMA_PATH = CONTRACT_ROOT / "gof-pattern-catalog.schema.json"
PROPOSAL_SCHEMA_PATH = CONTRACT_ROOT / "pattern-proposal.schema.json"
DECISION_SCHEMA_PATH = CONTRACT_ROOT / "pattern-decision.schema.json"
DEBATE_SCHEMA_PATH = CONTRACT_ROOT / "pattern-debate-receipt.schema.json"
CODE_UNIT_SCHEMA_PATH = CONTRACT_ROOT / "code-unit-contract.schema.json"
FUNCTION_REVIEW_SCHEMA_PATH = CONTRACT_ROOT / "function-design-review-receipt.schema.json"

CANONICAL_IDS = {
    *(f"GOF-CRE-{index:02d}" for index in range(1, 6)),
    *(f"GOF-STR-{index:02d}" for index in range(1, 8)),
    *(f"GOF-BEH-{index:02d}" for index in range(1, 12)),
}
PLACEHOLDER = re.compile(r"^(?:n/?a|tbd|todo|待补|unknown|x)$", re.IGNORECASE)
CORE_ROLES = {
    "DOMAIN_OWNER",
    "LANGUAGE_OWNER",
    "RELIABILITY_DATA_EXPERT",
    "SECURITY_TENANT_EXPERT",
    "QA_SRE_PERFORMANCE_EXPERT",
    "MAINTAINABILITY_RED_TEAM",
}


def load_json(path: Path) -> dict[str, Any]:
    payload = json.loads(path.read_text(encoding="utf-8"))
    if not isinstance(payload, dict):
        raise ValueError(f"expected JSON object: {path}")
    return payload


def walk_strings(value: Any, location: str = "$") -> list[tuple[str, str]]:
    found: list[tuple[str, str]] = []
    if isinstance(value, str):
        found.append((location, value))
    elif isinstance(value, dict):
        for key, child in value.items():
            found.extend(walk_strings(child, f"{location}.{key}"))
    elif isinstance(value, list):
        for index, child in enumerate(value):
            found.extend(walk_strings(child, f"{location}[{index}]"))
    return found


def reject_placeholders(payload: dict[str, Any]) -> None:
    for location, value in walk_strings(payload):
        if PLACEHOLDER.fullmatch(value.strip()):
            raise ValueError(f"placeholder value rejected at {location}: {value!r}")


def require_unique(values: list[str], label: str) -> None:
    if len(values) != len(set(values)):
        raise ValueError(f"duplicate {label}: {values}")


def resolve_hashed_ref(ref: dict[str, str]) -> Path:
    path = (REPO_ROOT / ref["path"]).resolve()
    if not path.is_relative_to(REPO_ROOT) or not path.is_file():
        raise ValueError(f"hashed reference is outside repository or missing: {ref['path']}")
    actual = hashlib.sha256(path.read_bytes()).hexdigest()
    if actual != ref["sha256"]:
        raise ValueError(f"hashed reference mismatch: {ref['path']}")
    return path


def canonical_sha256(value: Any) -> str:
    return hashlib.sha256(
        json.dumps(value, ensure_ascii=False, sort_keys=True, separators=(",", ":")).encode("utf-8")
    ).hexdigest()


def resolve_typed_ref(ref: dict[str, str], expected_kind: str) -> tuple[Path, dict[str, Any]]:
    path = resolve_hashed_ref(ref)
    payload = load_json(path)
    if payload.get("artifact_kind") != expected_kind:
        raise ValueError(
            f"typed reference expected {expected_kind}, got {payload.get('artifact_kind')!r}: {ref['path']}"
        )
    return path, payload


def require_keys(details: dict[str, Any], keys: set[str], label: str) -> None:
    missing = sorted(keys - set(details))
    if missing:
        raise ValueError(f"{label} profile contract is missing {missing}")


PROFILE_LANGUAGE_KIND: dict[str, tuple[set[str], set[str]]] = {
    "PURE@1": ({"go", "rust", "java", "typescript", "python"}, {"function", "method"}),
    "CONSTRUCTOR@1": ({"go", "rust", "java", "typescript", "python"}, {"constructor", "function", "method"}),
    "IO_ADAPTER@1": ({"go", "rust", "java", "typescript", "python"}, {"function", "method"}),
    "AUTHORITY_TX@1": ({"go", "rust", "java", "python"}, {"function", "method"}),
    "STATE_TRANSITION@1": ({"go", "rust", "java", "typescript", "python"}, {"function", "method"}),
    "WORKER@1": ({"go", "rust", "java", "python"}, {"worker", "function", "method"}),
    "CONSUMER@1": ({"go", "rust", "java", "python"}, {"consumer", "function", "method"}),
    "PRODUCER@1": ({"go", "rust", "java", "python"}, {"producer", "function", "method"}),
    "FLINK_OPERATOR@1": ({"java"}, {"function", "method"}),
    "UI_DECODER@1": ({"typescript"}, {"function"}),
    "UI_HOOK@1": ({"typescript"}, {"hook"}),
    "UI_VIEW_MODEL@1": ({"typescript"}, {"function"}),
    "UI_COMPONENT@1": ({"typescript"}, {"component"}),
    "CONFIG_WIRING@1": ({"go", "rust", "java", "typescript", "yaml", "json", "shell", "python"}, {"config_entrypoint", "function", "method"}),
}

PROFILE_REQUIRED_DETAILS: dict[str, set[str]] = {
    "PURE@1": {"determinism", "input_output_invariants"},
    "STATE_TRANSITION@1": {"states", "allowed_edges", "illegal_edges", "terminal_states", "revision_policy"},
    "AUTHORITY_TX@1": {"authority", "isolation", "lock_order", "idempotency_key", "payload_hash", "included_effect_ids", "commit_step", "crash_matrix", "commit_unknown_recovery"},
    "WORKER@1": {"owner", "queue_capacity", "overflow_policy", "backpressure", "concurrency", "retry", "dlq", "startup_recovery", "ack", "shutdown", "drain", "orphan_recovery"},
    "CONSUMER@1": {"owner", "queue_capacity", "backpressure", "retry", "dlq", "startup_recovery", "ack", "shutdown", "drain"},
    "PRODUCER@1": {"owner", "queue_capacity", "backpressure", "retry", "ack", "shutdown", "drain"},
    "FLINK_OPERATOR@1": {"operator_uid", "max_parallelism", "key_by", "state_descriptors", "serializer_snapshot", "ttl", "watermark", "late_side_output", "checkpoint", "savepoint", "source_sink_semantics"},
    "UI_DECODER@1": {"runtime_schema", "unknown_enum_policy", "revision_representation", "error_mapping"},
    "UI_HOOK@1": {"query_key_tenant", "query_key_session_epoch", "abort_signal", "unknown_receipt_recovery", "cache_invalidation"},
    "UI_COMPONENT@1": {"permission_states", "loading_empty_error", "disabled_readonly", "aria_contract", "focus_restore", "keyboard_contract"},
}


def validate_catalog(payload: dict[str, Any]) -> None:
    validate_against_schema(payload, CATALOG_SCHEMA_PATH)
    patterns = payload["patterns"]
    if set(patterns) != CANONICAL_IDS:
        raise ValueError("catalog pattern keys are not the canonical exact set")
    counts = {"CREATIONAL": 0, "STRUCTURAL": 0, "BEHAVIORAL": 0}
    for key, item in patterns.items():
        if item["pattern_id"] != key or item["rule_ref"] != key:
            raise ValueError(f"catalog identity mismatch for {key}")
        counts[item["category"]] += 1
        if item["source_summary"]["applicability"] == [item["project_adaptation"]["adoption_predicate"]]:
            raise ValueError(f"source applicability copies project adaptation for {key}")
    fingerprints = [json.dumps(item, ensure_ascii=False, sort_keys=True) for item in patterns.values()]
    require_unique(fingerprints, "catalog pattern body")
    if counts != {"CREATIONAL": 5, "STRUCTURAL": 7, "BEHAVIORAL": 11}:
        raise ValueError(f"catalog category count mismatch: {counts}")
    for source_ref in payload["source_refs"]:
        resolve_hashed_ref(source_ref)
    reject_placeholders(payload)


def derived_decision_id(atomic_pr_id: str) -> str:
    return f"PAT-{atomic_pr_id}"


def validate_pattern_proposal(payload: dict[str, Any]) -> None:
    validate_against_schema(payload, PROPOSAL_SCHEMA_PATH)
    reject_placeholders(payload)
    if payload["decision_id"] != derived_decision_id(payload["atomic_pr_id"]):
        raise ValueError("proposal decision_id is not derived from atomic_pr_id")
    if not payload["proposal_id"].startswith(f"PROP-{payload['atomic_pr_id']}-r"):
        raise ValueError("proposal_id is not derived from atomic_pr_id")
    trigger = payload["trigger_evidence"]
    if "NONE" in trigger and trigger != ["NONE"]:
        raise ValueError("NONE trigger cannot be combined with a real trigger")
    option_ids = [item["option_id"] for item in payload["options"]]
    require_unique(option_ids, "proposal option")
    for option in payload["options"]:
        designs = option["selected_designs"]
        pattern_ids = [item["pattern_id"] for item in designs]
        require_unique(pattern_ids, f"selected design in {option['option_id']}")
        primaries = [item for item in designs if item["selection_role"] == "PRIMARY"]
        constraints = option["distributed_constraint_ids"]
        primary_constraint = option["primary_distributed_constraint_id"]
        if primary_constraint is not None and primary_constraint not in constraints:
            raise ValueError("primary distributed constraint is outside the option exact-set")
        form = option["implementation_form"]
        if form in {"GOF", "NATIVE_LANGUAGE"} and (len(primaries) != 1 or primaries[0]["implementation_form"] != form):
            raise ValueError("GoF/native option requires exactly one primary design matching the option form")
        if form == "PROJECT_ADAPTATION" and (designs or not constraints or primary_constraint is None):
            raise ValueError("project option requires only a primary distributed constraint")
        if form in {"DIRECT", "NOT_APPLICABLE"} and designs:
            raise ValueError("direct/non-applicable option cannot select a GoF pattern")
        if form == "NOT_APPLICABLE" and (constraints or option["participant_bindings"]):
            raise ValueError("non-applicable option cannot carry constraints or participants")
        if option["participant_bindings"]:
            raise ValueError("option-level participant bindings are forbidden; bind participants per selected design")
    if canonical_sha256(sorted(payload["options"], key=lambda item: item["option_id"])) != payload["option_set_sha256"]:
        raise ValueError("proposal option-set digest mismatch")
    if payload["artifact_status"] == "FROZEN_CANDIDATE":
        if payload["readiness"]["blockers"]:
            raise ValueError("frozen proposal contains blockers")
        resolve_typed_ref(payload["catalog_ref"], "GOF_PATTERN_CATALOG")
        resolve_hashed_ref(payload["negative_test_manifest_ref"])


def validate_pattern_decision(payload: dict[str, Any]) -> None:
    validate_against_schema(payload, DECISION_SCHEMA_PATH)
    reject_placeholders(payload)
    if payload["decision_id"] != derived_decision_id(payload["atomic_pr_id"]):
        raise ValueError("decision_id is not deterministically derived from atomic_pr_id")
    _, proposal = resolve_typed_ref(payload["proposal_ref"], "PATTERN_DECISION_PROPOSAL")
    _, debate = resolve_typed_ref(payload["debate_receipt_ref"], "PATTERN_DEBATE_RECEIPT")
    validate_pattern_proposal(proposal)
    validate_debate(debate)
    if proposal["artifact_status"] != "FROZEN_CANDIDATE":
        raise ValueError("final ADR requires a frozen proposal")
    if debate["review_disposition"] != "UNIFIED":
        raise ValueError("final ADR requires a UNIFIED debate")
    if (
        debate["proposal_ref"]["proposal_id"] != payload["proposal_ref"]["proposal_id"]
        or debate["proposal_ref"]["sha256"] != payload["proposal_ref"]["sha256"]
    ):
        raise ValueError("final ADR and debate bind different proposal revisions")
    if proposal["decision_id"] != payload["decision_id"] or debate["decision_id"] != payload["decision_id"]:
        raise ValueError("final ADR upstream decision identity mismatch")
    if proposal["candidate"]["manifest_sha256"] != payload["candidate"]["manifest_sha256"] or debate["candidate"]["manifest_sha256"] != payload["candidate"]["manifest_sha256"]:
        raise ValueError("final ADR upstream candidate mismatch")
    options = {item["option_id"]: item for item in proposal["options"]}
    dispositions = payload["option_dispositions"]
    if {item["option_id"] for item in dispositions} != set(options):
        raise ValueError("final ADR disposition set differs from proposal option set")
    selected = [item["option_id"] for item in dispositions if item["status"] == "SELECTED"]
    if selected != [payload["selected_option_id"]] or payload["selected_option_id"] != debate["selected_option_id"]:
        raise ValueError("final ADR must select exactly the debated option")
    selected_option = options[payload["selected_option_id"]]
    if canonical_sha256(selected_option) != payload["selected_option_sha256"]:
        raise ValueError("final ADR selected-option digest mismatch")
    if payload["selected_option_sha256"] != debate["canonical_review_payload"]["selected_option_sha256"]:
        raise ValueError("final ADR selected-option digest differs from debate receipt")
    expected_outcome = canonical_sha256({
        "selected_option_id": payload["selected_option_id"],
        "selected_option_sha256": payload["selected_option_sha256"],
    })
    if payload["decision_outcome_sha256"] != expected_outcome:
        raise ValueError("final ADR outcome digest is not derived from its selected option")
    if payload["decision_outcome_sha256"] != debate["canonical_review_payload"]["decision_outcome_sha256"]:
        raise ValueError("final ADR outcome digest differs from debate receipt")
    if payload["artifact_status"] == "READY" and payload["readiness"]["blockers"]:
        raise ValueError("READY final ADR cannot carry blockers")


def validate_debate(payload: dict[str, Any]) -> None:
    validate_against_schema(payload, DEBATE_SCHEMA_PATH)
    reject_placeholders(payload)
    if payload["decision_id"] != derived_decision_id(payload["atomic_pr_id"]):
        raise ValueError("debate decision_id is not derived from atomic_pr_id")
    if payload["final_round"] > payload["rounds"]:
        raise ValueError("final_round exceeds total rounds")
    if payload["final_round"] != payload["rounds"]:
        raise ValueError("final_round must equal total rounds")
    try:
        datetime.fromisoformat(payload["signed_at"].replace("Z", "+00:00"))
    except ValueError as exc:
        raise ValueError("signed_at is not a real RFC3339 UTC timestamp") from exc
    _, proposal = resolve_typed_ref(payload["proposal_ref"], "PATTERN_DECISION_PROPOSAL")
    if proposal["decision_id"] != payload["decision_id"]:
        raise ValueError("debate references a proposal for another decision")
    if proposal["candidate"]["manifest_sha256"] != payload["candidate"]["manifest_sha256"]:
        raise ValueError("debate and proposal candidate mismatch")
    options = {item["option_id"]: item for item in proposal["options"]}
    submissions = payload["final_submissions"]
    if set(submissions) != CORE_ROLES:
        raise ValueError("final submissions do not cover the exact core roles")
    identities = [item["reviewer_identity"] for item in submissions.values()]
    require_unique(identities, "final reviewer identity")
    if payload["adjudication"]["reviewer_identity"] in identities:
        raise ValueError("adjudicator must be independent from core reviewers")
    if any(item["round"] != payload["final_round"] for item in submissions.values()):
        raise ValueError("each final submission must bind the final round")
    for item in submissions.values():
        resolve_hashed_ref(item["body_ref"])
    resolve_hashed_ref(payload["adjudication"]["body_ref"])
    if payload["review_disposition"] == "UNIFIED":
        if payload["unresolved_p0"]:
            raise ValueError("UNIFIED debate contains unresolved P0")
        if any(item["status"] == "ACTIVE" for item in payload["vetoes"]):
            raise ValueError("UNIFIED debate contains an active veto")
        if any(item["result"] != "PASS" for item in payload["negative_test_results"]):
            raise ValueError("UNIFIED debate contains a non-PASS negative test")
        outcomes = {item["recommended_option_id"] for item in submissions.values()}
        dispositions = {item["review_disposition"] for item in submissions.values()}
        if outcomes != {payload["selected_option_id"]} or dispositions != {"UNIFIED"}:
            raise ValueError("final expert submissions are not unanimous")
        if payload["adjudication"]["review_disposition"] != "UNIFIED":
            raise ValueError("adjudicator did not record UNIFIED")
        if payload["adjudication"]["selected_option_id"] != payload["selected_option_id"]:
            raise ValueError("adjudicator selected a different option")
        if payload["selected_option_id"] not in options:
            raise ValueError("debate selected an option outside the proposal")
    attestations = payload["attestations"]
    expected_roles = CORE_ROLES | {"ADJUDICATOR"}
    if set(attestations) != expected_roles:
        raise ValueError("attestations do not cover the exact role set")
    signed_identities = [item["reviewer_identity"] for item in attestations.values()]
    require_unique(signed_identities, "attestation identity")
    payload_hashes = {item["payload_sha256"] for item in attestations.values()}
    if payload_hashes != {payload["canonical_review_payload_sha256"]}:
        raise ValueError("attestations do not bind the canonical review payload")
    for role, item in attestations.items():
        expected_identity = payload["adjudication"]["reviewer_identity"] if role == "ADJUDICATOR" else submissions[role]["reviewer_identity"]
        if item["reviewer_identity"] != expected_identity:
            raise ValueError(f"{role} attestation identity does not match review identity")
        resolve_hashed_ref(item["signature_artifact_ref"])
    for result in payload["negative_test_results"]:
        resolve_hashed_ref(result["artifact_ref"])
    canonical = payload["canonical_review_payload"]
    canonical_hash = canonical_sha256(canonical)
    if canonical_hash != payload["canonical_review_payload_sha256"]:
        raise ValueError("canonical review payload digest mismatch")
    if canonical["candidate_manifest_sha256"] != payload["candidate"]["manifest_sha256"]:
        raise ValueError("canonical review payload candidate mismatch")
    if canonical["proposal_sha256"] != payload["proposal_ref"]["sha256"]:
        raise ValueError("canonical review payload proposal mismatch")
    if canonical["option_set_sha256"] != proposal["option_set_sha256"]:
        raise ValueError("canonical review payload option set mismatch")
    selected_option = options[payload["selected_option_id"]]
    if canonical["selected_option_id"] != payload["selected_option_id"] or canonical["selected_option_sha256"] != canonical_sha256(selected_option):
        raise ValueError("canonical review payload selected option mismatch")
    expected_outcome = canonical_sha256({
        "selected_option_id": canonical["selected_option_id"],
        "selected_option_sha256": canonical["selected_option_sha256"],
    })
    if canonical["decision_outcome_sha256"] != expected_outcome:
        raise ValueError("canonical decision outcome digest mismatch")
    catalog_path = resolve_hashed_ref(proposal["catalog_ref"])
    if canonical["catalog_sha256"] != hashlib.sha256(catalog_path.read_bytes()).hexdigest():
        raise ValueError("canonical review payload catalog mismatch")
    negative_path = resolve_hashed_ref(proposal["negative_test_manifest_ref"])
    if canonical["negative_test_manifest_sha256"] != hashlib.sha256(negative_path.read_bytes()).hexdigest():
        raise ValueError("canonical negative-test manifest mismatch")


def locator_ids(payload: dict[str, Any]) -> set[str]:
    return {
        *(item["locator_id"] for item in payload["context_locators"]),
        *(item["locator"]["locator_id"] for item in payload["code_units"]),
    }


def validate_code_unit_contract(payload: dict[str, Any]) -> None:
    validate_against_schema(payload, CODE_UNIT_SCHEMA_PATH)
    reject_placeholders(payload)
    units = payload["code_units"]
    unit_ids = [item["unit_id"] for item in units]
    require_unique(unit_ids, "code unit")
    primary = [item for item in units if item["primary"]]
    if payload["sql_migration_design"] is None and len(primary) != 1:
        raise ValueError(f"code-unit contract requires exactly one primary, got {len(primary)}")
    if payload["sql_migration_design"] is not None and units:
        raise ValueError("SQL migration contract cannot carry fake function code units")
    raw_locator_ids = [item["locator_id"] for item in payload["context_locators"]] + [item["locator"]["locator_id"] for item in units]
    require_unique(raw_locator_ids, "locator")
    known_locators = set(raw_locator_ids)
    for unit in units:
        locator = unit["locator"]
        allowed_languages, allowed_kinds = PROFILE_LANGUAGE_KIND[unit["profile_ref"]]
        if locator["language"] not in allowed_languages or unit["kind"] not in allowed_kinds:
            raise ValueError(f"{unit['profile_ref']} is incompatible with language/kind")
        expected_profile = unit["profile_ref"].removesuffix("@1")
        if unit["profile_contract"]["profile"] != expected_profile:
            raise ValueError("profile_contract.profile does not match profile_ref")
        require_keys(unit["profile_contract"]["details"], PROFILE_REQUIRED_DETAILS.get(unit["profile_ref"], {"contract"}), unit["profile_ref"])
        if unit["change_kind"] == "new" and locator["target_state"] != "PLANNED":
            raise ValueError("new code unit must use a PLANNED locator")
        if unit["change_kind"] in {"modify", "delete"} and locator["target_state"] != "EXISTING":
            raise ValueError("modify/delete code unit must use an EXISTING locator")
        if locator["target_state"] == "PLANNED" and locator["created_by_atomic_pr_id"] != payload["atomic_pr_id"]:
            raise ValueError("planned locator creator is not the current atomic PR")
        step_ids = [item["step_id"] for item in unit["body_steps"]]
        effect_ids = [item["effect_id"] for item in unit["side_effects"]]
        error_ids = [item["error_id"] for item in unit["errors"]]
        case_ids = [item["case_id"] for item in unit["tests"]]
        require_unique(step_ids, f"step in {unit['unit_id']}")
        require_unique(effect_ids, f"effect in {unit['unit_id']}")
        require_unique(error_ids, f"error in {unit['unit_id']}")
        require_unique(case_ids, f"test case in {unit['unit_id']}")
        for link in [*unit["callers"], *unit["callees"]]:
            if link["locator_id"] not in known_locators:
                raise ValueError(f"unknown caller/callee locator {link['locator_id']}")
        for step in unit["body_steps"]:
            if not set(step["error_ids"]).issubset(error_ids):
                raise ValueError(f"step {step['step_id']} references unknown error")
            invoke_ids = {value for value in step["invokes"] if value.startswith("LOC-")}
            if not invoke_ids.issubset(known_locators):
                raise ValueError(f"step {step['step_id']} invokes an unknown locator")
        for effect in unit["side_effects"]:
            if effect["body_step_id"] not in step_ids:
                raise ValueError(f"effect {effect['effect_id']} references unknown step")
        covered_steps = {step for test in unit["tests"] for step in test["covers_steps"]}
        if covered_steps != set(step_ids):
            raise ValueError(f"tests do not cover the exact step set in {unit['unit_id']}")
        test_oracles = {oracle for test in unit["tests"] for oracle in test["oracle_ids"]}
        for step in unit["body_steps"]:
            if not set(step["oracle_ids"]).issubset(test_oracles):
                raise ValueError(f"step {step['step_id']} references unknown oracle")
        if unit["profile_ref"] == "PURE@1" and unit["side_effects"]:
            raise ValueError("PURE code unit contains side effects")
        aspects = {name: unit[name]["applicability"] for name in ("atomicity", "idempotency", "concurrency", "timeout_cancel", "security", "compatibility")}
        if unit["profile_ref"] == "AUTHORITY_TX@1":
            if any(aspects[name] != "APPLICABLE" for name in ("atomicity", "idempotency", "concurrency", "timeout_cancel", "security", "compatibility")):
                raise ValueError("AUTHORITY_TX requires all transaction-critical aspects")
            if not unit["side_effects"]:
                raise ValueError("AUTHORITY_TX requires classified side effects")
        if unit["profile_ref"] in {"WORKER@1", "CONSUMER@1", "PRODUCER@1"}:
            if any(aspects[name] != "APPLICABLE" for name in ("concurrency", "timeout_cancel", "security", "compatibility")):
                raise ValueError("worker/consumer/producer requires concurrency, cancellation, security, compatibility")
        if unit["profile_ref"] == "FLINK_OPERATOR@1":
            if any(aspects[name] != "APPLICABLE" for name in ("idempotency", "concurrency", "timeout_cancel", "security", "compatibility")):
                raise ValueError("FLINK_OPERATOR requires checkpoint and lifecycle aspects")
        if unit["profile_ref"] in {"UI_DECODER@1", "UI_HOOK@1", "UI_COMPONENT@1"}:
            if aspects["security"] != "APPLICABLE" or aspects["compatibility"] != "APPLICABLE":
                raise ValueError("UI code unit requires security and compatibility aspects")
    for companion in payload["companions"]:
        if not primary or companion["unit_id"] not in unit_ids or companion["unit_id"] == primary[0]["unit_id"]:
            raise ValueError("companion must reference an existing non-primary code unit")
        companion_unit = next(item for item in units if item["unit_id"] == companion["unit_id"])
        if companion_unit["locator"]["path"] != primary[0]["locator"]["path"]:
            raise ValueError("companion is not in the primary file")
    if primary:
        non_primary = {item["unit_id"] for item in units if not item["primary"]}
        companion_ids = {item["unit_id"] for item in payload["companions"]}
        if non_primary != companion_ids:
            raise ValueError("non-primary code units do not equal the companion exact set")
    flow_nodes = set(payload["call_flow"]["nodes"])
    unit_locator_ids = {item["locator"]["locator_id"] for item in units}
    if flow_nodes != unit_locator_ids:
        raise ValueError("call-flow nodes are not the exact code-unit locator set")
    allowed_destinations = known_locators | {
        effect["effect_id"] for unit in units for effect in unit["side_effects"]
    }
    for edge in payload["call_flow"]["edges"]:
        if edge["from_locator_id"] not in unit_locator_ids:
            raise ValueError("flow edge source is not a code-unit locator")
        if edge["to_locator_or_effect_id"] not in allowed_destinations:
            raise ValueError("flow edge destination is unresolved")
    migration = payload["sql_migration_design"]
    if migration is not None:
        if payload["artifact_status"] == "DESIGN_CANDIDATE":
            if migration["file_sha256"] is None or migration["resolver_receipt_ref"] is None:
                raise ValueError("READY SQL migration lacks file hash or parser receipt")
        ordinals = [item["ordinal"] for item in migration["statements"]]
        require_unique([str(item) for item in ordinals], "SQL statement ordinal")
        if ordinals != list(range(1, len(ordinals) + 1)):
            raise ValueError("SQL statement ordinals are not contiguous from one")
        for statement in migration["statements"]:
            if statement["kind"] == "BACKFILL" and statement["backfill"] is None:
                raise ValueError("BACKFILL statement lacks resume design")
            if statement["kind"] != "BACKFILL" and statement["backfill"] is not None:
                raise ValueError("non-BACKFILL statement carries backfill design")
    if payload["artifact_status"] == "DESIGN_CANDIDATE":
        if payload["readiness"]["blockers"]:
            raise ValueError("DESIGN_CANDIDATE code-unit contract cannot carry blockers")
        _, decision = resolve_typed_ref(payload["pattern_decision_ref"], "FINAL_PATTERN_DECISION")
        validate_pattern_decision(decision)
        _, proposal = resolve_typed_ref(decision["proposal_ref"], "PATTERN_DECISION_PROPOSAL")
        selected_option = next(
            item for item in proposal["options"] if item["option_id"] == decision["selected_option_id"]
        )
        allowed_participants = {
            (design["pattern_id"], binding["participant_role"], binding["locator_id"], binding["exact_symbol"])
            for design in selected_option["selected_designs"]
            for binding in design["participant_bindings"]
        }
        for unit in units:
            for role in unit["pattern_roles"]:
                participant = (
                    role["pattern_id"], role["participant_role"],
                    unit["locator"]["locator_id"], unit["locator"]["qualified_symbol"],
                )
                if participant not in allowed_participants:
                    raise ValueError("code-unit participant role differs from the selected proposal option")
        for name in ("test", "evidence", "rollback"):
            resolve_hashed_ref(payload["plan_refs"][name])
        if payload["plan_refs"]["observation"] is not None:
            resolve_hashed_ref(payload["plan_refs"]["observation"])
        for ref in payload["contract_impact_refs"]:
            resolve_hashed_ref(ref)
        for unit in units:
            resolve_hashed_ref(unit["locator"]["resolver_receipt_ref"])
        for companion in payload["companions"]:
            resolve_hashed_ref(companion["ast_enumeration_receipt_ref"])
        ref_fields = [payload["pattern_decision_ref"]]
        ref_fields.extend(value for value in payload["plan_refs"].values() if value is not None)
        ref_fields.extend(payload["contract_impact_refs"])
        ref_fields.extend(
            unit["locator"]["resolver_receipt_ref"]
            for unit in units
            if unit["locator"]["resolver_receipt_ref"] is not None
        )
        for ref in ref_fields:
            if ref is None:
                raise ValueError("DESIGN_CANDIDATE code-unit contract contains a null required reference")
            resolve_hashed_ref(ref)


def validate_function_review(payload: dict[str, Any]) -> None:
    validate_against_schema(payload, FUNCTION_REVIEW_SCHEMA_PATH)
    reject_placeholders(payload)
    _, decision = resolve_typed_ref(payload["pattern_decision_ref"], "FINAL_PATTERN_DECISION")
    _, code_unit = resolve_typed_ref(payload["code_unit_contract_ref"], "CODE_UNIT_CONTRACT")
    validate_pattern_decision(decision)
    validate_code_unit_contract(code_unit)
    if code_unit["artifact_status"] != "DESIGN_CANDIDATE" or code_unit["review_status"] != "PENDING_FUNCTION_REVIEW":
        raise ValueError("function review requires a pending DESIGN_CANDIDATE code-unit contract")
    if decision["atomic_pr_id"] != payload["atomic_pr_id"] or code_unit["atomic_pr_id"] != payload["atomic_pr_id"]:
        raise ValueError("function review upstream atomic PR mismatch")
    candidate_hashes = {
        payload["candidate"]["manifest_sha256"], decision["candidate"]["manifest_sha256"],
        code_unit["candidate"]["manifest_sha256"]
    }
    if len(candidate_hashes) != 1:
        raise ValueError("function review upstream candidate mismatch")
    code_decision_ref = code_unit["pattern_decision_ref"]
    receipt_decision_ref = payload["pattern_decision_ref"]
    if (
        code_decision_ref["decision_id"] != receipt_decision_ref["decision_id"]
        or code_decision_ref["sha256"] != receipt_decision_ref["sha256"]
        or code_decision_ref["candidate_manifest_sha256"] != receipt_decision_ref["candidate_manifest_sha256"]
    ):
        raise ValueError("function review and code-unit contract bind different final ADR artifacts")
    exact_set = sorted(
        {
            (
                unit["locator"]["path"], unit["locator"]["qualified_symbol"],
                unit["locator"]["signature_after"], unit["locator"]["ast_node_sha256"]
            )
            for unit in code_unit["code_units"]
        }
    )
    if canonical_sha256(exact_set) != payload["function_exact_set_sha256"]:
        raise ValueError("function exact-set digest mismatch")
    oracle_set = sorted(
        {oracle for unit in code_unit["code_units"] for test in unit["tests"] for oracle in test["oracle_ids"]}
    )
    if canonical_sha256(oracle_set) != payload["test_oracle_exact_set_sha256"]:
        raise ValueError("test oracle exact-set digest mismatch")
    negative_path, _ = resolve_typed_ref(payload["negative_test_manifest_ref"], "NEGATIVE_TEST_MANIFEST")
    if payload["negative_test_manifest_ref"]["candidate_manifest_sha256"] != payload["candidate"]["manifest_sha256"]:
        raise ValueError("function review negative-test manifest candidate mismatch")
    if payload["review_disposition"] == "UNIFIED" and (payload["unresolved_p0"] or payload["vetoes"]):
        raise ValueError("UNIFIED function review contains P0 or veto")
    signed_payload = {
        "candidate_manifest_sha256": payload["candidate"]["manifest_sha256"],
        "pattern_decision_sha256": payload["pattern_decision_ref"]["sha256"],
        "code_unit_contract_sha256": payload["code_unit_contract_ref"]["sha256"],
        "function_exact_set_sha256": payload["function_exact_set_sha256"],
        "test_oracle_exact_set_sha256": payload["test_oracle_exact_set_sha256"],
        "negative_test_manifest_sha256": hashlib.sha256(negative_path.read_bytes()).hexdigest(),
        "review_disposition": payload["review_disposition"],
    }
    if canonical_sha256(signed_payload) != payload["signed_payload_sha256"]:
        raise ValueError("function review signed payload digest mismatch")
    identities = [item["reviewer_identity"] for item in payload["attestations"]]
    require_unique(identities, "function review identity")
    if {item["payload_sha256"] for item in payload["attestations"]} != {payload["signed_payload_sha256"]}:
        raise ValueError("function review attestations do not bind one payload")
    for item in payload["attestations"]:
        resolve_hashed_ref(item["signature_artifact_ref"])


def main() -> int:
    parser = argparse.ArgumentParser()
    parser.add_argument("--catalog", action="store_true", help="validate the canonical GoF catalog")
    parser.add_argument("--proposal", type=Path)
    parser.add_argument("--decision", type=Path)
    parser.add_argument("--debate", type=Path)
    parser.add_argument("--code-unit", type=Path)
    parser.add_argument("--function-review", type=Path)
    args = parser.parse_args()
    if not any((args.catalog, args.proposal, args.decision, args.debate, args.code_unit, args.function_review)):
        parser.error("select at least one artifact")
    loaded_proposal: dict[str, Any] | None = None
    loaded_decision: dict[str, Any] | None = None
    loaded_debate: dict[str, Any] | None = None
    loaded_code_unit: dict[str, Any] | None = None
    if args.catalog:
        validate_catalog(load_json(CATALOG_PATH))
        print("PASS catalog exact-set, category, identity, and placeholder checks")
    if args.proposal:
        loaded_proposal = load_json(args.proposal)
        validate_pattern_proposal(loaded_proposal)
        print(f"PASS pattern proposal {args.proposal}")
    if args.decision:
        loaded_decision = load_json(args.decision)
        validate_pattern_decision(loaded_decision)
        print(f"PASS pattern decision {args.decision}")
    if args.debate:
        loaded_debate = load_json(args.debate)
        validate_debate(loaded_debate)
        print(f"PASS debate receipt {args.debate}")
    if args.code_unit:
        loaded_code_unit = load_json(args.code_unit)
        validate_code_unit_contract(loaded_code_unit)
        print(f"PASS code-unit contract {args.code_unit}")
    if args.function_review:
        validate_function_review(load_json(args.function_review))
        print(f"PASS function-design review {args.function_review}")
    artifacts = [item for item in (loaded_proposal, loaded_decision, loaded_debate, loaded_code_unit) if item is not None]
    if len(artifacts) > 1:
        atomic_ids = {item["atomic_pr_id"] for item in artifacts}
        candidate_hashes = {item["candidate"]["manifest_sha256"] for item in artifacts}
        if len(atomic_ids) != 1 or len(candidate_hashes) != 1:
            raise ValueError("design artifacts do not bind the same atomic PR and candidate")
        if loaded_decision is not None and loaded_debate is not None:
            if loaded_debate["decision_id"] != loaded_decision["decision_id"]:
                raise ValueError("debate references a different pattern decision")
            if loaded_debate["selected_option_id"] != loaded_decision["selected_option_id"]:
                raise ValueError("debate and final ADR selected option mismatch")
        if loaded_decision is not None and loaded_code_unit is not None:
            ref = loaded_code_unit["pattern_decision_ref"]
            if ref is None or ref["decision_id"] != loaded_decision["decision_id"]:
                raise ValueError("code-unit contract references a different pattern decision")
            actual_decision_hash = hashlib.sha256(args.decision.read_bytes()).hexdigest()
            if ref["sha256"] != actual_decision_hash:
                raise ValueError("code-unit pattern decision SHA mismatch")
            if ref["candidate_manifest_sha256"] != loaded_decision["candidate"]["manifest_sha256"]:
                raise ValueError("code-unit pattern decision candidate mismatch")
            if loaded_proposal is None:
                _, loaded_proposal = resolve_typed_ref(loaded_decision["proposal_ref"], "PATTERN_DECISION_PROPOSAL")
            option = next(item for item in loaded_proposal["options"] if item["option_id"] == loaded_decision["selected_option_id"])
            selected = {item["pattern_id"] for item in option["selected_designs"]} | set(option["distributed_constraint_ids"])
            role_patterns = {role["pattern_id"] for unit in loaded_code_unit["code_units"] for role in unit["pattern_roles"]}
            if not role_patterns.issubset(selected):
                raise ValueError("function participant role references an unselected pattern")
            if option["implementation_form"] in {"DIRECT", "NOT_APPLICABLE"} and role_patterns:
                raise ValueError("direct or non-applicable decision cannot assign pattern roles")
        print("PASS cross-artifact atomic PR, candidate, decision hash, and participant checks")
    print("PROOF_CEILING DESIGN_CONTRACT_ONLY_NOT_EXECUTION_AUTHORIZATION")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
