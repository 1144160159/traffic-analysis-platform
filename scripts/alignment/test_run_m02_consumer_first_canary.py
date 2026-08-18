#!/usr/bin/env python3

from __future__ import annotations

import copy
import importlib.util
import json
import subprocess
from pathlib import Path


SCRIPT = Path(__file__).with_name("run_m02_consumer_first_canary.py")
SPEC = importlib.util.spec_from_file_location("run_m02_consumer_first_canary", SCRIPT)
assert SPEC is not None and SPEC.loader is not None
MODULE = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(MODULE)


def expect_block(plan: dict, code: str) -> None:
    try:
        MODULE.validate_plan_semantics(plan)
    except MODULE.CanaryBlocked as error:
        assert error.code == code, error
        return
    raise AssertionError(f"mutation did not block: {code}")


def main() -> int:
    summary = MODULE.self_check()
    assert summary["status"] == "PASS"
    assert summary["default_plan_authorization"] == "PENDING_EXTERNAL_APPROVAL"
    assert summary["default_plan_production_applied"] is False
    assert [item[1] for item in summary["stage_sequence"]] == [
        "VERIFY_TOPIC_ACL",
        "VERIFY_CONSUMERS_RUNNING",
        "ENABLE_GATEWAY_WRITERS",
        "ENABLE_PROBE_PRODUCER",
        "OBSERVE_AND_RECONCILE",
    ]
    plan = MODULE.load_plan(MODULE.DEFAULT_PLAN)
    reordered = copy.deepcopy(plan)
    reordered["stages"][2], reordered["stages"][3] = reordered["stages"][3], reordered["stages"][2]
    expect_block(reordered, "BLOCK_STAGE_ORDER")
    unsafe_rollback = copy.deepcopy(plan)
    unsafe_rollback["rollback"]["steps"][0], unsafe_rollback["rollback"]["steps"][1] = (
        unsafe_rollback["rollback"]["steps"][1], unsafe_rollback["rollback"]["steps"][0]
    )
    expect_block(unsafe_rollback, "BLOCK_ROLLBACK_ORDER")
    missing_threshold = copy.deepcopy(plan)
    missing_threshold["stop_thresholds"].pop()
    expect_block(missing_threshold, "BLOCK_STOP_THRESHOLD_EXACT_SET")
    approved_without_requests = copy.deepcopy(plan)
    approved_without_requests["execution_authorization"]["status"] = "APPROVED"
    expect_block(approved_without_requests, "BLOCK_AUTHORIZATION_REQUEST_SET")
    query_drift = copy.deepcopy(plan)
    query_drift["stop_thresholds"][0]["query"] += " or vector(0)"
    expect_block(query_drift, "BLOCK_PROMQL_IDENTITY")
    scope_drift = copy.deepcopy(plan)
    scope_drift["scope"]["probe_ids"] = ["different-probe"]
    expect_block(scope_drift, "BLOCK_PROMQL_IDENTITY")
    assert MODULE.gateway_env(plan, False)[0].endswith("=false")
    assert MODULE.probe_env(plan, False)[0].endswith("=false")
    acl_catalog = json.loads((MODULE.ROOT / "contracts/events/kafka-acl-catalog.v1.json").read_text())
    expected_acls = MODULE.expected_m02_acls(acl_catalog)
    assert ("User:traffic-ingest-gateway", "WRITE") in expected_acls[("ACL_TOPIC", "flow.events.v1")]
    assert ("User:traffic-flink-session", "READ") in expected_acls[("ACL_GROUP", "flink-session-job")]
    sections = MODULE.parse_marked_sections(
        "@@TOPIC:flow.events.v1\nTopic: flow.events.v1 PartitionCount: 16\n"
        "@@ACL_GROUP:flink-session-job\n"
        "(principal=User:traffic-flink-session, host=*, operation=Read, permissionType=Allow)\n"
    )
    assert "PartitionCount: 16" in sections[("TOPIC", "flow.events.v1")]
    assert MODULE.acl_entries(sections[("ACL_GROUP", "flink-session-job")]) == {
        ("User:traffic-flink-session", "READ")
    }
    assert MODULE.denied_acl_entries(
        "(principal=User:traffic-flink-session, host=*, operation=Read, permissionType=Deny)"
    ) == {("User:traffic-flink-session", "READ")}
    lag_sections = MODULE.parse_marked_sections(
        "@@GROUP_LAG:flink-session-job\n"
        + "\n".join(
            f"flink-session-job flow.events.v1 {partition} {partition + 10} {partition + 10} 0 consumer host client"
            for partition in range(16)
        )
        + "\n@@GROUP_LAG:flink-pcap-index-job\n"
        + "\n".join(
            f"flink-pcap-index-job pcap.index.v1 {partition} {partition + 20} {partition + 20} 0 consumer host client"
            for partition in range(8)
        )
    )
    lags = MODULE.parse_consumer_group_lag(lag_sections)
    assert lags == {
        "flink-session-job": {"partition_count": 16, "total_lag": 0},
        "flink-pcap-index-job": {"partition_count": 8, "total_lag": 0},
    }
    incomplete_lag = copy.deepcopy(lag_sections)
    incomplete_lag[("GROUP_LAG", "flink-session-job")] = "flink-session-job flow.events.v1 0 10 10 0"
    try:
        MODULE.parse_consumer_group_lag(incomplete_lag)
    except MODULE.CanaryBlocked as error:
        assert error.code == "BLOCK_KAFKA_OFFSET_PARTITION_SET", error
    else:
        raise AssertionError("missing consumer partitions did not block reconciliation")

    original_kubectl = MODULE.kubectl
    before = {"metadata": {"labels": {"app": "ingest"}}, "spec": {"containers": [{"name": "ingest", "env": []}]}}
    applied = copy.deepcopy(before)
    applied["spec"]["containers"][0]["env"] = [{"name": "M02_FLOW_WRITER_V1_ENABLED", "value": "true"}]
    current = copy.deepcopy(before)
    calls: list[tuple[str, ...]] = []

    def fake_kubectl(namespace: str, *args: str, check: bool = True, input_bytes: bytes | None = None):
        nonlocal current
        del namespace, check, input_bytes
        calls.append(args)
        if args[:2] == ("get", "deployment/ingest-gateway"):
            return subprocess.CompletedProcess(args, 0, json.dumps({"spec": {"template": current}}).encode())
        if args[:3] == ("set", "env", "deployment/ingest-gateway"):
            current = copy.deepcopy(applied)
            return subprocess.CompletedProcess(args, 0, b"updated")
        if args[:2] == ("patch", "deployment/ingest-gateway"):
            patch_body = json.loads(args[args.index("-p") + 1])
            assert patch_body[0] == {"op": "test", "path": "/spec/template", "value": applied}
            assert patch_body[1] == {"op": "replace", "path": "/spec/template", "value": before}
            current = copy.deepcopy(before)
            return subprocess.CompletedProcess(args, 0, b"patched")
        if args[:3] == ("rollout", "status", "deployment/ingest-gateway"):
            return subprocess.CompletedProcess(args, 0, b"ready")
        raise AssertionError(args)

    try:
        MODULE.kubectl = fake_kubectl
        mutation = MODULE.begin_workload_env("traffic-analysis", "deployment/ingest-gateway", ["FLAG=true"])
        MODULE.wait_workload_rollout("traffic-analysis", mutation)
        restored = MODULE.rollback(plan, [mutation])
        assert restored == [{
            "resource": "deployment/ingest-gateway",
            "before_template_sha256": MODULE.canonical_sha256(before),
            "applied_template_sha256": MODULE.canonical_sha256(applied),
            "patch_exit_code": 0,
            "rollout_exit_code": 0,
            "restored_template_sha256": MODULE.canonical_sha256(before),
            "restored_exact": True,
        }]
        assert current == before
    finally:
        MODULE.kubectl = original_kubectl
    print("PASS M02 consumer-first canary runner self-test")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
