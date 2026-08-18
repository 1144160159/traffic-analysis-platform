#!/usr/bin/env python3
"""Verify M09 N009 against a run-scoped PostgreSQL authority in Kubernetes."""

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
PREREQUISITES = ROOT / "scripts/alignment/fixtures/m09_forensics_task_prerequisites.sql"
MIGRATION = ROOT / "deployments/postgres/migrations/202608031600_forensics_task_command_atomic.sql"
TEST_PATTERN = "^TestForensicsTaskCommandAtomicEphemeralPostgres$"
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
        "app.kubernetes.io/name": "m09-forensics-task-command-canary",
        "traffic.analysis/canary-run": run_id,
    }


def validate(image: str, run_id: str, node: str) -> str:
    if not IMAGE_RE.fullmatch(image) or image.endswith(":latest"):
        raise CanaryError("--image must be a non-latest candidate image")
    if not NODE_RE.fullmatch(node):
        raise CanaryError("invalid Kubernetes node name")
    parsed = uuid.UUID(run_id)
    if str(parsed) != run_id:
        raise CanaryError("--run-id must be a canonical lowercase UUID")
    for path in (PREREQUISITES, MIGRATION):
        if not path.is_file():
            raise CanaryError(f"missing PostgreSQL input: {path.relative_to(ROOT)}")
    return parsed.hex[:10]


def objects(name: str, image: str, run_id: str, node: str, password: str) -> list[dict[str, Any]]:
    common = labels(run_id)
    dsn = f"postgres://postgres:{password}@{name}.{DB_NAMESPACE}.svc:5432/traffic_platform?sslmode=disable"
    selector = {"traffic.analysis/canary-postgres": name}
    return [
        {
            "apiVersion": "v1", "kind": "ConfigMap",
            "metadata": {"name": name, "namespace": DB_NAMESPACE, "labels": common},
            "data": {
                "00-prerequisites.sql": PREREQUISITES.read_text(encoding="utf-8"),
                "10-forensics-task-command.sql": MIGRATION.read_text(encoding="utf-8"),
            },
        },
        {
            "apiVersion": "v1", "kind": "Secret", "type": "Opaque",
            "metadata": {"name": name, "namespace": DB_NAMESPACE, "labels": common},
            "stringData": {"password": password},
        },
        {
            "apiVersion": "v1", "kind": "Secret", "type": "Opaque",
            "metadata": {"name": name, "namespace": APP_NAMESPACE, "labels": common},
            "stringData": {"dsn": dsn},
        },
        {
            "apiVersion": "v1", "kind": "Service",
            "metadata": {"name": name, "namespace": DB_NAMESPACE, "labels": common},
            "spec": {"selector": selector, "ports": [{"name": "postgres", "port": 5432, "targetPort": 5432}]},
        },
        {
            "apiVersion": "v1", "kind": "Pod",
            "metadata": {"name": name, "namespace": DB_NAMESPACE, "labels": {**common, **selector}},
            "spec": {
                "nodeName": node, "automountServiceAccountToken": False, "restartPolicy": "Never",
                "securityContext": {"seccompProfile": {"type": "RuntimeDefault"}},
                "containers": [{
                    "name": "postgres", "image": POSTGRES_IMAGE, "imagePullPolicy": "IfNotPresent",
                    "env": [
                        {"name": "POSTGRES_DB", "value": "traffic_platform"},
                        {"name": "POSTGRES_USER", "value": "postgres"},
                        {"name": "POSTGRES_PASSWORD", "valueFrom": {"secretKeyRef": {"name": name, "key": "password"}}},
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
            "apiVersion": "batch/v1", "kind": "Job",
            "metadata": {
                "name": name, "namespace": APP_NAMESPACE, "labels": common,
                "annotations": {
                    "traffic.analysis/shared-postgres-touched": "false",
                    "traffic.analysis/shared-clickhouse-touched": "false",
                    "traffic.analysis/shared-kafka-touched": "false",
                    "traffic.analysis/shared-minio-touched": "false",
                    "traffic.analysis/production-applied": "false",
                },
            },
            "spec": {
                "backoffLimit": 0,
                "template": {
                    "metadata": {"labels": common},
                    "spec": {
                        "nodeName": node, "automountServiceAccountToken": False, "restartPolicy": "Never",
                        "securityContext": {"runAsNonRoot": True, "runAsUser": 1000, "runAsGroup": 1000, "fsGroup": 1000, "seccompProfile": {"type": "RuntimeDefault"}},
                        "containers": [{
                            "name": "go-integration", "image": image, "imagePullPolicy": "Never",
                            "args": ["-test.v", f"-test.run={TEST_PATTERN}", "-test.count=1"],
                            "env": [{"name": "FORENSICS_TASK_ATOMIC_EPHEMERAL_PG_DSN", "valueFrom": {"secretKeyRef": {"name": name, "key": "dsn"}}}],
                            "securityContext": {"allowPrivilegeEscalation": False, "readOnlyRootFilesystem": True, "capabilities": {"drop": ["ALL"]}},
                            "resources": {"requests": {"cpu": "100m", "memory": "128Mi"}, "limits": {"cpu": "1", "memory": "512Mi"}},
                            "volumeMounts": [{"name": "tmp", "mountPath": "/tmp"}],
                        }],
                        "volumes": [{"name": "tmp", "emptyDir": {}}],
                    },
                },
            },
        },
    ]


def apply(items: list[dict[str, Any]]) -> None:
    body = "\n---\n".join(yaml.safe_dump(item, sort_keys=False) for item in items)
    kubectl("apply", "-f", "-", input_text=body)


def wait_ready(name: str, timeout: int) -> None:
    result = kubectl("wait", "--for=condition=Ready", f"pod/{name}", "-n", DB_NAMESPACE, f"--timeout={timeout}s", check=False)
    if result.returncode != 0:
        logs = kubectl("logs", name, "-n", DB_NAMESPACE, check=False).stdout
        raise CanaryError(f"ephemeral PostgreSQL did not become ready:\n{logs}")


def wait_job(name: str, timeout: int) -> tuple[str, dict[str, Any]]:
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
                raise CanaryError(f"Kubernetes forensics task integration failed:\n{logs}")
        time.sleep(1)
    else:
        logs = kubectl("logs", f"job/{name}", "-n", APP_NAMESPACE, check=False).stdout
        raise CanaryError(f"Kubernetes forensics task integration timed out:\n{logs}")
    logs = kubectl("logs", f"job/{name}", "-n", APP_NAMESPACE, check=False).stdout
    if "PASS" not in logs or "SKIP" in logs:
        raise CanaryError(f"Kubernetes forensics task integration did not prove execution:\n{logs}")
    pods = json.loads(kubectl("get", "pod", "-n", APP_NAMESPACE, "-l", f"job-name={name}", "-o", "json").stdout)["items"]
    if len(pods) != 1:
        raise CanaryError(f"expected one integration pod, observed {len(pods)}")
    pod = pods[0]
    container = pod["status"]["containerStatuses"][0]
    return logs, {
        "namespace": APP_NAMESPACE, "job_name": name, "job_uid": job["metadata"]["uid"],
        "pod_name": pod["metadata"]["name"], "pod_uid": pod["metadata"]["uid"], "node": pod["spec"]["nodeName"],
        "container_id": container.get("containerID"), "image": container.get("image"), "image_id": container.get("imageID"),
        "started_at": pod.get("status", {}).get("startTime"), "completed_at": job.get("status", {}).get("completionTime"),
        "shared_postgres_touched": False, "shared_clickhouse_touched": False,
        "shared_kafka_touched": False, "shared_minio_touched": False,
    }


def postgres_oracle(name: str) -> dict[str, Any]:
    sql = """SELECT json_build_object(
      'migration_count',(SELECT count(*) FROM alignment_schema_migrations WHERE version='202608031600'),
      'sentinel',(SELECT marker FROM codex_ephemeral_forensics_task_sentinel LIMIT 1),
      'remaining_tasks',(SELECT count(*) FROM tasks),
      'remaining_history',(SELECT count(*) FROM forensics_task_history),
      'remaining_outbox',(SELECT count(*) FROM forensics_task_outbox),
      'remaining_requests',(SELECT count(*) FROM forensics_task_requests)
    );"""
    result = kubectl("exec", "-i", "-n", DB_NAMESPACE, name, "--", "sh", "-ec",
                     'PGPASSWORD="$POSTGRES_PASSWORD" exec psql -v ON_ERROR_STOP=1 -U postgres -d traffic_platform -At',
                     input_text=sql)
    oracle = json.loads(result.stdout.strip())
    if oracle != {"migration_count": 1, "sentinel": "ephemeral-only", "remaining_tasks": 0, "remaining_history": 0, "remaining_outbox": 0, "remaining_requests": 0}:
        raise CanaryError(f"unexpected PostgreSQL cleanup oracle: {oracle}")
    return oracle


def cleanup(run_id: str) -> None:
    selector = f"traffic.analysis/canary-run={run_id}"
    for namespace in (APP_NAMESPACE, DB_NAMESPACE):
        kubectl("delete", "job,pod,service,configmap,secret", "-n", namespace, "-l", selector,
                "--ignore-not-found=true", "--wait=true", "--timeout=120s", check=False)
        remaining = kubectl("get", "job,pod,service,configmap,secret", "-n", namespace, "-l", selector, "-o", "name", check=False).stdout.strip()
        if remaining:
            raise CanaryError(f"run-scoped resources remain in {namespace}: {remaining}")


def main() -> None:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--image", required=True)
    parser.add_argument("--run-id", default=str(uuid.uuid4()))
    parser.add_argument("--node", default="8-2tb")
    parser.add_argument("--timeout", type=int, default=300)
    parser.add_argument("--output", type=Path)
    parser.add_argument("--keep", action="store_true")
    args = parser.parse_args()
    if args.timeout < 60 or args.timeout > 900:
        raise CanaryError("--timeout must be between 60 and 900 seconds")
    suffix = validate(args.image, args.run_id, args.node)
    name = f"m09-n009-forensics-{suffix}"
    password = "m09-" + secrets.token_hex(16)
    applied = False
    evidence: dict[str, Any] | None = None
    logs = ""
    oracle: dict[str, Any] = {}
    try:
        manifests = objects(name, args.image, args.run_id, args.node, password)
        apply(manifests[:-1])
        applied = True
        wait_ready(name, args.timeout)
        apply([manifests[-1]])
        logs, evidence = wait_job(name, args.timeout)
        oracle = postgres_oracle(name)
    finally:
        if applied and not args.keep:
            cleanup(args.run_id)
    if evidence is None:
        raise CanaryError("Kubernetes integration produced no evidence")
    envelope = {
        "artifact_kind": "M09_FORENSICS_TASK_COMMAND_TEST_RESULT",
        "task_id": "T1-M09-N009", "run_id": args.run_id, "status": "PASS",
        "test_pattern": TEST_PATTERN,
        "inputs": {
            "prerequisites_sha256": hashlib.sha256(PREREQUISITES.read_bytes()).hexdigest(),
            "migration_sha256": hashlib.sha256(MIGRATION.read_bytes()).hexdigest(),
            "postgres_image": POSTGRES_IMAGE,
        },
        "test_output_sha256": hashlib.sha256(logs.encode("utf-8")).hexdigest(),
        "postgres_oracle": oracle, "kubernetes": evidence,
        "run_scoped_resources_removed": not args.keep, "production_applied": False,
        "writer_default_enabled": False, "compatible_worker_required": True,
    }
    payload = json.dumps(envelope, sort_keys=True)
    if args.output is not None:
        args.output.parent.mkdir(parents=True, exist_ok=True)
        temporary = args.output.with_name(f".{args.output.name}.tmp")
        temporary.write_text(json.dumps(envelope, indent=2, sort_keys=True) + "\n", encoding="utf-8")
        temporary.replace(args.output)
    print(payload)


if __name__ == "__main__":
    main()
