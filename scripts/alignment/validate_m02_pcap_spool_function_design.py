#!/usr/bin/env python3
"""Validate the candidate-bound M02 PCAP durable-spool function design."""

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
DESIGN = REPO / "contracts/alignment/m02-pcap-spool-function-design.v1.json"
SCHEMA = REPO / "contracts/alignment/m02-pcap-spool-function-design.schema.json"
PREVIEW = REPO / "contracts/alignment/m02-code-direct-leaf-catalog.v1.json"

EXPECTED_LEAF_ROLES = {
    "M02-N006-L04": "UPLOAD_CONSUMER",
    "M02-N006-L05": "DURABLE_SPOOL",
    "M02-N006-L06": "BUFFER_LEASE",
    "M02-N006-L07": "JOURNAL_PENDING",
    "M02-N006-L08": "ROTATOR_PRODUCER",
    "M02-N006-L12": "OBJECT_RECEIPT",
    "M02-N006-L15": "JOURNAL_RECEIPT",
    "M02-N006-L16": "MAIN_UPLOAD",
    "M02-N006-L17": "RECOVERY",
}
EXPECTED_P0 = {f"M02-PCAP-P0-{index:02d}" for index in range(1, 11)}
EXPECTED_GAPS = {f"M02-PCAP-GAP-{index:02d}" for index in range(1, 4)}
EXPECTED_TESTS = {f"TC-M02-PCAP-{index:02d}" for index in range(1, 19)}


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
        raise ValueError("PCAP design references the wrong M02 preview catalog")
    if preview_ref["sha256"] != sha256(PREVIEW):
        raise ValueError("PCAP design preview catalog hash drifted")
    preview = json.loads(PREVIEW.read_text(encoding="utf-8"))
    preview_leaves = {item["leaf_id"]: item for item in preview["leaves"]}

    bindings = payload["leaf_bindings"]
    by_leaf = {item["leaf_id"]: item for item in bindings}
    if len(by_leaf) != len(bindings) or set(by_leaf) != set(EXPECTED_LEAF_ROLES):
        raise ValueError("PCAP leaf binding exact-set drifted")
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
    if not re.fullmatch(r"[0-9a-f]{40}", candidate["commit"]):
        raise ValueError("candidate commit is not immutable")
    if not (REPO / ".git").exists():
        raise ValueError("repository git metadata is unavailable")
    for relative, expected in candidate["source_blob_sha256"].items():
        path = REPO / relative
        if not path.is_file() or sha256(path) != expected:
            raise ValueError(f"candidate source hash drifted: {relative}")

    findings = payload["current_static_findings"]
    if {item["finding_id"] for item in findings} != EXPECTED_P0 or len(findings) != len(EXPECTED_P0):
        raise ValueError("PCAP P0 finding exact-set drifted")
    if any(item["path"] not in candidate["source_blob_sha256"] for item in findings):
        raise ValueError("a PCAP finding is not bound to a candidate source blob")

    gaps = payload["missing_atomic_leaves"]
    if {item["gap_id"] for item in gaps} != EXPECTED_GAPS or len(gaps) != len(EXPECTED_GAPS):
        raise ValueError("PCAP missing-leaf exact-set drifted")
    if any(item["status"] != "BLOCKED_MISSING_ATOMIC_LEAF_AND_STABLE_ID" for item in gaps):
        raise ValueError("a missing atomic leaf is not blocked")
    if any("Result<" not in item["proposed_signature"] for item in gaps):
        raise ValueError("a missing atomic leaf lacks a typed Result signature")

    revision = payload["registry_revision_gate"]
    if revision["decision"] != "KEEP_GAPS_BLOCKED_UNTIL_FORMAL_REGISTRY_REVISION":
        raise ValueError("PCAP registry revision decision drifted")
    if revision["active_global_m02_pr_count"] != 34 or revision["preview_leaf_count"] != 207:
        raise ValueError("PCAP registry revision baseline counts drifted")
    if len(revision["revision_requirements"]) < 5:
        raise ValueError("PCAP registry revision gate is incomplete")

    tests = payload["negative_tests"]
    if {item["case_id"] for item in tests} != EXPECTED_TESTS or len(tests) != len(EXPECTED_TESTS):
        raise ValueError("PCAP test exact-set drifted")
    if any(item["execution_status"] != "NOT_RUN" for item in tests):
        raise ValueError("design-only PCAP tests cannot claim execution")

    contracts = payload["function_contracts"]
    contract_ids = [item["contract_id"] for item in contracts]
    if len(contract_ids) != len(set(contract_ids)):
        raise ValueError("duplicate PCAP function contract ID")
    if {item["leaf_id"] for item in contracts} != set(EXPECTED_LEAF_ROLES):
        raise ValueError("PCAP function contracts do not cover the exact bound leaf set")
    for contract in contracts:
        if not set(contract["tests"]).issubset(EXPECTED_TESTS):
            raise ValueError(f"function contract references an unknown test: {contract['contract_id']}")
        before = contract["signature_before"]
        if contract["change_kind"] == "modify":
            if before is None or not source_contains_signature(REPO / contract["path"], before):
                raise ValueError(f"before signature is absent from candidate source: {contract['contract_id']}")
        elif before is not None:
            raise ValueError(f"planned PCAP companion carries a before signature: {contract['contract_id']}")
        step_ids = [step.split(" ", 1)[0] for step in contract["body_steps"]]
        if step_ids != [f"B{index:02d}" for index in range(1, len(step_ids) + 1)]:
            raise ValueError(f"body steps are not contiguous: {contract['contract_id']}")

    sequence = payload["sequencing"]
    if [item["order"] for item in sequence] != list(range(1, len(sequence) + 1)):
        raise ValueError("PCAP sequence order is not contiguous")
    if {item["leaf_id"] for item in sequence} != set(EXPECTED_LEAF_ROLES):
        raise ValueError("PCAP sequence does not cover the exact bound leaf set")

    states = set(payload["spool_state_machine"]["states"])
    for transition in payload["spool_state_machine"]["transitions"]:
        if transition["from"] not in states or transition["to"] not in states:
            raise ValueError("PCAP transition references an unknown state")
    required_states = {"SPOOL_DURABLE", "JOURNALED_PENDING", "OBJECT_WRITTEN", "METADATA_ACCEPTED", "CLEANUP_AUTHORIZED", "QUARANTINED"}
    if not required_states.issubset(states):
        raise ValueError("PCAP state machine omits a required durability state")

    forbidden = set(payload["claims"]["forbidden"])
    if not {"REGISTRY_INTEGRATED", "IMPLEMENTED", "TESTED", "EXECUTION_AUTHORIZED", "M02_ACCEPTED"}.issubset(forbidden):
        raise ValueError("PCAP claim ceiling is incomplete")


def expect_reject(name: str, mutate: Callable[[dict[str, Any]], None], source: dict[str, Any]) -> None:
    candidate = copy.deepcopy(source)
    mutate(candidate)
    try:
        validate(candidate)
    except (ValueError, KeyError):
        return
    raise ValueError(f"malicious PCAP design mutation was accepted: {name}")


def self_test(payload: dict[str, Any]) -> None:
    expect_reject("preview hash drift", lambda value: value["preview_catalog_ref"].update(sha256="0" * 64), payload)
    expect_reject("leaf identity drift", lambda value: value["leaf_bindings"][0].update(atomic_pr_id="T1-M02-P199-WRT-n006-l04"), payload)
    expect_reject("candidate source drift", lambda value: value["candidate"]["source_blob_sha256"].update({"rust/probe-agent/probe-agent/src/main.rs": "0" * 64}), payload)
    expect_reject("false test PASS", lambda value: value["negative_tests"][0].update(execution_status="PASS"), payload)
    expect_reject("missing atomic gap", lambda value: value["missing_atomic_leaves"].pop(), payload)
    expect_reject("premature registry revision", lambda value: value["registry_revision_gate"].update(decision="ASSIGN_P308_NOW"), payload)
    expect_reject("fabricated before signature", lambda value: next(item for item in value["function_contracts"] if item["change_kind"] == "modify").update(signature_before="fn missing()"), payload)
    expect_reject("unknown transition state", lambda value: value["spool_state_machine"]["transitions"][0].update(to="LOST"), payload)
    expect_reject("false integrated claim", lambda value: value["claims"]["forbidden"].remove("REGISTRY_INTEGRATED"), payload)


def main() -> int:
    parser = argparse.ArgumentParser()
    parser.add_argument("--self-test", action="store_true")
    args = parser.parse_args()
    payload = json.loads(DESIGN.read_text(encoding="utf-8"))
    validate(payload)
    if args.self_test:
        self_test(payload)
    print("PASS M02 PCAP spool design: 9 frozen leaves, 9 function contracts, 10 candidate-bound P0 findings, 3 missing atomic leaves BLOCKED, 18 NOT_RUN tests")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
