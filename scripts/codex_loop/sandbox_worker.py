#!/usr/bin/env python3
"""Bridge scheduler queue items into sandbox plan/execution runs."""

from __future__ import annotations

import argparse
import json
import subprocess
import sys
from datetime import datetime
from pathlib import Path
from typing import Any

from lib import RUNS_ROOT, SCRIPT_ROOT, ensure_run_dir, make_run_id, rel_path, repo_path, run_git, write_json, write_text
from lock_manager import heartbeat_lock, lock_expired, read_lock
from queue_backend import claim_item, complete_item, display_path, fail_item


def load_json(path: Path) -> dict[str, Any]:
    return json.loads(path.read_text(encoding="utf-8"))


def safe_slug(value: str) -> str:
    return "".join(ch.lower() if ch.isalnum() else "-" for ch in value).strip("-") or "task"


def run_script(name: str, args: list[str]) -> dict[str, Any]:
    command = [sys.executable, "-B", str(SCRIPT_ROOT / name), *args]
    proc = subprocess.run(
        command,
        cwd=repo_path("."),
        text=True,
        stdout=subprocess.PIPE,
        stderr=subprocess.STDOUT,
        check=False,
    )
    return {
        "name": name,
        "command": command,
        "exit_code": proc.returncode,
        "output_tail": proc.stdout[-4000:],
    }


def read_child_summary(run_id: str) -> dict[str, Any] | None:
    path = RUNS_ROOT / run_id / "run-summary.json"
    if not path.exists():
        return None
    return load_json(path)


def check_lock(plan: dict[str, Any], require_lock: bool) -> str:
    lock = plan.get("lock")
    if not require_lock and not lock:
        return "not_required"
    current = read_lock()
    expected_run = str(lock.get("payload", {}).get("run_id")) if lock else None
    if not current:
        return "missing"
    if lock_expired(current):
        return "expired"
    if expected_run and str(current.get("run_id")) != expected_run:
        return "mismatch"
    lease_seconds = int(plan.get("lease_seconds") or current.get("lease_seconds") or 3600)
    heartbeat_lock(str(current.get("run_id")), lease_seconds)
    return "valid"


def select_items(plan: dict[str, Any], max_tasks: int, lock_status: str, require_lock: bool) -> list[dict[str, Any]]:
    if (require_lock or plan.get("lock")) and lock_status != "valid":
        return []
    return list(plan.get("queue", {}).get("selected", []))[:max_tasks]


def render_report(summary: dict[str, Any]) -> str:
    lines = [
        "# Codex Loop Sandbox Worker Report",
        "",
        f"- run_id: `{summary['run_id']}`",
        f"- status: `{summary['status']}`",
        f"- stage: `{summary['stage']}`",
        f"- driver: `{summary['driver']}`",
        f"- execute_sandbox: `{summary['execute_sandbox']}`",
        f"- claim_queue: `{summary['claim_queue']}`",
        f"- lock_status: `{summary['lock_status']}`",
        f"- selected: `{summary['selected_count']}`",
        "",
        "## Sandbox Plans",
    ]
    if summary.get("plans"):
        for item in summary["plans"]:
            plan_status = (item.get("summary") or {}).get("status")
            workspace = ((item.get("workspace_isolation") or {}).get("workspace_path")) or "source-worktree"
            lines.append(f"- `{item['task_id']}` -> `{item['run_id']}` exit `{item['exit_code']}` status `{plan_status}` workspace `{workspace}`")
    else:
        lines.append("- none")
    lines.extend(["", "## Sandbox Executions"])
    if summary.get("executions"):
        for item in summary["executions"]:
            execution_status = (item.get("summary") or {}).get("status")
            lines.append(f"- `{item['task_id']}` -> `{item['run_id']}` exit `{item['exit_code']}` status `{execution_status}`")
    else:
        lines.append("- none")
    lines.extend(["", "## Queue"])
    if summary.get("queue_claims"):
        for item in summary["queue_claims"]:
            status = "claimed" if item.get("claimed") else f"skipped: {item.get('reason')}"
            lines.append(f"- `{item.get('task_id')}` {status}")
    else:
        lines.append("- not used")
    if summary.get("queue_updates"):
        lines.extend(["", "## Queue Updates"])
        for item in summary["queue_updates"]:
            task_id = item.get("task_id")
            state = item.get("state") or item.get("reason") or item
            lines.append(f"- `{task_id}` -> `{state}`")
    lines.extend(
        [
            "",
            "## Guardrail",
            "- Plan-only mode does not claim or mutate the persistent queue.",
            "- Queue mutation requires --claim-queue and --execute-sandbox.",
            "- Real sandbox execution is still gated by sandbox_executor.py and its environment policy.",
            "",
        ]
    )
    return "\n".join(lines)


def plan_task(args: argparse.Namespace, item: dict[str, Any], run_id: str) -> dict[str, Any]:
    task_id = str(item.get("id"))
    task_path = str(item.get("path") or "")
    child_slug = safe_slug(task_id)
    sandbox_run_id = f"{run_id}-{child_slug}-sandbox"
    workflow_run_id = f"{sandbox_run_id}-workflow"
    script_args = [
        "--task",
        task_path,
        "--run-id",
        sandbox_run_id,
        "--child-run-id",
        workflow_run_id,
        "--stage",
        args.stage,
        "--driver",
        args.driver,
    ]
    if args.policy:
        script_args.extend(["--policy", args.policy])
    if args.namespace:
        script_args.extend(["--namespace", args.namespace])
    if args.image:
        script_args.extend(["--image", args.image])
    if args.validate:
        script_args.append("--validate")
    result = run_script("sandbox.py", script_args)
    return {
        "task_id": task_id,
        "task_path": task_path,
        "run_id": sandbox_run_id,
        "child_run_id": workflow_run_id,
        "sandbox_plan": f"doc/02_acceptance/runs/{sandbox_run_id}/sandbox/sandbox-plan.json",
        "workspace_isolation": item.get("workspace_isolation"),
        "exit_code": result["exit_code"],
        "command": result["command"],
        "output_tail": result["output_tail"],
        "summary": read_child_summary(sandbox_run_id),
    }


def execute_plan(args: argparse.Namespace, task_id: str, plan_path: str, run_id: str) -> dict[str, Any]:
    child_slug = safe_slug(task_id)
    execution_run_id = f"{run_id}-{child_slug}-sandbox-exec"
    script_args = [
        "--sandbox-plan",
        plan_path,
        "--run-id",
        execution_run_id,
        "--timeout-seconds",
        str(args.timeout_seconds),
    ]
    if args.execute_sandbox:
        script_args.append("--execute")
    if args.cleanup:
        script_args.append("--cleanup")
    result = run_script("sandbox_executor.py", script_args)
    return {
        "task_id": task_id,
        "run_id": execution_run_id,
        "sandbox_plan": plan_path,
        "exit_code": result["exit_code"],
        "command": result["command"],
        "output_tail": result["output_tail"],
        "summary": read_child_summary(execution_run_id),
    }


def plan_status(plan_result: dict[str, Any]) -> str | None:
    summary = plan_result.get("summary") or {}
    return summary.get("status")


def execution_status(execution_result: dict[str, Any]) -> str | None:
    summary = execution_result.get("summary") or {}
    return summary.get("status")


def status_for(
    queue_guard_blocked: bool,
    lock_status: str,
    require_lock: bool,
    selected: list[dict[str, Any]],
    queue_claims: list[dict[str, Any]],
    plans: list[dict[str, Any]],
    executions: list[dict[str, Any]],
    execute_sandbox: bool,
) -> str:
    if queue_guard_blocked:
        return "SANDBOX_WORKER_QUEUE_GUARD_BLOCKED"
    if (require_lock or lock_status != "not_required") and lock_status != "valid":
        return "SANDBOX_WORKER_LOCK_BLOCKED"
    if selected and not plans and queue_claims:
        blocking = [
            item
            for item in queue_claims
            if not item.get("claimed") and str(item.get("reason")) not in {"state done", "state quarantined"}
        ]
        return "SANDBOX_WORKER_QUEUE_BLOCKED" if blocking else "SANDBOX_WORKER_PLANNED"
    if any(item.get("exit_code") not in {0} for item in plans):
        return "SANDBOX_WORKER_FAILED"
    if any(plan_status(item) == "SANDBOX_PLAN_BLOCKED" for item in plans):
        return "SANDBOX_WORKER_PLAN_BLOCKED"
    if execute_sandbox:
        if any(item.get("exit_code") not in {0} for item in executions):
            if any(execution_status(item) == "SANDBOX_EXECUTION_BLOCKED" for item in executions):
                return "SANDBOX_WORKER_EXECUTION_BLOCKED"
            return "SANDBOX_WORKER_FAILED"
        if executions and all(execution_status(item) == "SANDBOX_EXECUTION_COMPLETED" for item in executions):
            return "SANDBOX_WORKER_COMPLETED"
        return "SANDBOX_WORKER_EXECUTION_BLOCKED"
    if any(plan_status(item) == "SANDBOX_PLAN_DEGRADED" for item in plans):
        return "SANDBOX_WORKER_DEGRADED"
    return "SANDBOX_WORKER_PLANNED"


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--scheduler-plan", required=True)
    parser.add_argument("--run-id", default=None)
    parser.add_argument("--stage", choices=["prepare", "dry-run", "execute-local"], default="prepare")
    parser.add_argument("--driver", choices=["kubernetes-job", "local-container"], default="kubernetes-job")
    parser.add_argument("--policy", default=None)
    parser.add_argument("--namespace", default=None)
    parser.add_argument("--image", default=None)
    parser.add_argument("--max-tasks", type=int, default=1)
    parser.add_argument("--require-lock", action="store_true")
    parser.add_argument("--claim-queue", action="store_true")
    parser.add_argument("--queue-backend", choices=["repo-json", "sqlite", "http"], default=None)
    parser.add_argument("--queue-path", default=None)
    parser.add_argument("--execute-sandbox", action="store_true")
    parser.add_argument("--timeout-seconds", type=int, default=1800)
    parser.add_argument("--cleanup", action="store_true")
    parser.add_argument("--validate", action="store_true")
    args = parser.parse_args()

    run_id = args.run_id or make_run_id("sandbox-worker")
    run_dir = ensure_run_dir(run_id)
    out_dir = run_dir / "sandbox-worker"
    out_dir.mkdir(parents=True, exist_ok=True)
    plan_path = repo_path(args.scheduler_plan)
    scheduler_plan = load_json(plan_path)
    lock_status = check_lock(scheduler_plan, args.require_lock)
    selected = select_items(scheduler_plan, args.max_tasks, lock_status, args.require_lock)
    queue_backend = args.queue_backend or str(scheduler_plan.get("queue_backend") or "repo-json")
    queue_path = args.queue_path or scheduler_plan.get("queue_path")
    lease_seconds = int(scheduler_plan.get("lease_seconds") or 3600)
    queue_guard_blocked = bool(args.claim_queue and not args.execute_sandbox)

    plans: list[dict[str, Any]] = []
    executions: list[dict[str, Any]] = []
    queue_claims: list[dict[str, Any]] = []
    queue_updates: list[dict[str, Any]] = []

    if not queue_guard_blocked:
        for item in selected:
            task_id = str(item.get("id"))
            if args.claim_queue:
                claim = claim_item(task_id, run_id, lease_seconds, backend=queue_backend, path=queue_path)
                queue_claims.append(claim)
                if not claim.get("claimed"):
                    continue
            plan_result = plan_task(args, item, run_id)
            plans.append(plan_result)
            plan_ready = plan_status(plan_result) == "SANDBOX_PLAN_READY"
            execution_result: dict[str, Any] | None = None
            if args.execute_sandbox and plan_ready:
                execution_result = execute_plan(args, task_id, plan_result["sandbox_plan"], run_id)
                executions.append(execution_result)
            if args.claim_queue:
                result = {
                    "stage": args.stage,
                    "driver": args.driver,
                    "sandbox_plan_run_id": plan_result["run_id"],
                    "sandbox_plan_status": plan_status(plan_result),
                    "sandbox_execution_run_id": execution_result["run_id"] if execution_result else None,
                    "sandbox_execution_status": execution_status(execution_result) if execution_result else None,
                }
                if execution_result and execution_status(execution_result) == "SANDBOX_EXECUTION_COMPLETED":
                    result["status"] = "sandbox_execution_completed"
                    queue_updates.append(complete_item(task_id, run_id, result, backend=queue_backend, path=queue_path))
                else:
                    result["status"] = "sandbox_execution_not_completed"
                    queue_updates.append(fail_item(task_id, run_id, result, backend=queue_backend, path=queue_path))

    status = status_for(
        queue_guard_blocked,
        lock_status,
        args.require_lock,
        selected,
        queue_claims,
        plans,
        executions,
        args.execute_sandbox,
    )
    summary = {
        "run_id": run_id,
        "run_kind": "sandbox_worker",
        "status": status,
        "created_at": datetime.now().isoformat(timespec="seconds"),
        "commit": run_git(["rev-parse", "HEAD"]).strip(),
        "scheduler_plan": rel_path(plan_path),
        "stage": args.stage,
        "driver": args.driver,
        "execute_sandbox": args.execute_sandbox,
        "claim_queue": args.claim_queue,
        "validate": args.validate,
        "cleanup": args.cleanup,
        "timeout_seconds": args.timeout_seconds,
        "lock_status": lock_status,
        "selected_count": len(selected),
        "queue_backend": queue_backend if args.claim_queue else None,
        "queue_path": display_path(queue_path),
        "queue_claims": queue_claims,
        "queue_updates": queue_updates,
        "plans": plans,
        "executions": executions,
        "outputs": ["sandbox-worker/sandbox-worker-summary.json", "sandbox-worker/sandbox-worker-report.md"],
    }
    write_json(out_dir / "sandbox-worker-summary.json", summary)
    write_text(out_dir / "sandbox-worker-report.md", render_report(summary))
    write_json(run_dir / "run-summary.json", summary)
    print(out_dir)
    print(f"status={status} plans={len(plans)} executions={len(executions)}")
    return 0 if status in {"SANDBOX_WORKER_PLANNED", "SANDBOX_WORKER_DEGRADED", "SANDBOX_WORKER_COMPLETED"} else 2


if __name__ == "__main__":
    raise SystemExit(main())
