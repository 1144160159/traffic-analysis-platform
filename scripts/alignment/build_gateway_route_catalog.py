#!/usr/bin/env python3
"""Build the versioned T-GW-001 APISIX route and policy catalog."""

from __future__ import annotations

import argparse
import hashlib
import json
from pathlib import Path
from typing import Any, Iterable

import yaml


ROOT = Path(__file__).resolve().parents[2]
ROUTE_SOURCE = ROOT / "deployments/kubernetes/configmaps/apisix-routes.yaml"
OPENAPI_SOURCE = ROOT / "contracts/openapi/alignment-v1.openapi.json"
OUTPUT = ROOT / "contracts/gateway/route-catalog.v1.json"
HTTP_METHODS = {"get", "post", "put", "patch", "delete", "head", "options", "trace"}
AUTH_PLUGINS = {"openid-connect", "jwt-auth", "key-auth", "basic-auth", "hmac-auth"}
RATE_LIMIT_PLUGINS = {"limit-req", "limit-count", "limit-conn"}


def _relative(path: Path) -> str:
    return path.relative_to(ROOT).as_posix()


def _sha256(value: bytes | str) -> str:
    if isinstance(value, str):
        value = value.encode("utf-8")
    return hashlib.sha256(value).hexdigest()


def _canonical_sha256(value: Any) -> str:
    payload = json.dumps(value, ensure_ascii=False, sort_keys=True, separators=(",", ":"))
    return _sha256(payload)


def _yaml_documents(paths: Iterable[Path]) -> Iterable[tuple[Path, dict[str, Any]]]:
    for path in paths:
        try:
            documents = yaml.safe_load_all(path.read_text(encoding="utf-8"))
            for document in documents:
                if isinstance(document, dict):
                    yield path, document
        except (OSError, yaml.YAMLError):
            continue


def _routes() -> tuple[list[dict[str, Any]], str]:
    document = next(yaml.safe_load_all(ROUTE_SOURCE.read_text(encoding="utf-8")))
    rendered = document["data"]["apisix.yaml"]
    config = yaml.safe_load(rendered)
    return list(config.get("routes") or []), _sha256(rendered)


def _services() -> dict[str, dict[str, Any]]:
    paths = sorted((ROOT / "deployments/kubernetes").rglob("*.yaml"))
    paths += sorted((ROOT / "deployments/kubernetes").rglob("*.yml"))
    paths += sorted((ROOT / "go/control-plane/deployments/kubernetes").glob("*.yaml"))
    services: dict[str, dict[str, Any]] = {}
    for path, document in _yaml_documents(paths):
        if document.get("kind") != "Service":
            continue
        metadata = document.get("metadata") or {}
        name = str(metadata.get("name") or "")
        namespace = str(metadata.get("namespace") or "default")
        if not name:
            continue
        key = f"{name}.{namespace}.svc"
        entry = {
            "name": name,
            "namespace": namespace,
            "ports": sorted(
                {
                    str(port.get("port"))
                    for port in (document.get("spec") or {}).get("ports") or []
                    if port.get("port") is not None
                }
            ),
            "sources": [],
        }
        current = services.setdefault(key, entry)
        source = _relative(path)
        if source not in current["sources"]:
            current["sources"].append(source)
    for service in services.values():
        service["sources"].sort()
    return services


def _openapi_operations() -> tuple[list[dict[str, Any]], dict[str, Any]]:
    document = json.loads(OPENAPI_SOURCE.read_text(encoding="utf-8"))
    server_prefix = str((document.get("servers") or [{"url": ""}])[0].get("url") or "")
    operations: list[dict[str, Any]] = []
    for path, path_item in sorted((document.get("paths") or {}).items()):
        for method, operation in sorted(path_item.items()):
            if method.lower() not in HTTP_METHODS or not isinstance(operation, dict):
                continue
            operations.append(
                {
                    "operation_id": operation.get("operationId"),
                    "method": method.upper(),
                    "path": f"{server_prefix.rstrip('/')}/{path.lstrip('/')}",
                    "feature_id": operation.get("x-feature-id"),
                    "required_scope": operation.get("x-required-scope"),
                    "compatibility_scopes": operation.get("x-compatibility-scopes") or [],
                }
            )
    return operations, document


def _matches(uri: str, path: str) -> bool:
    if uri == "/*":
        return True
    if uri.endswith("*"):
        return path.startswith(uri[:-1])
    return path == uri


def _zone(uri: str) -> tuple[str, str, bool, str | None]:
    if uri.startswith(("/auth", "/realms", "/resources", "/login-actions")):
        return "public_identity", "identity-platform-owner", False, None
    if uri.startswith("/api/"):
        return "business_api", "security-platform-owner", True, "authenticated_tenant_claim"
    if uri.startswith("/ws"):
        return "business_websocket", "alert-domain-owner", True, "authenticated_tenant_claim"
    if uri == "/*":
        return "public_web_ui", "web-ui-owner", False, None
    return "internal_management", "security-platform-owner", True, None


def _upstream(route: dict[str, Any], services: dict[str, dict[str, Any]]) -> dict[str, Any] | None:
    upstream = route.get("upstream")
    if not upstream:
        return None
    nodes: list[dict[str, Any]] = []
    for node, weight in sorted((upstream.get("nodes") or {}).items()):
        host, separator, port = str(node).rpartition(":")
        service = services.get(host)
        nodes.append(
            {
                "endpoint": node,
                "host": host,
                "port": port if separator else None,
                "weight": weight,
                "service_declared": service is not None,
                "service_sources": (service or {}).get("sources") or [],
            }
        )
    return {
        "scheme": upstream.get("scheme", "http"),
        "type": upstream.get("type", "roundrobin"),
        "nodes": nodes,
        "timeout": upstream.get("timeout"),
        "retries": upstream.get("retries"),
    }


def build_catalog() -> dict[str, Any]:
    routes, rendered_sha256 = _routes()
    operations, openapi = _openapi_operations()
    services = _services()
    route_entries: list[dict[str, Any]] = []
    route_ids = {str(route.get("id")) for route in routes}
    operation_coverage: dict[str, list[str]] = {str(op["operation_id"]): [] for op in operations}

    for route in routes:
        route_id = str(route.get("id"))
        uri = str(route.get("uri") or "")
        methods = sorted(str(value).upper() for value in (route.get("methods") or [])) or ["ANY"]
        plugins = route.get("plugins") or {}
        zone, owner, auth_required, tenant_source = _zone(uri)
        matched_operations = [op for op in operations if _matches(uri, op["path"])]
        if uri == "/*":
            matched_operations = []
        for operation in matched_operations:
            operation_coverage[str(operation["operation_id"])].append(route_id)
        scopes = sorted({str(op["required_scope"]) for op in matched_operations if op["required_scope"]})
        missing_scopes = sorted(
            str(op["operation_id"]) for op in matched_operations if not op["required_scope"]
        )
        upstream = _upstream(route, services)
        observed_auth = sorted(AUTH_PLUGINS & set(plugins))
        observed_rate_limit = sorted(RATE_LIMIT_PLUGINS & set(plugins))
        gaps: list[str] = []
        if auth_required and not observed_auth:
            gaps.append("gateway_authentication_missing")
        if zone == "internal_management" and not observed_auth:
            gaps.append("management_entry_not_gateway_protected")
        if auth_required and missing_scopes:
            gaps.append("openapi_required_scope_missing")
        if auth_required and "request-validation" not in plugins:
            gaps.append("gateway_request_validation_missing")
        if auth_required and "client-control" not in plugins:
            gaps.append("gateway_body_limit_missing")
        if zone not in {"public_web_ui"} and not observed_rate_limit:
            gaps.append("gateway_rate_limit_missing")
        if upstream and upstream["timeout"] is None:
            gaps.append("upstream_timeout_implicit")
        if upstream and upstream["retries"] is None:
            gaps.append("upstream_retry_implicit")
        if upstream and any(not node["service_declared"] for node in upstream["nodes"]):
            gaps.append("upstream_service_not_declared")

        entry = {
            "route_id": route_id,
            "uri": uri,
            "methods": methods,
            "priority": route.get("priority"),
            "trust_zone": zone,
            "owner": owner,
            "upstream": upstream,
            "authentication": {
                "required": auth_required,
                "observed_plugins": observed_auth,
                "status": "implemented" if observed_auth else ("missing" if auth_required else "not_required"),
            },
            "authorization": {
                "enforcement": "service_operation_scope" if auth_required else "not_required",
                "required_scopes": scopes,
                "operations_missing_scope": missing_scopes,
            },
            "tenant_source": tenant_source,
            "limits": {
                "body_bytes": None,
                "field_limits": "openapi_schema" if matched_operations else None,
                "rate_limit_plugins": observed_rate_limit,
            },
            "cors": plugins.get("cors"),
            "cache": plugins.get("proxy-cache"),
            "websocket": bool(route.get("enable_websocket")),
            "audit": {
                "gateway_plugin": "http-logger" if "http-logger" in plugins else None,
                "service_business_audit_required": auth_required,
            },
            "rollback": {
                "authority": _relative(ROUTE_SOURCE),
                "strategy": "restore_previous_content_hash_and_restart_statefulset",
            },
            "openapi_operations": matched_operations,
            "plugins": plugins,
            "blocking_gaps": sorted(set(gaps)),
            "source_sha256": _canonical_sha256(route),
        }
        route_entries.append(entry)

    overlaps = {
        operation_id: ids for operation_id, ids in sorted(operation_coverage.items()) if len(ids) > 1
    }
    uncovered = sorted(operation_id for operation_id, ids in operation_coverage.items() if not ids)
    gap_counts: dict[str, int] = {}
    for route in route_entries:
        for gap in route["blocking_gaps"]:
            gap_counts[gap] = gap_counts.get(gap, 0) + 1
    missing_scope_operations = sorted(
        str(op["operation_id"]) for op in operations if not op["required_scope"]
    )
    catalog: dict[str, Any] = {
        "schema_version": 1,
        "control_id": "T-GW-001",
        "status": "candidate_default_off",
        "production_applied": False,
        "authority": {
            "route_source": _relative(ROUTE_SOURCE),
            "route_source_sha256": _sha256(ROUTE_SOURCE.read_bytes()),
            "rendered_apisix_sha256": rendered_sha256,
            "openapi_source": _relative(OPENAPI_SOURCE),
            "openapi_sha256": _sha256(OPENAPI_SOURCE.read_bytes()),
        },
        "trust_zones": {
            "public_web_ui": "static application shell only; no business API authorization",
            "public_identity": "identity protocol endpoints; authentication cannot precede login",
            "business_api": "authenticated tenant-bound REST operations",
            "business_websocket": "authenticated tenant-bound streaming session",
            "internal_management": "operator-only infrastructure administration",
        },
        "policy_contract": {
            "external_tls_boundary": "external load balancer or ingress; evidence required before G2",
            "admin_api_exposure": "ClusterIP only; external exposure forbidden",
            "tenant_identity": "trusted authenticated claim only; request tenant headers are not authoritative",
            "timeout_and_retry": "explicit per route; mutation retries require idempotency proof",
            "audit": "gateway access trace plus transactional service audit for business mutations",
            "rollout": "content-hashed shadow, internal tenant canary, atomic previous-hash rollback",
        },
        "routes": sorted(route_entries, key=lambda item: (int(item["route_id"]) if item["route_id"].isdigit() else 10**9, item["route_id"])),
        "openapi_coverage": {
            "operations": len(operations),
            "uncovered_operation_ids": uncovered,
            "overlapping_operation_routes": overlaps,
            "operations_missing_required_scope": missing_scope_operations,
        },
        "counts": {
            "routes": len(route_entries),
            "route_ids": len(route_ids),
            "protected_routes": sum(route["authentication"]["required"] for route in route_entries),
            "protected_routes_with_gateway_auth": sum(
                route["authentication"]["status"] == "implemented" for route in route_entries
            ),
            "routes_with_blocking_gaps": sum(bool(route["blocking_gaps"]) for route in route_entries),
            "declared_services": len(services),
            "gap_counts": dict(sorted(gap_counts.items())),
        },
        "acceptance": {
            "catalog_integrity_gate": "must_pass",
            "security_compliance_gate": "partial_until_blocking_gaps_empty",
            "negative_tests": [
                "undocumented_route",
                "removed_route",
                "protected_route_without_recorded_auth_gap",
                "uncovered_openapi_operation",
                "undeclared_upstream_service",
                "admin_api_external_exposure",
            ],
        },
    }
    catalog["catalog_sha256"] = _canonical_sha256(catalog)
    return catalog


def main() -> int:
    parser = argparse.ArgumentParser()
    parser.add_argument("--output", type=Path, default=OUTPUT)
    parser.add_argument("--check", action="store_true")
    args = parser.parse_args()
    catalog = build_catalog()
    if args.check:
        if not args.output.is_file():
            print(json.dumps({"status": "FAIL", "error": f"missing {args.output}"}))
            return 1
        actual = json.loads(args.output.read_text(encoding="utf-8"))
        status = "PASS" if actual == catalog else "FAIL"
        print(json.dumps({"status": status, "catalog_sha256": catalog["catalog_sha256"]}, indent=2))
        return 0 if status == "PASS" else 1
    args.output.parent.mkdir(parents=True, exist_ok=True)
    args.output.write_text(json.dumps(catalog, ensure_ascii=False, indent=2) + "\n", encoding="utf-8")
    print(json.dumps({"output": _relative(args.output.resolve()), "catalog_sha256": catalog["catalog_sha256"], "counts": catalog["counts"]}, ensure_ascii=False, indent=2))
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
