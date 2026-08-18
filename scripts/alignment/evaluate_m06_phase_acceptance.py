#!/usr/bin/env python3
"""Evaluate one M06 producer phase from immutable runtime receipts."""

from __future__ import annotations

import argparse
import json
import re
from pathlib import Path
from typing import Any


ROOT = Path(__file__).resolve().parents[2]
CONTRACT_PATH = ROOT / "contracts/alignment/m06-phase-acceptance.v1.json"
SHA_RE = re.compile(r"^[0-9a-f]{64}$")


def evaluate(manifest: dict[str, Any], contract: dict[str, Any]) -> dict[str, Any]:
    errors: list[dict[str, Any]] = []
    phase = manifest.get("phase")
    phase_contract = contract.get("phases", {}).get(phase)
    if manifest.get("schema_version") != 1 or manifest.get("artifact_kind") != "M06_PHASE_ACCEPTANCE_MANIFEST":
        errors.append({"code": "MANIFEST_IDENTITY", "detail": None})
    if phase_contract is None:
        errors.append({"code": "PHASE", "detail": phase})
        phase_contract = {"canary_phases": [], "oracles": []}
    candidate_id = manifest.get("candidate_id")
    profile_id = manifest.get("profile_id")
    environment_id = manifest.get("environment_id")
    if not SHA_RE.fullmatch(str(candidate_id or "")):
        errors.append({"code": "CANDIDATE_ID", "detail": candidate_id})
    if not isinstance(profile_id, str) or not profile_id or not isinstance(environment_id, str) or not environment_id:
        errors.append({"code": "PROFILE_ENVIRONMENT", "detail": [profile_id, environment_id]})

    readiness = manifest.get("consumer_readiness", {})
    expected_binding = (candidate_id, profile_id, environment_id)
    observed_binding = (
        readiness.get("candidate_id"), readiness.get("profile_id"), readiness.get("environment_id")
    )
    if readiness.get("state") != "RUNNING" or readiness.get("observed_before_producer") is not True or observed_binding != expected_binding:
        errors.append({"code": "CONSUMER_NOT_READY_FIRST", "detail": readiness})
    if not SHA_RE.fullmatch(str(readiness.get("receipt_sha256", ""))):
        errors.append({"code": "CONSUMER_RECEIPT_HASH", "detail": readiness.get("receipt_sha256")})

    canaries = manifest.get("canary_results", [])
    if not isinstance(canaries, list):
        errors.append({"code": "CANARY_RESULT_SHAPE", "detail": type(canaries).__name__})
        canaries = []
    observed_canary_phases = [item.get("phase") for item in canaries if isinstance(item, dict)]
    if observed_canary_phases != phase_contract["canary_phases"]:
        errors.append({"code": "CANARY_PHASE_ORDER", "detail": observed_canary_phases})
    for item in canaries:
        if not isinstance(item, dict):
            errors.append({"code": "CANARY_RESULT", "detail": item})
            continue
        if (
            item.get("status") != "PASS"
            or item.get("candidate_id") != candidate_id
            or item.get("profile_id") != profile_id
            or item.get("environment_id") != environment_id
            or item.get("activation_retained_for_acceptance") is not True
            or not SHA_RE.fullmatch(str(item.get("receipt_sha256", "")))
        ):
            errors.append({"code": "CANARY_RESULT", "detail": item.get("phase")})

    source = manifest.get("source_authority", {})
    if not isinstance(source, dict):
        errors.append({"code": "SOURCE_AUTHORITY_SHAPE", "detail": type(source).__name__})
        source = {}
    if source.get("real_source") is not True or source.get("fixture") is not False or source.get("postgres_seed") is not False:
        errors.append({"code": "SOURCE_REALITY", "detail": source})
    if phase == "device-logs" and source.get("kind") != "network_device_syslog_connector":
        errors.append({"code": "DEVICE_SOURCE_AUTHORITY", "detail": source.get("kind")})
    source_tuple_sha256 = manifest.get("source_tuple_sha256")
    if not SHA_RE.fullmatch(str(source_tuple_sha256 or "")):
        errors.append({"code": "SOURCE_TUPLE_HASH", "detail": source_tuple_sha256})

    source_binding = (
        source.get("candidate_id"), source.get("profile_id"), source.get("environment_id"),
        source.get("source_tuple_sha256"),
    )
    if source_binding != (candidate_id, profile_id, environment_id, source_tuple_sha256):
        errors.append({"code": "SOURCE_AUTHORITY_BINDING", "detail": source_binding})
    if not SHA_RE.fullmatch(str(source.get("receipt_sha256", ""))):
        errors.append({"code": "SOURCE_AUTHORITY_RECEIPT_HASH", "detail": source.get("receipt_sha256")})

    oracle_receipts = manifest.get("oracle_receipts", {})
    if not isinstance(oracle_receipts, dict):
        errors.append({"code": "ORACLE_RECEIPT_SHAPE", "detail": type(oracle_receipts).__name__})
        oracle_receipts = {}
    expected_oracles = phase_contract["oracles"]
    if set(oracle_receipts) != set(expected_oracles):
        errors.append({"code": "ORACLE_EXACT_SET", "detail": sorted(oracle_receipts)})
    for oracle in expected_oracles:
        receipt = oracle_receipts.get(oracle, {})
        if (
            receipt.get("status") != "PASS"
            or receipt.get("candidate_id") != candidate_id
            or receipt.get("profile_id") != profile_id
            or receipt.get("environment_id") != environment_id
            or receipt.get("source_tuple_sha256") != source_tuple_sha256
            or not SHA_RE.fullmatch(str(receipt.get("receipt_sha256", "")))
        ):
            errors.append({"code": "ORACLE_RECEIPT", "detail": oracle})

    production_applied = manifest.get("production_applied")
    if production_applied is not True:
        errors.append({"code": "PRODUCTION_APPLIED_REQUIRED", "detail": production_applied})
    return {
        "schema_version": 1,
        "artifact_kind": "M06_PHASE_ACCEPTANCE_RECEIPT",
        "phase": phase,
        "candidate_id": candidate_id,
        "profile_id": profile_id,
        "environment_id": environment_id,
        "status": "PASS" if not errors else "FAIL",
        "source_tuple_sha256": source_tuple_sha256,
        "oracles": expected_oracles,
        "errors": errors,
        "production_applied": production_applied,
        "claim": "bounded producer rail acceptance" if not errors else None,
        "does_not_prove": ["four-source aggregate closure", "fusion gain", "attack-chain completion"],
    }


def main() -> int:
    parser = argparse.ArgumentParser()
    parser.add_argument("--manifest", type=Path, required=True)
    parser.add_argument("--output", type=Path)
    args = parser.parse_args()
    manifest_path = args.manifest.resolve(strict=True)
    if not manifest_path.is_relative_to(ROOT.resolve()):
        raise SystemExit("manifest must be inside repository")
    manifest = json.loads(manifest_path.read_text(encoding="utf-8"))
    contract = json.loads(CONTRACT_PATH.read_text(encoding="utf-8"))
    result = evaluate(manifest, contract)
    payload = json.dumps(result, sort_keys=True, indent=2) + "\n"
    if args.output:
        output = args.output.resolve()
        if output.exists():
            raise SystemExit(f"refusing to overwrite acceptance receipt: {output}")
        output.parent.mkdir(parents=True, exist_ok=True)
        output.write_text(payload, encoding="utf-8")
    print(payload, end="")
    return 0 if result["status"] == "PASS" else 1


if __name__ == "__main__":
    raise SystemExit(main())
