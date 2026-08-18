#!/usr/bin/env python3
"""Run the M09-N010 worker authority matrix on run-scoped K8s dependencies."""

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
MINIO_IMAGE = "docker.io/minio/minio:latest"
MC_IMAGE = "docker.io/minio/mc:latest"
PREREQUISITES = ROOT / "scripts/alignment/fixtures/m09_forensics_task_prerequisites.sql"
TASK_MIGRATION = ROOT / "deployments/postgres/migrations/202608031600_forensics_task_command_atomic.sql"
WORKER_MIGRATION = ROOT / "deployments/postgres/migrations/202608151930_m09_forensics_worker_checkpoint_manifest.sql"
IMAGE_RE = re.compile(r"^[a-zA-Z0-9][a-zA-Z0-9._/@:-]+$")
NODE_RE = re.compile(r"^[a-z0-9]([-a-z0-9.]*[a-z0-9])?$")


class CanaryError(RuntimeError):
    pass


def kubectl(*args: str, input_text: str | None = None, check: bool = True) -> subprocess.CompletedProcess[str]:
    result = subprocess.run(
        ["kubectl", *args], input=input_text, text=True,
        stdout=subprocess.PIPE, stderr=subprocess.PIPE, check=False,
    )
    if check and result.returncode != 0:
        raise CanaryError(
            f"kubectl failed ({result.returncode}): {' '.join(args)}\n"
            f"stdout:\n{result.stdout}\nstderr:\n{result.stderr}"
        )
    return result


def labels(run_id: str) -> dict[str, str]:
    return {
        "app.kubernetes.io/name": "m09-forensics-worker-canary",
        "traffic.analysis/canary-run": run_id,
    }


def validate(images: list[str], run_id: str, node: str) -> str:
    for image in images:
        if not IMAGE_RE.fullmatch(image) or image.endswith(":latest"):
            raise CanaryError("test images must be non-latest candidate images")
    if not NODE_RE.fullmatch(node):
        raise CanaryError("invalid Kubernetes node name")
    parsed = uuid.UUID(run_id)
    if str(parsed) != run_id:
        raise CanaryError("--run-id must be a canonical lowercase UUID")
    for path in (PREREQUISITES, TASK_MIGRATION, WORKER_MIGRATION):
        if not path.is_file():
            raise CanaryError(f"missing input: {path.relative_to(ROOT)}")
    return parsed.hex[:10]


def apply(items: list[dict[str, Any]]) -> None:
    body = "\n---\n".join(yaml.safe_dump(item, sort_keys=False) for item in items)
    kubectl("apply", "-f", "-", input_text=body)


def wait_pod(name: str, namespace: str, timeout: int) -> None:
    result = kubectl("wait", "--for=condition=Ready", f"pod/{name}", "-n", namespace, f"--timeout={timeout}s", check=False)
    if result.returncode != 0:
        logs = kubectl("logs", name, "-n", namespace, check=False).stdout
        raise CanaryError(f"pod {namespace}/{name} did not become ready:\n{logs}")


def pod_receipt(name: str, namespace: str) -> dict[str, Any]:
    pod = json.loads(kubectl("get", "pod", name, "-n", namespace, "-o", "json").stdout)
    statuses = pod.get("status", {}).get("containerStatuses", [])
    if len(statuses) != 1:
        raise CanaryError(f"expected one dependency container in {namespace}/{name}")
    container = statuses[0]
    if not container.get("ready") or not container.get("imageID"):
        raise CanaryError(f"dependency {namespace}/{name} lacks a ready immutable image receipt")
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


def wait_job(name: str, timeout: int, require_pass: bool = True) -> tuple[str, dict[str, Any]]:
    deadline = time.time() + timeout
    job: dict[str, Any] = {}
    while time.time() < deadline:
        response = kubectl("get", "job", name, "-n", APP_NAMESPACE, "-o", "json", check=False)
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
    if require_pass and ("PASS" not in logs or "SKIP" in logs):
        raise CanaryError(f"Kubernetes job {name} did not prove execution:\n{logs}")
    pods = json.loads(kubectl("get", "pod", "-n", APP_NAMESPACE, "-l", f"job-name={name}", "-o", "json").stdout)["items"]
    if len(pods) != 1:
        raise CanaryError(f"expected one pod for {name}, observed {len(pods)}")
    pod = pods[0]
    container = pod["status"]["containerStatuses"][0]
    return logs, {
        "job_name": name, "job_uid": job["metadata"]["uid"],
        "pod_name": pod["metadata"]["name"], "pod_uid": pod["metadata"]["uid"],
        "node": pod["spec"]["nodeName"], "image": container.get("image"),
        "image_id": container.get("imageID"), "container_id": container.get("containerID"),
        "started_at": pod.get("status", {}).get("startTime"),
        "completed_at": job.get("status", {}).get("completionTime"),
    }


def dependency_objects(name: str, run_id: str, node: str, pg_password: str, minio_secret: str) -> list[dict[str, Any]]:
    common = labels(run_id)
    postgres_selector = {"traffic.analysis/canary-postgres": name}
    clickhouse_selector = {"traffic.analysis/canary-clickhouse": name}
    minio_selector = {"traffic.analysis/canary-minio": name}
    clickhouse_sql = """
CREATE DATABASE IF NOT EXISTS traffic;
CREATE TABLE IF NOT EXISTS traffic.pcap_index_v2 (
  tenant_id String, probe_id String, file_key String, bucket String,
  object_version String, etag String, original_size UInt64, stored_size UInt64,
  compression LowCardinality(String), manifest_version UInt16,
  kafka_topic String, kafka_partition Int32, kafka_offset Int64,
  kafka_key_sha256 FixedString(64), kafka_headers_sha256 FixedString(64), raw_sha256 FixedString(64),
  projection_identity FixedString(64), ts_start DateTime64(3, 'UTC'), ts_end DateTime64(3, 'UTC'),
  byte_size UInt64, zstd_level UInt8, sha256 String, community_id String, flow_id String,
  offset_start Nullable(UInt64), offset_end Nullable(UInt64), bloom_filter_b64 String,
  community_ids Array(String), created_ts DateTime64(3, 'UTC')
) ENGINE=ReplacingMergeTree(created_ts)
PARTITION BY toYYYYMMDD(ts_start) ORDER BY (tenant_id,ts_start,probe_id,file_key);
""".strip()
    return [
        {"apiVersion": "v1", "kind": "ConfigMap", "metadata": {"name": name, "namespace": DB_NAMESPACE, "labels": common}, "data": {
            "00-prerequisites.sql": PREREQUISITES.read_text(encoding="utf-8"),
            "10-task.sql": TASK_MIGRATION.read_text(encoding="utf-8"),
            "20-worker.sql": WORKER_MIGRATION.read_text(encoding="utf-8"),
        }},
        {"apiVersion": "v1", "kind": "ConfigMap", "metadata": {"name": f"{name}-ch", "namespace": DB_NAMESPACE, "labels": common}, "data": {"10-authority.sql": clickhouse_sql}},
        {"apiVersion": "v1", "kind": "Secret", "type": "Opaque", "metadata": {"name": name, "namespace": DB_NAMESPACE, "labels": common}, "stringData": {"pg-password": pg_password, "minio-access": "m09minio", "minio-secret": minio_secret}},
        {"apiVersion": "v1", "kind": "Secret", "type": "Opaque", "metadata": {"name": name, "namespace": APP_NAMESPACE, "labels": common}, "stringData": {
            "pg-dsn": f"postgres://postgres:{pg_password}@{name}.{DB_NAMESPACE}.svc:5432/traffic_platform?sslmode=disable",
            "minio-access": "m09minio", "minio-secret": minio_secret,
        }},
        {"apiVersion": "v1", "kind": "Service", "metadata": {"name": name, "namespace": DB_NAMESPACE, "labels": common}, "spec": {"selector": postgres_selector, "ports": [{"name": "postgres", "port": 5432, "targetPort": 5432}]}},
        {"apiVersion": "v1", "kind": "Service", "metadata": {"name": f"{name}-ch", "namespace": DB_NAMESPACE, "labels": common}, "spec": {"selector": clickhouse_selector, "ports": [{"name": "native", "port": 9000, "targetPort": 9000}]}},
        {"apiVersion": "v1", "kind": "Service", "metadata": {"name": f"{name}-minio", "namespace": DB_NAMESPACE, "labels": common}, "spec": {"selector": minio_selector, "ports": [{"name": "api", "port": 9000, "targetPort": 9000}]}},
        {"apiVersion": "v1", "kind": "Pod", "metadata": {"name": name, "namespace": DB_NAMESPACE, "labels": {**common, **postgres_selector}}, "spec": {
            "nodeName": node, "automountServiceAccountToken": False, "restartPolicy": "Never",
            "securityContext": {"seccompProfile": {"type": "RuntimeDefault"}},
            "containers": [{"name": "postgres", "image": POSTGRES_IMAGE, "imagePullPolicy": "IfNotPresent", "env": [
                {"name": "POSTGRES_DB", "value": "traffic_platform"}, {"name": "POSTGRES_USER", "value": "postgres"},
                {"name": "POSTGRES_PASSWORD", "valueFrom": {"secretKeyRef": {"name": name, "key": "pg-password"}}},
            ], "ports": [{"containerPort": 5432}], "readinessProbe": {"exec": {"command": ["sh", "-ec", "pg_isready -h 127.0.0.1 -U postgres -d traffic_platform"]}, "periodSeconds": 2, "failureThreshold": 90},
            "resources": {"requests": {"cpu": "100m", "memory": "192Mi"}, "limits": {"cpu": "1", "memory": "1Gi"}},
            "volumeMounts": [{"name": "data", "mountPath": "/var/lib/postgresql/data"}, {"name": "init", "mountPath": "/docker-entrypoint-initdb.d", "readOnly": True}]}],
            "volumes": [{"name": "data", "emptyDir": {}}, {"name": "init", "configMap": {"name": name}}],
        }},
        {"apiVersion": "v1", "kind": "Pod", "metadata": {"name": f"{name}-ch", "namespace": DB_NAMESPACE, "labels": {**common, **clickhouse_selector}}, "spec": {
            "nodeName": node, "automountServiceAccountToken": False, "restartPolicy": "Never",
            "securityContext": {"seccompProfile": {"type": "RuntimeDefault"}},
            "containers": [{"name": "clickhouse", "image": CLICKHOUSE_IMAGE, "imagePullPolicy": "IfNotPresent", "env": [{"name": "CLICKHOUSE_SKIP_USER_SETUP", "value": "1"}],
                "ports": [{"containerPort": 9000}], "readinessProbe": {"exec": {"command": ["clickhouse-client", "--query", "SELECT 1"]}, "periodSeconds": 2, "failureThreshold": 90},
                "resources": {"requests": {"cpu": "100m", "memory": "256Mi"}, "limits": {"cpu": "1", "memory": "1Gi"}},
                "volumeMounts": [{"name": "data", "mountPath": "/var/lib/clickhouse"}, {"name": "init", "mountPath": "/docker-entrypoint-initdb.d", "readOnly": True}]}],
            "volumes": [{"name": "data", "emptyDir": {}}, {"name": "init", "configMap": {"name": f"{name}-ch"}}],
        }},
        {"apiVersion": "v1", "kind": "Pod", "metadata": {"name": f"{name}-minio", "namespace": DB_NAMESPACE, "labels": {**common, **minio_selector}}, "spec": {
            "nodeName": node, "automountServiceAccountToken": False, "restartPolicy": "Never",
            "securityContext": {"seccompProfile": {"type": "RuntimeDefault"}},
            "containers": [{"name": "minio", "image": MINIO_IMAGE, "imagePullPolicy": "IfNotPresent", "args": ["server", "/data"], "env": [
                {"name": "MINIO_ROOT_USER", "valueFrom": {"secretKeyRef": {"name": name, "key": "minio-access"}}},
                {"name": "MINIO_ROOT_PASSWORD", "valueFrom": {"secretKeyRef": {"name": name, "key": "minio-secret"}}},
            ], "ports": [{"containerPort": 9000}], "readinessProbe": {"httpGet": {"path": "/minio/health/live", "port": 9000}, "periodSeconds": 2, "failureThreshold": 90},
            "resources": {"requests": {"cpu": "100m", "memory": "256Mi"}, "limits": {"cpu": "1", "memory": "1Gi"}}, "volumeMounts": [{"name": "data", "mountPath": "/data"}]}],
            "volumes": [{"name": "data", "emptyDir": {}}],
        }},
    ]


def test_job(name: str, run_id: str, node: str, image: str, suite: str) -> dict[str, Any]:
    common = labels(run_id)
    patterns = {
        "repository": "^TestForensicsVersionedExecutionEphemeralPostgres$",
        "index": "^TestRestorationSourceClickHouseRoundTrip$",
        "s3": "^TestRestorationObjectAuthorityRoundTrip$",
    }
    env: list[dict[str, Any]] = []
    if suite == "repository":
        env = [{"name": "FORENSICS_TASK_ATOMIC_EPHEMERAL_PG_DSN", "valueFrom": {"secretKeyRef": {"name": name, "key": "pg-dsn"}}}]
    elif suite == "index":
        env = [
            {"name": "M03_RESTORATION_CLICKHOUSE_INTEGRATION_ENABLED", "value": "true"},
            {"name": "M03_RESTORATION_CLICKHOUSE_SENTINEL", "value": "codex_ephemeral_m03_restoration_clickhouse"},
            {"name": "M03_RESTORATION_CLICKHOUSE_NATIVE_ADDR", "value": f"{name}-ch.{DB_NAMESPACE}.svc:9000"},
        ]
    else:
        env = [
            {"name": "M03_RESTORATION_MINIO_INTEGRATION_ENABLED", "value": "true"},
            {"name": "M03_RESTORATION_MINIO_SENTINEL", "value": "codex_ephemeral_m03_restoration_minio"},
            {"name": "M03_RESTORATION_MINIO_ENDPOINT", "value": f"{name}-minio.{DB_NAMESPACE}.svc:9000"},
            {"name": "M03_RESTORATION_MINIO_ACCESS_KEY", "valueFrom": {"secretKeyRef": {"name": name, "key": "minio-access"}}},
            {"name": "M03_RESTORATION_MINIO_SECRET_KEY", "valueFrom": {"secretKeyRef": {"name": name, "key": "minio-secret"}}},
        ]
    job_name = f"{name}-{suite}"
    return {"apiVersion": "batch/v1", "kind": "Job", "metadata": {"name": job_name, "namespace": APP_NAMESPACE, "labels": common,
        "annotations": {"traffic.analysis/shared-stores-touched": "false", "traffic.analysis/production-applied": "false"}}, "spec": {"backoffLimit": 0, "template": {"metadata": {"labels": common}, "spec": {
            "nodeName": node, "automountServiceAccountToken": False, "restartPolicy": "Never",
            "securityContext": {"runAsNonRoot": True, "runAsUser": 1000, "runAsGroup": 1000, "fsGroup": 1000, "seccompProfile": {"type": "RuntimeDefault"}},
            "containers": [{"name": suite, "image": image, "imagePullPolicy": "Never", "args": ["-test.v", f"-test.run={patterns[suite]}", "-test.count=1"], "env": env,
                "securityContext": {"allowPrivilegeEscalation": False, "readOnlyRootFilesystem": True, "capabilities": {"drop": ["ALL"]}},
                "resources": {"requests": {"cpu": "100m", "memory": "128Mi"}, "limits": {"cpu": "1", "memory": "512Mi"}}, "volumeMounts": [{"name": "tmp", "mountPath": "/tmp"}]}],
            "volumes": [{"name": "tmp", "emptyDir": {}}],
        }}}}


def minio_init_job(name: str, run_id: str, node: str) -> dict[str, Any]:
    common = labels(run_id)
    return {"apiVersion": "batch/v1", "kind": "Job", "metadata": {"name": f"{name}-minio-init", "namespace": APP_NAMESPACE, "labels": common}, "spec": {"backoffLimit": 0, "template": {"metadata": {"labels": common}, "spec": {
        "nodeName": node, "automountServiceAccountToken": False, "restartPolicy": "Never",
        "containers": [{"name": "mc", "image": MC_IMAGE, "imagePullPolicy": "IfNotPresent", "command": ["/bin/sh", "-ec"], "args": [
            "attempt=0; until "
            f"mc alias set owned http://{name}-minio.{DB_NAMESPACE}.svc:9000 \"$MINIO_ACCESS\" \"$MINIO_SECRET\"; "
            "do attempt=$((attempt+1)); if [ \"$attempt\" -ge 60 ]; then exit 1; fi; sleep 1; done; "
            "mc mb --with-lock owned/pcap-archive; mc mb --with-lock owned/forensics-quarantine; "
            "mc version enable owned/pcap-archive; mc version enable owned/forensics-quarantine; echo PASS"
        ], "env": [
            {"name": "MINIO_ACCESS", "valueFrom": {"secretKeyRef": {"name": name, "key": "minio-access"}}},
            {"name": "MINIO_SECRET", "valueFrom": {"secretKeyRef": {"name": name, "key": "minio-secret"}}},
        ]}],
    }}}}


def postgres_oracle(name: str) -> dict[str, Any]:
    sql = """SELECT json_build_object(
      'task_migration',(SELECT count(*) FROM alignment_schema_migrations WHERE version='202608031600'),
      'worker_migration',(SELECT count(*) FROM alignment_schema_migrations WHERE version='202608151930'),
      'sentinel',(SELECT marker FROM codex_ephemeral_forensics_task_sentinel LIMIT 1),
      'tasks',(SELECT count(*) FROM tasks),
      'checkpoints',(SELECT count(*) FROM forensics_task_checkpoints),
      'manifests',(SELECT count(*) FROM forensics_job_manifests)
    );"""
    result = kubectl("exec", "-i", "-n", DB_NAMESPACE, name, "--", "sh", "-ec", 'PGPASSWORD="$POSTGRES_PASSWORD" exec psql -v ON_ERROR_STOP=1 -U postgres -d traffic_platform -At', input_text=sql)
    oracle = json.loads(result.stdout.strip())
    expected = {"task_migration": 1, "worker_migration": 1, "sentinel": "ephemeral-only", "tasks": 0, "checkpoints": 0, "manifests": 0}
    if oracle != expected:
        raise CanaryError(f"unexpected PostgreSQL cleanup oracle: {oracle}")
    return oracle


def cleanup(run_id: str) -> None:
    selector = f"traffic.analysis/canary-run={run_id}"
    for namespace in (APP_NAMESPACE, DB_NAMESPACE):
        kubectl("delete", "job,pod,service,configmap,secret", "-n", namespace, "-l", selector,
                "--ignore-not-found=true", "--wait=true", "--timeout=180s", check=False)
        remaining = kubectl("get", "job,pod,service,configmap,secret", "-n", namespace, "-l", selector, "-o", "name", check=False).stdout.strip()
        if remaining:
            raise CanaryError(f"run-scoped resources remain in {namespace}: {remaining}")


def main() -> None:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--repository-image", required=True)
    parser.add_argument("--index-image", required=True)
    parser.add_argument("--s3-image", required=True)
    parser.add_argument("--run-id", default=str(uuid.uuid4()))
    parser.add_argument("--node", default="8-2tb")
    parser.add_argument("--timeout", type=int, default=420)
    parser.add_argument("--output", type=Path)
    parser.add_argument("--keep", action="store_true")
    args = parser.parse_args()
    if args.timeout < 120 or args.timeout > 1200:
        raise CanaryError("--timeout must be between 120 and 1200 seconds")
    images = [args.repository_image, args.index_image, args.s3_image]
    suffix = validate(images, args.run_id, args.node)
    name = f"m09-n010-worker-{suffix}"
    pg_password = "m09-" + secrets.token_hex(16)
    minio_secret = "m09-minio-" + secrets.token_hex(16)
    applied = False
    receipts: list[dict[str, Any]] = []
    dependencies: list[dict[str, Any]] = []
    logs_by_suite: dict[str, str] = {}
    try:
        apply(dependency_objects(name, args.run_id, args.node, pg_password, minio_secret))
        applied = True
        for pod_name in (name, f"{name}-ch", f"{name}-minio"):
            wait_pod(pod_name, DB_NAMESPACE, args.timeout)
            dependencies.append(pod_receipt(pod_name, DB_NAMESPACE))
        apply([minio_init_job(name, args.run_id, args.node)])
        init_logs, init_receipt = wait_job(f"{name}-minio-init", args.timeout)
        logs_by_suite["minio_init"] = init_logs
        receipts.append(init_receipt)
        for suite, image in zip(("repository", "index", "s3"), images):
            apply([test_job(name, args.run_id, args.node, image, suite)])
            logs, receipt = wait_job(f"{name}-{suite}", args.timeout)
            logs_by_suite[suite] = logs
            receipts.append(receipt)
        pg_oracle = postgres_oracle(name)
        clickhouse_count = int(kubectl("exec", "-n", DB_NAMESPACE, f"{name}-ch", "--", "clickhouse-client", "--query", "SELECT count() FROM traffic.pcap_index_v2").stdout.strip())
        if clickhouse_count != 1:
            raise CanaryError(f"unexpected ClickHouse source authority count: {clickhouse_count}")
    finally:
        if applied and not args.keep:
            cleanup(args.run_id)
    envelope = {
        "artifact_kind": "M09_FORENSICS_WORKER_TEST_RESULT", "task_id": "T1-M09-N010",
        "run_id": args.run_id, "status": "PASS",
        "profile_id": "M09-N010-K8S-VERSIONED-WORKER-V1",
        "inputs": {
            "prerequisites_sha256": hashlib.sha256(PREREQUISITES.read_bytes()).hexdigest(),
            "task_migration_sha256": hashlib.sha256(TASK_MIGRATION.read_bytes()).hexdigest(),
            "worker_migration_sha256": hashlib.sha256(WORKER_MIGRATION.read_bytes()).hexdigest(),
            "postgres_image": POSTGRES_IMAGE, "clickhouse_image": CLICKHOUSE_IMAGE,
            "minio_image": MINIO_IMAGE, "test_images": images,
        },
        "test_output_sha256": {key: hashlib.sha256(value.encode()).hexdigest() for key, value in logs_by_suite.items()},
        "postgres_oracle": pg_oracle, "clickhouse_authority_rows": clickhouse_count,
        "kubernetes_dependencies": dependencies, "kubernetes": receipts,
        "run_scoped_resources_removed": not args.keep,
        "production_applied": False, "shared_postgres_touched": False,
        "shared_clickhouse_touched": False, "shared_minio_touched": False,
        "worker_default_enabled": False, "writer_default_enabled": False,
    }
    if args.output is not None:
        args.output.parent.mkdir(parents=True, exist_ok=True)
        temporary = args.output.with_name(f".{args.output.name}.tmp")
        temporary.write_text(json.dumps(envelope, indent=2, sort_keys=True) + "\n", encoding="utf-8")
        temporary.replace(args.output)
    print(json.dumps(envelope, sort_keys=True))


if __name__ == "__main__":
    main()
