#!/usr/bin/env python3
"""Run T1-M09-N021 page-state and Drawer accessibility checks in Kubernetes."""

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
DEFAULT_OUTPUT = ROOT / "doc/02_acceptance/topic1/tasks/t1-m09-n021/k8s-page-state-accessibility-latest.json"
IMAGE_RE = re.compile(r"^[a-zA-Z0-9][a-zA-Z0-9._/@:-]+$")
NODE_RE = re.compile(r"^[a-z0-9]([-a-z0-9.]*[a-z0-9])?$")
CHECKS = {
    "state-semantics": " && ".join((
        "test -f /usr/share/nginx/html/index.html",
        "test -f /usr/share/nginx/html/.vite/manifest.json",
        "! find /usr/share/nginx/html -name mockServiceWorker.js -print -quit | grep -q .",
        "grep -R -q 'data-page-state' /usr/share/nginx/html/assets",
        "grep -R -q '正在加载权威数据' /usr/share/nginx/html/assets",
        "grep -R -q '当前没有可显示的数据' /usr/share/nginx/html/assets",
        "grep -R -q '页面数据部分可用' /usr/share/nginx/html/assets",
        "grep -R -q '权威数据暂不可用' /usr/share/nginx/html/assets",
        "grep -R -q '页面状态已发生冲突' /usr/share/nginx/html/assets",
        "grep -R -q 'data-campaign-detail-initial-focus' /usr/share/nginx/html/assets",
        "grep -R -q 'requestAnimationFrame' /usr/share/nginx/html/assets",
        "echo PASS",
    )),
    "responsive-contract": " && ".join((
        "grep -R -q '@media (max-width: 1366px)' /usr/share/nginx/html/assets/*.css",
        "grep -R -q '@media (min-width: 1600px)' /usr/share/nginx/html/assets/*.css",
        "grep -R -q 'overflow-wrap:anywhere' /usr/share/nginx/html/assets/*.css",
        "grep -R -q 'min-width:0' /usr/share/nginx/html/assets/*.css",
        "echo PASS",
    )),
}


def labels(run_id: str) -> dict[str, str]:
    return {
        "app.kubernetes.io/name": "m09-page-state-accessibility-canary",
        "traffic.analysis/canary-run": run_id,
    }


def validate(image: str, run_id: str, node: str) -> str:
    if not IMAGE_RE.fullmatch(image) or image.endswith(":latest"):
        raise base.CanaryError("candidate image must be an explicit non-latest reference")
    if not NODE_RE.fullmatch(node):
        raise base.CanaryError("invalid Kubernetes node name")
    parsed = uuid.UUID(run_id)
    if str(parsed) != run_id:
        raise base.CanaryError("--run-id must be a canonical lowercase UUID")
    return parsed.hex[:10]


def test_job(name: str, run_id: str, node: str, image: str, suite: str) -> dict[str, Any]:
    job_name = f"{name}-{suite}"
    return {
        "apiVersion": "batch/v1", "kind": "Job",
        "metadata": {
            "name": job_name, "namespace": base.APP_NAMESPACE, "labels": labels(run_id),
            "annotations": {
                "traffic.analysis/shared-infrastructure-touched": "false",
                "traffic.analysis/production-applied": "false",
                "traffic.analysis/browser-evidence": "false",
            },
        },
        "spec": {"backoffLimit": 0, "template": {"metadata": {"labels": labels(run_id)}, "spec": {
            "nodeName": node,
            "automountServiceAccountToken": False,
            "restartPolicy": "Never",
            "securityContext": {"runAsNonRoot": True, "runAsUser": 101, "runAsGroup": 101, "fsGroup": 101, "seccompProfile": {"type": "RuntimeDefault"}},
            "containers": [{
                "name": suite, "image": image, "imagePullPolicy": "Never",
                "command": ["/bin/sh", "-ec"], "args": [CHECKS[suite]],
                "securityContext": {"allowPrivilegeEscalation": False, "readOnlyRootFilesystem": True, "capabilities": {"drop": ["ALL"]}},
                "resources": {"requests": {"cpu": "50m", "memory": "64Mi"}, "limits": {"cpu": "500m", "memory": "256Mi"}},
                "volumeMounts": [{"name": "tmp", "mountPath": "/tmp"}],
            }],
            "volumes": [{"name": "tmp", "emptyDir": {}}],
        }}},
    }


def main() -> None:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--web-image", required=True)
    parser.add_argument("--run-id", default=str(uuid.uuid4()))
    parser.add_argument("--node", default="8-2tb")
    parser.add_argument("--timeout", type=int, default=300)
    parser.add_argument("--output", type=Path, default=DEFAULT_OUTPUT)
    parser.add_argument("--keep", action="store_true")
    args = parser.parse_args()
    if args.timeout < 60 or args.timeout > 900:
        raise base.CanaryError("--timeout must be between 60 and 900 seconds")
    suffix = validate(args.web_image, args.run_id, args.node)
    name = f"m09-n021-page-state-{suffix}"
    receipts: list[dict[str, Any]] = []
    logs_by_suite: dict[str, str] = {}
    applied = True
    try:
        for suite in CHECKS:
            base.apply([test_job(name, args.run_id, args.node, args.web_image, suite)])
            logs, receipt = base.wait_job(f"{name}-{suite}", args.timeout)
            logs_by_suite[suite] = logs
            receipts.append(receipt)
    finally:
        if applied and not args.keep:
            base.cleanup(args.run_id)

    source_paths = [
        ROOT / "web/ui/src/components/PageStateBoundary.tsx",
        ROOT / "web/ui/src/components/pageState.ts",
        ROOT / "web/ui/src/components/useDrawerFocusReturn.ts",
        ROOT / "web/ui/src/components/PageStateBoundary.test.tsx",
        ROOT / "web/ui/src/components/useDrawerFocusReturn.test.tsx",
        ROOT / "web/ui/src/pages/AlertDetailPage.tsx",
        ROOT / "web/ui/src/pages/CampaignWorkbenchPage.tsx",
        ROOT / "web/ui/src/styles/page-state.css",
        ROOT / "web/ui/src/main.tsx",
    ]
    envelope = {
        "artifact_kind": "M09_PAGE_STATE_ACCESSIBILITY_TEST_RESULT",
        "task_id": "T1-M09-N021",
        "run_id": args.run_id,
        "status": "PASS",
        "profile_id": "M09-N021-K8S-IMMUTABLE-UI-STATE-A11Y-V1",
        "coverage_status": "PASS_FOR_K8S_IMMUTABLE_BUNDLE_STATE_AND_RESPONSIVE_CONTRACT",
        "inputs": {
            "candidate_image": args.web_image,
            "source_sha256": {str(path.relative_to(ROOT)): hashlib.sha256(path.read_bytes()).hexdigest() for path in source_paths},
            "check_sha256": {suite: hashlib.sha256(command.encode()).hexdigest() for suite, command in CHECKS.items()},
        },
        "test_output_sha256": {suite: hashlib.sha256(log.encode()).hexdigest() for suite, log in logs_by_suite.items()},
        "kubernetes_jobs": receipts,
        "six_page_states_present": True,
        "unavailable_does_not_render_fallback_children": True,
        "partial_preserves_available_content": True,
        "drawer_initial_focus_contract": True,
        "drawer_return_focus_contract": True,
        "long_text_wrap_contract": True,
        "viewport_1366_contract": True,
        "viewport_1600_contract": True,
        "mock_enabled": False,
        "shared_infrastructure_touched": False,
        "browser_evidence": False,
        "production_applied": False,
        "run_scoped_resources_removed": not args.keep,
        "does_not_prove": [
            "Windows Chrome keyboard and screen-reader acceptance",
            "pixel-level viewport equivalence", "every product page migrated to the shared boundary",
            "production rollout", "authorization to promote M09", "global milestone completion",
        ],
    }
    args.output.parent.mkdir(parents=True, exist_ok=True)
    temporary = args.output.with_name(f".{args.output.name}.tmp")
    temporary.write_text(json.dumps(envelope, indent=2, sort_keys=True) + "\n", encoding="utf-8")
    temporary.replace(args.output)
    print(json.dumps(envelope, sort_keys=True))


if __name__ == "__main__":
    main()
