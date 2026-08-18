#!/usr/bin/env python3
"""Run M09-N015 OpenSearch PIT/cursor and immutable UI checks on Kubernetes."""

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
    ROOT / "doc/02_acceptance/topic1/tasks/t1-m09-n015"
    / "k8s-opensearch-cursor-latest.json"
)
IMAGE_RE = re.compile(r"^[a-zA-Z0-9][a-zA-Z0-9._/@:-]+$")
NODE_RE = re.compile(r"^[a-z0-9]([-a-z0-9.]*[a-z0-9])?$")
INTEGRATION_TEST = "^TestOpenSearchCursorK8sIntegration$"
CLEANUP_TEST = "^TestOpenSearchCursorK8sCleanupOracle$"
UNIT_TEST = "^Test(OpenSearchCursorLiveTraversalUsesBoundedStableSearchAfter|OpenSearchLiveCursorFailsClosedAfterAliasSwitch|OpenSearchCursorRejectsTenantTamperAndQueryDriftBeforeSearch|OpenSearchPITCursorCreatesRotatesAndClosesTenantContext|OpenSearchCursorFailsClosedOnTimeoutOrShardFailure|SearchCursorCodecExpiresAndRejectsUnknownClaims)$"
BUNDLE_CHECK = " && ".join((
    "test -f /usr/share/nginx/html/index.html",
    "test -f /usr/share/nginx/html/.vite/manifest.json",
    "! find /usr/share/nginx/html -name mockServiceWorker.js -print -quit | grep -q .",
    "grep -R -q '/v1/alerts/search' /usr/share/nginx/html/assets",
    "grep -R -q 'PIT 一致性分页' /usr/share/nginx/html/assets",
    "grep -R -q '不回退伪数据' /usr/share/nginx/html/assets",
    "grep -R -q 'ALERT_SEARCH_CURSOR_V1_ENABLED' /usr/share/nginx/html",
    "echo PASS",
))


def labels(run_id: str) -> dict[str, str]:
    return {
        "app.kubernetes.io/name": "m09-opensearch-cursor-canary",
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


def test_job(name: str, run_id: str, suffix: str, node: str, image: str, suite: str) -> dict[str, Any]:
    container: dict[str, Any] = {
        "name": suite,
        "image": image,
        "imagePullPolicy": "Never",
        "securityContext": {
            "allowPrivilegeEscalation": False,
            "readOnlyRootFilesystem": True,
            "capabilities": {"drop": ["ALL"]},
        },
        "resources": {
            "requests": {"cpu": "100m", "memory": "128Mi"},
            "limits": {"cpu": "1", "memory": "512Mi"},
        },
        "volumeMounts": [{"name": "tmp", "mountPath": "/tmp"}],
    }
    if suite in ("integration", "cleanup"):
        selected = INTEGRATION_TEST if suite == "integration" else CLEANUP_TEST
        container["args"] = ["-test.v", f"-test.run={selected}", "-test.count=1"]
        container["env"] = [
            {"name": "RUN_M09_N015_OPENSEARCH_INTEGRATION", "value": "1"},
            {"name": "M09_N015_RESOURCE_SUFFIX", "value": suffix},
            {"name": "M09_N015_OPENSEARCH_URL", "value": "http://opensearch.middleware.svc:9200"},
            {"name": "OPENSEARCH_USERNAME", "value": "admin"},
            {"name": "OPENSEARCH_PASSWORD", "valueFrom": {"secretKeyRef": {
                "name": "traffic-credentials", "key": "OPENSEARCH_ADMIN_PASSWORD",
            }}},
        ]
    elif suite == "unit":
        container["args"] = ["-test.v", f"-test.run={UNIT_TEST}", "-test.count=1"]
    else:
        container["command"] = ["/bin/sh", "-ec"]
        container["args"] = [BUNDLE_CHECK]
    return {
        "apiVersion": "batch/v1",
        "kind": "Job",
        "metadata": {
            "name": f"{name}-{suite}",
            "namespace": base.APP_NAMESPACE,
            "labels": labels(run_id),
            "annotations": {
                "traffic.analysis/shared-opensearch-touched": str(suite == "integration").lower(),
                "traffic.analysis/run-scoped-opensearch-cleanup": str(suite == "cleanup").lower(),
                "traffic.analysis/production-applied": "false",
            },
        },
        "spec": {
            "backoffLimit": 0,
            "template": {
                "metadata": {"labels": labels(run_id)},
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
                    "containers": [container],
                    "volumes": [{"name": "tmp", "emptyDir": {}}],
                },
            },
        },
    }


def main() -> None:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--test-image", required=True)
    parser.add_argument("--web-image", required=True)
    parser.add_argument("--run-id", default=str(uuid.uuid4()))
    parser.add_argument("--node", default="8-2tb")
    parser.add_argument("--timeout", type=int, default=420)
    parser.add_argument("--output", type=Path, default=DEFAULT_OUTPUT)
    parser.add_argument("--keep", action="store_true")
    args = parser.parse_args()
    if args.timeout < 120 or args.timeout > 1200:
        raise base.CanaryError("--timeout must be between 120 and 1200 seconds")
    images = [args.test_image, args.web_image]
    suffix = validate(images, args.run_id, args.node)
    name = f"m09-n015-os-{suffix}"
    receipts: list[dict[str, Any]] = []
    logs_by_suite: dict[str, str] = {}
    applied = False
    integration_error: Exception | None = None
    try:
        base.apply([test_job(name, args.run_id, suffix, args.node, args.test_image, "integration")])
        applied = True
        logs, receipt = base.wait_job(f"{name}-integration", args.timeout)
        logs_by_suite["integration"] = logs
        receipts.append(receipt)
    except Exception as error:  # cleanup must still run against the exact suffix
        integration_error = error
    finally:
        try:
            base.apply([test_job(name, args.run_id, suffix, args.node, args.test_image, "cleanup")])
            applied = True
            logs, receipt = base.wait_job(f"{name}-cleanup", args.timeout)
            logs_by_suite["cleanup"] = logs
            receipts.append(receipt)
        finally:
            if applied and not args.keep:
                base.cleanup(args.run_id)
    if integration_error is not None:
        raise integration_error

    for suite, image in (("unit", args.test_image), ("web", args.web_image)):
        try:
            base.apply([test_job(name, args.run_id, suffix, args.node, image, suite)])
            logs, receipt = base.wait_job(f"{name}-{suite}", args.timeout)
            logs_by_suite[suite] = logs
            receipts.append(receipt)
        finally:
            if not args.keep:
                base.cleanup(args.run_id)

    source_paths = [
        ROOT / "go/control-plane/internal/alert/repository/opensearch.go",
        ROOT / "go/control-plane/internal/alert/repository/opensearch_cursor.go",
        ROOT / "go/control-plane/internal/alert/repository/alert_snapshot.go",
        ROOT / "go/control-plane/internal/alert/api/handler.go",
        ROOT / "go/control-plane/internal/alert/config/config.go",
        ROOT / "web/ui/src/pages/AlertTriagePage.tsx",
        ROOT / "web/ui/src/services/alertSearchCursorApi.ts",
        ROOT / "contracts/openapi/alignment-v1.openapi.json",
        ROOT / "deployments/kubernetes/applications/go-services.yaml",
        ROOT / "deployments/kubernetes/applications/web-ui.yaml",
    ]
    envelope = {
        "artifact_kind": "M09_OPENSEARCH_CURSOR_TEST_RESULT",
        "task_id": "T1-M09-N015",
        "run_id": args.run_id,
        "status": "PASS",
        "profile_id": "M09-N015-K8S-OPENSEARCH-PIT-UI-V1",
        "coverage_status": "PASS_FOR_RUN_SCOPED_K8S_OPENSEARCH_AND_IMMUTABLE_UI_BUNDLE",
        "inputs": {
            "candidate_images": images,
            "source_sha256": {str(path.relative_to(ROOT)): hashlib.sha256(path.read_bytes()).hexdigest() for path in source_paths},
            "bundle_check_sha256": hashlib.sha256(BUNDLE_CHECK.encode()).hexdigest(),
        },
        "test_output_sha256": {suite: hashlib.sha256(log.encode()).hexdigest() for suite, log in logs_by_suite.items()},
        "kubernetes_jobs": receipts,
        "stable_search_after_sort": True,
        "tenant_query_size_hmac_bound": True,
        "live_cursor_alias_switch_fails_closed": True,
        "pit_alias_switch_keeps_frozen_snapshot": True,
        "source_target_watermark_present": True,
        "opensearch_unavailable_fails_closed": True,
        "shared_opensearch_touched": True,
        "run_scoped_indices_and_alias_removed": True,
        "mock_enabled": False,
        "backend_runtime_flag_enabled_in_production": False,
        "frontend_runtime_flag_enabled_in_production": False,
        "production_applied": False,
        "run_scoped_kubernetes_resources_removed": not args.keep,
        "does_not_prove": [
            "production rollout", "72M-document performance", "Windows Chrome acceptance",
            "global milestone completion", "authorization to enable either cursor runtime flag",
        ],
    }
    args.output.parent.mkdir(parents=True, exist_ok=True)
    temporary = args.output.with_name(f".{args.output.name}.tmp")
    temporary.write_text(json.dumps(envelope, indent=2, sort_keys=True) + "\n", encoding="utf-8")
    temporary.replace(args.output)
    print(json.dumps(envelope, sort_keys=True))


if __name__ == "__main__":
    main()
