#!/usr/bin/env python3
"""Semantic validator for contracts/forensics/file-restoration.v1.json."""

from __future__ import annotations

import copy
import json
import sys
from pathlib import Path


REPO = Path(__file__).resolve().parents[2]
CONTRACT = REPO / "contracts/forensics/file-restoration.v1.json"
SCHEMA = REPO / "contracts/forensics/file-restoration.v1.schema.json"
STATUSES = ["complete", "partial", "truncated", "corrupt", "oversize", "unsupported"]
PROFILES = {"http1-response-body-v1", "ftp-passive-retr-v1", "smtp-data-mime-v1"}


def require(condition: bool, code: str) -> None:
    if not condition:
        raise ValueError(code)


def validate(contract: dict) -> None:
    require(SCHEMA.is_file(), "FILE_RESTORE_SCHEMA_MISSING")
    require(contract.get("schema_version") == "1.0.0", "FILE_RESTORE_SCHEMA_VERSION")
    require(contract.get("contract_id") == "traffic.file-restoration.v1", "FILE_RESTORE_CONTRACT_ID")
    require(contract.get("requirement_id") == "REQ-T1-FILE-RESTORE-001", "FILE_RESTORE_REQUIREMENT_ID")
    require(contract.get("runtime_default") == "off", "FILE_RESTORE_DEFAULT_MUST_BE_OFF")
    require(contract.get("status") == "candidate_pending_forensics_security_acceptance_signature", "FILE_RESTORE_UNSIGNED_STATUS")

    authority = contract["authority"]
    require("never a restored file" in authority["pcap_cut_role"], "FILE_RESTORE_PCAP_CUT_SEPARATION")
    require(set(authority["required_approver_roles"]) == {"traffic-parser-feature-owner", "forensics-owner", "security-owner"}, "FILE_RESTORE_APPROVER_SET")

    profiles = contract["approved_protocol_profiles"]
    require({row["profile_id"] for row in profiles} == PROFILES, "FILE_RESTORE_PROFILE_EXACT_SET")
    require(len(profiles) == 3, "FILE_RESTORE_PROFILE_DUPLICATE")
    require(all(row["activation_status"] == "default_off_pending_approval" for row in profiles), "FILE_RESTORE_PROFILE_ACTIVATION")
    require(all(row["accepted_framing"] and row["excluded"] for row in profiles), "FILE_RESTORE_PROFILE_BOUNDARY")

    unsupported = contract["unapproved_protocol_policy"]
    require(unsupported["status"] == "unsupported" and unsupported["object_write"] == "forbidden", "FILE_RESTORE_UNAPPROVED_FAIL_CLOSED")
    require("outside approved_protocol_profiles" in unsupported["claim"], "FILE_RESTORE_UNAPPROVED_CLAIM")

    reassembly = contract["tcp_reassembly"]
    require(set(reassembly["flow_selector"]) == {"tenant_id", "probe_id", "session_id", "community_id", "flow_id", "five_tuple", "time_window"}, "FILE_RESTORE_SOURCE_SELECTOR")
    require("manifest-v2 PCAP containers" in reassembly["container_selection"] and "five-tuple filtering" in reassembly["container_selection"], "FILE_RESTORE_CONTAINER_SELECTION")
    require("never fill bytes" in reassembly["gap_policy"], "FILE_RESTORE_GAP_NO_FABRICATION")
    require("unequal bytes are corrupt" in reassembly["duplicate_policy"], "FILE_RESTORE_OVERLAP_CORRUPT")
    require("source capture timestamps only" in reassembly["event_time_policy"], "FILE_RESTORE_EVENT_TIME")

    require(contract["terminal_statuses"] == STATUSES, "FILE_RESTORE_STATUS_EXACT_ORDER")
    decision = contract["terminal_status_decision"]
    require(decision["precedence"] == ["unsupported", "oversize", "corrupt", "truncated", "partial", "complete"], "FILE_RESTORE_STATUS_PRECEDENCE")
    require(set(decision["rules"]) == set(STATUSES), "FILE_RESTORE_STATUS_RULES")
    require(set(decision["object_policy"]) == set(STATUSES), "FILE_RESTORE_STATUS_OBJECT_POLICY")
    require(decision["object_policy"]["unsupported"] == "forbidden", "FILE_RESTORE_UNSUPPORTED_OBJECT")
    require("quarantine_only" in decision["object_policy"]["corrupt"], "FILE_RESTORE_CORRUPT_QUARANTINE")

    manifest = contract["manifest"]
    required_identity = {"restoration_id", "tenant_id", "revision", "idempotency_key", "created_at", "completed_at"}
    required_source = {"session_id", "community_id", "flow_ids", "pcap_index_ids", "source_object_receipts", "packet_ranges", "tcp_sequence_ranges"}
    required_file = {"wire_filename", "sanitized_filename", "visible_size", "wire_sha256", "content_sha256"}
    required_object = {"bucket", "object_key", "object_version", "etag", "size_bytes", "sha256", "retention_until"}
    require(required_identity <= set(manifest["identity_fields"]), "FILE_RESTORE_IDENTITY_FIELDS")
    require(required_source <= set(manifest["source_fields"]), "FILE_RESTORE_SOURCE_FIELDS")
    require(required_file <= set(manifest["file_fields"]), "FILE_RESTORE_FILE_FIELDS")
    require(required_object <= set(manifest["object_fields"]), "FILE_RESTORE_OBJECT_FIELDS")
    require({"executable", "automatic_open", "automatic_decompress", "quarantined", "download_authorization_scope"} <= set(manifest["security_fields"]), "FILE_RESTORE_SECURITY_FIELDS")
    require(any("always false" in row for row in manifest["required_invariants"]), "FILE_RESTORE_INERT_INVARIANT")

    steps = contract["object_commit_protocol"]["steps"]
    require(len(steps) == 7, "FILE_RESTORE_COMMIT_STEP_COUNT")
    object_step = next((i for i, row in enumerate(steps) if "quarantine key" in row), -1)
    receipt_step = next((i for i, row in enumerate(steps) if "trusted receipt" in row), -1)
    manifest_step = next((i for i, row in enumerate(steps) if "commit restoration manifest" in row), -1)
    download_step = next((i for i, row in enumerate(steps) if "downloads eligible" in row), -1)
    require(0 <= object_step < receipt_step < manifest_step < download_step, "FILE_RESTORE_COMMIT_ORDER")
    crash = contract["object_commit_protocol"]["crash_recovery"]
    require(set(crash) == {"before_object_receipt", "after_object_receipt_before_manifest", "after_manifest_commit", "manifest_without_object"}, "FILE_RESTORE_CRASH_MATRIX")
    require("never expose" in crash["after_object_receipt_before_manifest"], "FILE_RESTORE_ORPHAN_NOT_EXPOSED")

    safety = contract["safety_limits"]
    require({"max_source_bytes", "max_stream_bytes", "max_object_bytes", "max_expansion_ratio", "task_timeout", "tenant_concurrency", "retention_duration"} <= set(safety["required_config_keys"]), "FILE_RESTORE_LIMIT_KEYS")
    require("never from an untrusted path" in safety["path_policy"], "FILE_RESTORE_PATH_POLICY")
    require("never executed" in safety["execution_policy"], "FILE_RESTORE_EXECUTION_POLICY")
    require("never automatically expand" in safety["archive_policy"], "FILE_RESTORE_ARCHIVE_POLICY")

    rollout = contract["compatibility_and_rollout"]
    require("unchanged and explicitly separate" in rollout["pcap_cut_api"], "FILE_RESTORE_LEGACY_CUT_COMPATIBILITY")
    require("idle" in rollout["consumer_first"], "FILE_RESTORE_CONSUMER_FIRST")
    require("N016" in rollout["activation_gate"] and "N017" in rollout["activation_gate"], "FILE_RESTORE_ACTIVATION_DEPENDENCIES")
    forbidden = contract["claims"]["forbidden"]
    require("PCAP cutting is file restoration" in forbidden, "FILE_RESTORE_FORBIDDEN_CUT_CLAIM")
    require("runtime activation is approved" in forbidden, "FILE_RESTORE_FORBIDDEN_ACTIVATION_CLAIM")


def expect_failure(contract: dict, code: str) -> None:
    try:
        validate(contract)
    except ValueError as exc:
        require(str(exc) == code, f"FILE_RESTORE_MUTATION_WRONG_ERROR expected={code} actual={exc}")
        return
    raise ValueError(f"FILE_RESTORE_MUTATION_NOT_REJECTED expected={code}")


def self_test(contract: dict) -> None:
    mutated = copy.deepcopy(contract)
    mutated["terminal_statuses"].remove("corrupt")
    expect_failure(mutated, "FILE_RESTORE_STATUS_EXACT_ORDER")

    mutated = copy.deepcopy(contract)
    mutated["authority"]["pcap_cut_role"] = "PCAP cut is restored output"
    expect_failure(mutated, "FILE_RESTORE_PCAP_CUT_SEPARATION")

    mutated = copy.deepcopy(contract)
    mutated["unapproved_protocol_policy"]["object_write"] = "allowed"
    expect_failure(mutated, "FILE_RESTORE_UNAPPROVED_FAIL_CLOSED")

    mutated = copy.deepcopy(contract)
    mutated["manifest"]["identity_fields"].remove("tenant_id")
    expect_failure(mutated, "FILE_RESTORE_IDENTITY_FIELDS")

    mutated = copy.deepcopy(contract)
    mutated["object_commit_protocol"]["steps"][4], mutated["object_commit_protocol"]["steps"][5] = mutated["object_commit_protocol"]["steps"][5], mutated["object_commit_protocol"]["steps"][4]
    expect_failure(mutated, "FILE_RESTORE_COMMIT_ORDER")

    mutated = copy.deepcopy(contract)
    mutated["safety_limits"]["execution_policy"] = "may execute restored content"
    expect_failure(mutated, "FILE_RESTORE_EXECUTION_POLICY")


def main() -> int:
    contract = json.loads(CONTRACT.read_text(encoding="utf-8"))
    validate(contract)
    self_test(contract)
    print(json.dumps({"result": "pass", "profiles": 3, "terminal_statuses": 6, "mutations": 6}))
    return 0


if __name__ == "__main__":
    try:
        raise SystemExit(main())
    except (OSError, json.JSONDecodeError, KeyError, TypeError, ValueError) as exc:
        print(str(exc), file=sys.stderr)
        raise SystemExit(1)
