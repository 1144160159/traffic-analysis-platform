#!/usr/bin/env python3
"""Run the M09 N002 encrypted-stats characterization binary in Kubernetes."""

from __future__ import annotations

import argparse
import hashlib
import json
import re
import subprocess
import time
import uuid
from pathlib import Path
from typing import Any

import yaml


NAMESPACE = "traffic-analysis"
IMAGE_RE = re.compile(r"^[a-zA-Z0-9][a-zA-Z0-9._/@:-]+$")
NODE_RE = re.compile(r"^[a-z0-9]([-a-z0-9.]*[a-z0-9])?$")
TEST_PATTERN = "Test(GetEncryptedTrafficStats|ClickHouseEncryptedTrafficStatsService|EncryptedTraffic)"


class CanaryError(RuntimeError):
    pass


def kubectl(*args: str, input_text: str | None = None, check: bool = True):
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


def validate_inputs(image: str, run_id: str, node: str) -> str:
    if not IMAGE_RE.fullmatch(image):
        raise CanaryError("invalid image reference")
    if not NODE_RE.fullmatch(node):
        raise CanaryError("invalid Kubernetes node name")
    try:
        parsed = uuid.UUID(run_id)
    except ValueError as error:
        raise CanaryError("run-id must be a UUID") from error
    return parsed.hex[:10]


def build_job(name: str, image: str, run_id: str, node: str) -> dict[str, Any]:
    labels = {
        "app.kubernetes.io/name": "m09-encrypted-stats-seam-canary",
        "traffic.analysis/canary-run": run_id,
    }
    return {
        "apiVersion": "batch/v1",
        "kind": "Job",
        "metadata": {
            "name": name,
            "namespace": NAMESPACE,
            "labels": labels,
            "annotations": {
                "traffic.analysis/shared-postgres-touched": "false",
                "traffic.analysis/shared-clickhouse-touched": "false",
                "traffic.analysis/shared-kafka-touched": "false",
                "traffic.analysis/shared-minio-touched": "false",
                "traffic.analysis/shared-flink-touched": "false",
                "traffic.analysis/production-applied": "false",
            },
        },
        "spec": {
            "backoffLimit": 0,
            "ttlSecondsAfterFinished": 300,
            "template": {
                "metadata": {"labels": labels},
                "spec": {
                    "automountServiceAccountToken": False,
                    "restartPolicy": "Never",
                    "nodeName": node,
                    "securityContext": {
                        "runAsNonRoot": True,
                        "runAsUser": 1000,
                        "runAsGroup": 1000,
                        "fsGroup": 1000,
                        "seccompProfile": {"type": "RuntimeDefault"},
                    },
                    "containers": [
                        {
                            "name": "go-characterization",
                            "image": image,
                            "imagePullPolicy": "Never",
                            "args": ["-test.v", f"-test.run={TEST_PATTERN}", "-test.count=1"],
                            "securityContext": {
                                "allowPrivilegeEscalation": False,
                                "readOnlyRootFilesystem": True,
                                "capabilities": {"drop": ["ALL"]},
                            },
                            "resources": {
                                "requests": {"cpu": "100m", "memory": "128Mi"},
                                "limits": {"cpu": "1", "memory": "512Mi"},
                            },
                            "volumeMounts": [{"name": "tmp", "mountPath": "/tmp"}],
                        }
                    ],
                    "volumes": [{"name": "tmp", "emptyDir": {}}],
                },
            },
        },
    }


def wait_and_collect(job_name: str, timeout: int) -> tuple[str, dict[str, Any]]:
    deadline = time.time() + timeout
    job: dict[str, Any] = {}
    while time.time() < deadline:
        response = kubectl("get", "job", job_name, "-n", NAMESPACE, "-o", "json", check=False)
        if response.returncode == 0:
            job = json.loads(response.stdout)
            status = job.get("status", {})
            if status.get("succeeded") == 1:
                break
            if status.get("failed", 0) > 0:
                logs = kubectl("logs", "-n", NAMESPACE, f"job/{job_name}", check=False)
                raise CanaryError(f"Kubernetes characterization failed:\n{logs.stdout}{logs.stderr}")
        time.sleep(1)
    else:
        raise CanaryError(f"timed out waiting for Job {job_name}")

    pods = json.loads(
        kubectl("get", "pod", "-n", NAMESPACE, "-l", f"job-name={job_name}", "-o", "json").stdout
    )["items"]
    if len(pods) != 1:
        raise CanaryError(f"expected one characterization pod, observed {len(pods)}")
    pod = pods[0]
    logs = kubectl("logs", "-n", NAMESPACE, pod["metadata"]["name"]).stdout
    if "PASS" not in logs:
        raise CanaryError(f"test output does not contain PASS:\n{logs}")
    container = pod["status"]["containerStatuses"][0]
    return logs, {
        "namespace": NAMESPACE,
        "job_name": job_name,
        "job_uid": job["metadata"]["uid"],
        "pod_name": pod["metadata"]["name"],
        "pod_uid": pod["metadata"]["uid"],
        "node": pod["spec"]["nodeName"],
        "started_at": pod.get("status", {}).get("startTime"),
        "completed_at": job.get("status", {}).get("completionTime"),
        "container_id": container.get("containerID"),
        "image": container.get("image"),
        "image_id": container.get("imageID"),
        "shared_postgres_touched": False,
        "shared_clickhouse_touched": False,
        "shared_kafka_touched": False,
        "shared_minio_touched": False,
        "shared_flink_touched": False,
    }


def cleanup(run_id: str) -> None:
    selector = f"traffic.analysis/canary-run={run_id}"
    kubectl(
        "delete", "job", "-n", NAMESPACE, "-l", selector,
        "--ignore-not-found=true", "--wait=true", "--timeout=120s", check=False,
    )
    remaining = kubectl(
        "get", "job,pod", "-n", NAMESPACE, "-l", selector, "-o", "name", check=False
    ).stdout.strip()
    if remaining:
        raise CanaryError(f"run-scoped resources remain: {remaining}")


def main() -> None:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--image", required=True)
    parser.add_argument("--run-id", default=str(uuid.uuid4()))
    parser.add_argument("--node", default="8-2tb")
    parser.add_argument("--timeout", type=int, default=300)
    parser.add_argument("--output", type=Path)
    parser.add_argument("--keep", action="store_true")
    args = parser.parse_args()
    if args.timeout < 30 or args.timeout > 900:
        raise CanaryError("--timeout must be between 30 and 900 seconds")

    suffix = validate_inputs(args.image, args.run_id, args.node)
    job_name = f"m09-n002-stats-{suffix}"
    applied = False
    logs = ""
    evidence: dict[str, Any] | None = None
    try:
        kubectl("apply", "-f", "-", input_text=yaml.safe_dump(build_job(job_name, args.image, args.run_id, args.node), sort_keys=False))
        applied = True
        logs, evidence = wait_and_collect(job_name, args.timeout)
    finally:
        if applied and not args.keep:
            cleanup(args.run_id)
    if evidence is None:
        raise CanaryError("Kubernetes characterization produced no result")

    envelope = {
        "artifact_kind": "M09_ENCRYPTED_STATS_SEAM_TEST_RESULT",
        "task_id": "T1-M09-N002",
        "run_id": args.run_id,
        "status": "PASS",
        "test_pattern": TEST_PATTERN,
        "test_output_sha256": hashlib.sha256(logs.encode("utf-8")).hexdigest(),
        "kubernetes": evidence,
        "run_scoped_resources_removed": not args.keep,
        "production_applied": False,
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
