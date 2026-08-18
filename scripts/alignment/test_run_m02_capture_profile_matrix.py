#!/usr/bin/env python3

from __future__ import annotations

import importlib.util
import json
from pathlib import Path


SCRIPT = Path(__file__).with_name("run_m02_capture_profile_matrix.py")
SPEC = importlib.util.spec_from_file_location("run_m02_capture_profile_matrix", SCRIPT)
assert SPEC is not None and SPEC.loader is not None
MODULE = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(MODULE)


def expect_counter_failure(profile: dict, counters: dict, code: str) -> None:
    try:
        MODULE.reconcile_counters(profile, counters)
    except MODULE.MatrixRejection as error:
        assert error.code == code, error
        return
    raise AssertionError(f"counter mutation did not fail: {code}")


def main() -> int:
    profile = json.loads(MODULE.PROFILE.read_text(encoding="utf-8"))
    exact = {
        "offered_packets": 1_000,
        "mirror_ingress_packets": 1_000,
        "nic_rx_packets": 1_000,
        "captured_packets": 1_000,
        "capture_allocation_drops": 0,
        "capture_kernel_drops": 0,
        "capture_errors": 0,
        "approved_counter_error_packets": 0,
    }
    assert MODULE.reconcile_counters(profile, exact)["unexplained_difference_packets"] == 0
    missing = dict(exact)
    missing.pop("nic_rx_packets")
    expect_counter_failure(profile, missing, "REJECT_AUTHORITATIVE_COUNTER_MISSING")
    unexplained = dict(exact)
    unexplained["captured_packets"] = 999
    expect_counter_failure(profile, unexplained, "REJECT_UNEXPLAINED_DIFF_NONZERO")
    system_drop = dict(exact)
    system_drop["captured_packets"] = 999
    system_drop["capture_kernel_drops"] = 1
    expect_counter_failure(profile, system_drop, "REJECT_CAPTURE_STOP_THRESHOLD")
    impossible = dict(exact)
    impossible["captured_packets"] = 1_001
    expect_counter_failure(profile, impossible, "REJECT_CAPTURE_COUNTER_ORDERING")
    assert set(MODULE.REQUIRED_CASES) == {
        "REALTIME_CAPTURE", "OFFLINE_REPLAY", "RESTART_RECOVERY",
        "BACKPRESSURE", "DISK_FULL", "OBJECT_FAILURE",
    }
    print("PASS M02 capture profile runner counter and case matrix self-test")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
