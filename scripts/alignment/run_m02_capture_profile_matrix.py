#!/usr/bin/env python3
"""Execute or preflight the M02 signed capture profile matrix fail-closed."""

from __future__ import annotations

import argparse
import datetime as dt
import hashlib
import json
import os
import subprocess
from pathlib import Path
from typing import Any

from validate_m02_capture_profile import validate as validate_profile
from validate_m02_external_activity_receipt_contract import validate_semantics as validate_receipt_semantics
from validate_m02_offline_pcap_input import validate_manifest, verify_fixture_root


ROOT = Path(__file__).resolve().parents[2]
PROFILE = ROOT / "contracts/quality/m02-approved-ten-gigabit-or-higher-profile.v1.json"
COUNTERS = ROOT / "contracts/quality/m02-capture-counter-attribution.v1.json"
SOURCE_MANIFEST = ROOT / "contracts/capture/offline-pcap-input.v1.json"
N014_RESULT = ROOT / "doc/02_acceptance/topic1/m02/n014/loopback-kafka-minio/test-result.json"
REQUIRED_CASES = (
    "REALTIME_CAPTURE",
    "OFFLINE_REPLAY",
    "RESTART_RECOVERY",
    "BACKPRESSURE",
    "DISK_FULL",
    "OBJECT_FAILURE",
)
REQUIRED_ROLES = ["PROJECT_OWNER", "TEST_OWNER", "ACCEPTANCE_AUTHORITY"]


class MatrixRejection(RuntimeError):
    def __init__(self, code: str, detail: str) -> None:
        super().__init__(f"{code}: {detail}")
        self.code = code
        self.detail = detail


def sha256_path(path: Path) -> str:
    digest = hashlib.sha256()
    with path.open("rb") as handle:
        for chunk in iter(lambda: handle.read(1024 * 1024), b""):
            digest.update(chunk)
    return digest.hexdigest()


def read_json(path: Path) -> dict[str, Any]:
    if not path.is_file():
        raise MatrixRejection("BLOCK_M02_PROFILE_INPUT_MISSING", str(path))
    return json.loads(path.read_text(encoding="utf-8"))


def parse_timestamp(value: str) -> dt.datetime:
    try:
        timestamp = dt.datetime.fromisoformat(value.replace("Z", "+00:00"))
    except ValueError as error:
        raise MatrixRejection("BLOCK_M02_PROFILE_RECEIPT_TIME_INVALID", value) from error
    if timestamp.tzinfo is None:
        raise MatrixRejection("BLOCK_M02_PROFILE_RECEIPT_TIME_INVALID", value)
    return timestamp.astimezone(dt.timezone.utc)


def input_hash_map(receipt: dict[str, Any]) -> dict[str, str]:
    return {item["artifact_id"]: item["sha256"] for item in receipt["input_hashes"]}


def require_approved_inputs(
    profile_path: Path,
    counter_path: Path,
    source_path: Path,
    receipt_path: Path,
    environment_path: Path,
    now: dt.datetime | None = None,
) -> dict[str, Any]:
    profile = read_json(profile_path)
    counters = read_json(counter_path)
    source = read_json(source_path)
    receipt = read_json(receipt_path)
    environment = read_json(environment_path)
    validate_profile(profile, counters)
    validate_manifest(source)
    validate_receipt_semantics(receipt)
    if profile["profile_status"] != "PENDING_SIGNATURE":
        raise MatrixRejection("BLOCK_M02_PROFILE_SIGNED_BODY_STATE_DRIFT", profile["profile_status"])
    if counters["method_status"] != "PENDING_SIGNATURE":
        raise MatrixRejection("BLOCK_M02_COUNTER_SIGNED_BODY_STATE_DRIFT", counters["method_status"])
    if receipt["activity_id"] != "EXT-T1-M02-N015-PROFILE-APPROVAL" or receipt["activity_type"] != "PROFILE_APPROVAL":
        raise MatrixRejection("BLOCK_M02_PROFILE_RECEIPT_IDENTITY", receipt.get("activity_id", ""))
    if receipt["result"] != "PASS" or receipt["signature_verification"]["status"] != "PASS":
        raise MatrixRejection("BLOCK_M02_PROFILE_RECEIPT_NOT_TRUSTED", receipt["result"])
    payload = receipt["activity_payload"]
    if payload["decision"] != "APPROVE":
        raise MatrixRejection("BLOCK_M02_PROFILE_RECEIPT_REJECTED", payload["decision"])
    if payload["required_authority_roles"] != REQUIRED_ROLES or profile["approval"]["required_authorities"] != REQUIRED_ROLES:
        raise MatrixRejection("BLOCK_M02_PROFILE_ROLE_QUORUM", "3-of-3 exact roles absent")
    current = now or dt.datetime.now(dt.timezone.utc)
    if not parse_timestamp(payload["valid_from"]) <= current < parse_timestamp(payload["valid_until"]):
        raise MatrixRejection("BLOCK_M02_PROFILE_RECEIPT_EXPIRED", current.isoformat())
    expected_hashes = {
        "capture-profile-contract": sha256_path(profile_path),
        "counter-contract": sha256_path(counter_path),
        "offline-source-manifest": sha256_path(source_path),
        "runtime-environment-manifest": sha256_path(environment_path),
    }
    observed_hashes = input_hash_map(receipt)
    for artifact_id, expected in expected_hashes.items():
        if observed_hashes.get(artifact_id) != expected:
            raise MatrixRejection(
                "BLOCK_M02_PROFILE_INPUT_HASH_DRIFT",
                f"{artifact_id} expected={expected} observed={observed_hashes.get(artifact_id)}",
            )
    if receipt["profile_id"] != profile["profile_id"] or payload["profile_id"] != profile["profile_id"]:
        raise MatrixRejection("BLOCK_M02_PROFILE_ID_DRIFT", profile["profile_id"])
    if receipt["candidate_manifest_sha256"] != environment.get("candidate_manifest_sha256"):
        raise MatrixRejection("BLOCK_M02_PROFILE_CANDIDATE_DRIFT", "receipt and environment differ")
    if environment.get("environment_id") != profile["environment"]["environment_id"]:
        raise MatrixRejection("BLOCK_M02_PROFILE_ENVIRONMENT_DRIFT", "profile and runtime environment differ")
    return {
        "profile": profile,
        "counters": counters,
        "source": source,
        "receipt": receipt,
        "environment": environment,
        "hashes": expected_hashes,
    }


def reconcile_counters(profile: dict[str, Any], sample: dict[str, Any]) -> dict[str, int]:
    required = {
        "offered_packets", "mirror_ingress_packets", "nic_rx_packets", "captured_packets",
        "capture_allocation_drops", "capture_kernel_drops", "capture_errors",
        "approved_counter_error_packets",
    }
    missing = sorted(required - set(sample))
    if missing:
        raise MatrixRejection("REJECT_AUTHORITATIVE_COUNTER_MISSING", ",".join(missing))
    values = {name: sample[name] for name in required}
    if any(not isinstance(value, int) or value < 0 for value in values.values()):
        raise MatrixRejection("REJECT_AUTHORITATIVE_COUNTER_INVALID", "counters must be nonnegative integers")
    observed_ingress_loss = values["offered_packets"] - values["mirror_ingress_packets"]
    nic_drop = values["mirror_ingress_packets"] - values["nic_rx_packets"]
    system_drop = values["capture_allocation_drops"] + values["capture_kernel_drops"]
    capture_difference = values["nic_rx_packets"] - values["captured_packets"] - system_drop
    unexplained = capture_difference - values["approved_counter_error_packets"]
    if min(observed_ingress_loss, nic_drop, capture_difference, unexplained) < 0:
        raise MatrixRejection("REJECT_CAPTURE_COUNTER_ORDERING", "counter deltas are not physically monotonic")
    result = {
        "observed_ingress_loss_packets": observed_ingress_loss,
        "nic_attributable_drop_packets": nic_drop,
        "system_attributable_drop_packets": system_drop,
        "capture_counter_difference_packets": capture_difference,
        "unexplained_difference_packets": unexplained,
        "capture_error_count": values["capture_errors"],
    }
    thresholds = profile["stop_thresholds"]
    for key, threshold in thresholds.items():
        if result[key] > threshold:
            code = "REJECT_UNEXPLAINED_DIFF_NONZERO" if key == "unexplained_difference_packets" else "REJECT_CAPTURE_STOP_THRESHOLD"
            raise MatrixRejection(code, f"{key}={result[key]} threshold={threshold}")
    return result


def run_command(command: list[str], env: dict[str, str] | None = None) -> dict[str, Any]:
    completed = subprocess.run(
        command, cwd=ROOT, env=env,
        stdout=subprocess.PIPE, stderr=subprocess.STDOUT, check=False,
    )
    if completed.returncode != 0:
        raise RuntimeError(
            f"matrix command failed exit={completed.returncode}: "
            + completed.stdout.decode(errors="replace")[-4000:]
        )
    return {
        "command": command,
        "exit_code": completed.returncode,
        "stdout_sha256": hashlib.sha256(completed.stdout).hexdigest(),
    }


def run_engineering_matrix(fixture_root: Path) -> list[dict[str, Any]]:
    source = read_json(SOURCE_MANIFEST)
    receipts = verify_fixture_root(source, fixture_root)
    results: list[dict[str, Any]] = [{"case": "OFFLINE_SOURCE_HASH", "receipts": receipts}]
    cargo_root = ROOT / "rust/probe-agent"
    cases = [
        ("OFFLINE_REPLAY", ["cargo", "test", "-p", "probe-agent", "--test", "pcap_offline_test"]),
        ("RESTART_RECOVERY", ["cargo", "test", "-p", "probe-agent", "archiver::spool::tests::startup_rebuilds_final_file_without_journal_and_removes_owned_temp", "--", "--exact"]),
        ("BACKPRESSURE", ["cargo", "test", "-p", "probe-agent", "sender::batch::tests::sender_respects_output_backpressure_and_drains_on_close", "--", "--exact"]),
        ("DISK_FULL", ["cargo", "test", "-p", "probe-agent", "archiver::disk_monitor::tests::cleanup_deletes_only_exact_authorized_claim", "--", "--exact"]),
        ("OBJECT_FAILURE", ["cargo", "test", "-p", "probe-agent", "archiver::upload_journal::tests::retry_wait_resumes_the_exact_durable_phase_without_regression", "--", "--exact"]),
    ]
    for name, command in cases:
        completed = subprocess.run(command, cwd=cargo_root, stdout=subprocess.PIPE, stderr=subprocess.STDOUT, check=False)
        output = completed.stdout.decode(errors="replace")
        if completed.returncode != 0:
            raise RuntimeError(f"{name} failed: " + output[-4000:])
        if name == "OFFLINE_REPLAY":
            if "test result: ok." not in output or "0 passed" in output:
                raise RuntimeError(f"{name} did not execute any tests: " + output[-4000:])
        elif "test result: ok. 1 passed" not in output:
            raise RuntimeError(f"{name} did not execute the exact target test: " + output[-4000:])
        results.append({
            "case": name,
            "command": command,
            "exit_code": completed.returncode,
            "stdout_sha256": hashlib.sha256(completed.stdout).hexdigest(),
        })
    return results


def main() -> int:
    parser = argparse.ArgumentParser()
    parser.add_argument("--run-id", required=True)
    parser.add_argument("--fixture-root", type=Path, required=True)
    parser.add_argument("--approval-receipt", type=Path)
    parser.add_argument("--environment-manifest", type=Path)
    parser.add_argument("--counter-snapshot", type=Path)
    parser.add_argument("--engineering-only", action="store_true")
    parser.add_argument("--result-output", type=Path)
    args = parser.parse_args()
    result: dict[str, Any] = {
        "schema_version": 1,
        "artifact_kind": "M02_CAPTURE_PROFILE_MATRIX_RESULT",
        "run_id": args.run_id,
        "status": "FAIL",
        "production_applied": False,
        "profile_claim": "NOT_EVALUATED",
        "cases": [],
        "input_hashes": {},
        "rejection_code": "",
        "errors": [],
    }
    try:
        if args.engineering_only:
            result["cases"] = run_engineering_matrix(args.fixture_root)
            result["status"] = "PASS"
            result["profile_claim"] = "ENGINEERING_REGRESSION_ONLY_NOT_PROFILE_APPROVAL"
        else:
            if not args.approval_receipt or not args.environment_manifest or not args.counter_snapshot:
                raise MatrixRejection("BLOCK_M02_SIGNED_PROFILE_INPUT_MISSING", "receipt environment and counters are required")
            approved = require_approved_inputs(
                PROFILE, COUNTERS, SOURCE_MANIFEST,
                args.approval_receipt.resolve(), args.environment_manifest.resolve(),
            )
            n014 = read_json(N014_RESULT)
            if n014.get("status") != "PASS" or n014.get("production_applied") is not False:
                raise MatrixRejection("BLOCK_M02_N014_LOOPBACK_NOT_PASS", n014.get("status", "MISSING"))
            counter_result = reconcile_counters(approved["profile"], read_json(args.counter_snapshot.resolve()))
            result["cases"] = run_engineering_matrix(args.fixture_root)
            result["cases"].append({"case": "REALTIME_CAPTURE", "counter_reconcile": counter_result})
            result["input_hashes"] = approved["hashes"]
            result["status"] = "PASS"
            result["profile_claim"] = "PASS_FOR_COVERED_PROFILE"
    except MatrixRejection as error:
        result["status"] = "BLOCKED" if error.code.startswith("BLOCK_") else "FAIL"
        result["rejection_code"] = error.code
        result["errors"] = [error.detail]
    except Exception as error:
        result["status"] = "FAIL"
        result["errors"] = [str(error)]
    rendered = json.dumps(result, ensure_ascii=False, indent=2, sort_keys=True) + "\n"
    if args.result_output:
        output = args.result_output.resolve()
        output.parent.mkdir(parents=True, exist_ok=True)
        temporary = output.with_name(output.name + ".tmp")
        temporary.write_text(rendered, encoding="utf-8")
        temporary.replace(output)
    print(rendered, end="")
    return 0 if result["status"] == "PASS" else 1


if __name__ == "__main__":
    raise SystemExit(main())
