#!/usr/bin/env python3
"""Run the M08 internal Python/ONNX/Flink parity profile on Kubernetes."""

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
CONFIG_FILES = {
    "model_inference_parity.py": ROOT / "mlops/scripts/model_inference_parity.py",
    "profile.json": ROOT / "contracts/mlops/m08-model-inference-parity-internal.v1.json",
    "profile.schema.json": ROOT / "contracts/mlops/model-inference-parity-profile.schema.json",
    "result.schema.json": ROOT / "contracts/mlops/model-inference-parity-result.schema.json",
}


class CanaryError(RuntimeError):
    pass


def run(
    command: list[str], *, input_text: str | None = None, check: bool = True,
) -> subprocess.CompletedProcess[str]:
    result = subprocess.run(command, input=input_text, text=True, capture_output=True)
    if check and result.returncode != 0:
        detail = result.stderr.strip() or result.stdout.strip()
        raise CanaryError(f"command failed ({' '.join(command[:5])}): {detail}")
    return result


def kubectl(
    *args: str, input_text: str | None = None, check: bool = True,
) -> subprocess.CompletedProcess[str]:
    return run(["kubectl", *args], input_text=input_text, check=check)


def sha256(path: Path) -> str:
    return hashlib.sha256(path.read_bytes()).hexdigest()


def validate_inputs(
    python_image: str, flink_image: str, candidate: str, run_id: str, node: str,
) -> str:
    for name, image in (("python", python_image), ("flink", flink_image)):
        if not image or image.endswith(":latest") or "@sha256:" in image:
            raise CanaryError(f"--{name}-image must be a non-latest local candidate tag")
    if not re.fullmatch(r"[0-9a-f]{64}", candidate):
        raise CanaryError("--candidate-sha256 must be lowercase SHA-256")
    try:
        parsed = uuid.UUID(run_id)
    except ValueError as exc:
        raise CanaryError("--run-id must be a canonical lowercase UUID") from exc
    if str(parsed) != run_id:
        raise CanaryError("--run-id must be a canonical lowercase UUID")
    if not re.fullmatch(r"[a-zA-Z0-9.-]+", node):
        raise CanaryError("invalid Kubernetes node name")
    for name, path in CONFIG_FILES.items():
        if not path.is_file() or path.stat().st_size == 0:
            raise CanaryError(f"missing parity input: {name}")
    for name in ("profile.json", "profile.schema.json", "result.schema.json"):
        try:
            json.loads(CONFIG_FILES[name].read_text(encoding="utf-8"))
        except json.JSONDecodeError as exc:
            raise CanaryError(f"invalid parity JSON input: {name}") from exc
    return run_id.replace("-", "")[:10]


def labels(run_id: str) -> dict[str, str]:
    return {
        "app.kubernetes.io/name": "m08-model-inference-parity",
        "traffic.analysis/canary-run": run_id,
        "traffic.analysis/task": "t1-m08-n017",
    }


def names(suffix: str) -> dict[str, str]:
    return {
        "config": f"m08-n017-parity-{suffix}",
        "job": f"m08-n017-parity-{suffix}",
    }


def container_security() -> dict[str, Any]:
    return {
        "allowPrivilegeEscalation": False,
        "readOnlyRootFilesystem": True,
        "capabilities": {"drop": ["ALL"]},
    }


def mounts() -> list[dict[str, Any]]:
    return [
        {"name": "contracts", "mountPath": "/parity-contracts", "readOnly": True},
        {"name": "work", "mountPath": "/parity-work"},
        {"name": "tmp", "mountPath": "/tmp"},
    ]


def build_objects(
    resource_names: dict[str, str], python_image: str, flink_image: str,
    candidate: str, run_id: str, node: str,
) -> list[dict[str, Any]]:
    common_labels = labels(run_id)
    config = {
        "apiVersion": "v1",
        "kind": "ConfigMap",
        "metadata": {
            "name": resource_names["config"],
            "namespace": NAMESPACE,
            "labels": common_labels,
        },
        "data": {
            name: path.read_text(encoding="utf-8")
            for name, path in CONFIG_FILES.items()
        },
    }
    common_args = [
        "--workdir", "/parity-work",
        "--profile", "/parity-contracts/profile.json",
        "--profile-schema", "/parity-contracts/profile.schema.json",
    ]
    job = {
        "apiVersion": "batch/v1",
        "kind": "Job",
        "metadata": {
            "name": resource_names["job"],
            "namespace": NAMESPACE,
            "labels": common_labels,
            "annotations": {
                "traffic.analysis/shared-postgres-touched": "false",
                "traffic.analysis/shared-clickhouse-touched": "false",
                "traffic.analysis/shared-kafka-touched": "false",
                "traffic.analysis/shared-flink-touched": "false",
                "traffic.analysis/claim-scope": "internal-engineering-only",
            },
        },
        "spec": {
            "backoffLimit": 0,
            "activeDeadlineSeconds": 600,
            "ttlSecondsAfterFinished": 1800,
            "template": {
                "metadata": {
                    "labels": common_labels,
                    "annotations": {
                        "traffic.analysis/candidate-sha256": candidate,
                        "traffic.analysis/production-applied": "false",
                    },
                },
                "spec": {
                    "nodeName": node,
                    "automountServiceAccountToken": False,
                    "restartPolicy": "Never",
                    "securityContext": {
                        "runAsNonRoot": True,
                        "runAsUser": 1000,
                        "runAsGroup": 1000,
                        "fsGroup": 1000,
                        "seccompProfile": {"type": "RuntimeDefault"},
                    },
                    "initContainers": [
                        {
                            "name": "prepare-python-and-onnx",
                            "image": python_image,
                            "imagePullPolicy": "Never",
                            "command": ["python", "/parity-contracts/model_inference_parity.py"],
                            "args": [
                                "prepare", *common_args,
                                "--candidate-sha256", candidate,
                                "--run-id", run_id,
                            ],
                            "resources": {
                                "requests": {"cpu": "250m", "memory": "512Mi"},
                                "limits": {"cpu": "2", "memory": "2Gi"},
                            },
                            "securityContext": container_security(),
                            "volumeMounts": mounts(),
                        },
                        {
                            "name": "run-flink-route",
                            "image": flink_image,
                            "imagePullPolicy": "Never",
                            "command": ["java"],
                            "args": [
                                "-Djava.io.tmpdir=/tmp",
                                "-Dlog4j.configurationFile=file:/opt/flink/conf/log4j-console.properties",
                                "-cp", "/opt/flink/usrlib/flink-behavior-job.jar:/opt/flink/lib/*",
                                "com.traffic.flink.behavior.detector.ModelInferenceParityMain",
                                "/parity-work/parity-input.json",
                                "/parity-work/baseline-model.onnx",
                                "/parity-work/flink-receipt.json",
                            ],
                            "resources": {
                                "requests": {"cpu": "250m", "memory": "768Mi"},
                                "limits": {"cpu": "2", "memory": "2Gi"},
                            },
                            "securityContext": container_security(),
                            "volumeMounts": mounts(),
                        },
                    ],
                    "containers": [
                        {
                            "name": "finalize-parity",
                            "image": python_image,
                            "imagePullPolicy": "Never",
                            "command": ["python", "/parity-contracts/model_inference_parity.py"],
                            "args": [
                                "finalize", *common_args,
                                "--result-schema", "/parity-contracts/result.schema.json",
                                "--output", "/parity-work/result.json",
                            ],
                            "resources": {
                                "requests": {"cpu": "100m", "memory": "256Mi"},
                                "limits": {"cpu": "1", "memory": "1Gi"},
                            },
                            "securityContext": container_security(),
                            "volumeMounts": mounts(),
                        }
                    ],
                    "volumes": [
                        {"name": "contracts", "configMap": {"name": resource_names["config"]}},
                        {"name": "work", "emptyDir": {"sizeLimit": "1Gi"}},
                        {"name": "tmp", "emptyDir": {"sizeLimit": "1Gi"}},
                    ],
                },
            },
        },
    }
    return [config, job]


def apply(objects: list[dict[str, Any]]) -> None:
    body = "\n---\n".join(yaml.safe_dump(item, sort_keys=False) for item in objects)
    kubectl("apply", "-f", "-", input_text=body)


def pod_for_job(job_name: str) -> dict[str, Any]:
    result = kubectl(
        "get", "pod", "-n", NAMESPACE, "-l", f"job-name={job_name}", "-o", "json",
    )
    pods = json.loads(result.stdout).get("items", [])
    if len(pods) != 1:
        raise CanaryError(f"expected one parity pod, got {len(pods)}")
    return pods[0]


def wait_and_collect(
    job_name: str, timeout: int, candidate: str, node: str,
    python_image: str, flink_image: str,
) -> tuple[dict[str, Any], dict[str, Any]]:
    deadline = time.time() + timeout
    succeeded = False
    failed = False
    job: dict[str, Any] = {}
    while time.time() < deadline:
        job = json.loads(kubectl(
            "get", "job", job_name, "-n", NAMESPACE, "-o", "json",
        ).stdout)
        status = job.get("status", {})
        if status.get("succeeded", 0) == 1:
            succeeded = True
            break
        if status.get("failed", 0) > 0:
            failed = True
            break
        time.sleep(2)
    pod = pod_for_job(job_name)
    pod_name = pod["metadata"]["name"]
    logs = {
        name: kubectl("logs", pod_name, "-n", NAMESPACE, "-c", name, check=False).stdout
        for name in ("prepare-python-and-onnx", "run-flink-route", "finalize-parity")
    }
    if not succeeded:
        disposition = "failed" if failed else "timed out"
        raise CanaryError(
            f"Kubernetes parity job {disposition}: " + json.dumps(logs)
        )
    if pod.get("spec", {}).get("nodeName") != node:
        raise CanaryError("Kubernetes parity pod ran on the wrong node")
    annotation = pod.get("metadata", {}).get("annotations", {}).get(
        "traffic.analysis/candidate-sha256"
    )
    if annotation != candidate:
        raise CanaryError("Kubernetes parity pod lost candidate identity")
    spec = pod["spec"]
    images = {container["name"]: container["image"] for container in spec["initContainers"]}
    images.update({container["name"]: container["image"] for container in spec["containers"]})
    if images != {
        "prepare-python-and-onnx": python_image,
        "run-flink-route": flink_image,
        "finalize-parity": python_image,
    }:
        raise CanaryError("Kubernetes parity pod image binding drifted")
    result_objects = []
    for line in logs["finalize-parity"].splitlines():
        if not line.startswith("{"):
            continue
        try:
            value = json.loads(line)
        except json.JSONDecodeError:
            continue
        if isinstance(value, dict) and value.get("result_sha256"):
            result_objects.append(value)
    if len(result_objects) != 1:
        raise CanaryError(f"expected one parity result, got {len(result_objects)}")
    result = result_objects[0]
    expected = {
        "run_id": pod["metadata"]["labels"]["traffic.analysis/canary-run"],
        "candidate_sha256": candidate,
        "claim_scope": "INTERNAL_ENGINEERING_ONLY",
        "cnas_claim_authorized": False,
        "production_promotion_authorized": False,
        "status": "PASS",
        "sample_count": 64,
    }
    for field, wanted in expected.items():
        if result.get(field) != wanted:
            raise CanaryError(f"parity result {field} drifted: {result.get(field)!r}")
    statuses = pod.get("status", {}).get("initContainerStatuses", [])
    statuses += pod.get("status", {}).get("containerStatuses", [])
    if len(statuses) != 3 or any(
        item.get("state", {}).get("terminated", {}).get("exitCode") != 0 for item in statuses
    ):
        raise CanaryError("Kubernetes parity container termination status is incomplete")
    evidence = {
        "namespace": NAMESPACE,
        "job_name": job_name,
        "job_uid": job.get("metadata", {}).get("uid"),
        "pod_name": pod_name,
        "pod_uid": pod["metadata"]["uid"],
        "node": node,
        "started_at": pod.get("status", {}).get("startTime"),
        "completed_at": job.get("status", {}).get("completionTime"),
        "container_ids": {
            item["name"]: item.get("containerID", "") for item in statuses
        },
        "image_ids": {
            item["name"]: item.get("imageID", "") for item in statuses
        },
        "images": images,
        "shared_postgres_touched": False,
        "shared_clickhouse_touched": False,
        "shared_kafka_touched": False,
        "shared_flink_touched": False,
    }
    return result, evidence


def cleanup(run_id: str) -> None:
    selector = f"traffic.analysis/canary-run={run_id}"
    kubectl(
        "delete", "job,configmap", "-n", NAMESPACE, "-l", selector,
        "--ignore-not-found=true", "--wait=true", "--timeout=120s", check=False,
    )
    deadline = time.time() + 30
    while time.time() < deadline:
        remaining = kubectl(
            "get", "job,configmap", "-n", NAMESPACE, "-l", selector,
            "-o", "name", check=False,
        ).stdout.strip()
        if not remaining:
            return
        time.sleep(1)
    raise CanaryError("run-scoped parity resources were not removed")


def main() -> None:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--python-image", required=True)
    parser.add_argument("--flink-image", required=True)
    parser.add_argument("--candidate-sha256", required=True)
    parser.add_argument("--run-id", default=str(uuid.uuid4()))
    parser.add_argument("--node", default="8-2tb")
    parser.add_argument("--timeout", type=int, default=600)
    parser.add_argument("--keep", action="store_true")
    parser.add_argument(
        "--output",
        type=Path,
        help="write the final evidence envelope atomically as JSON",
    )
    args = parser.parse_args()
    if args.timeout < 60 or args.timeout > 1200:
        raise CanaryError("--timeout must be between 60 and 1200 seconds")
    suffix = validate_inputs(
        args.python_image, args.flink_image, args.candidate_sha256,
        args.run_id, args.node,
    )
    resource_names = names(suffix)
    objects = build_objects(
        resource_names, args.python_image, args.flink_image,
        args.candidate_sha256, args.run_id, args.node,
    )
    applied = False
    result: dict[str, Any] | None = None
    evidence: dict[str, Any] | None = None
    try:
        apply(objects)
        applied = True
        result, evidence = wait_and_collect(
            resource_names["job"], args.timeout, args.candidate_sha256,
            args.node, args.python_image, args.flink_image,
        )
    finally:
        if applied and not args.keep:
            cleanup(args.run_id)
    if result is None or evidence is None:
        raise CanaryError("Kubernetes parity canary did not produce a result")
    envelope = {
        "task_id": "T1-M08-N017",
        "parity_result": result,
        "kubernetes": evidence,
        "config_sha256": {name: sha256(path) for name, path in CONFIG_FILES.items()},
        "run_scoped_resources_removed": not args.keep,
        "production_applied": False,
    }
    if args.output is not None:
        args.output.parent.mkdir(parents=True, exist_ok=True)
        temporary = args.output.with_name(f".{args.output.name}.tmp")
        temporary.write_text(
            json.dumps(envelope, indent=2, sort_keys=True) + "\n",
            encoding="utf-8",
        )
        temporary.replace(args.output)
    print(json.dumps(envelope, sort_keys=True))


if __name__ == "__main__":
    main()
