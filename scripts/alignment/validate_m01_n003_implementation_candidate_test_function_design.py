#!/usr/bin/env python3
"""Validate the blocked P005/P006 function and evidence design.

This validator proves only that the static design is exact, source-bound and
fail-closed.  It never creates a fixture, candidate, signature, execution
receipt or test result.
"""

from __future__ import annotations

import argparse
import copy
import hashlib
import json
from pathlib import Path
from typing import Any, Callable

from build_topic1_task_registry import validate_against_schema


REPO = Path(__file__).resolve().parents[2]
DESIGN = REPO / "contracts/alignment/m01-n003-implementation-candidate-test-function-design.v1.json"
SCHEMA = REPO / "contracts/alignment/m01-n003-implementation-candidate-test-function-design.schema.json"
TASK_REGISTRY = REPO / "contracts/alignment/task-registry.v1.json"
CLAIM_CATALOG = REPO / "contracts/alignment/developer-claim-package-catalog.v1.json"
PR_DESIGN_CATALOG = REPO / "contracts/alignment/pr-design-application-catalog.v1.json"
BUILDER = REPO / "scripts/alignment/build_topic1_task_registry.py"
EARLY_TRUST_ALLOCATION = REPO / "contracts/alignment/m01-early-trust-train-allocation.v1.json"
EARLY_TRUST_CATALOG = REPO / "contracts/alignment/m01-early-trust-train-catalog.v1.json"

P005 = "T1-M01-P005-REF-n003-s1"
P006 = "T1-M01-P006-TST-PRE-n003-s2"
P048 = "T1-M01-P048-IDX-n010-task-completion"
P066 = "T1-M01-P066-IDX-n015-task-completion"

EXPECTED_LEAVES = {P005, P006}
EXPECTED_CASES = {
    "active-prebuilt-without-provenance",
    "duplicate-prebuilt-path",
    "binary-image-sha-mismatch",
    "builder-recipe-mismatch",
    "image-deployed-digest-mismatch",
    "supply-chain-artifact-missing",
    "tracked-or-root-exclusion",
    "self-reported-signature-pass",
}
EXPECTED_UNREACHABLE_BEFORE_TRUST = {
    "active-prebuilt-without-provenance",
    "duplicate-prebuilt-path",
    "binary-image-sha-mismatch",
    "builder-recipe-mismatch",
    "image-deployed-digest-mismatch",
}
EXPECTED_REACHABLE_BEFORE_TRUST = EXPECTED_CASES - EXPECTED_UNREACHABLE_BEFORE_TRUST
FORMAL_CLI_FLAGS = {
    "--mode",
    "--subject-pr-id",
    "--subject-work-id",
    "--milestone-id",
    "--run-id",
    "--gate-id",
    "--candidate-manifest",
    "--design-candidate-manifest",
    "--profile-id",
    "--environment-id",
    "--execution-package-sha256",
    "--plan",
    "--time-window",
    "--fixture-root",
    "--result",
    "--case-report",
}
FUNCTION_CLI_FLAGS = FORMAL_CLI_FLAGS | {"--self-test"}
REQUIRED_CASE_FIELDS = {
    "case_id",
    "expected_outcome",
    "actual_outcome",
    "status",
    "rejection_code",
    "actual_rejection",
    "input_sha256s",
    "output_sha256s",
}


def sha256(path: Path) -> str:
    return hashlib.sha256(path.read_bytes()).hexdigest()


def load(path: Path) -> dict[str, Any]:
    value = json.loads(path.read_text(encoding="utf-8"))
    if not isinstance(value, dict):
        raise ValueError(f"JSON root must be an object: {path}")
    return value


def all_prs(registry: dict[str, Any]) -> dict[str, dict[str, Any]]:
    result: dict[str, dict[str, Any]] = {}
    for task in registry["tasks"]:
        for pr in task["pr_sequence"]:
            if pr["pr_id"] in result:
                raise ValueError(f"active task registry repeats an atomic PR: {pr['pr_id']}")
            result[pr["pr_id"]] = pr
    return result


def depends_on(start: str, target: str, prs: dict[str, dict[str, Any]]) -> bool:
    pending = list(prs[start]["depends_on_prs"])
    seen: set[str] = set()
    while pending:
        current = pending.pop()
        if current == target:
            return True
        if current in seen:
            continue
        seen.add(current)
        node = prs.get(current)
        if node is not None:
            pending.extend(node["depends_on_prs"])
    return False


def validate(payload: dict[str, Any]) -> None:
    validate_against_schema(payload, SCHEMA)

    source_refs = payload["source_refs"]
    source_roles = [item["role"] for item in source_refs]
    source_paths = [item["path"] for item in source_refs]
    if len(source_roles) != len(set(source_roles)) or len(source_paths) != len(set(source_paths)):
        raise ValueError("source role/path exact-set is not unique")
    for ref in source_refs:
        path = (REPO / ref["path"]).resolve()
        if not path.is_relative_to(REPO) or not path.is_file() or path.is_symlink():
            raise ValueError(f"source ref is absent, unsafe or a symlink: {ref['path']}")
        if sha256(path) != ref["sha256"]:
            raise ValueError(f"source hash drifted: {ref['path']}")

    task_registry = load(TASK_REGISTRY)
    claim_catalog = load(CLAIM_CATALOG)
    pr_design_catalog = load(PR_DESIGN_CATALOG)
    prs = all_prs(task_registry)
    claims = {item["atomic_pr_id"]: item for item in claim_catalog["packages"]}
    applications = {item["atomic_pr_id"]: item for item in pr_design_catalog["entries"]}
    for pr_id in EXPECTED_LEAVES:
        if pr_id not in prs or pr_id not in claims or pr_id not in applications:
            raise ValueError(f"active P005/P006 identity is missing: {pr_id}")

    leaf_contracts = payload["leaf_contracts"]
    by_leaf = {item["atomic_pr_id"]: item for item in leaf_contracts}
    if len(by_leaf) != len(leaf_contracts) or set(by_leaf) != EXPECTED_LEAVES:
        raise ValueError("P005/P006 leaf contract exact-set drifted")
    if by_leaf[P005]["pr_type"] != "REF" or by_leaf[P006]["pr_type"] != "TST-PRE":
        raise ValueError("P005/P006 PR type drifted")
    for pr_id in EXPECTED_LEAVES:
        if claims[pr_id]["formal_execution_status"] != "BLOCKED_UNTIL_SIGNED_OVERLAY":
            raise ValueError(f"active leaf unexpectedly claims execution readiness: {pr_id}")
        if by_leaf[pr_id]["current_execution_status"] != claims[pr_id]["formal_execution_status"]:
            raise ValueError(f"design execution status differs from active claim: {pr_id}")

    function = payload["function_contract"]
    if function["atomic_pr_id"] != P005 or function["qualified_symbol"] != "main":
        raise ValueError("P005 function contract does not bind the exact main symbol")
    active_main_targets = [
        item for item in claims[P005]["change_targets"]
        if item["path"] == function["path"] and item["symbol_or_pointer"] == "main"
    ]
    if len(active_main_targets) != 1:
        raise ValueError("active P005 claim does not own one exact main locator")
    flags = [item["flag"] for item in function["cli_arguments"]]
    if len(flags) != len(set(flags)) or set(flags) != FUNCTION_CLI_FLAGS:
        raise ValueError("corrected main CLI flag exact-set drifted")
    steps = function["body_steps"]
    step_ids = [item.split(" ", 1)[0] for item in steps]
    if step_ids != [f"B{index:02d}" for index in range(1, len(steps) + 1)]:
        raise ValueError("main body steps are not contiguous")

    cases = payload["case_recipes"]
    by_case = {item["case_id"]: item for item in cases}
    if len(by_case) != len(cases) or set(by_case) != EXPECTED_CASES:
        raise ValueError("P006 exact eight-case set drifted")
    test_ids = [item["test_id"] for item in cases]
    if set(test_ids) != {f"TC-M01-N003-{index:02d}" for index in range(1, 9)}:
        raise ValueError("P006 test ID exact-set drifted")
    for case_id, case in by_case.items():
        expected_code = "REJECT_" + case_id.replace("-", "_").upper()
        if case["expected_rejection_code"] != expected_code:
            raise ValueError(f"case rejection code drifted: {case_id}")
        if case["execution_status"] != "NOT_RUN":
            raise ValueError(f"static design claims a case was executed: {case_id}")
        if not case["required_fixture_parameters"] or not case["assertions"]:
            raise ValueError(f"case lacks a parameterized semantic oracle: {case_id}")
        fragments = [item["expected_error_fragment"] for item in case["assertions"]]
        if len(fragments) != len(set(fragments)):
            raise ValueError(f"case repeats an assertion error fragment: {case_id}")

    analysis = payload["dependency_analysis"]
    if set(analysis["unreachable_before_trust_case_ids"]) != EXPECTED_UNREACHABLE_BEFORE_TRUST:
        raise ValueError("pre-trust unreachable case exact-set drifted")
    if set(analysis["reachable_before_trust_case_ids"]) != EXPECTED_REACHABLE_BEFORE_TRUST:
        raise ValueError("pre-trust reachable case exact-set drifted")
    path = analysis["current_path_to_trust_owner"]
    if path[0] != P006 or path[-1] != P048:
        raise ValueError("declared P006-to-P048 dependency path endpoints drifted")
    for left, right in zip(path, path[1:]):
        if left not in prs or right not in prs or not depends_on(right, left, prs):
            raise ValueError(f"declared trust dependency path is not active: {left} -> {right}")
    if not depends_on(P048, P006, prs):
        raise ValueError("active registry no longer proves the P006-to-P048 ancestor path")
    if not analysis["direct_dependency_cycle_if_added"]:
        raise ValueError("design denies the reproduced P006/P048 back-edge cycle")
    preview = analysis["preview_resolution"]
    if preview != {
        "status": "VALIDATED_PREVIEW_NOT_REGISTERED",
        "terminal_atomic_pr_id": P066,
        "p005_dependency_ids": ["T1-M01-P002-IDX-n001-task-completion", P066],
        "p006_dependency_ids": [P005, P066],
        "candidate_dag": "PASS",
        "global_switch_decision": "BLOCKED_PREVIEW_ONLY",
    }:
        raise ValueError("early-trust preview dependency binding exact-set drifted")
    allocation = load(EARLY_TRUST_ALLOCATION)
    catalog = load(EARLY_TRUST_CATALOG)
    revisions = {
        item["atomic_pr_id"]: item["revised_dependency_ids"]
        for item in allocation["dependency_revisions"]
    }
    if revisions.get(P005) != preview["p005_dependency_ids"] or revisions.get(P006) != preview["p006_dependency_ids"]:
        raise ValueError("early-trust preview does not implement the declared P005/P006 dependency contract")
    if catalog["global_switch_gate"]["decision"] != "BLOCKED_PREVIEW_ONLY" or not catalog["validation"]["p006_back_edge_absent"]:
        raise ValueError("early-trust preview weakens the no-switch or no-back-edge gate")
    if any(
        edge["from"] == P006 and edge["to"] == P048
        or edge["from"] == P048 and edge["to"] == P006
        for edge in catalog["candidate_edges"]
    ):
        raise ValueError("early-trust preview contains a P006/P048 back-edge")

    evidence = payload["evidence_contract"]
    active_command = next(
        item["command"] for item in claims[P006]["verification_checks"]
        if item["check_id"].endswith("-declared-case-matrix")
    )
    if evidence["current_command"] != active_command:
        raise ValueError("captured current P006 command differs from the active claim")
    missing_current_flags = FORMAL_CLI_FLAGS - {
        token for token in active_command.split() if token.startswith("--")
    }
    if not {
        "--run-id", "--candidate-manifest", "--design-candidate-manifest",
        "--profile-id", "--environment-id", "--execution-package-sha256",
        "--plan", "--time-window",
    }.issubset(missing_current_flags):
        raise ValueError("current command no longer reproduces the candidate-binding gap")
    corrected_flags = {
        token for token in evidence["corrected_command_argv"] if token.startswith("--")
    }
    if corrected_flags != FORMAL_CLI_FLAGS:
        raise ValueError("corrected command argv lacks an exact required CLI flag set")
    if evidence["gate_scope"] != "G0_ONLY_NEGATIVE_CONTRACT_MATRIX":
        raise ValueError("P006 static negative matrix falsely claims a second gate")
    if set(evidence["case_result_required_fields"]) != REQUIRED_CASE_FIELDS:
        raise ValueError("case result does not require actual rejection and exact identity fields")
    artifact_paths = {item["path"] for item in evidence["artifacts"]}
    active_output_paths = {item["path"] for item in claims[P006]["generated_outputs"]}
    if artifact_paths != active_output_paths:
        raise ValueError("P006 double-evidence artifact exact-set drifted")

    amendments = payload["required_registry_amendments"]
    amendment_ids = [item["amendment_id"] for item in amendments]
    status_by_amendment = {item["amendment_id"]: item["status"] for item in amendments}
    if len(amendment_ids) != len(set(amendment_ids)) or status_by_amendment.get("M01-N003-AMD-01") != "PREVIEW_ALLOCATED_NOT_REGISTERED" or any(
        status != "REQUIRED_NOT_APPLIED"
        for amendment_id, status in status_by_amendment.items()
        if amendment_id != "M01-N003-AMD-01"
    ):
        raise ValueError("required registry amendment status/identity drifted")

    sequence = payload["sequencing"]
    if [item["order"] for item in sequence] != list(range(1, len(sequence) + 1)):
        raise ValueError("P005/P006 execution sequence is not contiguous")
    if sequence[0]["status"] != "DESIGNED" or sequence[1]["status"] != "DESIGNED_PREVIEW" or any(
        item["status"] != "BLOCKED" for item in sequence[2:]
    ):
        raise ValueError("static design sequence makes a downstream action prematurely ready")

    forbidden = set(payload["claims"]["forbidden"])
    if not {
        "FUNCTION_DESIGN_REVIEWED", "REGISTRY_AMENDED", "P005_IMPLEMENTED",
        "P006_EXECUTED", "CASE_PASS", "CANDIDATE_PASS", "TRUST_READY",
        "EXECUTION_AUTHORIZED", "PARENT_COMPLETE", "MILESTONE_COMPLETE",
        "PRODUCTION_ACCEPTED",
    }.issubset(forbidden):
        raise ValueError("static design proof ceiling is incomplete")

    builder_text = BUILDER.read_text(encoding="utf-8")
    required_fragments = {
        "require_trusted_signature_verifier(f\"{context} external artifact provenance\")",
        "raise ValueError(f\"{context} active excluded artifact/prebuilt set is not exact\")",
        "raise ValueError(f\"{context} repeats a prebuilt path/provenance identity\")",
        "raise ValueError(f\"{context} prebuilt binary/image/SBOM provenance mismatch\")",
        "raise ValueError(f\"{context} deployed image is not the attested immutable digest\")",
        "raise ValueError(f\"{context} prebuilt build recipe is not a candidate Git blob\")",
    }
    missing_fragments = [item for item in required_fragments if item not in builder_text]
    if missing_fragments:
        raise ValueError(f"active validator branch evidence drifted: {missing_fragments}")


def expect_reject(
    name: str,
    expected: str,
    mutate: Callable[[dict[str, Any]], None],
    source: dict[str, Any],
) -> None:
    candidate = copy.deepcopy(source)
    mutate(candidate)
    try:
        validate(candidate)
    except (ValueError, KeyError) as exc:
        if expected not in str(exc):
            raise ValueError(
                f"mutation {name} hit the wrong guard; expected {expected!r}, got {str(exc)!r}"
            ) from exc
        return
    raise ValueError(f"malicious M01-N003 design mutation was accepted: {name}")


def self_test(payload: dict[str, Any]) -> None:
    expect_reject(
        "source hash drift",
        "source hash drifted",
        lambda value: value["source_refs"][0].update(sha256="0" * 64),
        payload,
    )
    expect_reject(
        "case omission",
        "exact eight-case set drifted",
        lambda value: value["case_recipes"][-1].update(
            case_id=value["case_recipes"][0]["case_id"]
        ),
        payload,
    )
    expect_reject(
        "false case execution",
        "schema const mismatch at $.case_recipes[0].execution_status",
        lambda value: value["case_recipes"][0].update(execution_status="PASS"),
        payload,
    )
    expect_reject(
        "missing CLI binding",
        "corrected main CLI flag exact-set drifted",
        lambda value: value["function_contract"]["cli_arguments"].pop(),
        payload,
    )
    expect_reject(
        "noncontiguous body step",
        "main body steps are not contiguous",
        lambda value: value["function_contract"]["body_steps"].__setitem__(1, "B03 skip B02"),
        payload,
    )
    expect_reject(
        "false dual gate",
        "schema const mismatch at $.evidence_contract.gate_scope",
        lambda value: value["evidence_contract"].update(gate_scope="G0_AND_G1"),
        payload,
    )
    expect_reject(
        "missing actual rejection",
        "case result does not require actual rejection",
        lambda value: value["evidence_contract"]["case_result_required_fields"].__setitem__(
            value["evidence_contract"]["case_result_required_fields"].index("actual_rejection"),
            "actual_rejection_alias",
        ),
        payload,
    )
    expect_reject(
        "dependency cycle denial",
        "schema const mismatch at $.dependency_analysis.direct_dependency_cycle_if_added",
        lambda value: value["dependency_analysis"].update(direct_dependency_cycle_if_added=False),
        payload,
    )
    expect_reject(
        "preview back edge",
        "schema enum mismatch at $.dependency_analysis.preview_resolution.p006_dependency_ids[1]",
        lambda value: value["dependency_analysis"]["preview_resolution"]["p006_dependency_ids"].__setitem__(
            1, P048
        ),
        payload,
    )
    expect_reject(
        "premature ready",
        "static design sequence makes a downstream action prematurely ready",
        lambda value: value["sequencing"][2].update(status="DESIGNED"),
        payload,
    )


def main() -> int:
    parser = argparse.ArgumentParser()
    parser.add_argument("--self-test", action="store_true")
    args = parser.parse_args()
    payload = load(DESIGN)
    validate(payload)
    if args.self_test:
        self_test(payload)
    print(
        "PASS M01-N003 P005/P006 static design: 1 function, 8 NOT_RUN cases, "
        "2 evidence artifacts, 7 reproduced DoR findings; execution remains BLOCKED"
    )
    print(
        "PROOF_CEILING STATIC_DESIGN_ONLY_NOT_REGISTRY_AMENDMENT_FUNCTION_REVIEW_"
        "IMPLEMENTATION_TEST_EXECUTION_CANDIDATE_PASS_AUTHORIZATION_OR_ACCEPTANCE"
    )
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
