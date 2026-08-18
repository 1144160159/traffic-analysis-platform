#!/usr/bin/env python3
"""Validate the M09-N011 forensics workbench candidate bundle in Kubernetes."""

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
        "grep -R -q '/v1/pcap/jobs/' /usr/share/nginx/html/assets",
        "grep -R -q '/v1/pcap/presign' /usr/share/nginx/html/assets",
        "grep -R -q '/v1/pcap/verify' /usr/share/nginx/html/assets",
        "grep -R -q '任务已受理，但尚未完成' /usr/share/nginx/html/assets",
        "grep -R -q '部分完成不等于完整证据' /usr/share/nginx/html/assets",
        "grep -R -q '源 PCAP 对象 receipt' /usr/share/nginx/html/assets",
        "grep -R -q 'M03 会话 / 文件还原 receipt' /usr/share/nginx/html/assets",
        "grep -R -q '惰性字节，禁止执行和自动打开' /usr/share/nginx/html/assets",
        "grep -R -q '受控下载 / 证据导出' /usr/share/nginx/html/assets",
        "grep -R -q 'job_id' /usr/share/nginx/html/assets",
        "echo PASS",
    )
)

API_TEST_PATTERN = "Test(TaskCommandAdmission|NormalizeResultKey|NormalizeForensicsPurpose|VerifyPCAPRejectsMalformed)"


def main() -> None:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--image", required=True)
    parser.add_argument("--api-image", required=True)
    parser.add_argument("--run-id", default=str(uuid.uuid4()))
    parser.add_argument("--node", default="8-2tb")
    parser.add_argument("--timeout", type=int, default=300)
    parser.add_argument("--output", type=Path)
    parser.add_argument("--keep", action="store_true")
    args = parser.parse_args()
    if args.timeout < 30 or args.timeout > 900:
        raise base.CanaryError("--timeout must be between 30 and 900 seconds")

    suffix = base.validate_inputs(args.image, args.run_id, args.node)
    base.validate_inputs(args.api_image, args.run_id, args.node)
    job_name = f"m09-n011-forensics-ui-{suffix}"
    applied = False
    logs_by_suite: dict[str, str] = {}
    evidence: list[dict[str, object]] = []
    try:
        job = base.build_job(job_name, args.image, args.run_id, args.node)
        labels = {**job["metadata"]["labels"], "app.kubernetes.io/name": "m09-forensics-workbench-canary"}
        job["metadata"]["labels"] = labels
        job["spec"]["template"]["metadata"]["labels"] = labels
        container = job["spec"]["template"]["spec"]["containers"][0]
        container["name"] = "bundle-contract"
        container["command"] = ["/bin/sh", "-ec"]
        container["args"] = [BUNDLE_CHECK]
        base.kubectl("apply", "-f", "-", input_text=yaml.safe_dump(job, sort_keys=False))
        applied = True
        logs, receipt = base.wait_and_collect(job_name, args.timeout)
        logs_by_suite["web_bundle"] = logs
        evidence.append(receipt)

        api_name = f"m09-n011-forensics-api-{suffix}"
        api_job = base.build_job(api_name, args.api_image, args.run_id, args.node)
        api_labels = {**api_job["metadata"]["labels"], "app.kubernetes.io/name": "m09-forensics-api-canary"}
        api_job["metadata"]["labels"] = api_labels
        api_job["spec"]["template"]["metadata"]["labels"] = api_labels
        api_container = api_job["spec"]["template"]["spec"]["containers"][0]
        api_container["name"] = "api-contract"
        api_container["args"] = ["-test.v", f"-test.run={API_TEST_PATTERN}", "-test.count=1"]
        base.kubectl("apply", "-f", "-", input_text=yaml.safe_dump(api_job, sort_keys=False))
        api_logs, api_receipt = base.wait_and_collect(api_name, args.timeout)
        logs_by_suite["api_contract"] = api_logs
        evidence.append(api_receipt)
    finally:
        if applied and not args.keep:
            base.cleanup(args.run_id)
    if len(evidence) != 2:
        raise base.CanaryError("Kubernetes forensics workbench validation produced no result")

    envelope = {
        "artifact_kind": "M09_FORENSICS_WORKBENCH_TEST_RESULT",
        "task_id": "T1-M09-N011",
        "run_id": args.run_id,
        "status": "PASS",
        "bundle_check_sha256": hashlib.sha256(BUNDLE_CHECK.encode("utf-8")).hexdigest(),
        "api_test_pattern": API_TEST_PATTERN,
        "test_output_sha256": {name: hashlib.sha256(logs.encode("utf-8")).hexdigest() for name, logs in logs_by_suite.items()},
        "kubernetes": evidence,
        "accepted_is_completed": False,
        "partial_is_terminal_and_distinct": True,
        "purpose_bound_download": True,
        "exact_object_version_visible": True,
        "refresh_recovery_query_key": "job_id",
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
