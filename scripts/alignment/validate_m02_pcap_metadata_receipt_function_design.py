#!/usr/bin/env python3
"""Validate the candidate-bound M02 PCAP metadata Kafka receipt design."""

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
DESIGN = REPO / "contracts/alignment/m02-pcap-metadata-receipt-function-design.v1.json"
SCHEMA = REPO / "contracts/alignment/m02-pcap-metadata-receipt-function-design.schema.json"
PREVIEW = REPO / "contracts/alignment/m02-code-direct-leaf-catalog.v1.json"

EXPECTED_LEAF_ROLES = {
    "M02-N001-L08": "PCAP_META", "M02-N001-L09": "REQUEST_CONTRACT", "M02-N001-L10": "RESPONSE_CONTRACT",
    "M02-N003-L02": "TOPIC_CONTRACT", "M02-N003-L08": "TOPIC_MATERIALIZATION",
    "M02-N008-L06": "GO_KAFKA_WRITER", "M02-N008-L07": "GO_HANDLER", "M02-N011-L07": "RUST_METADATA_CLIENT",
}
EXPECTED_FUNCTION_LEAVES = {"M02-N008-L06", "M02-N008-L07", "M02-N011-L07"}
EXPECTED_P0 = {f"M02-PCAP-RECEIPT-P0-{index:02d}" for index in range(1, 10)}
EXPECTED_TESTS = {f"TC-M02-PCAP-RECEIPT-{index:02d}" for index in range(1, 16)}


def sha256(path: Path) -> str:
    return hashlib.sha256(path.read_bytes()).hexdigest()


def normalize(value: str) -> str:
    return " ".join(value.split())


def source_contains_signature(path: Path, signature: str) -> bool:
    return normalize(signature) in normalize(path.read_text(encoding="utf-8"))


def validate(payload: dict[str, Any]) -> None:
    validate_against_schema(payload, SCHEMA)
    preview_ref = payload["preview_catalog_ref"]
    if preview_ref["path"] != PREVIEW.relative_to(REPO).as_posix() or preview_ref["sha256"] != sha256(PREVIEW):
        raise ValueError("PCAP receipt preview catalog ref drifted")
    preview = json.loads(PREVIEW.read_text(encoding="utf-8"))
    preview_leaves = {item["leaf_id"]: item for item in preview["leaves"]}

    bindings = payload["leaf_bindings"]
    by_leaf = {item["leaf_id"]: item for item in bindings}
    if len(by_leaf) != len(bindings) or set(by_leaf) != set(EXPECTED_LEAF_ROLES):
        raise ValueError("PCAP receipt leaf binding exact-set drifted")
    for leaf_id, role in EXPECTED_LEAF_ROLES.items():
        binding = by_leaf[leaf_id]
        leaf = preview_leaves.get(leaf_id)
        if binding["role"] != role or leaf is None:
            raise ValueError(f"PCAP receipt binding role or leaf drifted: {leaf_id}")
        if leaf["atomic_pr_id"] != binding["atomic_pr_id"] or binding["locator"] not in leaf["write_locators"]:
            raise ValueError(f"PCAP receipt ID or locator differs from preview: {leaf_id}")

    candidate = payload["candidate"]
    if not re.fullmatch(r"[0-9a-f]{40}", candidate["commit"]):
        raise ValueError("PCAP receipt candidate commit is not immutable")
    for relative, expected in candidate["source_blob_sha256"].items():
        path = REPO / relative
        if not path.is_file() or sha256(path) != expected:
            raise ValueError(f"PCAP receipt source hash drifted: {relative}")

    findings = payload["current_static_findings"]
    if {item["finding_id"] for item in findings} != EXPECTED_P0 or len(findings) != len(EXPECTED_P0):
        raise ValueError("PCAP receipt P0 exact-set drifted")
    if any(item["path"] not in candidate["source_blob_sha256"] for item in findings):
        raise ValueError("a PCAP receipt finding lacks candidate source binding")

    gap = payload["missing_atomic_leaf"]
    if gap["gap_id"] != "M02-PCAP-RECEIPT-GAP-01" or gap["status"] != "BLOCKED_MISSING_TRANSACTIONAL_RECEIPT_OUTBOX_ATOMIC_LEAF_AND_SUPPORT_TRAIN":
        raise ValueError("PCAP receipt transactional outbox gap drifted")
    if len(gap["required_locators"]) < 3:
        raise ValueError("PCAP receipt outbox gap lacks required locators")

    tests = payload["negative_tests"]
    if {item["case_id"] for item in tests} != EXPECTED_TESTS or len(tests) != len(EXPECTED_TESTS):
        raise ValueError("PCAP receipt test exact-set drifted")
    if any(item["execution_status"] != "NOT_RUN" for item in tests):
        raise ValueError("PCAP receipt design tests cannot claim execution")

    contracts = payload["function_contracts"]
    if {item["leaf_id"] for item in contracts} != EXPECTED_FUNCTION_LEAVES:
        raise ValueError("PCAP receipt function leaf set drifted")
    if len({item["contract_id"] for item in contracts}) != len(contracts):
        raise ValueError("duplicate PCAP receipt contract ID")
    for contract in contracts:
        if not set(contract["tests"]).issubset(EXPECTED_TESTS):
            raise ValueError(f"unknown PCAP receipt test in {contract['contract_id']}")
        before = contract["signature_before"]
        if contract["change_kind"] == "modify":
            if before is None or not source_contains_signature(REPO / contract["path"], before):
                raise ValueError(f"before signature absent from receipt candidate: {contract['contract_id']}")
        elif before is not None:
            raise ValueError(f"planned receipt companion carries a before signature: {contract['contract_id']}")
        step_ids = [step.split(" ", 1)[0] for step in contract["body_steps"]]
        if step_ids != [f"B{index:02d}" for index in range(1, len(step_ids) + 1)]:
            raise ValueError(f"receipt body steps are not contiguous: {contract['contract_id']}")

    sequence = payload["sequencing"]
    if [item["order"] for item in sequence] != list(range(1, len(sequence) + 1)) or {item["leaf_id"] for item in sequence} != set(EXPECTED_LEAF_ROLES):
        raise ValueError("PCAP receipt sequencing exact-set drifted")

    states = set(payload["receipt_state_machine"]["states"])
    for transition in payload["receipt_state_machine"]["transitions"]:
        if transition["from"] not in states or transition["to"] not in states:
            raise ValueError("PCAP receipt transition references unknown state")
    if not {"INTENT_DURABLE", "PUBLISH_UNKNOWN", "KAFKA_ACKED", "RECEIPT_DURABLE", "REPLAY_RECEIVED", "BLOCKED"}.issubset(states):
        raise ValueError("PCAP receipt state machine omits required states")

    forbidden = set(payload["claims"]["forbidden"])
    if not {"TRANSACTIONAL_OUTBOX_IMPLEMENTED", "IMPLEMENTED", "TESTED", "KAFKA_DURABILITY_PROVEN", "CLICKHOUSE_INDEXED", "EXECUTION_AUTHORIZED"}.issubset(forbidden):
        raise ValueError("PCAP receipt claim ceiling is incomplete")


def expect_reject(name: str, mutate: Callable[[dict[str, Any]], None], source: dict[str, Any]) -> None:
    candidate = copy.deepcopy(source)
    mutate(candidate)
    try:
        validate(candidate)
    except (ValueError, KeyError):
        return
    raise ValueError(f"malicious PCAP receipt design mutation was accepted: {name}")


def self_test(payload: dict[str, Any]) -> None:
    expect_reject("preview drift", lambda value: value["preview_catalog_ref"].update(sha256="0" * 64), payload)
    expect_reject("leaf drift", lambda value: value["leaf_bindings"][0].update(atomic_pr_id="T1-M02-P110-CTR-n001-l08"), payload)
    expect_reject("source drift", lambda value: value["candidate"]["source_blob_sha256"].update({"proto/traffic/v1/ingest.proto": "0" * 64}), payload)
    expect_reject("missing outbox gap", lambda value: value.pop("missing_atomic_leaf"), payload)
    expect_reject("false test PASS", lambda value: value["negative_tests"][0].update(execution_status="PASS"), payload)
    expect_reject("fabricated before", lambda value: next(item for item in value["function_contracts"] if item["change_kind"] == "modify").update(signature_before="func missing()"), payload)
    expect_reject("unknown state", lambda value: value["receipt_state_machine"]["transitions"][0].update(to="DROPPED"), payload)
    expect_reject("false durability claim", lambda value: value["claims"]["forbidden"].remove("KAFKA_DURABILITY_PROVEN"), payload)


def main() -> int:
    parser = argparse.ArgumentParser()
    parser.add_argument("--self-test", action="store_true")
    args = parser.parse_args()
    payload = json.loads(DESIGN.read_text(encoding="utf-8"))
    validate(payload)
    if args.self_test:
        self_test(payload)
    print("PASS M02 PCAP metadata receipt design: 8 frozen leaves, 6 function contracts, 9 candidate-bound P0 findings, transactional outbox gap BLOCKED, 15 NOT_RUN tests")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
