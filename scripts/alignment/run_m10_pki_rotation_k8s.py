#!/usr/bin/env python3
"""Run T1-M10-N008 atomic PKI rotation tests on two Kubernetes nodes."""

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
OUTPUT = ROOT / "doc/02_acceptance/topic1/tasks/t1-m10-n008/k8s-pki-rotation-negative-tests-latest.json"
PKI_CATALOG = Path("contracts/security/pki-catalog.v1.json")
N007 = Path("doc/02_acceptance/topic1/tasks/t1-m10-n007/k8s-authz-negative-tests-latest.json")
SOURCE_FILES = (
    PKI_CATALOG,
    Path("contracts/alignment/m10-pki-rotation-k8s-result.schema.json"),
    Path("scripts/alignment/build_pki_catalog.py"),
    Path("scripts/alignment/verify_pki_catalog.py"),
    Path("scripts/alignment/run_m10_pki_test_binary.sh"),
    Path("scripts/alignment/Dockerfile.m10-pki-prebuilt"),
    Path("scripts/alignment/run_m10_pki_rotation_k8s.py"),
    Path("scripts/alignment/verify_m10_pki_rotation_k8s.py"),
    Path("rust/probe-agent/scripts/generate-mtls-certs.sh"),
    Path("rust/probe-agent/probe-agent/src/sender/grpc.rs"),
    Path("deployments/kubernetes/deploy.sh"),
    Path("deployments/kubernetes/applications/go-services.yaml"),
    Path("deployments/kubernetes/applications/probe-agent.yaml"),
    Path("go/control-plane/internal/common/pki/reloader.go"),
    Path("go/control-plane/internal/common/pki/reloader_test.go"),
    Path("go/control-plane/internal/ingest/config/config.go"),
    Path("go/control-plane/internal/ingest/config/config_test.go"),
    Path("go/control-plane/cmd/ingest-gateway/main.go"),
    Path("tests/alignment/test_m10_pki_rotation_k8s.py"),
    Path("doc/07_alignment/runbooks/T1-M10-N008-pki-rotation.md"),
    N007,
)
IMAGE_RE = re.compile(r"^[a-zA-Z0-9][a-zA-Z0-9._/@:-]+$")
NODE_RE = re.compile(r"^[a-z0-9]([-a-z0-9.]*[a-z0-9])?$")
EXPECTED_DIMENSIONS = [
    "wrong_ca",
    "expired_certificate",
    "san_mismatch",
    "revoked_serial",
    "interrupted_rotation",
    "dual_trust_cutover",
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
        "app.kubernetes.io/name": "m10-pki-rotation-tests",
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
                "traffic.analysis/shared-infrastructure-touched": "false",
                "traffic.analysis/production-applied": "false",
            },
        },
        "spec": {
            "backoffLimit": 0,
            "ttlSecondsAfterFinished": 300,
            "template": {
                "metadata": {"labels": labels},
                "spec": {
                    "nodeName": node,
                    "automountServiceAccountToken": False,
                    "restartPolicy": "Never",
                    "securityContext": {
                        "runAsNonRoot": True,
                        "runAsUser": 65532,
                        "runAsGroup": 65532,
                        "fsGroup": 65532,
                        "seccompProfile": {"type": "RuntimeDefault"},
                    },
                    "containers": [
                        {
                            "name": "pki-tests",
                            "image": image,
                            "imagePullPolicy": "Never",
                            "resources": {
                                "requests": {"cpu": "25m", "memory": "32Mi"},
                                "limits": {"cpu": "500m", "memory": "128Mi"},
                            },
                            "securityContext": {
                                "allowPrivilegeEscalation": False,
                                "readOnlyRootFilesystem": True,
                                "capabilities": {"drop": ["ALL"]},
                            },
                            "volumeMounts": [{"name": "tmp", "mountPath": "/tmp"}],
                        }
                    ],
                    "volumes": [{"name": "tmp", "emptyDir": {}}],
                },
            },
        },
    }


def validate_result(result: dict[str, Any]) -> None:
    if result != {
        "status": "PASS",
        "profile_id": "M10-N008-K8S-ATOMIC-PKI-ROTATION-V1",
        "test_groups": 1,
        "top_level_tests": 4,
        "fail_closed_dimensions": EXPECTED_DIMENSIONS,
    }:
        raise base.CanaryError(f"unexpected PKI test receipt: {result}")


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
    names = [f"m10-n008-pki-{suffix}-{index}" for index in range(1, len(nodes) + 1)]
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

    catalog = json.loads((ROOT / PKI_CATALOG).read_text(encoding="utf-8"))
    n007 = json.loads((ROOT / N007).read_text(encoding="utf-8"))
    blockers = [
        "APPROVED_ISSUER_AND_OFFLINE_ROOT_CUSTODY_REQUIRED",
        "LIVE_REVOCATION_AND_ROLLBACK_REQUIRED",
        "PER_PROBE_UNIQUE_CERTIFICATE_REQUIRED",
        "TLS_ROTATION_RUNTIME_ENABLEMENT_REQUIRED",
        "TWO_CONSECUTIVE_LIVE_ROTATIONS_REQUIRED",
    ]
    if n007.get("acceptance_status") != "PASS":
        blockers.append("N007_AUTHZ_ENABLEMENT_REQUIRED")
    if n007.get("candidate_id") is None:
        blockers.append("DEPLOYABLE_CANDIDATE_REQUIRED")
    evidence = {
        "schema_version": 1,
        "artifact_kind": "M10_PKI_ROTATION_K8S_NEGATIVE_TEST_RESULT",
        "task_id": "T1-M10-N008",
        "atomic_pr_ids": ["T1-M10-P018-OPS-n008-s1", "T1-M10-P019-TST-POST-n008-s2"],
        "run_id": args.run_id,
        "status": "PASS",
        "engineering_status": "PASS",
        "acceptance_status": "BLOCKED_NOT_ENABLED",
        "profile_id": "M10-N008-K8S-ATOMIC-PKI-ROTATION-V1",
        "environment_kind": "KUBERNETES",
        "candidate_id": n007.get("candidate_id"),
        "pki_catalog_sha256": catalog["catalog_sha256"],
        "source_sha256": {str(path): sha256(ROOT / path) for path in SOURCE_FILES},
        "catalog_counts": catalog["counts"],
        "build_toolchain": "go version go1.25.12 linux/amd64",
        "fail_closed_dimensions": EXPECTED_DIMENSIONS,
        "kubernetes_jobs": jobs,
        "blocking_codes": sorted(set(blockers)),
        "required_gates": {
            "G0": "BLOCKED_BY_DEPLOYABLE_CANDIDATE",
            "G2": "PASS_UNIT_AND_K8S_SELF_TEST",
            "G3": "PASS_CROSS_NODE_SELF_TEST",
            "G6": "BLOCKED_NOT_ENABLED",
        },
        "shared_infrastructure_touched": False,
        "production_applied": False,
        "run_scoped_resources_removed": not args.keep,
        "allowed_claims": [
            "The bound N008 atomic PKI candidate passed declared fail-closed TLS tests on both Kubernetes nodes"
        ],
        "does_not_prove": [
            "an approved issuer or offline root custody exists",
            "the N008 runtime flag or live Secret was changed",
            "per-probe certificates were issued",
            "two consecutive live rotations or live rollback passed",
            "G6 or production promotion is authorized",
        ],
    }
    output = args.output.resolve(strict=False)
    if output not in {OUTPUT.resolve(strict=False), Path("/tmp/m10-pki-rotation-k8s.json")}:
        raise base.CanaryError("output must be the N008 evidence path or documented /tmp path")
    output.parent.mkdir(parents=True, exist_ok=True)
    temporary = output.with_name(f".{output.name}.tmp")
    temporary.write_text(json.dumps(evidence, indent=2, sort_keys=True) + "\n", encoding="utf-8")
    temporary.replace(output)
    print(json.dumps(evidence, sort_keys=True))


if __name__ == "__main__":
    main()
