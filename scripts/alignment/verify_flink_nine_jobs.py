#!/usr/bin/env python3
"""Read-only gate for the canonical nine-job Flink production topology."""

from __future__ import annotations

import argparse
import json
import urllib.request
from typing import Any


EXPECTED_TASKS = {
    "Alert Generator Job": 16,
    "Behavior Detection Job": 12,
    "CEP Correlation Job": 32,
    "Device Log Job": 2,
    "Feature Extraction Job v3 (Full Enhanced)": 18,
    "PCAP Index Job v2": 2,
    "Rule Engine Job (Enhanced)": 12,
    "Session Aggregation Job V2": 24,
    "User Behavior Anomaly Detection Job": 10,
}


def get_json(endpoint: str, path: str) -> dict[str, Any]:
    opener = urllib.request.build_opener(urllib.request.ProxyHandler({}))
    with opener.open(f"{endpoint.rstrip('/')}{path}", timeout=10) as response:
        return json.load(response)


def verify(endpoint: str) -> dict[str, Any]:
    overview = get_json(endpoint, "/jobs/overview")
    active = [job for job in overview["jobs"] if job["state"] == "RUNNING"]
    by_name: dict[str, list[dict[str, Any]]] = {}
    for job in active:
        by_name.setdefault(job["name"], []).append(job)

    errors: list[str] = []
    if len(active) != len(EXPECTED_TASKS):
        errors.append(f"expected 9 active jobs, found {len(active)}")

    results = []
    for name, expected_tasks in EXPECTED_TASKS.items():
        matches = by_name.get(name, [])
        if len(matches) != 1:
            errors.append(f"{name}: expected one active job, found {len(matches)}")
            continue
        job = matches[0]
        running = job["tasks"]["running"]
        total = job["tasks"]["total"]
        if running != expected_tasks or total != expected_tasks:
            errors.append(
                f"{name}: expected {expected_tasks}/{expected_tasks} tasks, "
                f"found {running}/{total}"
            )
        checkpoints = get_json(endpoint, f"/jobs/{job['jid']}/checkpoints")
        exceptions = get_json(endpoint, f"/jobs/{job['jid']}/exceptions")
        latest = checkpoints.get("latest", {}).get("completed")
        root_exception = exceptions.get("root_exception")
        if not latest:
            errors.append(f"{name}: no completed checkpoint after recovery")
        if root_exception:
            errors.append(f"{name}: root exception is present")
        results.append(
            {
                "jid": job["jid"],
                "name": name,
                "running_tasks": running,
                "total_tasks": total,
                "restored_checkpoints": checkpoints["counts"].get("restored", 0),
                "latest_completed_checkpoint": latest["id"] if latest else None,
                "latest_checkpoint_path": latest["external_path"] if latest else None,
                "root_exception": root_exception,
            }
        )

    unexpected = sorted(set(by_name) - set(EXPECTED_TASKS))
    if unexpected:
        errors.append(f"unexpected active jobs: {unexpected}")
    return {
        "schema_version": 1,
        "endpoint": endpoint,
        "status": "PASS" if not errors else "FAIL",
        "expected_active_jobs": len(EXPECTED_TASKS),
        "actual_active_jobs": len(active),
        "jobs": sorted(results, key=lambda item: item["name"]),
        "errors": errors,
    }


def main() -> int:
    parser = argparse.ArgumentParser()
    parser.add_argument(
        "--endpoint",
        default="http://flink-jobmanager.flink.svc:8081",
        help="Flink REST endpoint",
    )
    parser.add_argument("--output", help="optional JSON output path")
    args = parser.parse_args()
    result = verify(args.endpoint)
    payload = json.dumps(result, ensure_ascii=False, indent=2) + "\n"
    if args.output:
        with open(args.output, "w", encoding="utf-8") as target:
            target.write(payload)
    else:
        print(payload, end="")
    return 0 if result["status"] == "PASS" else 1


if __name__ == "__main__":
    raise SystemExit(main())
