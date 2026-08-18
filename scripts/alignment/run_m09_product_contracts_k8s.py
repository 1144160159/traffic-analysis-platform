#!/usr/bin/env python3
"""Run the M09 N001 contract validator in an isolated Kubernetes Job."""

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


ROOT = Path(__file__).resolve().parents[2]
NAMESPACE = "traffic-analysis"
FILES = tuple(
    Path(value)
    for value in (
        "contracts/alignment/feature-contract.schema.json",
        "contracts/alignment/canonical-registry.json",
        "contracts/alignment/work-packages.json",
        "contracts/alignment/feature-contract-registry.v1.json",
        "contracts/alignment/features/F-ENCRYPTED-001.json",
        "contracts/alignment/features/F-ENCRYPTED-002.json",
        "contracts/alignment/features/F-FORENSICS-001.json",
        "doc/07_alignment/runbooks/F-ENCRYPTED-001-rollback.md",
        "doc/07_alignment/runbooks/F-ENCRYPTED-002-rollback.md",
        "doc/07_alignment/runbooks/F-FORENSICS-001-rollback.md",
        "scripts/alignment/validate_m09_product_contracts.py",
    )
)
IMAGE_RE = re.compile(r"^[a-zA-Z0-9][a-zA-Z0-9._/@:-]+$")
NODE_RE = re.compile(r"^[a-z0-9]([-a-z0-9.]*[a-z0-9])?$")


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
    for relative in FILES:
        path = (ROOT / relative).resolve(strict=False)
        if not path.is_relative_to(ROOT.resolve()) or not path.is_file():
            raise CanaryError(f"required file missing: {relative}")
    return parsed.hex[:10]


def names(suffix: str) -> dict[str, str]:
    name = f"m09-n001-contract-{suffix}"
    return {"configmap": name, "job": name}


def build_objects(
    resource_names: dict[str, str], image: str, run_id: str, node: str
) -> list[dict[str, Any]]:
    labels = {
        "app.kubernetes.io/name": "m09-product-contract-canary",
        "traffic.analysis/canary-run": run_id,
    }
    data: dict[str, str] = {}
    items = []
    for index, relative in enumerate(FILES):
        key = f"file-{index:02d}"
        data[key] = (ROOT / relative).read_text(encoding="utf-8")
        items.append({"key": key, "path": str(relative)})
    configmap = {
        "apiVersion": "v1",
        "kind": "ConfigMap",
        "metadata": {
            "name": resource_names["configmap"],
            "namespace": NAMESPACE,
            "labels": labels,
        },
        "data": data,
    }
    job = {
        "apiVersion": "batch/v1",
        "kind": "Job",
        "metadata": {
            "name": resource_names["job"],
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
                        "runAsUser": 65532,
                        "runAsGroup": 65532,
                        "fsGroup": 65532,
                        "seccompProfile": {"type": "RuntimeDefault"},
                    },
                    "containers": [
                        {
                            "name": "validate-contracts",
                            "image": image,
                            "imagePullPolicy": "Never",
                            "command": ["python3"],
                            "args": [
                                "/workspace/scripts/alignment/validate_m09_product_contracts.py"
                            ],
                            "securityContext": {
                                "allowPrivilegeEscalation": False,
                                "readOnlyRootFilesystem": True,
                                "capabilities": {"drop": ["ALL"]},
                            },
                            "resources": {
                                "requests": {"cpu": "100m", "memory": "128Mi"},
                                "limits": {"cpu": "1", "memory": "512Mi"},
                            },
                            "volumeMounts": [
                                {"name": "workspace", "mountPath": "/workspace", "readOnly": True},
                                {"name": "tmp", "mountPath": "/tmp"},
                            ],
                        }
                    ],
                    "volumes": [
                        {
                            "name": "workspace",
                            "configMap": {
                                "name": resource_names["configmap"],
                                "items": items,
                            },
                        },
                        {"name": "tmp", "emptyDir": {}},
                    ],
                },
            },
        },
    }
    return [configmap, job]


def wait_and_collect(job_name: str, timeout: int) -> tuple[dict[str, Any], dict[str, Any]]:
    deadline = time.time() + timeout
    job: dict[str, Any] = {}
    while time.time() < deadline:
        response = kubectl(
            "get", "job", job_name, "-n", NAMESPACE, "-o", "json", check=False
        )
        if response.returncode == 0:
            job = json.loads(response.stdout)
            status = job.get("status", {})
            if status.get("succeeded") == 1:
                break
            if status.get("failed", 0) > 0:
                logs = kubectl("logs", "-n", NAMESPACE, f"job/{job_name}", check=False)
                raise CanaryError(f"Kubernetes validation failed:\n{logs.stdout}{logs.stderr}")
        time.sleep(1)
    else:
        raise CanaryError(f"timed out waiting for Job {job_name}")
    pods = json.loads(
        kubectl(
            "get", "pod", "-n", NAMESPACE, "-l", f"job-name={job_name}", "-o", "json"
        ).stdout
    )["items"]
    if len(pods) != 1:
        raise CanaryError(f"expected one validation pod, observed {len(pods)}")
    pod = pods[0]
    logs = kubectl("logs", "-n", NAMESPACE, pod["metadata"]["name"]).stdout.strip()
    try:
        result = json.loads(logs.splitlines()[-1])
    except (IndexError, json.JSONDecodeError) as error:
        raise CanaryError(f"validation logs do not end with JSON: {logs}") from error
    if result.get("status") != "PASS":
        raise CanaryError(f"validation did not pass: {result}")
    container = pod["status"]["containerStatuses"][0]
    return result, {
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
        "delete", "job,configmap", "-n", NAMESPACE, "-l", selector,
        "--ignore-not-found=true", "--wait=true", "--timeout=120s", check=False,
    )
    remaining = kubectl(
        "get", "job,configmap,pod", "-n", NAMESPACE, "-l", selector, "-o", "name", check=False
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
    resource_names = names(suffix)
    objects = build_objects(resource_names, args.image, args.run_id, args.node)
    applied = False
    result = evidence = None
    try:
        kubectl("apply", "-f", "-", input_text=yaml.safe_dump_all(objects, sort_keys=False))
        applied = True
        result, evidence = wait_and_collect(resource_names["job"], args.timeout)
    finally:
        if applied and not args.keep:
            cleanup(args.run_id)
    if result is None or evidence is None:
        raise CanaryError("Kubernetes validation produced no result")
    envelope = {
        "artifact_kind": "M09_PRODUCT_CONTRACT_TEST_RESULT",
        "task_id": "T1-M09-N001",
        "run_id": args.run_id,
        "status": "PASS",
        "profile_id": "M09-N001-K8S-READONLY-PRODUCT-CONTRACT-V1",
        "inputs": {
            "candidate_image": args.image,
            "source_sha256": {
                str(relative): hashlib.sha256((ROOT / relative).read_bytes()).hexdigest()
                for relative in FILES
            },
        },
        "validation": result,
        "kubernetes": evidence,
        "run_scoped_resources_removed": not args.keep,
        "production_applied": False,
        "does_not_prove": [
            "production rollout",
            "same-candidate M09 integration",
            "Windows Chrome acceptance",
            "authorization to promote M09",
        ],
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
