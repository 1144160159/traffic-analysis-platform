#!/usr/bin/env python3
"""Bounded, tenant-scoped, plan-only cross-store reconciliation for T-OBS-001."""

from __future__ import annotations

import argparse
import hashlib
import json
import re
from pathlib import Path
from typing import Any


ALLOWED_SOURCES = {"postgresql", "kafka", "clickhouse", "opensearch", "nebulagraph", "minio"}
TRACE_RE = re.compile(r"^[0-9a-f]{32}$")
SHA_RE = re.compile(r"^[0-9a-f]{64}$")
FORBIDDEN_SCOPE_VALUES = {"*", "all", "any", "_all"}
DEFAULT_MAX_RECORDS = 10_000


class ReconcileInputError(ValueError):
    """Raised when the run boundary is unsafe or structurally invalid."""


def _canonical_sha256(value: Any) -> str:
    encoded = json.dumps(value, ensure_ascii=False, sort_keys=True, separators=(",", ":")).encode("utf-8")
    return hashlib.sha256(encoded).hexdigest()


def _validate_scope(payload: dict[str, Any], max_records: int) -> tuple[str, str, str]:
    tenant_id = str(payload.get("tenant_id", "")).strip()
    data_domain = str(payload.get("data_domain", "")).strip()
    authority = str(payload.get("authoritative_source", "")).strip().lower()
    if not tenant_id or tenant_id.lower() in FORBIDDEN_SCOPE_VALUES:
        raise ReconcileInputError("a concrete tenant_id is required; wildcard scope is forbidden")
    if not data_domain or data_domain.lower() in FORBIDDEN_SCOPE_VALUES:
        raise ReconcileInputError("a concrete data_domain is required; wildcard scope is forbidden")
    if authority not in ALLOWED_SOURCES:
        raise ReconcileInputError("authoritative_source must be one of the registered stores")
    if max_records < 1 or max_records > DEFAULT_MAX_RECORDS:
        raise ReconcileInputError(f"max_records must be between 1 and {DEFAULT_MAX_RECORDS}")
    return tenant_id, data_domain, authority


def _validate_watermark(source: str, watermark: Any) -> list[str]:
    if not isinstance(watermark, dict):
        return [f"{source}: watermark must be an object"]
    required = ("position_kind", "position", "observed_at", "trace_id", "state")
    errors = [f"{source}: watermark.{key} is required" for key in required if not str(watermark.get(key, "")).strip()]
    trace_id = str(watermark.get("trace_id", "")).strip()
    if trace_id and not TRACE_RE.fullmatch(trace_id):
        errors.append(f"{source}: watermark.trace_id is not a lowercase W3C trace ID")
    return errors


def _parse_records(source: str, records: Any) -> tuple[dict[str, dict[str, Any]], list[dict[str, Any]]]:
    parsed: dict[str, dict[str, Any]] = {}
    invalid: list[dict[str, Any]] = []
    if not isinstance(records, list):
        return parsed, [{"source": source, "index": None, "reason": "records must be an array"}]
    for index, raw in enumerate(records):
        reasons: list[str] = []
        if not isinstance(raw, dict):
            invalid.append({"source": source, "index": index, "reason": "record must be an object"})
            continue
        record_id = str(raw.get("record_id", "")).strip()
        trace_id = str(raw.get("trace_id", "")).strip()
        sha256 = str(raw.get("sha256", "")).strip()
        version = raw.get("version")
        if not record_id:
            reasons.append("record_id is required")
        if not isinstance(version, int) or isinstance(version, bool) or version < 0:
            reasons.append("version must be a non-negative integer")
        if not SHA_RE.fullmatch(sha256):
            reasons.append("sha256 must be 64 lowercase hexadecimal characters")
        if not TRACE_RE.fullmatch(trace_id):
            reasons.append("trace_id must be a lowercase W3C trace ID")
        if record_id in parsed:
            reasons.append("duplicate record_id")
        if reasons:
            invalid.append({"source": source, "index": index, "record_id": record_id, "reason": "; ".join(reasons)})
            continue
        parsed[record_id] = {"record_id": record_id, "version": version, "sha256": sha256, "trace_id": trace_id}
    return parsed, invalid


def reconcile(payload: dict[str, Any], max_records: int = DEFAULT_MAX_RECORDS) -> dict[str, Any]:
    tenant_id, data_domain, authority = _validate_scope(payload, max_records)
    sources = payload.get("sources")
    if not isinstance(sources, list) or not sources:
        raise ReconcileInputError("sources must be a non-empty array")

    by_source: dict[str, dict[str, dict[str, Any]]] = {}
    watermarks: dict[str, dict[str, Any]] = {}
    unparseable: list[dict[str, Any]] = []
    total_records = 0
    for entry_index, entry in enumerate(sources):
        if not isinstance(entry, dict):
            unparseable.append({"source": "unknown", "index": entry_index, "reason": "source entry must be an object"})
            continue
        source = str(entry.get("source", "")).strip().lower()
        if source not in ALLOWED_SOURCES:
            unparseable.append({"source": source or "unknown", "index": entry_index, "reason": "unregistered source"})
            continue
        if source in by_source:
            unparseable.append({"source": source, "index": entry_index, "reason": "duplicate source entry"})
            continue
        watermark = entry.get("watermark")
        watermark_errors = _validate_watermark(source, watermark)
        unparseable.extend({"source": source, "index": entry_index, "reason": error} for error in watermark_errors)
        records, invalid = _parse_records(source, entry.get("records"))
        total_records += len(entry.get("records", [])) if isinstance(entry.get("records"), list) else 0
        by_source[source] = records
        watermarks[source] = watermark if isinstance(watermark, dict) else {}
        unparseable.extend(invalid)

    if total_records > max_records:
        raise ReconcileInputError(f"input contains {total_records} records, exceeding max_records={max_records}")
    if authority not in by_source:
        raise ReconcileInputError("authoritative_source is missing from sources")

    missing: list[dict[str, Any]] = []
    extra: list[dict[str, Any]] = []
    stale_version: list[dict[str, Any]] = []
    hash_mismatch: list[dict[str, Any]] = []
    trace_mismatch: list[dict[str, Any]] = []
    authoritative = by_source[authority]

    for source in sorted(by_source):
        if source == authority:
            continue
        target = by_source[source]
        for record_id in sorted(authoritative):
            expected = authoritative[record_id]
            actual = target.get(record_id)
            if actual is None:
                missing.append({"source": source, "record_id": record_id, "expected_version": expected["version"]})
                continue
            if actual["version"] != expected["version"]:
                stale_version.append({
                    "source": source,
                    "record_id": record_id,
                    "expected_version": expected["version"],
                    "actual_version": actual["version"],
                    "direction": "behind" if actual["version"] < expected["version"] else "ahead",
                })
            elif actual["sha256"] != expected["sha256"]:
                hash_mismatch.append({"source": source, "record_id": record_id, "version": actual["version"]})
            if actual["trace_id"] != expected["trace_id"]:
                trace_mismatch.append({"source": source, "record_id": record_id})
        for record_id in sorted(set(target) - set(authoritative)):
            extra.append({"source": source, "record_id": record_id, "actual_version": target[record_id]["version"]})

    categories = {
        "missing": missing,
        "extra": extra,
        "stale_version": stale_version,
        "hash_mismatch": hash_mismatch,
        "unparseable": sorted(unparseable, key=lambda item: (str(item.get("source")), str(item.get("index")), str(item.get("record_id", "")))),
        "trace_mismatch": trace_mismatch,
    }
    counts = {key: len(value) for key, value in categories.items()}
    repair_plan = {
        "mode": "plan_only",
        "automatic_execution": False,
        "reproject": [
            {"source": item["source"], "record_id": item["record_id"], "action": "bounded_reproject_from_authority_after_approval"}
            for item in missing + stale_version + hash_mismatch + trace_mismatch
        ],
        "quarantine_review": [
            {"source": item["source"], "record_id": item["record_id"], "action": "quarantine_review_no_delete"}
            for item in extra
        ],
        "stop_reasons": ["unparseable_input"] if unparseable else [],
    }
    has_differences = any(counts.values())
    output = {
        "schema_version": 1,
        "status": "PARTIAL" if has_differences else "PASS",
        "partial": has_differences,
        "tenant_id": tenant_id,
        "data_domain": data_domain,
        "authoritative_source": authority,
        "source_watermarks": {source: watermarks[source] for source in sorted(watermarks)},
        "counts": counts,
        "differences": categories,
        "repair_plan": repair_plan,
    }
    output["report_sha256"] = _canonical_sha256(output)
    return output


def parse_args() -> argparse.Namespace:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--input", required=True, type=Path, help="Normalized JSON reconcile manifest")
    parser.add_argument("--output", type=Path, help="Optional report path; stdout is always emitted")
    parser.add_argument("--max-records", type=int, default=DEFAULT_MAX_RECORDS)
    parser.add_argument("--mode", choices=("plan",), default="plan")
    return parser.parse_args()


def main() -> int:
    args = parse_args()
    try:
        payload = json.loads(args.input.read_text(encoding="utf-8"))
        result = reconcile(payload, max_records=args.max_records)
    except (OSError, json.JSONDecodeError, ReconcileInputError) as exc:
        print(json.dumps({"status": "FAIL", "error": str(exc)}, ensure_ascii=False, indent=2))
        return 2
    rendered = json.dumps(result, ensure_ascii=False, indent=2, sort_keys=True) + "\n"
    if args.output:
        args.output.write_text(rendered, encoding="utf-8")
    print(rendered, end="")
    return 0 if result["status"] == "PASS" else 3


if __name__ == "__main__":
    raise SystemExit(main())
