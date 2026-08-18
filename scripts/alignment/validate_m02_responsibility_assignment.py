#!/usr/bin/env python3
"""Validate an M02 responsibility manifest without inventing identities or signatures."""

from __future__ import annotations

import argparse
import copy
import hashlib
import json
from pathlib import Path
from typing import Any, Callable

from build_topic1_task_registry import validate_against_schema


REPO = Path(__file__).resolve().parents[2]
SCHEMA = REPO / "contracts/alignment/m02-responsibility-assignment.schema.json"
TASK_REGISTRY = REPO / "contracts/alignment/task-registry.v1.json"
REPLACEMENT = REPO / "contracts/alignment/m02-code-direct-leaf-catalog.v4.json"


def digest(path: Path) -> str:
    return hashlib.sha256(path.read_bytes()).hexdigest()


def hash_ref(path: Path) -> dict[str, str]:
    return {"path": path.relative_to(REPO).as_posix(), "sha256": digest(path)}


def source_tasks() -> dict[str, dict[str, Any]]:
    payload = json.loads(TASK_REGISTRY.read_text(encoding="utf-8"))
    return {item["task_id"]: item for item in payload["tasks"] if item["milestone_id"] == "T1-M02"}


def validate(payload: dict[str, Any]) -> None:
    validate_against_schema(payload, SCHEMA)
    if payload["source_task_registry"] != hash_ref(TASK_REGISTRY):
        raise ValueError("responsibility source task registry hash drifted")
    if payload["source_replacement_catalog"] != hash_ref(REPLACEMENT):
        raise ValueError("responsibility replacement catalog hash drifted")
    expected = source_tasks()
    actual = {item["task_id"]: item for item in payload["assignments"]}
    if len(actual) != 16 or set(actual) != set(expected):
        raise ValueError("responsibility task exact-set drifted")
    for task_id, assignment in actual.items():
        if assignment["owner_role"] != expected[task_id]["responsibility"]["owner_role"]:
            raise ValueError(f"responsibility owner role drifted: {task_id}")
        identities = [assignment["owner"], *assignment["reviewers"], *assignment["approvers"]]
        if len(identities) != len(set(identities)):
            raise ValueError(f"responsibility identities are not independent: {task_id}")


def fixture() -> dict[str, Any]:
    assignments = []
    for index, (task_id, task) in enumerate(sorted(source_tasks().items()), start=1):
        assignments.append({
            "task_id": task_id,
            "owner_role": task["responsibility"]["owner_role"],
            "owner": f"owner-{index}@example.invalid",
            "reviewers": [f"reviewer-{index}@example.invalid"],
            "approvers": [f"approver-{index}@example.invalid"],
        })
    return {
        "schema_version": "1.0.0",
        "artifact_kind": "M02_RESPONSIBILITY_ASSIGNMENT",
        "assignment_id": "M02-RESP-SELFTEST",
        "source_task_registry": hash_ref(TASK_REGISTRY),
        "source_replacement_catalog": hash_ref(REPLACEMENT),
        "assignments": assignments,
        "proof_ceiling": "NAMED_RESPONSIBILITY_INPUT_ONLY_REQUIRES_INDEPENDENT_SIGNED_INTAKE_NOT_EXECUTION_AUTHORIZATION",
    }


def expect_failure(label: str, payload: dict[str, Any], mutate: Callable[[dict[str, Any]], None], expected: str) -> None:
    candidate = copy.deepcopy(payload)
    mutate(candidate)
    try:
        validate(candidate)
    except (ValueError, TypeError) as exc:
        if expected not in str(exc):
            raise ValueError(f"negative case {label} hit wrong assertion: {exc}") from exc
        return
    raise ValueError(f"negative case {label} did not fail")


def self_test() -> None:
    payload = fixture()
    validate(payload)
    expect_failure("task omission", payload, lambda item: item["assignments"].pop(), "schema minItems failed")
    expect_failure("task duplicate", payload, lambda item: item["assignments"][1].update({"task_id": item["assignments"][0]["task_id"]}), "task exact-set drifted")
    expect_failure("registry hash", payload, lambda item: item["source_task_registry"].update({"sha256": "0" * 64}), "source task registry hash drifted")
    expect_failure("catalog hash", payload, lambda item: item["source_replacement_catalog"].update({"sha256": "0" * 64}), "replacement catalog hash drifted")
    expect_failure("owner role", payload, lambda item: item["assignments"][0].update({"owner_role": "wrong-role"}), "owner role drifted")
    expect_failure("identity reuse", payload, lambda item: item["assignments"][0]["reviewers"].__setitem__(0, item["assignments"][0]["owner"]), "identities are not independent")
    expect_failure("missing reviewer", payload, lambda item: item["assignments"][0].update({"reviewers": []}), "schema minItems failed")
    expect_failure("missing approver", payload, lambda item: item["assignments"][0].update({"approvers": []}), "schema minItems failed")


def main() -> int:
    parser = argparse.ArgumentParser()
    parser.add_argument("path", nargs="?")
    parser.add_argument("--self-test", action="store_true")
    args = parser.parse_args()
    if args.self_test:
        self_test()
        print("PASS M02 responsibility assignment: 1 positive and 8 targeted exact-set/hash/role/identity negative cases")
        print("PROOF_CEILING NAMED_RESPONSIBILITY_INPUT_ONLY_REQUIRES_INDEPENDENT_SIGNED_INTAKE_NOT_EXECUTION_AUTHORIZATION")
        return 0
    if not args.path:
        parser.error("path is required unless --self-test is used")
    validate(json.loads(Path(args.path).read_text(encoding="utf-8")))
    print(f"PASS {args.path}")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
