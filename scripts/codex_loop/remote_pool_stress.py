#!/usr/bin/env python3
"""Stress the HTTP queue service with multiple remote-style workers."""

from __future__ import annotations

import argparse
import os
import threading
import time
from collections import Counter, defaultdict
from concurrent.futures import ThreadPoolExecutor, as_completed
from datetime import datetime
from typing import Any
from urllib.parse import urlparse

from lib import ensure_run_dir, make_run_id, run_git, write_json, write_text
from queue_backend import claim_item, complete_item, display_path, enqueue_plan, fail_item, queue_status
from queue_service import make_server, request_json


def now_iso() -> str:
    return datetime.now().isoformat(timespec="seconds")


def task_id(prefix: str, index: int) -> str:
    return f"{prefix}-{index:03d}"


def stress_plan(run_id: str, prefix: str, tasks: int, max_retries: int) -> dict[str, Any]:
    selected = []
    for index in range(tasks):
        selected.append(
            {
                "id": task_id(prefix, index + 1),
                "title": f"Remote pool stress task {index + 1}",
                "priority": "P2",
                "mode": "plan",
                "data_mode": "synthetic",
                "lane_primary": "infra",
                "lane_dependent": [],
                "subsystems": ["codex_loop"],
                "acceptance_type": "smoke",
                "risk_level": "low",
                "resource_weight": 1,
                "resource_group": "remote-pool-stress",
                "score": max(1, 100 - index),
                "allowed_paths": ["scripts/codex_loop"],
                "path": "scripts/codex_loop/tasks/CLE-P0-SCREEN-001.yaml",
                "attempt": 1,
                "max_retries": max_retries,
            }
        )
    return {
        "run_id": f"{run_id}-remote-stress-seed",
        "run_kind": "scheduler",
        "status": "REMOTE_POOL_STRESS_SEED",
        "created_at": now_iso(),
        "queue": {"selected": selected, "deferred": []},
    }


def compact_response(response: dict[str, Any]) -> dict[str, Any]:
    return {
        key: value
        for key, value in response.items()
        if key in {"task_id", "worker_run_id", "claimed", "completed", "failed", "reason", "state", "http_status", "unreachable", "error", "backend"}
    }


def server_error(response: dict[str, Any]) -> bool:
    status = response.get("http_status")
    if response.get("unreachable"):
        return True
    if isinstance(status, int) and status >= 500:
        return True
    return bool(response.get("error")) and status is None


def is_loopback_url(value: str) -> bool:
    parsed = urlparse(value)
    return (parsed.hostname or "") in {"127.0.0.1", "localhost", "::1"}


def external_service_findings(service_url: str | None, allow_external_service: bool, token: str | None) -> list[dict[str, str]]:
    findings: list[dict[str, str]] = []
    if not service_url:
        return findings
    parsed = urlparse(service_url)
    if parsed.scheme not in {"http", "https"} or not parsed.netloc:
        findings.append({"level": "blocker", "code": "REMOTE_POOL_SERVICE_URL_INVALID", "message": "--service-url must be an http(s) base URL."})
        return findings
    if is_loopback_url(service_url):
        return findings
    if not allow_external_service and os.environ.get("CODEX_LOOP_ALLOW_REMOTE_POOL_STRESS") != "1":
        findings.append(
            {
                "level": "blocker",
                "code": "REMOTE_POOL_EXTERNAL_SERVICE_NOT_ALLOWED",
                "message": "Non-loopback queue service stress requires --allow-external-service or CODEX_LOOP_ALLOW_REMOTE_POOL_STRESS=1.",
            }
        )
    if not token:
        findings.append({"level": "blocker", "code": "REMOTE_POOL_EXTERNAL_TOKEN_REQUIRED", "message": "Non-loopback queue service stress requires CODEX_LOOP_QUEUE_TOKEN or --auth-token."})
    return findings


def worker_attempts(
    run_id: str,
    worker_index: int,
    task_ids: list[str],
    base_url: str,
    rounds: int,
    lease_seconds: int,
    completion_delay_ms: int,
) -> dict[str, Any]:
    worker_run_id = f"{run_id}-remote-worker-{worker_index + 1:02d}"
    claims: list[dict[str, Any]] = []
    completions: list[dict[str, Any]] = []
    for round_index in range(rounds):
        for item_id in task_ids:
            claim = claim_item(item_id, worker_run_id, lease_seconds, backend="http", path=base_url)
            claims.append({"round": round_index + 1, **compact_response(claim)})
            if not claim.get("claimed"):
                continue
            if completion_delay_ms > 0:
                time.sleep(completion_delay_ms / 1000)
            complete = complete_item(
                item_id,
                worker_run_id,
                {
                    "status": "remote_pool_stress_complete",
                    "round": round_index + 1,
                    "worker_run_id": worker_run_id,
                },
                backend="http",
                path=base_url,
            )
            completions.append({"round": round_index + 1, **compact_response(complete)})
    return {
        "worker_run_id": worker_run_id,
        "claims": claims,
        "completions": completions,
        "successful_claims": sum(1 for item in claims if item.get("claimed")),
        "successful_completions": sum(1 for item in completions if item.get("completed")),
    }


def lease_integrity_checks(run_id: str, base_url: str, item_id: str, lease_seconds: int) -> dict[str, Any]:
    owner = f"{run_id}-lease-owner"
    intruder = f"{run_id}-lease-intruder"
    claim = claim_item(item_id, owner, lease_seconds, backend="http", path=base_url)
    foreign_complete = complete_item(
        item_id,
        intruder,
        {"status": "foreign_complete_should_be_rejected"},
        backend="http",
        path=base_url,
    )
    foreign_fail = fail_item(
        item_id,
        intruder,
        {"status": "foreign_fail_should_be_rejected"},
        backend="http",
        path=base_url,
    )
    owner_complete = complete_item(
        item_id,
        owner,
        {"status": "lease_integrity_owner_complete"},
        backend="http",
        path=base_url,
    )
    return {
        "task_id": item_id,
        "owner": owner,
        "intruder": intruder,
        "claim": compact_response(claim),
        "foreign_complete": compact_response(foreign_complete),
        "foreign_fail": compact_response(foreign_fail),
        "owner_complete": compact_response(owner_complete),
        "passed": bool(claim.get("claimed"))
        and not bool(foreign_complete.get("completed"))
        and not bool(foreign_fail.get("failed"))
        and bool(owner_complete.get("completed")),
    }


def render_report(summary: dict[str, Any]) -> str:
    lines = [
        "# Codex Loop Remote Pool Stress",
        "",
        f"- run_id: `{summary['run_id']}`",
        f"- status: `{summary['status']}`",
        f"- run_kind: `{summary.get('run_kind')}`",
        f"- workers: `{summary['workers']}`",
        f"- tasks: `{summary['tasks']}`",
        f"- rounds: `{summary['rounds']}`",
        f"- backend: `{summary['queue_backend']}`",
        f"- service_mode: `{summary.get('service_mode')}`",
        f"- queue_path: `{summary.get('queue_path')}`",
        f"- target_counts: `{summary.get('target_counts')}`",
        f"- duplicate_successful_claims: `{summary['duplicate_successful_claims']}`",
        f"- successful_completions: `{summary['successful_completions']}`",
        "",
        "## Worker Results",
    ]
    for item in summary.get("workers_detail", []):
        lines.append(
            f"- `{item['worker_run_id']}` claims `{item['successful_claims']}` completions `{item['successful_completions']}`"
        )
    lines.extend(["", "## Lease Integrity"])
    integrity = summary.get("lease_integrity") or {}
    lines.append(f"- passed: `{integrity.get('passed')}` task: `{integrity.get('task_id')}`")
    lines.extend(["", "## Findings"])
    findings = summary.get("findings") or []
    if findings:
        for item in findings:
            lines.append(f"- `{item['level']}` `{item['code']}`: {item['message']}")
    else:
        lines.append("- none")
    lines.extend(
        [
            "",
            "## Guardrail",
            "- This stress validates remote queue arbitration over HTTP and only counts this run's synthetic tasks.",
            "- It does not execute product tasks, live writes, external Codex, or sandbox jobs.",
            "- External queue services require explicit authorization and are not stopped unless --stop-external-service is set.",
            "",
        ]
    )
    return "\n".join(lines)


def target_queue_counts(final_queue: dict[str, Any], task_ids: list[str]) -> dict[str, Any]:
    target_set = set(task_ids)
    counts: Counter[str] = Counter()
    seen: set[str] = set()
    for item in final_queue.get("items", []) or []:
        item_id = str(item.get("task_id"))
        if item_id not in target_set:
            continue
        seen.add(item_id)
        counts[str(item.get("state") or "unknown")] += 1
    if not final_queue.get("items") and target_set:
        return {"counts": {}, "missing": sorted(target_set), "seen": 0}
    return {"counts": dict(counts), "missing": sorted(target_set - seen), "seen": len(seen)}


def derive_findings(
    args: argparse.Namespace,
    enqueue: dict[str, Any],
    workers: list[dict[str, Any]],
    integrity: dict[str, Any],
    final_queue: dict[str, Any],
    target_counts: dict[str, Any],
) -> list[dict[str, str]]:
    findings: list[dict[str, str]] = []
    if args.tasks <= 0:
        return findings
    if not enqueue.get("counts"):
        findings.append({"level": "blocker", "code": "REMOTE_POOL_ENQUEUE_FAILED", "message": "Seed plan was not enqueued through HTTP queue backend."})
    if not integrity.get("passed"):
        findings.append({"level": "blocker", "code": "QUEUE_LEASE_OWNER_NOT_ENFORCED", "message": "Queue accepted a complete/fail from a worker that did not own the claim, or owner completion failed."})
    successful_claims_by_task: dict[str, list[str]] = defaultdict(list)
    completion_failures = []
    unreachable = []
    for worker in workers:
        for claim in worker.get("claims", []):
            if server_error(claim):
                unreachable.append(claim)
            if claim.get("claimed"):
                successful_claims_by_task[str(claim.get("task_id"))].append(str(worker.get("worker_run_id")))
        for complete in worker.get("completions", []):
            if server_error(complete):
                unreachable.append(complete)
            if not complete.get("completed"):
                completion_failures.append(complete)
    duplicates = {key: value for key, value in successful_claims_by_task.items() if len(value) > 1}
    if duplicates:
        findings.append({"level": "blocker", "code": "REMOTE_POOL_DUPLICATE_CLAIM", "message": f"Tasks claimed more than once: {sorted(duplicates)}."})
    if completion_failures:
        findings.append({"level": "blocker", "code": "REMOTE_POOL_COMPLETION_FAILED", "message": f"{len(completion_failures)} claimed tasks failed completion."})
    if unreachable:
        findings.append({"level": "blocker", "code": "REMOTE_POOL_HTTP_UNREACHABLE", "message": f"{len(unreachable)} queue HTTP calls were unreachable or server errors."})
    counts = target_counts.get("counts") or {}
    if int(counts.get("done") or 0) != args.tasks:
        findings.append({"level": "blocker", "code": "REMOTE_POOL_FINAL_COUNTS_UNEXPECTED", "message": f"Expected {args.tasks} done target tasks, got {counts}."})
    if target_counts.get("missing"):
        findings.append({"level": "blocker", "code": "REMOTE_POOL_TARGET_TASKS_MISSING", "message": f"Target tasks missing from final queue status: {target_counts.get('missing')}."})
    for state in ("claimed", "queued", "failed", "quarantined"):
        if int(counts.get(state) or 0) > 0:
            findings.append({"level": "blocker", "code": "REMOTE_POOL_QUEUE_NOT_DRAINED", "message": f"Final target queue still has {counts.get(state)} `{state}` tasks."})
    return findings


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--run-id", default=None)
    parser.add_argument("--host", default="127.0.0.1")
    parser.add_argument("--service-url", default=None)
    parser.add_argument("--allow-external-service", action="store_true")
    parser.add_argument("--stop-external-service", action="store_true")
    parser.add_argument("--worker-only", action="store_true")
    parser.add_argument("--worker-index", type=int, default=0)
    parser.add_argument("--workers", type=int, default=4)
    parser.add_argument("--tasks", type=int, default=8)
    parser.add_argument("--rounds", type=int, default=3)
    parser.add_argument("--lease-seconds", type=int, default=60)
    parser.add_argument("--completion-delay-ms", type=int, default=0)
    parser.add_argument("--task-prefix", default=None)
    parser.add_argument("--max-retries", type=int, default=3)
    parser.add_argument("--queue-path", default=None)
    parser.add_argument("--auth-token", default=None)
    args = parser.parse_args()

    run_id = args.run_id or make_run_id("remote-pool-stress")
    run_dir = ensure_run_dir(run_id)
    out_dir = run_dir / "remote-pool-stress"
    out_dir.mkdir(parents=True, exist_ok=True)
    task_prefix = args.task_prefix or f"REMOTE-POOL-STRESS-{run_id}"
    queue_path = args.queue_path or f"doc/02_acceptance/runs/{run_id}/remote-pool-stress/queue.sqlite3"
    old_token = os.environ.get("CODEX_LOOP_QUEUE_TOKEN")
    token = args.auth_token or old_token
    service_mode = "external" if args.service_url else "embedded-loopback"
    base_url = str(args.service_url or "").rstrip("/")
    server = None
    thread = None
    guard_findings = external_service_findings(args.service_url, args.allow_external_service, token)
    if not guard_findings and not args.service_url:
        token = args.auth_token or old_token or f"{run_id}-remote-pool-token"
        server = make_server(args.host, 0, "sqlite", queue_path, token, quiet=True)
        host, port = server.server_address[:2]
        base_url = f"http://{host}:{port}"
        thread = threading.Thread(target=server.serve_forever, kwargs={"poll_interval": 0.1}, daemon=True)
    task_ids = [task_id(task_prefix, index + 1) for index in range(args.tasks)]
    enqueue: dict[str, Any] = {}
    integrity: dict[str, Any] = {}
    workers: list[dict[str, Any]] = []
    final_queue: dict[str, Any] = {}
    stop_response: dict[str, Any] = {}
    if guard_findings:
        summary = {
            "run_id": run_id,
            "run_kind": "remote_pool_stress",
            "status": "REMOTE_POOL_STRESS_BLOCKED",
            "created_at": now_iso(),
            "commit": run_git(["rev-parse", "HEAD"]).strip(),
            "queue_backend": "http",
            "service_backend": "external-http" if args.service_url else "sqlite",
            "service_mode": service_mode,
            "service_url": base_url or args.service_url,
            "queue_path": display_path(args.service_url or queue_path),
            "task_prefix": task_prefix,
            "workers": max(1, args.workers),
            "tasks": args.tasks,
            "rounds": max(1, args.rounds),
            "lease_seconds": args.lease_seconds,
            "completion_delay_ms": args.completion_delay_ms,
            "enqueue": enqueue,
            "lease_integrity": integrity,
            "workers_detail": workers,
            "target_counts": {"counts": {}, "missing": task_ids, "seen": 0},
            "duplicate_successful_claims": 0,
            "successful_completions": 0,
            "final_queue": final_queue,
            "stop_response": stop_response,
            "findings": guard_findings,
            "outputs": [
                "remote-pool-stress/stress-summary.json",
                "remote-pool-stress/stress-report.md",
                "remote-pool-stress/worker-results.json",
                "remote-pool-stress/http-responses.json",
            ],
        }
        write_json(out_dir / "stress-summary.json", summary)
        write_text(out_dir / "stress-report.md", render_report(summary))
        write_json(out_dir / "worker-results.json", {"workers": workers})
        write_json(out_dir / "http-responses.json", {"enqueue": enqueue, "lease_integrity": integrity, "final_queue": final_queue, "stop_response": stop_response})
        write_json(run_dir / "run-summary.json", summary)
        print(out_dir)
        print(f"status=REMOTE_POOL_STRESS_BLOCKED tasks={args.tasks} workers={max(1, args.workers)} findings={len(guard_findings)}")
        return 2
    if token:
        os.environ["CODEX_LOOP_QUEUE_TOKEN"] = token
    if args.worker_only:
        if thread:
            thread.start()
            time.sleep(0.05)
        try:
            if not base_url:
                guard_findings.append({"level": "blocker", "code": "REMOTE_POOL_WORKER_SERVICE_URL_REQUIRED", "message": "--worker-only requires --service-url or embedded service."})
            worker_result = {}
            if not guard_findings:
                worker_result = worker_attempts(
                    run_id,
                    args.worker_index,
                    task_ids,
                    base_url,
                    max(1, args.rounds),
                    args.lease_seconds,
                    args.completion_delay_ms,
                )
            status = "REMOTE_POOL_WORKER_COMPLETED" if not guard_findings else "REMOTE_POOL_WORKER_BLOCKED"
            summary = {
                "run_id": run_id,
                "run_kind": "remote_pool_worker",
                "status": status,
                "created_at": now_iso(),
                "commit": run_git(["rev-parse", "HEAD"]).strip(),
                "queue_backend": "http",
                "service_backend": "external-http" if service_mode == "external" else "sqlite",
                "service_mode": service_mode,
                "service_url": base_url,
                "queue_path": display_path(args.service_url or queue_path),
                "task_prefix": task_prefix,
                "workers": 1,
                "worker_index": args.worker_index,
                "tasks": args.tasks,
                "rounds": max(1, args.rounds),
                "lease_seconds": args.lease_seconds,
                "completion_delay_ms": args.completion_delay_ms,
                "enqueue": {},
                "lease_integrity": {},
                "workers_detail": [worker_result] if worker_result else [],
                "target_counts": None,
                "duplicate_successful_claims": None,
                "successful_completions": worker_result.get("successful_completions") if worker_result else 0,
                "final_queue": {},
                "stop_response": {"skipped": True, "reason": "worker-only mode"},
                "findings": guard_findings,
                "outputs": [
                    "remote-pool-stress/stress-summary.json",
                    "remote-pool-stress/stress-report.md",
                    "remote-pool-stress/worker-results.json",
                    "remote-pool-stress/http-responses.json",
                ],
            }
            write_json(out_dir / "stress-summary.json", summary)
            write_text(out_dir / "stress-report.md", render_report(summary))
            write_json(out_dir / "worker-results.json", {"workers": summary["workers_detail"]})
            write_json(out_dir / "http-responses.json", {"worker_only": worker_result})
            write_json(run_dir / "run-summary.json", summary)
            print(out_dir)
            print(f"status={status} tasks={args.tasks} worker_index={args.worker_index} findings={len(guard_findings)}")
            return 0 if status == "REMOTE_POOL_WORKER_COMPLETED" else 2
        finally:
            if thread:
                thread.join(timeout=5)
            if thread and thread.is_alive() and server:
                server.shutdown()
                thread.join(timeout=5)
            if server:
                server.server_close()
            if old_token is None:
                os.environ.pop("CODEX_LOOP_QUEUE_TOKEN", None)
            else:
                os.environ["CODEX_LOOP_QUEUE_TOKEN"] = old_token
    if thread:
        thread.start()
        time.sleep(0.05)
    try:
        enqueue = enqueue_plan(stress_plan(run_id, task_prefix, args.tasks, args.max_retries), backend="http", path=base_url)
        if task_ids:
            integrity = lease_integrity_checks(run_id, base_url, task_ids[0], args.lease_seconds)
        remaining_task_ids = task_ids[1:] if task_ids else []
        if remaining_task_ids:
            with ThreadPoolExecutor(max_workers=max(1, args.workers)) as pool:
                futures = [
                    pool.submit(
                        worker_attempts,
                        run_id,
                        worker_index,
                        remaining_task_ids,
                        base_url,
                        max(1, args.rounds),
                        args.lease_seconds,
                        args.completion_delay_ms,
                    )
                    for worker_index in range(max(1, args.workers))
                ]
                for future in as_completed(futures):
                    workers.append(future.result())
            workers.sort(key=lambda item: str(item.get("worker_run_id")))
        final_queue = queue_status(backend="http", path=base_url, include_items=True)
        if service_mode == "embedded-loopback" or args.stop_external_service:
            stop_response = request_json(base_url, "POST", "/v1/service/stop", token, {})
        else:
            stop_response = {"skipped": True, "reason": "external service not stopped by default"}
    finally:
        if thread:
            thread.join(timeout=5)
        if thread and thread.is_alive() and server:
            server.shutdown()
            thread.join(timeout=5)
        if server:
            server.server_close()
        if old_token is None:
            os.environ.pop("CODEX_LOOP_QUEUE_TOKEN", None)
        else:
            os.environ["CODEX_LOOP_QUEUE_TOKEN"] = old_token

    target_counts = target_queue_counts(final_queue, task_ids)
    findings = derive_findings(args, enqueue, workers, integrity, final_queue, target_counts)
    claim_counter = Counter()
    for worker in workers:
        for claim in worker.get("claims", []):
            if claim.get("claimed"):
                claim_counter[str(claim.get("task_id"))] += 1
    duplicate_successful_claims = sum(1 for count in claim_counter.values() if count > 1)
    successful_completions = sum(int(worker.get("successful_completions") or 0) for worker in workers)
    if integrity.get("owner_complete", {}).get("completed"):
        successful_completions += 1
    status = "REMOTE_POOL_STRESS_EMPTY" if args.tasks <= 0 else "REMOTE_POOL_STRESS_COMPLETED"
    if any(item["level"] == "blocker" for item in findings):
        status = "REMOTE_POOL_STRESS_BLOCKED"
    summary = {
        "run_id": run_id,
        "run_kind": "remote_pool_stress",
        "status": status,
        "created_at": now_iso(),
        "commit": run_git(["rev-parse", "HEAD"]).strip(),
        "queue_backend": "http",
        "service_backend": "external-http" if service_mode == "external" else "sqlite",
        "service_mode": service_mode,
        "service_url": base_url,
        "queue_path": display_path(args.service_url or queue_path),
        "task_prefix": task_prefix,
        "workers": max(1, args.workers),
        "tasks": args.tasks,
        "rounds": max(1, args.rounds),
        "lease_seconds": args.lease_seconds,
        "completion_delay_ms": args.completion_delay_ms,
        "enqueue": enqueue,
        "lease_integrity": integrity,
        "workers_detail": workers,
        "target_counts": target_counts,
        "duplicate_successful_claims": duplicate_successful_claims,
        "successful_completions": successful_completions,
        "final_queue": final_queue,
        "stop_response": stop_response,
        "findings": findings,
        "outputs": [
            "remote-pool-stress/stress-summary.json",
            "remote-pool-stress/stress-report.md",
            "remote-pool-stress/worker-results.json",
            "remote-pool-stress/http-responses.json",
        ],
    }
    write_json(out_dir / "stress-summary.json", summary)
    write_text(out_dir / "stress-report.md", render_report(summary))
    write_json(out_dir / "worker-results.json", {"workers": workers})
    write_json(out_dir / "http-responses.json", {"enqueue": enqueue, "lease_integrity": integrity, "final_queue": final_queue, "stop_response": stop_response})
    write_json(run_dir / "run-summary.json", summary)
    print(out_dir)
    print(f"status={status} tasks={args.tasks} workers={max(1, args.workers)} findings={len(findings)}")
    return 0 if status in {"REMOTE_POOL_STRESS_COMPLETED", "REMOTE_POOL_STRESS_EMPTY"} else 2


if __name__ == "__main__":
    raise SystemExit(main())
