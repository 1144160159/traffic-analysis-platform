#!/usr/bin/env python3
"""Capture read-only T1-M10-N006 APISIX candidate/live route and workload diff."""

from __future__ import annotations

import argparse
import hashlib
import json
import os
import subprocess
from pathlib import Path
from typing import Any

import yaml


ROOT = Path(__file__).resolve().parents[2]
OUTPUT = ROOT / "doc/02_acceptance/topic1/tasks/t1-m10-n006/k8s-apisix-route-diff-latest.json"
ROUTE_SOURCE = Path("deployments/kubernetes/configmaps/apisix-routes.yaml")
GATEWAY_SOURCE = Path("deployments/kubernetes/infrastructure/08-gateway.yaml")
CATALOG = Path("contracts/gateway/route-catalog.v1.json")
N001 = Path("deployments/releases/topic1/m10-deployable-candidate-closure.v1.json")
N005 = Path("doc/02_acceptance/topic1/tasks/t1-m10-n005/k8s-approved-additive-plan-latest.json")
SOURCE_FILES = (
    ROUTE_SOURCE, GATEWAY_SOURCE, CATALOG, N001, N005,
    Path("contracts/alignment/m10-apisix-route-diff.schema.json"),
    Path("scripts/alignment/materialize_m10_apisix_routes.py"),
    Path("scripts/alignment/build_gateway_route_catalog.py"),
    Path("scripts/alignment/verify_gateway_route_catalog.py"),
    Path("scripts/alignment/capture_m10_apisix_route_diff.py"),
    Path("scripts/alignment/verify_m10_apisix_route_diff.py"),
)


class CaptureError(RuntimeError):
    pass


def sha256(path: Path) -> str:
    return hashlib.sha256(path.read_bytes()).hexdigest()


def canonical_sha256(value: Any) -> str:
    return hashlib.sha256(json.dumps(value, sort_keys=True, separators=(",", ":")).encode()).hexdigest()


def direct_environment() -> dict[str, str]:
    environment = os.environ.copy()
    for key in ("HTTP_PROXY", "HTTPS_PROXY", "ALL_PROXY", "http_proxy", "https_proxy", "all_proxy"):
        environment.pop(key, None)
    return environment


def kubectl(*args: str) -> str:
    completed = subprocess.run(
        ["kubectl", "--request-timeout=15s", *args], cwd=ROOT,
        env=direct_environment(), text=True, capture_output=True, check=False, timeout=30,
    )
    if completed.returncode:
        raise CaptureError(completed.stderr.strip() or "kubectl failed")
    return completed.stdout


def kubectl_json(*args: str) -> dict[str, Any]:
    value = json.loads(kubectl(*args, "-o", "json"))
    if not isinstance(value, dict):
        raise CaptureError("kubectl JSON object required")
    return value


def secret_keys(namespace: str, name: str) -> dict[str, Any]:
    template = (
        "{{.metadata.uid}}{{\"\\n\"}}{{.metadata.resourceVersion}}{{\"\\n\"}}"
        "{{range $key,$value := .data}}{{$key}}{{\"\\n\"}}{{end}}"
    )
    completed = subprocess.run(
        ["kubectl", "--request-timeout=15s", "get", "secret", name, "-n", namespace, "-o", f"go-template={template}"],
        cwd=ROOT, env=direct_environment(), text=True, capture_output=True, check=False, timeout=30,
    )
    if completed.returncode:
        return {"namespace": namespace, "name": name, "exists": False, "uid": None, "resource_version": None, "keys": []}
    lines = completed.stdout.splitlines()
    return {
        "namespace": namespace, "name": name, "exists": True,
        "uid": lines[0] if lines else None,
        "resource_version": lines[1] if len(lines) > 1 else None,
        "keys": sorted(lines[2:]),
    }


def route_index(routes: list[dict[str, Any]]) -> dict[str, dict[str, Any]]:
    result: dict[str, dict[str, Any]] = {}
    for route in routes:
        route_id = str(route.get("id") or "")
        if not route_id or route_id in result:
            raise ValueError("route IDs must be non-empty and unique")
        result[route_id] = route
    return result


def compare_routes(candidate_routes: list[dict[str, Any]], live_routes: list[dict[str, Any]]) -> dict[str, Any]:
    candidate = route_index(candidate_routes)
    live = route_index(live_routes)
    candidate_ids, live_ids = set(candidate), set(live)
    common = candidate_ids & live_ids
    changed = sorted(route_id for route_id in common if canonical_sha256(candidate[route_id]) != canonical_sha256(live[route_id]))
    unchanged = sorted(common - set(changed))
    missing = sorted(candidate_ids - live_ids)
    extra = sorted(live_ids - candidate_ids)
    return {
        "candidate_count": len(candidate), "live_count": len(live),
        "missing_live_ids": missing, "extra_live_ids": extra,
        "changed_ids": changed, "unchanged_ids": unchanged,
        "zero_diff": not missing and not extra and not changed,
    }


def apisix_container(statefulset: dict[str, Any]) -> dict[str, Any]:
    containers = statefulset.get("spec", {}).get("template", {}).get("spec", {}).get("containers", [])
    return next((item for item in containers if item.get("name") == "apisix"), {})


def workload_policy(statefulset: dict[str, Any]) -> dict[str, Any]:
    pod_spec = statefulset.get("spec", {}).get("template", {}).get("spec", {})
    container = apisix_container(statefulset)
    env = {item.get("name"): item for item in container.get("env", [])}
    mounts = {item.get("name"): item for item in container.get("volumeMounts", [])}
    volumes = {item.get("name"): item for item in pod_spec.get("volumes", [])}
    oidc = env.get("APISIX_OIDC_CLIENT_SECRET", {}).get("valueFrom", {}).get("secretKeyRef", {})
    ca = volumes.get("traffic-platform-ca", {}).get("secret", {})
    complete = (
        oidc.get("name") == "traffic-credentials" and oidc.get("key") == "OIDC_CLIENT_SECRET"
        and env.get("SSL_CERT_FILE", {}).get("value") == "/etc/traffic-platform-ca/ca.crt"
        and mounts.get("traffic-platform-ca", {}).get("mountPath") == "/etc/traffic-platform-ca"
        and ca.get("secretName") == "traffic-platform-ca"
    )
    return {
        "oidc_secret_env": oidc.get("name") == "traffic-credentials" and oidc.get("key") == "OIDC_CLIENT_SECRET",
        "ssl_cert_file": env.get("SSL_CERT_FILE", {}).get("value"),
        "ca_mount": mounts.get("traffic-platform-ca", {}).get("mountPath"),
        "ca_secret": ca.get("secretName"), "policy_complete": complete,
    }


def load_candidate() -> tuple[dict[str, Any], list[dict[str, Any]], dict[str, Any], dict[str, Any]]:
    route_document = yaml.safe_load((ROOT / ROUTE_SOURCE).read_text(encoding="utf-8"))
    routes = yaml.safe_load(route_document["data"]["apisix.yaml"]).get("routes", [])
    gateway_documents = list(yaml.safe_load_all((ROOT / GATEWAY_SOURCE).read_text(encoding="utf-8")))
    statefulset = next(item for item in gateway_documents if item and item.get("kind") == "StatefulSet" and item.get("metadata", {}).get("name") == "apisix")
    catalog = json.loads((ROOT / CATALOG).read_text(encoding="utf-8"))
    return route_document, routes, statefulset, catalog


def capture() -> dict[str, Any]:
    route_document, candidate_routes, candidate_statefulset, catalog = load_candidate()
    live_configmap = kubectl_json("get", "configmap", "apisix-routes", "-n", "gateway")
    live_statefulset = kubectl_json("get", "statefulset", "apisix", "-n", "gateway")
    live_raw = live_configmap.get("data", {}).get("apisix.yaml", "")
    live_routes = (yaml.safe_load(live_raw) or {}).get("routes", [])
    route_diff = compare_routes(candidate_routes, live_routes)
    candidate_workload = workload_policy(candidate_statefulset)
    live_workload = workload_policy(live_statefulset)
    credentials = secret_keys("gateway", "traffic-credentials")
    ca = secret_keys("gateway", "traffic-platform-ca")
    n001 = json.loads((ROOT / N001).read_text(encoding="utf-8"))
    n005 = json.loads((ROOT / N005).read_text(encoding="utf-8"))
    blockers: list[str] = []
    if n005.get("acceptance_status") != "PASS": blockers.append("N005_APPLY_AUTHORIZATION_REQUIRED")
    if not route_diff["zero_diff"]: blockers.append("LIVE_ROUTE_SET_OR_CONTENT_DIFF")
    if not live_workload["policy_complete"]: blockers.append("LIVE_GATEWAY_WORKLOAD_POLICY_DIFF")
    if "OIDC_CLIENT_SECRET" not in credentials["keys"]: blockers.append("GATEWAY_OIDC_CLIENT_SECRET_REQUIRED")
    if not ca["exists"] or "ca.crt" not in ca["keys"]: blockers.append("GATEWAY_OIDC_CA_SECRET_REQUIRED")
    if catalog.get("counts", {}).get("routes_with_blocking_gaps"): blockers.append("CANDIDATE_ROUTE_POLICY_GAP")
    blocking_codes = sorted(set(blockers))
    candidate_raw = route_document["data"]["apisix.yaml"]
    return {
        "schema_version": 1, "artifact_kind": "M10_APISIX_ROUTE_DIFF_RESULT",
        "task_id": "T1-M10-N006", "atomic_pr_id": "T1-M10-P013-OPS-n006-s1",
        "status": "PASS", "engineering_status": "PASS",
        "acceptance_status": "PASS" if not blocking_codes else "BLOCKED_ROUTE_DIFF",
        "environment_kind": "KUBERNETES", "candidate_id": n001.get("candidate_id"),
        "source_sha256": {str(path): sha256(ROOT / path) for path in SOURCE_FILES},
        "candidate": {
            "route_count": len(candidate_routes),
            "rendered_apisix_sha256": hashlib.sha256(candidate_raw.encode()).hexdigest(),
            "catalog_sha256": catalog.get("catalog_sha256"), "policy_counts": catalog.get("counts"),
            "workload_policy": candidate_workload,
        },
        "live": {
            "route_count": len(live_routes), "rendered_apisix_sha256": hashlib.sha256(live_raw.encode()).hexdigest(),
            "configmap_uid": live_configmap.get("metadata", {}).get("uid"),
            "configmap_resource_version": live_configmap.get("metadata", {}).get("resourceVersion"),
            "statefulset_uid": live_statefulset.get("metadata", {}).get("uid"),
            "statefulset_resource_version": live_statefulset.get("metadata", {}).get("resourceVersion"),
            "statefulset_generation": live_statefulset.get("metadata", {}).get("generation"),
            "workload_policy": live_workload,
        },
        "route_diff": route_diff,
        "workload_diff": {"candidate": candidate_workload, "live": live_workload, "matches": candidate_workload == live_workload},
        "secret_key_observation": {"values_captured": False, "traffic_credentials": credentials, "traffic_platform_ca": ca},
        "rollback_baseline": {
            "strategy": "restore_previous_configmap_content_hash_and_statefulset_template_then_rollout_status",
            "previous_rendered_apisix_sha256": hashlib.sha256(live_raw.encode()).hexdigest(),
            "previous_configmap_uid": live_configmap.get("metadata", {}).get("uid"),
            "previous_configmap_resource_version": live_configmap.get("metadata", {}).get("resourceVersion"),
            "previous_statefulset_generation": live_statefulset.get("metadata", {}).get("generation"),
        },
        "blocking_codes": blocking_codes,
        "required_gates": {"G0": "BLOCKED_BY_N002", "G1": "PASS_REPOSITORY_POLICY_ONLY", "G6": "BLOCKED"},
        "shared_infrastructure_touched": False, "production_applied": False,
        "allowed_claims": ["The 58-route candidate is fully materialized and the current Kubernetes route/workload diff was captured read-only"],
        "does_not_prove": ["the candidate was applied", "OIDC login works", "route diff is zero", "G6 rollback passed", "production promotion is authorized"],
    }


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--output", type=Path, default=OUTPUT)
    args = parser.parse_args()
    output = args.output.resolve(strict=False)
    if output not in {OUTPUT.resolve(strict=False), Path("/tmp/m10-apisix-route-diff.json")}:
        raise SystemExit("output must be the N006 evidence path or documented /tmp path")
    evidence = capture()
    output.parent.mkdir(parents=True, exist_ok=True)
    temporary = output.with_name(f".{output.name}.tmp")
    temporary.write_text(json.dumps(evidence, indent=2, sort_keys=True) + "\n", encoding="utf-8")
    temporary.replace(output)
    print(json.dumps(evidence, sort_keys=True))
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
