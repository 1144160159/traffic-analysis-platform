#!/usr/bin/env python3
"""Run T1-M09-N018 on run-scoped PostgreSQL and Kafka inside Kubernetes."""

from __future__ import annotations

import argparse
import hashlib
import json
import re
import secrets
import time
import uuid
from pathlib import Path
from typing import Any

import run_m09_alert_evidence_links_k8s as base


ROOT = Path(__file__).resolve().parents[2]
TOPIC = "whitelist.events.v2"
PREREQUISITES = ROOT / "scripts/alignment/fixtures/m09_whitelist_governance_prerequisites.sql"
GOVERNANCE_MIGRATION = ROOT / "deployments/postgres/migrations/202608031610_whitelist_governance_v2.sql"
PROJECTION_MIGRATION = ROOT / "deployments/postgres/migrations/202608071930_whitelist_rule_projection_v1.sql"
READINESS_MIGRATION = ROOT / "deployments/postgres/migrations/202608161100_m09_whitelist_consumer_readiness_v2.sql"
DEFAULT_OUTPUT = ROOT / "doc/02_acceptance/topic1/tasks/t1-m09-n018/k8s-whitelist-governance-latest.json"
IMAGE_RE = re.compile(r"^[a-zA-Z0-9][a-zA-Z0-9._/@:-]+$")
NODE_RE = re.compile(r"^[a-z0-9]([-a-z0-9.]*[a-z0-9])?$")
CONSUMER_UNIT = "^Test(WhitelistRuleConsumer|PostgresWhitelistProjection|WhitelistProjectionReadiness)"
AUTHORITY_UNIT = "^Test(WhitelistTransition|WhitelistUpdateShape|WhitelistCommandReason|WhitelistStatePairs|WhitelistEntrySupports|VerifyProducerReadiness)"
BUNDLE_CHECK = " && ".join((
    "test -f /usr/share/nginx/html/index.html",
    "test -f /usr/share/nginx/html/.vite/manifest.json",
    "! find /usr/share/nginx/html -name mockServiceWorker.js -print -quit | grep -q .",
    "grep -R -q 'command_reason' /usr/share/nginx/html/assets",
    "grep -R -q 'rule_ack_event_id' /usr/share/nginx/html/assets",
    "grep -R -q '审计与规则投影 ACK' /usr/share/nginx/html/assets",
    "echo PASS",
))


def labels(run_id: str) -> dict[str, str]:
    return {
        "app.kubernetes.io/name": "m09-whitelist-governance-canary",
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
    for path in (PREREQUISITES, GOVERNANCE_MIGRATION, PROJECTION_MIGRATION, READINESS_MIGRATION):
        if not path.is_file():
            raise base.CanaryError(f"missing input: {path.relative_to(ROOT)}")
    return parsed.hex[:10]


def dependency_objects(name: str, run_id: str, node: str, password: str) -> list[dict[str, Any]]:
    common = labels(run_id)
    pg_selector = {"traffic.analysis/canary-postgres": name}
    kafka_selector = {"traffic.analysis/canary-kafka": name}
    kafka_name = f"{name}-kafka"
    kafka_dns = f"{kafka_name}.{base.DB_NAMESPACE}.svc"
    return [
        {
            "apiVersion": "v1", "kind": "ConfigMap",
            "metadata": {"name": name, "namespace": base.DB_NAMESPACE, "labels": common},
            "data": {
                "00-prerequisites.sql": PREREQUISITES.read_text(encoding="utf-8"),
                "10-governance.sql": GOVERNANCE_MIGRATION.read_text(encoding="utf-8"),
                "20-projection.sql": PROJECTION_MIGRATION.read_text(encoding="utf-8"),
                "30-readiness.sql": READINESS_MIGRATION.read_text(encoding="utf-8"),
            },
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
            "spec": {"selector": pg_selector, "ports": [{"name": "postgres", "port": 5432}]},
        },
        {
            "apiVersion": "v1", "kind": "Service",
            "metadata": {"name": kafka_name, "namespace": base.DB_NAMESPACE, "labels": common},
            "spec": {"selector": kafka_selector, "ports": [{"name": "kafka", "port": 9092}]},
        },
        {
            "apiVersion": "v1", "kind": "Pod",
            "metadata": {"name": name, "namespace": base.DB_NAMESPACE, "labels": {**common, **pg_selector}},
            "spec": {
                "nodeName": node, "automountServiceAccountToken": False, "restartPolicy": "Never",
                "securityContext": {"seccompProfile": {"type": "RuntimeDefault"}},
                "containers": [{
                    "name": "postgres", "image": base.POSTGRES_IMAGE, "imagePullPolicy": "IfNotPresent",
                    "env": [
                        {"name": "POSTGRES_DB", "value": "traffic_platform"},
                        {"name": "POSTGRES_USER", "value": "postgres"},
                        {"name": "POSTGRES_PASSWORD", "valueFrom": {"secretKeyRef": {"name": name, "key": "pg-password"}}},
                    ],
                    "ports": [{"containerPort": 5432}],
                    "readinessProbe": {"exec": {"command": ["sh", "-ec", "pg_isready -h 127.0.0.1 -U postgres -d traffic_platform"]}, "periodSeconds": 2, "failureThreshold": 90},
                    "resources": {"requests": {"cpu": "100m", "memory": "192Mi"}, "limits": {"cpu": "1", "memory": "1Gi"}},
                    "volumeMounts": [{"name": "data", "mountPath": "/var/lib/postgresql/data"}, {"name": "init", "mountPath": "/docker-entrypoint-initdb.d", "readOnly": True}],
                }],
                "volumes": [{"name": "data", "emptyDir": {}}, {"name": "init", "configMap": {"name": name}}],
            },
        },
        {
            "apiVersion": "v1", "kind": "Pod",
            "metadata": {"name": kafka_name, "namespace": base.DB_NAMESPACE, "labels": {**common, **kafka_selector}},
            "spec": {
                "nodeName": node, "automountServiceAccountToken": False, "restartPolicy": "Never",
                "securityContext": {"seccompProfile": {"type": "RuntimeDefault"}},
                "containers": [{
                    "name": "redpanda", "image": base.KAFKA_IMAGE, "imagePullPolicy": "Never",
                    "args": [
                        "start", "--mode", "dev-container", "--check=false", "--smp", "1",
                        "--memory", "768M", "--reserve-memory", "0M",
                        "--kafka-addr", "internal://0.0.0.0:9092",
                        "--advertise-kafka-addr", f"internal://{kafka_dns}:9092",
                        "--rpc-addr", "0.0.0.0:33145", "--advertise-rpc-addr", f"{kafka_dns}:33145",
                    ],
                    "ports": [{"containerPort": 9092}, {"containerPort": 33145}],
                    "readinessProbe": {"exec": {"command": ["rpk", "cluster", "health"]}, "periodSeconds": 2, "failureThreshold": 90},
                    "resources": {"requests": {"cpu": "200m", "memory": "768Mi"}, "limits": {"cpu": "2", "memory": "1Gi"}},
                    "volumeMounts": [{"name": "data", "mountPath": "/var/lib/redpanda/data"}],
                }],
                "volumes": [{"name": "data", "emptyDir": {}}],
            },
        },
    ]


def create_topic(name: str, timeout: int) -> dict[str, Any]:
    pod = f"{name}-kafka"
    deadline = time.monotonic() + min(timeout, 90)
    readiness = None
    while time.monotonic() < deadline:
        readiness = base.kubectl(
            "exec", "-n", base.DB_NAMESPACE, pod, "--", "rpk", "cluster", "info",
            "--brokers", "127.0.0.1:9092", check=False,
        )
        if readiness.returncode == 0:
            break
        time.sleep(1)
    else:
        stderr = readiness.stderr if readiness is not None else "no readiness attempt"
        raise base.CanaryError(f"whitelist Kafka advertised listener did not become ready:\n{stderr}")
    base.kubectl("exec", "-n", base.DB_NAMESPACE, pod, "--", "rpk", "topic", "create", TOPIC,
                 "--brokers", "127.0.0.1:9092", "--partitions", "3", "--replicas", "1",
                 "-c", "cleanup.policy=delete", "-c", "retention.ms=3600000")
    description = base.kubectl("exec", "-n", base.DB_NAMESPACE, pod, "--", "rpk", "topic", "describe", TOPIC,
                               "--brokers", "127.0.0.1:9092", "-p").stdout
    partitions = {line.split()[0] for line in description.splitlines()[1:] if line.split() and line.split()[0].isdigit()}
    if partitions != {"0", "1", "2"}:
        raise base.CanaryError(f"whitelist topic partition receipt mismatch:\n{description}")
    return {"topic": TOPIC, "partitions": 3, "replication_factor": 1,
            "description_sha256": hashlib.sha256(description.encode()).hexdigest()}


def test_job(name: str, run_id: str, node: str, image: str, suite: str) -> dict[str, Any]:
    container: dict[str, Any] = {
        "name": suite, "image": image, "imagePullPolicy": "Never",
        "securityContext": {"allowPrivilegeEscalation": False, "readOnlyRootFilesystem": True, "capabilities": {"drop": ["ALL"]}},
        "resources": {"requests": {"cpu": "100m", "memory": "128Mi"}, "limits": {"cpu": "1", "memory": "512Mi"}},
        "volumeMounts": [{"name": "tmp", "mountPath": "/tmp"}],
    }
    if suite == "integration":
        container["args"] = ["-test.v", "-test.run=^TestWhitelistGovernanceEphemeralKubernetes$", "-test.count=1"]
        container["env"] = [
            {"name": "WHITELIST_GOVERNANCE_K8S_INTEGRATION", "value": "run-scoped-only"},
            {"name": "WHITELIST_GOVERNANCE_K8S_RUN_ID", "value": run_id},
            {"name": "WHITELIST_GOVERNANCE_K8S_PG_DSN", "valueFrom": {"secretKeyRef": {"name": name, "key": "pg-dsn"}}},
            {"name": "WHITELIST_GOVERNANCE_K8S_KAFKA_BROKER", "value": f"{name}-kafka.{base.DB_NAMESPACE}.svc:9092"},
        ]
    elif suite == "consumer-unit":
        container["args"] = ["-test.v", f"-test.run={CONSUMER_UNIT}", "-test.count=1"]
    elif suite == "authority-unit":
        container["args"] = ["-test.v", f"-test.run={AUTHORITY_UNIT}", "-test.count=1"]
    else:
        container["command"] = ["/bin/sh", "-ec"]
        container["args"] = [BUNDLE_CHECK]
    job_name = f"{name}-{suite}"
    return {
        "apiVersion": "batch/v1", "kind": "Job",
        "metadata": {"name": job_name, "namespace": base.APP_NAMESPACE, "labels": labels(run_id),
                     "annotations": {"traffic.analysis/shared-postgres-touched": "false", "traffic.analysis/shared-kafka-touched": "false", "traffic.analysis/production-applied": "false"}},
        "spec": {"backoffLimit": 0, "template": {"metadata": {"labels": labels(run_id)}, "spec": {
            "nodeName": node, "automountServiceAccountToken": False, "restartPolicy": "Never",
            "securityContext": {"runAsNonRoot": True, "runAsUser": 1000, "runAsGroup": 1000, "fsGroup": 1000, "seccompProfile": {"type": "RuntimeDefault"}},
            "containers": [container], "volumes": [{"name": "tmp", "emptyDir": {}}],
        }}},
    }


def postgres_oracle(name: str) -> dict[str, Any]:
    query = """SELECT json_build_object(
      'governance_migration',(SELECT count(*) FROM alignment_schema_migrations WHERE version='202608031610'),
      'projection_migration',(SELECT count(*) FROM alignment_schema_migrations WHERE version='202608071930'),
      'readiness_migration',(SELECT count(*) FROM alignment_schema_migrations WHERE version='202608161100'),
      'sentinel',(SELECT marker FROM codex_ephemeral_whitelist_governance_sentinel LIMIT 1),
      'tenants',(SELECT count(*) FROM tenants),
      'whitelist',(SELECT count(*) FROM whitelist),
      'history',(SELECT count(*) FROM whitelist_entry_versions),
      'outbox',(SELECT count(*) FROM whitelist_event_outbox),
      'effects',(SELECT count(*) FROM whitelist_rule_effects),
      'projection',(SELECT count(*) FROM whitelist_rule_projection),
      'readiness',(SELECT count(*) FROM whitelist_consumer_readiness_receipt),
      'audit',(SELECT count(*) FROM audit_logs));"""
    result = base.kubectl("exec", "-i", "-n", base.DB_NAMESPACE, name, "--", "sh", "-ec",
                          'PGPASSWORD="$POSTGRES_PASSWORD" exec psql -v ON_ERROR_STOP=1 -U postgres -d traffic_platform -At', input_text=query)
    oracle = json.loads(result.stdout.strip())
    expected = {"governance_migration": 1, "projection_migration": 1, "readiness_migration": 1,
                "sentinel": "ephemeral-only", "tenants": 0, "whitelist": 0, "history": 0,
                "outbox": 0, "effects": 0, "projection": 0, "readiness": 0, "audit": 0}
    if oracle != expected:
        raise base.CanaryError(f"unexpected whitelist PostgreSQL cleanup oracle: {oracle}")
    return oracle


def main() -> None:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--consumer-image", required=True)
    parser.add_argument("--authority-image", required=True)
    parser.add_argument("--web-image", required=True)
    parser.add_argument("--run-id", default=str(uuid.uuid4()))
    parser.add_argument("--node", default="8-2tb")
    parser.add_argument("--timeout", type=int, default=420)
    parser.add_argument("--output", type=Path, default=DEFAULT_OUTPUT)
    parser.add_argument("--keep", action="store_true")
    args = parser.parse_args()
    if args.timeout < 120 or args.timeout > 1200:
        raise base.CanaryError("--timeout must be between 120 and 1200 seconds")
    images = [args.consumer_image, args.authority_image, args.web_image]
    suffix = validate(images, args.run_id, args.node)
    name = f"m09-n018-whitelist-{suffix}"
    password = "m09-n018-" + secrets.token_hex(16)
    dependencies: list[dict[str, Any]] = []
    receipts: list[dict[str, Any]] = []
    logs_by_suite: dict[str, str] = {}
    applied = False
    try:
        base.apply(dependency_objects(name, args.run_id, args.node, password))
        applied = True
        for pod_name in (name, f"{name}-kafka"):
            base.wait_pod(pod_name, base.DB_NAMESPACE, args.timeout)
            dependencies.append(base.pod_receipt(pod_name, base.DB_NAMESPACE))
        topic_receipt = create_topic(name, args.timeout)
        suites = (("integration", args.consumer_image), ("consumer-unit", args.consumer_image),
                  ("authority-unit", args.authority_image), ("web", args.web_image))
        for suite, image in suites:
            base.apply([test_job(name, args.run_id, args.node, image, suite)])
            logs, receipt = base.wait_job(f"{name}-{suite}", args.timeout)
            logs_by_suite[suite] = logs
            receipts.append(receipt)
        oracle = postgres_oracle(name)
    finally:
        if applied and not args.keep:
            base.cleanup(args.run_id)

    source_paths = [
        ROOT / "go/control-plane/internal/alert/whitelist/command_atomic.go",
        ROOT / "go/control-plane/internal/alert/whitelist/producer_readiness.go",
        ROOT / "go/control-plane/internal/alert/whitelist/handler.go",
        ROOT / "go/control-plane/internal/alert/whitelist/whitelist.go",
        ROOT / "go/control-plane/internal/rules/consumer/whitelist_rule_effect_consumer.go",
        ROOT / "go/control-plane/cmd/alert-service/main.go",
        ROOT / "go/control-plane/cmd/rule-manager/main.go",
        ROOT / "web/ui/src/services/whitelistGovernanceApi.ts",
        ROOT / "web/ui/src/pages/WhitelistGovernancePage.tsx",
        ROOT / "contracts/openapi/alignment-v1.openapi.json",
        ROOT / "contracts/events/kafka-json-events-v1.schema.json",
        ROOT / "deployments/kubernetes/applications/go-services.yaml",
    ]
    envelope = {
        "artifact_kind": "M09_WHITELIST_GOVERNANCE_TEST_RESULT", "task_id": "T1-M09-N018",
        "run_id": args.run_id, "status": "PASS", "profile_id": "M09-N018-K8S-PG-KAFKA-UI-V1",
        "coverage_status": "PASS_FOR_RUN_SCOPED_K8S_DRAFT_APPROVAL_EXPIRY_PROJECTION_ACK",
        "inputs": {
            "candidate_images": images,
            "source_sha256": {str(path.relative_to(ROOT)): hashlib.sha256(path.read_bytes()).hexdigest() for path in source_paths},
            "prerequisites_sha256": hashlib.sha256(PREREQUISITES.read_bytes()).hexdigest(),
            "governance_migration_sha256": hashlib.sha256(GOVERNANCE_MIGRATION.read_bytes()).hexdigest(),
            "projection_migration_sha256": hashlib.sha256(PROJECTION_MIGRATION.read_bytes()).hexdigest(),
            "readiness_migration_sha256": hashlib.sha256(READINESS_MIGRATION.read_bytes()).hexdigest(),
        },
        "test_output_sha256": {key: hashlib.sha256(value.encode()).hexdigest() for key, value in logs_by_suite.items()},
        "topic_receipt": topic_receipt, "postgres_cleanup_oracle": oracle,
        "kubernetes_dependencies": dependencies, "kubernetes_jobs": receipts,
        "draft_from_fp_feedback": True, "two_person_approval": True, "expiry_sweeper": True,
        "broker_projection_ack": True, "deterministic_rule_revision": True,
        "producer_readiness_join": True, "producer_default_enabled": False,
        "consumer_default_enabled": False, "detection_matcher_default_enabled": False,
        "real_network_blocking_executed": False, "mock_enabled": False,
        "shared_postgres_touched": False, "shared_kafka_touched": False,
        "production_applied": False, "run_scoped_resources_removed": not args.keep,
        "does_not_prove": ["production rollout", "shared data migration", "performance or rollback drill",
                           "Windows Chrome acceptance", "authorization to enable whitelist producer or matcher", "global milestone completion"],
    }
    args.output.parent.mkdir(parents=True, exist_ok=True)
    temporary = args.output.with_name(f".{args.output.name}.tmp")
    temporary.write_text(json.dumps(envelope, indent=2, sort_keys=True) + "\n", encoding="utf-8")
    temporary.replace(args.output)
    print(json.dumps(envelope, sort_keys=True))


if __name__ == "__main__":
    main()
