#!/usr/bin/env python3
"""Render an immutable, approval-bound and still-suspended projection repair Job."""

from __future__ import annotations

import argparse
import hashlib
import json
import re
from datetime import datetime, timezone
from pathlib import Path
from typing import Any

import yaml

from candidate_snapshot import build_snapshot


ROOT = Path(__file__).resolve().parents[2]
CONTRACT = ROOT / "contracts/opensearch/projection-repair-execution.v1.json"
SHA256_RE = re.compile(r"^[0-9a-f]{64}$")
IMAGE_RE = re.compile(r"^[a-z0-9][a-z0-9./_:-]*@sha256:[0-9a-f]{64}$")
WILDCARD_RE = re.compile(r"[*?\[\]]")
REQUIRED_APPROVALS = ("sre", "qa", "security", "domain_accountable")


def file_sha256(path: Path) -> str:
    digest = hashlib.sha256()
    with path.open("rb") as stream:
        for chunk in iter(lambda: stream.read(1024 * 1024), b""):
            digest.update(chunk)
    return digest.hexdigest()


def load_json(path: Path) -> dict[str, Any]:
    value = json.loads(path.read_text(encoding="utf-8"))
    if not isinstance(value, dict):
        raise ValueError(f"expected JSON object: {path}")
    return value


def parse_time(value: str) -> datetime:
    normalized = value[:-1] + "+00:00" if value.endswith("Z") else value
    parsed = datetime.fromisoformat(normalized)
    if parsed.tzinfo is None:
        raise ValueError("timestamp must include timezone")
    return parsed.astimezone(timezone.utc)


def dns_name(prefix: str, run_id: str) -> str:
    suffix = re.sub(r"[^a-z0-9-]+", "-", run_id.lower()).strip("-")
    value = f"{prefix}-{suffix}".strip("-")
    if not suffix or len(value) > 63:
        raise ValueError("run ID cannot be represented as a Kubernetes DNS label")
    return value


def argv_value(argv: list[str], flag: str) -> str:
    if argv.count(flag) != 1:
        raise ValueError(f"proposed argv must contain exactly one {flag}")
    index = argv.index(flag)
    if index + 1 >= len(argv):
        raise ValueError(f"proposed argv is missing the value for {flag}")
    return str(argv[index + 1])


def validate_inputs(
    review: dict[str, Any], review_sha: str,
    approval: dict[str, Any], approval_sha: str,
    *, now: datetime, current_content_sha256: str,
) -> tuple[str, str, list[str]]:
    if not SHA256_RE.fullmatch(review_sha) or not SHA256_RE.fullmatch(approval_sha):
        raise ValueError("artifact SHA-256 binding is invalid")
    if review.get("schema_version") != 1 or review.get("mode") != "REPAIR_REVIEW_PACKAGE":
        raise ValueError("repair review package identity is invalid")
    if review.get("execution_authorized") is not False or review.get("production_applied") is not False or review.get("production_mutations") != []:
        raise ValueError("repair review package must remain non-authorizing and non-mutating")
    bindings = review.get("bindings")
    if not isinstance(bindings, dict) or bindings.get("g0_candidate_content_sha256") != current_content_sha256:
        raise ValueError("repair review package does not bind the current source snapshot")
    shadow_captured_at = parse_time(str(bindings.get("shadow_captured_at", "")))
    shadow_age = (now.astimezone(timezone.utc) - shadow_captured_at).total_seconds()
    if shadow_age < 0 or shadow_age > 900:
        raise ValueError("repair review shadow is expired or captured in the future")
    image_digest = bindings.get("immutable_tool_image_digest")
    if not isinstance(image_digest, str) or not re.fullmatch(r"sha256:[0-9a-f]{64}", image_digest):
        raise ValueError("repair review package has no immutable tool image digest")

    if approval.get("schema_version") != 1 or approval.get("mode") != "AUTHORIZED_BOUNDED_REPAIR" or approval.get("execution_authorized") is not True:
        raise ValueError("separate repair approval bundle is not authorized")
    if approval.get("review_package_sha256") != review_sha:
        raise ValueError("repair approval bundle does not bind the review package")
    image = approval.get("immutable_tool_image")
    if not isinstance(image, str) or not IMAGE_RE.fullmatch(image) or not image.endswith("@" + image_digest):
        raise ValueError("approved image must be the exact review-bound repository@sha256 reference")
    requested_by = approval.get("requested_by")
    nonce = approval.get("approval_nonce")
    if not isinstance(requested_by, str) or not requested_by.strip() or not isinstance(nonce, str) or not nonce.strip():
        raise ValueError("repair approval requester and nonce are required")
    not_before = parse_time(str(approval.get("not_before", "")))
    expires_at = parse_time(str(approval.get("expires_at", "")))
    if expires_at <= not_before or (expires_at - not_before).total_seconds() > 14_400:
        raise ValueError("repair approval window must be positive and at most four hours")
    current = now.astimezone(timezone.utc)
    if current < not_before or current > expires_at:
        raise ValueError("repair approval window is not active")

    approvals = approval.get("approvals")
    if not isinstance(approvals, dict) or set(approvals) != set(REQUIRED_APPROVALS):
        raise ValueError("repair approval bundle must contain exactly four required roles")
    identities: set[str] = set()
    for role in REQUIRED_APPROVALS:
        item = approvals[role]
        if not isinstance(item, dict) or item.get("status") != "APPROVED":
            raise ValueError(f"repair approval is missing for role {role}")
        identity = item.get("approved_by")
        if not isinstance(identity, str) or not identity.strip() or identity == requested_by:
            raise ValueError(f"repair role {role} is empty or self-approved")
        if identity in identities:
            raise ValueError("repair approval identities must be distinct")
        identities.add(identity)
        approved_at = parse_time(str(item.get("approved_at", "")))
        if approved_at < not_before or approved_at > expires_at:
            raise ValueError(f"repair approval timestamp is outside the window for role {role}")

    proposed = review.get("proposed_execution")
    argv = proposed.get("argv") if isinstance(proposed, dict) else None
    if not isinstance(argv, list) or not argv or argv[0] != "alert-projection-reconcile" or not all(isinstance(item, str) for item in argv):
        raise ValueError("repair review package must contain an argv-only reconcile command")
    if proposed.get("shell") is not None:
        raise ValueError("shell repair commands are forbidden")
    required_flags = (
        "--tenant", "--start", "--end", "--alert-ids", "--expected-cluster-uuid",
        "--expected-read-target", "--expected-write-alias", "--expected-write-index", "--max-documents",
    )
    for flag in required_flags:
        value = argv_value(argv, flag)
        if not value or WILDCARD_RE.search(value):
            raise ValueError(f"repair scope {flag} must be exact and non-wildcard")
    repair_ids = [item for item in argv_value(argv, "--alert-ids").split(",") if item]
    if not repair_ids or len(repair_ids) != len(set(repair_ids)):
        raise ValueError("repair alert IDs must be explicit and unique")
    if int(argv_value(argv, "--max-documents")) != len(repair_ids) or len(repair_ids) > 10_000:
        raise ValueError("repair document budget must equal the exact alert ID count")
    expected_bindings = {
        "--tenant": "tenant_id", "--start": "start_time", "--end": "end_time",
        "--expected-cluster-uuid": "cluster_uuid", "--expected-read-target": "read_target",
        "--expected-write-alias": "write_alias", "--expected-write-index": "write_index",
    }
    for flag, binding_name in expected_bindings.items():
        if argv_value(argv, flag) != bindings.get(binding_name):
            raise ValueError(f"repair argv {flag} does not match review binding {binding_name}")
    if repair_ids != bindings.get("repair_ids"):
        raise ValueError("repair argv alert IDs do not match the review binding")
    if len(repair_ids) != int(bindings.get("missing_count", 0)) + int(bindings.get("stale_count", 0)):
        raise ValueError("repair alert IDs do not match missing plus stale counts")
    return image, requested_by, argv


def secret_env(name: str, key: str) -> dict[str, Any]:
    return {"name": name, "valueFrom": {"secretKeyRef": {"name": "traffic-credentials", "key": key}}}


def render_documents(
    *, review: dict[str, Any], review_text: str, review_sha: str,
    approval: dict[str, Any], approval_text: str, approval_sha: str,
    image: str, requested_by: str, proposed_argv: list[str], run_id: str,
    namespace: str = "applications",
) -> list[dict[str, Any]]:
    job_name = dns_name("alert-projection-repair", run_id)
    artifact_name = dns_name("alert-projection-repair-artifacts", run_id)
    labels = {
        "traffic.io/remediation-id": "T-OS-004",
        "traffic.io/migration-phase": "canary-repair",
        "traffic.io/run-id": run_id,
    }
    argv = [requested_by if item == "APPROVED_OPERATOR_REQUIRED" else item for item in proposed_argv[1:]]
    argv.extend([
        "--review-package", "/etc/alert-projection-repair/review.json",
        "--approval-bundle", "/etc/alert-projection-repair/approval.json",
        "--expected-review-sha256", review_sha,
        "--expected-approval-sha256", approval_sha,
        "--expected-tool-image", image,
    ])
    service_account = {
        "apiVersion": "v1", "kind": "ServiceAccount",
        "metadata": {"name": job_name, "namespace": namespace, "labels": labels},
        "automountServiceAccountToken": False,
    }
    artifacts = {
        "apiVersion": "v1", "kind": "ConfigMap",
        "metadata": {"name": artifact_name, "namespace": namespace, "labels": labels},
        "immutable": True,
        "data": {"review.json": review_text, "approval.json": approval_text},
    }
    env = [
        {"name": "CLICKHOUSE_HOSTS", "value": "clickhouse-1.middleware.svc:9000,clickhouse-2.middleware.svc:9000"},
        {"name": "CLICKHOUSE_DATABASE", "value": "traffic"},
        {"name": "CLICKHOUSE_USERNAME", "value": "default"},
        secret_env("CLICKHOUSE_PASSWORD", "CLICKHOUSE_PASSWORD"),
        {"name": "OPENSEARCH_ADDRS", "value": "http://opensearch.middleware.svc:9200"},
        {"name": "OPENSEARCH_USERNAME", "value": "admin"},
        secret_env("OPENSEARCH_PASSWORD", "OPENSEARCH_ADMIN_PASSWORD"),
        {"name": "OPENSEARCH_ALERTS_V2_ENABLED", "value": "true"},
        {"name": "OPENSEARCH_ALERTS_READ_ALIAS", "value": argv_value(proposed_argv, "--expected-read-target")},
        {"name": "OPENSEARCH_ALERTS_WRITE_ALIAS", "value": argv_value(proposed_argv, "--expected-write-alias")},
        {"name": "OPENSEARCH_ALERT_PROJECTION_REBUILD_MAX_DOCUMENTS", "value": argv_value(proposed_argv, "--max-documents")},
        {"name": "OPENSEARCH_ALERT_PROJECTION_REPAIR_PER_SECOND", "value": "50"},
        {"name": "OPENSEARCH_ALERT_PROJECTION_STOP_ERROR_COUNT", "value": "1"},
        {"name": "AUTH_POSTGRES_HOST", "value": "postgres-primary.databases.svc"},
        {"name": "AUTH_POSTGRES_PORT", "value": "5432"},
        {"name": "AUTH_POSTGRES_DATABASE", "value": "traffic_platform"},
        {"name": "AUTH_POSTGRES_USERNAME", "value": "postgres"},
        secret_env("AUTH_POSTGRES_PASSWORD", "PG_PASSWORD"),
        {"name": "AUTH_POSTGRES_SSL_MODE", "value": "require"},
    ]
    annotations = {
        "traffic.io/execution-mode": "separately-approved-explicit-unsuspend-required",
        "traffic.io/review-sha256": review_sha,
        "traffic.io/approval-sha256": approval_sha,
        "traffic.io/approval-nonce": str(approval["approval_nonce"]),
        "traffic.io/approval-expires-at": str(approval["expires_at"]),
        "traffic.io/g0-content-sha256": str(review["bindings"]["g0_candidate_content_sha256"]),
        "traffic.io/shadow-binding-sha256": str(review["bindings"]["shadow_binding_sha256"]),
    }
    job = {
        "apiVersion": "batch/v1", "kind": "Job",
        "metadata": {"name": job_name, "namespace": namespace, "labels": labels, "annotations": annotations},
        "spec": {
            "suspend": True, "backoffLimit": 0, "activeDeadlineSeconds": 3600,
            "ttlSecondsAfterFinished": 604800,
            "template": {
                "metadata": {"labels": labels, "annotations": annotations},
                "spec": {
                    "serviceAccountName": job_name, "automountServiceAccountToken": False,
                    "restartPolicy": "Never", "terminationGracePeriodSeconds": 30,
                    "securityContext": {"runAsNonRoot": True, "runAsUser": 65532, "runAsGroup": 65532, "seccompProfile": {"type": "RuntimeDefault"}},
                    "containers": [{
                        "name": "repair", "image": image, "imagePullPolicy": "IfNotPresent",
                        "command": ["/usr/local/bin/alert-projection-reconcile"], "args": argv, "env": env,
                        "securityContext": {"allowPrivilegeEscalation": False, "readOnlyRootFilesystem": True, "capabilities": {"drop": ["ALL"]}},
                        "resources": {"requests": {"cpu": "100m", "memory": "128Mi"}, "limits": {"cpu": "1", "memory": "512Mi"}},
                        "volumeMounts": [{"name": "artifacts", "mountPath": "/etc/alert-projection-repair", "readOnly": True}],
                    }],
                    "volumes": [{"name": "artifacts", "configMap": {"name": artifact_name}}],
                },
            },
        },
    }
    network_policy = {
        "apiVersion": "networking.k8s.io/v1", "kind": "NetworkPolicy",
        "metadata": {"name": job_name, "namespace": namespace, "labels": labels},
        "spec": {
            "podSelector": {"matchLabels": labels}, "policyTypes": ["Ingress", "Egress"], "ingress": [],
            "egress": [
                {"to": [{"namespaceSelector": {"matchLabels": {"kubernetes.io/metadata.name": "kube-system"}}}], "ports": [{"protocol": "UDP", "port": 53}, {"protocol": "TCP", "port": 53}]},
                {"to": [{"namespaceSelector": {"matchLabels": {"kubernetes.io/metadata.name": "middleware"}}}], "ports": [{"protocol": "TCP", "port": 9000}, {"protocol": "TCP", "port": 9200}]},
                {"to": [{"namespaceSelector": {"matchLabels": {"kubernetes.io/metadata.name": "databases"}}}], "ports": [{"protocol": "TCP", "port": 5432}]},
            ],
        },
    }
    return [service_account, artifacts, network_policy, job]


def main() -> int:
    parser = argparse.ArgumentParser()
    parser.add_argument("--review-package", type=Path, required=True)
    parser.add_argument("--approval-bundle", type=Path, required=True)
    parser.add_argument("--run-id", required=True)
    parser.add_argument("--namespace", default="applications")
    parser.add_argument("--output", type=Path, required=True)
    args = parser.parse_args()
    output = args.output.resolve()
    if output.exists():
        raise SystemExit(f"refusing to overwrite rendered repair candidate: {output}")
    try:
        review_path = args.review_package.resolve()
        approval_path = args.approval_bundle.resolve()
        review = load_json(review_path)
        approval = load_json(approval_path)
        review_sha = file_sha256(review_path)
        approval_sha = file_sha256(approval_path)
        image, requested_by, argv = validate_inputs(
            review, review_sha, approval, approval_sha,
            now=datetime.now(timezone.utc), current_content_sha256=str(build_snapshot()["content_sha256"]),
        )
        documents = render_documents(
            review=review, review_text=review_path.read_text(encoding="utf-8"), review_sha=review_sha,
            approval=approval, approval_text=approval_path.read_text(encoding="utf-8"), approval_sha=approval_sha,
            image=image, requested_by=requested_by, proposed_argv=argv, run_id=args.run_id,
            namespace=args.namespace,
        )
    except (OSError, json.JSONDecodeError, ValueError, TypeError, KeyError) as exc:
        raise SystemExit(str(exc)) from exc
    output.parent.mkdir(parents=True, exist_ok=True)
    output.write_text(yaml.safe_dump_all(documents, sort_keys=False), encoding="utf-8")
    print(json.dumps({
        "status": "RENDERED_SUSPENDED", "output": str(output), "output_sha256": file_sha256(output),
        "image": image, "execution_authorized_by_bundle": True, "job_suspended": True,
        "production_applied": False, "production_mutations": [],
    }, ensure_ascii=False))
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
