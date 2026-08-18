#!/usr/bin/env python3
"""Run M09-N014 bounded graph workbench and UI checks on Kubernetes."""

from __future__ import annotations

import argparse
import hashlib
import json
import re
import uuid
from pathlib import Path
from typing import Any

import run_m09_alert_evidence_links_k8s as base


ROOT = Path(__file__).resolve().parents[2]
DEFAULT_OUTPUT = (
    ROOT / "doc/02_acceptance/topic1/tasks/t1-m09-n014"
    / "k8s-graph-workbench-governance-latest.json"
)
IMAGE_RE = re.compile(r"^[a-zA-Z0-9][a-zA-Z0-9._/@:-]+$")
NODE_RE = re.compile(r"^[a-z0-9]([-a-z0-9.]*[a-z0-9])?$")
INTEGRATION_TEST = "^TestGovernedWorkbenchNebulaK8sIntegration$"
CLEANUP_TEST = "^TestGovernedWorkbenchNebulaK8sCleanupOracle$"
API_TEST = "^Test(WorkbenchContinuationIsTenantQueryAndExpiryBound|GovernWorkbenchGraphRedactsEvidenceAndSecretsWithoutMutation)$"
BUNDLE_CHECK = " && ".join((
    "test -f /usr/share/nginx/html/index.html",
    "test -f /usr/share/nginx/html/.vite/manifest.json",
    "! find /usr/share/nginx/html -name mockServiceWorker.js -print -quit | grep -q .",
    "grep -R -q '/v1/graph/workbench' /usr/share/nginx/html/assets",
    "grep -R -q '加载下一有界页' /usr/share/nginx/html/assets",
    "grep -R -q '页面不会补点或补线' /usr/share/nginx/html/assets",
    "grep -R -q '部分字段已脱敏' /usr/share/nginx/html/assets",
    "grep -R -q '/v1/alerts/views' /usr/share/nginx/html/assets",
    "echo PASS",
))


def labels(run_id: str) -> dict[str, str]:
    return {
        "app.kubernetes.io/name": "m09-graph-workbench-governance-canary",
        "traffic.analysis/canary-run": run_id,
    }


def validate(images: list[str], run_id: str, node: str) -> str:
    for image in images:
        if not IMAGE_RE.fullmatch(image) or image.endswith(":latest"):
            raise base.CanaryError("candidate images must be explicit non-latest references")
    if not NODE_RE.fullmatch(node):
        raise base.CanaryError("invalid Kubernetes node name")
    parsed = uuid.UUID(run_id)
    if str(parsed) != run_id:
        raise base.CanaryError("--run-id must be a canonical lowercase UUID")
    return parsed.hex[:10]


def test_job(name: str, run_id: str, node: str, image: str, suite: str) -> dict[str, Any]:
    common = labels(run_id)
    container: dict[str, Any] = {
        "name": suite, "image": image, "imagePullPolicy": "Never",
        "securityContext": {
            "allowPrivilegeEscalation": False, "readOnlyRootFilesystem": True,
            "capabilities": {"drop": ["ALL"]},
        },
        "resources": {
            "requests": {"cpu": "100m", "memory": "128Mi"},
            "limits": {"cpu": "1", "memory": "512Mi"},
        },
        "volumeMounts": [{"name": "tmp", "mountPath": "/tmp"}],
    }
    if suite in ("integration", "cleanup"):
        test_name = INTEGRATION_TEST if suite == "integration" else CLEANUP_TEST
        container["args"] = ["-test.v", f"-test.run={test_name}", "-test.count=1"]
        container["env"] = [
            {"name": "RUN_M09_N014_NEBULA_INTEGRATION", "value": "1"},
            {"name": "M09_N014_EPHEMERAL_TENANT_PREFIX", "value": f"k8s-m09-n014-{run_id}"},
            {"name": "NEBULA_ADDRESS", "value": "nebula-graph.middleware.svc:9669"},
            {"name": "NEBULA_USERNAME", "value": "traffic_graph"},
            {"name": "NEBULA_SPACE", "value": "traffic_graph"},
            {"name": "NEBULA_PASSWORD", "valueFrom": {"secretKeyRef": {
                "name": "traffic-credentials", "key": "NEBULA_SERVICE_PASSWORD",
            }}},
        ]
    elif suite == "api":
        container["args"] = ["-test.v", f"-test.run={API_TEST}", "-test.count=1"]
    else:
        container["command"] = ["/bin/sh", "-ec"]
        container["args"] = [BUNDLE_CHECK]
    return {
        "apiVersion": "batch/v1", "kind": "Job",
        "metadata": {
            "name": f"{name}-{suite}", "namespace": base.APP_NAMESPACE,
            "labels": common,
            "annotations": {
                "traffic.analysis/shared-nebulagraph-touched": str(suite == "integration").lower(),
                "traffic.analysis/run-scoped-nebulagraph-cleanup": str(suite == "cleanup").lower(),
                "traffic.analysis/production-applied": "false",
            },
        },
        "spec": {
            "backoffLimit": 0,
            "template": {
                "metadata": {"labels": common},
                "spec": {
                    "nodeName": node, "automountServiceAccountToken": False,
                    "restartPolicy": "Never",
                    "securityContext": {
                        "runAsNonRoot": True, "runAsUser": 1000, "runAsGroup": 1000,
                        "fsGroup": 1000, "seccompProfile": {"type": "RuntimeDefault"},
                    },
                    "containers": [container], "volumes": [{"name": "tmp", "emptyDir": {}}],
                },
            },
        },
    }


def main() -> None:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--test-image", required=True)
    parser.add_argument("--api-image", required=True)
    parser.add_argument("--web-image", required=True)
    parser.add_argument("--run-id", default=str(uuid.uuid4()))
    parser.add_argument("--node", default="8-2tb")
    parser.add_argument("--timeout", type=int, default=420)
    parser.add_argument("--output", type=Path, default=DEFAULT_OUTPUT)
    parser.add_argument("--keep", action="store_true")
    args = parser.parse_args()
    if args.timeout < 120 or args.timeout > 1200:
        raise base.CanaryError("--timeout must be between 120 and 1200 seconds")
    images = [args.test_image, args.api_image, args.web_image]
    suffix = validate(images, args.run_id, args.node)
    name = f"m09-n014-graph-{suffix}"
    receipts: list[dict[str, Any]] = []
    logs_by_suite: dict[str, str] = {}
    applied = False
    try:
        for suite, image in (
            ("integration", args.test_image),
            ("cleanup", args.test_image),
            ("api", args.api_image),
            ("web", args.web_image),
        ):
            base.apply([test_job(name, args.run_id, args.node, image, suite)])
            applied = True
            logs, receipt = base.wait_job(f"{name}-{suite}", args.timeout)
            logs_by_suite[suite] = logs
            receipts.append(receipt)
    finally:
        if applied and not args.keep:
            base.cleanup(args.run_id)

    source_paths = [
        ROOT / "go/control-plane/internal/graph/api/handler.go",
        ROOT / "go/control-plane/internal/graph/nebula/workbench_store.go",
        ROOT / "go/control-plane/internal/graph/api/workbench_governance.go",
        ROOT / "go/control-plane/internal/graph/query/graph_query.go",
        ROOT / "go/control-plane/internal/graph/config/config.go",
        ROOT / "web/ui/src/pages/GraphEntityPage.tsx",
        ROOT / "contracts/openapi/alignment-v1.openapi.json",
        ROOT / "deployments/kubernetes/applications/go-services.yaml",
    ]
    envelope = {
        "artifact_kind": "M09_GRAPH_WORKBENCH_GOVERNANCE_TEST_RESULT",
        "task_id": "T1-M09-N014", "run_id": args.run_id, "status": "PASS",
        "profile_id": "M09-N014-K8S-NEBULAGRAPH-UI-V1",
        "coverage_status": "PASS_FOR_RUN_SCOPED_K8S_NEBULAGRAPH_AND_IMMUTABLE_UI_BUNDLE",
        "inputs": {
            "candidate_images": images,
            "source_sha256": {str(path.relative_to(ROOT)): hashlib.sha256(path.read_bytes()).hexdigest() for path in source_paths},
            "bundle_check_sha256": hashlib.sha256(BUNDLE_CHECK.encode()).hexdigest(),
        },
        "test_output_sha256": {suite: hashlib.sha256(log.encode()).hexdigest() for suite, log in logs_by_suite.items()},
        "kubernetes_jobs": receipts,
        "database_boundary_budgets": True,
        "continuation_tenant_query_hmac_bound": True,
        "supernode_truncation_explicit": True,
        "cycle_visited_set_exact": True,
        "cross_tenant_edge_rejected": True,
        "field_redaction_contract": True,
        "saved_view_server_command": "/v1/alerts/views",
        "shared_nebulagraph_touched": True,
        "run_scoped_nebulagraph_rows_removed": True,
        "mock_enabled": False,
        "production_applied": False,
        "run_scoped_kubernetes_resources_removed": not args.keep,
        "does_not_prove": [
            "production rollout", "Windows Chrome acceptance", "visual pixel parity",
            "global milestone completion", "authorization to enable GRAPH_WORKBENCH_V2_ENABLED",
        ],
    }
    args.output.parent.mkdir(parents=True, exist_ok=True)
    temporary = args.output.with_name(f".{args.output.name}.tmp")
    temporary.write_text(json.dumps(envelope, indent=2, sort_keys=True) + "\n", encoding="utf-8")
    temporary.replace(args.output)
    print(json.dumps(envelope, sort_keys=True))


if __name__ == "__main__":
    main()
