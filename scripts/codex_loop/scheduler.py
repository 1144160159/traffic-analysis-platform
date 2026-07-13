#!/usr/bin/env python3
"""Create a guarded task queue with workspace locks and retry accounting."""

from __future__ import annotations

import argparse
import json
from datetime import datetime
from pathlib import Path
from typing import Any

from lib import ensure_run_dir, list_of, load_yaml_subset, make_run_id, rel_path, repo_path, run_git, write_json, write_text
from lock_manager import acquire_lock
from queue_backend import display_path, enqueue_plan
from resource_quota import DEFAULT_POLICY as DEFAULT_QUOTA_POLICY
from resource_quota import load_policy as load_quota_policy
from resource_quota import new_usage, quota_findings, record_item, usage_snapshot


MAX_DEFAULT_RETRIES = 3
RETRY_CONSUMING_STATUSES = {
    "WORKFLOW_FAILED",
    "IMPLEMENTATION_BLOCKED",
    "PATCH_REJECTED",
    "LOCAL_FAILED",
    "REPAIRING",
}


def load_json(path: Path | None) -> dict[str, Any]:
    if not path or not path.exists():
        return {}
    return json.loads(path.read_text(encoding="utf-8"))


def load_tasks(tasks_dir: Path) -> dict[str, dict[str, Any]]:
    tasks: dict[str, dict[str, Any]] = {}
    for path in sorted(tasks_dir.glob("*.yaml")):
        task = load_yaml_subset(path)
        task["_path"] = rel_path(path)
        tasks[str(task.get("id"))] = task
    return tasks


def paths_overlap(left: list[str], right: list[str]) -> bool:
    for a in left:
        a = a.rstrip("/")
        for b in right:
            b = b.rstrip("/")
            if a == b or a.startswith(f"{b}/") or b.startswith(f"{a}/"):
                return True
    return False


def run_attempts(evidence: dict[str, Any], task_id: str) -> int:
    return sum(
        1
        for run in evidence.get("runs", [])
        if run.get("task_id") == task_id and str(run.get("status")) in RETRY_CONSUMING_STATUSES
    )


def task_queue_item(task: dict[str, Any], recommendation: dict[str, Any], evidence: dict[str, Any]) -> dict[str, Any]:
    task_id = str(task.get("id"))
    attempts = run_attempts(evidence, task_id)
    lane = task.get("lane") or {}
    execution = task.get("execution") or {}
    data_plan = task.get("data_plan") or {}
    return {
        "id": task_id,
        "title": task.get("title"),
        "priority": task.get("priority"),
        "status": task.get("status"),
        "score": recommendation.get("score"),
        "mode": execution.get("mode"),
        "data_mode": data_plan.get("mode"),
        "lane_primary": lane.get("primary"),
        "lane_dependent": list_of(lane.get("dependent")),
        "subsystems": list_of(task.get("subsystems")),
        "acceptance_type": task.get("acceptance_type"),
        "risk_level": (task.get("risk") or {}).get("level"),
        "allowed_paths": list_of((task.get("workspace") or {}).get("allowed_paths")),
        "attempt": attempts + 1,
        "max_retries": MAX_DEFAULT_RETRIES,
        "path": task.get("_path"),
    }


def build_queue(
    guidance: dict[str, Any],
    tasks: dict[str, dict[str, Any]],
    evidence: dict[str, Any],
    max_items: int,
    quota_policy: dict[str, Any],
    enforce_quota: bool,
) -> dict[str, Any]:
    selected: list[dict[str, Any]] = []
    deferred: list[dict[str, str]] = []
    quota_deferred: list[dict[str, Any]] = []
    usage = new_usage()
    for recommendation in guidance.get("recommended_next", []):
        task_id = str(recommendation.get("id"))
        task = tasks.get(task_id)
        if not task:
            continue
        item = task_queue_item(task, recommendation, evidence)
        if item["attempt"] > item["max_retries"]:
            deferred.append({"id": task_id, "reason": "retry budget exhausted"})
            continue
        conflict = next((current for current in selected if paths_overlap(item["allowed_paths"], current["allowed_paths"])), None)
        if conflict:
            deferred.append({"id": task_id, "reason": f"workspace path overlaps {conflict['id']}"})
            continue
        if enforce_quota:
            findings = quota_findings(item, usage, quota_policy)
            if findings:
                first = findings[0]
                quota_deferred.append({"id": task_id, "reason": str(first.get("message")), "quota_code": str(first.get("code")), "findings": findings})
                deferred.append({"id": task_id, "reason": str(first.get("message"))})
                continue
            item = record_item(item, usage, quota_policy)
        selected.append(item)
        if len(selected) >= max_items:
            break
    return {
        "selected": selected,
        "deferred": deferred,
        "quota": {
            "enabled": enforce_quota,
            "policy": quota_policy.get("_path"),
            "usage": usage_snapshot(usage),
            "deferred": quota_deferred,
        },
    }


def render_queue(plan: dict[str, Any]) -> str:
    lines = [
        "# Codex Loop Scheduler Queue",
        "",
        f"- run_id: `{plan['run_id']}`",
        f"- status: `{plan['status']}`",
        f"- selected: `{len(plan['queue']['selected'])}`",
        f"- deferred: `{len(plan['queue']['deferred'])}`",
        "",
        "## Selected",
    ]
    if plan["queue"]["selected"]:
        for item in plan["queue"]["selected"]:
            lines.append(f"- `{item['id']}` score `{item['score']}` attempt `{item['attempt']}/{item['max_retries']}` paths: {', '.join(item['allowed_paths'])}")
    else:
        lines.append("- none")
    lines.extend(["", "## Deferred"])
    if plan["queue"]["deferred"]:
        for item in plan["queue"]["deferred"]:
            lines.append(f"- `{item['id']}`: {item['reason']}")
    else:
        lines.append("- none")
    quota = plan.get("quota") or plan.get("queue", {}).get("quota") or {}
    lines.extend(["", "## Resource Quota"])
    lines.append(f"- enabled: `{quota.get('enabled')}`")
    lines.append(f"- policy: `{quota.get('policy')}`")
    lines.append(f"- usage: `{(quota.get('usage') or {})}`")
    if quota.get("deferred"):
        for item in quota["deferred"]:
            lines.append(f"- `{item.get('id')}` quota deferred: {item.get('reason')}")
    else:
        lines.append("- quota deferred: none")
    lines.extend(["", "## Lock"])
    lock = plan.get("lock")
    if lock:
        lines.append(f"- acquired: `{lock.get('acquired')}`")
        lines.append(f"- path: `{lock.get('path')}`")
    else:
        lines.append("- not requested")
    lines.extend(["", "## Persistent Queue"])
    persistent_queue = plan.get("persistent_queue")
    if persistent_queue:
        counts = persistent_queue.get("counts") or {}
        lines.append(f"- path: `{persistent_queue.get('path')}`")
        lines.append(f"- enqueued: `{len(persistent_queue.get('enqueued') or [])}`")
        lines.append(f"- updated: `{len(persistent_queue.get('updated') or [])}`")
        lines.append(f"- skipped: `{len(persistent_queue.get('skipped') or [])}`")
        lines.append(f"- counts: `{counts}`")
    else:
        lines.append("- not requested")
    lines.extend(
        [
            "",
            "## Guardrail",
            "- The scheduler writes queue intent only; it does not run workflow.py by itself.",
            "- One workspace lock protects code-modifying tasks from overlapping in this MVP.",
            "",
        ]
    )
    return "\n".join(lines)


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--guidance", required=True)
    parser.add_argument("--context-dir", default=None)
    parser.add_argument("--tasks-dir", default="scripts/codex_loop/tasks")
    parser.add_argument("--max-items", type=int, default=3)
    parser.add_argument("--acquire-lock", action="store_true")
    parser.add_argument("--lease-seconds", type=int, default=3600)
    parser.add_argument("--persist-queue", action="store_true")
    parser.add_argument("--queue-backend", choices=["repo-json", "sqlite", "http"], default="repo-json")
    parser.add_argument("--queue-path", default=None)
    parser.add_argument("--quota-policy", default=str(DEFAULT_QUOTA_POLICY))
    parser.add_argument("--skip-quota", action="store_true")
    parser.add_argument("--run-id", default=None)
    args = parser.parse_args()

    run_id = args.run_id or make_run_id("scheduler")
    run_dir = ensure_run_dir(run_id)
    out_dir = run_dir / "scheduler"
    out_dir.mkdir(parents=True, exist_ok=True)
    guidance_path = repo_path(args.guidance)
    guidance = load_json(guidance_path)
    context_dir = repo_path(args.context_dir) if args.context_dir else guidance_path.parent.parent / "context"
    evidence = load_json(context_dir / "evidence-ledger.json")
    tasks = load_tasks(repo_path(args.tasks_dir))
    quota_policy = load_quota_policy(args.quota_policy)
    queue = build_queue(guidance, tasks, evidence, args.max_items, quota_policy, not args.skip_quota)
    lock = None
    if args.acquire_lock and queue["selected"]:
        lock = acquire_lock(run_id, str(queue["selected"][0]["id"]), args.lease_seconds)
    status = "LOCK_ACQUIRED" if lock and lock.get("acquired") else "SCHEDULER_PLANNED"
    if lock and not lock.get("acquired"):
        status = "LOCK_BLOCKED"
    plan = {
        "run_id": run_id,
        "run_kind": "scheduler",
        "status": status,
        "created_at": datetime.now().isoformat(timespec="seconds"),
        "commit": run_git(["rev-parse", "HEAD"]).strip(),
        "guidance": rel_path(guidance_path),
        "context_dir": rel_path(context_dir),
        "queue": queue,
        "quota": queue.get("quota"),
        "lock": lock,
        "lease_seconds": args.lease_seconds if args.acquire_lock else None,
        "queue_backend": args.queue_backend,
        "queue_path": display_path(args.queue_path),
        "persistent_queue": None,
        "outputs": ["scheduler/scheduler-plan.json", "scheduler/queue.md"],
    }
    if args.persist_queue:
        plan["persistent_queue"] = enqueue_plan(plan, backend=args.queue_backend, path=args.queue_path)
        if args.queue_backend == "sqlite":
            plan["outputs"].append("../.loop/queue.sqlite3")
        else:
            plan["outputs"].extend(["../.loop/queue.json", "../.loop/queue-events.jsonl"])
    write_json(out_dir / "scheduler-plan.json", plan)
    write_text(out_dir / "queue.md", render_queue(plan))
    write_json(run_dir / "run-summary.json", plan)
    print(out_dir)
    print(f"status={status} selected={len(queue['selected'])} deferred={len(queue['deferred'])}")
    return 2 if status == "LOCK_BLOCKED" else 0


if __name__ == "__main__":
    raise SystemExit(main())
