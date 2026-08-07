#!/usr/bin/env python3
"""Verify the repository baseline for T-MINIO-002/003/004 without live mutation."""

from __future__ import annotations

import json
import re
from pathlib import Path
from typing import Any

import yaml


ROOT = Path(__file__).resolve().parents[2]
CONTRACT = ROOT / "contracts/minio/object-governance.v1.json"
LIFECYCLE = ROOT / "deployments/kubernetes/init-jobs/06-minio-lifecycle.yaml"
INFRASTRUCTURE = ROOT / "deployments/kubernetes/infrastructure/06-minio.yaml"
MODEL_SOURCES = (
    ROOT / "mlops/scripts/register_model.py",
    ROOT / "mlops/workflows/mlops-configmap.yaml",
)
WORKFLOWS = (
    ROOT / "mlops/workflows/training-workflow.yaml",
    ROOT / "mlops/workflows/cron-training-workflow.yaml",
    ROOT / "mlops/workflows/mlops-workflow-template.yaml",
    ROOT / "deployments/kubernetes/argo-events/mlops-training-template.yaml",
)
REQUIRED_ENV = {
    "MINIO_ENDPOINT",
    "MINIO_ACCESS_KEY",
    "MINIO_SECRET_KEY",
    "MINIO_BUCKET",
    "MINIO_SECURE",
}
REQUIRED_CLASSES = {
    "pcap",
    "evidence",
    "report",
    "export",
    "model",
    "flink_checkpoint",
    "flink_savepoint",
    "argo_artifact",
}
REQUIRED_BUCKET_FIELDS = {
    "bucket",
    "object_classes",
    "owner",
    "key_rule",
    "manifest_authority",
    "versioning",
    "object_lock",
    "legal_hold",
    "lifecycle",
    "encryption",
    "replication",
    "quota",
    "policy",
    "restore_priority",
}


def load_json(path: Path) -> dict[str, Any]:
    return json.loads(path.read_text(encoding="utf-8"))


def yaml_documents(path: Path) -> list[Any]:
    return list(yaml.safe_load_all(path.read_text(encoding="utf-8")))


def walk(value: Any):
    yield value
    if isinstance(value, dict):
        for nested in value.values():
            yield from walk(nested)
    elif isinstance(value, list):
        for nested in value:
            yield from walk(nested)


def workflow_minio_env_sets(path: Path) -> list[dict[str, dict[str, Any]]]:
    result: list[dict[str, dict[str, Any]]] = []
    for document in yaml_documents(path):
        for value in walk(document):
            if not isinstance(value, list):
                continue
            entries = {
                str(item.get("name")): item
                for item in value
                if isinstance(item, dict) and item.get("name")
            }
            if "MINIO_ENDPOINT" in entries:
                result.append(entries)
    return result


def repository_lifecycle_buckets(path: Path = LIFECYCLE) -> set[str]:
    documents = yaml_documents(path)
    config_map = next(
        document
        for document in documents
        if isinstance(document, dict)
        and document.get("kind") == "ConfigMap"
        and document.get("metadata", {}).get("name") == "minio-lifecycle-policy"
    )
    buckets = set()
    for name in (config_map.get("data") or {}):
        if name.endswith("-lifecycle.json"):
            buckets.add(name.removesuffix("-lifecycle.json"))
    return buckets


def governed_bootstrap_buckets(path: Path = LIFECYCLE) -> tuple[set[str], set[str]]:
    documents = yaml_documents(path)
    job = next(
        document
        for document in documents
        if isinstance(document, dict)
        and document.get("kind") == "Job"
        and document.get("metadata", {}).get("name") == "init-minio-lifecycle"
    )
    containers = job.get("spec", {}).get("template", {}).get("spec", {}).get("containers") or []
    script = "\n".join(
        str(argument)
        for container in containers
        for argument in container.get("args") or []
    )
    created = set(re.findall(r"mc\s+mb\s+--ignore-existing\s+local/([a-z0-9-]+)", script))
    verified = set(re.findall(r"mc\s+stat\s+local/([a-z0-9-]+)", script))
    return created, verified


def infrastructure_facts(path: Path = INFRASTRUCTURE) -> dict[str, Any]:
    documents = yaml_documents(path)
    stateful_set = next(item for item in documents if isinstance(item, dict) and item.get("kind") == "StatefulSet")
    pdb = next(item for item in documents if isinstance(item, dict) and item.get("kind") == "PodDisruptionBudget")
    proxy = next(
        item
        for item in documents
        if isinstance(item, dict)
        and item.get("kind") == "Deployment"
        and item.get("metadata", {}).get("name") == "minio-proxy"
    )
    return {
        "server_replicas": stateful_set.get("spec", {}).get("replicas"),
        "pdb_max_unavailable": pdb.get("spec", {}).get("maxUnavailable"),
        "proxy_replicas": proxy.get("spec", {}).get("replicas"),
    }


def source_policy_errors(source: str, label: str) -> list[str]:
    errors = []
    patterns = {
        "MinIO credential or bucket getenv fallback": r"os\.getenv\(['\"]MINIO_(?:ACCESS_KEY|SECRET_KEY|BUCKET)['\"]\s*,",
        "literal insecure MinIO client": r"secure\s*=\s*False",
        "application bucket creation": r"\.make_bucket\s*\(",
    }
    for description, pattern in patterns.items():
        if re.search(pattern, source):
            errors.append(f"{label}: {description}")
    return errors


def verify(root: Path = ROOT, contract: dict[str, Any] | None = None) -> dict[str, Any]:
    root = root.resolve()
    contract = contract or load_json(root / CONTRACT.relative_to(ROOT))
    errors: list[str] = []

    if contract.get("status") == "closed" or contract.get("production_applied") is not False:
        errors.append("contract must remain implementing and production_applied=false until live gates pass")
    if set(contract.get("remediation_ids") or []) != {"T-MINIO-002", "T-MINIO-003", "T-MINIO-004"}:
        errors.append("contract remediation IDs are incomplete")
    policy = contract.get("security_policy") or {}
    for key in (
        "application_root_credentials_allowed",
        "runtime_bucket_creation_allowed",
        "production_plaintext_allowed",
    ):
        if policy.get(key) is not False:
            errors.append(f"security policy {key} must be false")
    for key in (
        "credentials_from_secret_provider_required",
        "per_service_prefix_policy_required",
        "tls_required",
        "certificate_verification_required",
    ):
        if policy.get(key) is not True:
            errors.append(f"security policy {key} must be true")

    registry = contract.get("bucket_registry") or []
    buckets = [item.get("bucket") for item in registry if isinstance(item, dict)]
    if len(buckets) != len(set(buckets)):
        errors.append("bucket registry contains duplicate buckets")
    covered_classes: set[str] = set()
    for item in registry:
        if not isinstance(item, dict):
            errors.append("bucket registry item is not an object")
            continue
        missing = sorted(REQUIRED_BUCKET_FIELDS - set(item))
        if missing:
            errors.append(f"bucket {item.get('bucket')}: missing fields {missing}")
        covered_classes.update(item.get("object_classes") or [])
    if set(contract.get("required_object_classes") or []) != REQUIRED_CLASSES:
        errors.append("required object class catalog drift")
    if covered_classes != REQUIRED_CLASSES:
        errors.append(f"bucket registry object class coverage drift: {sorted(REQUIRED_CLASSES - covered_classes)}")

    lifecycle_path = root / LIFECYCLE.relative_to(ROOT)
    actual_lifecycle = repository_lifecycle_buckets(lifecycle_path)
    expected_lifecycle = set(contract.get("repository_lifecycle_buckets") or [])
    if actual_lifecycle != expected_lifecycle:
        errors.append(
            f"lifecycle bucket drift: expected={sorted(expected_lifecycle)} actual={sorted(actual_lifecycle)}"
        )
    registered_buckets = set(buckets)
    bootstrap_created, bootstrap_verified = governed_bootstrap_buckets(lifecycle_path)
    if bootstrap_created != registered_buckets:
        errors.append(
            "governed bootstrap bucket drift: "
            f"expected={sorted(registered_buckets)} actual={sorted(bootstrap_created)}"
        )
    if bootstrap_verified != registered_buckets:
        errors.append(
            "governed bootstrap verification drift: "
            f"expected={sorted(registered_buckets)} actual={sorted(bootstrap_verified)}"
        )

    for source_path in MODEL_SOURCES:
        relative = source_path.relative_to(ROOT)
        candidate_path = root / relative
        errors.extend(source_policy_errors(candidate_path.read_text(encoding="utf-8"), relative.as_posix()))

    workflow_sets = 0
    for source_path in WORKFLOWS:
        relative = source_path.relative_to(ROOT)
        candidate_path = root / relative
        sets = workflow_minio_env_sets(candidate_path)
        if not sets:
            errors.append(f"{relative}: no MinIO environment set")
            continue
        workflow_sets += len(sets)
        for entries in sets:
            missing = sorted(REQUIRED_ENV - set(entries))
            if missing:
                errors.append(f"{relative}: missing MinIO env {missing}")
            for key in ("MINIO_ACCESS_KEY", "MINIO_SECRET_KEY"):
                reference = (entries.get(key) or {}).get("valueFrom", {}).get("secretKeyRef")
                if not reference:
                    errors.append(f"{relative}: {key} must use secretKeyRef")
            secure_value = str((entries.get("MINIO_SECURE") or {}).get("value", "")).lower()
            if secure_value not in {"true", "false"}:
                errors.append(f"{relative}: MINIO_SECURE must be explicit")

    facts = infrastructure_facts(root / INFRASTRUCTURE.relative_to(ROOT))
    target = contract.get("topology_target") or {}
    if facts["server_replicas"] != target.get("minimum_servers"):
        errors.append("candidate MinIO server replica count differs from the four-server baseline")
    if facts["pdb_max_unavailable"] != 1:
        errors.append("MinIO PDB must keep maxUnavailable=1")
    if facts["proxy_replicas"] >= target.get("proxy_replicas", 2):
        proxy_gap_visible = True
    else:
        proxy_gap_visible = any("minio-proxy" in gap for gap in contract.get("known_gaps") or [])
    if not proxy_gap_visible:
        errors.append("single-replica proxy gap is not visible")
    if not contract.get("known_gaps") or not contract.get("closure_blockers"):
        errors.append("known gaps and closure blockers must remain explicit")

    return {
        "schema_version": 1,
        "contract_id": contract.get("contract_id"),
        "remediation_ids": contract.get("remediation_ids"),
        "status": "PASS" if not errors else "FAIL",
        "coverage_status": contract.get("coverage_status"),
        "production_applied": contract.get("production_applied"),
        "bucket_count": len(registry),
        "object_class_count": len(covered_classes),
        "repository_lifecycle_buckets": sorted(actual_lifecycle),
        "governed_bootstrap_buckets": sorted(bootstrap_created),
        "governed_bootstrap_verified_buckets": sorted(bootstrap_verified),
        "workflow_minio_env_sets": workflow_sets,
        "infrastructure": facts,
        "known_gap_count": len(contract.get("known_gaps") or []),
        "closure_blocker_count": len(contract.get("closure_blockers") or []),
        "errors": errors,
    }


def main() -> int:
    result = verify()
    print(json.dumps(result, ensure_ascii=False, indent=2))
    return 0 if result["status"] == "PASS" else 1


if __name__ == "__main__":
    raise SystemExit(main())
