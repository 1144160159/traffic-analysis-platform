#!/usr/bin/env python3
"""Verify T1-M10-N007 policy determinism and fail-closed semantics."""

from __future__ import annotations

import copy
import json
import sys
from pathlib import Path
from typing import Any


ROOT_HINT = Path(__file__).resolve().parents[2]
if str(ROOT_HINT) not in sys.path:
    sys.path.insert(0, str(ROOT_HINT))

from scripts.alignment import build_m10_authz_policy as builder


OUTPUT = builder.OUTPUT
SCHEMA = builder.ROOT / "contracts/alignment/m10-authz-policy.schema.json"


def load(path: Path) -> dict[str, Any]:
    value = json.loads(path.read_text(encoding="utf-8"))
    if not isinstance(value, dict):
        raise ValueError("N007 policy must be an object")
    return value


def validate(expected: dict[str, Any], actual: dict[str, Any]) -> list[str]:
    errors: list[str] = []
    if actual != expected:
        errors.append("policy does not equal the current deterministic build")
    roles = actual.get("roles")
    if not isinstance(roles, list):
        return [*errors, "roles must be a list"]
    role_ids = [role.get("role_id") for role in roles if isinstance(role, dict)]
    if role_ids != list(builder.ROLE_IDS):
        errors.append("role set or order drifted")
    for role in roles:
        if not isinstance(role, dict):
            errors.append("role is not an object")
            continue
        scopes = role.get("scopes")
        if not isinstance(scopes, list) or scopes != sorted(set(scopes)):
            errors.append(f"role {role.get('role_id')} scopes are not sorted and unique")
        if isinstance(scopes, list) and any(scope == "*" or str(scope).endswith(":*") for scope in scopes):
            errors.append(f"role {role.get('role_id')} contains a wildcard")
        if (isinstance(scopes, list) and "admin:cross_tenant" in scopes) or role.get("cross_tenant") is not False:
            errors.append(f"role {role.get('role_id')} permits cross-tenant access")
    operations = actual.get("operations")
    if not isinstance(operations, list):
        return [*errors, "operations must be a list"]
    operation_ids = [item.get("operation_id") for item in operations if isinstance(item, dict)]
    if len(operation_ids) != len(set(operation_ids)):
        errors.append("operation IDs are not unique")
    for operation in operations:
        if not isinstance(operation, dict):
            errors.append("operation is not an object")
            continue
        tenant = operation.get("tenant_policy", {})
        obj = operation.get("object_policy", {})
        fields = operation.get("field_policy", {})
        if tenant != {
            "identity_source": "VERIFIED_TOKEN_CLAIM_ONLY",
            "request_assertion": "OPTIONAL_BUT_MUST_MATCH",
            "repository_predicate": "REQUIRED",
            "cross_tenant": "DENY",
        }:
            errors.append(f"{operation.get('operation_id')} tenant policy is not fail-closed")
        if obj.get("missing_or_cross_tenant_status") != 404 or obj.get("existence_oracle") != "FORBIDDEN":
            errors.append(f"{operation.get('operation_id')} object policy leaks existence")
        if fields.get("enforcement") != "EXPLICIT_ALLOWLIST":
            errors.append(f"{operation.get('operation_id')} field policy is not allowlist-only")
        never = fields.get("never_expose_fields")
        if never != list(builder.NEVER_EXPOSE_FIELDS):
            errors.append(f"{operation.get('operation_id')} secret field denylist drifted")
        if not operation.get("required_scope") or not operation.get("authorized_roles"):
            errors.append(f"{operation.get('operation_id')} lacks a scope or role")
    counts = actual.get("counts", {})
    if counts.get("roles") != len(roles) or counts.get("operations") != len(operations):
        errors.append("declared policy counts drifted")
    if any(counts.get(key) != 0 for key in (
        "operations_without_scope", "operations_without_authorized_role",
        "roles_with_wildcards", "roles_with_cross_tenant",
    )):
        errors.append("policy gap counters are non-zero")
    without_hash = copy.deepcopy(actual)
    declared_hash = without_hash.pop("policy_sha256", None)
    if declared_hash != builder.canonical_sha256(without_hash):
        errors.append("policy_sha256 is invalid")
    return errors


def main() -> int:
    if not SCHEMA.is_file() or not OUTPUT.is_file():
        print("FAIL: N007 schema or policy is absent")
        return 1
    errors = validate(builder.build(), load(OUTPUT))
    if errors:
        for error in errors:
            print(f"FAIL: {error}")
        return 1
    print("PASS: T1-M10-N007 least-privilege role and operation policy is current and fail-closed")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
