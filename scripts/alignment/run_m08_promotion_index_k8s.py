#!/usr/bin/env python3
"""Validate the M08 exact evidence index in an isolated Kubernetes Job."""

from __future__ import annotations

import argparse
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
INPUT_RELATIVE = Path(
    "doc/02_acceptance/topic1/work-orders/t1-m08-p038-idx-n018-s1/promotion-input.json"
)
STATIC_FILES = (
    Path("contracts/alignment/m08-promotion-gate.v1.json"),
    Path("contracts/alignment/m08-promotion-manifest.schema.json"),
    Path("contracts/alignment/m08-release-pointer.schema.json"),
    Path("contracts/releases/topic1/t1-m08-release-pointer.json"),
    INPUT_RELATIVE,
    INPUT_RELATIVE.with_name("current-index.json"),
    Path("scripts/alignment/evaluate_m08_promotion.py"),
    Path("scripts/alignment/validate_m08_promotion_artifacts.py"),
)
IMAGE_RE = re.compile(r"^[a-zA-Z0-9][a-zA-Z0-9._/@:-]+$")
NODE_RE = re.compile(r"^[a-z0-9]([-a-z0-9.]*[a-z0-9])?$")


class CanaryError(RuntimeError):
    pass


def run(
    command: list[str], *, input_text: str | None = None, check: bool = True
) -> subprocess.CompletedProcess[str]:
    result = subprocess.run(
        command,
        input=input_text,
        text=True,
        stdout=subprocess.PIPE,
        stderr=subprocess.PIPE,
        check=False,
    )
    if check and result.returncode != 0:
        raise CanaryError(
            f"command failed ({result.returncode}): {' '.join(command)}\n"
            f"stdout:\n{result.stdout}\nstderr:\n{result.stderr}"
        )
    return result


def kubectl(*args: str, input_text: str | None = None, check: bool = True):
    return run(["kubectl", *args], input_text=input_text, check=check)


def required_files() -> list[Path]:
    promotion_input = json.loads((ROOT / INPUT_RELATIVE).read_text(encoding="utf-8"))
    evidence = promotion_input.get("evidence")
    if not isinstance(evidence, dict):
        raise CanaryError("promotion input evidence must be an object")
    relative_files = set(STATIC_FILES)
    for binding in evidence.values():
        if not isinstance(binding, dict) or not isinstance(binding.get("path"), str):
            raise CanaryError("promotion input contains an invalid evidence binding")
        relative_files.add(Path(binding["path"]))
    paths = sorted(relative_files, key=lambda path: str(path))
    for relative in paths:
        path = (ROOT / relative).resolve(strict=False)
        if not path.is_relative_to(ROOT.resolve()) or not path.is_file():
            raise CanaryError(f"required repository file is absent: {relative}")
    return paths


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


def names(suffix: str) -> dict[str, str]:
    return {
        "configmap": f"m08-n018-index-{suffix}",
        "job": f"m08-n018-index-{suffix}",
    }


def build_objects(
    resource_names: dict[str, str], image: str, run_id: str, node: str
) -> list[dict[str, Any]]:
    files = required_files()
    labels = {
        "app.kubernetes.io/name": "m08-promotion-index-canary",
        "traffic.analysis/canary-run": run_id,
    }
    data: dict[str, str] = {}
    items = []
    for index, relative in enumerate(files):
        key = f"file-{index:03d}"
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
                            "name": "validate-index",
                            "image": image,
                            "imagePullPolicy": "Never",
                            "command": ["python3"],
                            "args": [
                                "/workspace/scripts/alignment/validate_m08_promotion_artifacts.py"
                            ],
                            "resources": {
                                "requests": {"cpu": "100m", "memory": "128Mi"},
                                "limits": {"cpu": "1", "memory": "1Gi"},
                            },
                            "securityContext": {
                                "allowPrivilegeEscalation": False,
                                "readOnlyRootFilesystem": True,
                                "capabilities": {"drop": ["ALL"]},
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


def apply(objects: list[dict[str, Any]]) -> None:
    manifest = yaml.safe_dump_all(objects, sort_keys=False)
    kubectl("apply", "-f", "-", input_text=manifest)


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
                logs = kubectl(
                    "logs", "-n", NAMESPACE, f"job/{job_name}", check=False
                )
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
        raise CanaryError(f"validation log did not end with JSON: {logs}") from error
    if result.get("status") != "PASS":
        raise CanaryError(f"validation result is not PASS: {result}")
    container = pod["status"]["containerStatuses"][0]
    evidence = {
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
    return result, evidence


def cleanup(run_id: str) -> None:
    selector = f"traffic.analysis/canary-run={run_id}"
    kubectl(
        "delete",
        "job,configmap",
        "-n",
        NAMESPACE,
        "-l",
        selector,
        "--ignore-not-found=true",
        "--wait=true",
        "--timeout=120s",
        check=False,
    )
    remaining = kubectl(
        "get", "job,configmap,pod", "-n", NAMESPACE, "-l", selector, "-o", "name", check=False
    ).stdout.strip()
    if remaining:
        raise CanaryError(f"run-scoped resources remain after cleanup: {remaining}")


def write_output(path: Path, envelope: dict[str, Any]) -> None:
    payload = json.dumps(envelope, indent=2, sort_keys=True) + "\n"
    path.parent.mkdir(parents=True, exist_ok=True)
    temporary = path.with_name(f".{path.name}.tmp")
    temporary.write_text(payload, encoding="utf-8")
    temporary.replace(path)


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
    result: dict[str, Any] | None = None
    evidence: dict[str, Any] | None = None
    try:
        apply(objects)
        applied = True
        result, evidence = wait_and_collect(resource_names["job"], args.timeout)
    finally:
        if applied and not args.keep:
            cleanup(args.run_id)
    if result is None or evidence is None:
        raise CanaryError("Kubernetes index validation did not produce a result")
    envelope = {
        "task_id": "T1-M08-N018",
        "validation": result,
        "kubernetes": evidence,
        "run_scoped_resources_removed": not args.keep,
        "production_applied": False,
    }
    if args.output is not None:
        write_output(args.output, envelope)
    print(json.dumps(envelope, sort_keys=True))


if __name__ == "__main__":
    main()
