#!/usr/bin/env python3
"""Validate the M02 Flow WAL/partial-ACK design against current source blobs."""

from __future__ import annotations

import argparse
import copy
import hashlib
import json
import re
from pathlib import Path
from typing import Any, Callable

from build_topic1_task_registry import validate_against_schema


REPO = Path(__file__).resolve().parents[2]
DESIGN = REPO / "contracts/alignment/m02-partial-ack-function-design.v1.json"
SCHEMA = REPO / "contracts/alignment/m02-partial-ack-function-design.schema.json"
PREVIEW = REPO / "contracts/alignment/m02-code-direct-leaf-catalog.v1.json"

EXPECTED_LEAF_ROLES = {
    "M02-N001-L05": "PROTO_DISPOSITION_CONTRACT",
    "M02-N007-L03": "RUST_WAL",
    "M02-N007-L04": "RUST_BATCH_SEND",
    "M02-N007-L05": "RUST_STREAM_SEND",
    "M02-N007-L06": "RUST_RETRY",
    "M02-N008-L03": "GO_KAFKA_WRITER",
    "M02-N008-L04": "GO_INGEST_HANDLER",
}
EXPECTED_DISPOSITIONS = {
    "KAFKA_ACKED",
    "DUPLICATE_COMMITTED",
    "REJECTED_INVALID",
    "RETRYABLE",
    "OUTCOME_UNKNOWN",
}
EXPECTED_TESTS = {f"TC-M02-ACK-{index:02d}" for index in range(1, 16)}
EXPECTED_P0 = {f"M02-ACK-P0-{index:02d}" for index in range(1, 7)}


def sha256(path: Path) -> str:
    return hashlib.sha256(path.read_bytes()).hexdigest()


def normalize(value: str) -> str:
    return " ".join(value.split())


def source_contains_signature(path: Path, signature: str) -> bool:
    return normalize(signature) in normalize(path.read_text(encoding="utf-8"))


def validate(payload: dict[str, Any]) -> None:
    validate_against_schema(payload, SCHEMA)

    preview_ref = payload["preview_catalog_ref"]
    if preview_ref["path"] != PREVIEW.relative_to(REPO).as_posix():
        raise ValueError("partial ACK design references the wrong M02 preview catalog")
    if preview_ref["sha256"] != sha256(PREVIEW):
        raise ValueError("partial ACK design preview catalog hash drifted")
    preview = json.loads(PREVIEW.read_text(encoding="utf-8"))
    preview_leaves = {item["leaf_id"]: item for item in preview["leaves"]}

    bindings = payload["leaf_bindings"]
    by_leaf = {item["leaf_id"]: item for item in bindings}
    if len(by_leaf) != len(bindings) or set(by_leaf) != set(EXPECTED_LEAF_ROLES):
        raise ValueError("partial ACK leaf binding exact-set drifted")
    for leaf_id, role in EXPECTED_LEAF_ROLES.items():
        binding = by_leaf[leaf_id]
        if binding["role"] != role:
            raise ValueError(f"{leaf_id} role drifted")
        preview_leaf = preview_leaves.get(leaf_id)
        if preview_leaf is None or preview_leaf["atomic_pr_id"] != binding["atomic_pr_id"]:
            raise ValueError(f"{leaf_id} atomic PR ID differs from the preview catalog")
        if binding["locator"] not in preview_leaf["write_locators"]:
            raise ValueError(f"{leaf_id} locator differs from the preview catalog")

    candidate = payload["candidate"]
    head = (REPO / ".git").exists()
    if not re.fullmatch(r"[0-9a-f]{40}", candidate["commit"]):
        raise ValueError("candidate commit is not immutable")
    if not head:
        raise ValueError("repository git metadata is unavailable")
    for relative, expected in candidate["source_blob_sha256"].items():
        path = REPO / relative
        if not path.is_file() or sha256(path) != expected:
            raise ValueError(f"candidate source hash drifted: {relative}")

    findings = payload["current_static_findings"]
    if {item["finding_id"] for item in findings} != EXPECTED_P0:
        raise ValueError("P0 static finding exact-set drifted")
    if any(item["path"] not in candidate["source_blob_sha256"] for item in findings):
        raise ValueError("a P0 finding is not bound to a candidate source blob")

    dispositions = {item["name"]: item for item in payload["dispositions"]}
    if set(dispositions) != EXPECTED_DISPOSITIONS:
        raise ValueError("partial ACK disposition exact-set drifted")
    for name in ("KAFKA_ACKED", "DUPLICATE_COMMITTED"):
        if not dispositions[name]["terminal"] or not dispositions[name]["counts_as_accepted"]:
            raise ValueError(f"{name} must be terminal accepted")
    for name in ("RETRYABLE", "OUTCOME_UNKNOWN"):
        if dispositions[name]["terminal"] or dispositions[name]["dedup_commit_allowed"]:
            raise ValueError(f"{name} must remain nonterminal and cannot commit dedup")
    if dispositions["REJECTED_INVALID"]["dedup_commit_allowed"]:
        raise ValueError("invalid input cannot commit dedup")

    tests = payload["negative_tests"]
    if {item["case_id"] for item in tests} != EXPECTED_TESTS or len(tests) != len(EXPECTED_TESTS):
        raise ValueError("partial ACK negative-test exact-set drifted")
    if any(item["execution_status"] != "NOT_RUN" for item in tests):
        raise ValueError("design-only test cases cannot claim execution")

    contracts = payload["function_contracts"]
    contract_ids = [item["contract_id"] for item in contracts]
    if len(contract_ids) != len(set(contract_ids)):
        raise ValueError("duplicate partial ACK function contract ID")
    for contract in contracts:
        if contract["leaf_id"] not in by_leaf:
            raise ValueError(f"function contract is not bound to a preview leaf: {contract['contract_id']}")
        if not set(contract["tests"]).issubset(EXPECTED_TESTS):
            raise ValueError(f"function contract references an unknown test: {contract['contract_id']}")
        before = contract["signature_before"]
        if contract["change_kind"] == "modify":
            if before is None or not source_contains_signature(REPO / contract["path"], before):
                raise ValueError(f"before signature is absent from candidate source: {contract['contract_id']}")
        elif before is not None:
            raise ValueError(f"planned companion carries a before signature: {contract['contract_id']}")
        step_ids = [step.split(" ", 1)[0] for step in contract["body_steps"]]
        if step_ids != [f"B{index:02d}" for index in range(1, len(step_ids) + 1)]:
            raise ValueError(f"body steps are not contiguous: {contract['contract_id']}")

    sequence = payload["sequencing"]
    if [item["order"] for item in sequence] != list(range(1, len(sequence) + 1)):
        raise ValueError("partial ACK sequence order is not contiguous")
    if set(item["leaf_id"] for item in sequence) != set(EXPECTED_LEAF_ROLES):
        raise ValueError("partial ACK sequence does not cover the exact leaf set")

    states = set(payload["wal_state_machine"]["states"])
    for transition in payload["wal_state_machine"]["transitions"]:
        if transition["from"] not in states or transition["to"] not in states:
            raise ValueError("WAL transition references an unknown state")

    forbidden = set(payload["claims"]["forbidden"])
    if not {"IMPLEMENTED", "TESTED", "EXECUTION_AUTHORIZED", "M02_ACCEPTED"}.issubset(forbidden):
        raise ValueError("partial ACK claim ceiling is incomplete")


def expect_reject(name: str, mutate: Callable[[dict[str, Any]], None], source: dict[str, Any]) -> None:
    candidate = copy.deepcopy(source)
    mutate(candidate)
    try:
        validate(candidate)
    except (ValueError, KeyError):
        return
    raise ValueError(f"malicious partial ACK design mutation was accepted: {name}")


def self_test(payload: dict[str, Any]) -> None:
    expect_reject("preview hash drift", lambda value: value["preview_catalog_ref"].update(sha256="0" * 64), payload)
    expect_reject("leaf identity drift", lambda value: value["leaf_bindings"][0].update(atomic_pr_id="T1-M02-P199-CTR-n001-l05"), payload)
    expect_reject("candidate source drift", lambda value: value["candidate"]["source_blob_sha256"].update({"proto/traffic/v1/ingest.proto": "0" * 64}), payload)
    expect_reject("missing disposition", lambda value: value["dispositions"].pop(), payload)
    expect_reject("false test PASS", lambda value: value["negative_tests"][0].update(execution_status="PASS"), payload)
    expect_reject("fabricated before signature", lambda value: value["function_contracts"][0].update(signature_before="pub fn save(&self) -> Result<()>"), payload)
    expect_reject("unknown transition state", lambda value: value["wal_state_machine"]["transitions"][0].update(to="LOST"), payload)
    expect_reject("dedup on unknown", lambda value: next(item for item in value["dispositions"] if item["name"] == "OUTCOME_UNKNOWN").update(dedup_commit_allowed=True), payload)


def main() -> int:
    parser = argparse.ArgumentParser()
    parser.add_argument("--self-test", action="store_true")
    args = parser.parse_args()
    payload = json.loads(DESIGN.read_text(encoding="utf-8"))
    validate(payload)
    if args.self_test:
        self_test(payload)
    print("PASS M02 partial ACK function design: 7 frozen leaves, 8 function contracts, 6 candidate-bound P0 findings, 15 NOT_RUN tests, execution remains BLOCKED")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
