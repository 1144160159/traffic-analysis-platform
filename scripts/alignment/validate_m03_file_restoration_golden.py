#!/usr/bin/env python3
"""Validate the M03 file-restoration golden corpus and production runner seam."""

from __future__ import annotations

import copy
import hashlib
import json
import re
import sys
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parent))
from build_topic1_task_registry import validate_against_schema


ROOT = Path(__file__).resolve().parents[2]
CORPUS = ROOT / "tests/fixtures/forensics/file-restoration/golden-corpus.v1.json"
SCHEMA = ROOT / "tests/fixtures/forensics/file-restoration/golden-corpus.v1.schema.json"
CONTRACT = ROOT / "contracts/forensics/file-restoration.v1.json"
GO_RUNNER = ROOT / "go/control-plane/internal/forensics/restoration/golden_corpus_test.go"
EXPECTED_CASES = {
    "tcp-out-of-order-retransmission",
    "tcp-gap-no-invention",
    "tcp-unequal-overlap-corrupt",
    "tcp-capture-truncated",
    "tcp-stream-oversize",
    "http-content-length-path-traversal",
    "http-chunked-content-encoding-inert",
    "http-conflicting-framing-corrupt",
    "http-websocket-unsupported",
    "http-declared-oversize",
    "http-archive-remains-inert",
    "ftp-passive-retr-complete",
    "ftp-ambiguous-data-unsupported",
    "ftp-tls-unsupported",
    "smtp-base64-attachment-complete",
    "smtp-unknown-encoding-unsupported",
    "smtp-missing-dot-truncated",
    "smtp-multipart-ambiguous-unsupported",
}
REQUIRED_CATEGORIES = {
    "out_of_order",
    "identical_retransmission",
    "packet_loss",
    "no_invented_bytes",
    "unequal_retransmission",
    "capture_truncation",
    "stream_limit",
    "path_traversal",
    "encoded_no_auto_decompress",
    "contradictory_framing",
    "archive",
    "no_expansion",
    "no_execution",
    "ambiguous_data_connection",
    "encrypted",
    "bad_encoding",
    "missing_terminator",
    "ambiguous_multiple_files",
}
STATUSES = {"complete", "partial", "truncated", "corrupt", "oversize", "unsupported"}
PROFILES = {"http1-response-body-v1", "ftp-passive-retr-v1", "smtp-data-mime-v1"}
OBJECT_POLICIES = {"not_applicable", "required", "metadata_only", "forbidden", "optional_quarantine"}


def require(condition: bool, code: str) -> None:
    if not condition:
        raise ValueError(code)


def sha256(path: Path) -> str:
    return hashlib.sha256(path.read_bytes()).hexdigest()


def load(path: Path) -> dict:
    value = json.loads(path.read_text(encoding="utf-8"))
    require(isinstance(value, dict), "FILE_RESTORE_GOLDEN_ROOT_NOT_OBJECT")
    return value


def validate(corpus: dict) -> None:
    require(SCHEMA.is_file() and CONTRACT.is_file() and GO_RUNNER.is_file(), "FILE_RESTORE_GOLDEN_REQUIRED_FILE_MISSING")
    validate_against_schema(corpus, SCHEMA)
    require(set(corpus) == {"schema_version", "corpus_id", "contract_id", "claim_boundary", "limits", "cases"}, "FILE_RESTORE_GOLDEN_ROOT_KEYS")
    require(corpus["schema_version"] == "1.0.0" and corpus["corpus_id"] == "m03-file-restoration-golden-v1", "FILE_RESTORE_GOLDEN_IDENTITY")
    require(corpus["contract_id"] == "traffic.file-restoration.v1", "FILE_RESTORE_GOLDEN_CONTRACT_ID")
    boundary = corpus["claim_boundary"]
    require("exact approved profiles" in boundary["allowed"], "FILE_RESTORE_GOLDEN_ALLOWED_BOUNDARY")
    require(set(boundary["forbidden"]) == {
        "all application protocols are supported",
        "restored content is safe",
        "restored content may be executed or automatically opened",
        "runtime activation is approved",
    }, "FILE_RESTORE_GOLDEN_FORBIDDEN_BOUNDARY")
    limits = corpus["limits"]
    require(set(limits) == {"max_stream_bytes", "max_object_bytes", "max_part_count", "max_mime_depth", "max_expansion_ratio"}, "FILE_RESTORE_GOLDEN_LIMIT_KEYS")
    require(all(isinstance(value, (int, float)) and value > 0 for value in limits.values()), "FILE_RESTORE_GOLDEN_LIMIT_VALUES")
    require(limits["max_object_bytes"] <= limits["max_stream_bytes"], "FILE_RESTORE_GOLDEN_LIMIT_ORDER")

    cases = corpus["cases"]
    require(isinstance(cases, list) and len(cases) == 18, "FILE_RESTORE_GOLDEN_CASE_COUNT")
    case_ids = [row.get("case_id") for row in cases]
    require(set(case_ids) == EXPECTED_CASES and len(case_ids) == len(set(case_ids)), "FILE_RESTORE_GOLDEN_CASE_SET")
    categories: set[str] = set()
    statuses: set[str] = set()
    profiles: set[str] = set()
    for row in cases:
        require(set(row) == {"case_id", "stage", "categories", "input", "expected"}, "FILE_RESTORE_GOLDEN_CASE_KEYS")
        require(re.fullmatch(r"[a-z0-9-]+", row["case_id"]) is not None, "FILE_RESTORE_GOLDEN_CASE_ID")
        require(row["stage"] in {"reassembly", "extractor"}, "FILE_RESTORE_GOLDEN_STAGE")
        require(len(row["categories"]) == len(set(row["categories"])) and row["categories"], "FILE_RESTORE_GOLDEN_CATEGORY_SHAPE")
        categories.update(row["categories"])
        expected = row["expected"]
        status = expected.get("status")
        statuses.add(status)
        require(status in STATUSES, "FILE_RESTORE_GOLDEN_STATUS")
        object_policy = expected.get("object_policy")
        require(object_policy in OBJECT_POLICIES, "FILE_RESTORE_GOLDEN_OBJECT_POLICY")
        if row["stage"] == "extractor":
            profile = row["input"].get("profile_id")
            profiles.add(profile)
            require(expected.get("inert") is True, "FILE_RESTORE_GOLDEN_INERT_REQUIRED")
            require(object_policy != "not_applicable", "FILE_RESTORE_GOLDEN_EXTRACTOR_OBJECT_POLICY")
            if status == "unsupported":
                require(object_policy == "forbidden" and expected.get("content") == "", "FILE_RESTORE_GOLDEN_UNSUPPORTED_OBJECT")
        else:
            require(object_policy == "not_applicable", "FILE_RESTORE_GOLDEN_REASSEMBLY_OBJECT_POLICY")
    require(REQUIRED_CATEGORIES <= categories, "FILE_RESTORE_GOLDEN_MALICIOUS_COVERAGE")
    require(statuses == STATUSES, "FILE_RESTORE_GOLDEN_STATUS_COVERAGE")
    require(profiles == PROFILES, "FILE_RESTORE_GOLDEN_PROFILE_COVERAGE")

    runner = GO_RUNNER.read_text(encoding="utf-8")
    require("golden-corpus.v1.json" in runner and "reassembly.Reassemble" in runner and "extractor.Extract" in runner, "FILE_RESTORE_GOLDEN_PRODUCTION_RUNNER")
    require("result.Quarantined" in runner and "!result.Executable" in runner and "!result.AutomaticOpen" in runner and "!result.AutomaticDecompress" in runner, "FILE_RESTORE_GOLDEN_INERT_RUNNER")


def expect_failure(corpus: dict, code: str) -> None:
    try:
        validate(corpus)
    except ValueError as exc:
        require(str(exc) == code, f"FILE_RESTORE_GOLDEN_MUTATION_WRONG_ERROR expected={code} actual={exc}")
        return
    raise ValueError(f"FILE_RESTORE_GOLDEN_MUTATION_NOT_REJECTED expected={code}")


def self_test(corpus: dict) -> None:
    mutated = copy.deepcopy(corpus)
    mutated["cases"][0]["case_id"] = "replacement-case"
    expect_failure(mutated, "FILE_RESTORE_GOLDEN_CASE_SET")

    mutated = copy.deepcopy(corpus)
    unsupported = next(row for row in mutated["cases"] if row["expected"]["status"] == "unsupported")
    unsupported["expected"]["object_policy"] = "required"
    expect_failure(mutated, "FILE_RESTORE_GOLDEN_UNSUPPORTED_OBJECT")

    mutated = copy.deepcopy(corpus)
    extractor_case = next(row for row in mutated["cases"] if row["stage"] == "extractor")
    extractor_case["expected"]["inert"] = False
    expect_failure(mutated, "FILE_RESTORE_GOLDEN_INERT_REQUIRED")

    mutated = copy.deepcopy(corpus)
    for row in mutated["cases"]:
        row["categories"] = [category for category in row["categories"] if category != "no_execution"]
    expect_failure(mutated, "FILE_RESTORE_GOLDEN_MALICIOUS_COVERAGE")


def main() -> int:
    corpus = load(CORPUS)
    validate(corpus)
    self_test(corpus)
    print(json.dumps({
        "result": "pass",
        "corpus_sha256": sha256(CORPUS),
        "cases": len(corpus["cases"]),
        "profiles": 3,
        "statuses": 6,
        "mutations": 4,
        "runtime_activation": "NOT_APPROVED",
    }, sort_keys=True))
    return 0


if __name__ == "__main__":
    try:
        raise SystemExit(main())
    except (OSError, json.JSONDecodeError, KeyError, TypeError, ValueError) as exc:
        print(str(exc), file=sys.stderr)
        raise SystemExit(1)
