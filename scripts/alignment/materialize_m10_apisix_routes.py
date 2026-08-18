#!/usr/bin/env python3
"""Materialize explicit T1-M10-N006 APISIX route policies deterministically."""

from __future__ import annotations

import argparse
from pathlib import Path
from typing import Any

import yaml


ROOT = Path(__file__).resolve().parents[2]
SOURCE = ROOT / "deployments/kubernetes/configmaps/apisix-routes.yaml"
OIDC_SECRET_REF = "$ENV://APISIX_OIDC_CLIENT_SECRET"
OIDC_DISCOVERY = "https://keycloak.iam.svc:8443/realms/master/.well-known/openid-configuration"


class LiteralString(str):
    pass


class Dumper(yaml.SafeDumper):
    pass


def _literal(dumper: yaml.SafeDumper, value: LiteralString):
    return dumper.represent_scalar("tag:yaml.org,2002:str", value, style="|")


Dumper.add_representer(LiteralString, _literal)


def zone(uri: str) -> str:
    if uri.startswith(("/auth", "/realms", "/resources", "/login-actions")):
        return "public_identity"
    if uri.startswith("/api/"):
        return "business_api"
    if uri.startswith("/ws"):
        return "business_websocket"
    if uri == "/*":
        return "public_web_ui"
    return "internal_management"


def body_limit(uri: str) -> int:
    if uri.startswith("/api/v1/pcap"):
        return 1_073_741_824
    if uri.startswith(("/api/v1/evidence", "/api/v1/forensics")):
        return 268_435_456
    if uri.startswith("/ws"):
        return 1_048_576
    return 10_485_760


def timeout(uri: str) -> dict[str, int]:
    if uri.startswith("/ws"):
        return {"connect": 5, "send": 60, "read": 3600}
    if uri.startswith(("/api/v1/pcap", "/api/v1/evidence", "/api/v1/forensics")):
        return {"connect": 5, "send": 120, "read": 120}
    if zone(uri) == "internal_management":
        return {"connect": 5, "send": 60, "read": 60}
    return {"connect": 5, "send": 30, "read": 30}


def rate_limit(uri: str) -> dict[str, Any]:
    rate, burst = (30, 60) if zone(uri) in {"public_identity", "internal_management"} else (100, 200)
    return {
        "rate": rate,
        "burst": burst,
        "key": "remote_addr",
        "key_type": "var",
        "nodelay": True,
        "rejected_code": 429,
        "policy": "local",
    }


def materialize_route(route: dict[str, Any]) -> dict[str, Any]:
    result = dict(route)
    uri = str(result.get("uri") or "")
    trust_zone = zone(uri)
    protected = trust_zone in {"business_api", "business_websocket", "internal_management"}
    plugins = dict(result.get("plugins") or {})
    plugins["request-id"] = {
        "header_name": "X-Request-ID",
        "include_in_response": True,
        "algorithm": "uuid",
    }
    if trust_zone != "public_web_ui":
        plugins["limit-req"] = rate_limit(uri)
    if protected:
        plugins["openid-connect"] = {
            "client_id": "traffic-ui",
            "client_secret": OIDC_SECRET_REF,
            "discovery": OIDC_DISCOVERY,
            "bearer_only": True,
            "realm": "master",
            "use_jwks": True,
            "ssl_verify": True,
        }
        plugins["client-control"] = {"max_body_size": body_limit(uri)}
        plugins["request-validation"] = {
            "header_schema": {
                "type": "object",
                "properties": {
                    "content-length": {"type": "string", "pattern": "^[0-9]{1,10}$"},
                    "content-type": {"type": "string", "maxLength": 256},
                },
                "additionalProperties": True,
            },
            "rejected_code": 400,
            "rejected_msg": "request headers violate the gateway contract",
        }
    result["plugins"] = plugins
    if result.get("upstream"):
        upstream = dict(result["upstream"])
        upstream["timeout"] = timeout(uri)
        upstream["retries"] = 0
        result["upstream"] = upstream
    return result


def materialize_document(source: Path = SOURCE) -> dict[str, Any]:
    document = yaml.safe_load(source.read_text(encoding="utf-8"))
    config = yaml.safe_load(document["data"]["apisix.yaml"])
    config["routes"] = [materialize_route(route) for route in config.get("routes") or []]
    rendered_routes = yaml.safe_dump(config, sort_keys=False, allow_unicode=True, width=120)
    document["data"]["apisix.yaml"] = LiteralString(rendered_routes)
    document["data"]["config.yaml"] = LiteralString(document["data"]["config.yaml"])
    return document


def render(document: dict[str, Any]) -> str:
    return yaml.dump(document, Dumper=Dumper, sort_keys=False, allow_unicode=True, width=120)


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--output", type=Path, default=SOURCE)
    parser.add_argument("--check", action="store_true")
    args = parser.parse_args()
    output = args.output.resolve(strict=False)
    if output not in {SOURCE.resolve(strict=False), Path("/tmp/m10-apisix-routes.yaml")}:
        raise SystemExit("output must be the route source or documented /tmp path")
    rendered = render(materialize_document(SOURCE))
    if args.check:
        if not output.is_file() or output.read_text(encoding="utf-8") != rendered:
            print("FAIL: M10 APISIX route materialization is stale")
            return 1
        print("PASS: M10 APISIX route materialization is deterministic and current")
        return 0
    output.write_text(rendered, encoding="utf-8")
    print(f"PASS: wrote {output}")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
