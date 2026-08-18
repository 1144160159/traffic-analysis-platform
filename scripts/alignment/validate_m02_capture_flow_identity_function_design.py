#!/usr/bin/env python3
"""Validate the candidate-bound M02 capture-to-FlowEvent identity design."""

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
DESIGN = REPO / "contracts/alignment/m02-capture-flow-identity-function-design.v1.json"
SCHEMA = REPO / "contracts/alignment/m02-capture-flow-identity-function-design.schema.json"
PREVIEW = REPO / "contracts/alignment/m02-code-direct-leaf-catalog.v1.json"

EXPECTED_LEAF_ROLES = {
    "M02-N001-L01": "EVENT_HEADER_CONTRACT",
    "M02-N001-L02": "FLOW_EVENT_CONTRACT",
    "M02-N004-L15": "OFFLINE_TIMESTAMP",
    "M02-N005-L01": "CANONICAL_FLOW_KEY",
    "M02-N005-L02": "COMMUNITY_ID",
    "M02-N005-L03": "FULL_PATH_SAMPLE",
    "M02-N005-L04": "FAST_PATH_SAMPLE",
    "M02-N005-L05": "FLOW_EVENT_BUILD",
    "M02-N005-L06": "STABLE_PARTITION",
}
EXPECTED_FUNCTION_LEAVES = {
    "M02-N004-L15",
    "M02-N005-L01",
    "M02-N005-L02",
    "M02-N005-L03",
    "M02-N005-L04",
    "M02-N005-L05",
    "M02-N005-L06",
}
EXPECTED_P0 = {f"M02-FLOW-ID-P0-{index:02d}" for index in range(1, 20)}
EXPECTED_TESTS = {f"TC-M02-FLOW-ID-{index:02d}" for index in range(1, 23)}
EXPECTED_GAPS = {f"M02-FLOW-ID-GAP-{index:02d}" for index in range(1, 14)}
REQUIRED_SOURCE_PATHS = {
    "proto/traffic/v1/common.proto",
    "proto/traffic/v1/flow.proto",
    "rust/probe-agent/probe-agent/src/capture/mod.rs",
    "rust/probe-agent/probe-agent/src/capture/af_packet.rs",
    "rust/probe-agent/probe-agent/src/capture/xdp.rs",
    "rust/probe-agent/probe-agent/src/capture/pcap_offline.rs",
    "rust/probe-agent/probe-agent/src/parser/mod.rs",
    "rust/probe-agent/probe-agent/src/aggregator/flow_table.rs",
    "rust/probe-agent/probe-agent/src/aggregator/community_id.rs",
    "rust/probe-agent/probe-agent/src/aggregator/packet_processor.rs",
    "rust/probe-agent/probe-agent/src/aggregator/eviction.rs",
    "rust/probe-agent/probe-agent/src/aggregator/partitioned_flow_table.rs",
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
    if (
        preview_ref["path"] != PREVIEW.relative_to(REPO).as_posix()
        or preview_ref["sha256"] != sha256(PREVIEW)
    ):
        raise ValueError("capture flow identity preview catalog ref drifted")
    preview = json.loads(PREVIEW.read_text(encoding="utf-8"))
    preview_leaves = {item["leaf_id"]: item for item in preview["leaves"]}

    bindings = payload["leaf_bindings"]
    by_leaf = {item["leaf_id"]: item for item in bindings}
    if len(by_leaf) != len(bindings) or set(by_leaf) != set(EXPECTED_LEAF_ROLES):
        raise ValueError("capture flow identity leaf binding exact-set drifted")
    for leaf_id, role in EXPECTED_LEAF_ROLES.items():
        binding = by_leaf[leaf_id]
        leaf = preview_leaves.get(leaf_id)
        if leaf is None or binding["role"] != role:
            raise ValueError(f"capture flow identity role or leaf drifted: {leaf_id}")
        if (
            leaf["atomic_pr_id"] != binding["atomic_pr_id"]
            or binding["locator"] not in leaf["write_locators"]
        ):
            raise ValueError(f"capture flow identity ID or locator differs from preview: {leaf_id}")

    candidate = payload["candidate"]
    if not re.fullmatch(r"[0-9a-f]{40}", candidate["commit"]):
        raise ValueError("capture flow identity candidate commit is not immutable")
    source_hashes = candidate["source_blob_sha256"]
    if set(source_hashes) != REQUIRED_SOURCE_PATHS:
        raise ValueError("capture flow identity candidate source exact-set drifted")
    for relative, expected in source_hashes.items():
        path = REPO / relative
        if not path.is_file() or sha256(path) != expected:
            raise ValueError(f"capture flow identity source hash drifted: {relative}")

    findings = payload["current_static_findings"]
    if {item["finding_id"] for item in findings} != EXPECTED_P0 or len(findings) != len(EXPECTED_P0):
        raise ValueError("capture flow identity P0 exact-set drifted")
    if any(item["path"] not in source_hashes for item in findings):
        raise ValueError("a capture flow identity finding lacks candidate source binding")

    contracts = payload["function_contracts"]
    if {item["leaf_id"] for item in contracts} != EXPECTED_FUNCTION_LEAVES:
        raise ValueError("capture flow identity function leaf exact-set drifted")
    if len({item["contract_id"] for item in contracts}) != len(contracts):
        raise ValueError("duplicate capture flow identity contract ID")
    for contract in contracts:
        if contract["path"] not in source_hashes:
            raise ValueError(f"function contract lacks candidate source binding: {contract['contract_id']}")
        if not set(contract["tests"]).issubset(EXPECTED_TESTS):
            raise ValueError(f"unknown test in function contract: {contract['contract_id']}")
        before = contract["signature_before"]
        if contract["change_kind"] == "modify":
            if before is None or not source_contains_signature(REPO / contract["path"], before):
                raise ValueError(f"before signature absent from identity candidate: {contract['contract_id']}")
        elif before is not None:
            raise ValueError(f"new identity companion carries before signature: {contract['contract_id']}")
        step_ids = [step.split(" ", 1)[0] for step in contract["body_steps"]]
        if step_ids != [f"B{index:02d}" for index in range(1, len(step_ids) + 1)]:
            raise ValueError(f"identity body steps are not contiguous: {contract['contract_id']}")

    tests = payload["negative_tests"]
    if {item["case_id"] for item in tests} != EXPECTED_TESTS or len(tests) != len(EXPECTED_TESTS):
        raise ValueError("capture flow identity test exact-set drifted")
    if any(item["execution_status"] != "NOT_RUN" for item in tests):
        raise ValueError("capture flow identity design cannot claim test execution")

    gaps = payload["missing_atomic_leaves"]
    if {item["gap_id"] for item in gaps} != EXPECTED_GAPS or len(gaps) != len(EXPECTED_GAPS):
        raise ValueError("capture flow identity missing-leaf exact-set drifted")
    required_gap_tokens = {
        "M02-FLOW-ID-GAP-01": {"CaptureTimestamp::from_unix_parts", "CaptureTimestamp::epoch_micros"},
        "M02-FLOW-ID-GAP-02": {"AfPacketCapture::recv_frames_with_timestamps"},
        "M02-FLOW-ID-GAP-03": {"XdpCapture::consume_rx_with_timestamps"},
        "M02-FLOW-ID-GAP-04": {"canonicalize_observation"},
        "M02-FLOW-ID-GAP-05": {"PacketParser::decode_flow_fields"},
        "M02-FLOW-ID-GAP-06": {"FlowSampleBuilder::build"},
        "M02-FLOW-ID-GAP-07": {"FlowValue::apply_event_time"},
        "M02-FLOW-ID-GAP-08": {"EvictionClock::eviction_now"},
        "M02-FLOW-ID-GAP-09": {"FlowSnapshot::try_from_removed"},
        "M02-FLOW-ID-GAP-10": {"FlowEventIdentity::derive"},
        "M02-FLOW-ID-GAP-11": {"PacketProcessor::process_batch"},
        "M02-FLOW-ID-GAP-12": {"FlowAggregationKey::new"},
        "M02-FLOW-ID-GAP-13": {"FlowEvent.identity_revision"},
    }
    for gap in gaps:
        locators = " ".join(gap["required_locators"])
        if not all(token in locators for token in required_gap_tokens[gap["gap_id"]]):
            raise ValueError(f"capture flow identity gap locators weakened: {gap['gap_id']}")

    sequence = payload["sequencing"]
    if [item["order"] for item in sequence] != list(range(1, 10)):
        raise ValueError("capture flow identity sequence order drifted")
    if {item["leaf_id"] for item in sequence} != set(EXPECTED_LEAF_ROLES):
        raise ValueError("capture flow identity sequence leaf exact-set drifted")

    machine = payload["identity_state_machine"]
    states = set(machine["states"])
    for transition in machine["transitions"]:
        if transition["from"] not in states or transition["to"] not in states:
            raise ValueError("capture flow identity transition references unknown state")
    if not {
        "RAW_CAPTURED", "TIME_CANONICAL", "PARSED_FULL", "PARSED_FAST",
        "SAMPLE_CANONICAL", "FLOW_EVENT_BUILT", "IDENTITY_FROZEN",
        "REPLAY_MATCHED", "REJECTED", "BLOCKED",
    }.issubset(states):
        raise ValueError("capture flow identity state machine omits required states")

    direction = payload["direction_contract"]
    if "bidirectional" not in direction["aggregate_direction"] or "client/server" not in direction["aggregate_direction"]:
        raise ValueError("capture flow aggregate direction contract weakened")
    if "A-to-B" not in direction["counter_meaning"] or "B-to-A" not in direction["counter_meaning"]:
        raise ValueError("capture flow counter direction contract weakened")
    if "one_way=true" not in direction["one_way_policy"]:
        raise ValueError("capture flow one-way ICMP contract weakened")

    by_test = {item["case_id"]: item for item in tests}
    if "for each fixed count" not in by_test["TC-M02-FLOW-ID-06"]["oracle"]:
        raise ValueError("partition-count property oracle became impossible")
    if "excluding produced_at" not in by_test["TC-M02-FLOW-ID-11"]["oracle"]:
        raise ValueError("replay semantic projection does not exclude processing telemetry")
    if "source precision" not in by_test["TC-M02-FLOW-ID-15"]["oracle"]:
        raise ValueError("PCAP precision parity oracle weakened")
    if "injected fixture clock" not in by_test["TC-M02-FLOW-ID-16"]["injection"]:
        raise ValueError("capture-mode parity demands impossible live wall-clock equality")

    pcap_contract = next(item for item in contracts if item["leaf_id"] == "M02-N004-L15")
    if "PcapRecord" not in pcap_contract["signature_after"] or "u64" in pcap_contract["signature_after"]:
        raise ValueError("checked PCAP reader still returns an untyped timestamp")

    forbidden = set(payload["claims"]["forbidden"])
    if not {
        "CAPTURE_TIMESTAMP_IMPLEMENTED", "FULL_FAST_PARITY_PROVEN",
        "COMMUNITY_ID_CROSS_LANGUAGE_PROVEN", "REPLAY_IDENTITY_PROVEN",
        "FUNCTION_DESIGN_REVIEWED", "IMPLEMENTED", "TESTED",
        "EXECUTION_AUTHORIZED", "N004_ACCEPTED", "N005_ACCEPTED", "M02_ACCEPTED",
    }.issubset(forbidden):
        raise ValueError("capture flow identity claim ceiling is incomplete")


def expect_reject(
    name: str,
    mutate: Callable[[dict[str, Any]], None],
    source: dict[str, Any],
) -> None:
    candidate = copy.deepcopy(source)
    mutate(candidate)
    try:
        validate(candidate)
    except (ValueError, KeyError):
        return
    raise ValueError(f"malicious capture flow identity mutation was accepted: {name}")


def self_test(payload: dict[str, Any]) -> None:
    expect_reject("preview drift", lambda value: value["preview_catalog_ref"].update(sha256="0" * 64), payload)
    expect_reject("leaf drift", lambda value: value["leaf_bindings"][0].update(atomic_pr_id="T1-M02-P102-CTR-n001-l01"), payload)
    expect_reject("source omission", lambda value: value["candidate"]["source_blob_sha256"].pop("rust/probe-agent/probe-agent/src/capture/xdp.rs"), payload)
    expect_reject("false test PASS", lambda value: value["negative_tests"][0].update(execution_status="PASS"), payload)
    expect_reject("gap omission", lambda value: value["missing_atomic_leaves"].pop(), payload)
    expect_reject("fabricated before", lambda value: next(item for item in value["function_contracts"] if item["change_kind"] == "modify").update(signature_before="fn missing()"), payload)
    expect_reject("unknown state", lambda value: value["identity_state_machine"]["transitions"][0].update(to="DROPPED"), payload)
    expect_reject("direction weakened", lambda value: value["direction_contract"].update(aggregate_direction="c2s"), payload)
    expect_reject("false parity claim", lambda value: value["claims"]["forbidden"].remove("FULL_FAST_PARITY_PROVEN"), payload)


def main() -> int:
    parser = argparse.ArgumentParser()
    parser.add_argument("--self-test", action="store_true")
    args = parser.parse_args()
    payload = json.loads(DESIGN.read_text(encoding="utf-8"))
    validate(payload)
    if args.self_test:
        self_test(payload)
    print(
        "PASS M02 capture flow identity design: 9 frozen leaves, 7 function contracts, "
        "19 candidate-bound P0 findings, 13 missing atomic leaves BLOCKED, 22 NOT_RUN tests"
    )
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
