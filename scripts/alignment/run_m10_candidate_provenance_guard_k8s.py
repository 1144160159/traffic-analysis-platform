#!/usr/bin/env python3
"""Run T1-M10-N002 candidate provenance guard in a read-only Kubernetes Job."""

from __future__ import annotations

import argparse
import hashlib
import json
import re
import sys
import time
import uuid
from pathlib import Path
from typing import Any

import yaml

SCRIPT_DIR = Path(__file__).resolve().parent
if str(SCRIPT_DIR) not in sys.path:
    sys.path.insert(0, str(SCRIPT_DIR))

import run_m09_journey_evidence_k8s as base
import candidate_snapshot


ROOT = Path(__file__).resolve().parents[2]
NAMESPACE = "traffic-analysis"
DEFAULT_OUTPUT = ROOT / "doc/02_acceptance/topic1/tasks/t1-m10-n002/k8s-candidate-provenance-latest.json"
IMAGE_RE = re.compile(r"^[a-zA-Z0-9][a-zA-Z0-9._/@:-]+$")
NODE_RE = re.compile(r"^[a-z0-9]([-a-z0-9.]*[a-z0-9])?$")
MOUNT_FILES = (
    *candidate_snapshot.ACTIVE_MANIFESTS,
    *candidate_snapshot.BUILD_SELECTORS,
    *(Path(recipe) for recipes in candidate_snapshot.PREBUILT_RECIPES.values() for recipe in recipes),
)
SOURCE_FILES = (
    Path("scripts/alignment/candidate_snapshot.py"),
    Path("scripts/alignment/capture_g0.py"),
    Path("scripts/alignment/verify_m10_candidate_provenance_k8s.py"),
    Path("scripts/alignment/run_m10_candidate_provenance_guard_k8s.py"),
    Path("scripts/alignment/Dockerfile.m10-candidate-provenance"),
    *MOUNT_FILES,
)


def sha256(path: Path) -> str:
    return hashlib.sha256(path.read_bytes()).hexdigest()


def validate(image: str, run_id: str, node: str, timeout: int) -> str:
    if not IMAGE_RE.fullmatch(image) or image.endswith(":latest"):
        raise base.CanaryError("image must be an explicit non-latest reference")
    if not NODE_RE.fullmatch(node):
        raise base.CanaryError("invalid Kubernetes node name")
    if timeout < 30 or timeout > 600:
        raise base.CanaryError("timeout must be between 30 and 600 seconds")
    parsed = uuid.UUID(run_id)
    if str(parsed) != run_id:
        raise base.CanaryError("run-id must be a canonical lowercase UUID")
    return parsed.hex[:10]


def local_expected() -> dict[str, Any]:
    result = candidate_snapshot.scan_candidate_artifact_provenance(ROOT)
    return {
        "status": result["status"],
        "blocking_codes": result["blocking_codes"],
        "active_image_count": len(result["active_first_party_images"]),
        "excluded_prebuilt_count": len(result["excluded_prebuilt_artifacts"]),
    }


def objects(name: str, image: str, run_id: str, node: str) -> list[dict[str, Any]]:
    labels = {
        "app.kubernetes.io/name": "m10-candidate-provenance-guard",
        "traffic.analysis/canary-run": run_id,
    }
    data = {"expected.json": json.dumps(local_expected(), sort_keys=True)}
    items = [{"key": "expected.json", "path": "expected.json"}]
    for index, relative in enumerate(MOUNT_FILES):
        path = ROOT / relative
        if not path.is_file():
            continue
        key = f"source-{index:03d}"
        data[key] = path.read_text(encoding="utf-8")
        items.append({"key": key, "path": str(relative)})
    configmap = {
        "apiVersion": "v1", "kind": "ConfigMap",
        "metadata": {"name": name, "namespace": NAMESPACE, "labels": labels},
        "data": data,
    }
    job = {
        "apiVersion": "batch/v1", "kind": "Job",
        "metadata": {
            "name": name, "namespace": NAMESPACE, "labels": labels,
            "annotations": {
                "traffic.analysis/shared-infrastructure-touched": "false",
                "traffic.analysis/production-applied": "false",
                "traffic.analysis/expected-guard-status": "BLOCKED",
            },
        },
        "spec": {"backoffLimit": 0, "ttlSecondsAfterFinished": 300, "template": {
            "metadata": {"labels": labels},
            "spec": {
                "nodeName": node, "automountServiceAccountToken": False,
                "restartPolicy": "Never",
                "securityContext": {
                    "runAsNonRoot": True, "runAsUser": 65532, "runAsGroup": 65532,
                    "fsGroup": 65532, "seccompProfile": {"type": "RuntimeDefault"},
                },
                "containers": [{
                    "name": "guard", "image": image, "imagePullPolicy": "Never",
                    "args": ["--root", "/workspace", "--expected", "/workspace/expected.json"],
                    "resources": {
                        "requests": {"cpu": "25m", "memory": "32Mi"},
                        "limits": {"cpu": "250m", "memory": "128Mi"},
                    },
                    "securityContext": {
                        "allowPrivilegeEscalation": False, "readOnlyRootFilesystem": True,
                        "capabilities": {"drop": ["ALL"]},
                    },
                    "volumeMounts": [
                        {"name": "workspace", "mountPath": "/workspace", "readOnly": True},
                        {"name": "tmp", "mountPath": "/tmp"},
                    ],
                }],
                "volumes": [
                    {"name": "workspace", "configMap": {"name": name, "items": items}},
                    {"name": "tmp", "emptyDir": {}},
                ],
            },
        }},
    }
    return [configmap, job]


def wait_job(name: str, timeout: int) -> tuple[dict[str, Any], dict[str, Any]]:
    deadline = time.time() + timeout
    job: dict[str, Any] = {}
    while time.time() < deadline:
        response = base.kubectl("get", "job", name, "-n", NAMESPACE, "-o", "json", check=False)
        if response.returncode == 0:
            job = json.loads(response.stdout)
            if job.get("status", {}).get("succeeded") == 1:
                break
            if job.get("status", {}).get("failed", 0):
                logs = base.kubectl("logs", "-n", NAMESPACE, f"job/{name}", check=False)
                raise base.CanaryError(f"provenance Job failed:\n{logs.stdout}{logs.stderr}")
        time.sleep(1)
    else:
        raise base.CanaryError(f"timed out waiting for Job {name}")
    pods = json.loads(base.kubectl(
        "get", "pod", "-n", NAMESPACE, "-l", f"job-name={name}", "-o", "json"
    ).stdout)["items"]
    if len(pods) != 1:
        raise base.CanaryError("expected exactly one provenance pod")
    pod = pods[0]
    logs = base.kubectl("logs", "-n", NAMESPACE, pod["metadata"]["name"]).stdout.strip()
    result = json.loads(logs.splitlines()[-1])
    if result.get("status") != "PASS" or result.get("guard_status") != "BLOCKED":
        raise base.CanaryError(f"unexpected guard receipt: {result}")
    container = pod["status"]["containerStatuses"][0]
    identity = {
        "namespace": NAMESPACE, "job_name": name, "job_uid": job["metadata"]["uid"],
        "pod_name": pod["metadata"]["name"], "pod_uid": pod["metadata"]["uid"],
        "node": pod["spec"]["nodeName"], "image": container.get("image"),
        "image_id": container.get("imageID"), "container_id": container.get("containerID"),
        "started_at": pod.get("status", {}).get("startTime"),
        "completed_at": job.get("status", {}).get("completionTime"),
    }
    return result, identity


def main() -> None:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--image", required=True)
    parser.add_argument("--run-id", default=str(uuid.uuid4()))
    parser.add_argument("--node", default="8-2tb")
    parser.add_argument("--timeout", type=int, default=180)
    parser.add_argument("--output", type=Path, default=DEFAULT_OUTPUT)
    parser.add_argument("--keep", action="store_true")
    args = parser.parse_args()
    suffix = validate(args.image, args.run_id, args.node, args.timeout)
    name = f"m10-n002-provenance-{suffix}"
    try:
        base.apply(objects(name, args.image, args.run_id, args.node))
        result, identity = wait_job(name, args.timeout)
    finally:
        if not args.keep:
            base.cleanup(args.run_id)
    binary_inventory = {
        path: sha256(ROOT / path)
        for path in candidate_snapshot.PREBUILT_RECIPES
        if (ROOT / path).is_file()
    }
    evidence = {
        "artifact_kind": "M10_CANDIDATE_PROVENANCE_GUARD_TEST_RESULT",
        "task_id": "T1-M10-N002", "run_id": args.run_id, "status": "PASS",
        "engineering_status": "PASS",
        "acceptance_status": "BLOCKED_NO_DEPLOYABLE_CANDIDATE",
        "profile_id": "M10-N002-K8S-READONLY-PROVENANCE-FAIL-CLOSED-V1",
        "candidate_id": None, "environment_kind": "KUBERNETES",
        "source_sha256": {
            str(path): sha256(ROOT / path) for path in SOURCE_FILES if (ROOT / path).is_file()
        },
        "excluded_binary_sha256": binary_inventory,
        "kubernetes_job": identity, "validation": result,
        "required_gates": {"G0": "BLOCKED", "G1": "NOT_EXECUTED", "G6": "NOT_EXECUTED"},
        "shared_infrastructure_touched": False, "production_applied": False,
        "run_scoped_resources_removed": not args.keep,
        "allowed_claims": ["The provenance guard reproduced the expected fail-closed result in a read-only Kubernetes Job"],
        "does_not_prove": [
            "candidate provenance is complete", "G0, G1, or G6 passed",
            "a deployable candidate exists", "production promotion is authorized",
        ],
    }
    args.output.parent.mkdir(parents=True, exist_ok=True)
    temporary = args.output.with_name(f".{args.output.name}.tmp")
    temporary.write_text(json.dumps(evidence, indent=2, sort_keys=True) + "\n", encoding="utf-8")
    temporary.replace(args.output)
    print(json.dumps(evidence, sort_keys=True))


if __name__ == "__main__":
    main()
