#!/usr/bin/env python3
from __future__ import annotations

import copy
import importlib.util
from pathlib import Path


SCRIPT = Path(__file__).with_name("run_m06_producer_rail_canary.py")
SPEC = importlib.util.spec_from_file_location("run_m06_producer_rail_canary", SCRIPT)
MODULE = importlib.util.module_from_spec(SPEC)
assert SPEC.loader is not None
SPEC.loader.exec_module(MODULE)


def expect_block(code: str, fn) -> None:
    try:
        fn()
    except MODULE.CanaryBlocked as error:
        assert error.code == code, error
        return
    raise AssertionError(f"expected {code}")


def main() -> None:
    plan = MODULE.load_plan(MODULE.DEFAULT_PLAN)
    assert plan["phases"] == MODULE.PHASES
    assert plan["profile_id"] == "M06_K8S_PRODUCER_RAIL_CANARY_V1"
    assert plan["execution_authorization"]["status"] == "PENDING_EXTERNAL_APPROVAL"
    assert plan["production_applied"] is False
    for phase in MODULE.PHASES:
        commands = MODULE.dry_run_commands(plan, phase)
        assert commands and all(command[0] == "kubectl" for command in commands)
        assert all("apply" not in command for command in commands)

    expect_block("BLOCK_TENANT_SCOPE", lambda: MODULE.validate_scope(plan, "asset-events"))

    scoped = copy.deepcopy(plan)
    scoped["scope"] = {
        "tenant_id": "tenant-canary",
        "probe_ids": ["probe-a", "probe-b"],
        "device_ip_tenant_map": {"192.0.2.10": "tenant-canary"},
    }
    assert MODULE.phase_values(scoped, "asset-events") == {
        "ASSET_EVENT_OUTBOX_TENANT_ID": "tenant-canary",
        "ASSET_EVENT_OUTBOX_ENABLED": "true",
    }
    assert MODULE.phase_values(scoped, "asset-binding-ingest")["M02_CANARY_PROBE_IDS"] == "probe-a,probe-b"
    tenant, probes, device_map = MODULE.validate_scope(scoped, "device-logs")
    assert (tenant, probes, device_map) == (
        "tenant-canary", ["probe-a", "probe-b"], {"192.0.2.10": "tenant-canary"}
    )

    duplicate = copy.deepcopy(scoped)
    duplicate["scope"]["probe_ids"] = ["probe-a", "probe-a"]
    expect_block("BLOCK_PROBE_SCOPE", lambda: MODULE.validate_scope(duplicate, "asset-binding-probe"))

    cross_tenant_device = copy.deepcopy(scoped)
    cross_tenant_device["scope"]["device_ip_tenant_map"] = {"192.0.2.10": "tenant-other"}
    expect_block("BLOCK_DEVICE_MAP_SCOPE", lambda: MODULE.validate_scope(cross_tenant_device, "device-logs"))

    expect_block(
        "BLOCK_EXTERNAL_APPROVAL_PENDING",
        lambda: MODULE.validate_execution_authority(scoped, "asset-events"),
    )
    assert MODULE.required_rbac("asset-events", "deployment/asset-service") == [
        ("get", "deployment"), ("patch", "deployment")
    ]
    assert MODULE.required_rbac("device-logs", "statefulset/device-log-collector") == [
        ("get", "statefulset"),
        ("patch", "statefulset"),
        ("get", "configmaps"),
        ("patch", "configmaps"),
        ("get", "externalsecrets.external-secrets.io"),
    ]
    print("PASS M06 producer rail canary runner self-test")


if __name__ == "__main__":
    main()
