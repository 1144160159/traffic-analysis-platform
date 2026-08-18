#!/usr/bin/env python3
"""Run T1-M10-N007 fail-closed authorization tests on Kubernetes nodes."""

from __future__ import annotations

import argparse
import hashlib
import json
import re
import sys
import uuid
from pathlib import Path
from typing import Any


SCRIPT_DIR = Path(__file__).resolve().parent
if str(SCRIPT_DIR) not in sys.path:
    sys.path.insert(0, str(SCRIPT_DIR))

import run_m09_journey_evidence_k8s as base


ROOT = Path(__file__).resolve().parents[2]
NAMESPACE = "traffic-analysis"
OUTPUT = ROOT / "doc/02_acceptance/topic1/tasks/t1-m10-n007/k8s-authz-negative-tests-latest.json"
POLICY = Path("contracts/authz/m10-minimal-role-policy.v1.json")
N006 = Path("doc/02_acceptance/topic1/tasks/t1-m10-n006/k8s-apisix-route-diff-latest.json")
SOURCE_FILES = (
    POLICY,
    Path("contracts/alignment/m10-authz-policy.schema.json"),
    Path("contracts/alignment/m10-authz-k8s-result.schema.json"),
    Path("scripts/alignment/build_m10_authz_policy.py"),
    Path("scripts/alignment/verify_m10_authz_policy.py"),
    Path("scripts/alignment/run_m10_authz_test_binaries.sh"),
    Path("scripts/alignment/Dockerfile.m10-authz-tests"),
    Path("scripts/alignment/Dockerfile.m10-authz-prebuilt"),
    Path("scripts/alignment/run_m10_authz_k8s.py"),
    Path("scripts/alignment/verify_m10_authz_k8s.py"),
    Path("go/control-plane/internal/common/httpx/authorization.go"),
    Path("go/control-plane/internal/common/httpx/authorization_test.go"),
    Path("go/control-plane/internal/common/httpx/auth.go"),
    Path("go/control-plane/internal/common/httpx/tenant.go"),
    Path("go/control-plane/internal/auth/model/user.go"),
    Path("go/control-plane/internal/auth/model/role_policy_contract_test.go"),
    Path("go/control-plane/internal/auth/service/auth_service.go"),
    Path("go/control-plane/internal/auth/service/auth_service_role_policy_test.go"),
    Path("go/control-plane/internal/asset/api/auth.go"),
    Path("go/control-plane/internal/asset/api/auth_test.go"),
    Path("go/control-plane/cmd/graph-service/auth_middleware_test.go"),
    Path("tests/alignment/test_m10_authz_policy.py"),
    Path("tests/alignment/test_m10_authz_k8s.py"),
    N006,
)
IMAGE_RE = re.compile(r"^[a-zA-Z0-9][a-zA-Z0-9._/@:-]+$")
NODE_RE = re.compile(r"^[a-z0-9]([-a-z0-9.]*[a-z0-9])?$")
EXPECTED_DIMENSIONS = [
    "missing_token", "expired_token", "scope_escalation", "cross_tenant",
    "guessed_object_id", "field_escalation",
]


def sha256(path: Path) -> str:
    return hashlib.sha256(path.read_bytes()).hexdigest()


def validate_inputs(image: str, run_id: str, nodes: list[str], timeout: int) -> str:
    if not IMAGE_RE.fullmatch(image) or image.endswith(":latest"):
        raise base.CanaryError("image must be an explicit non-latest reference")
    parsed = uuid.UUID(run_id)
    if str(parsed) != run_id:
        raise base.CanaryError("run-id must be a canonical lowercase UUID")
    if len(nodes) != 2 or len(nodes) != len(set(nodes)) or any(not NODE_RE.fullmatch(node) for node in nodes):
        raise base.CanaryError("exactly two distinct valid Kubernetes nodes are required")
    if timeout < 30 or timeout > 600:
        raise base.CanaryError("timeout must be between 30 and 600 seconds")
    return parsed.hex[:10]


def job_object(name: str, image: str, run_id: str, node: str) -> dict[str, Any]:
    labels = {
        "app.kubernetes.io/name": "m10-authz-negative-tests",
        "traffic.analysis/canary-run": run_id,
    }
    return {
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
                    "fsGroup": 65532, "seccompProfile": {"type": "RuntimeDefault"},
                },
                "containers": [{
                    "name": "authz-tests", "image": image, "imagePullPolicy": "Never",
                    "resources": {
                        "requests": {"cpu": "50m", "memory": "64Mi"},
                        "limits": {"cpu": "1", "memory": "512Mi"},
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
    }


def validate_result(result: dict[str, Any]) -> None:
    if result != {
        "status": "PASS",
        "profile_id": "M10-N007-K8S-FAIL-CLOSED-AUTHZ-V1",
        "test_groups": 5,
        "top_level_tests": 16,
        "fail_closed_dimensions": EXPECTED_DIMENSIONS,
    }:
        raise base.CanaryError(f"unexpected authz test receipt: {result}")


def main() -> None:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--image", required=True)
    parser.add_argument("--run-id", default=str(uuid.uuid4()))
    parser.add_argument("--nodes", default="8-2tb,zeus-server")
    parser.add_argument("--timeout", type=int, default=180)
    parser.add_argument("--output", type=Path, default=OUTPUT)
    parser.add_argument("--keep", action="store_true")
    args = parser.parse_args()
    nodes = [item.strip() for item in args.nodes.split(",") if item.strip()]
    suffix = validate_inputs(args.image, args.run_id, nodes, args.timeout)
    names = [f"m10-n007-authz-{suffix}-{index}" for index in range(1, len(nodes) + 1)]
    try:
        base.apply([job_object(name, args.image, args.run_id, node) for name, node in zip(names, nodes)])
        jobs: list[dict[str, Any]] = []
        for name in names:
            result, identity = base.wait_and_collect(name, args.timeout)
            validate_result(result)
            jobs.append({"identity": identity, "result": result})
    finally:
        if not args.keep:
            base.cleanup(args.run_id)

    policy = json.loads((ROOT / POLICY).read_text(encoding="utf-8"))
    n006 = json.loads((ROOT / N006).read_text(encoding="utf-8"))
    blockers = ["AUTHZ_POLICY_RUNTIME_ENABLEMENT_REQUIRED", "POST_ENABLE_LIVE_NEGATIVE_TESTS_REQUIRED"]
    if n006.get("acceptance_status") != "PASS":
        blockers.append("N006_ROUTE_MATERIALIZATION_REQUIRED")
    if n006.get("candidate_id") is None:
        blockers.append("DEPLOYABLE_CANDIDATE_REQUIRED")
    evidence = {
        "schema_version": 1,
        "artifact_kind": "M10_AUTHZ_K8S_NEGATIVE_TEST_RESULT",
        "task_id": "T1-M10-N007",
        "atomic_pr_ids": ["T1-M10-P015-OPS-n007-s1", "T1-M10-P016-TST-POST-n007-s2"],
        "run_id": args.run_id,
        "status": "PASS", "engineering_status": "PASS",
        "acceptance_status": "BLOCKED_NOT_ENABLED",
        "profile_id": "M10-N007-K8S-FAIL-CLOSED-AUTHZ-V1",
        "environment_kind": "KUBERNETES",
        "candidate_id": n006.get("candidate_id"),
        "policy_sha256": policy["policy_sha256"],
        "source_sha256": {str(path): sha256(ROOT / path) for path in SOURCE_FILES},
        "policy_counts": policy["counts"],
        "build_toolchain": "go version go1.25.12 linux/amd64",
        "fail_closed_dimensions": EXPECTED_DIMENSIONS,
        "kubernetes_jobs": jobs,
        "blocking_codes": sorted(set(blockers)),
        "required_gates": {"G0": "BLOCKED_BY_N002", "G2": "PASS_UNIT_AND_K8S_SELF_TEST", "G3": "PASS_CROSS_NODE_SELF_TEST", "G6": "BLOCKED_NOT_ENABLED"},
        "shared_infrastructure_touched": False,
        "production_applied": False,
        "run_scoped_resources_removed": not args.keep,
        "allowed_claims": ["The bound N007 authorization candidate passed the declared fail-closed Go tests on both Kubernetes nodes"],
        "does_not_prove": [
            "the N006 gateway candidate was applied", "live Keycloak tokens were exercised",
            "live service endpoints rejected every negative case", "G6 rollback passed",
            "production promotion is authorized",
        ],
    }
    output = args.output.resolve(strict=False)
    if output not in {OUTPUT.resolve(strict=False), Path("/tmp/m10-authz-k8s.json")}:
        raise base.CanaryError("output must be the N007 evidence path or documented /tmp path")
    output.parent.mkdir(parents=True, exist_ok=True)
    temporary = output.with_name(f".{output.name}.tmp")
    temporary.write_text(json.dumps(evidence, indent=2, sort_keys=True) + "\n", encoding="utf-8")
    temporary.replace(output)
    print(json.dumps(evidence, sort_keys=True))


if __name__ == "__main__":
    main()
