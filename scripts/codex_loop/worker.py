#!/usr/bin/env python3
"""Execute a scheduler queue through guarded workflow runs."""

from __future__ import annotations

import argparse
import json
import subprocess
import sys
from datetime import datetime
from pathlib import Path
from typing import Any

from lib import SCRIPT_ROOT, ensure_run_dir, make_run_id, rel_path, repo_path, run_git, write_json, write_text
from lock_manager import heartbeat_lock, lock_expired, read_lock
from queue_backend import claim_item, complete_item, display_path, fail_item


def load_json(path: Path) -> dict[str, Any]:
    return json.loads(path.read_text(encoding="utf-8"))


def safe_slug(value: str) -> str:
    return "".join(ch.lower() if ch.isalnum() else "-" for ch in value).strip("-")


def run_workflow(task_ref: str, child_run_id: str, context_dir: str, guidance: str, stage: str, allow_blocker_execution: bool) -> dict[str, Any]:
    command = [
        sys.executable,
        "-B",
        str(SCRIPT_ROOT / "workflow.py"),
        "--task",
        task_ref,
        "--run-id",
        child_run_id,
        "--context-dir",
        context_dir,
        "--guidance",
        guidance,
        "--stage",
        stage,
    ]
    if allow_blocker_execution:
        command.append("--allow-blocker-execution")
    proc = subprocess.run(
        command,
        cwd=repo_path("."),
        text=True,
        stdout=subprocess.PIPE,
        stderr=subprocess.STDOUT,
        check=False,
    )
    return {
        "task_ref": task_ref,
        "run_id": child_run_id,
        "command": command,
        "exit_code": proc.returncode,
        "output_tail": proc.stdout[-4000:],
    }


def render_report(summary: dict[str, Any]) -> str:
    lines = [
        "# Codex Loop Worker Report",
        "",
        f"- run_id: `{summary['run_id']}`",
        f"- status: `{summary['status']}`",
        f"- stage: `{summary['stage']}`",
        f"- executed: `{len(summary['executions'])}`",
        f"- lock_status: `{summary.get('lock_status')}`",
        f"- queue_status: `{summary.get('queue_status')}`",
        "",
        "## Executions",
    ]
    if summary["executions"]:
        for item in summary["executions"]:
            workspace = ((item.get("workspace_isolation") or {}).get("workspace_path")) or "source-worktree"
            lines.append(f"- `{item['task_id']}` -> `{item['run_id']}` exit `{item['exit_code']}` workspace `{workspace}`")
    else:
        lines.append("- none")
    lines.extend(["", "## Queue Claims"])
    if summary.get("queue_claims"):
        for item in summary["queue_claims"]:
            status = "claimed" if item.get("claimed") else f"skipped: {item.get('reason')}"
            lines.append(f"- `{item.get('task_id')}` {status}")
    else:
        lines.append("- not used")
    lines.extend(
        [
            "",
            "## Guardrail",
            "- Worker defaults to workflow prepare stage.",
            "- It does not call external Codex or apply patches by itself.",
            "",
        ]
    )
    return "\n".join(lines)


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--scheduler-plan", required=True)
    parser.add_argument("--run-id", default=None)
    parser.add_argument("--stage", choices=["prepare", "dry-run", "execute-local"], default="prepare")
    parser.add_argument("--max-tasks", type=int, default=1)
    parser.add_argument("--allow-blocker-execution", action="store_true")
    parser.add_argument("--require-lock", action="store_true")
    parser.add_argument("--claim-queue", action="store_true")
    parser.add_argument("--queue-backend", choices=["repo-json", "sqlite", "http"], default=None)
    parser.add_argument("--queue-path", default=None)
    args = parser.parse_args()

    run_id = args.run_id or make_run_id("worker")
    run_dir = ensure_run_dir(run_id)
    out_dir = run_dir / "worker"
    out_dir.mkdir(parents=True, exist_ok=True)
    plan_path = repo_path(args.scheduler_plan)
    plan = load_json(plan_path)
    lock = plan.get("lock")
    lock_status = "not_required"
    if args.require_lock or lock:
        current = read_lock()
        expected_run = str(lock.get("payload", {}).get("run_id")) if lock else None
        if not current:
            lock_status = "missing"
        elif lock_expired(current):
            lock_status = "expired"
        elif expected_run and str(current.get("run_id")) != expected_run:
            lock_status = "mismatch"
        else:
            lock_status = "valid"
        if lock_status == "valid":
            lease_seconds = int(plan.get("lease_seconds") or current.get("lease_seconds") or 3600)
            heartbeat_lock(str(current.get("run_id")), lease_seconds)
    selected = plan.get("queue", {}).get("selected", [])[: args.max_tasks]
    if (args.require_lock or lock) and lock_status != "valid":
        selected = []
    context_dir = str(plan.get("context_dir"))
    guidance = str(plan.get("guidance"))
    executions = []
    queue_claims: list[dict[str, Any]] = []
    queue_updates: list[dict[str, Any]] = []
    queue_status = "not_used"
    use_queue = bool(args.claim_queue or plan.get("persistent_queue"))
    queue_backend = args.queue_backend or str(plan.get("queue_backend") or "repo-json")
    queue_path = args.queue_path or plan.get("queue_path")
    if use_queue:
        queue_status = f"enabled:{queue_backend}"
    for item in selected:
        if lock_status == "valid":
            current = read_lock()
            heartbeat_lock(str(current.get("run_id")), int(plan.get("lease_seconds") or current.get("lease_seconds") or 3600))
        task_id = str(item.get("id"))
        if use_queue:
            claim = claim_item(task_id, run_id, int(plan.get("lease_seconds") or 3600), backend=queue_backend, path=queue_path)
            queue_claims.append(claim)
            if not claim.get("claimed"):
                continue
        child_run_id = f"{run_id}-{safe_slug(task_id)}"
        task_ref = str(item.get("path") or task_id)
        execution = run_workflow(task_ref, child_run_id, context_dir, guidance, args.stage, args.allow_blocker_execution)
        execution["task_id"] = task_id
        execution["workspace_isolation"] = item.get("workspace_isolation")
        executions.append(execution)
        if use_queue:
            result = {
                "status": "workflow_exit_0" if execution["exit_code"] == 0 else "workflow_failed",
                "exit_code": execution["exit_code"],
                "stage": args.stage,
                "child_run_id": child_run_id,
            }
            if execution["exit_code"] == 0:
                queue_updates.append(complete_item(task_id, run_id, result, backend=queue_backend, path=queue_path))
            else:
                queue_updates.append(fail_item(task_id, run_id, result, backend=queue_backend, path=queue_path))

    if (args.require_lock or lock) and lock_status != "valid":
        status = "WORKER_LOCK_BLOCKED"
    elif use_queue and selected and not executions:
        blocking = [
            item
            for item in queue_claims
            if not item.get("claimed") and str(item.get("reason")) not in {"state done", "state quarantined"}
        ]
        status = "WORKER_QUEUE_BLOCKED" if blocking else "WORKER_COMPLETED"
    else:
        status = "WORKER_COMPLETED" if all(item["exit_code"] == 0 for item in executions) else "WORKER_FAILED"
    summary = {
        "run_id": run_id,
        "run_kind": "worker",
        "status": status,
        "stage": args.stage,
        "created_at": datetime.now().isoformat(timespec="seconds"),
        "commit": run_git(["rev-parse", "HEAD"]).strip(),
        "scheduler_plan": rel_path(plan_path),
        "lock_status": lock_status,
        "queue_status": queue_status,
        "queue_backend": queue_backend if use_queue else None,
        "queue_path": display_path(queue_path),
        "queue_claims": queue_claims,
        "queue_updates": queue_updates,
        "executions": executions,
        "outputs": ["worker/worker-summary.json", "worker/worker-report.md"],
    }
    write_json(out_dir / "worker-summary.json", summary)
    write_text(out_dir / "worker-report.md", render_report(summary))
    write_json(run_dir / "run-summary.json", summary)
    print(out_dir)
    print(f"status={status} executed={len(executions)}")
    return 1 if status in {"WORKER_FAILED", "WORKER_LOCK_BLOCKED", "WORKER_QUEUE_BLOCKED"} else 0


if __name__ == "__main__":
    raise SystemExit(main())
