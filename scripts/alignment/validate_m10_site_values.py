#!/usr/bin/env python3
"""Strict semantic validator for T1-M10-N003 Kubernetes site values."""

from __future__ import annotations

import argparse
import json
import re
from pathlib import Path
from typing import Any

import yaml


ROOT = Path(__file__).resolve().parents[2]
DEFAULT_INPUT = ROOT / "deployments/kubernetes/site-values.v1.template.yaml"
DEFAULT_TENANTS = {"default", "public", "default-tenant", "tenant-default", "*"}
DNS_RE = re.compile(r"^[a-z0-9]([-a-z0-9.]*[a-z0-9])?$")
SENSITIVE_KEYS = {
    "password", "secret", "token", "privatekey", "private_key",
    "accesskey", "access_key", "secretkey", "secret_key", "clientsecret",
}


def require_mapping(value: Any, path: str, errors: list[str]) -> dict[str, Any]:
    if not isinstance(value, dict):
        errors.append(f"{path} must be an object")
        return {}
    return value


def exact_keys(value: dict[str, Any], required: set[str], path: str, errors: list[str]) -> None:
    missing = sorted(required - set(value))
    unknown = sorted(set(value) - required)
    if missing:
        errors.append(f"{path} missing fields: {','.join(missing)}")
    if unknown:
        errors.append(f"{path} unknown fields: {','.join(unknown)}")


def positive_integer_map(value: Any, path: str, errors: list[str]) -> None:
    mapping = require_mapping(value, path, errors)
    if not mapping:
        errors.append(f"{path} must not be empty")
    for key, item in mapping.items():
        if isinstance(item, bool) or not isinstance(item, int) or item <= 0:
            errors.append(f"{path}.{key} must be a positive integer")


def validate_secret_key_ref(value: Any, path: str, errors: list[str]) -> None:
    mapping = require_mapping(value, path, errors)
    exact_keys(mapping, {"namespace", "name", "key"}, path, errors)
    for key in ("namespace", "name", "key"):
        if not isinstance(mapping.get(key), str) or not mapping[key]:
            errors.append(f"{path}.{key} must be a non-empty string")


def reject_plaintext(value: Any, path: str, errors: list[str], in_secret_keys: bool = False) -> None:
    if isinstance(value, dict):
        for key, child in value.items():
            normalized = key.replace("-", "_").lower()
            if normalized in SENSITIVE_KEYS and not in_secret_keys:
                errors.append(f"{path}.{key} plaintext secret field is forbidden")
            reject_plaintext(child, f"{path}.{key}", errors, in_secret_keys or key == "keys")
    elif isinstance(value, list):
        for index, child in enumerate(value):
            reject_plaintext(child, f"{path}[{index}]", errors, in_secret_keys)
    elif isinstance(value, str) and "-----BEGIN" in value:
        errors.append(f"{path} inline key/certificate material is forbidden")


def validate_site_values(value: Any) -> list[str]:
    errors: list[str] = []
    root = require_mapping(value, "$", errors)
    exact_keys(root, {"apiVersion", "kind", "metadata", "global", "site", "tenants", "secretRefs", "verification"}, "$", errors)
    if root.get("apiVersion") != "traffic.analysis.local/v1" or root.get("kind") != "SiteValues":
        errors.append("apiVersion/kind identity mismatch")
    metadata = require_mapping(root.get("metadata"), "$.metadata", errors)
    exact_keys(metadata, {"name", "contractVersion"}, "$.metadata", errors)
    if metadata.get("contractVersion") != 1:
        errors.append("metadata.contractVersion must be 1")
    global_values = require_mapping(root.get("global"), "$.global", errors)
    exact_keys(global_values, {"namespaces", "imagePolicy", "releasePackage"}, "$.global", errors)
    namespaces = global_values.get("namespaces")
    if not isinstance(namespaces, list) or not namespaces or len(namespaces) != len(set(namespaces)):
        errors.append("global.namespaces must be a non-empty unique list")
    policy = require_mapping(global_values.get("imagePolicy"), "$.global.imagePolicy", errors)
    exact_keys(policy, {"requireDigestPinned", "forbidLatestTag"}, "$.global.imagePolicy", errors)
    if policy.get("requireDigestPinned") is not True or policy.get("forbidLatestTag") is not True:
        errors.append("global image policy must require digests and forbid latest")
    package = require_mapping(global_values.get("releasePackage"), "$.global.releasePackage", errors)
    exact_keys(package, {"requiredPaths"}, "$.global.releasePackage", errors)
    if not isinstance(package.get("requiredPaths"), list) or not package["requiredPaths"]:
        errors.append("global.releasePackage.requiredPaths must be non-empty")

    site = require_mapping(root.get("site"), "$.site", errors)
    exact_keys(site, {"siteId", "cluster", "storage", "network", "retention", "quota", "externalDependencies"}, "$.site", errors)
    if str(site.get("siteId", "")).lower() in DEFAULT_TENANTS or not DNS_RE.fullmatch(str(site.get("siteId", ""))):
        errors.append("site.siteId is empty, default, or non-canonical")
    cluster = require_mapping(site.get("cluster"), "$.site.cluster", errors)
    exact_keys(cluster, {"kubernetesVersion", "minNodes", "cniProvider", "storageClass"}, "$.site.cluster", errors)
    if not isinstance(cluster.get("minNodes"), int) or cluster.get("minNodes", 0) < 2:
        errors.append("site.cluster.minNodes must be at least 2")
    storage = require_mapping(site.get("storage"), "$.site.storage", errors)
    exact_keys(storage, {"clickhouse", "kafka", "postgres", "redis", "minio", "flinkState", "flinkCheckpoints", "flinkSavepoints"}, "$.site.storage", errors)
    if not storage or any(not isinstance(item, str) or not item for item in storage.values()):
        errors.append("site.storage values must be non-empty strings")
    network = require_mapping(site.get("network"), "$.site.network", errors)
    exact_keys(network, {"clusterDomain", "ports", "caBundleSecretRef"}, "$.site.network", errors)
    if not DNS_RE.fullmatch(str(network.get("clusterDomain", ""))):
        errors.append("site.network.clusterDomain is invalid")
    ports = require_mapping(network.get("ports"), "$.site.network.ports", errors)
    exact_keys(ports, {"apisixHttp", "apisixNodePort", "kafkaBootstrap", "clickhouseHttp", "clickhouseNative", "postgres", "redis", "redisSentinel", "minioApi", "keycloakHttps"}, "$.site.network.ports", errors)
    for key, port in ports.items():
        if isinstance(port, bool) or not isinstance(port, int) or not 1 <= port <= 65535:
            errors.append(f"site.network.ports.{key} is invalid")
    validate_secret_key_ref(network.get("caBundleSecretRef"), "$.site.network.caBundleSecretRef", errors)
    positive_integer_map(site.get("retention"), "$.site.retention", errors)
    positive_integer_map(site.get("quota"), "$.site.quota", errors)
    exact_keys(require_mapping(site.get("retention"), "$.site.retention", errors), {"kafkaDefaultHours", "auditDays", "evidenceDays", "objectDays"}, "$.site.retention", errors)
    exact_keys(require_mapping(site.get("quota"), "$.site.quota", errors), {"cpuCores", "memoryGi", "storageGi", "ingressMbps"}, "$.site.quota", errors)
    dependencies = site.get("externalDependencies")
    if not isinstance(dependencies, list) or not dependencies:
        errors.append("site.externalDependencies must be non-empty")
        dependencies = []
    dependency_ids: list[str] = []
    for index, dependency in enumerate(dependencies):
        path = f"$.site.externalDependencies[{index}]"
        item = require_mapping(dependency, path, errors)
        exact_keys(item, {"id", "dns", "port", "tls", "caSecretRef"}, path, errors)
        dependency_ids.append(str(item.get("id", "")))
        if not DNS_RE.fullmatch(str(item.get("dns", ""))):
            errors.append(f"{path}.dns is invalid")
        port = item.get("port")
        if isinstance(port, bool) or not isinstance(port, int) or not 1 <= port <= 65535:
            errors.append(f"{path}.port is invalid")
        if item.get("tls") is True:
            validate_secret_key_ref(item.get("caSecretRef"), f"{path}.caSecretRef", errors)
        elif item.get("caSecretRef") is not None:
            errors.append(f"{path}.caSecretRef must be null when tls=false")
    if len(dependency_ids) != len(set(dependency_ids)):
        errors.append("site external dependency ids must be unique")

    secret_refs = require_mapping(root.get("secretRefs"), "$.secretRefs", errors)
    exact_keys(secret_refs, {"strategy", "required"}, "$.secretRefs", errors)
    if secret_refs.get("strategy") not in {"ExternalSecret", "SealedSecret"}:
        errors.append("secretRefs.strategy is unsupported")
    required_refs = secret_refs.get("required")
    if not isinstance(required_refs, list) or not required_refs:
        errors.append("secretRefs.required must be non-empty")
        required_refs = []
    ref_names: list[str] = []
    for index, reference in enumerate(required_refs):
        path = f"$.secretRefs.required[{index}]"
        item = require_mapping(reference, path, errors)
        exact_keys(item, {"refName", "namespace", "name", "keys"}, path, errors)
        ref_names.append(str(item.get("refName", "")))
        if not all(isinstance(item.get(key), str) and item[key] for key in ("refName", "namespace", "name")):
            errors.append(f"{path} identity fields must be non-empty")
        keys = item.get("keys")
        if not isinstance(keys, list) or not keys or len(keys) != len(set(keys)):
            errors.append(f"{path}.keys must be a non-empty unique list")
    if len(ref_names) != len(set(ref_names)):
        errors.append("secret ref names must be unique")

    tenants = root.get("tenants")
    if not isinstance(tenants, list) or not tenants:
        errors.append("tenants must be non-empty")
        tenants = []
    tenant_ids: list[str] = []
    for index, tenant in enumerate(tenants):
        path = f"$.tenants[{index}]"
        item = require_mapping(tenant, path, errors)
        exact_keys(item, {"tenantId", "namespace", "retention", "quota", "secretRefNames"}, path, errors)
        tenant_id = str(item.get("tenantId", ""))
        tenant_ids.append(tenant_id)
        if tenant_id.lower() in DEFAULT_TENANTS or not DNS_RE.fullmatch(tenant_id):
            errors.append(f"{path}.tenantId is default or non-canonical")
        positive_integer_map(item.get("retention"), f"{path}.retention", errors)
        positive_integer_map(item.get("quota"), f"{path}.quota", errors)
        exact_keys(require_mapping(item.get("retention"), f"{path}.retention", errors), {"evidenceDays", "objectDays"}, f"{path}.retention", errors)
        exact_keys(require_mapping(item.get("quota"), f"{path}.quota", errors), {"maxProbes", "maxConcurrentExports", "maxStorageGi"}, f"{path}.quota", errors)
        names = item.get("secretRefNames")
        if not isinstance(names, list) or not names or not set(names).issubset(set(ref_names)):
            errors.append(f"{path}.secretRefNames contains an unknown reference")
    if len(tenant_ids) != len(set(tenant_ids)):
        errors.append("tenant ids must be unique")
    verification = require_mapping(root.get("verification"), "$.verification", errors)
    exact_keys(verification, {"rejectUnknownFields", "rejectPlaintextSecrets", "rejectDefaultTenant"}, "$.verification", errors)
    if any(verification.get(key) is not True for key in verification):
        errors.append("verification fail-closed flags must all be true")
    reject_plaintext(root, "$", errors)
    return sorted(set(errors))


def load(path: Path) -> Any:
    return yaml.safe_load(path.read_text(encoding="utf-8"))


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--input", type=Path, default=DEFAULT_INPUT)
    parser.add_argument("--json", action="store_true")
    args = parser.parse_args()
    errors = validate_site_values(load(args.input))
    result = {"status": "PASS" if not errors else "FAIL", "input": str(args.input), "errors": errors}
    if args.json:
        print(json.dumps(result, sort_keys=True))
    elif errors:
        for error in errors:
            print(f"FAIL: {error}")
    else:
        print("PASS: T1-M10-N003 site values are strict and fail-closed")
    return 0 if not errors else 1


if __name__ == "__main__":
    raise SystemExit(main())
