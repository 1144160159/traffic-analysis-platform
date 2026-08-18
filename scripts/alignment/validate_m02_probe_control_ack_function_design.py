#!/usr/bin/env python3
"""Validate the candidate-bound M02 probe control and ACK function design."""

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
DESIGN = REPO / "contracts/alignment/m02-probe-control-ack-function-design.v1.json"
SCHEMA = REPO / "contracts/alignment/m02-probe-control-ack-function-design.schema.json"
PREVIEW = REPO / "contracts/alignment/m02-code-direct-leaf-catalog.v1.json"

EXPECTED_LEAVES = {f"M02-N012-L{index:02d}" for index in range(1, 24)}
EXPECTED_FUNCTION_LEAVES = {
    "M02-N012-L05", "M02-N012-L06", "M02-N012-L07", "M02-N012-L08",
    "M02-N012-L09", "M02-N012-L10", "M02-N012-L11", "M02-N012-L14",
    "M02-N012-L15", "M02-N012-L16", "M02-N012-L18", "M02-N012-L20",
    "M02-N012-L66", "M02-N012-L69", "M02-N012-L78", "M02-N012-L90",
    "M02-N012-L99", "M02-N012-L102", "M02-N012-L105", "M02-N012-L108",
    "M02-N012-L111", "M02-N012-L114", "M02-N012-L117",
}
EXPECTED_APPEND_ONLY_BINDING_LEAVES = {
    "M02-N012-L66", "M02-N012-L69", "M02-N012-L78", "M02-N012-L90",
    "M02-N012-L99", "M02-N012-L102", "M02-N012-L105", "M02-N012-L108",
    "M02-N012-L111", "M02-N012-L114", "M02-N012-L117",
}
EXPECTED_FINDINGS = {
    *{f"M02-PROBE-CTRL-P0-{index:02d}" for index in range(1, 9)},
    *{f"M02-PROBE-CTRL-P1-{index:02d}" for index in range(1, 6)},
}
EXPECTED_GAPS = {f"M02-PROBE-CTRL-GAP-{index:02d}" for index in range(1, 32)}
EXPECTED_TESTS = {f"TC-M02-PROBE-CTRL-{index:02d}" for index in range(1, 35)}
REQUIRED_GAP_TOKENS = {
    "M02-PROBE-CTRL-GAP-01": {"publishProbeOperationOutboxItem"},
    "M02-PROBE-CTRL-GAP-02": {"ApplyProbeOperationProjection"},
    "M02-PROBE-CTRL-GAP-03": {"RouteMessage"},
    "M02-PROBE-CTRL-GAP-04": {"RedisCommandStore.List", "DeleteIfRevision"},
    "M02-PROBE-CTRL-GAP-05": {"classifyProbeAckError", "newProbeAckConsumerConfig"},
    "M02-PROBE-CTRL-GAP-06": {"expireProbeOperations"},
    "M02-PROBE-CTRL-GAP-07": {"NewKeyedProducer"},
    "M02-PROBE-CTRL-GAP-08": {"ProbePipelineReadinessReceipt", "AllowClaim"},
    "M02-PROBE-CTRL-GAP-09": {"probeOperationExpiredEvent"},
    "M02-PROBE-CTRL-GAP-10": {"probe_pipeline_readiness_epochs", "FenceClaim"},
    "M02-PROBE-CTRL-GAP-11": {"probe_operation_outbox", "KeyedProducer.Send"},
    "M02-PROBE-CTRL-GAP-12": {"probe_operation_event_projection_event_type_check", "claimProbeOperationOutbox"},
    "M02-PROBE-CTRL-GAP-13": {"IssueRenewRevoke"},
    "M02-PROBE-CTRL-GAP-14": {"GenerationConsumer.SetGroupLifecycleObserver"},
    "M02-PROBE-CTRL-GAP-15": {"wireProbeControlGroupLifecycle", "ProbeControlReadinessPublisher.Publish"},
    "M02-PROBE-CTRL-GAP-16": {"ProbeGroupReadinessReceiptV1"},
    "M02-PROBE-CTRL-GAP-17": {"create-topics.sh", "01-kafka-topics.yaml", "probe.group-readiness.v1"},
    "M02-PROBE-CTRL-GAP-18": {"ProbeReadinessConsumer.StartGeneration"},
    "M02-PROBE-CTRL-GAP-19": {"probe.group-readiness.v1"},
    "M02-PROBE-CTRL-GAP-20": {"ProbeGroupReadinessTopic"},
    "M02-PROBE-CTRL-GAP-21": {"ProbeGroupReadinessTopic"},
    "M02-PROBE-CTRL-GAP-22": {"GenerationConsumer.Run", "ConsumerGroup.Next", "Generation.Start"},
    "M02-PROBE-CTRL-GAP-23": {"ingest-gateway.KAFKA_PROBE_GROUP_READINESS_TOPIC", "KAFKA_PROBE_GROUP_READINESS_TOPIC"},
    "M02-PROBE-CTRL-GAP-24": {"alert-service.KAFKA_PROBE_GROUP_READINESS_TOPIC", "KAFKA_PROBE_GROUP_READINESS_TOPIC"},
    "M02-PROBE-CTRL-GAP-25": {"NewGenerationMessageProcessor", "GenerationMessageProcessor.ProcessPartition"},
    "M02-PROBE-CTRL-GAP-26": {"ProbeAckConsumer.StartGeneration"},
    "M02-PROBE-CTRL-GAP-27": {"ProbeOperationEventConsumer.StartGeneration"},
    "M02-PROBE-CTRL-GAP-28": {"Router.StartGeneration"},
    "M02-PROBE-CTRL-GAP-29": {"ProbeControlReadinessPublisher.RunRenewal"},
    "M02-PROBE-CTRL-GAP-30": {"startProbeAuthorityGenerationConsumers"},
    "M02-PROBE-CTRL-GAP-31": {"startProbeCommandGenerationConsumer"},
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
        raise ValueError("probe control preview catalog ref drifted")
    preview = json.loads(PREVIEW.read_text(encoding="utf-8"))
    resolution = payload["append_only_resolution"]
    if resolution != {
        "path": "contracts/alignment/m02-code-direct-leaf-allocation.v2.json",
        "revision_id": "M02-CODE-DIRECT-V2",
        "allocation_epoch": "T1-M02-P308-P506",
        "status": "ALLOCATED_V2_PREVIEW_NOT_IMPLEMENTED",
    }:
        raise ValueError("probe control append-only resolution drifted")
    preview_leaves = {item["leaf_id"]: item for item in preview["leaves"]}

    bindings = payload["leaf_bindings"]
    by_leaf = {item["leaf_id"]: item for item in bindings}
    if len(bindings) != len(by_leaf) or set(by_leaf) != EXPECTED_LEAVES:
        raise ValueError("probe control leaf binding exact-set drifted")
    for leaf_id, binding in by_leaf.items():
        leaf = preview_leaves.get(leaf_id)
        if leaf is None or leaf["parent_task_id"] != "T1-M02-N012":
            raise ValueError(f"probe control preview leaf missing or wrong parent: {leaf_id}")
        if leaf["atomic_pr_id"] != binding["atomic_pr_id"] or binding["locator"] not in leaf["write_locators"]:
            raise ValueError(f"probe control leaf ID or locator differs from preview: {leaf_id}")

    append_bindings = payload["append_only_leaf_bindings"]
    append_by_leaf = {item["leaf_id"]: item for item in append_bindings}
    if len(append_bindings) != len(append_by_leaf) or set(append_by_leaf) != EXPECTED_APPEND_ONLY_BINDING_LEAVES:
        raise ValueError("probe control append-only leaf binding exact-set drifted")
    allocation = json.loads((REPO / resolution["path"]).read_text(encoding="utf-8"))
    allocation_steps = {
        item["leaf_id"]: item
        for train in allocation["trains"]
        for item in train["steps"]
    }
    for leaf_id, binding in append_by_leaf.items():
        step = allocation_steps.get(leaf_id)
        if step is None or step["atomic_pr_id"] != binding["atomic_pr_id"]:
            raise ValueError(f"probe control append-only leaf ID differs from allocation: {leaf_id}")
        if binding["locator"] not in [step["primary_locator"], *step["companion_locators"]]:
            raise ValueError(f"probe control append-only locator differs from allocation: {leaf_id}")

    candidate = payload["candidate"]
    if not re.fullmatch(r"[0-9a-f]{40}", candidate["commit"]):
        raise ValueError("probe control candidate commit is not immutable")
    for relative, expected in candidate["source_blob_sha256"].items():
        path = REPO / relative
        if not path.is_file() or sha256(path) != expected:
            raise ValueError(f"probe control source hash drifted: {relative}")

    findings = payload["current_static_findings"]
    if len(findings) != len(EXPECTED_FINDINGS) or {item["finding_id"] for item in findings} != EXPECTED_FINDINGS:
        raise ValueError("probe control finding exact-set drifted")
    if any(item["path"] not in candidate["source_blob_sha256"] for item in findings):
        raise ValueError("a probe control finding lacks candidate source binding")
    if sum(item["severity"] == "P0" for item in findings) != 8 or sum(item["severity"] == "P1" for item in findings) != 5:
        raise ValueError("probe control severity count drifted")

    gaps = payload["append_only_gap_resolutions"]
    by_gap = {item["gap_id"]: item for item in gaps}
    if len(gaps) != len(by_gap) or set(by_gap) != EXPECTED_GAPS:
        raise ValueError("probe control gap exact-set drifted")
    for gap_id, tokens in REQUIRED_GAP_TOKENS.items():
        locators = " ".join(by_gap[gap_id]["required_locators"])
        if not all(token in locators for token in tokens):
            raise ValueError(f"probe control gap locator drifted: {gap_id}")
        if by_gap[gap_id]["status"] != "ALLOCATED_V2_PREVIEW_NOT_IMPLEMENTED":
            raise ValueError(f"probe control gap allocation status drifted: {gap_id}")

    tests = payload["negative_tests"]
    if len(tests) != len(EXPECTED_TESTS) or {item["case_id"] for item in tests} != EXPECTED_TESTS:
        raise ValueError("probe control test exact-set drifted")
    if any(item["execution_status"] != "NOT_RUN" for item in tests):
        raise ValueError("probe control static design cannot claim an executed test")
    oracle_by_id = {item["case_id"]: item["oracle"] for item in tests}
    if "same partition for each fixed partition count" not in oracle_by_id["TC-M02-PROBE-CTRL-10"]:
        raise ValueError("partition oracle incorrectly promises stability across counts")
    if "Gateway acceptance can be true" not in oracle_by_id["TC-M02-PROBE-CTRL-11"]:
        raise ValueError("applied false and Gateway acceptance scopes were collapsed")
    if "different Kafka offset" not in tests[8]["injection"]:
        raise ValueError("cross-offset redelivery oracle is absent")
    if "traffic-ingest-gateway may write" not in oracle_by_id["TC-M02-PROBE-CTRL-22"]:
        raise ValueError("readiness transport exclusive writer ACL oracle is absent")
    if "producer-before-consumer" not in next(item for item in tests if item["case_id"] == "TC-M02-PROBE-CTRL-24")["injection"]:
        raise ValueError("readiness transport startup-order oracle is absent")
    if "ConsumerGroup Next and Generation Start" not in next(item for item in tests if item["case_id"] == "TC-M02-PROBE-CTRL-25")["injection"]:
        raise ValueError("real Kafka generation oracle is absent")
    if "publisher may still publish durable producer-first receipts" not in oracle_by_id["TC-M02-PROBE-CTRL-23"]:
        raise ValueError("producer-first and dispatcher fail-closed scopes were collapsed")
    if "dual-path topology" not in oracle_by_id["TC-M02-PROBE-CTRL-31"] or "duplicate same-group Reader" not in oracle_by_id["TC-M02-PROBE-CTRL-32"]:
        raise ValueError("generation consumer cutover oracle is absent")
    if "two lease TTLs" not in next(item for item in tests if item["case_id"] == "TC-M02-PROBE-CTRL-33")["injection"]:
        raise ValueError("generation-bound readiness renewal oracle is absent")
    offset_case = next(item for item in tests if item["case_id"] == "TC-M02-PROBE-CTRL-34")
    if (
        "invalid processor config" not in offset_case["injection"]
        or "Generation CommitOffsets response loss" not in offset_case["injection"]
        or "COMMIT_OUTCOME_UNKNOWN" not in offset_case["oracle"]
        or "same or a later offset" not in offset_case["oracle"]
    ):
        raise ValueError("generation offset durability oracle is absent")

    contracts = payload["function_contracts"]
    if len(contracts) != len(EXPECTED_FUNCTION_LEAVES) or {item["leaf_id"] for item in contracts} != EXPECTED_FUNCTION_LEAVES:
        raise ValueError("probe control function leaf exact-set drifted")
    if len({item["contract_id"] for item in contracts}) != len(contracts):
        raise ValueError("duplicate probe control function contract ID")
    for contract in contracts:
        if not set(contract["tests"]).issubset(EXPECTED_TESTS):
            raise ValueError(f"unknown test in {contract['contract_id']}")
        before = contract["signature_before"]
        path = REPO / contract["path"]
        if contract["change_kind"] == "modify":
            if before is None or not source_contains_signature(path, before):
                raise ValueError(f"before signature absent from probe control candidate: {contract['contract_id']}")
        elif before is not None:
            raise ValueError(f"planned probe control companion has a before signature: {contract['contract_id']}")
        step_ids = [step.split(" ", 1)[0] for step in contract["body_steps"]]
        if step_ids != [f"B{index:02d}" for index in range(1, len(step_ids) + 1)]:
            raise ValueError(f"probe control body steps are not contiguous: {contract['contract_id']}")
    contracts_by_leaf = {item["leaf_id"]: item for item in contracts}
    required_generation_callees = {
        "M02-N012-L78": {"GenerationConsumer.Run", "GenerationMessageProcessor.ProcessPartition", "ProbeReadinessConsumer.handle"},
        "M02-N012-L90": {"kafka.NewConsumerGroup", "ConsumerGroup.Next", "Generation.Start"},
        "M02-N012-L99": {"assigned partition fetch", "MessageHandler", "DLQProducer.Send", "DLQAcknowledgementBarrier", "Generation.CommitOffsets", "commit observer"},
        "M02-N012-L102": {"GenerationConsumer.Run", "GenerationMessageProcessor.ProcessPartition", "ProbeAckConsumer.handle"},
        "M02-N012-L105": {"GenerationConsumer.Run", "GenerationMessageProcessor.ProcessPartition", "ProbeOperationEventConsumer.handle"},
        "M02-N012-L108": {"GenerationConsumer.Run", "GenerationMessageProcessor.ProcessPartition", "Router.Route"},
        "M02-N012-L111": {"ProbeControlReadinessPublisher.Publish", "time.NewTimer"},
        "M02-N012-L114": {"commonkafka.NewGenerationConsumer", "commonkafka.NewGenerationMessageProcessor", "ProbeAckConsumer.StartGeneration", "ProbeOperationEventConsumer.StartGeneration", "ProbeReadinessConsumer.StartGeneration"},
        "M02-N012-L117": {"commonkafka.NewGenerationConsumer", "commonkafka.NewGenerationMessageProcessor", "Router.StartGeneration", "wireProbeControlGroupLifecycle", "ProbeControlReadinessPublisher.RunRenewal"},
    }
    for leaf_id, expected_callees in required_generation_callees.items():
        if not expected_callees.issubset(set(contracts_by_leaf[leaf_id]["callees"])):
            raise ValueError(f"probe control generation call chain drifted: {leaf_id}")
    if "NewGenerationMessageProcessor" not in contracts_by_leaf["M02-N012-L99"]["signature_after"]:
        raise ValueError("generation message processor factory contract is absent")
    for leaf_id in {"M02-N012-L78", "M02-N012-L102", "M02-N012-L105", "M02-N012-L108"}:
        if "processor *commonkafka.GenerationMessageProcessor" not in contracts_by_leaf[leaf_id]["signature_after"]:
            raise ValueError(f"generation adapter lacks an injected processor: {leaf_id}")
    legacy_constructor = "commonkafka.NewConsumer"
    if any(legacy_constructor in item["callees"] for item in contracts if item["leaf_id"] in {"M02-N012-L18", "M02-N012-L20", "M02-N012-L78", "M02-N012-L114", "M02-N012-L117"}):
        raise ValueError("probe control startup retained an ambiguous legacy Reader constructor")

    sequence = payload["sequencing"]
    if [item["order"] for item in sequence] != list(range(1, 24)) or {item["leaf_id"] for item in sequence} != EXPECTED_LEAVES:
        raise ValueError("probe control sequencing exact-set drifted")

    states = set(payload["authority_state_machine"]["states"])
    required_states = {"DESIRED_DURABLE", "CONTROL_BROKER_ACKED", "DELIVERY_CACHE_DURABLE", "AGENT_TERMINAL_ACK_DURABLE", "GATEWAY_ACK_ACCEPTED", "ACK_AUTHORITY_DURABLE", "PROJECTION_DURABLE", "CONFLICT", "BLOCKED"}
    if not required_states.issubset(states):
        raise ValueError("probe control authority state machine omits required states")
    for transition in payload["authority_state_machine"]["transitions"]:
        if transition["from"] not in states or transition["to"] not in states:
            raise ValueError("probe control transition references an unknown state")

    acceptance = payload["ack_acceptance_contract"]
    if "never means applied true" not in acceptance["legacy_field"] or "PostgreSQL" not in acceptance["forbidden_interpretation"]:
        raise ValueError("probe ACK acceptance scope was overstated")

    forbidden = set(payload["claims"]["forbidden"])
    required_forbidden = {"FUNCTION_DESIGN_REVIEWED", "IMPLEMENTED", "TESTED", "REAL_KAFKA_PROVEN", "ROLLOUT_EXECUTED", "EXECUTION_AUTHORIZED", "N012_ACCEPTED", "M02_ACCEPTED"}
    if not required_forbidden.issubset(forbidden):
        raise ValueError("probe control claim ceiling is incomplete")


def expect_reject(name: str, mutate: Callable[[dict[str, Any]], None], source: dict[str, Any]) -> None:
    candidate = copy.deepcopy(source)
    mutate(candidate)
    try:
        validate(candidate)
    except (ValueError, KeyError):
        return
    raise ValueError(f"malicious probe control design mutation was accepted: {name}")


def self_test(payload: dict[str, Any]) -> None:
    expect_reject("preview drift", lambda value: value["preview_catalog_ref"].update(sha256="0" * 64), payload)
    expect_reject("leaf drift", lambda value: value["leaf_bindings"][0].update(atomic_pr_id="T1-M02-P999-CTR-n012-l01"), payload)
    expect_reject("append-only binding omission", lambda value: value["append_only_leaf_bindings"].pop(), payload)
    expect_reject("source drift", lambda value: value["candidate"]["source_blob_sha256"].update({"proto/traffic/v1/ingest.proto": "0" * 64}), payload)
    expect_reject("missing gap resolution", lambda value: value["append_only_gap_resolutions"].pop(), payload)
    expect_reject("false test PASS", lambda value: value["negative_tests"][0].update(execution_status="PASS"), payload)
    expect_reject("fabricated before", lambda value: next(item for item in value["function_contracts"] if item["change_kind"] == "modify").update(signature_before="func missing()"), payload)
    expect_reject("unknown state", lambda value: value["authority_state_machine"]["transitions"][0].update(to="DROPPED"), payload)
    expect_reject("false applied scope", lambda value: value["ack_acceptance_contract"].update(legacy_field="accepted means applied true"), payload)
    expect_reject("bad partition oracle", lambda value: next(item for item in value["negative_tests"] if item["case_id"] == "TC-M02-PROBE-CTRL-10").update(oracle="partition index is identical across every partition count"), payload)
    expect_reject("generation cutover omission", lambda value: next(item for item in value["function_contracts"] if item["leaf_id"] == "M02-N012-L114")["callees"].remove("ProbeAckConsumer.StartGeneration"), payload)
    expect_reject("renewal oracle drift", lambda value: next(item for item in value["negative_tests"] if item["case_id"] == "TC-M02-PROBE-CTRL-33").update(injection="single initial receipt only"), payload)
    expect_reject("offset durability oracle drift", lambda value: next(item for item in value["negative_tests"] if item["case_id"] == "TC-M02-PROBE-CTRL-34").update(oracle="commit before handler durability"), payload)
    expect_reject("processor factory omission", lambda value: next(item for item in value["function_contracts"] if item["leaf_id"] == "M02-N012-L99").update(signature_after="func (p *GenerationMessageProcessor) ProcessPartition() error"), payload)
    expect_reject("false rollout claim", lambda value: value["claims"]["forbidden"].remove("ROLLOUT_EXECUTED"), payload)


def main() -> int:
    parser = argparse.ArgumentParser()
    parser.add_argument("--self-test", action="store_true")
    args = parser.parse_args()
    payload = json.loads(DESIGN.read_text(encoding="utf-8"))
    validate(payload)
    if args.self_test:
        self_test(payload)
    print("PASS M02 probe control ACK design: 23 frozen leaves, 23 function contracts, 8 P0 plus 5 P1 findings, 31 gaps allocated in v2 preview but not implemented, 34 NOT_RUN tests")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
