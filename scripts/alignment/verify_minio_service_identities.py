#!/usr/bin/env python3
"""Verify the repository-only MinIO service identity expand phase."""

from __future__ import annotations

import json
from pathlib import Path
from typing import Any

import yaml


ROOT = Path(__file__).resolve().parents[2]
CONTRACT = ROOT / "contracts/minio/service-identities.v1.json"
IDENTITY_MANIFEST = ROOT / "deployments/kubernetes/security/minio-service-identities.v1.yaml"
EXTERNAL_SECRETS = ROOT / "deployments/kubernetes/security/external-secrets-template.yaml"

EXPECTED_IDENTITIES = {
    "alert-service",
    "argo-artifact-controller",
    "asset-service",
    "flink-state-model-reader",
    "forensics-service",
    "mlops-model-writer",
    "probe-agent",
}
FORBIDDEN_ADMIN_ACTIONS = {
    "admin:*",
    "s3:CreateBucket",
    "s3:DeleteBucket",
    "s3:ForceDeleteBucket",
    "s3:PutBucketPolicy",
}
ROOT_MINIO_KEYS = {"MINIO_ACCESS_KEY", "MINIO_SECRET_KEY"}


def load_json(path: Path) -> dict[str, Any]:
    return json.loads(path.read_text(encoding="utf-8"))


def yaml_documents(path: Path) -> list[dict[str, Any]]:
    documents = list(yaml.safe_load_all(path.read_text(encoding="utf-8")))
    return [document for document in documents if isinstance(document, dict)]


def walk(value: Any):
    yield value
    if isinstance(value, dict):
        for nested in value.values():
            yield from walk(nested)
    elif isinstance(value, list):
        for nested in value:
            yield from walk(nested)


def find_resource(documents: list[dict[str, Any]], kind: str, name: str, namespace: str) -> dict[str, Any]:
    return next(
        document
        for document in documents
        if document.get("kind") == kind
        and document.get("metadata", {}).get("name") == name
        and document.get("metadata", {}).get("namespace") == namespace
    )


def policy_action_resources(policy: dict[str, Any]) -> tuple[set[tuple[str, str]], dict[tuple[str, str], set[str]], list[str]]:
    pairs: set[tuple[str, str]] = set()
    prefixes: dict[tuple[str, str], set[str]] = {}
    errors: list[str] = []
    statements = policy.get("Statement") or []
    if policy.get("Version") != "2012-10-17" or not isinstance(statements, list):
        errors.append("policy must use IAM version 2012-10-17 and a statement list")
        return pairs, prefixes, errors
    for statement in statements:
        if not isinstance(statement, dict) or statement.get("Effect") != "Allow":
            errors.append("policy statements must be explicit Allow objects")
            continue
        if "Principal" in statement or "NotAction" in statement or "NotResource" in statement:
            errors.append("identity policy cannot use Principal, NotAction, or NotResource")
        actions = statement.get("Action") or []
        resources = statement.get("Resource") or []
        if isinstance(actions, str):
            actions = [actions]
        if isinstance(resources, str):
            resources = [resources]
        for action in actions:
            for resource in resources:
                pairs.add((str(action), str(resource)))
        condition_prefixes = (
            statement.get("Condition", {})
            .get("StringLike", {})
            .get("s3:prefix", [])
        )
        if isinstance(condition_prefixes, str):
            condition_prefixes = [condition_prefixes]
        for action in actions:
            for resource in resources:
                if str(action) in {"s3:ListBucket", "s3:ListBucketMultipartUploads"}:
                    prefixes.setdefault((str(action), str(resource)), set()).update(map(str, condition_prefixes))
    return pairs, prefixes, errors


def expected_policy_facts(identity: dict[str, Any]) -> tuple[set[tuple[str, str]], dict[tuple[str, str], set[str]]]:
    pairs: set[tuple[str, str]] = set()
    prefixes: dict[tuple[str, str], set[str]] = {}
    for permission in identity.get("permissions") or []:
        bucket = str(permission["bucket"])
        bucket_resource = f"arn:aws:s3:::{bucket}"
        object_resources = {
            f"arn:aws:s3:::{bucket}/*" if prefix == "*" else f"arn:aws:s3:::{bucket}/{prefix}"
            for prefix in permission.get("prefixes") or []
        }
        for action in permission.get("bucket_actions") or []:
            pairs.add((str(action), bucket_resource))
            if action in {"s3:ListBucket", "s3:ListBucketMultipartUploads"}:
                prefixes[(str(action), bucket_resource)] = set(map(str, permission.get("prefixes") or []))
        for action in permission.get("object_actions") or []:
            for resource in object_resources:
                pairs.add((str(action), resource))
    return pairs, prefixes


def collect_secret_refs(path: Path) -> set[tuple[str, str]]:
    refs: set[tuple[str, str]] = set()
    roots: list[Any] = list(yaml_documents(path))
    for document in list(roots):
        for value in walk(document):
            if isinstance(value, str) and ("accessKeySecret:" in value or "secretKeySecret:" in value):
                try:
                    embedded = yaml.safe_load(value)
                except yaml.YAMLError:
                    embedded = None
                if isinstance(embedded, (dict, list)):
                    roots.append(embedded)
    for root in roots:
        for value in walk(root):
            if not isinstance(value, dict):
                continue
            for key in ("secretKeyRef", "accessKeySecret", "secretKeySecret"):
                reference = value.get(key)
                if isinstance(reference, dict) and reference.get("name") and reference.get("key"):
                    refs.add((str(reference["name"]), str(reference["key"])))
    return refs


def external_secret_map(path: Path) -> dict[tuple[str, str], dict[str, Any]]:
    result: dict[tuple[str, str], dict[str, Any]] = {}
    for document in yaml_documents(path):
        if document.get("kind") != "ExternalSecret":
            continue
        metadata = document.get("metadata") or {}
        key = (str(metadata.get("namespace")), str(metadata.get("name")))
        if key in result:
            raise ValueError(f"duplicate ExternalSecret {key[0]}/{key[1]}")
        result[key] = document
    return result


def external_secret_properties(document: dict[str, Any]) -> dict[str, tuple[str, str]]:
    result: dict[str, tuple[str, str]] = {}
    for item in document.get("spec", {}).get("data") or []:
        if not isinstance(item, dict):
            continue
        remote = item.get("remoteRef") or {}
        result[str(item.get("secretKey"))] = (str(remote.get("key")), str(remote.get("property")))
    return result


def verify(root: Path = ROOT, contract: dict[str, Any] | None = None) -> dict[str, Any]:
    root = root.resolve()
    contract = contract or load_json(root / CONTRACT.relative_to(ROOT))
    errors: list[str] = []

    if contract.get("status") == "closed" or contract.get("production_applied") is not False:
        errors.append("identity contract must remain implementing and production_applied=false")
    if set(contract.get("remediation_ids") or []) != {"T-MINIO-003", "T-MINIO-004"}:
        errors.append("identity contract remediation IDs drift")
    bootstrap = contract.get("bootstrap") or {}
    if bootstrap.get("suspended_by_default") is not True or bootstrap.get("expand_only") is not True:
        errors.append("identity bootstrap must be expand-only and suspended by default")
    if bootstrap.get("legacy_identity_removed") is not False:
        errors.append("expand phase cannot claim legacy identity removal")
    guardrails = contract.get("guardrails") or {}
    for key in (
        "wildcard_bucket_resource_allowed",
        "wildcard_s3_action_allowed",
        "admin_action_allowed",
        "application_root_credentials_allowed",
        "secret_values_in_repository_allowed",
        "tls_cutover_in_this_slice",
        "live_apply_in_this_slice",
    ):
        if guardrails.get(key) is not False:
            errors.append(f"guardrail {key} must be false")
    max_policy_bytes = int(guardrails.get("policy_max_bytes") or 0)
    if max_policy_bytes <= 0:
        errors.append("policy_max_bytes must be positive")

    identities = contract.get("identities") or []
    identity_ids = [str(identity.get("identity_id")) for identity in identities]
    if set(identity_ids) != EXPECTED_IDENTITIES or len(identity_ids) != len(set(identity_ids)):
        errors.append(f"identity registry drift: {sorted(identity_ids)}")

    manifest_docs = yaml_documents(root / IDENTITY_MANIFEST.relative_to(ROOT))
    try:
        policy_config = find_resource(manifest_docs, "ConfigMap", "minio-service-identity-policies-v1", "minio")
        job = find_resource(manifest_docs, "Job", str(bootstrap.get("job_name")), str(bootstrap.get("namespace")))
    except StopIteration:
        policy_config, job = {}, {}
        errors.append("identity policy ConfigMap or bootstrap Job is missing")

    if job:
        if job.get("spec", {}).get("suspend") is not True:
            errors.append("identity bootstrap Job must be suspended")
        annotations = job.get("metadata", {}).get("annotations") or {}
        if annotations.get("traffic.platform/approval-required") != "true":
            errors.append("identity bootstrap Job must require approval")
        if annotations.get("traffic.platform/production-applied") != "false":
            errors.append("identity bootstrap Job cannot claim production apply")
        pod_spec = job.get("spec", {}).get("template", {}).get("spec", {})
        if pod_spec.get("automountServiceAccountToken") is not False:
            errors.append("identity bootstrap Job must disable service account token mounting")
        containers = pod_spec.get("containers") or []
        if len(containers) != 1:
            errors.append("identity bootstrap Job must have exactly one container")
            container = {}
        else:
            container = containers[0]
        env_map = {str(item.get("name")): item for item in container.get("env") or [] if isinstance(item, dict)}
        script = "\n".join(map(str, container.get("args") or []))
        if env_map.get("MC_CONFIG_DIR", {}).get("value") != "/tmp/mc":
            errors.append("read-only bootstrap container must place mc configuration under /tmp")
        root_refs = {
            name: (entry.get("valueFrom", {}).get("secretKeyRef") or {})
            for name, entry in env_map.items()
            if name in {"MINIO_ROOT_USER", "MINIO_ROOT_PASSWORD"}
        }
        if root_refs.get("MINIO_ROOT_USER") != {"name": bootstrap.get("root_secret"), "key": "MINIO_ACCESS_KEY"}:
            errors.append("bootstrap root user reference drift")
        if root_refs.get("MINIO_ROOT_PASSWORD") != {"name": bootstrap.get("root_secret"), "key": "MINIO_SECRET_KEY"}:
            errors.append("bootstrap root password reference drift")
    else:
        env_map, script = {}, ""

    policy_data = policy_config.get("data") or {}
    expected_policy_files = {f"{identity.get('policy_name')}.json" for identity in identities}
    if set(policy_data) != expected_policy_files:
        errors.append("policy ConfigMap file registry drift")

    consumer_ref_cache: dict[str, set[tuple[str, str]]] = {}
    for identity in identities:
        identity_id = str(identity.get("identity_id"))
        policy_name = str(identity.get("policy_name"))
        policy_file = f"{policy_name}.json"
        raw_policy = policy_data.get(policy_file)
        if raw_policy is None:
            continue
        try:
            policy = json.loads(raw_policy)
        except json.JSONDecodeError as error:
            errors.append(f"{identity_id}: invalid policy JSON: {error}")
            continue
        compact_size = len(json.dumps(policy, separators=(",", ":")).encode("utf-8"))
        if compact_size > max_policy_bytes:
            errors.append(f"{identity_id}: policy is {compact_size} bytes, exceeds {max_policy_bytes}")
        actual_pairs, actual_prefixes, policy_errors = policy_action_resources(policy)
        errors.extend(f"{identity_id}: {error}" for error in policy_errors)
        expected_pairs, expected_prefixes = expected_policy_facts(identity)
        if actual_pairs != expected_pairs:
            errors.append(f"{identity_id}: policy action/resource pairs drift")
        if actual_prefixes != expected_prefixes:
            errors.append(f"{identity_id}: policy list-prefix conditions drift")
        actions = {action for action, _ in actual_pairs}
        resources = {resource for _, resource in actual_pairs}
        if "s3:*" in actions or "*" in actions or actions & FORBIDDEN_ADMIN_ACTIONS:
            errors.append(f"{identity_id}: wildcard or administrative action is forbidden")
        if "arn:aws:s3:::*" in resources or "*" in resources:
            errors.append(f"{identity_id}: wildcard bucket resource is forbidden")

        access_property = str(identity.get("bootstrap_access_property"))
        secret_property = str(identity.get("bootstrap_secret_property"))
        for property_name in (access_property, secret_property):
            reference = (env_map.get(property_name, {}).get("valueFrom", {}).get("secretKeyRef") or {})
            if reference != {"name": bootstrap.get("identity_secret"), "key": property_name}:
                errors.append(f"{identity_id}: bootstrap env {property_name} reference drift")
        for command in ("mc admin policy create", "mc admin user add", "mc admin policy attach"):
            if command not in script:
                errors.append(f"bootstrap script is missing {command}")
        if policy_name not in script:
            errors.append(f"{identity_id}: policy is not invoked by bootstrap script")

        expected_secret_refs = {
            (str(secret["name"]), "accesskey") for secret in identity.get("workload_secrets") or []
        } | {
            (str(secret["name"]), "secretkey") for secret in identity.get("workload_secrets") or []
        }
        for relative in identity.get("consumers") or []:
            relative = str(relative)
            refs = consumer_ref_cache.setdefault(relative, collect_secret_refs(root / relative))
            identity_names = {str(secret["name"]) for secret in identity.get("workload_secrets") or []}
            if not any(name in identity_names and key in {"accesskey", "secretkey"} for name, key in refs):
                errors.append(f"{identity_id}: consumer {relative} does not reference its scoped secret")
            if any(name == bootstrap.get("root_secret") and key in ROOT_MINIO_KEYS for name, key in refs):
                errors.append(f"{relative}: application root MinIO credential reference is forbidden")
            if any(name in {"minio-secret", "my-minio-cred"} for name, _ in refs):
                errors.append(f"{relative}: obsolete MinIO secret reference remains")

    try:
        external_map = external_secret_map(root / EXTERNAL_SECRETS.relative_to(ROOT))
    except ValueError as error:
        external_map = {}
        errors.append(str(error))
    remote_key = str((contract.get("external_secret_source") or {}).get("remote_key"))
    bootstrap_key = (str(bootstrap.get("namespace")), str(bootstrap.get("identity_secret")))
    bootstrap_external = external_map.get(bootstrap_key)
    expected_bootstrap_properties = {
        str(identity[property])
        for identity in identities
        for property in ("bootstrap_access_property", "bootstrap_secret_property")
    }
    if bootstrap_external is None:
        errors.append("MinIO bootstrap ExternalSecret is missing")
    else:
        actual = external_secret_properties(bootstrap_external)
        expected = {name: (remote_key, name) for name in expected_bootstrap_properties}
        if actual != expected:
            errors.append("MinIO bootstrap ExternalSecret property map drift")

    for identity in identities:
        access_property = str(identity.get("bootstrap_access_property"))
        secret_property = str(identity.get("bootstrap_secret_property"))
        for secret in identity.get("workload_secrets") or []:
            key = (str(secret.get("namespace")), str(secret.get("name")))
            document = external_map.get(key)
            if document is None:
                errors.append(f"{key[0]}/{key[1]} ExternalSecret is missing")
                continue
            actual = external_secret_properties(document)
            expected = {
                "accesskey": (remote_key, access_property),
                "secretkey": (remote_key, secret_property),
            }
            if actual != expected:
                errors.append(f"{key[0]}/{key[1]} ExternalSecret property map drift")

    result = {
        "status": "PASS" if not errors else "FAIL",
        "contract_id": contract.get("contract_id"),
        "production_applied": contract.get("production_applied"),
        "identity_count": len(identities),
        "policy_count": len(policy_data),
        "consumer_manifest_count": len(consumer_ref_cache),
        "errors": errors,
    }
    return result


def main() -> int:
    result = verify()
    print(json.dumps(result, ensure_ascii=False, indent=2, sort_keys=True))
    return 0 if result["status"] == "PASS" else 1


if __name__ == "__main__":
    raise SystemExit(main())
