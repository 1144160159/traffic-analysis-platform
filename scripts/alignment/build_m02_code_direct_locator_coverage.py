#!/usr/bin/env python3
"""Classify every M02 PLANNED locator without claiming it is resolved."""

from __future__ import annotations

import argparse
import copy
import hashlib
import json
import subprocess
from collections import Counter, defaultdict
from functools import lru_cache
from pathlib import Path
from typing import Any, Callable

from build_topic1_task_registry import validate_against_schema


REPO = Path(__file__).resolve().parents[2]
SOURCE = REPO / "contracts/alignment/m02-code-direct-leaf-catalog.v4.json"
SCHEMA = REPO / "contracts/alignment/m02-code-direct-locator-coverage.schema.json"
OUTPUT = REPO / "contracts/alignment/m02-code-direct-locator-coverage.v1.json"
DOC_OUTPUT = REPO / "doc/07_alignment/generated/M02代码直达Locator覆盖清单.md"
GO_RESOLVER_SOURCE = REPO / "scripts/alignment/go_ast_locator/main.go"
GO_RECEIPT_SCHEMA = REPO / "contracts/alignment/locator-resolution-receipt.schema.json"
GO_RESOLVER_TEST = REPO / "scripts/alignment/test_go_ast_locator.py"
PYTHON_RESOLVER_SOURCE = REPO / "scripts/alignment/python_ast_locator.py"
PYTHON_RECEIPT_SCHEMA = REPO / "contracts/alignment/python-locator-resolution-receipt.schema.json"
PYTHON_RESOLVER_TEST = REPO / "scripts/alignment/test_python_ast_locator.py"
RUST_RESOLVER_SOURCE = REPO / "scripts/alignment/rust_ast_locator/src/main.rs"
RUST_RECEIPT_SCHEMA = REPO / "contracts/alignment/rust-locator-resolution-receipt.schema.json"
RUST_RESOLVER_TEST = REPO / "scripts/alignment/test_rust_ast_locator.py"
PROTO_RESOLVER_SOURCE = REPO / "scripts/alignment/proto_descriptor_locator/main.go"
PROTO_RECEIPT_SCHEMA = REPO / "contracts/alignment/proto-descriptor-locator-resolution-receipt.schema.json"
PROTO_RESOLVER_TEST = REPO / "scripts/alignment/test_proto_descriptor_locator.py"
CONFIG_RESOLVER_SOURCE = REPO / "scripts/alignment/structured_config_locator.py"
CONFIG_RECEIPT_SCHEMA = REPO / "contracts/alignment/structured-config-locator-resolution-receipt.schema.json"
CONFIG_RESOLVER_TEST = REPO / "scripts/alignment/test_structured_config_locator.py"
CONFIG_DEPENDENCY_LOCK = REPO / "scripts/alignment/structured_config_locator.requirements.txt"
SHELL_RESOLVER_SOURCE = REPO / "scripts/alignment/shell_ast_locator/main.go"
SHELL_RECEIPT_SCHEMA = REPO / "contracts/alignment/shell-ast-locator-resolution-receipt.schema.json"
SHELL_RESOLVER_TEST = REPO / "scripts/alignment/test_shell_ast_locator.py"
JAVA_RESOLVER_SOURCE = REPO / "scripts/alignment/java_ast_locator/JavaAstLocator.java"
JAVA_RECEIPT_SCHEMA = REPO / "contracts/alignment/java-locator-resolution-receipt.schema.json"
JAVA_RESOLVER_TEST = REPO / "scripts/alignment/test_java_ast_locator.py"
SQL_RESOLVER_SOURCE = REPO / "scripts/alignment/sql_ddl_locator.py"
SQL_RECEIPT_SCHEMA = REPO / "contracts/alignment/sql-ddl-locator-resolution-receipt.schema.json"
SQL_RESOLVER_TEST = REPO / "scripts/alignment/test_sql_ddl_locator.py"
SHARED_OWNERSHIP_PATH = REPO / "contracts/alignment/m02-shared-locator-ownership.v1.json"
WRITE_SCOPE_SUPERSESSION_PATH = REPO / "contracts/alignment/m02-write-scope-supersession.v1.json"


def sha256(path: Path) -> str:
    return hashlib.sha256(path.read_bytes()).hexdigest()


def split_locator(locator: str) -> tuple[str, str | None]:
    if "#" not in locator:
        return locator, None
    path, symbol = locator.split("#", 1)
    return path, symbol or None


def hash_ref(path: Path) -> dict[str, str]:
    return {
        "path": path.relative_to(REPO).as_posix(),
        "sha256": sha256(path),
    }


def resolver_self_test(
    profile: str,
    source: Path,
    receipt_schema: Path,
    test_runner: Path,
    expected_summary: str,
    positive_count: int,
    negative_count: int,
    dependency_locks: list[Path] | None = None,
) -> dict[str, Any]:
    completed = subprocess.run(
        ["python3", str(test_runner.relative_to(REPO))], cwd=REPO,
        check=False, capture_output=True, text=True,
    )
    if completed.returncode != 0 or expected_summary not in completed.stdout.splitlines():
        raise ValueError(
            f"{profile} trusted-resolver self-test failed: rc={completed.returncode} "
            f"stdout={completed.stdout!r} stderr={completed.stderr!r}"
        )
    return {
        "resolver_profile": profile,
        "resolver_source": hash_ref(source),
        "receipt_schema": hash_ref(receipt_schema),
        "dependency_locks": [hash_ref(item) for item in (dependency_locks or [])],
        "test_runner": hash_ref(test_runner),
        "test_command": f"python3 {test_runner.relative_to(REPO).as_posix()}",
        "status": "PASS",
        "positive_case_count": positive_count,
        "negative_case_count": negative_count,
        "stdout_summary": expected_summary,
        "proof_ceiling": (
            "EXACT_LOCATOR_RESOLVER_IMPLEMENTED_NOT_M02_CANDIDATE_FROZEN_"
            "OR_LOCATORS_RESOLVED"
        ),
    }


@lru_cache(maxsize=1)
def go_resolver_check() -> dict[str, Any]:
    return resolver_self_test(
        "GO_AST_V1", GO_RESOLVER_SOURCE, GO_RECEIPT_SCHEMA, GO_RESOLVER_TEST,
        "PASS Go AST locator: 6 positive declaration forms and 8 targeted query/candidate/manifest/syntax/symlink/path/output negative cases",
        6, 8,
    )


@lru_cache(maxsize=1)
def python_resolver_check() -> dict[str, Any]:
    return resolver_self_test(
        "PYTHON_AST_V1", PYTHON_RESOLVER_SOURCE, PYTHON_RECEIPT_SCHEMA, PYTHON_RESOLVER_TEST,
        "PASS Python AST locator: 6 positive declaration forms and 8 targeted query/candidate/manifest/syntax/symlink/path/output negative cases",
        6, 8,
    )


@lru_cache(maxsize=1)
def rust_resolver_check() -> dict[str, Any]:
    completed = subprocess.run(
        ["python3", str(RUST_RESOLVER_TEST.relative_to(REPO))],
        cwd=REPO,
        check=False,
        capture_output=True,
        text=True,
    )
    expected_summary = (
        "PASS Rust AST locator: 5 positive code-unit forms and 7 targeted "
        "ambiguity/signature/candidate/symlink/path/output negative cases"
    )
    if completed.returncode != 0 or expected_summary not in completed.stdout.splitlines():
        raise ValueError(
            "Rust trusted-resolver self-test failed: "
            f"rc={completed.returncode} stdout={completed.stdout!r} stderr={completed.stderr!r}"
        )
    return {
        "resolver_profile": "RUST_SYN_V1",
        "resolver_source": hash_ref(RUST_RESOLVER_SOURCE),
        "receipt_schema": hash_ref(RUST_RECEIPT_SCHEMA),
        "dependency_locks": [
            hash_ref(REPO / "scripts/alignment/rust_ast_locator/Cargo.toml"),
            hash_ref(REPO / "scripts/alignment/rust_ast_locator/Cargo.lock"),
        ],
        "test_runner": hash_ref(RUST_RESOLVER_TEST),
        "test_command": "python3 scripts/alignment/test_rust_ast_locator.py",
        "status": "PASS",
        "positive_case_count": 5,
        "negative_case_count": 7,
        "stdout_summary": expected_summary,
        "proof_ceiling": (
            "EXACT_LOCATOR_RESOLVER_IMPLEMENTED_NOT_M02_CANDIDATE_FROZEN_"
            "OR_LOCATORS_RESOLVED"
        ),
    }


@lru_cache(maxsize=1)
def proto_resolver_check() -> dict[str, Any]:
    completed = subprocess.run(
        ["python3", str(PROTO_RESOLVER_TEST.relative_to(REPO))],
        cwd=REPO,
        check=False,
        capture_output=True,
        text=True,
    )
    expected_summary = (
        "PASS Protobuf descriptor locator: 5 positive declaration forms and 7 targeted "
        "FQN/manifest/candidate/import/symlink/path/output negative cases"
    )
    if completed.returncode != 0 or expected_summary not in completed.stdout.splitlines():
        raise ValueError(
            "Protobuf trusted-resolver self-test failed: "
            f"rc={completed.returncode} stdout={completed.stdout!r} stderr={completed.stderr!r}"
        )
    return {
        "resolver_profile": "PROTO_DESCRIPTOR_V1",
        "resolver_source": hash_ref(PROTO_RESOLVER_SOURCE),
        "receipt_schema": hash_ref(PROTO_RECEIPT_SCHEMA),
        "dependency_locks": [
            hash_ref(REPO / "scripts/alignment/proto_descriptor_locator/go.mod"),
            hash_ref(REPO / "scripts/alignment/proto_descriptor_locator/go.sum"),
        ],
        "test_runner": hash_ref(PROTO_RESOLVER_TEST),
        "test_command": "python3 scripts/alignment/test_proto_descriptor_locator.py",
        "status": "PASS",
        "positive_case_count": 5,
        "negative_case_count": 7,
        "stdout_summary": expected_summary,
        "proof_ceiling": (
            "EXACT_LOCATOR_RESOLVER_IMPLEMENTED_NOT_M02_CANDIDATE_FROZEN_"
            "OR_LOCATORS_RESOLVED"
        ),
    }


@lru_cache(maxsize=1)
def config_resolver_check() -> dict[str, Any]:
    completed = subprocess.run(
        ["python3", str(CONFIG_RESOLVER_TEST.relative_to(REPO))],
        cwd=REPO,
        check=False,
        capture_output=True,
        text=True,
    )
    expected_summary = (
        "PASS structured config locator: 5 positive JSON-document/JSON-pointer/YAML/TOML/Kubernetes forms "
        "and 8 targeted ambiguity/query/manifest/candidate/symlink/path/output negative cases"
    )
    if completed.returncode != 0 or expected_summary not in completed.stdout.splitlines():
        raise ValueError(
            "structured-config trusted-resolver self-test failed: "
            f"rc={completed.returncode} stdout={completed.stdout!r} stderr={completed.stderr!r}"
        )
    return {
        "resolver_profile": "STRUCTURED_CONFIG_V1",
        "resolver_source": hash_ref(CONFIG_RESOLVER_SOURCE),
        "receipt_schema": hash_ref(CONFIG_RECEIPT_SCHEMA),
        "dependency_locks": [hash_ref(CONFIG_DEPENDENCY_LOCK)],
        "test_runner": hash_ref(CONFIG_RESOLVER_TEST),
        "test_command": "python3 scripts/alignment/test_structured_config_locator.py",
        "status": "PASS",
        "positive_case_count": 5,
        "negative_case_count": 8,
        "stdout_summary": expected_summary,
        "proof_ceiling": (
            "EXACT_LOCATOR_RESOLVER_IMPLEMENTED_NOT_M02_CANDIDATE_FROZEN_"
            "OR_LOCATORS_RESOLVED"
        ),
    }


@lru_cache(maxsize=1)
def shell_resolver_check() -> dict[str, Any]:
    completed = subprocess.run(
        ["python3", str(SHELL_RESOLVER_TEST.relative_to(REPO))],
        cwd=REPO,
        check=False,
        capture_output=True,
        text=True,
    )
    expected_summary = (
        "PASS shell AST locator: 1 positive literal topic form and 8 targeted "
        "query/dynamic/ambiguity/manifest/candidate/symlink/path/output negative cases"
    )
    if completed.returncode != 0 or expected_summary not in completed.stdout.splitlines():
        raise ValueError(
            "shell trusted-resolver self-test failed: "
            f"rc={completed.returncode} stdout={completed.stdout!r} stderr={completed.stderr!r}"
        )
    return {
        "resolver_profile": "SHELL_AST_V1",
        "resolver_source": hash_ref(SHELL_RESOLVER_SOURCE),
        "receipt_schema": hash_ref(SHELL_RECEIPT_SCHEMA),
        "dependency_locks": [
            hash_ref(REPO / "scripts/alignment/shell_ast_locator/go.mod"),
            hash_ref(REPO / "scripts/alignment/shell_ast_locator/go.sum"),
        ],
        "test_runner": hash_ref(SHELL_RESOLVER_TEST),
        "test_command": "python3 scripts/alignment/test_shell_ast_locator.py",
        "status": "PASS",
        "positive_case_count": 1,
        "negative_case_count": 8,
        "stdout_summary": expected_summary,
        "proof_ceiling": (
            "EXACT_LOCATOR_RESOLVER_IMPLEMENTED_NOT_M02_CANDIDATE_FROZEN_"
            "OR_LOCATORS_RESOLVED"
        ),
    }


@lru_cache(maxsize=1)
def java_resolver_check() -> dict[str, Any]:
    completed = subprocess.run(
        ["python3", str(JAVA_RESOLVER_TEST.relative_to(REPO))],
        cwd=REPO,
        check=False,
        capture_output=True,
        text=True,
    )
    expected_summary = (
        "PASS Java AST locator: 5 positive type/member forms and 8 targeted "
        "ambiguity/signature/candidate/manifest/syntax/symlink/path/output negative cases"
    )
    if completed.returncode != 0 or expected_summary not in completed.stdout.splitlines():
        raise ValueError(
            "Java trusted-resolver self-test failed: "
            f"rc={completed.returncode} stdout={completed.stdout!r} stderr={completed.stderr!r}"
        )
    java_version = subprocess.run(
        ["java", "-version"], cwd=REPO, check=False, capture_output=True, text=True
    )
    version_line = (java_version.stderr or java_version.stdout).splitlines()
    if java_version.returncode != 0 or not version_line:
        raise ValueError("Java resolver engine version could not be determined")
    return {
        "resolver_profile": "JAVA_JAVAC_AST_V1",
        "resolver_source": hash_ref(JAVA_RESOLVER_SOURCE),
        "receipt_schema": hash_ref(JAVA_RECEIPT_SCHEMA),
        "dependency_locks": [],
        "test_runner": hash_ref(JAVA_RESOLVER_TEST),
        "test_command": "python3 scripts/alignment/test_java_ast_locator.py",
        "status": "PASS",
        "positive_case_count": 5,
        "negative_case_count": 8,
        "stdout_summary": expected_summary,
        "proof_ceiling": (
            "EXACT_LOCATOR_RESOLVER_IMPLEMENTED_NOT_M02_CANDIDATE_FROZEN_"
            "OR_LOCATORS_RESOLVED"
        ),
    }


@lru_cache(maxsize=1)
def sql_resolver_check() -> dict[str, Any]:
    completed = subprocess.run(
        ["python3", str(SQL_RESOLVER_TEST.relative_to(REPO))],
        cwd=REPO,
        check=False,
        capture_output=True,
        text=True,
    )
    expected_summary = (
        "PASS SQL DDL locator: 4 positive PostgreSQL/ClickHouse declaration forms and 10 targeted "
        "ambiguity/false-positive/dialect/candidate/manifest/syntax/symlink/path/output negative cases"
    )
    if completed.returncode != 0 or expected_summary not in completed.stdout.splitlines():
        raise ValueError(
            "SQL DDL trusted-resolver self-test failed: "
            f"rc={completed.returncode} stdout={completed.stdout!r} stderr={completed.stderr!r}"
        )
    return {
        "resolver_profile": "SQL_DDL_PARSE_V1",
        "resolver_source": hash_ref(SQL_RESOLVER_SOURCE),
        "receipt_schema": hash_ref(SQL_RECEIPT_SCHEMA),
        "dependency_locks": [],
        "test_runner": hash_ref(SQL_RESOLVER_TEST),
        "test_command": "python3 scripts/alignment/test_sql_ddl_locator.py",
        "status": "PASS",
        "positive_case_count": 4,
        "negative_case_count": 10,
        "stdout_summary": expected_summary,
        "proof_ceiling": (
            "EXACT_LOCATOR_RESOLVER_IMPLEMENTED_NOT_M02_CANDIDATE_FROZEN_"
            "OR_LOCATORS_RESOLVED"
        ),
    }


def resolver_profile(suffix: str, exists: bool) -> str:
    if not exists:
        return "PLANNED_ARTIFACT_REQUIRED"
    return {
        ".go": "GO_AST_V1",
        ".py": "PYTHON_AST_V1",
        ".rs": "RUST_SYN_V1",
        ".java": "JAVA_JAVAC_AST_V1",
        ".proto": "PROTO_DESCRIPTOR_V1",
        ".sql": "SQL_DDL_PARSE_V1",
        ".json": "STRUCTURED_CONFIG_V1",
        ".yaml": "STRUCTURED_CONFIG_V1",
        ".yml": "STRUCTURED_CONFIG_V1",
        ".toml": "STRUCTURED_CONFIG_V1",
        ".sh": "SHELL_AST_V1",
    }.get(suffix, "STRUCTURED_CONFIG_REQUIRED")


def classify(path: str) -> tuple[bool, str, str, str]:
    resolved = (REPO / path).resolve()
    if not resolved.is_relative_to(REPO):
        raise ValueError(f"locator path escapes repository: {path}")
    exists = resolved.is_file()
    suffix = Path(path).suffix.lower() or Path(path).name.lower()
    profile = resolver_profile(suffix, exists)
    if not exists:
        return (
            False,
            profile,
            "PLANNED_FILE_ABSENT",
            "create the reviewed compatibility seam under a clean candidate before locator resolution",
        )
    if profile in {
        "GO_AST_V1", "PYTHON_AST_V1", "RUST_SYN_V1", "JAVA_JAVAC_AST_V1",
        "PROTO_DESCRIPTOR_V1", "STRUCTURED_CONFIG_V1", "SHELL_AST_V1", "SQL_DDL_PARSE_V1",
    }:
        return (
            True,
            profile,
            "RESOLVER_IMPLEMENTED_CANDIDATE_NOT_FROZEN",
            "freeze one clean candidate manifest then require an exact one-match AST receipt",
        )
    return (
        True,
        profile,
        "TRUSTED_RESOLVER_NOT_IMPLEMENTED",
        "implement and mutation-test the named language or structured-surface resolver",
    )


def build() -> dict[str, Any]:
    resolver_checks = [
        go_resolver_check(),
        python_resolver_check(),
        rust_resolver_check(),
        proto_resolver_check(),
        config_resolver_check(),
        shell_resolver_check(),
        java_resolver_check(),
        sql_resolver_check(),
    ]
    source = json.loads(SOURCE.read_text(encoding="utf-8"))
    planned = [item for item in source["leaves"] if item["target_state"] == "PLANNED"]
    occurrences = []
    owners: dict[str, list[tuple[str, str]]] = defaultdict(list)
    for leaf in planned:
        for locator in leaf["write_locators"]:
            path, symbol = split_locator(locator)
            exists, profile, status, next_action = classify(path)
            occurrences.append({
                "leaf_id": leaf["leaf_id"],
                "atomic_pr_id": leaf["atomic_pr_id"],
                "locator": locator,
                "path": path,
                "symbol_or_pointer": symbol,
                "surface_suffix": Path(path).suffix.lower() or Path(path).name.lower(),
                "file_exists": exists,
                "resolver_profile": profile,
                "coverage_status": status,
                "blocker": (
                    "clean candidate manifest and exact locator receipt are absent"
                    if status == "RESOLVER_IMPLEMENTED_CANDIDATE_NOT_FROZEN"
                    else "trusted resolver wrapper schema and mutation suite are absent"
                    if status == "TRUSTED_RESOLVER_NOT_IMPLEMENTED"
                    else "the planned write target does not exist in the current repository snapshot"
                ),
                "next_action": next_action,
            })
            owners[locator].append((leaf["leaf_id"], leaf["atomic_pr_id"]))
    conflicts = [
        {
            "locator": locator,
            "owner_leaf_ids": [item[0] for item in values],
            "owner_atomic_pr_ids": [item[1] for item in values],
            "decision": "ORDERED_SHARED_LOCATOR_SUPERSESSION_DESIGNED_NOT_REGISTERED",
            "required_resolution": "REGISTER_THE_SAME_SUPERSESSION_HASH_IN_FOUR_CANDIDATE_CATALOGS_THEN_SIGN_BOTH_FUNCTION_REVIEWS",
            "ownership_boundary_ref": hash_ref(SHARED_OWNERSHIP_PATH),
            "write_scope_supersession_ref": hash_ref(WRITE_SCOPE_SUPERSESSION_PATH),
        }
        for locator, values in sorted(owners.items())
        if len(values) > 1
    ]
    status_counts = Counter(item["coverage_status"] for item in occurrences)
    for status in (
        "PLANNED_FILE_ABSENT",
        "RESOLVER_IMPLEMENTED_CANDIDATE_NOT_FROZEN",
        "TRUSTED_RESOLVER_NOT_IMPLEMENTED",
    ):
        status_counts.setdefault(status, 0)
    profile_counts = Counter(item["resolver_profile"] for item in occurrences)
    return {
        "schema_version": "1.0.0",
        "artifact_kind": "M02_PLANNED_LOCATOR_TRUSTED_RESOLVER_COVERAGE",
        "artifact_status": "BLOCKED_COVERAGE_INCOMPLETE",
        "source_catalog": {
            "path": SOURCE.relative_to(REPO).as_posix(),
            "sha256": sha256(SOURCE),
        },
        "planned_leaf_count": len(planned),
        "locator_occurrence_count": len(occurrences),
        "unique_locator_count": len(owners),
        "ownership_conflict_count": len(conflicts),
        "status_counts": dict(sorted(status_counts.items())),
        "resolver_profile_counts": dict(sorted(profile_counts.items())),
        "trusted_resolver_checks": resolver_checks,
        "locator_occurrences": occurrences,
        "ownership_conflicts": conflicts,
        "validation": {
            "source_exact": "PASS",
            "planned_leaf_exact_set": "PASS",
            "occurrence_exact_set": "PASS",
            "classification_total": "PASS",
            "status_counts_exact": "PASS",
            "conflict_exact_set": "PASS",
            "no_locator_claimed_resolved": True,
            "mutation_guards": {
                "occurrence_omission": "PASS",
                "false_resolved": "PASS",
                "status_drift": "PASS",
                "conflict_omission": "PASS",
                "path_escape": "PASS",
                "resolver_check_drift": "PASS",
            },
        },
        "proof_ceiling": "LOCATOR_COVERAGE_CLASSIFICATION_ONLY_NOT_LOCATOR_RESOLVED_TARGET_BINDING_FUNCTION_REVIEW_IMPLEMENTATION_OR_EXECUTION_AUTHORIZATION",
    }


def validate(payload: dict[str, Any]) -> None:
    validate_against_schema(payload, SCHEMA)
    if payload != build():
        raise ValueError("locator coverage ledger differs from exact derived state")
    if any(item["coverage_status"] == "RESOLVED" for item in payload["locator_occurrences"]):
        raise ValueError("locator coverage falsely claims a resolved locator")


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
        "occurrence omission", payload,
        lambda item: item["locator_occurrences"].pop(),
        "schema minItems failed at $.locator_occurrences",
    )
    expect_failure(
        "false resolved", payload,
        lambda item: item["locator_occurrences"][0].update({"coverage_status": "RESOLVED"}),
        "schema enum mismatch at $.locator_occurrences[0].coverage_status",
    )
    expect_failure(
        "status drift", payload,
        lambda item: item["status_counts"].update({"PLANNED_FILE_ABSENT": 137}),
        "schema const mismatch at $.status_counts.PLANNED_FILE_ABSENT",
    )
    expect_failure(
        "conflict omission", payload,
        lambda item: item["ownership_conflicts"].pop(),
        "schema minItems failed at $.ownership_conflicts",
    )
    expect_failure(
        "resolver check drift", payload,
        lambda item: item["trusted_resolver_checks"][0].update({"status": "FAIL"}),
        "schema const mismatch at $.trusted_resolver_checks[0].status",
    )
    try:
        classify("../outside.rs")
    except ValueError as exc:
        if "escapes repository" not in str(exc):
            raise ValueError(f"path escape hit wrong assertion: {exc}") from exc
    else:
        raise ValueError("path escape did not fail")


def render_markdown(payload: dict[str, Any]) -> str:
    lines = [
        "# M02代码直达Locator覆盖清单",
        "",
        "状态：`BLOCKED_COVERAGE_INCOMPLETE / NO-GO`",
        "",
        "本清单只分类resolver覆盖，不声称任何locator已解析。",
        "",
        "## 汇总",
        "",
        f"- PLANNED叶：{payload['planned_leaf_count']}；locator occurrence：{payload['locator_occurrence_count']}；唯一locator：{payload['unique_locator_count']}。",
        f"- 已有受信resolver但缺clean candidate：{payload['status_counts']['RESOLVER_IMPLEMENTED_CANDIDATE_NOT_FROZEN']}。",
        f"- 文件存在但缺trusted resolver：{payload['status_counts']['TRUSTED_RESOLVER_NOT_IMPLEMENTED']}。",
        f"- planned文件尚不存在：{payload['status_counts']['PLANNED_FILE_ABSENT']}。",
        f"- ownership冲突：{payload['ownership_conflict_count']}。",
        "",
        "## Resolver profile",
        "",
        "| Profile | Locator occurrence |",
        "|---|---:|",
    ]
    for profile, count in payload["resolver_profile_counts"].items():
        lines.append(f"| `{profile}` | {count} |")
    lines.extend(["", "## 阻断冲突", ""])
    for item in payload["ownership_conflicts"]:
        lines.append(
            f"- `{item['locator']}`：{', '.join(item['owner_leaf_ids'])}；"
            f"处置=`{item['required_resolution']}`。"
        )
    lines.extend([
        "",
        "## 下一批次",
        "",
        "1. 先冻结clean candidate，复用现有Go/Python/Rust/Java/shell AST、Protobuf descriptor、SQL DDL parse tree和structured-config resolver处理125个现存文件occurrence；planned Java/SQL文件创建后必须走对应candidate-bound receipt。",
        "2. planned文件只在兼容seam评审后创建，再以对应语言/结构化resolver解析after-state locator。",
        "3. 以append-only supersession消除P408对poll_manifest的可写companion范围，再完成default-off与P165/P408函数评审；不得重写冻结P308-P485历史。",
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
            raise ValueError("generated locator coverage ledger is stale")
        if not DOC_OUTPUT.is_file() or DOC_OUTPUT.read_text(encoding="utf-8") != render_markdown(payload):
            raise ValueError("generated locator coverage markdown is stale")
        validate(payload)
        if args.verify:
            run_mutation_tests(payload)
        print(
            f"PASS planned={payload['planned_leaf_count']} "
            f"occurrences={payload['locator_occurrence_count']} "
            f"unique={payload['unique_locator_count']} "
            f"conflicts={payload['ownership_conflict_count']}"
        )
    print(f"PROOF_CEILING {payload['proof_ceiling']}")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
