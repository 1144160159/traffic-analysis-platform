#!/usr/bin/env python3
"""Pure, fail-visible reconciliation for one M06 four-source real window."""

from __future__ import annotations

import argparse
import hashlib
import json
import re
from pathlib import Path
from typing import Any


ROOT = Path(__file__).resolve().parents[2]
CONTRACT_PATH = ROOT / "contracts/alignment/m06-four-source-window-reconcile.v1.json"
SHA_RE = re.compile(r"^[0-9a-f]{64}$")
TRACE_RE = re.compile(r"^[0-9a-f]{32}$")


def difference(rail: str, code: str, detail: Any) -> dict[str, Any]:
    return {"rail": rail, "code": code, "detail": detail}


def exact_identity(value: dict[str, Any]) -> tuple[Any, ...]:
    return (
        value.get("topic"), value.get("partition"), value.get("offset"),
        value.get("key_sha256"), value.get("payload_sha256"), value.get("trace_id"),
    )


def reconcile(run: dict[str, Any], contract: dict[str, Any]) -> dict[str, Any]:
    differences: list[dict[str, Any]] = []
    if run.get("schema_version") != 1 or run.get("artifact_kind") != "M06_FOUR_SOURCE_WINDOW_MANIFEST":
        differences.append(difference("run", "RUN_IDENTITY", "schema_version/artifact_kind"))
    candidate_id = run.get("candidate_id")
    profile_id = run.get("profile_id")
    environment_id = run.get("environment_id")
    tenant_id = run.get("tenant_id")
    if not isinstance(candidate_id, str) or not SHA_RE.fullmatch(candidate_id):
        differences.append(difference("run", "CANDIDATE_ID", candidate_id))
    for field, value in (("profile_id", profile_id), ("environment_id", environment_id), ("tenant_id", tenant_id)):
        if not isinstance(value, str) or not value.strip() or value == "*":
            differences.append(difference("run", "RUN_SCOPE", {field: value}))
    window = run.get("window", {})
    start_ms, end_ms = window.get("start_ms"), window.get("end_ms")
    if not isinstance(start_ms, int) or not isinstance(end_ms, int) or start_ms <= 0 or end_ms <= start_ms:
        differences.append(difference("run", "WINDOW", window))

    required_acceptance = contract["required_producer_rail_acceptance"]
    acceptance = run.get("producer_rail_acceptance", {})
    if not isinstance(acceptance, dict):
        differences.append(difference("run", "PRODUCER_RAIL_SHAPE", type(acceptance).__name__))
        acceptance = {}
    if set(acceptance) != set(required_acceptance):
        differences.append(difference("run", "PRODUCER_RAIL_EXACT_SET", sorted(acceptance)))
    for rail_id in required_acceptance:
        receipt = acceptance.get(rail_id, {})
        expected = (candidate_id, profile_id, environment_id, "PASS")
        observed = (
            receipt.get("candidate_id"), receipt.get("profile_id"),
            receipt.get("environment_id"), receipt.get("status"),
        )
        if observed != expected or not SHA_RE.fullmatch(str(receipt.get("receipt_sha256", ""))):
            differences.append(difference("run", "PRODUCER_RAIL_RECEIPT", {rail_id: observed}))

    required_rails = contract["required_rails"]
    rails = run.get("rails", {})
    if not isinstance(rails, dict):
        differences.append(difference("run", "SOURCE_RAIL_SHAPE", type(rails).__name__))
        rails = {}
    if set(rails) != set(required_rails):
        differences.append(difference("run", "SOURCE_RAIL_EXACT_SET", sorted(rails)))
    rail_results: dict[str, Any] = {}
    categories = contract["quality_categories"]
    for rail in required_rails:
        before = len(differences)
        item = rails.get(rail)
        if not isinstance(item, dict):
            differences.append(difference(rail, "MISSING_SOURCE", "rail receipt absent"))
            rail_results[rail] = {"status": "FAIL", "differences": differences[before:]}
            continue
        common = item.get("scope", {})
        observed_scope = (
            common.get("candidate_id"), common.get("profile_id"),
            common.get("environment_id"), common.get("tenant_id"),
            common.get("window_start_ms"), common.get("window_end_ms"),
        )
        expected_scope = (candidate_id, profile_id, environment_id, tenant_id, start_ms, end_ms)
        if observed_scope != expected_scope:
            differences.append(difference(rail, "SCOPE_MISMATCH", {"expected": expected_scope, "observed": observed_scope}))

        authority = item.get("source_authority", {})
        expected_kind = contract["required_real_source_kinds"][rail]
        if authority.get("kind") != expected_kind or authority.get("real_source") is not True:
            differences.append(difference(rail, "SOURCE_AUTHORITY", authority))
        if authority.get("fixture") is not False or authority.get("postgres_seed") is not False:
            differences.append(difference(rail, "SYNTHETIC_SOURCE_FORBIDDEN", authority))
        if not isinstance(authority.get("authority_id"), str) or not authority["authority_id"].strip():
            differences.append(difference(rail, "SOURCE_AUTHORITY_ID", authority.get("authority_id")))

        raw = item.get("raw_input", {})
        producer = item.get("producer_receipt", {})
        consumer = item.get("consumer_receipt", {})
        if not SHA_RE.fullmatch(str(raw.get("payload_sha256", ""))):
            differences.append(difference(rail, "RAW_PAYLOAD_HASH", raw.get("payload_sha256")))
        if raw.get("payload_sha256") != producer.get("payload_sha256"):
            differences.append(difference(rail, "RAW_PRODUCER_HASH_MISMATCH", None))
        if not TRACE_RE.fullmatch(str(raw.get("trace_id", ""))) or raw.get("trace_id") != producer.get("trace_id"):
            differences.append(difference(rail, "RAW_PRODUCER_TRACE_MISMATCH", None))
        event_time_ms = raw.get("event_time_ms")
        if not isinstance(event_time_ms, int) or not isinstance(start_ms, int) or not isinstance(end_ms, int) or not (start_ms <= event_time_ms <= end_ms):
            differences.append(difference(rail, "EVENT_OUTSIDE_WINDOW", event_time_ms))

        if producer.get("state") == "consumer_only" or producer.get("broker_ack") is not True:
            differences.append(difference(rail, "PRODUCER_NOT_ACKED", producer.get("state")))
        if producer.get("topic") != contract["canonical_topics"][rail]:
            differences.append(difference(rail, "TOPIC_MISMATCH", producer.get("topic")))
        if not isinstance(producer.get("partition"), int) or producer.get("partition") < 0 or not isinstance(producer.get("offset"), int) or producer.get("offset") < 0:
            differences.append(difference(rail, "BROKER_COORDINATES", producer))
        if not SHA_RE.fullmatch(str(producer.get("key_sha256", ""))) or not SHA_RE.fullmatch(str(producer.get("payload_sha256", ""))):
            differences.append(difference(rail, "BROKER_HASH", None))

        if consumer.get("status") != "accepted" or exact_identity(consumer) != exact_identity(producer):
            differences.append(difference(rail, "CONSUMER_RECEIPT_MISMATCH", {"producer": exact_identity(producer), "consumer": exact_identity(consumer)}))
        if consumer.get("source_version") != producer.get("offset", -2) + 1:
            differences.append(difference(rail, "SOURCE_VERSION", consumer.get("source_version")))
        watermark = consumer.get("watermark_ms")
        max_event_time = item.get("max_accepted_event_time_ms")
        if not isinstance(watermark, int) or not isinstance(max_event_time, int) or watermark < max_event_time or (isinstance(end_ms, int) and watermark > end_ms):
            differences.append(difference(rail, "WATERMARK", {"watermark_ms": watermark, "max_event_time_ms": max_event_time}))

        counts = item.get("quality_counts", {})
        if set(counts) != set(categories) or any(not isinstance(counts.get(category), int) or counts[category] < 0 for category in categories):
            differences.append(difference(rail, "QUALITY_COUNTS", counts))
        elif counts["accepted"] < 1 or sum(counts.values()) != item.get("source_record_count"):
            differences.append(difference(rail, "QUALITY_COUNT_RECONCILIATION", {"counts": counts, "source_record_count": item.get("source_record_count")}))

        targets = item.get("target_receipts", [])
        if not isinstance(targets, list):
            differences.append(difference(rail, "TARGET_RECEIPT_SHAPE", type(targets).__name__))
            targets = []
        target_names = [target.get("target") for target in targets if isinstance(target, dict)]
        required_targets = contract["required_targets"][rail]
        if target_names != required_targets or len(target_names) != len(set(target_names)):
            differences.append(difference(rail, "TARGET_EXACT_SET", target_names))
        for target in targets:
            if not isinstance(target, dict):
                differences.append(difference(rail, "TARGET_RECEIPT_SHAPE", target))
                continue
            if target.get("status") != "applied" or exact_identity(target) != exact_identity(producer):
                differences.append(difference(rail, "TARGET_SOURCE_MISMATCH", target.get("target")))
            if target.get("source_version") != consumer.get("source_version"):
                differences.append(difference(rail, "TARGET_VERSION_MISMATCH", target.get("target")))
            if not SHA_RE.fullmatch(str(target.get("projection_hash", ""))):
                differences.append(difference(rail, "TARGET_PROJECTION_HASH", target.get("target")))

        rail_results[rail] = {
            "status": "PASS" if len(differences) == before else "FAIL",
            "differences": differences[before:],
            "source_tuple": list(exact_identity(producer)[:3]),
            "watermark_ms": watermark,
        }

    production_applied = run.get("production_applied") is True
    if not production_applied:
        differences.append(difference("run", "PRODUCTION_APPLIED_REQUIRED", run.get("production_applied")))
        for rail in rail_results.values():
            rail["status"] = "FAIL"
    return {
        "schema_version": 1,
        "artifact_kind": "M06_FOUR_SOURCE_WINDOW_RECONCILIATION",
        "candidate_id": candidate_id,
        "profile_id": profile_id,
        "environment_id": environment_id,
        "tenant_id": tenant_id,
        "window": window,
        "status": "PASS" if not differences else "FAIL",
        "rail_results": rail_results,
        "differences": differences,
        "automatic_repair": False,
        "production_applied": production_applied,
        "claims": ["four-source independent ingress identity time quality and target receipts reconcile"] if not differences else [],
        "forbidden_claims": contract["forbidden_claims"],
    }


def main() -> int:
    parser = argparse.ArgumentParser()
    parser.add_argument("--manifest", type=Path, required=True)
    parser.add_argument("--output", type=Path)
    args = parser.parse_args()
    contract = json.loads(CONTRACT_PATH.read_text(encoding="utf-8"))
    manifest = args.manifest.resolve(strict=True)
    if not manifest.is_relative_to(ROOT.resolve()):
        raise SystemExit("manifest must be inside the repository")
    run = json.loads(manifest.read_text(encoding="utf-8"))
    result = reconcile(run, contract)
    payload = json.dumps(result, sort_keys=True, indent=2) + "\n"
    if args.output:
        output = args.output.resolve()
        if output.exists():
            raise SystemExit(f"refusing to overwrite reconciliation output: {output}")
        output.parent.mkdir(parents=True, exist_ok=True)
        output.write_text(payload, encoding="utf-8")
    print(payload, end="")
    return 0 if result["status"] == "PASS" else 1


if __name__ == "__main__":
    raise SystemExit(main())
