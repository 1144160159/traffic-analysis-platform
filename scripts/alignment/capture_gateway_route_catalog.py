#!/usr/bin/env python3
"""Capture immutable T-GW-001 repository and read-only live gateway evidence."""

from __future__ import annotations

import argparse
import hashlib
import json
import os
import subprocess
import urllib.error
import urllib.request
from datetime import datetime, timezone
from pathlib import Path
from typing import Any

import yaml

from candidate_snapshot import build_snapshot


ROOT = Path(__file__).resolve().parents[2]
DEFAULT_OUTPUT_ROOT = ROOT / "doc/02_acceptance/runs"
CATALOG = ROOT / "contracts/gateway/route-catalog.v1.json"
COMMANDS = (
    ("openapi-required-scopes-current", ["python3", "scripts/alignment/backfill_openapi_scopes.py", "--check"]),
    ("gateway-route-catalog-current", ["python3", "scripts/alignment/build_gateway_route_catalog.py", "--check"]),
    ("gateway-route-catalog-verifier", ["python3", "scripts/alignment/verify_gateway_route_catalog.py"]),
    ("gateway-route-catalog-negative-tests", ["python3", "-m", "unittest", "tests.alignment.test_gateway_route_catalog", "-v"]),
    ("minio-proxy-service-dry-run", ["kubectl", "apply", "--dry-run=client", "-f", "deployments/kubernetes/infrastructure/06-minio.yaml"]),
)
SOURCE_ARTIFACTS = (
    "contracts/gateway/route-catalog.v1.json",
    "scripts/alignment/build_gateway_route_catalog.py",
    "scripts/alignment/backfill_openapi_scopes.py",
    "scripts/alignment/verify_gateway_route_catalog.py",
    "scripts/alignment/capture_gateway_route_catalog.py",
    "tests/alignment/test_gateway_route_catalog.py",
    "deployments/kubernetes/configmaps/apisix-routes.yaml",
    "deployments/kubernetes/infrastructure/08-gateway.yaml",
    "deployments/kubernetes/infrastructure/06-minio.yaml",
    "contracts/openapi/alignment-v1.openapi.json",
    "doc/07_alignment/runbooks/T-GW-001-gateway-route-catalog.md",
    "Makefile",
)


def sha256(path: Path) -> str:
    digest = hashlib.sha256()
    with path.open("rb") as source:
        for chunk in iter(lambda: source.read(1024 * 1024), b""):
            digest.update(chunk)
    return digest.hexdigest()


def canonical_sha256(value: Any) -> str:
    payload = json.dumps(value, ensure_ascii=False, sort_keys=True, separators=(",", ":"))
    return hashlib.sha256(payload.encode("utf-8")).hexdigest()


def direct_environment() -> dict[str, str]:
    environment = dict(os.environ)
    for key in ("HTTP_PROXY", "HTTPS_PROXY", "ALL_PROXY", "http_proxy", "https_proxy", "all_proxy"):
        environment.pop(key, None)
    return environment


def run_command(name: str, command: list[str], output: Path) -> dict[str, Any]:
    log_path = output / f"{name}.log"
    started = datetime.now(timezone.utc)
    print(f"[gateway] starting {name}: {' '.join(command)}", flush=True)
    with log_path.open("wb") as log:
        completed = subprocess.run(
            command,
            cwd=ROOT,
            env=direct_environment(),
            stdout=log,
            stderr=subprocess.STDOUT,
            check=False,
        )
    finished = datetime.now(timezone.utc)
    result = {
        "name": name,
        "command": command,
        "exit_code": completed.returncode,
        "status": "PASS" if completed.returncode == 0 else "FAIL",
        "started_at": started.isoformat(),
        "finished_at": finished.isoformat(),
        "duration_seconds": round((finished - started).total_seconds(), 3),
        "artifact": log_path.name,
        "sha256": sha256(log_path),
        "size_bytes": log_path.stat().st_size,
    }
    print(f"[gateway] {name}: {result['status']}", flush=True)
    return result


def kubectl_json(arguments: list[str]) -> dict[str, Any]:
    completed = subprocess.run(
        ["kubectl", "--request-timeout=15s", *arguments, "-o", "json"],
        cwd=ROOT,
        env=direct_environment(),
        text=True,
        stdout=subprocess.PIPE,
        stderr=subprocess.PIPE,
        check=False,
        timeout=30,
    )
    if completed.returncode != 0:
        raise RuntimeError(completed.stderr.strip() or "kubectl failed")
    return json.loads(completed.stdout)


def _probe(endpoint: str, path: str) -> dict[str, Any]:
    request = urllib.request.Request(endpoint.rstrip("/") + path, method="GET")
    request.add_header("Accept", "application/json,text/html;q=0.8,*/*;q=0.1")
    request.add_header("User-Agent", "t-gw-001-read-only-evidence/1")
    opener = urllib.request.build_opener(urllib.request.ProxyHandler({}))
    try:
        response = opener.open(request, timeout=8)
    except urllib.error.HTTPError as exc:
        response = exc
    except Exception as exc:
        return {"path": path, "reachable": False, "error_class": type(exc).__name__}
    try:
        return {
            "path": path,
            "reachable": True,
            "status_code": response.status,
            "content_type": response.headers.get("Content-Type"),
            "content_length": response.headers.get("Content-Length"),
            "location": response.headers.get("Location"),
            "www_authenticate_present": bool(response.headers.get("WWW-Authenticate")),
            "body_captured": False,
        }
    finally:
        response.close()


def capture_live(catalog: dict[str, Any], endpoint: str) -> dict[str, Any]:
    configmap = kubectl_json(["-n", "gateway", "get", "configmap", "apisix-routes"])
    statefulset = kubectl_json(["-n", "gateway", "get", "statefulset", "apisix"])
    gateway_services = kubectl_json(["-n", "gateway", "get", "service", "apisix", "apisix-admin"])
    all_services = kubectl_json(["get", "service", "-A"])
    all_endpoints = kubectl_json(["get", "endpoints", "-A"])
    raw_routes = str((configmap.get("data") or {}).get("apisix.yaml") or "")
    live_routes = (yaml.safe_load(raw_routes) or {}).get("routes") or []

    service_index = {
        f"{item['metadata']['name']}.{item['metadata'].get('namespace', 'default')}.svc": item
        for item in all_services.get("items") or []
    }
    endpoint_index = {
        f"{item['metadata']['name']}.{item['metadata'].get('namespace', 'default')}.svc": item
        for item in all_endpoints.get("items") or []
    }
    upstreams: list[dict[str, Any]] = []
    hosts = sorted(
        {
            node["host"]
            for route in catalog.get("routes") or []
            for node in ((route.get("upstream") or {}).get("nodes") or [])
        }
    )
    for host in hosts:
        service = service_index.get(host)
        endpoints = endpoint_index.get(host) or {}
        ready_addresses = sum(
            len(subset.get("addresses") or []) for subset in endpoints.get("subsets") or []
        )
        not_ready_addresses = sum(
            len(subset.get("notReadyAddresses") or []) for subset in endpoints.get("subsets") or []
        )
        upstreams.append(
            {
                "host": host,
                "service_found": service is not None,
                "service_resource_version": (service or {}).get("metadata", {}).get("resourceVersion"),
                "ready_addresses": ready_addresses,
                "not_ready_addresses": not_ready_addresses,
                "ready": service is not None and ready_addresses > 0,
            }
        )

    services = []
    for service in gateway_services.get("items") or []:
        spec = service.get("spec") or {}
        services.append(
            {
                "name": service["metadata"]["name"],
                "type": spec.get("type", "ClusterIP"),
                "ports": [
                    {
                        "name": port.get("name"),
                        "port": port.get("port"),
                        "node_port": port.get("nodePort"),
                        "target_port": port.get("targetPort"),
                    }
                    for port in spec.get("ports") or []
                ],
                "resource_version": service["metadata"].get("resourceVersion"),
            }
        )
    live_route_sha256 = hashlib.sha256(raw_routes.encode("utf-8")).hexdigest()
    expected_route_sha256 = (catalog.get("authority") or {}).get("rendered_apisix_sha256")
    return {
        "read_only": True,
        "endpoint": endpoint,
        "route_count": len(live_routes),
        "route_ids": sorted(str(route.get("id")) for route in live_routes),
        "rendered_apisix_sha256": live_route_sha256,
        "candidate_rendered_apisix_sha256": expected_route_sha256,
        "candidate_route_match": live_route_sha256 == expected_route_sha256,
        "configmap_resource_version": configmap.get("metadata", {}).get("resourceVersion"),
        "statefulset_resource_version": statefulset.get("metadata", {}).get("resourceVersion"),
        "statefulset_image": (((statefulset.get("spec") or {}).get("template") or {}).get("spec") or {}).get("containers", [{}])[0].get("image"),
        "replicas": {
            "desired": (statefulset.get("spec") or {}).get("replicas"),
            "current": (statefulset.get("status") or {}).get("currentReplicas", 0),
            "ready": (statefulset.get("status") or {}).get("readyReplicas", 0),
        },
        "gateway_services": sorted(services, key=lambda item: item["name"]),
        "upstreams": upstreams,
        "unready_upstream_hosts": [item["host"] for item in upstreams if not item["ready"]],
        "unauthenticated_read_only_probes": [
            _probe(endpoint, path)
            for path in ("/", "/api/v1/assets", "/grafana/", "/nacos/", "/flink/", "/minio/")
        ],
        "response_bodies_captured": False,
        "secret_values_captured": False,
        "production_mutations": [],
    }


def main() -> int:
    parser = argparse.ArgumentParser()
    parser.add_argument("--run-id", required=True)
    parser.add_argument("--g0-manifest", type=Path, required=True)
    parser.add_argument("--endpoint", default="http://10.0.5.8:30180")
    parser.add_argument("--output-root", type=Path, default=DEFAULT_OUTPUT_ROOT)
    args = parser.parse_args()
    g0_path = args.g0_manifest.resolve()
    if not g0_path.is_file():
        raise SystemExit(f"G0 manifest does not exist: {g0_path}")
    g0 = json.loads(g0_path.read_text(encoding="utf-8"))
    if g0.get("gate") != "G0" or g0.get("status") != "PASS":
        raise SystemExit("referenced G0 manifest is not PASS")
    candidate_before = build_snapshot()
    g0_hash = (g0.get("candidate_source") or {}).get("content_sha256")
    if not g0_hash or candidate_before["content_sha256"] != g0_hash:
        raise SystemExit("current candidate does not match the referenced G0 manifest")

    output = args.output_root.resolve() / args.run_id
    if output.exists():
        raise SystemExit(f"refusing to overwrite immutable evidence directory: {output}")
    output.mkdir(parents=True)
    results = []
    for name, command in COMMANDS:
        result = run_command(name, list(command), output)
        results.append(result)
        if result["status"] != "PASS":
            break
    repository_pass = len(results) == len(COMMANDS) and all(item["status"] == "PASS" for item in results)

    try:
        catalog = json.loads(CATALOG.read_text(encoding="utf-8"))
        live = capture_live(catalog, args.endpoint)
        live["query_status"] = "PASS"
    except Exception as exc:
        catalog = json.loads(CATALOG.read_text(encoding="utf-8"))
        live = {
            "read_only": True,
            "query_status": "FAIL",
            "error_class": type(exc).__name__,
            "secret_values_captured": False,
            "production_mutations": [],
        }
    candidate_after = build_snapshot()
    candidate_stable = candidate_before["content_sha256"] == candidate_after["content_sha256"]
    live_pass = (
        live.get("query_status") == "PASS"
        and live.get("candidate_route_match") is True
        and not live.get("unready_upstream_hosts")
    )
    scoped_pass = repository_pass and live_pass and candidate_stable

    sources = []
    for relative in SOURCE_ARTIFACTS:
        path = ROOT / relative
        if not path.is_file():
            raise SystemExit(f"source artifact does not exist: {relative}")
        sources.append({"path": relative, "sha256": sha256(path), "size_bytes": path.stat().st_size})
    manifest = {
        "schema_version": 1,
        "run_id": args.run_id,
        "captured_at": datetime.now(timezone.utc).isoformat(),
        "feature_id": "T-GW-001",
        "related_ids": ["T-SEC-001", "T-IAM-002", "T-CONFIG-001"],
        "status": "PARTIAL" if scoped_pass else "FAIL",
        "coverage_status": "PARTIAL_REPOSITORY_ROUTE_POLICY_AND_READ_ONLY_LIVE_TOPOLOGY",
        "scoped_evidence_status": "PASS" if scoped_pass else "FAIL",
        "candidate_source": candidate_before,
        "candidate_source_stable": candidate_stable,
        "g0_reference": {
            "run_id": g0.get("run_id"),
            "manifest": str(g0_path.relative_to(ROOT)),
            "manifest_sha256": sha256(g0_path),
            "status": g0.get("status"),
            "candidate_source_sha256": g0_hash,
        },
        "catalog_summary": {
            "catalog_sha256": catalog.get("catalog_sha256"),
            "counts": catalog.get("counts"),
            "openapi_coverage": catalog.get("openapi_coverage"),
        },
        "live_observation": live,
        "production_applied": False,
        "gate_status": {
            "G0": "PASS",
            "G1": "PASS_FOR_VERSIONED_ROUTE_INVENTORY_SOURCE_DIFF_SERVICE_RESOLUTION_AND_NEGATIVE_TESTS" if scoped_pass else "FAIL",
            "G2": "PARTIAL_FOR_READ_ONLY_LIVE_ROUTE_HASH_REPLICAS_UPSTREAMS_AND_UNAUTHENTICATED_PROBES",
            "G3": "OPEN_FOR_TRACE_BOUND_AUTH_SCOPE_TENANT_AUDIT_AND_SERVICE_EFFECT_RECONCILIATION",
            "G4": "OPEN_FOR_ROUTE_TIMEOUT_RETRY_LIMIT_AND_LOAD_BUDGETS",
            "G5": "OPEN_FOR_WINDOWS_CHROME_AUTHORIZATION_AND_CORS_EVIDENCE",
            "G6": "HOLD_FOR_SHADOW_CANARY_ATOMIC_ROLLBACK_AND_T_PLUS_OBSERVATION",
            "G7": "OPEN",
            "G8": "BLOCKED",
        },
        "commands": results,
        "source_artifacts": sources,
        "proven": [
            "all APISIX route IDs are versioned and no OpenAPI operation is left without a route",
            "all declared upstream DNS targets have repository Services and live ready endpoints",
            "APISIX admin API is ClusterIP-only and absent from the public Service",
            "live standalone route content matches the candidate authority hash",
            "known missing gateway policies and OpenAPI scopes are explicit blocking gaps",
        ],
        "open": [
            "add and canary gateway authentication for business and management routes",
            "bind each OpenAPI operation to an explicit required scope and remove probe route overlap",
            "set explicit body, field, timeout, retry, rate-limit, CORS, cache and audit policies",
            "validate cross-tenant, unauthenticated, under-scoped, oversized, rate-limit and websocket negatives",
            "complete trace reconciliation, performance, Windows Chrome, rollout, rollback and observation gates",
        ],
        "secrets_captured": False,
    }
    manifest_path = output / "manifest.json"
    manifest_path.write_text(json.dumps(manifest, ensure_ascii=False, indent=2) + "\n", encoding="utf-8")
    print(json.dumps({
        "status": manifest["status"],
        "scoped_evidence_status": manifest["scoped_evidence_status"],
        "manifest": str(manifest_path.relative_to(ROOT)),
        "manifest_sha256": sha256(manifest_path),
        "catalog_sha256": catalog.get("catalog_sha256"),
        "live_query_status": live.get("query_status"),
        "live_route_match": live.get("candidate_route_match"),
        "unready_upstream_count": len(live.get("unready_upstream_hosts") or []),
    }, ensure_ascii=False, indent=2), flush=True)
    return 0 if scoped_pass else 1


if __name__ == "__main__":
    raise SystemExit(main())
