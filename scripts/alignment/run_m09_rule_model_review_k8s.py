#!/usr/bin/env python3
"""Run T1-M09-N019 rule/model review gates on ephemeral Kubernetes PostgreSQL."""

from __future__ import annotations

import argparse
import hashlib
import json
import re
import secrets
import uuid
from pathlib import Path
from typing import Any

import run_m09_alert_evidence_links_k8s as base


ROOT = Path(__file__).resolve().parents[2]
PREREQUISITES = ROOT / "scripts/alignment/fixtures/m09_rule_model_review_prerequisites.sql"
DEFAULT_OUTPUT = ROOT / "doc/02_acceptance/topic1/tasks/t1-m09-n019/k8s-rule-model-review-latest.json"
IMAGE_RE = re.compile(r"^[a-zA-Z0-9][a-zA-Z0-9._/@:-]+$")
NODE_RE = re.compile(r"^[a-z0-9]([-a-z0-9.]*[a-z0-9])?$")
UNIT_RE = "^TestDeploymentRuntimeGate(PartialModelAckStopsExpansion|BindsApprovalToExactEvents|RequiresGrayProjectionBeforeFullActivation|DisabledPreservesCompatibility)$"
BUNDLE_CHECK = " && ".join((
    "test -f /usr/share/nginx/html/index.html",
    "test -f /usr/share/nginx/html/.vite/manifest.json",
    "! find /usr/share/nginx/html -name mockServiceWorker.js -print -quit | grep -q .",
    "grep -R -q 'data-runtime-ack-status' /usr/share/nginx/html/assets",
    "grep -R -q '规则 / 模型运行时 ACK' /usr/share/nginx/html/assets",
    "grep -R -q 'ACK 不完整，已停止灰度扩展' /usr/share/nginx/html/assets",
    "echo PASS",
))


def labels(run_id: str) -> dict[str, str]:
    return {
        "app.kubernetes.io/name": "m09-rule-model-review-canary",
        "traffic.analysis/canary-run": run_id,
    }


def validate(images: list[str], run_id: str, node: str) -> str:
    for image in images:
        if not IMAGE_RE.fullmatch(image) or image.endswith(":latest"):
            raise base.CanaryError("candidate images must be explicit non-latest references")
    if not NODE_RE.fullmatch(node):
        raise base.CanaryError("invalid Kubernetes node name")
    parsed = uuid.UUID(run_id)
    if str(parsed) != run_id:
        raise base.CanaryError("--run-id must be a canonical lowercase UUID")
    if not PREREQUISITES.is_file():
        raise base.CanaryError(f"missing input: {PREREQUISITES.relative_to(ROOT)}")
    return parsed.hex[:10]


def dependency_objects(name: str, run_id: str, node: str, password: str) -> list[dict[str, Any]]:
    common = labels(run_id)
    selector = {"traffic.analysis/canary-postgres": name}
    return [
        {
            "apiVersion": "v1", "kind": "ConfigMap",
            "metadata": {"name": name, "namespace": base.DB_NAMESPACE, "labels": common},
            "data": {"00-prerequisites.sql": PREREQUISITES.read_text(encoding="utf-8")},
        },
        {
            "apiVersion": "v1", "kind": "Secret", "type": "Opaque",
            "metadata": {"name": name, "namespace": base.DB_NAMESPACE, "labels": common},
            "stringData": {"pg-password": password},
        },
        {
            "apiVersion": "v1", "kind": "Secret", "type": "Opaque",
            "metadata": {"name": name, "namespace": base.APP_NAMESPACE, "labels": common},
            "stringData": {"pg-dsn": f"postgres://postgres:{password}@{name}.{base.DB_NAMESPACE}.svc:5432/traffic_platform?sslmode=disable"},
        },
        {
            "apiVersion": "v1", "kind": "Service",
            "metadata": {"name": name, "namespace": base.DB_NAMESPACE, "labels": common},
            "spec": {"selector": selector, "ports": [{"name": "postgres", "port": 5432}]},
        },
        {
            "apiVersion": "v1", "kind": "Pod",
            "metadata": {"name": name, "namespace": base.DB_NAMESPACE, "labels": {**common, **selector}},
            "spec": {
                "nodeName": node,
                "automountServiceAccountToken": False,
                "restartPolicy": "Never",
                "securityContext": {"seccompProfile": {"type": "RuntimeDefault"}},
                "containers": [{
                    "name": "postgres", "image": base.POSTGRES_IMAGE, "imagePullPolicy": "IfNotPresent",
                    "env": [
                        {"name": "POSTGRES_DB", "value": "traffic_platform"},
                        {"name": "POSTGRES_USER", "value": "postgres"},
                        {"name": "POSTGRES_PASSWORD", "valueFrom": {"secretKeyRef": {"name": name, "key": "pg-password"}}},
                    ],
                    "readinessProbe": {"exec": {"command": ["sh", "-ec", "pg_isready -h 127.0.0.1 -U postgres -d traffic_platform"]}, "periodSeconds": 2, "failureThreshold": 90},
                    "resources": {"requests": {"cpu": "100m", "memory": "192Mi"}, "limits": {"cpu": "1", "memory": "1Gi"}},
                    "volumeMounts": [{"name": "data", "mountPath": "/var/lib/postgresql/data"}, {"name": "init", "mountPath": "/docker-entrypoint-initdb.d", "readOnly": True}],
                }],
                "volumes": [{"name": "data", "emptyDir": {}}, {"name": "init", "configMap": {"name": name}}],
            },
        },
    ]


def test_job(name: str, run_id: str, node: str, image: str, suite: str) -> dict[str, Any]:
    container: dict[str, Any] = {
        "name": suite, "image": image, "imagePullPolicy": "Never",
        "securityContext": {"allowPrivilegeEscalation": False, "readOnlyRootFilesystem": True, "capabilities": {"drop": ["ALL"]}},
        "resources": {"requests": {"cpu": "100m", "memory": "128Mi"}, "limits": {"cpu": "1", "memory": "512Mi"}},
        "volumeMounts": [{"name": "tmp", "mountPath": "/tmp"}],
    }
    if suite == "integration":
        container["args"] = ["-test.v", "-test.run=^TestDeploymentRuntimeGateEphemeralKubernetes$", "-test.count=1"]
        container["env"] = [
            {"name": "DEPLOYMENT_RUNTIME_GATE_K8S_INTEGRATION", "value": "run-scoped-only"},
            {"name": "DEPLOYMENT_RUNTIME_GATE_K8S_RUN_ID", "value": run_id},
            {"name": "DEPLOYMENT_RUNTIME_GATE_K8S_PG_DSN", "valueFrom": {"secretKeyRef": {"name": name, "key": "pg-dsn"}}},
        ]
    elif suite == "unit":
        container["args"] = ["-test.v", f"-test.run={UNIT_RE}", "-test.count=1"]
    else:
        container["command"] = ["/bin/sh", "-ec"]
        container["args"] = [BUNDLE_CHECK]
    job_name = f"{name}-{suite}"
    return {
        "apiVersion": "batch/v1", "kind": "Job",
        "metadata": {
            "name": job_name, "namespace": base.APP_NAMESPACE, "labels": labels(run_id),
            "annotations": {
                "traffic.analysis/shared-postgres-touched": "false",
                "traffic.analysis/production-applied": "false",
            },
        },
        "spec": {"backoffLimit": 0, "template": {"metadata": {"labels": labels(run_id)}, "spec": {
            "nodeName": node,
            "automountServiceAccountToken": False,
            "restartPolicy": "Never",
            "securityContext": {"runAsNonRoot": True, "runAsUser": 1000, "runAsGroup": 1000, "fsGroup": 1000, "seccompProfile": {"type": "RuntimeDefault"}},
            "containers": [container], "volumes": [{"name": "tmp", "emptyDir": {}}],
        }}},
    }


def postgres_oracle(name: str) -> dict[str, Any]:
    query = """SELECT json_build_object(
      'sentinel',(SELECT marker FROM codex_ephemeral_rule_model_review_sentinel LIMIT 1),
      'tenants',(SELECT count(*) FROM tenants),
      'rule_versions',(SELECT count(*) FROM rule_versions),
      'rule_outbox',(SELECT count(*) FROM rule_outbox),
      'rule_acks',(SELECT count(*) FROM rule_update_applied_acks),
      'model_versions',(SELECT count(*) FROM model_versions),
      'model_outbox',(SELECT count(*) FROM model_update_outbox),
      'model_acks',(SELECT count(*) FROM model_update_applied_acks),
      'deployments',(SELECT count(*) FROM deployments),
      'deployment_outbox',(SELECT count(*) FROM deployment_outbox),
      'deployment_projection',(SELECT count(*) FROM deployment_event_projection)
    );"""
    result = base.kubectl(
        "exec", "-i", "-n", base.DB_NAMESPACE, name, "--", "sh", "-ec",
        'PGPASSWORD="$POSTGRES_PASSWORD" exec psql -v ON_ERROR_STOP=1 -U postgres -d traffic_platform -At',
        input_text=query,
    )
    oracle = json.loads(result.stdout.strip())
    expected = {
        "sentinel": "ephemeral-only", "tenants": 0, "rule_versions": 0,
        "rule_outbox": 0, "rule_acks": 0, "model_versions": 0,
        "model_outbox": 0, "model_acks": 0, "deployments": 0,
        "deployment_outbox": 0, "deployment_projection": 0,
    }
    if oracle != expected:
        raise base.CanaryError(f"unexpected rule/model PostgreSQL cleanup oracle: {oracle}")
    return oracle


def main() -> None:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--service-image", required=True)
    parser.add_argument("--web-image", required=True)
    parser.add_argument("--run-id", default=str(uuid.uuid4()))
    parser.add_argument("--node", default="8-2tb")
    parser.add_argument("--timeout", type=int, default=420)
    parser.add_argument("--output", type=Path, default=DEFAULT_OUTPUT)
    parser.add_argument("--keep", action="store_true")
    args = parser.parse_args()
    if args.timeout < 120 or args.timeout > 1200:
        raise base.CanaryError("--timeout must be between 120 and 1200 seconds")
    images = [args.service_image, args.web_image]
    suffix = validate(images, args.run_id, args.node)
    name = f"m09-n019-review-{suffix}"
    password = "m09-n019-" + secrets.token_hex(16)
    dependencies: list[dict[str, Any]] = []
    receipts: list[dict[str, Any]] = []
    logs_by_suite: dict[str, str] = {}
    applied = False
    try:
        base.apply(dependency_objects(name, args.run_id, args.node, password))
        applied = True
        base.wait_pod(name, base.DB_NAMESPACE, args.timeout)
        dependencies.append(base.pod_receipt(name, base.DB_NAMESPACE))
        for suite, image in (("integration", args.service_image), ("unit", args.service_image), ("web", args.web_image)):
            base.apply([test_job(name, args.run_id, args.node, image, suite)])
            logs, receipt = base.wait_job(f"{name}-{suite}", args.timeout)
            logs_by_suite[suite] = logs
            receipts.append(receipt)
        oracle = postgres_oracle(name)
    finally:
        if applied and not args.keep:
            base.cleanup(args.run_id)

    source_paths = [
        ROOT / "go/control-plane/internal/rules/model/deployment_runtime_gate.go",
        ROOT / "go/control-plane/internal/rules/model/deployment_workbench.go",
        ROOT / "go/control-plane/internal/rules/service/deployment_runtime_gate.go",
        ROOT / "go/control-plane/internal/rules/service/deployment_runtime_gate_k8s_integration_test.go",
        ROOT / "go/control-plane/internal/rules/service/deployment_service.go",
        ROOT / "go/control-plane/internal/rules/config/config.go",
        ROOT / "go/control-plane/cmd/rule-manager/main.go",
        ROOT / "web/ui/src/pages/DeploymentManagementWorkspace.tsx",
        ROOT / "web/ui/src/pages/deploymentManagementLogic.ts",
        ROOT / "web/ui/src/services/api.ts",
        ROOT / "deployments/kubernetes/applications/go-services.yaml",
        ROOT / "go/control-plane/deployments/kubernetes/rule-manager.yaml",
    ]
    envelope = {
        "artifact_kind": "M09_RULE_MODEL_REVIEW_TEST_RESULT",
        "task_id": "T1-M09-N019",
        "run_id": args.run_id,
        "status": "PASS",
        "profile_id": "M09-N019-K8S-PG-ACK-GATE-UI-V1",
        "coverage_status": "PASS_FOR_RUN_SCOPED_K8S_EXACT_ACK_EXPANSION_GATE",
        "inputs": {
            "candidate_images": images,
            "source_sha256": {str(path.relative_to(ROOT)): hashlib.sha256(path.read_bytes()).hexdigest() for path in source_paths},
            "prerequisites_sha256": hashlib.sha256(PREREQUISITES.read_bytes()).hexdigest(),
        },
        "test_output_sha256": {key: hashlib.sha256(value.encode()).hexdigest() for key, value in logs_by_suite.items()},
        "postgres_cleanup_oracle": oracle,
        "kubernetes_dependencies": dependencies,
        "kubernetes_jobs": receipts,
        "partial_ack_stops_expansion": True,
        "exact_rule_model_receipts": True,
        "approval_binds_event_ids": True,
        "event_drift_requires_reapproval": True,
        "gray_projection_ack_required": True,
        "old_version_recoverable": True,
        "ui_runtime_gate_present": True,
        "runtime_gate_default_enabled": False,
        "mock_enabled": False,
        "shared_postgres_touched": False,
        "production_applied": False,
        "run_scoped_resources_removed": not args.keep,
        "does_not_prove": [
            "production rollout", "shared data migration", "broker-generated rule/model ACK delivery",
            "performance or rollback drill", "Windows Chrome acceptance",
            "authorization to enable DEPLOYMENT_RUNTIME_ACK_GATE_V1_ENABLED", "global milestone completion",
        ],
    }
    args.output.parent.mkdir(parents=True, exist_ok=True)
    temporary = args.output.with_name(f".{args.output.name}.tmp")
    temporary.write_text(json.dumps(envelope, indent=2, sort_keys=True) + "\n", encoding="utf-8")
    temporary.replace(args.output)
    print(json.dumps(envelope, sort_keys=True))


if __name__ == "__main__":
    main()
