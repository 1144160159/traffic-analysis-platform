#!/usr/bin/env python3
"""Generate blocked, developer-claimable M02 external-input work orders.

The catalog derives exact commands and output paths.  It deliberately never
creates a candidate, source implementation, receipt, person, signature, review
decision, execution authorization, or registry mutation.
"""

from __future__ import annotations

import argparse
from collections import Counter
import copy
import hashlib
import json
from pathlib import Path
from typing import Any, Callable

from build_topic1_task_registry import validate_against_schema
from generate_m02_gate_input_preflight import (
    DESIGN_MANIFEST_PATH,
    IMPLEMENTATION_MANIFEST_PATH,
    RESPONSIBILITY_INTAKE_PATH,
    RESPONSIBILITY_PATH,
    expected_compatibility_rows,
    expected_locator_rows,
)


REPO = Path(__file__).resolve().parents[2]
SCHEMA = REPO / "contracts/alignment/m02-external-input-work-order-catalog.schema.json"
OUTPUT = REPO / "contracts/alignment/m02-external-input-work-order-catalog.v1.json"
DOC_OUTPUT = REPO / "doc/07_alignment/generated/M02外部证据可领取工作单目录.md"

LOCATOR_COVERAGE = REPO / "contracts/alignment/m02-code-direct-locator-coverage.v1.json"
REVIEW_COVERAGE = REPO / "contracts/alignment/m02-function-review-coverage.v1.json"
TASK_REGISTRY = REPO / "contracts/alignment/task-registry.v1.json"
REPLACEMENT = REPO / "contracts/alignment/m02-code-direct-leaf-catalog.v4.json"
PREFLIGHT = REPO / "contracts/alignment/m02-gate-input-preflight.v1.json"
IMPLEMENTATION_CLOSURE = REPO / "contracts/alignment/m02-implementation-candidate-closure.v1.json"

TRUSTED_SIGNATURE_PATHS = [
    "contracts/alignment/signature-trust-policy.schema.json",
    "contracts/alignment/signature-verification-request.schema.json",
    "contracts/alignment/signature-verification-attestation.schema.json",
    "scripts/alignment/verify_trusted_signature.py",
    "scripts/alignment/test_trusted_signature_verifier.py",
    "deployments/security/topic1-trusted-signature-verifier.yaml",
]

LOCATOR_PROFILES: dict[str, dict[str, Any]] = {
    ".go": {
        "profile": "GO_AST_V1",
        "resolver": "scripts/alignment/go_ast_locator/main.go",
        "schema": "contracts/alignment/locator-resolution-receipt.schema.json",
        "working_directory": ".",
        "prefix": ["go", "run", "./scripts/alignment/go_ast_locator/main.go"],
        "query_flag": "--symbol",
    },
    ".py": {
        "profile": "PYTHON_AST_V1",
        "resolver": "scripts/alignment/python_ast_locator.py",
        "schema": "contracts/alignment/python-locator-resolution-receipt.schema.json",
        "working_directory": ".",
        "prefix": ["python3", "scripts/alignment/python_ast_locator.py"],
        "query_flag": "--symbol",
    },
    ".rs": {
        "profile": "RUST_SYN_V1",
        "resolver": "scripts/alignment/rust_ast_locator/src/main.rs",
        "schema": "contracts/alignment/rust-locator-resolution-receipt.schema.json",
        "working_directory": ".",
        "prefix": [
            "cargo", "run", "--quiet", "--locked", "--offline",
            "--manifest-path", "scripts/alignment/rust_ast_locator/Cargo.toml", "--",
        ],
        "query_flag": "--symbol",
    },
    ".java": {
        "profile": "JAVA_JAVAC_AST_V1",
        "resolver": "scripts/alignment/java_ast_locator/JavaAstLocator.java",
        "schema": "contracts/alignment/java-locator-resolution-receipt.schema.json",
        "working_directory": ".",
        "prefix": ["java", "-cp", "${M02_JAVA_LOCATOR_CLASSES}", "JavaAstLocator"],
        "query_flag": "--symbol",
    },
    ".proto": {
        "profile": "PROTO_DESCRIPTOR_V1",
        "resolver": "scripts/alignment/proto_descriptor_locator/main.go",
        "schema": "contracts/alignment/proto-descriptor-locator-resolution-receipt.schema.json",
        "working_directory": "scripts/alignment/proto_descriptor_locator",
        "prefix": ["go", "run", "-mod=readonly", "."],
        "query_flag": "--symbol",
    },
    ".sql": {
        "profile": "SQL_DDL_PARSE_V1",
        "resolver": "scripts/alignment/sql_ddl_locator.py",
        "schema": "contracts/alignment/sql-ddl-locator-resolution-receipt.schema.json",
        "working_directory": ".",
        "prefix": ["python3", "scripts/alignment/sql_ddl_locator.py"],
        "query_flag": "--query",
    },
    ".json": {
        "profile": "STRUCTURED_CONFIG_V1",
        "resolver": "scripts/alignment/structured_config_locator.py",
        "schema": "contracts/alignment/structured-config-locator-resolution-receipt.schema.json",
        "working_directory": ".",
        "prefix": ["python3", "scripts/alignment/structured_config_locator.py"],
        "query_flag": "--query",
    },
    ".yaml": {
        "profile": "STRUCTURED_CONFIG_V1",
        "resolver": "scripts/alignment/structured_config_locator.py",
        "schema": "contracts/alignment/structured-config-locator-resolution-receipt.schema.json",
        "working_directory": ".",
        "prefix": ["python3", "scripts/alignment/structured_config_locator.py"],
        "query_flag": "--query",
    },
    ".yml": {
        "profile": "STRUCTURED_CONFIG_V1",
        "resolver": "scripts/alignment/structured_config_locator.py",
        "schema": "contracts/alignment/structured-config-locator-resolution-receipt.schema.json",
        "working_directory": ".",
        "prefix": ["python3", "scripts/alignment/structured_config_locator.py"],
        "query_flag": "--query",
    },
    ".toml": {
        "profile": "STRUCTURED_CONFIG_V1",
        "resolver": "scripts/alignment/structured_config_locator.py",
        "schema": "contracts/alignment/structured-config-locator-resolution-receipt.schema.json",
        "working_directory": ".",
        "prefix": ["python3", "scripts/alignment/structured_config_locator.py"],
        "query_flag": "--query",
    },
    ".sh": {
        "profile": "SHELL_AST_V1",
        "resolver": "scripts/alignment/shell_ast_locator/main.go",
        "schema": "contracts/alignment/shell-ast-locator-resolution-receipt.schema.json",
        "working_directory": "scripts/alignment/shell_ast_locator",
        "prefix": ["go", "run", "-mod=readonly", "."],
        "query_flag": "--query",
    },
}

COMMON = {
    "allowed_claim": "work order identity, exact inputs, output path and commands are derived",
    "forbidden_claim": "candidate, identity, signature, review approval, receipt, implementation, execution, registry switch or acceptance exists",
    "proof_ceiling": "DEVELOPER_WORK_ORDER_ONLY_BLOCKED_UNTIL_EXTERNAL_PREREQUISITES",
}


def sha256(path: Path) -> str:
    return hashlib.sha256(path.read_bytes()).hexdigest()


def semantic_sha256(value: Any) -> str:
    return hashlib.sha256(
        json.dumps(value, ensure_ascii=False, sort_keys=True, separators=(",", ":")).encode()
    ).hexdigest()


def hash_ref(path: Path) -> dict[str, str]:
    return {"path": path.relative_to(REPO).as_posix(), "sha256": sha256(path)}


def stable_id(kind: str, subject: str) -> str:
    return f"M02-EI-{kind}-{semantic_sha256(subject)[:20].upper()}"


def load(path: Path) -> dict[str, Any]:
    payload = json.loads(path.read_text(encoding="utf-8"))
    if not isinstance(payload, dict):
        raise ValueError(f"source must be a JSON object: {path}")
    return payload


def step(step_id: str, run_when: str, cwd: str, argv: list[str], effect: str) -> dict[str, Any]:
    return {
        "step_id": step_id,
        "run_when": run_when,
        "working_directory": cwd,
        "argv": argv,
        "expected_effect": effect,
    }


def details(**values: Any) -> dict[str, Any]:
    defaults = {
        "locator": None,
        "locator_id": None,
        "path": None,
        "query": None,
        "resolver_profile": None,
        "source_file_exists": None,
        "target_locators": [],
        "leaf_id": None,
        "review_surface": None,
        "static_contract_paths": [],
        "task_id": None,
    }
    defaults.update(values)
    return defaults


def locator_path(locator: str) -> str:
    path = locator.split("#", 1)[0]
    if not path:
        raise ValueError(f"locator has no repository path: {locator}")
    return path


def locator_command(row: dict[str, Any], profile: dict[str, Any]) -> list[str]:
    command = [
        *profile["prefix"],
        "--source", row["path"],
        profile["query_flag"], row["query"],
    ]
    if Path(row["path"]).suffix.lower() == ".sql":
        dialect = "clickhouse" if row["path"].startswith("deployments/clickhouse/") else "postgres"
        command.extend(["--dialect", dialect])
    command.extend([
        "--locator-id", row["locator_id"],
        "--candidate-commit", "${M02_CANDIDATE_COMMIT}",
        "--candidate-manifest", DESIGN_MANIFEST_PATH,
        "--candidate-manifest-sha256", "${M02_DESIGN_MANIFEST_SHA256}",
        "--repo-root", "${M02_REPO_ROOT}",
        "--resolved-at", "${M02_RESOLVED_AT_RFC3339_UTC}",
        "--output", row["expected_path"],
    ])
    return command


def build() -> dict[str, Any]:
    locator_coverage = load(LOCATOR_COVERAGE)
    review_coverage = load(REVIEW_COVERAGE)
    registry = load(TASK_REGISTRY)
    replacement = load(REPLACEMENT)
    preflight = load(PREFLIGHT)
    implementation_closure = load(IMPLEMENTATION_CLOSURE)

    leaves = {item["atomic_pr_id"]: item for item in replacement["leaves"]}
    task_roles = {
        item["task_id"]: item["responsibility"]["owner_role"]
        for item in registry["tasks"]
        if item["milestone_id"] == "T1-M02"
    }
    if len(task_roles) != 16:
        raise ValueError("M02 task role exact-set drifted")

    source_state: dict[str, bool] = {}
    for item in locator_coverage["locator_occurrences"]:
        previous = source_state.setdefault(item["locator"], item["file_exists"])
        if previous != item["file_exists"]:
            raise ValueError(f"locator source existence disagrees: {item['locator']}")

    locator_rows = expected_locator_rows(locator_coverage)
    compatibility_rows = expected_compatibility_rows(locator_coverage)
    review_rows = sorted(review_coverage["leaf_reviews"], key=lambda item: item["atomic_pr_id"])
    planned_sources = sorted({row["path"] for row in locator_rows if not source_state[row["locator"]]})

    candidate_probe = preflight["candidate_intake"]
    responsibility_probe = preflight["responsibility_intake"]
    implementation_closure_ready = implementation_closure["artifact_status"].startswith("READY_")

    def probe_status(probe: dict[str, Any]) -> str:
        if probe["status"] == "STRUCTURALLY_VALID":
            return "READY"
        return "MISSING" if not probe["exists"] else "PARTIAL"

    prerequisites = [
        {
            "prerequisite_id": "M02-EXT-DESIGN-CANDIDATE",
            "authority_task_id": None,
            "description": "one exact 163-source design manifest frozen from a clean Git HEAD",
            "required_paths": [DESIGN_MANIFEST_PATH],
            "missing_paths": [] if candidate_probe["design_manifest"]["exists"] else [DESIGN_MANIFEST_PATH],
            "status": probe_status(candidate_probe["design_manifest"]),
            "blocks_work_order_kinds": ["LOCATOR_RESOLUTION", "COMPATIBILITY_DEFAULT_OFF_REVIEW", "FUNCTION_OR_EXEMPTION_REVIEW"],
            "next_actions": [
                "implement all missing planned product sources in an isolated reviewed Git worktree",
                "commit the exact M02 source set and freeze the immutable design manifest from that clean HEAD",
            ],
            "preparation_command_plan": [
                step(
                    "verify-design-candidate-freezer", "before freezing any candidate", ".",
                    ["python3", "scripts/alignment/test_freeze_m02_design_candidate.py"],
                    "prove clean/dirty, exact-set, symlink, path and immutable-output guards",
                ),
                step(
                    "freeze-design-candidate", "only in an isolated clean worktree after all 163 sources exist", ".",
                    [
                        "python3", "scripts/alignment/freeze_m02_design_candidate.py",
                        "--repo-root", "${M02_REPO_ROOT}", "--candidate-commit", "${M02_CANDIDATE_COMMIT}", "--write",
                    ],
                    "write one immutable design candidate manifest or fail closed",
                ),
            ],
        },
        {
            "prerequisite_id": "M02-EXT-IMPLEMENTATION-CANDIDATE",
            "authority_task_id": None,
            "description": "same-commit implementation identity with production-tree images SBOM attestations and five delivery artifacts",
            "required_paths": [IMPLEMENTATION_MANIFEST_PATH, IMPLEMENTATION_CLOSURE.relative_to(REPO).as_posix()],
            "missing_paths": [] if candidate_probe["implementation_manifest"]["exists"] else [IMPLEMENTATION_MANIFEST_PATH],
            "status": (
                "READY"
                if candidate_probe["implementation_manifest"]["status"] == "STRUCTURALLY_VALID" and implementation_closure_ready
                else "MISSING" if not candidate_probe["implementation_manifest"]["exists"]
                else "PARTIAL"
            ),
            "blocks_work_order_kinds": ["COMPATIBILITY_DEFAULT_OFF_REVIEW", "FUNCTION_OR_EXEMPTION_REVIEW"],
            "next_actions": [
                "build immutable images and collect image-internal binary hashes plus SBOM or provenance attestations",
                "bind configuration schema migration runtime supply-chain and five delivery artifacts to the same candidate commit",
                f"close the eleven-item implementation checklist currently READY={implementation_closure['readiness_counts']['READY']} PARTIAL={implementation_closure['readiness_counts']['PARTIAL']} MISSING={implementation_closure['readiness_counts']['MISSING']} INVALID={implementation_closure['readiness_counts']['INVALID']}",
                "submit the completed manifest to the existing implementation-candidate semantic validator and trusted verifier",
            ],
            "preparation_command_plan": [],
        },
        {
            "prerequisite_id": "M02-EXT-TRUSTED-SIGNATURE-VERIFIER",
            "authority_task_id": "T1-M01-N010",
            "description": "independent protected cryptographic verification capability required by all signed inputs",
            "required_paths": TRUSTED_SIGNATURE_PATHS,
            "missing_paths": [path for path in TRUSTED_SIGNATURE_PATHS if not (REPO / path).is_file()],
            "status": (
                "READY" if all((REPO / path).is_file() for path in TRUSTED_SIGNATURE_PATHS)
                else "PARTIAL" if any((REPO / path).is_file() for path in TRUSTED_SIGNATURE_PATHS)
                else "MISSING"
            ),
            "blocks_work_order_kinds": ["COMPATIBILITY_DEFAULT_OFF_REVIEW", "FUNCTION_OR_EXEMPTION_REVIEW", "RESPONSIBILITY_ASSIGNMENT"],
            "next_actions": [
                "complete T1-M01-N010 with an independently protected trust policy and real cryptographic verifier",
                "verify exact payload trust chain authority role validity and revocation state",
            ],
            "preparation_command_plan": [],
        },
        {
            "prerequisite_id": "M02-EXT-PLANNED-PRODUCT-SOURCES",
            "authority_task_id": None,
            "description": "planned product files must exist in the frozen candidate before after-state locators can resolve",
            "required_paths": planned_sources,
            "missing_paths": [path for path in planned_sources if not (REPO / path).is_file()],
            "status": "READY" if all((REPO / path).is_file() for path in planned_sources) else "MISSING",
            "blocks_work_order_kinds": ["LOCATOR_RESOLUTION"],
            "next_actions": [
                "claim the owning atomic implementation leaves and create every planned product source under reviewed write scopes",
                "commit all created sources before freezing the design candidate",
            ],
            "preparation_command_plan": [],
        },
        {
            "prerequisite_id": "M02-EXT-NAMED-RESPONSIBILITY",
            "authority_task_id": None,
            "description": "sixteen independent owner reviewer approver assignments and their signed intake",
            "required_paths": [RESPONSIBILITY_PATH, RESPONSIBILITY_INTAKE_PATH],
            "missing_paths": [
                path for path in [RESPONSIBILITY_PATH, RESPONSIBILITY_INTAKE_PATH]
                if not (REPO / path).is_file()
            ],
            "status": (
                "READY"
                if responsibility_probe["manifest"]["status"] == "STRUCTURALLY_VALID"
                and responsibility_probe["signed_intake"]["status"] == "STRUCTURALLY_VALID"
                else "MISSING"
                if not responsibility_probe["manifest"]["exists"] and not responsibility_probe["signed_intake"]["exists"]
                else "PARTIAL"
            ),
            "blocks_work_order_kinds": ["RESPONSIBILITY_ASSIGNMENT"],
            "next_actions": [
                "provide sixteen independent named owner reviewer and approver assignments without invented identities",
                "create the exact signed intake over the completed responsibility manifest",
            ],
            "preparation_command_plan": [
                step(
                    "validate-responsibility-manifest", "after all sixteen real assignments are provided", ".",
                    ["python3", "scripts/alignment/validate_m02_responsibility_assignment.py", RESPONSIBILITY_PATH],
                    "validate the sixteen-task exact-set and owner-role mapping before trusted signature verification",
                )
            ],
        },
    ]

    orders: list[dict[str, Any]] = []
    for row in locator_rows:
        suffix = Path(row["path"]).suffix.lower()
        profile = LOCATOR_PROFILES.get(suffix)
        if profile is None:
            raise ValueError(f"unsupported work-order locator suffix: {suffix}")
        consumers = row["consumer_atomic_pr_ids"]
        parents = sorted({leaves[item]["parent_task_id"] for item in consumers})
        compile_steps: list[dict[str, Any]] = []
        if suffix == ".java":
            compile_steps.append(step(
                "compile-java-locator", "after a trusted JDK is available", ".",
                [
                    "javac", "-encoding", "UTF-8", "-d", "${M02_JAVA_LOCATOR_CLASSES}",
                    "scripts/alignment/java_ast_locator/JavaAstLocator.java",
                ],
                "compile the hash-bound Java locator into an isolated caller-provided directory",
            ))
        exists = source_state[row["locator"]]
        blockers = ["clean 163-source design candidate manifest is absent"]
        prereq_ids = ["M02-EXT-DESIGN-CANDIDATE"]
        status = "BLOCKED_DESIGN_CANDIDATE_MISSING"
        if not exists:
            blockers.append("planned product source file is absent and must be created by its atomic implementation leaf")
            prereq_ids.append("M02-EXT-PLANNED-PRODUCT-SOURCES")
            status = "BLOCKED_PLANNED_PRODUCT_SOURCE_MISSING"
        input_paths = sorted({
            row["path"], DESIGN_MANIFEST_PATH,
            profile["resolver"], profile["schema"],
        })
        orders.append({
            "work_order_id": stable_id("LOC", row["locator"]),
            "work_order_kind": "LOCATOR_RESOLUTION",
            "subject_id": row["locator"],
            "consumer_atomic_pr_ids": consumers,
            "parent_task_ids": parents,
            "owner_role": "candidate-locator-operator",
            "details": details(
                locator=row["locator"], locator_id=row["locator_id"], path=row["path"],
                query=row["query"], resolver_profile=profile["profile"],
                source_file_exists=exists, target_locators=[row["locator"]],
            ),
            "input_paths": input_paths,
            "output_paths": [row["expected_path"]],
            "command_plan": [
                *compile_steps,
                step(
                    "resolve-exact-locator",
                    "only after all prerequisite IDs are READY and caller variables are exact",
                    profile["working_directory"], locator_command(row, profile),
                    "write one immutable candidate-bound exact locator receipt or fail closed",
                ),
            ],
            "aggregate_validation_argv": ["python3", "scripts/alignment/generate_m02_gate_input_preflight.py", "--verify"],
            "prerequisite_ids": prereq_ids,
            "status": status,
            "blocking_reasons": blockers,
            **COMMON,
        })

    for row in compatibility_rows:
        leaf = leaves[row["atomic_pr_id"]]
        output = row["expected_path"]
        orders.append({
            "work_order_id": stable_id("CMP", row["atomic_pr_id"]),
            "work_order_kind": "COMPATIBILITY_DEFAULT_OFF_REVIEW",
            "subject_id": row["atomic_pr_id"],
            "consumer_atomic_pr_ids": [row["atomic_pr_id"]],
            "parent_task_ids": [leaf["parent_task_id"]],
            "owner_role": "compatibility-review-coordinator",
            "details": details(target_locators=row["target_locators"], leaf_id=leaf["leaf_id"], review_surface="COMPATIBILITY_DEFAULT_OFF"),
            "input_paths": sorted({
                DESIGN_MANIFEST_PATH, IMPLEMENTATION_MANIFEST_PATH,
                *(locator_path(item) for item in row["target_locators"]),
            }),
            "output_paths": [output],
            "command_plan": [step(
                "validate-compatibility-review",
                "after independent named reviewers create and sign the immutable receipt",
                ".", ["python3", "scripts/alignment/validate_m02_compatibility_default_off_review.py", output],
                "validate structure and payload binding before independent trusted signature verification",
            )],
            "aggregate_validation_argv": ["python3", "scripts/alignment/generate_m02_gate_input_preflight.py", "--verify"],
            "prerequisite_ids": ["M02-EXT-DESIGN-CANDIDATE", "M02-EXT-IMPLEMENTATION-CANDIDATE", "M02-EXT-TRUSTED-SIGNATURE-VERIFIER"],
            "status": "BLOCKED_DESIGN_IMPLEMENTATION_REVIEW_AND_TRUSTED_SIGNATURE_MISSING",
            "blocking_reasons": [
                "clean same-commit candidate is absent",
                "independent compatibility review identities and decisions are absent",
                "T1-M01-N010 trusted cryptographic verifier is absent",
            ],
            **COMMON,
        })

    for row in review_rows:
        static_paths = sorted({
            item["source_design_path"]
            for item in [*row["static_bindings"], *row["static_function_contracts"]]
        })
        validator = (
            "scripts/alignment/validate_function_design_review_receipt.py"
            if row["review_surface"] == "FUNCTION_SET"
            else "scripts/alignment/validate_non_function_design_exemption_contract.py"
        )
        output = row["expected_artifact_path"]
        orders.append({
            "work_order_id": stable_id("REV", row["atomic_pr_id"]),
            "work_order_kind": "FUNCTION_OR_EXEMPTION_REVIEW",
            "subject_id": row["atomic_pr_id"],
            "consumer_atomic_pr_ids": [row["atomic_pr_id"]],
            "parent_task_ids": [row["parent_task_id"]],
            "owner_role": "function-design-review-coordinator" if row["review_surface"] == "FUNCTION_SET" else "non-function-exemption-review-coordinator",
            "details": details(
                target_locators=row["write_locators"], leaf_id=row["leaf_id"],
                review_surface=row["review_surface"], static_contract_paths=static_paths,
            ),
            "input_paths": sorted({
                DESIGN_MANIFEST_PATH, IMPLEMENTATION_MANIFEST_PATH, *static_paths,
                *(locator_path(item) for item in row["write_locators"]),
            }),
            "output_paths": [output],
            "command_plan": [step(
                "validate-review-receipt",
                "after independent named reviewers create and sign the immutable review artifact",
                ".", ["python3", validator, output],
                "validate specialized review semantics before independent trusted signature verification",
            )],
            "aggregate_validation_argv": ["python3", "scripts/alignment/generate_m02_gate_input_preflight.py", "--verify"],
            "prerequisite_ids": ["M02-EXT-DESIGN-CANDIDATE", "M02-EXT-IMPLEMENTATION-CANDIDATE", "M02-EXT-TRUSTED-SIGNATURE-VERIFIER"],
            "status": "BLOCKED_DESIGN_IMPLEMENTATION_REVIEW_AND_TRUSTED_SIGNATURE_MISSING",
            "blocking_reasons": [
                "clean same-commit candidate is absent",
                "independent specialized review identities and decision are absent",
                "T1-M01-N010 trusted cryptographic verifier is absent",
            ],
            **COMMON,
        })

    for task_id, owner_role in sorted(task_roles.items()):
        orders.append({
            "work_order_id": stable_id("RSP", task_id),
            "work_order_kind": "RESPONSIBILITY_ASSIGNMENT",
            "subject_id": task_id,
            "consumer_atomic_pr_ids": [],
            "parent_task_ids": [task_id],
            "owner_role": owner_role,
            "details": details(task_id=task_id),
            "input_paths": [
                "contracts/alignment/m02-responsibility-assignment.schema.json",
                "contracts/alignment/signed-contract-intake.schema.json",
                "contracts/alignment/task-registry.v1.json",
                "contracts/alignment/m02-code-direct-leaf-catalog.v4.json",
            ],
            "output_paths": [RESPONSIBILITY_PATH, RESPONSIBILITY_INTAKE_PATH],
            "command_plan": [step(
                "validate-responsibility-exact-set",
                "after all sixteen independent named assignments are recorded in one immutable manifest",
                ".", ["python3", "scripts/alignment/validate_m02_responsibility_assignment.py", RESPONSIBILITY_PATH],
                "validate the complete sixteen-task identity and owner-role exact-set",
            )],
            "aggregate_validation_argv": ["python3", "scripts/alignment/generate_m02_gate_input_preflight.py", "--verify"],
            "prerequisite_ids": ["M02-EXT-NAMED-RESPONSIBILITY", "M02-EXT-TRUSTED-SIGNATURE-VERIFIER"],
            "status": "BLOCKED_NAMED_RESPONSIBILITY_AND_TRUSTED_SIGNATURE_MISSING",
            "blocking_reasons": [
                "named owner reviewer and approver identities have not been provided",
                "responsibility signed intake has not been provided",
                "T1-M01-N010 trusted cryptographic verifier is absent",
            ],
            **COMMON,
        })

    orders.sort(key=lambda item: (item["work_order_kind"], item["work_order_id"]))
    ids = [item["work_order_id"] for item in orders]
    if len(orders) != 923 or len(ids) != len(set(ids)):
        raise ValueError("derived work-order count or identity uniqueness drifted")
    kind_counts = Counter(item["work_order_kind"] for item in orders)
    if kind_counts != {
        "LOCATOR_RESOLUTION": 269,
        "COMPATIBILITY_DEFAULT_OFF_REVIEW": 213,
        "FUNCTION_OR_EXEMPTION_REVIEW": 425,
        "RESPONSIBILITY_ASSIGNMENT": 16,
    }:
        raise ValueError(f"derived work-order kind counts drifted: {dict(kind_counts)}")

    contract_paths = {
        SCHEMA,
        REPO / "contracts/alignment/design-candidate-manifest.schema.json",
        REPO / "contracts/alignment/implementation-candidate.schema.json",
        REPO / "contracts/alignment/m02-compatibility-default-off-review.schema.json",
        REPO / "contracts/alignment/function-design-review-receipt.schema.json",
        REPO / "contracts/alignment/non-function-design-exemption.schema.json",
        REPO / "contracts/alignment/m02-responsibility-assignment.schema.json",
        REPO / "contracts/alignment/signed-contract-intake.schema.json",
        REPO / "scripts/alignment/validate_m02_compatibility_default_off_review.py",
        REPO / "scripts/alignment/validate_function_design_review_receipt.py",
        REPO / "scripts/alignment/validate_non_function_design_exemption_contract.py",
        REPO / "scripts/alignment/validate_m02_responsibility_assignment.py",
        REPO / "scripts/alignment/generate_m02_gate_input_preflight.py",
        REPO / "scripts/alignment/freeze_m02_design_candidate.py",
        REPO / "scripts/alignment/test_freeze_m02_design_candidate.py",
        REPO / "contracts/alignment/m02-implementation-candidate-closure.schema.json",
        REPO / "scripts/alignment/generate_m02_implementation_candidate_closure.py",
    }
    for profile in LOCATOR_PROFILES.values():
        contract_paths.add(REPO / profile["resolver"])
        contract_paths.add(REPO / profile["schema"])

    return {
        "schema_version": "1.0.0",
        "artifact_kind": "M02_EXTERNAL_INPUT_WORK_ORDER_CATALOG",
        "artifact_status": "BLOCKED_EXTERNAL_PREREQUISITES",
        "source_refs": [hash_ref(item) for item in sorted([LOCATOR_COVERAGE, REVIEW_COVERAGE, TASK_REGISTRY, REPLACEMENT, PREFLIGHT, IMPLEMENTATION_CLOSURE])],
        "contract_refs": [hash_ref(item) for item in sorted(contract_paths)],
        "external_prerequisites": prerequisites,
        "work_order_count": len(orders),
        "work_order_kind_counts": dict(sorted(kind_counts.items())),
        "work_order_status_counts": dict(sorted(Counter(item["status"] for item in orders).items())),
        "work_order_exact_set_sha256": semantic_sha256(ids),
        "work_orders": orders,
        "validation": {
            "source_hashes_exact": "PASS",
            "expected_sets_derived": "PASS",
            "work_order_ids_unique": "PASS",
            "output_paths_bound": "PASS",
            "command_profiles_bound": "PASS",
            "no_external_identity_signature_candidate_receipt_or_decision_created": True,
            "mutation_guards": {
                "work_order_omission": "PASS",
                "prerequisite_omission": "PASS",
                "preparation_command_drift": "PASS",
                "resolver_command_drift": "PASS",
                "output_path_drift": "PASS",
                "owner_role_drift": "PASS",
                "false_ready": "PASS",
            },
        },
        "proof_ceiling": "WORK_ORDER_ORCHESTRATION_ONLY_NOT_CANDIDATE_CREATION_IDENTITY_ASSIGNMENT_SIGNATURE_REVIEW_DECISION_LOCATOR_RECEIPT_IMPLEMENTATION_EXECUTION_REGISTRY_SWITCH_OR_ACCEPTANCE",
    }


def by_id(payload: dict[str, Any]) -> dict[str, dict[str, Any]]:
    return {item["work_order_id"]: item for item in payload["work_orders"]}


def validate_semantics(payload: dict[str, Any], *, exact: bool) -> None:
    validate_against_schema(payload, SCHEMA)
    expected = build()
    expected_prerequisites = {
        item["prerequisite_id"]: item for item in expected["external_prerequisites"]
    }
    actual_prerequisites = {
        item["prerequisite_id"]: item for item in payload["external_prerequisites"]
    }
    if list(actual_prerequisites) != list(expected_prerequisites):
        raise ValueError("external prerequisite exact-set drifted")
    for prerequisite_id, wanted in expected_prerequisites.items():
        actual = actual_prerequisites[prerequisite_id]
        if actual["preparation_command_plan"] != wanted["preparation_command_plan"]:
            raise ValueError(
                f"external prerequisite preparation command drifted: {prerequisite_id}"
            )
        if actual != wanted:
            raise ValueError(f"external prerequisite state drifted: {prerequisite_id}")
    expected_ids = [item["work_order_id"] for item in expected["work_orders"]]
    actual_ids = [item["work_order_id"] for item in payload["work_orders"]]
    if actual_ids != expected_ids:
        raise ValueError("work order exact-set drifted")
    actual, wanted = by_id(payload), by_id(expected)
    for work_id in expected_ids:
        if actual[work_id]["command_plan"] != wanted[work_id]["command_plan"]:
            raise ValueError(f"work order command drifted: {work_id}")
        if actual[work_id]["output_paths"] != wanted[work_id]["output_paths"]:
            raise ValueError(f"work order output path drifted: {work_id}")
        if actual[work_id]["owner_role"] != wanted[work_id]["owner_role"]:
            raise ValueError(f"work order owner role drifted: {work_id}")
        if actual[work_id]["status"] != wanted[work_id]["status"]:
            raise ValueError(f"work order status falsely advanced: {work_id}")
    if payload["work_order_exact_set_sha256"] != semantic_sha256(actual_ids):
        raise ValueError("work order exact-set hash drifted")
    if exact and payload != expected:
        raise ValueError("M02 external-input work-order catalog differs from exact derived state")


def expect_failure(label: str, payload: dict[str, Any], mutate: Callable[[dict[str, Any]], None], expected: str) -> None:
    candidate = copy.deepcopy(payload)
    mutate(candidate)
    try:
        validate_semantics(candidate, exact=False)
    except (ValueError, TypeError) as exc:
        if expected not in str(exc):
            raise ValueError(f"mutation {label} hit wrong assertion: {exc}") from exc
        return
    raise ValueError(f"mutation {label} did not fail")


def mutation_tests(payload: dict[str, Any]) -> None:
    locator_index = next(index for index, item in enumerate(payload["work_orders"]) if item["work_order_kind"] == "LOCATOR_RESOLUTION")
    responsibility_index = next(index for index, item in enumerate(payload["work_orders"]) if item["work_order_kind"] == "RESPONSIBILITY_ASSIGNMENT")
    expect_failure("work order omission", payload, lambda item: item["work_orders"].pop(), "schema minItems failed")
    expect_failure(
        "prerequisite omission", payload,
        lambda item: item["external_prerequisites"].pop(),
        "schema minItems failed",
    )
    expect_failure(
        "preparation command drift", payload,
        lambda item: item["external_prerequisites"][0]["preparation_command_plan"][-1]["argv"].append("--unsafe"),
        "external prerequisite preparation command drifted",
    )
    expect_failure(
        "resolver command drift", payload,
        lambda item: item["work_orders"][locator_index]["command_plan"][-1]["argv"].append("--untrusted"),
        "work order command drifted",
    )
    expect_failure(
        "output path drift", payload,
        lambda item: item["work_orders"][locator_index]["output_paths"].__setitem__(0, "doc/02_acceptance/topic1/m02/rogue.json"),
        "work order output path drifted",
    )
    expect_failure(
        "owner role drift", payload,
        lambda item: item["work_orders"][responsibility_index].update({"owner_role": "wrong-owner-role"}),
        "work order owner role drifted",
    )
    expect_failure(
        "false ready", payload,
        lambda item: item["work_orders"][locator_index].update({"status": "BLOCKED_DESIGN_IMPLEMENTATION_REVIEW_AND_TRUSTED_SIGNATURE_MISSING"}),
        "work order status falsely advanced",
    )


def render(payload: dict[str, Any]) -> str:
    counts = payload["work_order_kind_counts"]
    statuses = payload["work_order_status_counts"]
    lines = [
        "# M02外部证据可领取工作单目录", "",
        f"状态：`{payload['artifact_status']} / NO-GO`", "",
        "本目录把外部输入拆成923项稳定工作单，但不创建候选、产品实现、实名、签名、评审结论或收据。命令只能在对应前置条件真实满足后运行。", "",
        "## Exact-set", "",
        "| 工作单类型 | 数量 |", "|---|---:|",
        f"| Locator resolution | {counts['LOCATOR_RESOLUTION']} |",
        f"| Compatibility/default-off review | {counts['COMPATIBILITY_DEFAULT_OFF_REVIEW']} |",
        f"| Function review / non-function exemption | {counts['FUNCTION_OR_EXEMPTION_REVIEW']} |",
        f"| Named responsibility | {counts['RESPONSIBILITY_ASSIGNMENT']} |",
        f"| **合计** | **{payload['work_order_count']}** |", "",
        f"工作单ID exact-set SHA-256：`{payload['work_order_exact_set_sha256']}`。", "",
        "## 当前硬阻断", "",
        "| 前置条件 | 权责任务 | 状态 | 缺失 |", "|---|---|---|---:|",
    ]
    for item in payload["external_prerequisites"]:
        lines.append(f"| `{item['prerequisite_id']}` | `{item['authority_task_id'] or 'external input'}` | `{item['status']}` | {len(item['missing_paths'])}/{len(item['required_paths'])} |")
    lines.extend(["", "## 状态分布", ""])
    for status, count in sorted(statuses.items()):
        lines.append(f"- `{status}`: {count}")
    lines.extend([
        "", "## 使用方式", "",
        "从JSON目录按`work_order_id`领取；先满足`prerequisite_ids`，再逐项执行`command_plan.argv`，最后执行`aggregate_validation_argv`。`${...}`为调用方必须从同一冻结候选提供的显式变量，不允许用当前worktree或占位值替代。", "",
        "Java locator的编译目录`${M02_JAVA_LOCATOR_CLASSES}`必须是调用方提供的隔离临时目录；目录本身不会创建或清理它。", "",
        "即使923项全部生成外部产物，仍须由preflight和registry switch ledger重新做exact-set、可信验签与四目录原子切换复核。", "",
        "## 证明上限", "", f"`{payload['proof_ceiling']}`", "",
    ])
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
    encoded = json.dumps(payload, ensure_ascii=False, indent=2) + "\n"
    markdown = render(payload)
    if args.write:
        OUTPUT.write_text(encoded, encoding="utf-8")
        DOC_OUTPUT.parent.mkdir(parents=True, exist_ok=True)
        DOC_OUTPUT.write_text(markdown, encoding="utf-8")
        print(f"WROTE {OUTPUT.relative_to(REPO)}")
        print(f"WROTE {DOC_OUTPUT.relative_to(REPO)}")
    else:
        if not OUTPUT.is_file() or OUTPUT.read_text(encoding="utf-8") != encoded:
            raise ValueError("generated M02 external-input work-order catalog is stale")
        if not DOC_OUTPUT.is_file() or DOC_OUTPUT.read_text(encoding="utf-8") != markdown:
            raise ValueError("generated M02 external-input work-order markdown is stale")
        validate_semantics(payload, exact=True)
        if args.verify:
            mutation_tests(payload)
        print(
            f"PASS work_orders={payload['work_order_count']} locator={payload['work_order_kind_counts']['LOCATOR_RESOLUTION']} "
            f"compatibility={payload['work_order_kind_counts']['COMPATIBILITY_DEFAULT_OFF_REVIEW']} "
            f"review={payload['work_order_kind_counts']['FUNCTION_OR_EXEMPTION_REVIEW']} "
            f"responsibility={payload['work_order_kind_counts']['RESPONSIBILITY_ASSIGNMENT']}"
        )
    print(f"PROOF_CEILING {payload['proof_ceiling']}")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
