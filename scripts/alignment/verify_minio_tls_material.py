#!/usr/bin/env python3
"""Verify candidate-only MinIO TLS material without reading secret values."""

from __future__ import annotations

import json
from pathlib import Path
from typing import Any

import yaml


ROOT = Path(__file__).resolve().parents[2]
CONTRACT = Path("contracts/minio/tls-material.v1.json")
MATERIAL = Path("deployments/kubernetes/security/minio-tls-material.v1.yaml")
BASE_SERVER = Path("deployments/kubernetes/infrastructure/06-minio.yaml")

EXPECTED_SANS = {
    "minio",
    "minio.minio",
    "minio.minio.svc",
    "minio.minio.svc.cluster.local",
    *{f"minio-{index}.minio.minio.svc.cluster.local" for index in range(4)},
}
EXPECTED_CLIENT_NAMESPACES = {"traffic-analysis", "flink", "argo"}
EXPECTED_CLIENT_KEYS = {
    "traffic-analysis": ["ca.crt"],
    "flink": ["ca.crt", "truststore.p12"],
    "argo": ["ca.crt"],
}
EXPECTED_REMOTE_KEY = "traffic-platform-prod-minio-tls"


def _load_json(root: Path, relative: Path) -> dict[str, Any]:
    value = json.loads((root / relative).read_text(encoding="utf-8"))
    if not isinstance(value, dict):
        raise ValueError(f"{relative} must contain a JSON object")
    return value


def verify(root: Path = ROOT) -> dict[str, Any]:
    errors: list[str] = []
    missing = [str(path) for path in (CONTRACT, MATERIAL, BASE_SERVER) if not (root / path).is_file()]
    if missing:
        return {"status": "FAIL", "errors": [f"missing required files: {missing}"]}

    contract = _load_json(root, CONTRACT)
    if contract.get("schema_version") != 1 or contract.get("contract_id") != "traffic-platform-minio-tls-material-v1":
        errors.append("MinIO TLS material contract identity drifted")
    if set(contract.get("remediation_ids") or []) != {"T-MINIO-003", "T-PKI-001"}:
        errors.append("MinIO TLS material remediation IDs drifted")
    if contract.get("status") != "implementing" or contract.get("phase") != "adapter_bundle_built_default_off":
        errors.append("MinIO TLS material must remain an implementing default-off adapter phase")
    if contract.get("production_applied") is not False or contract.get("cutover_ready") is not False:
        errors.append("MinIO TLS material cannot claim production apply or cutover readiness")

    server = contract.get("server_identity") or {}
    if set(server.get("required_dns_sans") or []) != EXPECTED_SANS:
        errors.append("MinIO server DNS SAN inventory is incomplete")
    if set(server.get("secret_keys") or []) != {"public.crt", "private.key", "ca.crt"}:
        errors.append("MinIO server TLS Secret key contract drifted")
    if server.get("minimum_tls_version") != "TLS1.2" or server.get("renew_before_days") != 30:
        errors.append("MinIO TLS minimum version or renewal guard drifted")
    if server.get("file_mapping") != {
        "public.crt": "public.crt",
        "private.key": "private.key",
        "ca.crt": "CAs/ca.crt",
    }:
        errors.append("MinIO certificate directory mapping drifted")

    clients = contract.get("client_ca_distribution") or []
    client_namespaces = {str(item.get("namespace") or "") for item in clients}
    if client_namespaces != EXPECTED_CLIENT_NAMESPACES or len(clients) != 3:
        errors.append("MinIO client CA namespace inventory drifted")
    for item in clients:
        namespace = str(item.get("namespace") or "")
        if item.get("secret_keys") != EXPECTED_CLIENT_KEYS.get(namespace):
            errors.append(f"MinIO client TLS Secret keys drifted: {namespace}")

    guards = contract.get("guardrails") or {}
    for guard in (
        "secret_values_in_repository_allowed",
        "private_key_in_evidence_allowed",
        "insecure_skip_verify_allowed",
        "partial_server_or_client_cutover_allowed",
        "base_statefulset_modified_in_this_phase",
        "base_consumers_modified_in_this_phase",
        "live_apply_in_this_phase",
    ):
        if guards.get(guard) is not False:
            errors.append(f"MinIO TLS expand guard must remain false: {guard}")
    if len(contract.get("required_preflight") or []) < 7 or len(contract.get("known_gaps") or []) < 5:
        errors.append("MinIO TLS preflight or known-gap inventory is incomplete")

    manifest_text = (root / MATERIAL).read_text(encoding="utf-8")
    if "BEGIN PRIVATE KEY" in manifest_text or "BEGIN CERTIFICATE" in manifest_text:
        errors.append("MinIO TLS material manifest must not contain certificate or private key values")
    documents = [item for item in yaml.safe_load_all(manifest_text) if item]
    if len(documents) != 4 or any(item.get("kind") != "ExternalSecret" for item in documents):
        errors.append("MinIO TLS material must contain exactly four ExternalSecrets")
    observed: dict[tuple[str, str], dict[str, Any]] = {}
    for item in documents:
        metadata = item.get("metadata") or {}
        key = (str(metadata.get("namespace") or ""), str(metadata.get("name") or ""))
        if key in observed:
            errors.append(f"duplicate MinIO TLS ExternalSecret: {key}")
        observed[key] = item
        annotations = metadata.get("annotations") or {}
        if annotations.get("traffic.platform/production-applied") != "false" or annotations.get("traffic.platform/cutover-ready") != "false":
            errors.append(f"MinIO TLS ExternalSecret must remain candidate-only: {key}")
        spec = item.get("spec") or {}
        if spec.get("refreshInterval") != "1h" or (spec.get("secretStoreRef") or {}).get("name") != "traffic-platform-secret-store":
            errors.append(f"MinIO TLS ExternalSecret store or refresh drifted: {key}")
        if any((entry.get("remoteRef") or {}).get("key") != EXPECTED_REMOTE_KEY for entry in spec.get("data") or []):
            errors.append(f"MinIO TLS remote key drifted: {key}")

    expected_keys = {("minio", "minio-server-tls")} | {
        (namespace, "minio-client-ca") for namespace in EXPECTED_CLIENT_NAMESPACES
    }
    if set(observed) != expected_keys:
        errors.append("MinIO TLS ExternalSecret namespace/name set drifted")
    server_doc = observed.get(("minio", "minio-server-tls"), {})
    server_secret_keys = {entry.get("secretKey") for entry in (server_doc.get("spec") or {}).get("data") or []}
    if server_secret_keys != {"public.crt", "private.key", "ca.crt"}:
        errors.append("MinIO server ExternalSecret keys drifted")
    for namespace in EXPECTED_CLIENT_NAMESPACES:
        item = observed.get((namespace, "minio-client-ca"), {})
        keys = {entry.get("secretKey") for entry in (item.get("spec") or {}).get("data") or []}
        if keys != set(EXPECTED_CLIENT_KEYS[namespace]):
            errors.append(f"MinIO client TLS key set drifted: {namespace}")

    base_server = (root / BASE_SERVER).read_text(encoding="utf-8")
    if "minio-server-tls" in base_server or "--certs-dir" in base_server:
        errors.append("default-off TLS material phase must not activate the base MinIO StatefulSet")
    if "http://minio-{0...3}.minio.minio.svc.cluster.local/data" not in base_server:
        errors.append("base MinIO transport changed without a complete cutover bundle")

    return {
        "status": "PASS" if not errors else "FAIL",
        "contract_id": contract.get("contract_id"),
        "phase": contract.get("phase"),
        "production_applied": contract.get("production_applied"),
        "cutover_ready": contract.get("cutover_ready"),
        "server_san_count": len(server.get("required_dns_sans") or []),
        "client_ca_namespace_count": len(client_namespaces),
        "errors": errors,
    }


def main() -> int:
    result = verify()
    print(json.dumps(result, ensure_ascii=False, indent=2, sort_keys=True))
    return 0 if result["status"] == "PASS" else 1


if __name__ == "__main__":
    raise SystemExit(main())
