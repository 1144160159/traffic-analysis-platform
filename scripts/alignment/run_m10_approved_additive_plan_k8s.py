#!/usr/bin/env python3
"""Verify the blocked N005 additive-apply guard in a read-only Kubernetes Job."""

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


ROOT = Path(__file__).resolve().parents[2]
SCRIPT_DIR = Path(__file__).resolve().parent
if str(SCRIPT_DIR) not in sys.path:
    sys.path.insert(0, str(SCRIPT_DIR))

import run_m09_journey_evidence_k8s as base


NAMESPACE = "traffic-analysis"
DEFAULT_OUTPUT = ROOT / "doc/02_acceptance/topic1/tasks/t1-m10-n005/k8s-approved-additive-plan-latest.json"
PLAN = Path("deployments/releases/topic1/m10-approved-additive-plan.v1.json")
SOURCE_FILES = (
    PLAN,
    Path("contracts/alignment/m10-approved-additive-plan.schema.json"),
    Path("scripts/alignment/build_m10_approved_additive_plan.py"),
    Path("scripts/alignment/verify_m10_approved_additive_plan.py"),
    Path("scripts/alignment/guard_m10_approved_additive_apply.py"),
    Path("scripts/alignment/run_m10_approved_additive_plan_k8s.py"),
    Path("scripts/alignment/Dockerfile.m10-approved-additive-plan"),
)
IMAGE_RE = re.compile(r"^[a-zA-Z0-9][a-zA-Z0-9._/@:-]+$")
NODE_RE = re.compile(r"^[a-z0-9]([-a-z0-9.]*[a-z0-9])?$")


def sha256(path: Path) -> str:
    return hashlib.sha256(path.read_bytes()).hexdigest()


def validate_inputs(image: str, run_id: str, node: str, timeout: int) -> str:
    if not IMAGE_RE.fullmatch(image) or image.endswith(":latest"):
        raise base.CanaryError("image must be an explicit non-latest reference")
    if not NODE_RE.fullmatch(node):
        raise base.CanaryError("invalid Kubernetes node")
    if timeout < 30 or timeout > 600:
        raise base.CanaryError("timeout must be between 30 and 600 seconds")
    parsed = uuid.UUID(run_id)
    if str(parsed) != run_id:
        raise base.CanaryError("run-id must be a canonical lowercase UUID")
    return parsed.hex[:10]


def objects(name: str, image: str, run_id: str, node: str) -> list[dict[str, Any]]:
    labels = {
        "app.kubernetes.io/name": "m10-approved-additive-plan",
        "traffic.analysis/canary-run": run_id,
    }
    return [{
        "apiVersion": "batch/v1", "kind": "Job",
        "metadata": {
            "name": name, "namespace": NAMESPACE, "labels": labels,
            "annotations": {
                "traffic.analysis/shared-infrastructure-touched": "false",
                "traffic.analysis/production-applied": "false",
            },
        },
        "spec": {"backoffLimit": 0, "ttlSecondsAfterFinished": 300, "template": {
            "metadata": {"labels": labels},
            "spec": {
                "nodeName": node, "automountServiceAccountToken": False,
                "restartPolicy": "Never",
                "securityContext": {
                    "runAsNonRoot": True, "runAsUser": 65532, "runAsGroup": 65532,
                    "seccompProfile": {"type": "RuntimeDefault"},
                },
                "containers": [{
                    "name": "guard", "image": image, "imagePullPolicy": "Never",
                    "args": ["--root", "/workspace", "--expect-blocked", "--json"],
                    "resources": {
                        "requests": {"cpu": "25m", "memory": "32Mi"},
                        "limits": {"cpu": "250m", "memory": "128Mi"},
                    },
                    "securityContext": {
                        "allowPrivilegeEscalation": False, "readOnlyRootFilesystem": True,
                        "capabilities": {"drop": ["ALL"]},
                    },
                    "volumeMounts": [{"name": "tmp", "mountPath": "/tmp"}],
                }],
                "volumes": [{"name": "tmp", "emptyDir": {}}],
            },
        }},
    }]


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
                raise base.CanaryError(f"N005 guard Job failed:\n{logs.stdout}{logs.stderr}")
        time.sleep(1)
    else:
        raise base.CanaryError(f"timed out waiting for Job {name}")
    pods = json.loads(base.kubectl("get", "pod", "-n", NAMESPACE, "-l", f"job-name={name}", "-o", "json").stdout)["items"]
    if len(pods) != 1:
        raise base.CanaryError("expected exactly one N005 guard pod")
    pod = pods[0]
    result = json.loads(base.kubectl("logs", "-n", NAMESPACE, pod["metadata"]["name"]).stdout.strip().splitlines()[-1])
    if result.get("decision") != "BLOCKED" or result.get("mutating_client_started") is not False:
        raise base.CanaryError(f"N005 guard did not fail closed: {result}")
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
    suffix = validate_inputs(args.image, args.run_id, args.node, args.timeout)
    name = f"m10-n005-additive-{suffix}"
    try:
        base.apply(objects(name, args.image, args.run_id, args.node))
        decision, identity = wait_job(name, args.timeout)
    finally:
        if not args.keep:
            base.cleanup(args.run_id)
    plan = json.loads((ROOT / PLAN).read_text(encoding="utf-8"))
    evidence = {
        "artifact_kind": "M10_APPROVED_ADDITIVE_PLAN_K8S_TEST_RESULT",
        "task_id": "T1-M10-N005", "run_id": args.run_id,
        "status": "PASS", "engineering_status": "PASS",
        "acceptance_status": "BLOCKED_UNAPPROVED",
        "profile_id": "M10-N005-K8S-READONLY-FAIL-CLOSED-V1",
        "candidate_id": plan.get("candidate_id"), "environment_kind": "KUBERNETES",
        "source_sha256": {str(path): sha256(ROOT / path) for path in SOURCE_FILES},
        "kubernetes_job": identity, "authorization_decision": decision,
        "plan_status": plan["status"], "blocking_codes": plan["blocking_codes"],
        "required_gates": {"G0": "BLOCKED_BY_N002", "G1": "NOT_EXECUTED", "G6": "BLOCKED"},
        "shared_infrastructure_touched": False, "production_applied": False,
        "run_scoped_resources_removed": not args.keep,
        "allowed_claims": ["The N005 apply guard failed closed in a non-root read-only Kubernetes Job before starting a mutating client"],
        "does_not_prove": [
            "any artifact is approved", "G1 or G6 passed", "replay was executed against a database or Kafka",
            "half-failure recovery was exercised against shared infrastructure", "production application is authorized",
        ],
    }
    args.output.parent.mkdir(parents=True, exist_ok=True)
    temporary = args.output.with_name(f".{args.output.name}.tmp")
    temporary.write_text(json.dumps(evidence, indent=2, sort_keys=True) + "\n", encoding="utf-8")
    temporary.replace(args.output)
    print(json.dumps(evidence, sort_keys=True))


if __name__ == "__main__":
    main()
