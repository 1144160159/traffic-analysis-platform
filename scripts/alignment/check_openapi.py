#!/usr/bin/env python3
"""Dependency-free structural lint for the remediation OpenAPI source."""

from __future__ import annotations

import json
import re
from collections import Counter
from pathlib import Path


ROOT = Path(__file__).resolve().parents[2]
DOCUMENT = ROOT / "contracts/openapi/alignment-v1.openapi.json"
FEATURES = ROOT / "contracts/alignment/features"
METHODS = {"get", "post", "put", "patch", "delete"}


def main() -> int:
    document = json.loads(DOCUMENT.read_text(encoding="utf-8"))
    errors: list[str] = []
    operation_ids: list[str] = []
    feature_ids: set[str] = set()
    for path, path_item in document.get("paths", {}).items():
        placeholders = set(re.findall(r"\{([^}]+)\}", path))
        for method, operation in path_item.items():
            if method not in METHODS:
                continue
            operation_id = operation.get("operationId", "")
            operation_ids.append(operation_id)
            feature_id = operation.get("x-feature-id", "")
            feature_ids.add(feature_id)
            parameters = operation.get("parameters", [])
            declared = {
                parameter.get("name")
                for parameter in parameters
                if isinstance(parameter, dict) and "$ref" not in parameter and parameter.get("in") == "path"
            }
            if path.endswith("/{id}") or "/{id}/" in path:
                declared.add("id")
            if placeholders - declared:
                errors.append(f"{method.upper()} {path}: missing path parameters {sorted(placeholders - declared)}")
            responses = operation.get("responses", {})
            if not responses:
                errors.append(f"{operation_id}: responses are required")
    duplicates = sorted(key for key, value in Counter(operation_ids).items() if not key or value > 1)
    if duplicates:
        errors.append(f"operationId must be non-empty and unique: {duplicates}")
    contract_ids = {path.stem for path in FEATURES.glob("*.json")}
    unknown_features = sorted(feature_ids - contract_ids)
    if unknown_features:
        errors.append(f"OpenAPI feature IDs without Feature Contract: {unknown_features}")
    envelope = document.get("components", {}).get("schemas", {}).get("Envelope", {})
    if set(envelope.get("required", [])) != {"data", "meta", "error"}:
        errors.append("Envelope must require data, meta and error")
    meta_required = set(document.get("components", {}).get("schemas", {}).get("Meta", {}).get("required", []))
    expected_meta = {
        "contract_version",
        "schema_version",
        "snapshot_id",
        "as_of",
        "generated_at",
        "trace_id",
        "result_code",
        "partial",
        "missing_sections",
        "source_watermarks",
    }
    if meta_required != expected_meta:
        errors.append(f"Meta required fields differ: {sorted(meta_required ^ expected_meta)}")
    error_required = set(document.get("components", {}).get("schemas", {}).get("Error", {}).get("required", []))
    expected_error = {"code", "message", "trace_id", "retryable"}
    if error_required != expected_error:
        errors.append(f"Error required fields differ: {sorted(error_required ^ expected_error)}")
    print(json.dumps({"result": "pass" if not errors else "blocked", "operations": len(operation_ids), "errors": errors}, indent=2))
    return 1 if errors else 0


if __name__ == "__main__":
    raise SystemExit(main())
