#!/usr/bin/env python3
"""Run T1-M09-N020 response handoff and provider-receipt tests in Kubernetes."""

from __future__ import annotations

import argparse
import hashlib
import json
import re
import secrets
import uuid
from pathlib import Path
from typing import Any

import yaml

import run_m09_alert_evidence_links_k8s as base


ROOT = Path(__file__).resolve().parents[2]
SQL_ROOT = ROOT / "common/sql/pg"
DEFAULT_OUTPUT = ROOT / "doc/02_acceptance/topic1/tasks/t1-m09-n020/k8s-response-handoff-latest.json"
IMAGE_RE = re.compile(r"^[a-zA-Z0-9][a-zA-Z0-9._/@:-]+$")
NODE_RE = re.compile(r"^[a-z0-9]([-a-z0-9.]*[a-z0-9])?$")
API_INTEGRATION_RE = "^TestAlertResponse(WorkflowPostgresIntegration|CompensationQueuePostgresIntegration)$"
CONSUMER_INTEGRATION_RE = "^TestPostgresAlertResponse(ProjectionIntegration|ExternalExecutorIntegration)$"
UNIT_RE = "^Test(PostgresAlertResponseProjection(CompletesSimulation|BlocksLegacyRealEvent)|AlertResponseProjection(CommitsProviderReceiptStateAndAuditAtomically|RecordsUnknownEffectWithoutBlindRetry))$"
BUNDLE_CHECK = " && ".join((
    "test -f /usr/share/nginx/html/index.html",
    "test -f /usr/share/nginx/html/.vite/manifest.json",
    "! find /usr/share/nginx/html -name mockServiceWorker.js -print -quit | grep -q .",
    "grep -R -q 'alert-response-provider-receipt' /usr/share/nginx/html/assets",
    "grep -R -q 'Provider 回执' /usr/share/nginx/html/assets",
    "grep -R -q '等待执行回执' /usr/share/nginx/html/assets",
    "echo PASS",
))


def labels(run_id: str) -> dict[str, str]:
    return {
        "app.kubernetes.io/name": "m09-response-handoff-canary",
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
    if not SQL_ROOT.is_dir() or not list(SQL_ROOT.glob("*.sql")):
        raise base.CanaryError("common PostgreSQL schema inputs are missing")
    return parsed.hex[:10]


def apply(items: list[dict[str, Any]]) -> None:
    body = "\n---\n".join(yaml.safe_dump(item, sort_keys=False) for item in items)
    base.kubectl(
        "apply", "--server-side", "--field-manager=m09-n020-runner",
        "-f", "-", input_text=body,
    )


def postgres_config_data() -> dict[str, str]:
    data = {
        path.name: path.read_text(encoding="utf-8")
        for path in sorted(SQL_ROOT.glob("*.sql"))
    }
    data["99-m09-n020-guard.sql"] = (
        "CREATE TABLE remediation_ephemeral_guard (guard_value text primary key);\n"
        "INSERT INTO remediation_ephemeral_guard(guard_value) VALUES "
        "('alert-response-integration-v1');\n"
    )
    return data


def dependency_objects(name: str, run_id: str, node: str, password: str) -> list[dict[str, Any]]:
    common = labels(run_id)
    selector = {"traffic.analysis/canary-postgres": name}
    return [
        {
            "apiVersion": "v1", "kind": "ConfigMap",
            "metadata": {"name": name, "namespace": base.DB_NAMESPACE, "labels": common},
            "data": postgres_config_data(),
        },
        {
            "apiVersion": "v1", "kind": "Secret", "type": "Opaque",
            "metadata": {"name": name, "namespace": base.DB_NAMESPACE, "labels": common},
            "stringData": {"pg-password": password},
        },
        {
            "apiVersion": "v1", "kind": "Secret", "type": "Opaque",
            "metadata": {"name": name, "namespace": base.APP_NAMESPACE, "labels": common},
            "stringData": {
                "pg-dsn": f"postgres://postgres:{password}@{name}.{base.DB_NAMESPACE}.svc:5432/traffic_platform?sslmode=disable",
            },
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
                    "readinessProbe": {"exec": {"command": ["sh", "-ec", "pg_isready -h 127.0.0.1 -U postgres -d traffic_platform"]}, "periodSeconds": 2, "failureThreshold": 120},
                    "resources": {"requests": {"cpu": "100m", "memory": "256Mi"}, "limits": {"cpu": "1", "memory": "1Gi"}},
                    "volumeMounts": [
                        {"name": "data", "mountPath": "/var/lib/postgresql/data"},
                        {"name": "init", "mountPath": "/docker-entrypoint-initdb.d", "readOnly": True},
                    ],
                }],
                "volumes": [{"name": "data", "emptyDir": {}}, {"name": "init", "configMap": {"name": name}}],
            },
        },
    ]


def test_job(name: str, run_id: str, node: str, service_image: str, web_image: str, suite: str) -> dict[str, Any]:
    container: dict[str, Any] = {
        "name": suite,
        "image": web_image if suite == "web" else service_image,
        "imagePullPolicy": "Never",
        "securityContext": {"allowPrivilegeEscalation": False, "readOnlyRootFilesystem": True, "capabilities": {"drop": ["ALL"]}},
        "resources": {"requests": {"cpu": "100m", "memory": "128Mi"}, "limits": {"cpu": "1", "memory": "512Mi"}},
        "volumeMounts": [{"name": "tmp", "mountPath": "/tmp"}],
    }
    if suite == "web":
        container["command"] = ["/bin/sh", "-ec"]
        container["args"] = [BUNDLE_CHECK]
    else:
        binary = "/usr/local/bin/api-tests" if suite == "api-integration" else "/usr/local/bin/consumer-tests"
        pattern = API_INTEGRATION_RE if suite == "api-integration" else CONSUMER_INTEGRATION_RE if suite == "consumer-integration" else UNIT_RE
        container["command"] = [binary]
        container["args"] = ["-test.v", f"-test.run={pattern}", "-test.count=1"]
        if suite != "unit":
            container["env"] = [{
                "name": "ALERT_RESPONSE_EPHEMERAL_PG_DSN",
                "valueFrom": {"secretKeyRef": {"name": name, "key": "pg-dsn"}},
            }]
    job_name = f"{name}-{suite}"
    return {
        "apiVersion": "batch/v1", "kind": "Job",
        "metadata": {
            "name": job_name, "namespace": base.APP_NAMESPACE, "labels": labels(run_id),
            "annotations": {
                "traffic.analysis/shared-postgres-touched": "false",
                "traffic.analysis/direct-cleaning-owned": "false",
                "traffic.analysis/direct-blackhole-routing-owned": "false",
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
      'sentinel',(SELECT guard_value FROM remediation_ephemeral_guard LIMIT 1),
      'actions',(SELECT count(*) FROM alert_response_actions),
      'receipts',(SELECT count(*) FROM alert_response_execution_receipts),
      'simulated',(SELECT count(*) FROM alert_response_execution_receipts WHERE state='simulated_completed'),
      'blocked',(SELECT count(*) FROM alert_response_execution_receipts WHERE state='blocked_external_executor'),
      'completed',(SELECT count(*) FROM alert_response_execution_receipts WHERE state='completed'),
      'confirmed_external',(SELECT count(*) FROM alert_response_execution_receipts WHERE external_effect AND effect_state='confirmed'),
      'provider_receipts',(SELECT count(*) FROM alert_response_execution_receipts WHERE provider<>'' AND provider_receipt_id<>''),
      'audit',(SELECT count(*) FROM audit_logs WHERE action LIKE 'ALERT_RESPONSE_%')
    );"""
    result = base.kubectl(
        "exec", "-i", "-n", base.DB_NAMESPACE, name, "--", "sh", "-ec",
        'PGPASSWORD="$POSTGRES_PASSWORD" exec psql -v ON_ERROR_STOP=1 -U postgres -d traffic_platform -At',
        input_text=query,
    )
    oracle = json.loads(result.stdout.strip())
    if oracle.get("sentinel") != "alert-response-integration-v1":
        raise base.CanaryError(f"ephemeral PostgreSQL sentinel diverged: {oracle}")
    for field in ("actions", "receipts", "simulated", "blocked", "completed", "confirmed_external", "provider_receipts", "audit"):
        if int(oracle.get(field, 0)) < 1:
            raise base.CanaryError(f"response handoff PostgreSQL oracle lacks {field}: {oracle}")
    return oracle


def main() -> None:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--service-image", required=True)
    parser.add_argument("--web-image", required=True)
    parser.add_argument("--run-id", default=str(uuid.uuid4()))
    parser.add_argument("--node", default="8-2tb")
    parser.add_argument("--timeout", type=int, default=600)
    parser.add_argument("--output", type=Path, default=DEFAULT_OUTPUT)
    parser.add_argument("--keep", action="store_true")
    args = parser.parse_args()
    if args.timeout < 180 or args.timeout > 1200:
        raise base.CanaryError("--timeout must be between 180 and 1200 seconds")
    images = [args.service_image, args.web_image]
    suffix = validate(images, args.run_id, args.node)
    name = f"m09-n020-handoff-{suffix}"
    password = "m09-n020-" + secrets.token_hex(16)
    dependencies: list[dict[str, Any]] = []
    receipts: list[dict[str, Any]] = []
    logs_by_suite: dict[str, str] = {}
    applied = False
    try:
        applied = True
        apply(dependency_objects(name, args.run_id, args.node, password))
        base.wait_pod(name, base.DB_NAMESPACE, args.timeout)
        dependencies.append(base.pod_receipt(name, base.DB_NAMESPACE))
        for suite in ("api-integration", "consumer-integration", "unit", "web"):
            apply([test_job(name, args.run_id, args.node, args.service_image, args.web_image, suite)])
            logs, receipt = base.wait_job(f"{name}-{suite}", args.timeout)
            logs_by_suite[suite] = logs
            receipts.append(receipt)
        oracle = postgres_oracle(name)
    finally:
        if applied and not args.keep:
            base.cleanup(args.run_id)

    source_paths = [
        ROOT / "go/control-plane/internal/alert/api/handler_alert_actions.go",
        ROOT / "go/control-plane/internal/alert/api/handler_alert_actions_test.go",
        ROOT / "go/control-plane/internal/alert/api/handler_alert_response_workflow_integration_test.go",
        ROOT / "go/control-plane/internal/alert/consumer/alert_response_event_consumer.go",
        ROOT / "go/control-plane/internal/alert/consumer/alert_response_http_executor.go",
        ROOT / "web/ui/src/services/alertDetailActionApi.ts",
        ROOT / "web/ui/src/pages/AlertDetailPage.tsx",
        ROOT / "contracts/openapi/alignment-v1.openapi.json",
        ROOT / "go/control-plane/deployments/kubernetes/alert-service.yaml",
    ]
    envelope = {
        "artifact_kind": "M09_RESPONSE_HANDOFF_TEST_RESULT",
        "task_id": "T1-M09-N020",
        "run_id": args.run_id,
        "status": "PASS",
        "profile_id": "M09-N020-K8S-PG-PROVIDER-RECEIPT-UI-V1",
        "coverage_status": "PASS_FOR_RUN_SCOPED_K8S_RESPONSE_HANDOFF",
        "inputs": {
            "candidate_images": images,
            "source_sha256": {str(path.relative_to(ROOT)): hashlib.sha256(path.read_bytes()).hexdigest() for path in source_paths},
            "postgres_schema_sha256": {path.name: hashlib.sha256(path.read_bytes()).hexdigest() for path in sorted(SQL_ROOT.glob("*.sql"))},
        },
        "test_output_sha256": {key: hashlib.sha256(value.encode()).hexdigest() for key, value in logs_by_suite.items()},
        "postgres_oracle": oracle,
        "kubernetes_dependencies": dependencies,
        "kubernetes_jobs": receipts,
        "dry_run_receipt": True,
        "unconfigured_executor_fail_closed": True,
        "provider_receipt_persisted": True,
        "provider_receipt_read_api": True,
        "provider_receipt_visible_in_candidate_bundle": True,
        "execution_flags_default_enabled": False,
        "topic_one_direct_cleaning_owned": False,
        "topic_one_direct_blackhole_routing_owned": False,
        "mock_enabled": False,
        "shared_postgres_touched": False,
        "production_applied": False,
        "run_scoped_resources_removed": not args.keep,
        "does_not_prove": [
            "production rollout", "shared data migration", "production provider authorization",
            "production traffic cleaning or blackhole routing", "performance or rollback drill",
            "Windows Chrome acceptance", "authorization to enable alert response execution flags",
            "global milestone completion",
        ],
    }
    args.output.parent.mkdir(parents=True, exist_ok=True)
    temporary = args.output.with_name(f".{args.output.name}.tmp")
    temporary.write_text(json.dumps(envelope, indent=2, sort_keys=True) + "\n", encoding="utf-8")
    temporary.replace(args.output)
    print(json.dumps(envelope, sort_keys=True))


if __name__ == "__main__":
    main()
