#!/usr/bin/env python3
"""Run the M08 governed dataset/baseline seam as an isolated Kubernetes Job.

The job uses synthetic, run-scoped input and emptyDir storage.  It does not
query shared ClickHouse, write MinIO, submit an Argo workflow, or patch the
legacy/production MLOps workflow.  Those remain separate acceptance gates.
"""

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
NAMESPACE = "argo"
CONTRACT_FILES = (
    "dataset-manifest.schema.json",
    "evaluation-manifest.schema.json",
    "explanation-manifest.schema.json",
    "graph-snapshot-manifest.schema.json",
    "model-artifact-manifest.schema.json",
    "model-registration-receipt.schema.json",
    "training-run-manifest.schema.json",
)


class CanaryError(RuntimeError):
    pass


def run(
    command: list[str], *, input_text: str | None = None, check: bool = True,
) -> subprocess.CompletedProcess[str]:
    result = subprocess.run(command, input=input_text, text=True, capture_output=True)
    if check and result.returncode != 0:
        detail = result.stderr.strip() or result.stdout.strip()
        raise CanaryError(f"command failed ({' '.join(command[:4])}): {detail}")
    return result


def kubectl(
    *args: str, input_text: str | None = None, check: bool = True,
) -> subprocess.CompletedProcess[str]:
    return run(["kubectl", *args], input_text=input_text, check=check)


def sha256(path: Path) -> str:
    return hashlib.sha256(path.read_bytes()).hexdigest()


def validate_inputs(image: str, candidate: str, run_id: str, node: str) -> str:
    if not image or image.endswith(":latest") or "@sha256:" in image:
        raise CanaryError("--image must be a non-latest local candidate tag")
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
    for name in CONTRACT_FILES:
        path = ROOT / "contracts/mlops" / name
        if not path.is_file():
            raise CanaryError(f"missing M08 contract: {path.relative_to(ROOT)}")
        try:
            value = json.loads(path.read_text(encoding="utf-8"))
        except json.JSONDecodeError as exc:
            raise CanaryError(f"invalid M08 JSON contract: {name}") from exc
        if value.get("$schema") != "https://json-schema.org/draft/2020-12/schema":
            raise CanaryError(f"M08 contract is not draft 2020-12 JSON Schema: {name}")
    return run_id.replace("-", "")[:8]


def labels(run_id: str) -> dict[str, str]:
    return {
        "app.kubernetes.io/name": "m08-governed-training-canary",
        "traffic.analysis/canary-run": run_id,
    }


def resource_names(suffix: str) -> dict[str, str]:
    return {
        "contracts": f"m08-governed-contracts-{suffix}",
        "job": f"m08-governed-training-{suffix}",
    }


def build_objects(
    names: dict[str, str], image: str, candidate: str, run_id: str, node: str,
) -> list[dict[str, Any]]:
    common_labels = labels(run_id)
    contract_data = {
        name: (ROOT / "contracts/mlops" / name).read_text(encoding="utf-8")
        for name in CONTRACT_FILES
    }
    config_map = {
        "apiVersion": "v1",
        "kind": "ConfigMap",
        "metadata": {"name": names["contracts"], "namespace": NAMESPACE, "labels": common_labels},
        "data": contract_data,
    }
    job = {
        "apiVersion": "batch/v1",
        "kind": "Job",
        "metadata": {"name": names["job"], "namespace": NAMESPACE, "labels": common_labels},
        "spec": {
            "backoffLimit": 0,
            "activeDeadlineSeconds": 300,
            "template": {
                "metadata": {
                    "labels": common_labels,
                    "annotations": {"traffic.analysis/candidate-sha256": candidate},
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
                    "containers": [{
                        "name": "governed-training",
                        "image": image,
                        "imagePullPolicy": "Never",
                        "command": ["python", "/app/scripts/m08_governed_canary.py"],
                        "env": [
                            {"name": "M08_CANARY_RUN_ID", "value": run_id},
                            {"name": "M08_CANARY_CANDIDATE_SHA256", "value": candidate},
                            {"name": "M08_CANARY_CONTRACT_DIR", "value": "/contracts"},
                        ],
                        "resources": {
                            "requests": {"cpu": "100m", "memory": "256Mi"},
                            "limits": {"cpu": "1", "memory": "1Gi"},
                        },
                        "securityContext": {
                            "allowPrivilegeEscalation": False,
                            "readOnlyRootFilesystem": True,
                            "capabilities": {"drop": ["ALL"]},
                        },
                        "volumeMounts": [
                            {"name": "contracts", "mountPath": "/contracts", "readOnly": True},
                            {"name": "tmp", "mountPath": "/tmp"},
                        ],
                    }],
                    "volumes": [
                        {"name": "contracts", "configMap": {"name": names["contracts"]}},
                        {"name": "tmp", "emptyDir": {"sizeLimit": "1Gi"}},
                    ],
                },
            },
        },
    }
    return [config_map, job]


def apply(objects: list[dict[str, Any]]) -> None:
    body = "\n---\n".join(yaml.safe_dump(item, sort_keys=False) for item in objects)
    kubectl("apply", "-f", "-", input_text=body)


def wait_job(name: str, timeout: int) -> str:
    result = kubectl(
        "wait", "--for=condition=complete", f"job/{name}", "-n", NAMESPACE,
        f"--timeout={timeout}s", check=False,
    )
    logs = kubectl("logs", f"job/{name}", "-n", NAMESPACE, check=False).stdout
    if result.returncode != 0:
        raise CanaryError(f"Kubernetes M08 canary failed or timed out: {logs.strip()}")
    return logs


def parse_result(logs: str, run_id: str, candidate: str) -> dict[str, Any]:
    objects: list[dict[str, Any]] = []
    for line in logs.splitlines():
        if not line.startswith("{"):
            continue
        try:
            value = json.loads(line)
        except json.JSONDecodeError:
            continue
        if isinstance(value, dict) and "status" in value:
            objects.append(value)
    if len(objects) != 1:
        raise CanaryError(f"expected exactly one canary result object, got {len(objects)}")
    result = objects[0]
    expected = {
        "status": "PASS",
        "infrastructure": "kubernetes",
        "production_applied": False,
        "run_id": run_id,
        "candidate_sha256": candidate,
        "artifact_count": 5,
        "gnn_artifact_count": 7,
        "evaluation_artifact_count": 2,
        "explanation_artifact_count": 3,
        "model_export_artifact_count": 5,
        "model_registration_status": "registered",
        "model_registration_revision": 1,
        "model_registration_activation_event_created": False,
        "model_registration_storage_mode": "synthetic_canary_receipt",
        "temporary_storage_removed_on_exit": True,
    }
    for field, wanted in expected.items():
        if result.get(field) != wanted:
            raise CanaryError(f"canary result {field} drifted: {result.get(field)!r} != {wanted!r}")
    row_counts = {
        name: value.get("row_count")
        for name, value in result.get("split_counts", {}).items()
        if isinstance(value, dict)
    }
    if row_counts != {"train": 40, "validation": 16, "test": 16, "open_set": 8}:
        raise CanaryError(f"canary split counts drifted: {row_counts}")
    for field in (
        "dataset_sha256", "training_run_sha256", "gnn_training_run_sha256",
        "graph_snapshot_sha256", "evaluation_sha256", "explanation_sha256",
        "model_package_sha256",
        "model_registration_receipt_sha256", "model_registration_request_sha256",
    ):
        if not re.fullmatch(r"[0-9a-f]{64}", str(result.get(field, ""))):
            raise CanaryError(f"canary result has invalid {field}")
    try:
        uuid.UUID(str(result.get("dataset_id", "")))
    except ValueError as exc:
        raise CanaryError("canary result has invalid dataset_id") from exc
    try:
        uuid.UUID(str(result.get("evaluation_id", "")))
    except ValueError as exc:
        raise CanaryError("canary result has invalid evaluation_id") from exc
    try:
        uuid.UUID(str(result.get("explanation_id", "")))
    except ValueError as exc:
        raise CanaryError("canary result has invalid explanation_id") from exc
    if result.get("activation_authorized") is not False:
        raise CanaryError("canary evaluation must not authorize activation")
    if result.get("explanation_activation_authorized") is not False:
        raise CanaryError("canary explanation must not authorize activation")
    if result.get("model_export_activation_authorized") is not False:
        raise CanaryError("canary model export must not authorize activation")
    try:
        uuid.UUID(str(result.get("model_package_id", "")))
    except ValueError as exc:
        raise CanaryError("canary result has invalid model_package_id") from exc
    try:
        uuid.UUID(str(result.get("model_registration_receipt_id", "")))
    except ValueError as exc:
        raise CanaryError("canary result has invalid model_registration_receipt_id") from exc
    if result.get("model_signing_key_id") != f"ephemeral-canary/m08/{run_id}":
        raise CanaryError("canary result lost run-scoped signing key identity")
    if result.get("graph_ablation_state") != "EVALUATED":
        raise CanaryError("GNN canary must evaluate the exact four-variant ablation population")
    try:
        uuid.UUID(str(result.get("graph_snapshot_id", "")))
    except ValueError as exc:
        raise CanaryError("canary result has invalid graph_snapshot_id") from exc
    return result


def verify_pod_binding(job_name: str, candidate: str, node: str) -> None:
    result = kubectl("get", "pod", "-n", NAMESPACE, "-l", f"job-name={job_name}", "-o", "json")
    pods = json.loads(result.stdout).get("items", [])
    if len(pods) != 1:
        raise CanaryError(f"expected one canary pod, got {len(pods)}")
    pod = pods[0]
    actual_candidate = pod.get("metadata", {}).get("annotations", {}).get(
        "traffic.analysis/candidate-sha256"
    )
    actual_node = pod.get("spec", {}).get("nodeName")
    if actual_candidate != candidate or actual_node != node:
        raise CanaryError("Kubernetes pod lost candidate or node binding")


def cleanup(names: dict[str, str], run_id: str) -> None:
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
    raise CanaryError(
        f"run-scoped cleanup incomplete: {names['job']}, {names['contracts']}"
    )


def main() -> None:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--image", required=True)
    parser.add_argument("--candidate-sha256", required=True)
    parser.add_argument("--run-id", default=str(uuid.uuid4()))
    parser.add_argument("--node", default="8-2tb")
    parser.add_argument("--timeout", type=int, default=300)
    parser.add_argument("--keep", action="store_true")
    args = parser.parse_args()
    if args.timeout < 30 or args.timeout > 900:
        raise CanaryError("--timeout must be between 30 and 900 seconds")

    suffix = validate_inputs(args.image, args.candidate_sha256, args.run_id, args.node)
    names = resource_names(suffix)
    objects = build_objects(names, args.image, args.candidate_sha256, args.run_id, args.node)
    applied = False
    result: dict[str, Any] | None = None
    try:
        apply(objects)
        applied = True
        logs = wait_job(names["job"], args.timeout)
        verify_pod_binding(names["job"], args.candidate_sha256, args.node)
        result = parse_result(logs, args.run_id, args.candidate_sha256)
    finally:
        if applied and not args.keep:
            cleanup(names, args.run_id)

    if result is None:
        raise CanaryError("M08 canary did not produce a result")
    print(json.dumps({
        **result,
        "image": args.image,
        "node": args.node,
        "namespace": NAMESPACE,
        "contracts": {name: sha256(ROOT / "contracts/mlops" / name) for name in CONTRACT_FILES},
        "run_scoped_resources_removed": not args.keep,
        "shared_clickhouse_touched": False,
        "shared_minio_touched": False,
        "argo_workflow_submitted": False,
        "production_workflow_patched": False,
    }, sort_keys=True))


if __name__ == "__main__":
    main()
