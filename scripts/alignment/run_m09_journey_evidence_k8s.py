#!/usr/bin/env python3
"""Run the N023 read-only journey evidence aggregator in Kubernetes."""

from __future__ import annotations

import argparse
import json
import os
import re
import subprocess
import sys
import time
import uuid
from pathlib import Path
from typing import Any

import yaml

ROOT = Path(__file__).resolve().parents[2]
if str(ROOT) not in sys.path:
    sys.path.insert(0, str(ROOT))

from scripts.alignment import verify_m09_journey_evidence as verifier


NAMESPACE = "traffic-analysis"
IMAGE_RE = re.compile(r"^[a-zA-Z0-9][a-zA-Z0-9._/@:-]+$")
NODE_RE = re.compile(r"^[a-z0-9]([-a-z0-9.]*[a-z0-9])?$")
STATIC_FILES = (
    verifier.CONTRACT_RELATIVE,
    verifier.INPUT_RELATIVE,
    Path("scripts/alignment/verify_m09_journey_evidence.py"),
)
SOURCE_HASH_FILES = (
    verifier.CONTRACT_RELATIVE,
    verifier.INPUT_RELATIVE,
    Path("scripts/alignment/verify_m09_journey_evidence.py"),
    Path("scripts/alignment/run_m09_journey_evidence_k8s.py"),
    Path("scripts/alignment/Dockerfile.m09-journey-evidence"),
)


class CanaryError(RuntimeError):
    pass


def run(
    command: list[str], *, input_text: str | None = None, check: bool = True
) -> subprocess.CompletedProcess[str]:
    environment = os.environ.copy()
    if command and command[0] == "kubectl":
        for key in (
            "HTTP_PROXY",
            "HTTPS_PROXY",
            "ALL_PROXY",
            "http_proxy",
            "https_proxy",
            "all_proxy",
        ):
            environment.pop(key, None)
    result = subprocess.run(
        command,
        input=input_text,
        text=True,
        stdout=subprocess.PIPE,
        stderr=subprocess.PIPE,
        check=False,
        env=environment,
    )
    if check and result.returncode != 0:
        raise CanaryError(
            f"command failed ({result.returncode}): {' '.join(command)}\n"
            f"stdout:\n{result.stdout}\nstderr:\n{result.stderr}"
        )
    return result


def kubectl(*args: str, input_text: str | None = None, check: bool = True):
    return run(["kubectl", *args], input_text=input_text, check=check)


def validate_inputs(image: str, run_id: str, node: str, timeout: int) -> str:
    if not IMAGE_RE.fullmatch(image):
        raise CanaryError("invalid image reference")
    if not NODE_RE.fullmatch(node):
        raise CanaryError("invalid Kubernetes node name")
    if timeout < 30 or timeout > 900:
        raise CanaryError("--timeout must be between 30 and 900 seconds")
    try:
        parsed = uuid.UUID(run_id)
    except ValueError as error:
        raise CanaryError("run-id must be a UUID") from error
    return parsed.hex[:10]


def required_files() -> list[Path]:
    contract = verifier.load_json(ROOT / verifier.CONTRACT_RELATIVE)
    relatives = set(STATIC_FILES)
    for binding in contract.get("source_evidence", {}).values():
        if not isinstance(binding, dict) or not isinstance(binding.get("path"), str):
            raise CanaryError("N023 source evidence binding is invalid")
        relatives.add(Path(binding["path"]))
    candidate = contract.get("candidate_binding", {})
    for field in ("config_path", "route_config_path"):
        if not isinstance(candidate.get(field), str):
            raise CanaryError(f"N023 candidate binding is missing {field}")
        relatives.add(Path(candidate[field]))
    paths = sorted(relatives, key=str)
    for relative in paths:
        try:
            path = verifier.resolve_repository_path(ROOT, str(relative))
        except ValueError as error:
            raise CanaryError(str(error)) from error
        if not path.is_file():
            raise CanaryError(f"required repository file is absent: {relative}")
    return paths


def names(suffix: str) -> dict[str, str]:
    return {
        "configmap": f"m09-n023-journeys-{suffix}",
        "job": f"m09-n023-journeys-{suffix}",
    }


def build_objects(
    resource_names: dict[str, str], image: str, run_id: str, node: str
) -> list[dict[str, Any]]:
    labels = {
        "app.kubernetes.io/name": "m09-journey-evidence-canary",
        "traffic.analysis/canary-run": run_id,
    }
    data: dict[str, str] = {}
    items: list[dict[str, str]] = []
    for index, relative in enumerate(required_files()):
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
                "traffic.analysis/shared-nebulagraph-touched": "false",
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
                            "name": "aggregate",
                            "image": image,
                            "imagePullPolicy": "Never",
                            "args": [
                                "--root",
                                "/workspace",
                                "--input-only",
                                "--json",
                            ],
                            "resources": {
                                "requests": {"cpu": "50m", "memory": "64Mi"},
                                "limits": {"cpu": "500m", "memory": "256Mi"},
                            },
                            "securityContext": {
                                "allowPrivilegeEscalation": False,
                                "readOnlyRootFilesystem": True,
                                "capabilities": {"drop": ["ALL"]},
                            },
                            "volumeMounts": [
                                {
                                    "name": "workspace",
                                    "mountPath": "/workspace",
                                    "readOnly": True,
                                },
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
    kubectl("apply", "-f", "-", input_text=yaml.safe_dump_all(objects, sort_keys=False))


def wait_and_collect(
    job_name: str, timeout: int
) -> tuple[dict[str, Any], dict[str, Any]]:
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
                raise CanaryError(
                    f"Kubernetes journey aggregation failed:\n{logs.stdout}{logs.stderr}"
                )
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
        raise CanaryError(f"journey validation result is not PASS: {result}")
    container = pod["status"]["containerStatuses"][0]
    identity = {
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
    }
    return result, identity


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
        "get",
        "job,configmap,pod",
        "-n",
        NAMESPACE,
        "-l",
        selector,
        "-o",
        "name",
        check=False,
    ).stdout.strip()
    if remaining:
        raise CanaryError(f"run-scoped resources remain after cleanup: {remaining}")


def build_envelope(
    image: str,
    run_id: str,
    validation: dict[str, Any],
    job: dict[str, Any],
    removed: bool,
) -> dict[str, Any]:
    source_hashes = {
        str(relative): verifier.sha256_file(ROOT / relative)
        for relative in SOURCE_HASH_FILES
    }
    return {
        "artifact_kind": "M09_WINDOWS_JOURNEY_EVIDENCE_AGGREGATION_RESULT",
        "task_id": "T1-M09-N023",
        "run_id": run_id,
        "status": "PASS",
        "coverage_status": validation["coverage_status"],
        "profile_id": "M09-N023-K8S-READONLY-EVIDENCE-AGGREGATOR-V1",
        "inputs": {"candidate_image": image, "source_sha256": source_hashes},
        "validation": validation,
        "kubernetes_job": job,
        "run_scoped_resources_removed": removed,
        "production_applied": False,
        "shared_postgres_touched": False,
        "shared_clickhouse_touched": False,
        "shared_kafka_touched": False,
        "shared_minio_touched": False,
        "shared_nebulagraph_touched": False,
        "does_not_prove": [
            "designated Windows Chrome journey acceptance",
            "production rollout",
            "cross-storage final facts absent from a verified journey receipt",
            "authorization to promote M09",
            "global milestone completion",
        ],
    }


def main() -> None:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--image", required=True)
    parser.add_argument("--run-id", default=str(uuid.uuid4()))
    parser.add_argument("--node", default="8-2tb")
    parser.add_argument("--timeout", type=int, default=300)
    parser.add_argument("--output", type=Path)
    parser.add_argument("--keep", action="store_true")
    args = parser.parse_args()
    suffix = validate_inputs(args.image, args.run_id, args.node, args.timeout)
    resource_names = names(suffix)
    objects = build_objects(resource_names, args.image, args.run_id, args.node)
    applied = False
    validation: dict[str, Any] | None = None
    job: dict[str, Any] | None = None
    try:
        apply(objects)
        applied = True
        validation, job = wait_and_collect(resource_names["job"], args.timeout)
    finally:
        if applied and not args.keep:
            cleanup(args.run_id)
    if validation is None or job is None:
        raise CanaryError("Kubernetes journey aggregation did not produce a result")
    envelope = build_envelope(args.image, args.run_id, validation, job, not args.keep)
    if args.output is not None:
        args.output.parent.mkdir(parents=True, exist_ok=True)
        args.output.write_text(
            json.dumps(envelope, indent=2, sort_keys=True) + "\n", encoding="utf-8"
        )
    print(json.dumps(envelope, sort_keys=True))


if __name__ == "__main__":
    main()
