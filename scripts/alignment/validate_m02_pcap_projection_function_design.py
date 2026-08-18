#!/usr/bin/env python3
"""Validate the candidate-bound M02 PCAP carrier-to-ClickHouse design."""

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
DESIGN = REPO / "contracts/alignment/m02-pcap-projection-function-design.v1.json"
SCHEMA = REPO / "contracts/alignment/m02-pcap-projection-function-design.schema.json"
PREVIEW = REPO / "contracts/alignment/m02-code-direct-leaf-catalog.v1.json"

EXPECTED_LEAF_ROLES = {
    "M02-N010-L02": "CARRIER_PROCESS", "M02-N010-L03": "JOB_GRAPH", "M02-N010-L04": "CHECKPOINT",
    "M02-N011-L01": "PROTO_MANIFEST", "M02-N011-L02": "DDL_MIGRATION", "M02-N011-L03": "INSERT_SQL",
    "M02-N011-L04": "STATEMENT_BINDER", "M02-N011-L05": "CARRIER_SINK", "M02-N011-L06": "MANIFEST_VALIDATOR",
}
EXPECTED_FUNCTION_LEAVES = {
    "M02-N010-L02", "M02-N010-L03", "M02-N010-L04",
    "M02-N011-L03", "M02-N011-L04", "M02-N011-L05", "M02-N011-L06",
}
EXPECTED_P0 = {f"M02-PCAP-PROJECTION-P0-{index:02d}" for index in range(1, 11)}
EXPECTED_TESTS = {f"TC-M02-PCAP-PROJECTION-{index:02d}" for index in range(1, 17)}
EXPECTED_DDL_PATHS = {
    "common/sql/ch/00-all-tables.sql",
    "deployments/kubernetes/init-jobs/03-clickhouse-schema.yaml",
    "go/control-plane/deployments/docker/init/clickhouse_merged.sql",
    "java/flink-jobs/flink-pcap-index-job/deployments/docker/init-scripts/clickhouse/01-create-tables.sql",
}


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
        raise ValueError("PCAP projection preview catalog ref drifted")
    preview = json.loads(PREVIEW.read_text(encoding="utf-8"))
    preview_leaves = {item["leaf_id"]: item for item in preview["leaves"]}

    bindings = payload["leaf_bindings"]
    by_leaf = {item["leaf_id"]: item for item in bindings}
    if len(by_leaf) != len(bindings) or set(by_leaf) != set(EXPECTED_LEAF_ROLES):
        raise ValueError("PCAP projection leaf binding exact-set drifted")
    for leaf_id, role in EXPECTED_LEAF_ROLES.items():
        binding = by_leaf[leaf_id]
        leaf = preview_leaves.get(leaf_id)
        if binding["role"] != role or leaf is None:
            raise ValueError(f"PCAP projection binding role or leaf drifted: {leaf_id}")
        if leaf["atomic_pr_id"] != binding["atomic_pr_id"] or binding["locator"] not in leaf["write_locators"]:
            raise ValueError(f"PCAP projection ID or locator differs from preview: {leaf_id}")

    candidate = payload["candidate"]
    if not re.fullmatch(r"[0-9a-f]{40}", candidate["commit"]):
        raise ValueError("PCAP projection candidate commit is not immutable")
    for relative, expected in candidate["source_blob_sha256"].items():
        path = REPO / relative
        if not path.is_file() or sha256(path) != expected:
            raise ValueError(f"PCAP projection source hash drifted: {relative}")

    findings = payload["current_static_findings"]
    if {item["finding_id"] for item in findings} != EXPECTED_P0 or len(findings) != len(EXPECTED_P0):
        raise ValueError("PCAP projection P0 exact-set drifted")
    if any(item["path"] not in candidate["source_blob_sha256"] for item in findings):
        raise ValueError("a PCAP projection finding lacks candidate source binding")

    ddl_sources = payload["ddl_authority_analysis"]["sources"]
    if {item["path"] for item in ddl_sources} != EXPECTED_DDL_PATHS or len(ddl_sources) != len(EXPECTED_DDL_PATHS):
        raise ValueError("PCAP DDL source exact-set drifted")
    for source in ddl_sources:
        if source["sha256"] != candidate["source_blob_sha256"].get(source["path"]):
            raise ValueError(f"PCAP DDL analysis hash differs from candidate: {source['path']}")

    tests = payload["negative_tests"]
    if {item["case_id"] for item in tests} != EXPECTED_TESTS or len(tests) != len(EXPECTED_TESTS):
        raise ValueError("PCAP projection test exact-set drifted")
    if any(item["execution_status"] != "NOT_RUN" for item in tests):
        raise ValueError("PCAP projection design tests cannot claim execution")

    contracts = payload["function_contracts"]
    if {item["leaf_id"] for item in contracts} != EXPECTED_FUNCTION_LEAVES or len(contracts) != len(EXPECTED_FUNCTION_LEAVES):
        raise ValueError("PCAP projection function leaf exact-set drifted")
    if len({item["contract_id"] for item in contracts}) != len(contracts):
        raise ValueError("duplicate PCAP projection function contract ID")
    for contract in contracts:
        if not set(contract["tests"]).issubset(EXPECTED_TESTS):
            raise ValueError(f"unknown projection test in {contract['contract_id']}")
        before = contract["signature_before"]
        if contract["change_kind"] == "modify":
            if before is None or not source_contains_signature(REPO / contract["path"], before):
                raise ValueError(f"before signature absent from projection candidate: {contract['contract_id']}")
        elif before is not None:
            raise ValueError(f"planned projection companion carries a before signature: {contract['contract_id']}")
        step_ids = [step.split(" ", 1)[0] for step in contract["body_steps"]]
        if step_ids != [f"B{index:02d}" for index in range(1, len(step_ids) + 1)]:
            raise ValueError(f"projection body steps are not contiguous: {contract['contract_id']}")

    sequence = payload["sequencing"]
    if [item["order"] for item in sequence] != list(range(1, len(sequence) + 1)):
        raise ValueError("PCAP projection sequence order is not contiguous")
    if {item["leaf_id"] for item in sequence} != set(EXPECTED_LEAF_ROLES):
        raise ValueError("PCAP projection sequence does not cover the leaf exact-set")

    states = set(payload["projection_state_machine"]["states"])
    for transition in payload["projection_state_machine"]["transitions"]:
        if transition["from"] not in states or transition["to"] not in states:
            raise ValueError("PCAP projection transition references unknown state")
    if not {"CLICKHOUSE_ACKED", "CHECKPOINT_COMPLETE", "INDEXED_CANDIDATE", "RECONCILED", "BLOCKED"}.issubset(states):
        raise ValueError("PCAP projection state machine omits a required state")

    forbidden = set(payload["claims"]["forbidden"])
    if not {"DDL_AUTHORITY_SELECTED", "IMPLEMENTED", "TESTED", "CHECKPOINT_SAFE", "CLICKHOUSE_INDEXED", "RECONCILED", "EXECUTION_AUTHORIZED"}.issubset(forbidden):
        raise ValueError("PCAP projection claim ceiling is incomplete")


def expect_reject(name: str, mutate: Callable[[dict[str, Any]], None], source: dict[str, Any]) -> None:
    candidate = copy.deepcopy(source)
    mutate(candidate)
    try:
        validate(candidate)
    except (ValueError, KeyError):
        return
    raise ValueError(f"malicious PCAP projection design mutation was accepted: {name}")


def self_test(payload: dict[str, Any]) -> None:
    expect_reject("preview drift", lambda value: value["preview_catalog_ref"].update(sha256="0" * 64), payload)
    expect_reject("leaf drift", lambda value: value["leaf_bindings"][0].update(atomic_pr_id="T1-M02-P245-PRJ-n010-l02"), payload)
    expect_reject("source drift", lambda value: value["candidate"]["source_blob_sha256"].update({"proto/traffic/v1/pcap.proto": "0" * 64}), payload)
    expect_reject("DDL omission", lambda value: value["ddl_authority_analysis"]["sources"].pop(), payload)
    expect_reject("false test PASS", lambda value: value["negative_tests"][0].update(execution_status="PASS"), payload)
    expect_reject("fabricated before", lambda value: next(item for item in value["function_contracts"] if item["change_kind"] == "modify").update(signature_before="public static void missing()"), payload)
    expect_reject("unknown state", lambda value: value["projection_state_machine"]["transitions"][0].update(to="DROPPED"), payload)
    expect_reject("false indexed claim", lambda value: value["claims"]["forbidden"].remove("CLICKHOUSE_INDEXED"), payload)


def main() -> int:
    parser = argparse.ArgumentParser()
    parser.add_argument("--self-test", action="store_true")
    args = parser.parse_args()
    payload = json.loads(DESIGN.read_text(encoding="utf-8"))
    validate(payload)
    if args.self_test:
        self_test(payload)
    print("PASS M02 PCAP projection design: 9 frozen leaves, 7 function contracts, 10 candidate-bound P0 findings, 4 DDL drift inputs, 16 NOT_RUN tests, indexed remains BLOCKED")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
