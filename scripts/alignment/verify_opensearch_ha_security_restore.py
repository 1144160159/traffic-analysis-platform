#!/usr/bin/env python3
"""Verify the default-off T-OS-005 HA, TLS, alerting and restore candidate."""

from __future__ import annotations

import json
import subprocess
from pathlib import Path
from typing import Any

import yaml


ROOT = Path(__file__).resolve().parents[2]
CONTRACT = Path("contracts/opensearch/ha-security-restore.v1.json")
BASE = Path("deployments/kubernetes/infrastructure/05-opensearch.yaml")
OVERLAY = Path("deployments/kubernetes/security/opensearch-ha-v1/statefulset-ha-security.patch.yaml")
KUSTOMIZATION = Path("deployments/kubernetes/security/opensearch-ha-v1/kustomization.yaml")
EXTERNAL_SECRETS = Path("deployments/kubernetes/security/opensearch-ha-v1/external-secrets.template.yaml")
ROLES = Path("deployments/kubernetes/security/opensearch-ha-v1/service-roles.v1.json")
DOCKERFILE = Path("deployments/opensearch/Dockerfile.ha-v1")
ALERTS = Path("deployments/kubernetes/observability/opensearch-ha-alert-rules.yaml")
RENDERER = Path("scripts/alignment/render_opensearch_ha_security.py")
SNAPSHOT_TOOL = Path("scripts/alignment/opensearch_snapshot_restore.py")
RUNBOOK = Path("doc/07_alignment/runbooks/T-OS-005-snapshot-zone-tls-restore.md")
DEFAULT_DEPLOY = Path("deployments/kubernetes/deploy.sh")
PLACEHOLDER = "registry.invalid/traffic/opensearch-ha-v1@sha256:" + "0" * 64
REQUIRED = (CONTRACT, BASE, OVERLAY, KUSTOMIZATION, EXTERNAL_SECRETS, ROLES, DOCKERFILE,
            ALERTS, RENDERER, SNAPSHOT_TOOL, RUNBOOK, DEFAULT_DEPLOY)


def load_json(root: Path, path: Path) -> dict[str, Any]:
    return json.loads((root / path).read_text(encoding="utf-8"))


def render(root: Path) -> tuple[str, str | None]:
    completed = subprocess.run(
        ["python3", str(root / RENDERER)], cwd=root, text=True,
        stdout=subprocess.PIPE, stderr=subprocess.PIPE, check=False,
    )
    if completed.returncode:
        return "", completed.stderr.strip() or completed.stdout.strip()
    return completed.stdout, None


def verify(root: Path = ROOT) -> dict[str, Any]:
    errors: list[str] = []
    missing = [str(path) for path in REQUIRED if not (root / path).is_file()]
    if missing:
        return {"status": "FAIL", "errors": [f"missing required files: {missing}"]}

    contract = load_json(root, CONTRACT)
    if contract.get("remediation_id") != "T-OS-005" or contract.get("work_package") != "WP-23-OS":
        errors.append("contract must bind T-OS-005 to WP-23-OS")
    if contract.get("coverage_status") != "PARTIAL" or contract.get("production_applied") is not False:
        errors.append("repository candidate must remain PARTIAL and production_applied=false")
    domains = contract.get("failure_domains", {})
    if domains.get("required_distinct_zones") != 3 or domains.get("forced_awareness") is not True:
        errors.append("three-zone forced awareness contract drifted")
    security = contract.get("security", {})
    for key in ("http_tls", "transport_tls", "transport_hostname_verification", "audit_enabled"):
        if security.get(key) is not True:
            errors.append(f"security invariant must be true: {key}")
    if security.get("plaintext_fallback_allowed") is not False or security.get("default_admin_application_access_allowed") is not False:
        errors.append("plaintext fallback and default-admin application access must be forbidden")
    required_identities = {"alert-service", "asset-service", "flink-alert-generator", "opensearch-snapshot-operator", "opensearch-monitor"}
    if set(security.get("service_identities", [])) != required_identities:
        errors.append("per-service identity inventory drifted")
    snapshot = contract.get("snapshot_restore", {})
    if snapshot.get("restore_target_must_be_isolated") is not True or snapshot.get("same_cluster_restore_forbidden") is not True:
        errors.append("isolated restore and same-cluster prohibition drifted")
    expected_verification = {"mapping_sha256", "settings_sha256", "aliases", "document_count", "sample_document_ids",
                             "sample_versions", "sample_content_sha256", "query_oracle"}
    if set(snapshot.get("required_verification", [])) != expected_verification:
        errors.append("restore verification oracle inventory drifted")
    guards = contract.get("runtime_guards", {})
    if guards.get("overlay_in_default_deploy_path") is not False or guards.get("snapshot_tool_default_mode") != "plan":
        errors.append("candidate must remain opt-in and snapshot tooling must default to plan")
    if guards.get("production_mutations_in_repository_candidate") != [] or not contract.get("closure_blockers"):
        errors.append("production mutation and closure blocker evidence drifted")
    degradation = contract.get("client_degradation", {})
    if degradation.get("unavailable_must_not_render_as_empty") is not True or degradation.get("browser_evidence_required_before_closure") is not True:
        errors.append("OpenSearch outage must remain an explicit browser degradation, not an empty result")

    base = (root / BASE).read_text(encoding="utf-8")
    overlay = (root / OVERLAY).read_text(encoding="utf-8")
    kustomization = (root / KUSTOMIZATION).read_text(encoding="utf-8")
    deploy = (root / DEFAULT_DEPLOY).read_text(encoding="utf-8")
    if "plugins.security.ssl.http.enabled\n          value: 'false'" not in base:
        errors.append("base manifest drift must remain observable until the opt-in cutover")
    if "opensearch-ha-v1" in deploy or str(KUSTOMIZATION.parent) in deploy:
        errors.append("T-OS-005 overlay must not enter the default deploy path")
    if "../../infrastructure/05-opensearch.yaml" not in kustomization:
        errors.append("overlay must patch the canonical OpenSearch base")
    for token in (
        "topology.kubernetes.io/zone", "minDomains: 3", "whenUnsatisfiable: DoNotSchedule",
        "cluster.routing.allocation.awareness.force.zone.values", "zone-a,zone-b,zone-c",
        "cluster.routing.allocation.disk.watermark.low", "70%", "80%", "90%",
        "plugins.security.ssl.http.enabled", "plugins.security.ssl.http.clientauth_mode", "value: REQUIRE",
        "plugins.security.ssl.transport.enforce_hostname_verification", "plugins.security.audit.type",
        "opensearch-bootstrap-admin", "opensearch-monitor-tls", PLACEHOLDER,
    ):
        if token not in overlay:
            errors.append(f"HA/TLS overlay guard missing: {token}")
    probe_lines = "\n".join(line for line in overlay.splitlines() if "curl " in line)
    if "admin:" in probe_lines or "OPENSEARCH_INITIAL_ADMIN_PASSWORD" in probe_lines:
        errors.append("health probes must not use the admin identity")
    for token in ("--cert /var/run/opensearch-monitor/tls.crt", "--key /var/run/opensearch-monitor/tls.key", "--cacert"):
        if token not in probe_lines:
            errors.append(f"mTLS health probe guard missing: {token}")

    rendered, render_error = render(root)
    if render_error:
        errors.append(f"guarded renderer failed: {render_error}")
    else:
        try:
            documents = [item for item in yaml.safe_load_all(rendered) if item]
            statefulset = next(item for item in documents if item.get("kind") == "StatefulSet")
            pod_spec = statefulset["spec"]["template"]["spec"]
            container = next(item for item in pod_spec["containers"] if item["name"] == "opensearch")
            env = {item["name"]: item for item in container["env"]}
            if statefulset["spec"].get("replicas") != 3:
                errors.append("rendered target must retain three OpenSearch replicas")
            if env.get("plugins.security.ssl.http.enabled", {}).get("value") != "true":
                errors.append("rendered target did not override plaintext HTTP")
            if env.get("plugins.security.ssl.http.clientauth_mode", {}).get("value") != "REQUIRE":
                errors.append("rendered target did not require HTTP client certificates")
            if container.get("image") != PLACEHOLDER:
                errors.append("unapproved candidate must retain the fail-safe image guard")
            if len(pod_spec.get("topologySpreadConstraints", [])) != 1:
                errors.append("rendered target lost topology spread")
        except (KeyError, StopIteration, TypeError, yaml.YAMLError) as exc:
            errors.append(f"rendered target is structurally invalid: {exc}")

    external = (root / EXTERNAL_SECRETS).read_text(encoding="utf-8")
    for name in ("opensearch-node-tls", "opensearch-bootstrap-admin", "opensearch-admin-tls", "opensearch-monitor-tls",
                 "opensearch-snapshot-credentials", "alert-service-opensearch-tls", "asset-service-opensearch-tls",
                 "flink-alert-generator-opensearch-tls", "opensearch-snapshot-operator-tls"):
        if f"name: {name}" not in external:
            errors.append(f"external secret template missing: {name}")
    if "REPLACE_WITH_" not in external or "BEGIN PRIVATE KEY" in external:
        errors.append("external secret template must keep placeholders and contain no private keys")

    roles = load_json(root, ROLES)
    if roles.get("contains_secrets") is not False or roles.get("authentication", {}).get("mode") != "mutual_tls":
        errors.append("service roles must be secret-free and mutual-TLS bound")
    role_text = json.dumps(roles, sort_keys=True)
    for identity in ("CN=alert-service", "CN=asset-service", "CN=flink-alert-generator",
                     "CN=opensearch-snapshot-operator", "CN=opensearch-monitor"):
        if identity not in role_text:
            errors.append(f"service role identity missing: {identity}")
    for role in roles.get("roles", []):
        if "all_access" in role.get("cluster_permissions", []):
            errors.append("application roles must not receive all_access")
        for permission in role.get("index_permissions", []):
            if "*" in permission.get("index_patterns", []):
                errors.append("application roles must not receive wildcard-index permissions")

    dockerfile = (root / DOCKERFILE).read_text(encoding="utf-8")
    if "opensearchproject/opensearch@sha256:" not in dockerfile or "repository-s3" not in dockerfile:
        errors.append("snapshot plugin image must use a pinned base and install repository-s3")
    alerts = (root / ALERTS).read_text(encoding="utf-8")
    for alert in ("OpenSearchMetricsEndpointMissing", "OpenSearchClusterRed", "OpenSearchUnassignedShards",
                  "OpenSearchRelocationStalled", "OpenSearchPendingClusterTasks", "OpenSearchWriteRejections",
                  "OpenSearchDiskHighWatermark", "OpenSearchDiskFloodStage", "OpenSearchSnapshotStale",
                  "OpenSearchSnapshotFailure", "OpenSearchTLSCertificateExpiring", "OpenSearchZoneConcentration"):
        if f"alert: {alert}" not in alerts:
            errors.append(f"OpenSearch alert rule missing: {alert}")

    renderer = (root / RENDERER).read_text(encoding="utf-8")
    if "LoadRestrictionsNone" not in renderer or "--require-approved-image" not in renderer or "PLACEHOLDER_DIGEST" not in renderer:
        errors.append("guarded renderer must handle the external base and refuse the placeholder image")
    tool = (root / SNAPSHOT_TOOL).read_text(encoding="utf-8")
    for token in ("OpenSearch endpoint must use https", "same-cluster restore is forbidden", 'state.get("state") != "SUCCESS"',
                  "--approved-manifest-sha256", "approval manifest SHA-256 mismatch", "target_isolated",
                  "restore target index already exists", "snapshot already exists", "REQUIRED_VERIFICATION",
                  '"mutations": []', "wait_for_completion=true"):
        if token not in tool:
            errors.append(f"snapshot/restore safety guard missing: {token}")
    if "password" in json.dumps(load_json(root, ROLES)).lower() and "passwords_or_password_hashes_in_git" not in role_text:
        errors.append("role contract appears to contain credential material")

    return {
        "status": "PASS" if not errors else "FAIL",
        "remediation_id": contract.get("remediation_id"),
        "coverage_status": contract.get("coverage_status"),
        "production_applied": contract.get("production_applied"),
        "required_distinct_zones": domains.get("required_distinct_zones"),
        "service_identity_count": len(roles.get("roles", [])),
        "alert_rule_count": alerts.count("- alert:"),
        "fail_safe_image_guard": PLACEHOLDER in overlay,
        "errors": errors,
    }


def main() -> int:
    result = verify()
    print(json.dumps(result, ensure_ascii=False, indent=2))
    return 0 if result["status"] == "PASS" else 1


if __name__ == "__main__":
    raise SystemExit(main())
