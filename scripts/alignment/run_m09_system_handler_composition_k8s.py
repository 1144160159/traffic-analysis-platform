#!/usr/bin/env python3
"""Run the M09 N003 system-handler composition characterization in Kubernetes."""

from __future__ import annotations

import argparse
import hashlib
import json
import uuid
from pathlib import Path

import yaml

import run_m09_encrypted_stats_seam_k8s as base


TEST_PATTERN = "TestNewAlertSystemHandlerWiresEncryptedTrafficStatsSlice"


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
    base.TEST_PATTERN = TEST_PATTERN
    job_name = f"m09-n003-wiring-{suffix}"
    applied = False
    logs = ""
    evidence = None
    try:
        job = base.build_job(job_name, args.image, args.run_id, args.node)
        job["metadata"]["labels"]["app.kubernetes.io/name"] = "m09-system-handler-composition-canary"
        job["spec"]["template"]["metadata"]["labels"]["app.kubernetes.io/name"] = "m09-system-handler-composition-canary"
        base.kubectl("apply", "-f", "-", input_text=yaml.safe_dump(job, sort_keys=False))
        applied = True
        logs, evidence = base.wait_and_collect(job_name, args.timeout)
    finally:
        if applied and not args.keep:
            base.cleanup(args.run_id)
    if evidence is None:
        raise base.CanaryError("Kubernetes composition characterization produced no result")

    envelope = {
        "artifact_kind": "M09_SYSTEM_HANDLER_COMPOSITION_TEST_RESULT",
        "task_id": "T1-M09-N003",
        "run_id": args.run_id,
        "status": "PASS",
        "test_pattern": TEST_PATTERN,
        "test_output_sha256": hashlib.sha256(logs.encode("utf-8")).hexdigest(),
        "kubernetes": evidence,
        "run_scoped_resources_removed": not args.keep,
        "production_applied": False,
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

