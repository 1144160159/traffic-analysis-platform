#!/usr/bin/env python3
"""Validate the candidate-bound M02 PCAP raw-carrier consumer design."""

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
DESIGN = REPO / "contracts/alignment/m02-pcap-consumer-function-design.v1.json"
SCHEMA = REPO / "contracts/alignment/m02-pcap-consumer-function-design.schema.json"
PREVIEW = REPO / "contracts/alignment/m02-code-direct-leaf-catalog.v1.json"

EXPECTED_LEAF_ROLES = {
    "M02-N009-L04": "RAW_RECORD",
    "M02-N009-L05": "RAW_DESERIALIZER",
    "M02-N009-L06": "INDEXED_CARRIER",
    "M02-N009-L07": "PARSE_FUNCTION",
    "M02-N009-L08": "CANONICAL_DLQ",
    "M02-N009-L09": "CONSUMER_PIPELINE",
    "M02-N009-L11": "TEST_CONTRACT",
}
EXPECTED_FUNCTION_LEAVES = {f"M02-N009-L{index:02d}" for index in range(4, 10)}
EXPECTED_P0 = {f"M02-PCAP-CONSUMER-P0-{index:02d}" for index in range(1, 9)}
EXPECTED_TESTS = {f"TC-M02-PCAP-CONSUMER-{index:02d}" for index in range(1, 15)}


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
        raise ValueError("PCAP consumer design preview catalog ref drifted")
    preview = json.loads(PREVIEW.read_text(encoding="utf-8"))
    preview_leaves = {item["leaf_id"]: item for item in preview["leaves"]}

    bindings = payload["leaf_bindings"]
    by_leaf = {item["leaf_id"]: item for item in bindings}
    if len(by_leaf) != len(bindings) or set(by_leaf) != set(EXPECTED_LEAF_ROLES):
        raise ValueError("PCAP consumer leaf binding exact-set drifted")
    for leaf_id, role in EXPECTED_LEAF_ROLES.items():
        binding = by_leaf[leaf_id]
        preview_leaf = preview_leaves.get(leaf_id)
        if binding["role"] != role:
            raise ValueError(f"{leaf_id} role drifted")
        if preview_leaf is None or preview_leaf["atomic_pr_id"] != binding["atomic_pr_id"]:
            raise ValueError(f"{leaf_id} atomic PR ID differs from preview")
        if binding["locator"] not in preview_leaf["write_locators"]:
            raise ValueError(f"{leaf_id} locator differs from preview")

    candidate = payload["candidate"]
    if not re.fullmatch(r"[0-9a-f]{40}", candidate["commit"]):
        raise ValueError("PCAP consumer candidate commit is not immutable")
    for relative, expected in candidate["source_blob_sha256"].items():
        path = REPO / relative
        if not path.is_file() or sha256(path) != expected:
            raise ValueError(f"PCAP consumer candidate source hash drifted: {relative}")

    findings = payload["current_static_findings"]
    if {item["finding_id"] for item in findings} != EXPECTED_P0 or len(findings) != len(EXPECTED_P0):
        raise ValueError("PCAP consumer P0 exact-set drifted")
    if any(item["path"] not in candidate["source_blob_sha256"] for item in findings):
        raise ValueError("a PCAP consumer finding lacks a candidate-bound source blob")

    tests = payload["negative_tests"]
    if {item["case_id"] for item in tests} != EXPECTED_TESTS or len(tests) != len(EXPECTED_TESTS):
        raise ValueError("PCAP consumer negative-test exact-set drifted")
    if any(item["execution_status"] != "NOT_RUN" for item in tests):
        raise ValueError("design-only PCAP consumer tests cannot claim execution")

    contracts = payload["function_contracts"]
    if {item["leaf_id"] for item in contracts} != EXPECTED_FUNCTION_LEAVES or len(contracts) != len(EXPECTED_FUNCTION_LEAVES):
        raise ValueError("PCAP consumer function contract leaf exact-set drifted")
    contract_ids = [item["contract_id"] for item in contracts]
    if len(contract_ids) != len(set(contract_ids)):
        raise ValueError("duplicate PCAP consumer function contract ID")
    for contract in contracts:
        if not set(contract["tests"]).issubset(EXPECTED_TESTS):
            raise ValueError(f"unknown test in {contract['contract_id']}")
        before = contract["signature_before"]
        if contract["change_kind"] == "modify":
            if before is None or not source_contains_signature(REPO / contract["path"], before):
                raise ValueError(f"before signature absent from candidate source: {contract['contract_id']}")
        elif before is not None:
            raise ValueError(f"planned PCAP consumer companion has a before signature: {contract['contract_id']}")
        step_ids = [step.split(" ", 1)[0] for step in contract["body_steps"]]
        if step_ids != [f"B{index:02d}" for index in range(1, len(step_ids) + 1)]:
            raise ValueError(f"body steps are not contiguous: {contract['contract_id']}")

    sequence = payload["sequencing"]
    if [item["order"] for item in sequence] != list(range(1, len(sequence) + 1)):
        raise ValueError("PCAP consumer sequence order is not contiguous")
    if {item["leaf_id"] for item in sequence} != set(EXPECTED_LEAF_ROLES):
        raise ValueError("PCAP consumer sequence does not cover the bound leaf exact-set")

    states = set(payload["record_state_machine"]["states"])
    for transition in payload["record_state_machine"]["transitions"]:
        if transition["from"] not in states or transition["to"] not in states:
            raise ValueError("PCAP consumer transition references unknown state")
    if not {"RAW_CAPTURED", "MAIN_EMITTED", "DLQ_ACKED", "CHECKPOINT_ELIGIBLE", "BLOCKED"}.issubset(states):
        raise ValueError("PCAP consumer state machine omits a required state")

    forbidden = set(payload["claims"]["forbidden"])
    if not {"LIVE_SOURCE_ACTIVE", "CHECKPOINT_SAFE", "CLICKHOUSE_INDEXED", "IMPLEMENTED", "TESTED", "EXECUTION_AUTHORIZED"}.issubset(forbidden):
        raise ValueError("PCAP consumer claim ceiling is incomplete")


def expect_reject(name: str, mutate: Callable[[dict[str, Any]], None], source: dict[str, Any]) -> None:
    candidate = copy.deepcopy(source)
    mutate(candidate)
    try:
        validate(candidate)
    except (ValueError, KeyError):
        return
    raise ValueError(f"malicious PCAP consumer design mutation was accepted: {name}")


def self_test(payload: dict[str, Any]) -> None:
    expect_reject("preview hash drift", lambda value: value["preview_catalog_ref"].update(sha256="0" * 64), payload)
    expect_reject("leaf ID drift", lambda value: value["leaf_bindings"][0].update(atomic_pr_id="T1-M02-P230-PRJ-n009-l04"), payload)
    expect_reject("source hash drift", lambda value: value["candidate"]["source_blob_sha256"].update({"java/flink-jobs/flink-pcap-index-job/src/main/java/com/traffic/flink/pcap/PcapIndexJob.java": "0" * 64}), payload)
    expect_reject("false test PASS", lambda value: value["negative_tests"][0].update(execution_status="PASS"), payload)
    expect_reject("fabricated before signature", lambda value: next(item for item in value["function_contracts"] if item["change_kind"] == "modify").update(signature_before="public void missing()"), payload)
    expect_reject("missing P0", lambda value: value["current_static_findings"].pop(), payload)
    expect_reject("unknown state", lambda value: value["record_state_machine"]["transitions"][0].update(to="DROPPED"), payload)
    expect_reject("false indexed claim", lambda value: value["claims"]["forbidden"].remove("CLICKHOUSE_INDEXED"), payload)


def main() -> int:
    parser = argparse.ArgumentParser()
    parser.add_argument("--self-test", action="store_true")
    args = parser.parse_args()
    payload = json.loads(DESIGN.read_text(encoding="utf-8"))
    validate(payload)
    if args.self_test:
        self_test(payload)
    print("PASS M02 PCAP consumer design: 7 frozen leaves, 6 function contracts, 8 candidate-bound P0 findings, 14 NOT_RUN tests, live source and indexed claims remain BLOCKED")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
