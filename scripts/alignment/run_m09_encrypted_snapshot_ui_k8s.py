#!/usr/bin/env python3
"""Validate the M09 N008 encrypted snapshot UI bundle in Kubernetes."""

from __future__ import annotations

import argparse
import hashlib
import json
import uuid
from pathlib import Path

import yaml

import run_m09_encrypted_stats_seam_k8s as base


BUNDLE_CHECK = " && ".join(
    (
        "test -f /usr/share/nginx/html/index.html",
        "test -f /usr/share/nginx/html/.vite/manifest.json",
        "! find /usr/share/nginx/html -name mockServiceWorker.js -print -quit | grep -q .",
        "grep -F -q ': \"${USE_MOCK:=false}\"' /docker-entrypoint.sh",
        "grep -R -q '/v1/encrypted-traffic/snapshot' /usr/share/nginx/html/assets",
        "grep -R -q '加密传输本身不构成恶意判定' /usr/share/nginx/html/assets",
        "grep -R -q '实测为零' /usr/share/nginx/html/assets",
        "grep -R -q '无样本' /usr/share/nginx/html/assets",
        "grep -R -q '不可计算' /usr/share/nginx/html/assets",
        "grep -R -q '字段权限不足' /usr/share/nginx/html/assets",
        "grep -R -q '下钻首条事实' /usr/share/nginx/html/assets",
        "grep -R -q '缺失事实不会用模拟值填充' /usr/share/nginx/html/assets",
        "echo PASS",
    )
)


def main() -> None:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--image", required=True)
    parser.add_argument("--run-id", default=str(uuid.uuid4()))
    parser.add_argument("--node", default="8-2tb")
    parser.add_argument("--timeout", type=int, default=300)
    parser.add_argument("--output", type=Path)
    parser.add_argument("--keep", action="store_true")
    args = parser.parse_args()
    if args.timeout < 30 or args.timeout > 900:
        raise base.CanaryError("--timeout must be between 30 and 900 seconds")

    suffix = base.validate_inputs(args.image, args.run_id, args.node)
    job_name = f"m09-n008-encrypted-ui-{suffix}"
    applied = False
    logs = ""
    evidence = None
    try:
        job = base.build_job(job_name, args.image, args.run_id, args.node)
        labels = {**job["metadata"]["labels"], "app.kubernetes.io/name": "m09-encrypted-snapshot-ui-canary"}
        job["metadata"]["labels"] = labels
        job["spec"]["template"]["metadata"]["labels"] = labels
        container = job["spec"]["template"]["spec"]["containers"][0]
        container["name"] = "bundle-contract"
        container["command"] = ["/bin/sh", "-ec"]
        container["args"] = [BUNDLE_CHECK]
        base.kubectl("apply", "-f", "-", input_text=yaml.safe_dump(job, sort_keys=False))
        applied = True
        logs, evidence = base.wait_and_collect(job_name, args.timeout)
    finally:
        if applied and not args.keep:
            base.cleanup(args.run_id)
    if evidence is None:
        raise base.CanaryError("Kubernetes encrypted snapshot UI validation produced no result")

    envelope = {
        "artifact_kind": "M09_ENCRYPTED_SNAPSHOT_UI_TEST_RESULT",
        "task_id": "T1-M09-N008",
        "run_id": args.run_id,
        "status": "PASS",
        "bundle_check_sha256": hashlib.sha256(BUNDLE_CHECK.encode("utf-8")).hexdigest(),
        "test_output_sha256": hashlib.sha256(logs.encode("utf-8")).hexdigest(),
        "kubernetes": evidence,
        "mock_enabled": False,
        "production_applied": False,
        "run_scoped_resources_removed": not args.keep,
    }
    payload = json.dumps(envelope, sort_keys=True)
    if args.output is not None:
        args.output.parent.mkdir(parents=True, exist_ok=True)
        temporary = args.output.with_name(f".{args.output.name}.tmp")
        temporary.write_text(json.dumps(envelope, indent=2, sort_keys=True) + "\n", encoding="utf-8")
        temporary.replace(args.output)
    print(payload)


if __name__ == "__main__":
    main()
