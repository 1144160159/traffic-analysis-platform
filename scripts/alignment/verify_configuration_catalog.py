#!/usr/bin/env python3
"""Fail-closed T-CONFIG-001 catalog and source-drift verifier."""

from __future__ import annotations

import hashlib
import json
from pathlib import Path
from typing import Any

from build_configuration_catalog import OUTPUT, ROOT, build_catalog


REQUIRED_ENTRY_FIELDS = {
    "id",
    "runtime_binding_id",
    "key",
    "consumer",
    "owner",
    "type",
    "default",
    "default_present",
    "secret_default_nonempty",
    "required",
    "secret",
    "legal_range",
    "hot_reload",
    "restart_required",
    "environment_override",
    "deprecation",
    "sources",
}
REQUIRED_SHARED_DOMAINS = {
    "kafka_topics",
    "kafka_acl",
    "flink_job_topology",
    "clickhouse_schema",
    "minio_lifecycle",
    "iam_scopes",
    "apisix_routes",
}


def _sha256(path: Path) -> str:
    return hashlib.sha256(path.read_bytes()).hexdigest()


def verify() -> dict[str, Any]:
    errors: list[str] = []
    if not OUTPUT.exists():
        return {"status": "FAIL", "errors": [f"missing {OUTPUT.relative_to(ROOT)}"]}

    actual = json.loads(OUTPUT.read_text(encoding="utf-8"))
    expected = build_catalog()
    if actual != expected:
        errors.append("configuration catalog is stale relative to governed sources")

    if actual.get("schema_version") != 1 or actual.get("control_id") != "T-CONFIG-001":
        errors.append("catalog identity must be schema v1 and T-CONFIG-001")
    if actual.get("status") != "candidate_default_off":
        errors.append("catalog rollout must remain candidate_default_off before production evidence")
    if actual.get("parse_errors"):
        errors.append("governed source parse_errors must be empty")
    conflicting_runtime_bindings = actual.get("conflicting_runtime_bindings") or []
    if conflicting_runtime_bindings:
        errors.append(
            "runtime bindings have conflicting declarations: "
            + ", ".join(conflicting_runtime_bindings)
        )

    entries = actual.get("entries") or []
    ids = [str(item.get("id", "")) for item in entries]
    if len(ids) != len(set(ids)):
        errors.append("configuration entry IDs must be unique")
    for entry in entries:
        missing = sorted(REQUIRED_ENTRY_FIELDS - set(entry))
        if missing:
            errors.append(f"{entry.get('id')}: missing metadata {missing}")
        if not entry.get("owner") or not entry.get("consumer") or not entry.get("key"):
            errors.append(f"{entry.get('id')}: owner, consumer and key are required")
        legal_range = entry.get("legal_range") or {}
        if not legal_range.get("mode") or not legal_range.get("rule"):
            errors.append(f"{entry.get('id')}: legal_range must identify validator semantics")
        if entry.get("hot_reload") not in {"unsupported", "ack_required"}:
            errors.append(f"{entry.get('id')}: invalid hot_reload policy")
        if entry.get("secret") and entry.get("default") is not None:
            errors.append(f"{entry.get('id')}: secret default was not redacted")
        if entry.get("secret_default_nonempty"):
            errors.append(f"{entry.get('id')}: non-empty secret default is forbidden")
        for source in entry.get("sources") or []:
            reference = source.get("reference") or {}
            if "value" in reference:
                errors.append(f"{entry.get('id')}: rendered values must be represented by hash")
            if entry.get("secret") and source.get("kind") == "kubernetes_literal":
                errors.append(f"{entry.get('id')}: secret-like Kubernetes env must use secretKeyRef")

    shared = actual.get("shared_authorities") or []
    domains = {str(item.get("domain", "")) for item in shared}
    if domains != REQUIRED_SHARED_DOMAINS:
        errors.append(
            f"shared authority domains drift: missing={sorted(REQUIRED_SHARED_DOMAINS-domains)} "
            f"extra={sorted(domains-REQUIRED_SHARED_DOMAINS)}"
        )
    for item in shared:
        path = ROOT / str(item.get("path", ""))
        if not path.is_file() or item.get("source_sha256") != _sha256(path):
            errors.append(f"{item.get('domain')}: authority source hash mismatch")

    precedence = actual.get("precedence") or []
    if precedence != [
        "command_line",
        "environment_variable",
        "secret_reference",
        "configmap_or_file",
        "code_or_properties_default",
    ]:
        errors.append("configuration precedence is missing or ambiguous")
    effective_hash = actual.get("effective_hash") or {}
    if "secret_value" not in (effective_hash.get("exclude") or []):
        errors.append("effective config evidence must exclude secret values")
    if effective_hash.get("algorithm") != "sha256-canonical-json-v1":
        errors.append("effective config hash algorithm is not stable")

    counts = actual.get("counts") or {}
    if counts.get("entries") != len(entries):
        errors.append("entry count drift")
    if len(entries) < 1000:
        errors.append("catalog unexpectedly covers fewer than 1000 consumer-scoped keys")

    return {
        "status": "PASS" if not errors else "FAIL",
        "control_id": "T-CONFIG-001",
        "catalog_sha256": actual.get("catalog_sha256"),
        "counts": counts,
        "shared_authority_domains": sorted(domains),
        "errors": errors,
    }


def main() -> int:
    result = verify()
    print(json.dumps(result, ensure_ascii=False, indent=2))
    return 0 if result["status"] == "PASS" else 1


if __name__ == "__main__":
    raise SystemExit(main())
