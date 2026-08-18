#!/usr/bin/env python3
"""Build the T1-M10-N007 least-privilege role and operation policy."""

from __future__ import annotations

import argparse
import hashlib
import json
import re
from pathlib import Path
from typing import Any


ROOT = Path(__file__).resolve().parents[2]
OUTPUT = ROOT / "contracts/authz/m10-minimal-role-policy.v1.json"
OPENAPI = Path("contracts/openapi/alignment-v1.openapi.json")
SCOPES_GO = Path("go/control-plane/internal/auth/model/scopes.go")
ROLES_GO = Path("go/control-plane/internal/auth/model/user.go")
SOURCE_FILES = (
    OPENAPI,
    SCOPES_GO,
    ROLES_GO,
    Path("go/control-plane/internal/auth/service/auth_service.go"),
    Path("go/control-plane/internal/auth/middleware/auth_middleware.go"),
    Path("go/control-plane/internal/common/httpx/authorization.go"),
    Path("go/control-plane/internal/common/httpx/auth.go"),
    Path("go/control-plane/internal/common/httpx/tenant.go"),
    Path("go/control-plane/internal/asset/api/auth.go"),
)
ROLE_IDS = ("admin", "analyst", "operator", "viewer")
FIELD_CLASSES = {
    "admin": ["derived", "evidence_metadata", "identity", "operational_command", "public", "security_metadata", "tenant_bound", "workflow_annotation"],
    "analyst": ["derived", "evidence_metadata", "identity", "public", "tenant_bound", "workflow_annotation"],
    "operator": ["derived", "evidence_metadata", "identity", "operational_command", "public", "tenant_bound", "workflow_annotation"],
    "viewer": ["derived", "identity", "public", "tenant_bound"],
}
NEVER_EXPOSE_FIELDS = (
    "client_secret", "password", "password_hash", "private_key", "refresh_token",
    "secret", "token", "token_hash",
)


def sha256(path: Path) -> str:
    return hashlib.sha256(path.read_bytes()).hexdigest()


def canonical_sha256(value: Any) -> str:
    return hashlib.sha256(json.dumps(value, sort_keys=True, separators=(",", ":")).encode()).hexdigest()


def extract_braced(text: str, start: int) -> str:
    depth = 0
    for index in range(start, len(text)):
        if text[index] == "{":
            depth += 1
        elif text[index] == "}":
            depth -= 1
            if depth == 0:
                return text[start + 1:index]
    raise ValueError("unterminated Go brace block")


def parse_scope_catalog() -> tuple[dict[str, str], list[str]]:
    text = (ROOT / SCOPES_GO).read_text(encoding="utf-8")
    constants = dict(re.findall(r'^\s*(Scope[A-Za-z0-9]+)\s*=\s*"([^"]+)"', text, re.MULTILINE))
    marker = "var AllValidScopes = []string{"
    start = text.index(marker) + len(marker) - 1
    identifiers = re.findall(r"\bScope[A-Za-z0-9]+\b", extract_braced(text, start))
    scopes = [constants[name] for name in identifiers]
    if len(scopes) != len(set(scopes)):
        raise ValueError("AllValidScopes contains duplicate values")
    return constants, scopes


def parse_role_scopes(constants: dict[str, str], all_scopes: list[str]) -> dict[str, list[str]]:
    text = (ROOT / ROLES_GO).read_text(encoding="utf-8")
    roles: dict[str, list[str]] = {}
    roles["admin"] = sorted(
        scope for scope in all_scopes
        if scope != "*" and scope != "admin:cross_tenant" and not scope.endswith(":*")
    )
    for role in ("analyst", "viewer", "operator"):
        match = re.search(rf'"{role}"\s*:\s*\{{', text)
        if match is None:
            raise ValueError(f"DefaultRoleScopes is missing {role}")
        block = extract_braced(text, text.index("{", match.start()))
        identifiers = re.findall(r"\bScope[A-Za-z0-9]+\b", block)
        roles[role] = sorted({constants[name] for name in identifiers})
    if tuple(sorted(roles)) != ROLE_IDS:
        raise ValueError("role set drifted")
    return roles


def permission_allows(granted: list[str], required: str) -> bool:
    return any(
        candidate == required
        or candidate == "*"
        or (candidate.endswith(":*") and required.startswith(candidate[:-1]))
        for candidate in granted
    )


def build() -> dict[str, Any]:
    constants, all_scopes = parse_scope_catalog()
    roles = parse_role_scopes(constants, all_scopes)
    openapi = json.loads((ROOT / OPENAPI).read_text(encoding="utf-8"))
    operations: list[dict[str, Any]] = []
    for path in sorted(openapi.get("paths", {})):
        path_item = openapi["paths"][path]
        for method in ("delete", "get", "patch", "post", "put"):
            operation = path_item.get(method)
            if not isinstance(operation, dict):
                continue
            operation_id = operation.get("operationId")
            required_scope = operation.get("x-required-scope")
            if not isinstance(operation_id, str) or not operation_id:
                raise ValueError(f"{method.upper()} {path} has no operationId")
            if not isinstance(required_scope, str) or required_scope not in all_scopes:
                raise ValueError(f"{operation_id} has missing or unregistered x-required-scope")
            authorized_roles = sorted(
                role for role, granted in roles.items() if permission_allows(granted, required_scope)
            )
            if not authorized_roles:
                raise ValueError(f"{operation_id} has no authorized role")
            parameters = sorted(set(re.findall(r"\{([^{}]+)\}", path)))
            operations.append({
                "operation_id": operation_id,
                "method": method.upper(),
                "path": path,
                "action": required_scope.split(":", 1)[1],
                "required_scope": required_scope,
                "authorized_roles": authorized_roles,
                "tenant_policy": {
                    "identity_source": "VERIFIED_TOKEN_CLAIM_ONLY",
                    "request_assertion": "OPTIONAL_BUT_MUST_MATCH",
                    "repository_predicate": "REQUIRED",
                    "cross_tenant": "DENY",
                },
                "object_policy": {
                    "path_parameters": parameters,
                    "lookup": "TENANT_AND_OBJECT_ID" if parameters else "TENANT_SCOPED_COLLECTION",
                    "missing_or_cross_tenant_status": 404,
                    "existence_oracle": "FORBIDDEN",
                },
                "field_policy": {
                    "enforcement": "EXPLICIT_ALLOWLIST",
                    "role_field_classes": {role: FIELD_CLASSES[role] for role in authorized_roles},
                    "never_expose_fields": list(NEVER_EXPOSE_FIELDS),
                },
            })
    operation_ids = [item["operation_id"] for item in operations]
    if len(operation_ids) != len(set(operation_ids)):
        raise ValueError("OpenAPI operation IDs are not unique")
    role_items = [{
        "role_id": role,
        "scopes": roles[role],
        "field_classes": FIELD_CLASSES[role],
        "cross_tenant": False,
    } for role in ROLE_IDS]
    payload: dict[str, Any] = {
        "schema_version": 1,
        "artifact_kind": "M10_MINIMAL_ROLE_OPERATION_POLICY",
        "task_id": "T1-M10-N007",
        "atomic_pr_ids": ["T1-M10-P015-OPS-n007-s1", "T1-M10-P016-TST-POST-n007-s2"],
        "default_runtime_state": "off",
        "source_sha256": {str(path): sha256(ROOT / path) for path in SOURCE_FILES},
        "dependency": {
            "task_id": "T1-M10-N006",
            "required_acceptance_status": "PASS",
            "enforcement": "N007_KUBERNETES_EVIDENCE_GATE",
        },
        "runtime_enforcement": {
            "authentication": "httpx.Auth and auth middleware Authenticate",
            "tenant_binding": "httpx.ValidateRequestTenant",
            "resource_authorization": "httpx.AuthorizeResource",
            "field_allowlist": "httpx.AuthorizedFields",
            "asset_jwt_entrypoint": "asset/api.HTTPHandler.requestIdentity",
        },
        "roles": role_items,
        "operations": operations,
        "counts": {
            "roles": len(role_items),
            "registered_scopes": len(all_scopes),
            "operations": len(operations),
            "operations_without_scope": 0,
            "operations_without_authorized_role": 0,
            "roles_with_wildcards": 0,
            "roles_with_cross_tenant": 0,
        },
        "invariants": [
            "request tenant never creates identity",
            "all operations require one registered scope",
            "object lookup is tenant scoped and guessed identifiers do not disclose existence",
            "response and mutation fields require an explicit allowlist",
            "secret material is never exposed by a role field class",
        ],
    }
    payload["policy_sha256"] = canonical_sha256(payload)
    return payload


def render(value: dict[str, Any]) -> str:
    return json.dumps(value, indent=2, sort_keys=True) + "\n"


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    mode = parser.add_mutually_exclusive_group()
    mode.add_argument("--write", action="store_true")
    mode.add_argument("--check", action="store_true")
    args = parser.parse_args()
    content = render(build())
    if args.check:
        if not OUTPUT.is_file() or OUTPUT.read_text(encoding="utf-8") != content:
            print("FAIL: M10 N007 role/operation policy is stale")
            return 1
        print("PASS: M10 N007 role/operation policy is current")
        return 0
    if args.write:
        OUTPUT.parent.mkdir(parents=True, exist_ok=True)
        temporary = OUTPUT.with_name(f".{OUTPUT.name}.tmp")
        temporary.write_text(content, encoding="utf-8")
        temporary.replace(OUTPUT)
        print(OUTPUT.relative_to(ROOT))
        return 0
    print(content, end="")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
