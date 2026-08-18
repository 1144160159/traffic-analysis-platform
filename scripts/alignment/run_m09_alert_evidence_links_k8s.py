#!/usr/bin/env python3
"""Run the M09-N012 alert evidence link pipeline on run-scoped K8s dependencies."""

from __future__ import annotations

import argparse
import hashlib
import json
import re
import secrets
import subprocess
import time
import uuid
from pathlib import Path
from typing import Any

import yaml


ROOT = Path(__file__).resolve().parents[2]
APP_NAMESPACE = "traffic-analysis"
DB_NAMESPACE = "databases"
POSTGRES_IMAGE = "docker.io/library/postgres:16-alpine"
CLICKHOUSE_IMAGE = "docker.io/clickhouse/clickhouse-server:24.3-alpine"
KAFKA_IMAGE = (
    "docker.io/redpandadata/redpanda@sha256:"
    "dca9d37efbbae3c2dcdc07d6a45fa1e0a7a541bc9cdc03db3937b80a4a9eae3d"
)
TOPIC = "alert.evidence-links.v1"
PREREQUISITES = ROOT / "scripts/alignment/fixtures/m09_alert_evidence_link_prerequisites.sql"
MANIFEST_MIGRATION = ROOT / "deployments/postgres/migrations/202608091700_alert_evidence_manifest_v1.sql"
LINK_MIGRATION = ROOT / "deployments/postgres/migrations/202608160030_m09_alert_evidence_links_v1.sql"
CLICKHOUSE_FIXTURE = ROOT / "scripts/alignment/fixtures/m09_alert_evidence_link_clickhouse.sql"
DEFAULT_OUTPUT = (
    ROOT
    / "doc/02_acceptance/topic1/tasks/t1-m09-n012"
    / "k8s-alert-evidence-link-pipeline-latest.json"
)
IMAGE_RE = re.compile(r"^[a-zA-Z0-9][a-zA-Z0-9._/@:-]+$")
NODE_RE = re.compile(r"^[a-z0-9]([-a-z0-9.]*[a-z0-9])?$")


class CanaryError(RuntimeError):
    pass


def kubectl(
    *args: str, input_text: str | None = None, check: bool = True
) -> subprocess.CompletedProcess[str]:
    result = subprocess.run(
        ["kubectl", *args],
        input=input_text,
        text=True,
        stdout=subprocess.PIPE,
        stderr=subprocess.PIPE,
        check=False,
    )
    if check and result.returncode != 0:
        raise CanaryError(
            f"kubectl failed ({result.returncode}): {' '.join(args)}\n"
            f"stdout:\n{result.stdout}\nstderr:\n{result.stderr}"
        )
    return result


def labels(run_id: str) -> dict[str, str]:
    return {
        "app.kubernetes.io/name": "m09-alert-evidence-link-canary",
        "traffic.analysis/canary-run": run_id,
    }


def validate(images: list[str], run_id: str, node: str) -> str:
    for image in images:
        if not IMAGE_RE.fullmatch(image) or image.endswith(":latest"):
            raise CanaryError("test images must be explicit non-latest candidates")
    if not NODE_RE.fullmatch(node):
        raise CanaryError("invalid Kubernetes node name")
    parsed = uuid.UUID(run_id)
    if str(parsed) != run_id:
        raise CanaryError("--run-id must be a canonical lowercase UUID")
    for path in (PREREQUISITES, MANIFEST_MIGRATION, LINK_MIGRATION, CLICKHOUSE_FIXTURE):
        if not path.is_file():
            raise CanaryError(f"missing input: {path.relative_to(ROOT)}")
    return parsed.hex[:10]


def apply(items: list[dict[str, Any]]) -> None:
    body = "\n---\n".join(yaml.safe_dump(item, sort_keys=False) for item in items)
    kubectl("apply", "-f", "-", input_text=body)


def wait_pod(name: str, namespace: str, timeout: int) -> None:
    deadline = time.time() + timeout
    while time.time() < deadline:
        result = kubectl("get", "pod", name, "-n", namespace, "-o", "json", check=False)
        if result.returncode == 0:
            pod = json.loads(result.stdout)
            conditions = {
                item.get("type"): item.get("status")
                for item in pod.get("status", {}).get("conditions", [])
            }
            if conditions.get("Ready") == "True":
                return
            statuses = pod.get("status", {}).get("containerStatuses", [])
            if statuses and statuses[0].get("state", {}).get("terminated") is not None:
                break
        time.sleep(1)
    description = kubectl("describe", "pod", name, "-n", namespace, check=False).stdout
    logs = kubectl("logs", name, "-n", namespace, check=False).stdout
    raise CanaryError(
        f"pod {namespace}/{name} did not become ready:\n{description}\n{logs}"
    )


def pod_receipt(name: str, namespace: str) -> dict[str, Any]:
    pod = json.loads(kubectl("get", "pod", name, "-n", namespace, "-o", "json").stdout)
    statuses = pod.get("status", {}).get("containerStatuses", [])
    if len(statuses) != 1:
        raise CanaryError(f"expected one dependency container in {namespace}/{name}")
    container = statuses[0]
    if not container.get("ready") or not container.get("imageID"):
        raise CanaryError(f"dependency {namespace}/{name} lacks an immutable ready receipt")
    return {
        "namespace": namespace,
        "pod_name": name,
        "pod_uid": pod["metadata"]["uid"],
        "node": pod["spec"]["nodeName"],
        "image": container.get("image"),
        "image_id": container.get("imageID"),
        "container_id": container.get("containerID"),
        "started_at": pod.get("status", {}).get("startTime"),
    }


def wait_job(name: str, timeout: int) -> tuple[str, dict[str, Any]]:
    deadline = time.time() + timeout
    job: dict[str, Any] = {}
    while time.time() < deadline:
        response = kubectl(
            "get", "job", name, "-n", APP_NAMESPACE, "-o", "json", check=False
        )
        if response.returncode == 0:
            job = json.loads(response.stdout)
            status = job.get("status", {})
            if status.get("succeeded") == 1:
                break
            if status.get("failed", 0) > 0:
                logs = kubectl("logs", f"job/{name}", "-n", APP_NAMESPACE, check=False).stdout
                raise CanaryError(f"Kubernetes job {name} failed:\n{logs}")
        time.sleep(1)
    else:
        logs = kubectl("logs", f"job/{name}", "-n", APP_NAMESPACE, check=False).stdout
        raise CanaryError(f"Kubernetes job {name} timed out:\n{logs}")
    logs = kubectl("logs", f"job/{name}", "-n", APP_NAMESPACE, check=False).stdout
    if "PASS" not in logs or "SKIP" in logs:
        raise CanaryError(f"Kubernetes job {name} did not prove execution:\n{logs}")
    pods = json.loads(
        kubectl(
            "get", "pod", "-n", APP_NAMESPACE, "-l", f"job-name={name}", "-o", "json"
        ).stdout
    )["items"]
    if len(pods) != 1:
        raise CanaryError(f"expected one pod for {name}, observed {len(pods)}")
    pod = pods[0]
    container = pod["status"]["containerStatuses"][0]
    if not container.get("imageID"):
        raise CanaryError(f"test job {name} lacks an immutable image receipt")
    return logs, {
        "job_name": name,
        "job_uid": job["metadata"]["uid"],
        "pod_name": pod["metadata"]["name"],
        "pod_uid": pod["metadata"]["uid"],
        "node": pod["spec"]["nodeName"],
        "image": container.get("image"),
        "image_id": container.get("imageID"),
        "container_id": container.get("containerID"),
        "started_at": pod.get("status", {}).get("startTime"),
        "completed_at": job.get("status", {}).get("completionTime"),
    }


def dependency_objects(
    name: str, run_id: str, node: str, pg_password: str
) -> list[dict[str, Any]]:
    common = labels(run_id)
    postgres_selector = {"traffic.analysis/canary-postgres": name}
    clickhouse_selector = {"traffic.analysis/canary-clickhouse": name}
    kafka_selector = {"traffic.analysis/canary-kafka": name}
    kafka_name = f"{name}-kafka"
    kafka_dns = f"{kafka_name}.{DB_NAMESPACE}.svc"
    return [
        {
            "apiVersion": "v1", "kind": "ConfigMap",
            "metadata": {"name": name, "namespace": DB_NAMESPACE, "labels": common},
            "data": {
                "00-prerequisites.sql": PREREQUISITES.read_text(encoding="utf-8"),
                "10-manifest.sql": MANIFEST_MIGRATION.read_text(encoding="utf-8"),
                "20-links.sql": LINK_MIGRATION.read_text(encoding="utf-8"),
            },
        },
        {
            "apiVersion": "v1", "kind": "ConfigMap",
            "metadata": {"name": f"{name}-ch", "namespace": DB_NAMESPACE, "labels": common},
            "data": {"10-links.sql": CLICKHOUSE_FIXTURE.read_text(encoding="utf-8")},
        },
        {
            "apiVersion": "v1", "kind": "Secret", "type": "Opaque",
            "metadata": {"name": name, "namespace": DB_NAMESPACE, "labels": common},
            "stringData": {"pg-password": pg_password},
        },
        {
            "apiVersion": "v1", "kind": "Secret", "type": "Opaque",
            "metadata": {"name": name, "namespace": APP_NAMESPACE, "labels": common},
            "stringData": {
                "pg-dsn": (
                    f"postgres://postgres:{pg_password}@{name}.{DB_NAMESPACE}.svc:5432/"
                    "traffic_platform?sslmode=disable"
                )
            },
        },
        {
            "apiVersion": "v1", "kind": "Service",
            "metadata": {"name": name, "namespace": DB_NAMESPACE, "labels": common},
            "spec": {"selector": postgres_selector, "ports": [{"name": "postgres", "port": 5432}]},
        },
        {
            "apiVersion": "v1", "kind": "Service",
            "metadata": {"name": f"{name}-ch", "namespace": DB_NAMESPACE, "labels": common},
            "spec": {"selector": clickhouse_selector, "ports": [{"name": "native", "port": 9000}]},
        },
        {
            "apiVersion": "v1", "kind": "Service",
            "metadata": {"name": kafka_name, "namespace": DB_NAMESPACE, "labels": common},
            "spec": {"selector": kafka_selector, "ports": [{"name": "kafka", "port": 9092}]},
        },
        {
            "apiVersion": "v1", "kind": "Pod",
            "metadata": {
                "name": name, "namespace": DB_NAMESPACE,
                "labels": {**common, **postgres_selector},
            },
            "spec": {
                "nodeName": node, "automountServiceAccountToken": False,
                "restartPolicy": "Never", "securityContext": {"seccompProfile": {"type": "RuntimeDefault"}},
                "containers": [{
                    "name": "postgres", "image": POSTGRES_IMAGE, "imagePullPolicy": "IfNotPresent",
                    "env": [
                        {"name": "POSTGRES_DB", "value": "traffic_platform"},
                        {"name": "POSTGRES_USER", "value": "postgres"},
                        {"name": "POSTGRES_PASSWORD", "valueFrom": {"secretKeyRef": {"name": name, "key": "pg-password"}}},
                    ],
                    "ports": [{"containerPort": 5432}],
                    "readinessProbe": {"exec": {"command": ["sh", "-ec", "pg_isready -h 127.0.0.1 -U postgres -d traffic_platform"]}, "periodSeconds": 2, "failureThreshold": 90},
                    "resources": {"requests": {"cpu": "100m", "memory": "192Mi"}, "limits": {"cpu": "1", "memory": "1Gi"}},
                    "volumeMounts": [
                        {"name": "data", "mountPath": "/var/lib/postgresql/data"},
                        {"name": "init", "mountPath": "/docker-entrypoint-initdb.d", "readOnly": True},
                    ],
                }],
                "volumes": [{"name": "data", "emptyDir": {}}, {"name": "init", "configMap": {"name": name}}],
            },
        },
        {
            "apiVersion": "v1", "kind": "Pod",
            "metadata": {
                "name": f"{name}-ch", "namespace": DB_NAMESPACE,
                "labels": {**common, **clickhouse_selector},
            },
            "spec": {
                "nodeName": node, "automountServiceAccountToken": False,
                "restartPolicy": "Never", "securityContext": {"seccompProfile": {"type": "RuntimeDefault"}},
                "containers": [{
                    "name": "clickhouse", "image": CLICKHOUSE_IMAGE, "imagePullPolicy": "IfNotPresent",
                    "env": [{"name": "CLICKHOUSE_SKIP_USER_SETUP", "value": "1"}],
                    "ports": [{"containerPort": 9000}],
                    "readinessProbe": {"exec": {"command": ["clickhouse-client", "--query", "SELECT 1"]}, "periodSeconds": 2, "failureThreshold": 90},
                    "resources": {"requests": {"cpu": "100m", "memory": "256Mi"}, "limits": {"cpu": "1", "memory": "1Gi"}},
                    "volumeMounts": [
                        {"name": "data", "mountPath": "/var/lib/clickhouse"},
                        {"name": "init", "mountPath": "/docker-entrypoint-initdb.d", "readOnly": True},
                    ],
                }],
                "volumes": [{"name": "data", "emptyDir": {}}, {"name": "init", "configMap": {"name": f"{name}-ch"}}],
            },
        },
        {
            "apiVersion": "v1", "kind": "Pod",
            "metadata": {
                "name": kafka_name, "namespace": DB_NAMESPACE,
                "labels": {**common, **kafka_selector},
            },
            "spec": {
                "nodeName": node, "automountServiceAccountToken": False,
                "restartPolicy": "Never", "securityContext": {"seccompProfile": {"type": "RuntimeDefault"}},
                "containers": [{
                    "name": "redpanda", "image": KAFKA_IMAGE, "imagePullPolicy": "Never",
                    "args": [
                        "start", "--mode", "dev-container", "--check=false", "--smp", "1",
                        "--memory", "768M", "--reserve-memory", "0M",
                        "--kafka-addr", "internal://0.0.0.0:9092",
                        "--advertise-kafka-addr", f"internal://{kafka_dns}:9092",
                        "--rpc-addr", "0.0.0.0:33145",
                        "--advertise-rpc-addr", f"{kafka_dns}:33145",
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


def test_job(
    name: str, run_id: str, node: str, image: str, suite: str
) -> dict[str, Any]:
    common = labels(run_id)
    patterns = {
        "api": "^TestAlertEvidenceLinkPipelineEphemeralKubernetes$",
        "consumer": "^TestAlertEvidenceLinkConsumerEphemeralKubernetes$",
    }
    env: list[dict[str, Any]] = [
        {
            "name": "ALERT_EVIDENCE_LINK_EPHEMERAL_PG_DSN",
            "valueFrom": {"secretKeyRef": {"name": name, "key": "pg-dsn"}},
        },
        {
            "name": "ALERT_EVIDENCE_LINK_EPHEMERAL_KAFKA_BROKER",
            "value": f"{name}-kafka.{DB_NAMESPACE}.svc:9092",
        },
        {"name": "ALERT_EVIDENCE_LINK_CANARY_RUN_ID", "value": run_id},
    ]
    if suite == "consumer":
        env.append({
            "name": "ALERT_EVIDENCE_LINK_EPHEMERAL_CLICKHOUSE_HOST",
            "value": f"{name}-ch.{DB_NAMESPACE}.svc:9000",
        })
    job_name = f"{name}-{suite}"
    return {
        "apiVersion": "batch/v1", "kind": "Job",
        "metadata": {
            "name": job_name, "namespace": APP_NAMESPACE, "labels": common,
            "annotations": {
                "traffic.analysis/shared-postgres-touched": "false",
                "traffic.analysis/shared-kafka-touched": "false",
                "traffic.analysis/shared-clickhouse-touched": "false",
                "traffic.analysis/production-applied": "false",
            },
        },
        "spec": {
            "backoffLimit": 0,
            "template": {
                "metadata": {"labels": common},
                "spec": {
                    "nodeName": node, "automountServiceAccountToken": False,
                    "restartPolicy": "Never",
                    "securityContext": {
                        "runAsNonRoot": True, "runAsUser": 1000, "runAsGroup": 1000,
                        "fsGroup": 1000, "seccompProfile": {"type": "RuntimeDefault"},
                    },
                    "containers": [{
                        "name": suite, "image": image, "imagePullPolicy": "Never",
                        "args": ["-test.v", f"-test.run={patterns[suite]}", "-test.count=1"],
                        "env": env,
                        "securityContext": {
                            "allowPrivilegeEscalation": False, "readOnlyRootFilesystem": True,
                            "capabilities": {"drop": ["ALL"]},
                        },
                        "resources": {"requests": {"cpu": "100m", "memory": "128Mi"}, "limits": {"cpu": "1", "memory": "512Mi"}},
                        "volumeMounts": [{"name": "tmp", "mountPath": "/tmp"}],
                    }],
                    "volumes": [{"name": "tmp", "emptyDir": {}}],
                },
            },
        },
    }


def create_topic(name: str) -> dict[str, Any]:
    pod = f"{name}-kafka"
    kubectl(
        "exec", "-n", DB_NAMESPACE, pod, "--", "rpk", "topic", "create", TOPIC,
        "--brokers", "127.0.0.1:9092", "--partitions", "6", "--replicas", "1",
        "-c", "cleanup.policy=delete", "-c", "retention.ms=3600000",
    )
    description = kubectl(
        "exec", "-n", DB_NAMESPACE, pod, "--", "rpk", "topic", "describe", TOPIC,
        "--brokers", "127.0.0.1:9092", "-p",
    ).stdout
    topic_list = kubectl(
        "exec", "-n", DB_NAMESPACE, pod, "--", "rpk", "topic", "list",
        "--brokers", "127.0.0.1:9092",
    ).stdout
    partition_ids = {
        line.split()[0]
        for line in description.splitlines()[1:]
        if line.split() and line.split()[0].isdigit()
    }
    if TOPIC not in topic_list or partition_ids != {"0", "1", "2", "3", "4", "5"}:
        raise CanaryError(
            f"topic receipt mismatch:\nlist:\n{topic_list}\ndescription:\n{description}"
        )
    return {
        "topic": TOPIC,
        "partitions": 6,
        "replication_factor": 1,
        "topic_list_sha256": hashlib.sha256(topic_list.encode()).hexdigest(),
        "description_sha256": hashlib.sha256(description.encode()).hexdigest(),
    }


def postgres_oracle(name: str) -> dict[str, Any]:
    sql = """SELECT json_build_object(
      'manifest_migration',(SELECT count(*) FROM alignment_schema_migrations WHERE version='202608091700'),
      'link_migration',(SELECT count(*) FROM alignment_schema_migrations WHERE version='202608160030'),
      'sentinel',(SELECT marker FROM codex_ephemeral_alert_evidence_link_sentinel LIMIT 1),
      'tenants',(SELECT count(*) FROM tenants),
      'manifests',(SELECT count(*) FROM alert_evidence_manifests),
      'manifest_history',(SELECT count(*) FROM alert_evidence_manifest_history),
      'links',(SELECT count(*) FROM alert_evidence_links),
      'link_history',(SELECT count(*) FROM alert_evidence_link_history),
      'commands',(SELECT count(*) FROM alert_evidence_link_commands),
      'outbox',(SELECT count(*) FROM alert_evidence_link_outbox),
      'inbox',(SELECT count(*) FROM alert_evidence_link_projection_inbox),
      'deliveries',(SELECT count(*) FROM alert_evidence_link_projection_deliveries),
      'watermarks',(SELECT count(*) FROM alert_evidence_link_projection_watermarks),
      'audit',(SELECT count(*) FROM audit_logs)
    );"""
    result = kubectl(
        "exec", "-i", "-n", DB_NAMESPACE, name, "--", "sh", "-ec",
        'PGPASSWORD="$POSTGRES_PASSWORD" exec psql -v ON_ERROR_STOP=1 -U postgres -d traffic_platform -At',
        input_text=sql,
    )
    oracle = json.loads(result.stdout.strip())
    expected = {
        "manifest_migration": 1, "link_migration": 1, "sentinel": "ephemeral-only",
        "tenants": 0, "manifests": 0, "manifest_history": 0, "links": 0,
        "link_history": 0, "commands": 0, "outbox": 0, "inbox": 0,
        "deliveries": 0, "watermarks": 0, "audit": 0,
    }
    if oracle != expected:
        raise CanaryError(f"unexpected PostgreSQL cleanup oracle: {oracle}")
    return oracle


def clickhouse_oracle(name: str) -> dict[str, int]:
    count = int(
        kubectl(
            "exec", "-n", DB_NAMESPACE, f"{name}-ch", "--", "clickhouse-client",
            "--query", "SELECT count() FROM traffic.alert_evidence_links_v1_local",
        ).stdout.strip()
    )
    if count != 0:
        raise CanaryError(f"unexpected ClickHouse rows after cleanup: {count}")
    return {"alert_evidence_links_v1_local": count}


def cleanup(run_id: str) -> None:
    selector = f"traffic.analysis/canary-run={run_id}"
    for namespace in (APP_NAMESPACE, DB_NAMESPACE):
        kubectl(
            "delete", "job,pod,service,configmap,secret", "-n", namespace, "-l", selector,
            "--ignore-not-found=true", "--wait=true", "--timeout=180s", check=False,
        )
        remaining = kubectl(
            "get", "job,pod,service,configmap,secret", "-n", namespace, "-l", selector,
            "-o", "name", check=False,
        ).stdout.strip()
        if remaining:
            raise CanaryError(f"run-scoped resources remain in {namespace}: {remaining}")


def main() -> None:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--api-image", required=True)
    parser.add_argument("--consumer-image", required=True)
    parser.add_argument("--run-id", default=str(uuid.uuid4()))
    parser.add_argument("--node", default="8-2tb")
    parser.add_argument("--timeout", type=int, default=420)
    parser.add_argument("--output", type=Path, default=DEFAULT_OUTPUT)
    parser.add_argument("--keep", action="store_true")
    args = parser.parse_args()
    if args.timeout < 120 or args.timeout > 1200:
        raise CanaryError("--timeout must be between 120 and 1200 seconds")
    images = [args.api_image, args.consumer_image]
    suffix = validate(images, args.run_id, args.node)
    name = f"m09-n012-evidence-{suffix}"
    pg_password = "m09-n012-" + secrets.token_hex(16)
    applied = False
    dependencies: list[dict[str, Any]] = []
    receipts: list[dict[str, Any]] = []
    logs_by_suite: dict[str, str] = {}
    topic_receipt: dict[str, Any] = {}
    try:
        apply(dependency_objects(name, args.run_id, args.node, pg_password))
        applied = True
        for pod_name in (name, f"{name}-ch", f"{name}-kafka"):
            wait_pod(pod_name, DB_NAMESPACE, args.timeout)
            dependencies.append(pod_receipt(pod_name, DB_NAMESPACE))
        topic_receipt = create_topic(name)
        for suite, image in (("api", args.api_image), ("consumer", args.consumer_image)):
            apply([test_job(name, args.run_id, args.node, image, suite)])
            logs, receipt = wait_job(f"{name}-{suite}", args.timeout)
            logs_by_suite[suite] = logs
            receipts.append(receipt)
        pg_oracle = postgres_oracle(name)
        ch_oracle = clickhouse_oracle(name)
    finally:
        if applied and not args.keep:
            cleanup(args.run_id)
    envelope = {
        "artifact_kind": "M09_ALERT_EVIDENCE_LINK_PIPELINE_TEST_RESULT",
        "task_id": "T1-M09-N012", "run_id": args.run_id, "status": "PASS",
        "profile_id": "M09-N012-K8S-PG-REDPANDA-CLICKHOUSE-V1",
        "coverage_status": "PASS_FOR_RUN_SCOPED_K8S_COMMAND_OUTBOX_KAFKA_CONSUMER_PROJECTION",
        "inputs": {
            "prerequisites_sha256": hashlib.sha256(PREREQUISITES.read_bytes()).hexdigest(),
            "manifest_migration_sha256": hashlib.sha256(MANIFEST_MIGRATION.read_bytes()).hexdigest(),
            "link_migration_sha256": hashlib.sha256(LINK_MIGRATION.read_bytes()).hexdigest(),
            "clickhouse_fixture_sha256": hashlib.sha256(CLICKHOUSE_FIXTURE.read_bytes()).hexdigest(),
            "postgres_image": POSTGRES_IMAGE, "clickhouse_image": CLICKHOUSE_IMAGE,
            "kafka_image": KAFKA_IMAGE, "test_images": images,
        },
        "test_output_sha256": {
            key: hashlib.sha256(value.encode()).hexdigest()
            for key, value in logs_by_suite.items()
        },
        "topic_receipt": topic_receipt,
        "postgres_cleanup_oracle": pg_oracle,
        "clickhouse_cleanup_oracle": ch_oracle,
        "kubernetes_dependencies": dependencies,
        "kubernetes_jobs": receipts,
        "run_scoped_resources_removed": not args.keep,
        "production_applied": False,
        "shared_postgres_touched": False,
        "shared_kafka_touched": False,
        "shared_clickhouse_touched": False,
        "writer_default_enabled": False,
        "dispatcher_default_enabled": False,
        "consumer_default_enabled": False,
        "does_not_prove": [
            "production rollout", "shared data migration", "global task completion",
            "Windows Chrome acceptance", "performance or rollback drill",
        ],
    }
    args.output.parent.mkdir(parents=True, exist_ok=True)
    temporary = args.output.with_name(f".{args.output.name}.tmp")
    temporary.write_text(json.dumps(envelope, indent=2, sort_keys=True) + "\n", encoding="utf-8")
    temporary.replace(args.output)
    print(json.dumps(envelope, sort_keys=True))


if __name__ == "__main__":
    main()
