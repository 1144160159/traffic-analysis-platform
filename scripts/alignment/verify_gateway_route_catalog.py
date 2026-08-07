#!/usr/bin/env python3
"""Fail closed on T-GW-001 route inventory, coverage and policy drift."""

from __future__ import annotations

import hashlib
import json
from pathlib import Path
from typing import Any

import yaml

from build_gateway_route_catalog import OUTPUT, ROOT, build_catalog


REQUIRED_ROUTE_FIELDS = {
    "route_id",
    "uri",
    "methods",
    "priority",
    "trust_zone",
    "owner",
    "upstream",
    "authentication",
    "authorization",
    "tenant_source",
    "limits",
    "cors",
    "cache",
    "websocket",
    "audit",
    "rollback",
    "openapi_operations",
    "plugins",
    "blocking_gaps",
    "source_sha256",
}


def _canonical_sha256(value: Any) -> str:
    payload = json.dumps(value, ensure_ascii=False, sort_keys=True, separators=(",", ":"))
    return hashlib.sha256(payload.encode("utf-8")).hexdigest()


def _gateway_services() -> dict[str, dict[str, Any]]:
    path = ROOT / "deployments/kubernetes/infrastructure/08-gateway.yaml"
    result: dict[str, dict[str, Any]] = {}
    for document in yaml.safe_load_all(path.read_text(encoding="utf-8")):
        if not isinstance(document, dict) or document.get("kind") != "Service":
            continue
        name = str((document.get("metadata") or {}).get("name") or "")
        result[name] = document.get("spec") or {}
    return result


def verify() -> dict[str, Any]:
    errors: list[str] = []
    if not OUTPUT.is_file():
        return {"status": "FAIL", "errors": [f"missing {OUTPUT.relative_to(ROOT)}"]}
    actual = json.loads(OUTPUT.read_text(encoding="utf-8"))
    expected = build_catalog()
    if actual != expected:
        errors.append("gateway route catalog is stale relative to APISIX, OpenAPI or Service sources")

    if actual.get("schema_version") != 1 or actual.get("control_id") != "T-GW-001":
        errors.append("catalog identity must be schema v1 and T-GW-001")
    if actual.get("status") != "candidate_default_off" or actual.get("production_applied") is not False:
        errors.append("candidate must remain default-off until production rollout evidence exists")
    content = dict(actual)
    catalog_sha256 = content.pop("catalog_sha256", None)
    if catalog_sha256 != _canonical_sha256(content):
        errors.append("catalog_sha256 does not match canonical catalog content")

    routes = actual.get("routes") or []
    ids = [str(route.get("route_id") or "") for route in routes]
    if len(ids) != len(set(ids)) or any(not route_id for route_id in ids):
        errors.append("route IDs must be non-empty and unique")
    if len(routes) < 50:
        errors.append("route inventory unexpectedly contains fewer than 50 routes")
    for route in routes:
        route_id = route.get("route_id")
        missing = sorted(REQUIRED_ROUTE_FIELDS - set(route))
        if missing:
            errors.append(f"route {route_id}: missing metadata {missing}")
        authentication = route.get("authentication") or {}
        gaps = set(route.get("blocking_gaps") or [])
        if authentication.get("required") and not authentication.get("observed_plugins"):
            if "gateway_authentication_missing" not in gaps:
                errors.append(f"route {route_id}: missing gateway auth was not fail-closed as a gap")
        if route.get("trust_zone") == "internal_management" and not authentication.get(
            "observed_plugins"
        ):
            if "management_entry_not_gateway_protected" not in gaps:
                errors.append(f"route {route_id}: unprotected management entry was not recorded")
        upstream = route.get("upstream")
        if upstream:
            for node in upstream.get("nodes") or []:
                if not node.get("service_declared"):
                    errors.append(
                        f"route {route_id}: upstream {node.get('endpoint')} has no declared Service"
                    )
        if not route.get("owner") or not (route.get("rollback") or {}).get("strategy"):
            errors.append(f"route {route_id}: owner and rollback strategy are mandatory")

    coverage = actual.get("openapi_coverage") or {}
    if coverage.get("uncovered_operation_ids"):
        errors.append(
            "OpenAPI operations are not exposed by an APISIX route: "
            + ", ".join(coverage["uncovered_operation_ids"])
        )
    if coverage.get("operations_missing_required_scope"):
        errors.append(
            "OpenAPI operations are missing x-required-scope: "
            + ", ".join(coverage["operations_missing_required_scope"])
        )
    if coverage.get("operations", 0) < 80:
        errors.append("OpenAPI operation inventory unexpectedly contains fewer than 80 operations")

    services = _gateway_services()
    public = services.get("apisix") or {}
    admin = services.get("apisix-admin") or {}
    if admin.get("type", "ClusterIP") != "ClusterIP":
        errors.append("APISIX admin API Service must remain ClusterIP")
    if any(port.get("nodePort") for port in admin.get("ports") or []):
        errors.append("APISIX admin API must not expose a nodePort")
    if any(port.get("port") == 9180 for port in public.get("ports") or []):
        errors.append("public APISIX Service must not expose the admin port")

    counts = actual.get("counts") or {}
    if counts.get("routes") != len(routes) or counts.get("route_ids") != len(set(ids)):
        errors.append("route counts drift from catalog entries")
    compliance = "PASS" if not counts.get("routes_with_blocking_gaps") else "PARTIAL"
    return {
        "status": "PASS" if not errors else "FAIL",
        "control_id": "T-GW-001",
        "catalog_integrity": "PASS" if not errors else "FAIL",
        "security_compliance": compliance,
        "catalog_sha256": actual.get("catalog_sha256"),
        "counts": counts,
        "openapi_coverage": coverage,
        "errors": errors,
    }


def main() -> int:
    result = verify()
    print(json.dumps(result, ensure_ascii=False, indent=2))
    return 0 if result["status"] == "PASS" else 1


if __name__ == "__main__":
    raise SystemExit(main())
