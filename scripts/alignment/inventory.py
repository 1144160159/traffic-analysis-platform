#!/usr/bin/env python3
"""Build a deterministic compatibility inventory from repository sources."""

from __future__ import annotations

import argparse
import hashlib
import json
import re
from pathlib import Path
from typing import Iterable


ROOT = Path(__file__).resolve().parents[2]


def _read(root: Path, relative: str) -> str:
    return (root / relative).read_text(encoding="utf-8")


def _sha256(root: Path, relative: str) -> str:
    return hashlib.sha256((root / relative).read_bytes()).hexdigest()


def _tree_sha256(root: Path, relative: str, pattern: str) -> str:
    digest = hashlib.sha256()
    base = root / relative
    for path in sorted(base.rglob(pattern)):
        if path.is_file() and "/vendor/" not in path.as_posix():
            digest.update(path.relative_to(root).as_posix().encode("utf-8"))
            digest.update(b"\0")
            digest.update(path.read_bytes())
            digest.update(b"\0")
    return digest.hexdigest()


def _quoted_values(value: str) -> set[str]:
    return set(re.findall(r'["\']([^"\']+)["\']', value))


def _route_section(source: str, start: str, end: str) -> list[dict[str, str]]:
    section = source.split(start, 1)[1].split(end, 1)[0]
    pattern = re.compile(
        r'makeRoute\(\s*"(?P<domain>[^"]+)"\s*,\s*"(?P<id>[^"]+)"'
        r'\s*,\s*"(?P<title>[^"]+)"\s*,\s*"(?P<path>[^"]+)"',
        re.MULTILINE,
    )
    return [match.groupdict() for match in pattern.finditer(section)]


def _go_json_fields(root: Path) -> set[str]:
    fields: set[str] = set()
    for path in (root / "go/control-plane").rglob("*.go"):
        if "/vendor/" in path.as_posix():
            continue
        for field in re.findall(r'json:"([^",]+)', path.read_text(encoding="utf-8", errors="ignore")):
            if field and field != "-":
                fields.add(field)
    return fields


def _proto_fields(root: Path) -> set[str]:
    fields: set[str] = set()
    pattern = re.compile(
        r"^\s*(?:repeated\s+|optional\s+)?[.\w<>]+\s+([a-z][a-z0-9_]*)\s*=\s*\d+",
        re.MULTILINE,
    )
    for path in (root / "proto/traffic/v1").glob("*.proto"):
        fields.update(pattern.findall(path.read_text(encoding="utf-8")))
    return fields


def _sorted(values: Iterable[str]) -> list[str]:
    return sorted(set(values))


def build_inventory(root: Path = ROOT) -> dict[str, object]:
    root = root.resolve()
    route_source = _read(root, "web/ui/src/routes/routeManifest.tsx")
    plan_source = _read(root, "web/ui/src/services/pageApiPlans.ts")
    scope_source = _read(root, "go/control-plane/internal/auth/model/scopes.go")
    openapi = json.loads(_read(root, "contracts/openapi/alignment-v1.openapi.json"))
    kafka_catalog = json.loads(_read(root, "contracts/events/kafka-topic-catalog.v1.json"))

    nav_routes = _route_section(
        route_source,
        "export const navGroups",
        "export const navRoutes",
    )
    legacy_routes = _route_section(
        route_source,
        "export const legacyTopicRoutes",
        "export const detailRoutes",
    )
    detail_routes = _route_section(
        route_source,
        "export const detailRoutes",
        "export const allRoutes",
    )

    action_ids = set(re.findall(r'^\s+id:\s*"([^"]+)"', plan_source, re.MULTILINE))
    for path in (root / "web/ui/src").rglob("*"):
        if path.suffix not in {".ts", ".tsx"}:
            continue
        source = path.read_text(encoding="utf-8", errors="ignore")
        action_ids.update(re.findall(r"""actionId:\s*["']([^"']+)["']""", source))
        action_ids.update(re.findall(r"""data-action-id=["']([^"']+)["']""", source))
    for path in (root / "go/control-plane").rglob("*.go"):
        if "/vendor/" in path.as_posix():
            continue
        source = path.read_text(encoding="utf-8", errors="ignore")
        action_ids.update(
            match.group(1)
            for match in re.finditer(
                r'^\s*[A-Za-z0-9_]*(?:Action|ActionID)\s*=\s*"([a-z0-9][a-z0-9._-]+)"',
                source,
                re.MULTILINE,
            )
        )
    audit_events = set(re.findall(r'auditEvent:\s*"([A-Z][A-Z0-9_]+)"', plan_source))

    required_ui_scopes: set[str] = set()
    accepted_ui_scopes: set[str] = set()
    combined_ui_source = route_source + "\n" + plan_source
    for raw in re.findall(
        r'requiredScopes:\s*\[([^\]]*)\]',
        combined_ui_source,
        re.MULTILINE,
    ):
        required_ui_scopes.update(_quoted_values(raw))
    for raw in re.findall(
        r'acceptedScopes:\s*\[([^\]]*)\]',
        combined_ui_source,
        re.MULTILINE,
    ):
        accepted_ui_scopes.update(_quoted_values(raw))
    ui_scopes = required_ui_scopes | accepted_ui_scopes

    go_scopes = set(re.findall(r'Scope\w+\s*=\s*"([^"]+)"', scope_source))
    endpoint_paths = set(
        re.findall(
            r'["\']((?:/api)?/v1/[^"\']*|/ws(?:/[^"\']*)?)["\']',
            plan_source,
        )
    )
    operation_pattern = re.compile(
        r'id:\s*"(?P<id>[^"]+)"[\s\S]{0,700}?'
        r'method:\s*"(?P<method>GET|POST|PUT|PATCH|DELETE)"[\s\S]{0,300}?'
        r'endpoint:\s*"(?P<endpoint>[^"]+)"'
    )
    api_operations = {
        f"{match.group('method')} {match.group('endpoint')}"
        for match in operation_pattern.finditer(plan_source)
    }
    for endpoint in re.findall(r'primary:\s*"([^"]+)"', plan_source):
        api_operations.add(f"GET {endpoint}")
    for block in re.findall(
        r'(?:secondary|pageLoadSecondary):\s*\[([^\]]*)\]',
        plan_source,
        re.MULTILINE,
    ):
        for endpoint in _quoted_values(block):
            api_operations.add(f"GET {endpoint}")

    http_methods = {"get", "post", "put", "patch", "delete", "head", "options"}
    for endpoint, path_item in openapi.get("paths", {}).items():
        if not isinstance(path_item, dict):
            continue
        for method in path_item:
            if method.lower() in http_methods:
                api_operations.add(f"{method.upper()} {endpoint}")

    for proto_path in sorted((root / "proto/traffic/v1").glob("*.proto")):
        proto_source = proto_path.read_text(encoding="utf-8")
        for service_match in re.finditer(r"service\s+(\w+)\s*\{([\s\S]*?)\n\}", proto_source):
            service_name, service_body = service_match.groups()
            for rpc_name in re.findall(r"\brpc\s+(\w+)\s*\(", service_body):
                api_operations.add(f"gRPC {service_name}.{rpc_name}")

    for topic in kafka_catalog.get("topics", []):
        topic_name = topic.get("name")
        if not topic_name:
            continue
        if topic.get("consumers"):
            api_operations.add(f"consumer {topic_name}")
        if topic.get("producers"):
            api_operations.add(f"producer {topic_name}")

    response_fields = _go_json_fields(root) | _proto_fields(root)
    route_paths = {item["path"] for item in nav_routes + legacy_routes + detail_routes}

    return {
        "schema_version": 1,
        "sources": {
            "route_manifest": {
                "path": "web/ui/src/routes/routeManifest.tsx",
                "sha256": _sha256(root, "web/ui/src/routes/routeManifest.tsx"),
            },
            "page_api_plans": {
                "path": "web/ui/src/services/pageApiPlans.ts",
                "sha256": _sha256(root, "web/ui/src/services/pageApiPlans.ts"),
            },
            "scope_catalog": {
                "path": "go/control-plane/internal/auth/model/scopes.go",
                "sha256": _sha256(root, "go/control-plane/internal/auth/model/scopes.go"),
            },
            "openapi": {
                "path": "contracts/openapi/alignment-v1.openapi.json",
                "sha256": _sha256(root, "contracts/openapi/alignment-v1.openapi.json"),
            },
            "protobuf_catalog": {
                "path": "proto/traffic/v1/*.proto",
                "sha256": _tree_sha256(root, "proto/traffic/v1", "*.proto"),
            },
            "kafka_topic_catalog": {
                "path": "contracts/events/kafka-topic-catalog.v1.json",
                "sha256": _sha256(root, "contracts/events/kafka-topic-catalog.v1.json"),
            },
            "go_action_catalog": {
                "path": "go/control-plane/**/*.go",
                "sha256": _tree_sha256(root, "go/control-plane", "*.go"),
            },
        },
        "counts": {
            "formal_nav_routes": len(nav_routes),
            "legacy_routes": len(legacy_routes),
            "detail_routes": len(detail_routes),
            "actions": len(action_ids),
            "api_operations": len(api_operations),
            "response_fields": len(response_fields),
            "ui_scopes": len(ui_scopes),
            "required_ui_scopes": len(required_ui_scopes),
            "accepted_ui_scopes": len(accepted_ui_scopes),
            "go_scopes": len(go_scopes),
            "audit_events": len(audit_events),
        },
        "formal_nav_routes": nav_routes,
        "legacy_routes": legacy_routes,
        "detail_routes": detail_routes,
        "routes": _sorted(route_paths),
        "actions": _sorted(action_ids),
        "api_paths": _sorted(endpoint_paths),
        "api_operations": _sorted(api_operations),
        "response_fields": _sorted(response_fields),
        "scopes": _sorted(ui_scopes | go_scopes),
        "ui_scopes": _sorted(ui_scopes),
        "required_ui_scopes": _sorted(required_ui_scopes),
        "accepted_ui_scopes": _sorted(accepted_ui_scopes),
        "go_scopes": _sorted(go_scopes),
        "unknown_required_ui_scopes": _sorted(required_ui_scopes - go_scopes - {"*"}),
        "unknown_accepted_ui_scopes": _sorted(accepted_ui_scopes - go_scopes - {"*"}),
        "audit_events": _sorted(audit_events),
    }


def main() -> int:
    parser = argparse.ArgumentParser()
    parser.add_argument("--output", type=Path)
    parser.add_argument("--root", type=Path, default=ROOT)
    args = parser.parse_args()
    inventory = build_inventory(args.root)
    payload = json.dumps(inventory, ensure_ascii=False, indent=2) + "\n"
    if args.output:
        args.output.parent.mkdir(parents=True, exist_ok=True)
        args.output.write_text(payload, encoding="utf-8")
    else:
        print(payload, end="")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
