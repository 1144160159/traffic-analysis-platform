#!/usr/bin/env python3
"""Read-only G2/G3 verifier for the nine isolated Flink Application Clusters."""

from __future__ import annotations

import argparse
import hashlib
import json
import urllib.parse
import urllib.request
from pathlib import Path
from typing import Any, Callable


ROOT = Path(__file__).resolve().parents[2]
CONTRACT_PATH = ROOT / "contracts/flink/application-cluster-migration.v1.json"


def _contract() -> dict[str, Any]:
    return json.loads(CONTRACT_PATH.read_text(encoding="utf-8"))


def _digest(value: Any) -> str:
    payload = json.dumps(value, ensure_ascii=False, sort_keys=True, separators=(",", ":"))
    return hashlib.sha256(payload.encode("utf-8")).hexdigest()


def get_json(endpoint: str, path: str) -> dict[str, Any]:
    parsed = urllib.parse.urlparse(endpoint)
    if parsed.scheme not in {"http", "https"} or not parsed.hostname:
        raise ValueError(f"unsafe Flink REST endpoint: {endpoint}")
    opener = urllib.request.build_opener(urllib.request.ProxyHandler({}))
    with opener.open(f"{endpoint.rstrip('/')}{path}", timeout=10) as response:
        return json.load(response)


def verify(
    endpoints_manifest: dict[str, Any],
    fetch: Callable[[str, str], dict[str, Any]] = get_json,
) -> dict[str, Any]:
    contract = _contract()
    jobs = {job["id"]: job for job in contract["jobs"]}
    errors: list[str] = []
    if endpoints_manifest.get("schema_version") != 1:
        errors.append("endpoints manifest must use schema_version 1")
    if endpoints_manifest.get("contract_sha256") != _digest(contract):
        errors.append("endpoints manifest contract digest does not match repository contract")
    completed_order = endpoints_manifest.get("completed_migration_order")
    if not isinstance(completed_order, int) or not 1 <= completed_order <= 9:
        errors.append("completed_migration_order must be an integer in 1..9")
        completed_order = 0
    expected_jobs = {
        job_id: job for job_id, job in jobs.items()
        if job["migration_order"] <= completed_order
    }
    endpoints = endpoints_manifest.get("endpoints") or {}
    if set(endpoints) != set(expected_jobs):
        errors.append(
            f"endpoint job ids differ: missing={sorted(set(expected_jobs)-set(endpoints))}, "
            f"extra={sorted(set(endpoints)-set(expected_jobs))}"
        )

    results: list[dict[str, Any]] = []
    for job in sorted(contract["jobs"], key=lambda item: item["migration_order"]):
        if job["migration_order"] > completed_order:
            continue
        job_id = job["id"]
        entry = endpoints.get(job_id)
        if not isinstance(entry, dict):
            continue
        endpoint = str(entry.get("endpoint", ""))
        if entry.get("cluster_id") != job["cluster_id"]:
            errors.append(f"{job_id}: cluster_id differs from contract")
        try:
            overview = fetch(endpoint, "/jobs/overview")
        except Exception as exc:  # runtime evidence must retain the endpoint error
            errors.append(f"{job_id}: REST overview failed: {exc}")
            continue
        active = [item for item in overview.get("jobs", []) if item.get("state") == "RUNNING"]
        if len(active) != 1:
            errors.append(f"{job_id}: expected exactly one RUNNING job, found {len(active)}")
            continue
        observed = active[0]
        if observed.get("name") != job["job_name"]:
            errors.append(f"{job_id}: canonical job name mismatch: {observed.get('name')!r}")
        tasks = observed.get("tasks") or {}
        running = tasks.get("running")
        total = tasks.get("total")
        if running != job["expected_tasks"] or total != job["expected_tasks"]:
            errors.append(
                f"{job_id}: expected {job['expected_tasks']}/{job['expected_tasks']} tasks, "
                f"found {running}/{total}"
            )
        jid = str(observed.get("jid", ""))
        try:
            checkpoints = fetch(endpoint, f"/jobs/{jid}/checkpoints")
            exceptions = fetch(endpoint, f"/jobs/{jid}/exceptions")
        except Exception as exc:
            errors.append(f"{job_id}: checkpoint/exception evidence failed: {exc}")
            continue
        latest = (checkpoints.get("latest") or {}).get("completed")
        restored = int((checkpoints.get("counts") or {}).get("restored", 0))
        root_exception = exceptions.get("root_exception")
        checkpoint_prefix = f"s3://flink-checkpoints/checkpoints/application-clusters/{job_id}/"
        checkpoint_path = str((latest or {}).get("external_path", ""))
        if restored < 1:
            errors.append(f"{job_id}: no restore recorded after savepoint migration")
        if not latest or not checkpoint_path.startswith(checkpoint_prefix):
            errors.append(f"{job_id}: no new completed checkpoint under {checkpoint_prefix}")
        if root_exception:
            errors.append(f"{job_id}: root exception is present")
        results.append(
            {
                "job_id": job_id,
                "cluster_id": job["cluster_id"],
                "endpoint": endpoint,
                "jid": jid,
                "job_name": observed.get("name"),
                "running_tasks": running,
                "total_tasks": total,
                "restored": restored,
                "latest_checkpoint_id": (latest or {}).get("id"),
                "latest_checkpoint_path": checkpoint_path or None,
                "root_exception": root_exception,
            }
        )
    return {
        "schema_version": 1,
        "contract_sha256": _digest(contract),
        "status": "PASS" if not errors else "FAIL",
        "completed_migration_order": completed_order,
        "expected_application_clusters": len(expected_jobs),
        "verified_application_clusters": len(results),
        "expected_tasks": sum(job["expected_tasks"] for job in expected_jobs.values()),
        "verified_running_tasks": sum(int(item.get("running_tasks") or 0) for item in results),
        "clusters": results,
        "errors": errors,
    }


def main() -> int:
    parser = argparse.ArgumentParser()
    parser.add_argument("--endpoints-manifest", type=Path, required=True)
    parser.add_argument("--output", type=Path)
    args = parser.parse_args()
    source = json.loads(args.endpoints_manifest.read_text(encoding="utf-8"))
    result = verify(source)
    payload = json.dumps(result, ensure_ascii=False, indent=2) + "\n"
    if args.output:
        args.output.write_text(payload, encoding="utf-8")
    else:
        print(payload, end="")
    return 0 if result["status"] == "PASS" else 1


if __name__ == "__main__":
    raise SystemExit(main())
