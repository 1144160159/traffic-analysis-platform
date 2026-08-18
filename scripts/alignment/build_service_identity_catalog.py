#!/usr/bin/env python3
"""Build the redacted cross-component T-SEC-001 service identity catalog."""

from __future__ import annotations

import argparse
import hashlib
import json
import re
from collections import defaultdict
from pathlib import Path
from typing import Any, Iterable

import yaml


ROOT = Path(__file__).resolve().parents[2]
OUTPUT = ROOT / "contracts/security/service-identity-catalog.v1.json"
WORKLOAD_SOURCES = (
    ROOT / "deployments/log-collectors/device-logs.yaml",
    ROOT / "deployments/kubernetes/applications/go-services.yaml",
    ROOT / "deployments/kubernetes/applications/web-ui.yaml",
    ROOT / "deployments/kubernetes/applications/probe-agent.yaml",
    ROOT / "deployments/kubernetes/flink/flink-log-job.yaml",
)
IDENTITY_SOURCE = ROOT / "deployments/kubernetes/security/go-service-identities.v1.yaml"
KAFKA_AUTHORITY = ROOT / "contracts/events/kafka-acl-catalog.v1.json"
CONFIG_AUTHORITY = ROOT / "contracts/configuration/configuration-catalog.v1.json"
IAM_AUTHORITY = ROOT / "go/control-plane/internal/auth/model/scopes.go"
NETWORK_POLICY_AUTHORITY = ROOT / "deployments/kubernetes/security/00-network-policies.yaml"
EXTERNAL_SECRET_SOURCES = (
    ROOT / "deployments/log-collectors/device-logs-secret-ref.yaml",
    ROOT / "deployments/kubernetes/security/external-secrets-template.yaml",
    ROOT / "deployments/kubernetes/security/generated-kafka-service-identities.v1.yaml",
)
GO_RUNTIME_DOCKERFILE = ROOT / "go/control-plane/deployments/docker/Dockerfile.runtime"
SENSITIVE_NAME = re.compile(
    r"(?:PASSWORD|PASSWD|SECRET|TOKEN|PRIVATE_KEY|SIGNING_KEY|API_KEY|ACCESS_KEY|SECRET_KEY|CREDENTIAL)$",
    re.IGNORECASE,
)
TENANT_HEADER_GET = re.compile(r'\.Header\.Get\("X-Tenant-ID"\)')
TENANT_QUERY_GET = re.compile(r'URL\.Query\(\)\.Get\("tenant_id"\)')
GO_SERVICE_NAMES = {
    "ingest-gateway",
    "auth-service",
    "alert-service",
    "asset-service",
    "rule-manager",
    "graph-service",
    "threat-intel-service",
    "forensics-service",
}
OWNER = {
    "ingest-gateway": "probe-domain-owner",
    "auth-service": "security-platform-owner",
    "alert-service": "alert-domain-owner",
    "asset-service": "asset-domain-owner",
    "rule-manager": "rule-deployment-domain-owner",
    "graph-service": "graph-domain-owner",
    "threat-intel-service": "threat-intel-domain-owner",
    "forensics-service": "forensics-domain-owner",
    "web-ui": "web-ui-owner",
    "probe-agent": "probe-domain-owner",
    "flink-log-job": "flink-data-owner",
    "device-log-collector": "observability-data-quality-owner",
}
PRIVILEGED_EXCEPTIONS = {
    "probe-agent": {
        "reason": "packet capture requires host network and approved capture capabilities",
        "required_review": "capability and node admission review per release",
        "service_account_token_required": True,
    }
}


def _relative(path: Path) -> str:
    try:
        return path.relative_to(ROOT).as_posix()
    except ValueError:
        return path.as_posix()


def _sha256(value: bytes | str) -> str:
    if isinstance(value, str):
        value = value.encode("utf-8")
    return hashlib.sha256(value).hexdigest()


def _canonical_sha256(value: Any) -> str:
    payload = json.dumps(value, ensure_ascii=False, sort_keys=True, separators=(",", ":"))
    return _sha256(payload)


def _documents(path: Path) -> Iterable[dict[str, Any]]:
    for document in yaml.safe_load_all(path.read_text(encoding="utf-8")):
        if isinstance(document, dict):
            yield document


def _pod_spec(document: dict[str, Any]) -> dict[str, Any]:
    spec = document.get("spec") or {}
    if document.get("kind") == "CronJob":
        return (
            ((((spec.get("jobTemplate") or {}).get("spec") or {}).get("template") or {}).get("spec"))
            or {}
        )
    return ((spec.get("template") or {}).get("spec") or {})


def _service_accounts() -> dict[tuple[str, str], dict[str, Any]]:
    result: dict[tuple[str, str], dict[str, Any]] = {}
    paths = [IDENTITY_SOURCE, *WORKLOAD_SOURCES, *(ROOT / "deployments/kubernetes").rglob("*.yaml")]
    for path in sorted(set(paths)):
        for document in _documents(path):
            if document.get("kind") != "ServiceAccount":
                continue
            metadata = document.get("metadata") or {}
            namespace = str(metadata.get("namespace") or "default")
            name = str(metadata.get("name") or "")
            if name:
                result[(namespace, name)] = {
                    "source": _relative(path),
                    "automount_service_account_token": document.get("automountServiceAccountToken"),
                }
    return result


def _external_secrets() -> dict[tuple[str, str], dict[str, Any]]:
    result: dict[tuple[str, str], dict[str, Any]] = {}
    for path in EXTERNAL_SECRET_SOURCES:
        for document in _documents(path):
            if document.get("kind") != "ExternalSecret":
                continue
            metadata = document.get("metadata") or {}
            namespace = str(metadata.get("namespace") or "default")
            spec = document.get("spec") or {}
            name = str((spec.get("target") or {}).get("name") or metadata.get("name") or "")
            keys = sorted(
                str(item.get("secretKey"))
                for item in spec.get("data") or []
                if item.get("secretKey")
            )
            if name:
                result[(namespace, name)] = {
                    "source": _relative(path),
                    "keys": keys,
                    "refresh_interval": spec.get("refreshInterval"),
                    "secret_store_ref": spec.get("secretStoreRef"),
                }
    return result


def _secret_references(pod_spec: dict[str, Any], namespace: str) -> list[dict[str, Any]]:
    references: list[dict[str, Any]] = []
    containers = list(pod_spec.get("initContainers") or []) + list(pod_spec.get("containers") or [])
    for container in containers:
        container_name = str(container.get("name") or "unnamed")
        for env in container.get("env") or []:
            secret = ((env.get("valueFrom") or {}).get("secretKeyRef") or {})
            if secret.get("name") and secret.get("key"):
                references.append(
                    {
                        "namespace": namespace,
                        "secret_name": str(secret["name"]),
                        "key": str(secret["key"]),
                        "consumer": container_name,
                        "binding": f"env:{env.get('name')}",
                        "optional": bool(secret.get("optional", False)),
                    }
                )
        for env_from in container.get("envFrom") or []:
            secret = env_from.get("secretRef") or {}
            if secret.get("name"):
                references.append(
                    {
                        "namespace": namespace,
                        "secret_name": str(secret["name"]),
                        "key": "*",
                        "consumer": container_name,
                        "binding": "envFrom",
                        "optional": bool(secret.get("optional", False)),
                    }
                )
    for volume in pod_spec.get("volumes") or []:
        secret = volume.get("secret") or {}
        if secret.get("secretName"):
            references.append(
                {
                    "namespace": namespace,
                    "secret_name": str(secret["secretName"]),
                    "key": "mounted_items_or_all",
                    "consumer": "pod",
                    "binding": f"volume:{volume.get('name')}",
                    "optional": bool(secret.get("optional", False)),
                }
            )
    unique = {(_canonical_sha256(item), json.dumps(item, sort_keys=True)): item for item in references}
    return sorted(unique.values(), key=lambda item: (item["secret_name"], item["key"], item["consumer"]))


def _literal_secret_findings(pod_spec: dict[str, Any]) -> list[dict[str, Any]]:
    findings: list[dict[str, Any]] = []
    containers = list(pod_spec.get("initContainers") or []) + list(pod_spec.get("containers") or [])
    for container in containers:
        for env in container.get("env") or []:
            name = str(env.get("name") or "")
            if name and SENSITIVE_NAME.search(name) and "value" in env and str(env.get("value") or ""):
                findings.append(
                    {
                        "consumer": str(container.get("name") or "unnamed"),
                        "key": name,
                        "value_sha256": _sha256(str(env["value"])),
                    }
                )
    return findings


def _duplicate_env_keys(pod_spec: dict[str, Any]) -> list[dict[str, Any]]:
    findings: list[dict[str, Any]] = []
    for container in list(pod_spec.get("initContainers") or []) + list(pod_spec.get("containers") or []):
        seen: set[str] = set()
        duplicates: set[str] = set()
        for env in container.get("env") or []:
            name = str(env.get("name") or "")
            if name in seen:
                duplicates.add(name)
            seen.add(name)
        if duplicates:
            findings.append({"container": container.get("name"), "keys": sorted(duplicates)})
    return findings


def _container_security(container: dict[str, Any], workload_name: str) -> dict[str, Any]:
    security = container.get("securityContext") or {}
    capabilities = security.get("capabilities") or {}
    drops = {str(item) for item in capabilities.get("drop") or []}
    privileged = bool(security.get("privileged", False))
    exception = PRIVILEGED_EXCEPTIONS.get(workload_name) if privileged else None
    hardened = (
        security.get("runAsNonRoot") is True
        and security.get("runAsUser") not in {None, 0}
        and security.get("allowPrivilegeEscalation") is False
        and "ALL" in drops
        and (security.get("seccompProfile") or {}).get("type") == "RuntimeDefault"
    )
    return {
        "name": container.get("name"),
        "run_as_non_root": security.get("runAsNonRoot"),
        "run_as_user": security.get("runAsUser"),
        "run_as_group": security.get("runAsGroup"),
        "allow_privilege_escalation": security.get("allowPrivilegeEscalation"),
        "capabilities_drop": sorted(drops),
        "capabilities_add": sorted(str(item) for item in capabilities.get("add") or []),
        "seccomp_profile": (security.get("seccompProfile") or {}).get("type"),
        "privileged": privileged,
        "privileged_exception": exception,
        "hardened": hardened or exception is not None,
    }


def _image_identity(image: str) -> dict[str, Any]:
    return {
        "reference": image,
        "digest_pinned": "@sha256:" in image,
        "mutable_tag": "@sha256:" not in image,
    }


def _kafka_principals() -> tuple[dict[str, dict[str, Any]], list[dict[str, Any]]]:
    authority = json.loads(KAFKA_AUTHORITY.read_text(encoding="utf-8"))
    by_workload: dict[str, dict[str, Any]] = {}
    for principal in authority.get("principals") or []:
        workload = str((principal.get("credential") or {}).get("workload") or "")
        if workload:
            by_workload[workload] = {
                "principal_id": principal.get("id"),
                "principal": principal.get("principal"),
                "kind": principal.get("kind"),
                "rollout_state": principal.get("rollout_state"),
                "credential_namespace": (principal.get("credential") or {}).get("namespace"),
                "credential_secret": (principal.get("credential") or {}).get("secret_name"),
            }
    return by_workload, authority.get("principals") or []


def _tenant_fallback_sites() -> list[dict[str, Any]]:
    findings: list[dict[str, Any]] = []
    roots = [ROOT / "go/control-plane/cmd", ROOT / "go/control-plane/internal"]
    for root in roots:
        for path in sorted(root.rglob("*.go")):
            if path.name.endswith("_test.go"):
                continue
            for line_number, line in enumerate(path.read_text(encoding="utf-8").splitlines(), 1):
                kinds = []
                if TENANT_HEADER_GET.search(line):
                    kinds.append("request_header")
                if TENANT_QUERY_GET.search(line):
                    kinds.append("query_parameter")
                for kind in kinds:
                    findings.append(
                        {
                            "path": _relative(path),
                            "line": line_number,
                            "kind": kind,
                            "line_sha256": _sha256(line.strip()),
                        }
                    )
    return findings


def build_catalog() -> dict[str, Any]:
    service_accounts = _service_accounts()
    external_secrets = _external_secrets()
    kafka_by_workload, kafka_principals = _kafka_principals()
    workloads: list[dict[str, Any]] = []
    secret_consumers: dict[tuple[str, str, str], list[str]] = defaultdict(list)

    for path in WORKLOAD_SOURCES:
        for document in _documents(path):
            if document.get("kind") not in {"Deployment", "StatefulSet", "DaemonSet", "Job", "CronJob"}:
                continue
            metadata = document.get("metadata") or {}
            manifest_name = str(metadata.get("name") or "")
            namespace = str(metadata.get("namespace") or "default")
            pod_spec = _pod_spec(document)
            service_account_name = str(pod_spec.get("serviceAccountName") or "default")
            name = (
                service_account_name
                if service_account_name in OWNER
                and manifest_name.startswith(service_account_name)
                else manifest_name
            )
            service_account = service_accounts.get((namespace, service_account_name))
            secret_refs = _secret_references(pod_spec, namespace)
            for reference in secret_refs:
                secret_consumers[(namespace, reference["secret_name"], reference["key"])].append(name)
                provision = external_secrets.get((namespace, reference["secret_name"]))
                reference["external_secret_registered"] = provision is not None
                reference["external_secret_source"] = (provision or {}).get("source")
                reference["rotation_refresh_interval"] = (provision or {}).get("refresh_interval")
            containers = [
                _container_security(container, name) for container in pod_spec.get("containers") or []
            ]
            images = [_image_identity(str(container.get("image") or "")) for container in pod_spec.get("containers") or []]
            gaps: list[str] = []
            if service_account_name == "default" or service_account is None:
                gaps.append("dedicated_service_account_missing")
            token_exception = bool(
                (PRIVILEGED_EXCEPTIONS.get(name) or {}).get("service_account_token_required")
            )
            if pod_spec.get("automountServiceAccountToken") is not False and not token_exception:
                gaps.append("service_account_token_automount_not_disabled")
            if any(not container["hardened"] for container in containers):
                gaps.append("container_security_context_incomplete")
            if any(image["mutable_tag"] for image in images):
                gaps.append("runtime_image_not_digest_pinned")
            literal_findings = _literal_secret_findings(pod_spec)
            if literal_findings:
                gaps.append("literal_secret_material_in_workload")
            duplicate_env = _duplicate_env_keys(pod_spec)
            if duplicate_env:
                gaps.append("duplicate_environment_binding")
            if any(not reference["external_secret_registered"] for reference in secret_refs):
                gaps.append("secret_provisioner_unregistered")
            if name in GO_SERVICE_NAMES and name != "ingest-gateway":
                gaps.append("service_to_service_mtls_identity_missing")
            workload = {
                "workload_id": f"{namespace}/{document.get('kind')}/{name}",
                "name": name,
                "namespace": namespace,
                "kind": document.get("kind"),
                "owner": OWNER.get(name, "platform-contract-owner"),
                "source": _relative(path),
                "service_account": {
                    "name": service_account_name,
                    "declared": service_account is not None,
                    "source": (service_account or {}).get("source"),
                    "pod_token_automount": pod_spec.get("automountServiceAccountToken"),
                    "account_token_automount": (service_account or {}).get("automount_service_account_token"),
                    "rbac_grants_expected": False if name in GO_SERVICE_NAMES | {"web-ui"} else None,
                    "token_automount_exception": token_exception,
                },
                "containers": containers,
                "images": images,
                "secret_references": secret_refs,
                "literal_secret_findings": literal_findings,
                "duplicate_environment_bindings": duplicate_env,
                "kafka_identity": kafka_by_workload.get(name),
                "network_policy_authority": _relative(NETWORK_POLICY_AUTHORITY),
                "privileged_exception": PRIVILEGED_EXCEPTIONS.get(name),
                "blocking_gaps": sorted(set(gaps)),
            }
            workloads.append(workload)

    shared_sensitive: list[dict[str, Any]] = []
    for (namespace, secret_name, key), consumers in sorted(secret_consumers.items()):
        unique_consumers = sorted(set(consumers))
        if len(unique_consumers) < 2 or key in {"mounted_items_or_all", "*"}:
            continue
        if not SENSITIVE_NAME.search(key):
            continue
        shared_sensitive.append(
            {
                "namespace": namespace,
                "secret_name": secret_name,
                "key": key,
                "consumers": unique_consumers,
                "consumer_count": len(unique_consumers),
            }
        )
        for workload in workloads:
            if workload["name"] in unique_consumers:
                workload["blocking_gaps"] = sorted(
                    set(workload["blocking_gaps"]) | {"shared_sensitive_credential"}
                )

    tenant_fallbacks = _tenant_fallback_sites()
    gap_counts: dict[str, int] = defaultdict(int)
    for workload in workloads:
        for gap in workload["blocking_gaps"]:
            gap_counts[gap] += 1
    catalog: dict[str, Any] = {
        "schema_version": 1,
        "control_id": "T-SEC-001",
        "status": "candidate_default_off",
        "production_applied": False,
        "authorities": [
            {"domain": "kubernetes_workload_identity", "path": _relative(IDENTITY_SOURCE), "sha256": _sha256(IDENTITY_SOURCE.read_bytes())},
            {"domain": "kafka_principal_acl", "path": _relative(KAFKA_AUTHORITY), "sha256": _sha256(KAFKA_AUTHORITY.read_bytes())},
            {"domain": "configuration_and_secret_binding", "path": _relative(CONFIG_AUTHORITY), "sha256": _sha256(CONFIG_AUTHORITY.read_bytes())},
            {"domain": "iam_scopes", "path": _relative(IAM_AUTHORITY), "sha256": _sha256(IAM_AUTHORITY.read_bytes())},
            {"domain": "network_policy", "path": _relative(NETWORK_POLICY_AUTHORITY), "sha256": _sha256(NETWORK_POLICY_AUTHORITY.read_bytes())},
            {"domain": "go_runtime_user", "path": _relative(GO_RUNTIME_DOCKERFILE), "sha256": _sha256(GO_RUNTIME_DOCKERFILE.read_bytes())},
        ],
        "policy": {
            "kubernetes_identity": "one dedicated no-RBAC service account per workload; API token automount disabled unless explicitly required",
            "secret_material": "SecretRef or external provider only; values never enter catalog, ConfigMap, logs or evidence",
            "backend_identity": "unique least-privilege principal per workload and backend; shared admin/root accounts forbidden",
            "tenant_authority": "authenticated claims or verified probe identity only; request payload/header/query is never authoritative",
            "service_transport": "unique service identity with mTLS for service-to-service and probe traffic",
            "rotation": "trust bundle first, dual identity window, cutover acknowledgement, old credential revocation and reconcile",
            "privileged_workload": "explicit exception, minimum capabilities, node admission and per-release independent review",
        },
        "workloads": sorted(workloads, key=lambda item: item["workload_id"]),
        "kafka_principals": kafka_principals,
        "shared_sensitive_credentials": shared_sensitive,
        "tenant_authority_findings": {
            "trusted_source": "authenticated context or verified probe binding",
            "untrusted_fallback_sites": tenant_fallbacks,
            "untrusted_fallback_count": len(tenant_fallbacks),
            "status": "PARTIAL" if tenant_fallbacks else "PASS",
        },
        "counts": {
            "workloads": len(workloads),
            "dedicated_service_accounts": sum(item["service_account"]["name"] != "default" and item["service_account"]["declared"] for item in workloads),
            "workloads_with_token_automount_disabled": sum(item["service_account"]["pod_token_automount"] is False for item in workloads),
            "approved_token_automount_exceptions": sum(bool(item["service_account"]["token_automount_exception"]) for item in workloads),
            "hardened_or_approved_exception_containers": sum(container["hardened"] for item in workloads for container in item["containers"]),
            "containers": sum(len(item["containers"]) for item in workloads),
            "secret_references": sum(len(item["secret_references"]) for item in workloads),
            "literal_secret_findings": sum(len(item["literal_secret_findings"]) for item in workloads),
            "unregistered_secret_references": sum(
                not reference["external_secret_registered"]
                for item in workloads
                for reference in item["secret_references"]
            ),
            "kafka_principals": len(kafka_principals),
            "shared_sensitive_credentials": len(shared_sensitive),
            "tenant_untrusted_fallback_sites": len(tenant_fallbacks),
            "workloads_with_blocking_gaps": sum(bool(item["blocking_gaps"]) for item in workloads),
            "gap_counts": dict(sorted(gap_counts.items())),
        },
        "acceptance": {
            "catalog_integrity_gate": "must_pass",
            "security_compliance_gate": "partial_until_shared_credentials_tenant_fallbacks_and_workload_gaps_are_empty",
            "negative_tests": [
                "default_service_account",
                "service_account_token_automount",
                "privilege_escalation_or_capability_regression",
                "literal_secret_value",
                "shared_sensitive_credential",
                "untrusted_tenant_authority",
                "wildcard_or_shared_kafka_principal",
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
        print(json.dumps({"status": status, "catalog_sha256": catalog["catalog_sha256"], "counts": catalog["counts"]}, ensure_ascii=False, indent=2))
        return 0 if status == "PASS" else 1
    args.output.parent.mkdir(parents=True, exist_ok=True)
    args.output.write_text(json.dumps(catalog, ensure_ascii=False, indent=2) + "\n", encoding="utf-8")
    print(json.dumps({"status": "PASS", "output": _relative(args.output.resolve()), "catalog_sha256": catalog["catalog_sha256"], "counts": catalog["counts"]}, ensure_ascii=False, indent=2))
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
