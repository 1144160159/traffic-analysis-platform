#!/usr/bin/env python3
"""Run an exact Go test set and fail closed on zero-match or SKIP.

`go test -run` exits zero when the regular expression matches no test.  That is
useful interactively but is not a valid evidence oracle.  This runner consumes
`go test -json`, requires one `run` and one terminal `pass` event for every
declared top-level test, rejects every missing/duplicate/skip/fail event, and
writes an immutable machine-readable result.
"""

from __future__ import annotations

import argparse
import hashlib
import json
import os
import re
import subprocess
from pathlib import Path
from typing import Any, Iterable


REPO_ROOT = Path(__file__).resolve().parents[2]
GO_TEST_NAME = re.compile(r"^Test[A-Za-z0-9_]+$")


def decode_go_test_json(output: str) -> list[dict[str, Any]]:
    events: list[dict[str, Any]] = []
    for line_number, line in enumerate(output.splitlines(), start=1):
        if not line.strip():
            continue
        try:
            event = json.loads(line)
        except json.JSONDecodeError as exc:
            raise ValueError(f"go test emitted non-JSON line {line_number}: {line[:160]!r}") from exc
        if not isinstance(event, dict):
            raise ValueError(f"go test event {line_number} is not an object")
        events.append(event)
    return events


def assert_exact_go_test_events(
    events: Iterable[dict[str, Any]], expected_tests: Iterable[str]
) -> dict[str, dict[str, int]]:
    expected = tuple(expected_tests)
    if not expected or len(expected) != len(set(expected)):
        raise ValueError("expected Go test names must be a non-empty exact set")
    counts = {
        name: {"run": 0, "pass": 0, "fail": 0, "skip": 0}
        for name in expected
    }
    unexpected_terminal: list[str] = []
    for event in events:
        test = event.get("Test")
        action = event.get("Action")
        if not isinstance(test, str):
            continue
        # A parent test may still report PASS when a required assertion lives
        # in a skipped subtest.  Every fail/skip below an expected top-level
        # test is therefore terminal evidence, not noise to discard.
        expected_parent = next(
            (name for name in expected if test == name or test.startswith(name + "/")),
            None,
        )
        if "/" in test:
            if expected_parent is not None and action in {"fail", "skip"}:
                unexpected_terminal.append(f"{test}:{action}")
            continue
        if test in counts and action in counts[test]:
            counts[test][action] += 1
        elif action in {"fail", "skip"}:
            unexpected_terminal.append(f"{test}:{action}")
    failures = []
    for test, action_counts in counts.items():
        if action_counts != {"run": 1, "pass": 1, "fail": 0, "skip": 0}:
            failures.append(f"{test}={action_counts}")
    if failures or unexpected_terminal:
        raise ValueError(
            "exact Go test closure failed: "
            + "; ".join([*failures, *(f"unexpected {item}" for item in unexpected_terminal)])
        )
    return counts


def run_exact_go_tests(
    *, go_root: Path, package: str, test_names: list[str], env: dict[str, str] | None = None
) -> tuple[subprocess.CompletedProcess[str], list[dict[str, Any]], dict[str, dict[str, int]]]:
    if any(not GO_TEST_NAME.fullmatch(name) for name in test_names):
        raise ValueError(f"invalid top-level Go test name in {test_names!r}")
    escaped = "|".join(re.escape(name) for name in test_names)
    command = [
        "go", "test", "-json", package, "-run", f"^({escaped})$", "-count=1",
    ]
    completed = subprocess.run(
        command,
        cwd=go_root,
        env=env,
        text=True,
        stdout=subprocess.PIPE,
        stderr=subprocess.STDOUT,
        check=False,
    )
    events = decode_go_test_json(completed.stdout)
    if completed.returncode != 0:
        tail = completed.stdout[-8192:].strip()
        raise RuntimeError(f"go test exited {completed.returncode}: {tail}")
    counts = assert_exact_go_test_events(events, test_names)
    return completed, events, counts


def self_test() -> None:
    good = [
        {"Action": "run", "Test": "TestExact"},
        {"Action": "pass", "Test": "TestExact"},
    ]
    assert_exact_go_test_events(good, ["TestExact"])
    bad_cases = {
        "zero-match": [],
        "skip": [{"Action": "run", "Test": "TestExact"}, {"Action": "skip", "Test": "TestExact"}],
        "fail": [{"Action": "run", "Test": "TestExact"}, {"Action": "fail", "Test": "TestExact"}],
        "duplicate": [*good, *good],
        "subtest-skip": [
            {"Action": "run", "Test": "TestExact"},
            {"Action": "run", "Test": "TestExact/critical"},
            {"Action": "skip", "Test": "TestExact/critical"},
            {"Action": "pass", "Test": "TestExact"},
        ],
        "subtest-fail": [
            {"Action": "run", "Test": "TestExact"},
            {"Action": "run", "Test": "TestExact/critical"},
            {"Action": "fail", "Test": "TestExact/critical"},
            {"Action": "pass", "Test": "TestExact"},
        ],
    }
    for case, events in bad_cases.items():
        try:
            assert_exact_go_test_events(events, ["TestExact"])
        except ValueError:
            continue
        raise AssertionError(f"exact Go test self-test accepted {case}")


def main() -> int:
    parser = argparse.ArgumentParser()
    parser.add_argument("--go-root", type=Path, default=Path("go/control-plane"))
    parser.add_argument("--package")
    parser.add_argument("--test", action="append", dest="tests", default=[])
    parser.add_argument("--output", type=Path)
    parser.add_argument("--candidate-manifest", type=Path)
    parser.add_argument("--profile-id")
    parser.add_argument("--environment-id")
    parser.add_argument("--run-id")
    parser.add_argument("--subject-pr-id")
    parser.add_argument("--source", action="append", dest="sources", default=[])
    parser.add_argument("--self-test", action="store_true")
    args = parser.parse_args()
    if args.self_test:
        self_test()
        print("PASS exact Go test runner negatives: zero-match, skip, fail, duplicate, subtest-skip, subtest-fail")
        if not args.package:
            return 0
    if (
        not args.package or not args.tests or args.output is None
        or args.candidate_manifest is None or not args.profile_id
        or not args.environment_id or not args.run_id or not args.subject_pr_id
        or not args.sources
    ):
        parser.error(
            "--package, --test, --output, --candidate-manifest, --profile-id, "
            "--environment-id, --run-id, --subject-pr-id and at least one --source are required for a run"
        )

    go_root = (REPO_ROOT / args.go_root).resolve()
    if not go_root.is_relative_to(REPO_ROOT) or not (go_root / "go.mod").is_file():
        raise SystemExit(f"unsafe or invalid Go root: {args.go_root}")
    output = (REPO_ROOT / args.output).resolve()
    if not output.is_relative_to(REPO_ROOT):
        raise SystemExit(f"output escapes repository: {args.output}")
    if output.exists():
        raise SystemExit(f"refusing to overwrite exact Go test evidence: {args.output}")
    candidate_manifest = (REPO_ROOT / args.candidate_manifest).resolve()
    if not candidate_manifest.is_relative_to(REPO_ROOT) or not candidate_manifest.is_file():
        raise SystemExit(f"unsafe or missing candidate manifest: {args.candidate_manifest}")
    source_hashes: dict[str, str] = {}
    candidate_payload = json.loads(candidate_manifest.read_text(encoding="utf-8"))
    declared_sources = candidate_payload.get("source_blob_sha256")
    if not isinstance(declared_sources, dict):
        raise SystemExit("candidate manifest lacks source_blob_sha256")
    for source in args.sources:
        source_path = (REPO_ROOT / source).resolve()
        if not source_path.is_relative_to(REPO_ROOT) or not source_path.is_file():
            raise SystemExit(f"unsafe or missing test source: {source}")
        relative = source_path.relative_to(REPO_ROOT).as_posix()
        source_hashes[relative] = hashlib.sha256(
            source_path.read_bytes()
        ).hexdigest()
        if declared_sources.get(relative) != source_hashes[relative]:
            raise SystemExit(f"candidate manifest does not bind current source: {relative}")

    payload: dict[str, Any] = {
        "schema_version": 1,
        "artifact_kind": "EXACT_GO_TEST_RESULT",
        "subject_pr_id": args.subject_pr_id,
        "run_id": args.run_id,
        "status": "FAIL",
        "go_root": go_root.relative_to(REPO_ROOT).as_posix(),
        "package": args.package,
        "expected_tests": args.tests,
        "candidate_manifest": {
            "path": candidate_manifest.relative_to(REPO_ROOT).as_posix(),
            "sha256": hashlib.sha256(candidate_manifest.read_bytes()).hexdigest(),
        },
        "profile_id": args.profile_id,
        "environment_id": args.environment_id,
        "source_blob_sha256": source_hashes,
        "runner_sha256": hashlib.sha256(Path(__file__).read_bytes()).hexdigest(),
        "event_counts": {},
        "go_version": None,
        "go_test_exit_code": None,
        "errors": [],
        "proof_ceiling": "EXACT_GO_TEST_ONLY_NOT_RUNTIME_OR_PRODUCTION_ACCEPTANCE",
    }
    exit_code = 1
    try:
        version = subprocess.run(
            ["go", "version"], cwd=go_root, text=True,
            stdout=subprocess.PIPE, stderr=subprocess.STDOUT, check=True,
        ).stdout.strip()
        payload["go_version"] = version
        completed, _, counts = run_exact_go_tests(
            go_root=go_root, package=args.package, test_names=args.tests,
            env=os.environ.copy(),
        )
        payload["go_test_exit_code"] = completed.returncode
        payload["event_counts"] = counts
        payload["status"] = "PASS"
        exit_code = 0
    except Exception as exc:  # Preserve a bounded immutable failure fact.
        payload["errors"] = [str(exc)]

    output.parent.mkdir(parents=True, exist_ok=True)
    output.write_text(json.dumps(payload, ensure_ascii=False, indent=2) + "\n", encoding="utf-8")
    print(json.dumps(payload, ensure_ascii=False, indent=2))
    return exit_code


if __name__ == "__main__":
    raise SystemExit(main())
