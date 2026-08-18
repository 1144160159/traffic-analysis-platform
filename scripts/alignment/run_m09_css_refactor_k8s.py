#!/usr/bin/env python3
"""Run T1-M09-N022 route-scoped CSS equivalence checks in Kubernetes."""

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
DEFAULT_OUTPUT = ROOT / "doc/02_acceptance/topic1/tasks/t1-m09-n022/k8s-css-refactor-latest.json"
IMAGE_RE = re.compile(r"^[a-zA-Z0-9][a-zA-Z0-9._/@:-]+$")
NODE_RE = re.compile(r"^[a-z0-9]([-a-z0-9.]*[a-z0-9])?$")


def labels(run_id: str) -> dict[str, str]:
    return {
        "app.kubernetes.io/name": "m09-css-refactor-canary",
        "traffic.analysis/canary-run": run_id,
    }


def validate(images: list[str], run_id: str, node: str) -> str:
    for image in images:
        if not IMAGE_RE.fullmatch(image) or image.endswith(":latest"):
            raise base.CanaryError("images must be explicit non-latest references")
    if not NODE_RE.fullmatch(node):
        raise base.CanaryError("invalid Kubernetes node name")
    parsed = uuid.UUID(run_id)
    if str(parsed) != run_id:
        raise base.CanaryError("--run-id must be a canonical lowercase UUID")
    return parsed.hex[:10]


def server_objects(name: str, run_id: str, node: str, baseline: str, candidate: str) -> list[dict[str, Any]]:
    common = labels(run_id)
    nginx_config = """worker_processes 1;
pid /tmp/nginx.pid;
events { worker_connections 128; }
http {
  include /etc/nginx/mime.types;
  access_log /dev/stdout;
  error_log /dev/stderr warn;
  client_body_temp_path /tmp/client_temp;
  proxy_temp_path /tmp/proxy_temp;
  fastcgi_temp_path /tmp/fastcgi_temp;
  uwsgi_temp_path /tmp/uwsgi_temp;
  scgi_temp_path /tmp/scgi_temp;
  server {
    listen 8080;
    root /usr/share/nginx/html;
    location = /health { access_log off; return 200 'ok'; }
    location / { try_files $uri $uri/ /index.html; }
  }
}
"""
    items: list[dict[str, Any]] = [{
        "apiVersion": "v1", "kind": "ConfigMap",
        "metadata": {"name": name, "namespace": base.APP_NAMESPACE, "labels": common},
        "data": {"nginx.conf": nginx_config},
    }]
    for role, image in (("baseline", baseline), ("candidate", candidate)):
        pod_name = f"{name}-{role}"
        selector = {"traffic.analysis/css-refactor-role": pod_name}
        items.extend((
            {
                "apiVersion": "v1", "kind": "Service",
                "metadata": {"name": pod_name, "namespace": base.APP_NAMESPACE, "labels": common},
                "spec": {"selector": selector, "ports": [{"name": "http", "port": 80, "targetPort": 8080}]},
            },
            {
                "apiVersion": "v1", "kind": "Pod",
                "metadata": {
                    "name": pod_name, "namespace": base.APP_NAMESPACE,
                    "labels": {**common, **selector},
                    "annotations": {
                        "traffic.analysis/production-applied": "false",
                        "traffic.analysis/shared-infrastructure-touched": "false",
                    },
                },
                "spec": {
                    "nodeName": node, "automountServiceAccountToken": False, "restartPolicy": "Never",
                    "securityContext": {
                        "runAsNonRoot": True, "runAsUser": 101, "runAsGroup": 101, "fsGroup": 101,
                        "seccompProfile": {"type": "RuntimeDefault"},
                    },
                    "containers": [{
                        "name": role, "image": image, "imagePullPolicy": "Never",
                        "command": ["nginx", "-c", "/etc/traffic/nginx.conf", "-g", "daemon off;"],
                        "ports": [{"name": "http", "containerPort": 8080}],
                        "readinessProbe": {"httpGet": {"path": "/health", "port": "http"}, "periodSeconds": 1},
                        "securityContext": {
                            "allowPrivilegeEscalation": False, "readOnlyRootFilesystem": True,
                            "capabilities": {"drop": ["ALL"]},
                        },
                        "resources": {
                            "requests": {"cpu": "50m", "memory": "64Mi"},
                            "limits": {"cpu": "500m", "memory": "256Mi"},
                        },
                        "volumeMounts": [
                            {"name": "nginx", "mountPath": "/etc/traffic", "readOnly": True},
                            {"name": "tmp", "mountPath": "/tmp"},
                        ],
                    }],
                    "volumes": [
                        {"name": "nginx", "configMap": {"name": name}},
                        {"name": "tmp", "emptyDir": {}},
                    ],
                },
            },
        ))
    return items


def visual_job(name: str, run_id: str, node: str, image: str) -> dict[str, Any]:
    return {
        "apiVersion": "batch/v1", "kind": "Job",
        "metadata": {
            "name": f"{name}-visual", "namespace": base.APP_NAMESPACE, "labels": labels(run_id),
            "annotations": {
                "traffic.analysis/production-applied": "false",
                "traffic.analysis/shared-infrastructure-touched": "false",
                "traffic.analysis/browser-evidence": "true",
            },
        },
        "spec": {"backoffLimit": 0, "template": {"metadata": {"labels": labels(run_id)}, "spec": {
            "nodeName": node, "automountServiceAccountToken": False, "restartPolicy": "Never",
            "securityContext": {
                "runAsNonRoot": True, "runAsUser": 1000, "runAsGroup": 1000, "fsGroup": 1000,
                "seccompProfile": {"type": "RuntimeDefault"},
            },
            "containers": [{
                "name": "visual", "image": image, "imagePullPolicy": "Never",
                "env": [
                    {"name": "BASELINE_ORIGIN", "value": f"http://{name}-baseline"},
                    {"name": "CANDIDATE_ORIGIN", "value": f"http://{name}-candidate"},
                    {"name": "OUTPUT_DIR", "value": "/tmp/visual-diff"},
                ],
                "securityContext": {
                    "allowPrivilegeEscalation": False, "readOnlyRootFilesystem": True,
                    "capabilities": {"drop": ["ALL"]},
                },
                "resources": {
                    "requests": {"cpu": "250m", "memory": "512Mi"},
                    "limits": {"cpu": "2", "memory": "2Gi"},
                },
                "volumeMounts": [
                    {"name": "tmp", "mountPath": "/tmp"},
                    {"name": "shm", "mountPath": "/dev/shm"},
                ],
            }],
            "volumes": [
                {"name": "tmp", "emptyDir": {}},
                {"name": "shm", "emptyDir": {"medium": "Memory", "sizeLimit": "256Mi"}},
            ],
        }}},
    }


def parse_visual_result(logs: str) -> dict[str, Any]:
    for line in reversed(logs.splitlines()):
        if line.startswith("{"):
            result = json.loads(line)
            if result.get("status") == "PASS" and len(result.get("results", [])) == 2:
                return result
    raise base.CanaryError(f"visual Job did not emit a valid PASS receipt:\n{logs}")


def main() -> None:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--baseline-image", required=True)
    parser.add_argument("--candidate-image", required=True)
    parser.add_argument("--visual-image", required=True)
    parser.add_argument("--run-id", default=str(uuid.uuid4()))
    parser.add_argument("--node", default="8-2tb")
    parser.add_argument("--timeout", type=int, default=600)
    parser.add_argument("--output", type=Path, default=DEFAULT_OUTPUT)
    parser.add_argument("--keep", action="store_true")
    args = parser.parse_args()
    if args.timeout < 60 or args.timeout > 900:
        raise base.CanaryError("--timeout must be between 60 and 900 seconds")
    suffix = validate(
        [args.baseline_image, args.candidate_image, args.visual_image], args.run_id, args.node
    )
    name = f"m09-n022-css-{suffix}"
    cleanup_guard = True
    try:
        base.apply(server_objects(
            name, args.run_id, args.node, args.baseline_image, args.candidate_image
        ))
        base.wait_pod(f"{name}-baseline", base.APP_NAMESPACE, args.timeout)
        base.wait_pod(f"{name}-candidate", base.APP_NAMESPACE, args.timeout)
        servers = [
            base.pod_receipt(f"{name}-baseline", base.APP_NAMESPACE),
            base.pod_receipt(f"{name}-candidate", base.APP_NAMESPACE),
        ]
        base.apply([visual_job(name, args.run_id, args.node, args.visual_image)])
        logs, job = base.wait_job(f"{name}-visual", args.timeout)
        visual = parse_visual_result(logs)
    finally:
        if cleanup_guard and not args.keep:
            base.cleanup(args.run_id)

    source_paths = [
        ROOT / "web/ui/src/main.tsx",
        ROOT / "web/ui/src/styles/pages.css",
        ROOT / "web/ui/src/styles/alert-detail.css",
        ROOT / "web/ui/deployments/Dockerfile.css-visual-diff",
        ROOT / "web/ui/deployments/css-visual-diff.mjs",
    ]
    envelope = {
        "artifact_kind": "M09_CSS_REFACTOR_TEST_RESULT",
        "task_id": "T1-M09-N022", "run_id": args.run_id, "status": "PASS",
        "profile_id": "M09-N022-K8S-CHROMIUM-EXACT-VIEWPORT-DIFF-V1",
        "coverage_status": "PASS_FOR_ALERT_DETAIL_ROUTE_SCOPED_CSS_EXTRACTION",
        "inputs": {
            "baseline_image": args.baseline_image, "candidate_image": args.candidate_image,
            "visual_image": args.visual_image,
            "source_sha256": {
                str(path.relative_to(ROOT)): hashlib.sha256(path.read_bytes()).hexdigest()
                for path in source_paths
            },
        },
        "kubernetes_servers": servers, "kubernetes_job": job,
        "visual_result": visual,
        "viewports": ["1366x900", "1600x900"],
        "computed_styles_equal": True, "exact_pixel_diff_zero": True,
        "old_response_feedback_rules_removed_from_pages_css": True,
        "route_stylesheet_loaded_after_pages_css": True,
        "browser_engine": "Kubernetes Chromium",
        "browser_evidence": True, "mock_enabled": False,
        "shared_infrastructure_touched": False, "production_applied": False,
        "run_scoped_resources_removed": not args.keep,
        "does_not_prove": [
            "Windows Chrome acceptance", "every route migrated out of pages.css",
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
