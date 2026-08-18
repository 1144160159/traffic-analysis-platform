#!/usr/bin/env python3
"""Validate and execute the M06 four-source pre-enable fixture matrix."""

from __future__ import annotations

import argparse
import hashlib
import json
import subprocess
import time
from pathlib import Path
from typing import Any


ROOT = Path(__file__).resolve().parents[2]
MATRIX = ROOT / "tests/fixtures/m06/four-source-matrix.v1.json"
SOURCES = {"flow", "asset", "device_log", "user_behavior"}
KINDS = {"positive", "permission_negative", "bad_message", "replay"}


def candidate_digest(paths: list[str], root: Path = ROOT) -> str:
    digest = hashlib.sha256()
    for relative in paths:
        path = root / relative
        if not path.is_file():
            raise ValueError(f"candidate file is missing: {relative}")
        digest.update(
            f"{hashlib.sha256(path.read_bytes()).hexdigest()}  {relative}\n".encode()
        )
    return digest.hexdigest()


def validate_matrix(matrix: dict[str, Any], root: Path = ROOT) -> None:
    if matrix.get("accountable_task") != "T1-M06-N013":
        raise ValueError("matrix accountable_task must be T1-M06-N013")
    if matrix.get("state") != "pre_enable_fixture_only_not_promotion_evidence":
        raise ValueError("fixture matrix must not claim promotion evidence")
    if matrix.get("runtime_change") is not False:
        raise ValueError("fixture matrix must not request a runtime change")
    sources = matrix.get("sources")
    if not isinstance(sources, dict) or set(sources) != SOURCES:
        raise ValueError("fixture matrix must contain the exact four sources")
    seen_ids: set[str] = set()
    for source_name, source in sources.items():
        command = source.get("command")
        if not isinstance(command, list) or not command or not all(
            isinstance(item, str) and item for item in command
        ):
            raise ValueError(f"{source_name} command is invalid")
        if command[0] not in {"mvn", "go"}:
            raise ValueError(f"{source_name} command must execute owned code tests")
        workdir = root / str(source.get("workdir", ""))
        if not workdir.is_dir():
            raise ValueError(f"{source_name} workdir is missing")
        cases = source.get("cases")
        if not isinstance(cases, list) or {case.get("kind") for case in cases} != KINDS:
            raise ValueError(f"{source_name} must have the exact four fixture kinds")
        if len(cases) != len(KINDS):
            raise ValueError(f"{source_name} fixture kinds must be unique")
        for case in cases:
            case_id = str(case.get("case_id", ""))
            if not case_id or case_id in seen_ids:
                raise ValueError(f"fixture case_id is missing or reused: {case_id}")
            seen_ids.add(case_id)
            test_file = root / str(case.get("test_file", ""))
            if not test_file.is_file():
                raise ValueError(f"fixture test file is missing: {test_file}")
            selector = str(case.get("selector", ""))
            source_text = test_file.read_text(encoding="utf-8")
            if selector not in source_text:
                raise ValueError(f"fixture selector is missing: {case_id}:{selector}")
            for field in ("source_tuple", "mutation", "expected"):
                if not str(case.get(field, "")).strip():
                    raise ValueError(f"fixture field is missing: {case_id}:{field}")
    candidate_digest(matrix.get("candidate_files", []), root)
    boundary = matrix.get("claim_boundary", {})
    forbidden = " ".join(boundary.get("does_not_prove", []))
    for token in ("external source", "Kubernetes", "promotion"):
        if token not in forbidden:
            raise ValueError(f"claim boundary is missing: {token}")


def execute(matrix: dict[str, Any], root: Path = ROOT) -> dict[str, Any]:
    validate_matrix(matrix, root)
    started = time.monotonic()
    results: list[dict[str, Any]] = []
    for source_name, source in matrix["sources"].items():
        case_started = time.monotonic()
        completed = subprocess.run(
            source["command"],
            cwd=root / source["workdir"],
            text=True,
            stdout=subprocess.PIPE,
            stderr=subprocess.STDOUT,
            check=False,
        )
        output = completed.stdout or ""
        results.append(
            {
                "source": source_name,
                "exit_code": completed.returncode,
                "duration_ms": int((time.monotonic() - case_started) * 1000),
                "output_sha256": hashlib.sha256(output.encode()).hexdigest(),
                "output_tail": output[-4000:],
                "case_ids": [case["case_id"] for case in source["cases"]],
            }
        )
        if completed.returncode != 0:
            break
    passed = len(results) == len(SOURCES) and all(
        item["exit_code"] == 0 for item in results
    )
    return {
        "schema_version": 1,
        "task_id": "T1-M06-N013",
        "status": "PASS" if passed else "FAIL",
        "candidate_sha256": candidate_digest(matrix["candidate_files"], root),
        "environment": "ephemeral_test_processes_no_runtime_change",
        "production_applied": False,
        "duration_ms": int((time.monotonic() - started) * 1000),
        "source_results": results,
        "claim_boundary": matrix["claim_boundary"],
    }


def main() -> int:
    parser = argparse.ArgumentParser()
    parser.add_argument("--check", action="store_true")
    parser.add_argument("--output", type=Path)
    args = parser.parse_args()
    matrix = json.loads(MATRIX.read_text(encoding="utf-8"))
    if args.check:
        validate_matrix(matrix)
        result = {
            "status": "PASS",
            "task_id": "T1-M06-N013",
            "candidate_sha256": candidate_digest(matrix["candidate_files"]),
            "execution": "NOT_RUN",
        }
    else:
        result = execute(matrix)
    rendered = json.dumps(result, ensure_ascii=False, indent=2) + "\n"
    if args.output:
        args.output.write_text(rendered, encoding="utf-8")
    print(rendered, end="")
    return 0 if result["status"] == "PASS" else 1


if __name__ == "__main__":
    raise SystemExit(main())
