#!/usr/bin/env python3
"""Run a bounded Codex Loop executor pool from a scheduler plan or queue."""

from __future__ import annotations

import argparse
import json
import os
import subprocess
import sys
from concurrent.futures import ThreadPoolExecutor, as_completed
from datetime import datetime
from pathlib import Path
from typing import Any

from lib import RUNS_ROOT, SCRIPT_ROOT, ensure_run_dir, make_run_id, rel_path, repo_path, run_git, write_json, write_text
from queue_backend import display_path, normalize_backend, queue_status
from resource_quota import DEFAULT_POLICY as DEFAULT_QUOTA_POLICY
from resource_quota import apply_quotas, load_policy as load_quota_policy, normalize_item
from resource_monitor import DEFAULT_POLICY as DEFAULT_MONITOR_POLICY
from resource_monitor import build_resource_monitor, monitor_blocked, monitor_degraded, monitor_digest
from workspace_isolation import DEFAULT_POLICY as DEFAULT_ISOLATION_POLICY
from workspace_isolation import SUPPORTED_BACKENDS as SUPPORTED_WORKSPACE_BACKENDS
from workspace_isolation import build_isolation as build_workspace_isolation
from workspace_isolation import render_report as render_workspace_isolation_report


CLAIMABLE_STATES = {"queued", "failed"}


def load_json(path: Path | None) -> dict[str, Any]:
    if not path or not path.exists():
        return {}
    return json.loads(path.read_text(encoding="utf-8"))


def safe_slug(value: str) -> str:
    return "".join(ch.lower() if ch.isalnum() else "-" for ch in value).strip("-") or "task"


def run_command(command: list[str], timeout: int | None = None, cwd: Path | None = None, env: dict[str, str] | None = None) -> dict[str, Any]:
    try:
        proc = subprocess.run(
            command,
            cwd=cwd or repo_path("."),
            env=env,
            text=True,
            stdout=subprocess.PIPE,
            stderr=subprocess.STDOUT,
            timeout=timeout,
            check=False,
        )
        return {
            "command": command,
            "exit_code": proc.returncode,
            "output_tail": proc.stdout[-4000:],
            "timed_out": False,
        }
    except subprocess.TimeoutExpired as exc:
        output = exc.stdout if isinstance(exc.stdout, str) else (exc.stdout or b"").decode("utf-8", errors="replace")
        return {
            "command": command,
            "exit_code": None,
            "output_tail": output[-4000:],
            "timed_out": True,
        }


def child_summary(run_id: str) -> dict[str, Any] | None:
    path = RUNS_ROOT / run_id / "run-summary.json"
    if not path.exists():
        return None
    return load_json(path)


def scheduler_item_from_queue(item: dict[str, Any]) -> dict[str, Any]:
    return normalize_item(
        {
            "id": item.get("task_id"),
            "title": item.get("title"),
            "priority": item.get("priority"),
            "status": item.get("state"),
            "score": item.get("score"),
            "mode": item.get("mode"),
            "data_mode": item.get("data_mode"),
            "lane_primary": item.get("lane_primary"),
            "lane_dependent": item.get("lane_dependent") or [],
            "subsystems": item.get("subsystems") or [],
            "acceptance_type": item.get("acceptance_type"),
            "risk_level": item.get("risk_level"),
            "resource_weight": item.get("resource_weight"),
            "resource_group": item.get("resource_group"),
            "allowed_paths": item.get("allowed_paths") or [],
            "attempt": int(item.get("attempts") or 0) + 1,
            "max_retries": int(item.get("max_retries") or 3),
            "path": item.get("task_path"),
            "source_scheduler_plan": item.get("scheduler_plan"),
        }
    )


def load_source(args: argparse.Namespace) -> tuple[list[dict[str, Any]], dict[str, Any]]:
    if args.scheduler_plan:
        plan_path = repo_path(args.scheduler_plan)
        plan = load_json(plan_path)
        return list(plan.get("queue", {}).get("selected", [])), {
            "kind": "scheduler_plan",
            "scheduler_plan": rel_path(plan_path),
            "context_dir": plan.get("context_dir"),
            "guidance": plan.get("guidance"),
            "queue_backend": plan.get("queue_backend"),
            "queue_path": plan.get("queue_path"),
            "lease_seconds": plan.get("lease_seconds"),
        }
    backend = normalize_backend(args.queue_backend)
    status = queue_status(backend=backend, path=args.queue_path, include_items=True)
    items = [
        scheduler_item_from_queue(item)
        for item in status.get("items", [])
        if str(item.get("state")) in CLAIMABLE_STATES
    ]
    items.sort(key=lambda item: (-(int(item.get("score") or 0)), str(item.get("id"))))
    return items, {
        "kind": "queue",
        "queue_backend": backend,
        "queue_path": status.get("path"),
        "queue_counts": status.get("counts"),
        "recovered_claims": status.get("recovered_claims", []),
    }


def choose_items(args: argparse.Namespace, candidates: list[dict[str, Any]]) -> dict[str, Any]:
    policy = load_quota_policy(args.quota_policy)
    if args.skip_quota:
        selected = [normalize_item(item) for item in candidates[: args.max_tasks]]
        return {
            "enabled": False,
            "policy": policy.get("_path"),
            "max_items": args.max_tasks,
            "selected": selected,
            "deferred": [],
            "usage": {},
        }
    return apply_quotas([normalize_item(item) for item in candidates], policy, args.max_tasks)


def synthetic_plan(
    args: argparse.Namespace,
    pool_run_id: str,
    item: dict[str, Any],
    source: dict[str, Any],
    out_dir: Path,
) -> str:
    task_id = str(item.get("id"))
    source_plan = load_json(repo_path(item.get("source_scheduler_plan"))) if item.get("source_scheduler_plan") else {}
    context_dir = args.context_dir or source.get("context_dir") or source_plan.get("context_dir")
    guidance = args.guidance or source.get("guidance") or source_plan.get("guidance")
    if args.activate_workspaces:
        if context_dir:
            context_dir = str(repo_path(str(context_dir)))
        if guidance:
            guidance = str(repo_path(str(guidance)))
    queue_backend = args.queue_backend or source.get("queue_backend") or source_plan.get("queue_backend") or "repo-json"
    queue_path = args.queue_path or source_plan.get("queue_path")
    if source.get("queue_path") and not args.queue_path:
        queue_path = source.get("queue_path")
    selected = dict(item)
    selected.setdefault("path", item.get("task_path"))
    if args.activate_workspaces and selected.get("path"):
        selected["path"] = str(repo_path(str(selected["path"])))
    plan = {
        "run_id": f"{pool_run_id}-{safe_slug(task_id)}-synthetic-scheduler",
        "run_kind": "scheduler",
        "status": "SYNTHETIC_SCHEDULER_PLAN",
        "created_at": datetime.now().isoformat(timespec="seconds"),
        "commit": run_git(["rev-parse", "HEAD"]).strip(),
        "guidance": guidance,
        "context_dir": context_dir,
        "queue": {"selected": [selected], "deferred": []},
        "quota": {"from_executor_pool": pool_run_id},
        "lock": None,
        "lease_seconds": int(args.lease_seconds or source.get("lease_seconds") or source_plan.get("lease_seconds") or 3600),
        "queue_backend": queue_backend,
        "queue_path": queue_path,
        "persistent_queue": {"from_executor_pool": pool_run_id} if args.claim_queue else None,
        "outputs": [],
    }
    plans_dir = out_dir / "scheduler-plans"
    plans_dir.mkdir(parents=True, exist_ok=True)
    target = plans_dir / f"{safe_slug(task_id)}.scheduler-plan.json"
    write_json(target, plan)
    return str(target.resolve()) if args.activate_workspaces else rel_path(target)


def child_command(args: argparse.Namespace, task_id: str, scheduler_plan: str, child_run_id: str) -> list[str]:
    if args.runner == "workflow":
        command = [
            sys.executable,
            "-B",
            str(SCRIPT_ROOT / "worker.py"),
            "--scheduler-plan",
            scheduler_plan,
            "--run-id",
            child_run_id,
            "--stage",
            args.stage,
            "--max-tasks",
            "1",
            "--queue-backend",
            args.queue_backend,
        ]
        if args.claim_queue:
            command.append("--claim-queue")
        if args.allow_blocker_execution:
            command.append("--allow-blocker-execution")
    else:
        command = [
            sys.executable,
            "-B",
            str(SCRIPT_ROOT / "sandbox_worker.py"),
            "--scheduler-plan",
            scheduler_plan,
            "--run-id",
            child_run_id,
            "--stage",
            args.stage,
            "--max-tasks",
            "1",
            "--driver",
            args.sandbox_driver,
            "--timeout-seconds",
            str(args.timeout_seconds),
            "--queue-backend",
            args.queue_backend,
        ]
        if args.runner == "sandbox-execute":
            command.extend(["--execute-sandbox", "--claim-queue"])
        if args.sandbox_cleanup:
            command.append("--cleanup")
        if args.sandbox_validate:
            command.append("--validate")
    if args.queue_path:
        command.extend(["--queue-path", args.queue_path])
    return command


def guard_findings(args: argparse.Namespace, selected: list[dict[str, Any]]) -> list[dict[str, str]]:
    findings: list[dict[str, str]] = []
    backend = normalize_backend(args.queue_backend)
    if args.max_workers > 1 and args.runner != "sandbox-plan":
        if not args.allow_parallel_execution:
            findings.append({"level": "blocker", "code": "PARALLEL_EXECUTION_GATE_NOT_SET", "message": "Set --allow-parallel-execution before running non-plan workers with max_workers > 1."})
        if backend != "sqlite":
            findings.append({"level": "blocker", "code": "PARALLEL_EXECUTION_REQUIRES_SQLITE", "message": "Parallel queue mutation requires the sqlite queue backend."})
    if args.claim_queue and args.runner == "sandbox-plan":
        findings.append({"level": "blocker", "code": "PLAN_RUNNER_CANNOT_CLAIM_QUEUE", "message": "sandbox-plan runner cannot claim or mutate queue state."})
    if args.runner == "workflow" and args.max_workers > 1:
        findings.append({"level": "blocker", "code": "WORKFLOW_PARALLEL_NOT_ISOLATED", "message": "workflow runner is not isolated for parallel execution; use sandbox-plan or sandbox-execute."})
    for item in selected:
        if not item.get("path"):
            findings.append({"level": "blocker", "code": "TASK_PATH_MISSING", "message": f"Selected task `{item.get('id')}` has no task path."})
        if args.activate_workspaces:
            spec = item.get("workspace_isolation") or {}
            workspace_path = workspace_path_for(item)
            if not spec:
                findings.append({"level": "blocker", "code": "WORKSPACE_SPEC_MISSING", "message": f"Selected task `{item.get('id')}` has no workspace isolation spec."})
            elif spec.get("path_allowed") is not True:
                findings.append({"level": "blocker", "code": "WORKSPACE_PATH_NOT_ALLOWED", "message": f"Selected task `{item.get('id')}` workspace path is outside allowed roots."})
            elif not workspace_path or not workspace_path.exists():
                findings.append({"level": "blocker", "code": "WORKSPACE_NOT_CREATED", "message": f"Selected task `{item.get('id')}` workspace has not been created; run with --create-worktrees and CODEX_LOOP_ALLOW_WORKTREE_CREATE=1 first."})
    return findings


def resource_admission(args: argparse.Namespace) -> dict[str, Any] | None:
    if args.skip_resource_monitor:
        return None
    return build_resource_monitor(args.resource_monitor_policy, args.queue_backend, args.queue_path, args.resource_sample_seconds)


def workspace_isolation(args: argparse.Namespace, run_id: str, selected: list[dict[str, Any]], out_dir: Path) -> dict[str, Any] | None:
    if args.skip_workspace_isolation:
        return None
    isolation = {
        "run_id": run_id,
        **build_workspace_isolation(
            run_id,
            selected,
            args.workspace_isolation_policy,
            args.create_worktrees,
            args.workspace_backend,
        ),
        "outputs": [
            "executor-pool/workspace-isolation.json",
            "executor-pool/workspace-isolation.md",
        ],
    }
    write_json(out_dir / "workspace-isolation.json", isolation)
    write_text(out_dir / "workspace-isolation.md", render_workspace_isolation_report(isolation))
    return isolation


def attach_workspace_isolation(selected: list[dict[str, Any]], isolation: dict[str, Any] | None) -> list[dict[str, Any]]:
    if not isolation:
        return selected
    by_task = {str(item.get("task_id")): item for item in isolation.get("workspaces") or []}
    enriched: list[dict[str, Any]] = []
    for item in selected:
        next_item = dict(item)
        spec = by_task.get(str(next_item.get("id")))
        if spec:
            next_item["workspace_isolation"] = {
                "workspace_path": spec.get("workspace_path"),
                "absolute_workspace_path": spec.get("absolute_workspace_path"),
                "backend": spec.get("backend"),
                "base_commit": spec.get("base_commit"),
                "task_path": spec.get("task_path"),
                "allowed_paths": spec.get("allowed_paths") or [],
                "exists": spec.get("exists"),
                "exists_after": spec.get("exists_after"),
                "path_allowed": spec.get("path_allowed"),
            }
        enriched.append(next_item)
    return enriched


def workspace_path_for(item: dict[str, Any]) -> Path | None:
    spec = item.get("workspace_isolation") or {}
    raw = spec.get("absolute_workspace_path") or spec.get("workspace_path")
    if not raw:
        return None
    return repo_path(str(raw))


def workspace_activation_env(args: argparse.Namespace, item: dict[str, Any]) -> tuple[Path, dict[str, str] | None, dict[str, Any]]:
    source_root = repo_path(".").resolve()
    activation = {
        "requested": bool(args.activate_workspaces),
        "activated": False,
        "workspace_path": None,
        "runs_root": str(RUNS_ROOT),
        "controller_repo_root": str(source_root),
    }
    if not args.activate_workspaces:
        return source_root, None, activation
    workspace_path = workspace_path_for(item)
    activation["workspace_path"] = str(workspace_path) if workspace_path else None
    if not workspace_path or not workspace_path.exists():
        return source_root, None, activation
    env = os.environ.copy()
    env["CODEX_LOOP_REPO_ROOT"] = str(workspace_path.resolve())
    env["CODEX_LOOP_RUNS_ROOT"] = str(RUNS_ROOT)
    env["CODEX_LOOP_CONTROLLER_REPO_ROOT"] = str(source_root)
    env["CODEX_LOOP_ACTIVE_WORKSPACE"] = str(workspace_path.resolve())
    activation["activated"] = True
    activation["workspace_path"] = str(workspace_path.resolve())
    return workspace_path.resolve(), env, activation


def run_child(args: argparse.Namespace, pool_run_id: str, item: dict[str, Any], plan_path: str) -> dict[str, Any]:
    task_id = str(item.get("id"))
    child_run_id = f"{pool_run_id}-{safe_slug(task_id)}-{args.runner}"
    command = child_command(args, task_id, plan_path, child_run_id)
    cwd, env, activation = workspace_activation_env(args, item)
    result = run_command(command, timeout=args.child_timeout_seconds, cwd=cwd, env=env)
    return {
        "task_id": task_id,
        "runner": args.runner,
        "scheduler_plan": plan_path,
        "run_id": child_run_id,
        "workspace_activation": activation,
        "command": command,
        "exit_code": result.get("exit_code"),
        "timed_out": result.get("timed_out"),
        "output_tail": result.get("output_tail"),
        "summary": child_summary(child_run_id),
    }


def derive_status(findings: list[dict[str, str]], selected: list[dict[str, Any]], results: list[dict[str, Any]], runner: str) -> str:
    if findings:
        return "EXECUTOR_POOL_BLOCKED"
    if not selected:
        return "EXECUTOR_POOL_EMPTY"
    if any(item.get("exit_code") not in {0} for item in results):
        return "EXECUTOR_POOL_FAILED"
    if runner == "sandbox-plan":
        return "EXECUTOR_POOL_PLANNED"
    return "EXECUTOR_POOL_COMPLETED"


def render_report(summary: dict[str, Any]) -> str:
    lines = [
        "# Codex Loop Executor Pool",
        "",
        f"- run_id: `{summary['run_id']}`",
        f"- status: `{summary['status']}`",
        f"- runner: `{summary['runner']}`",
        f"- max_workers: `{summary['max_workers']}`",
        f"- requested_max_workers: `{summary.get('requested_max_workers', summary['max_workers'])}`",
        f"- selected: `{len(summary.get('selected') or [])}`",
        f"- executed: `{len(summary.get('children') or [])}`",
        "",
        "## Resource Admission",
        f"- monitor_status: `{(summary.get('resource_monitor') or {}).get('status', 'skipped')}`",
        f"- adjustment: `{summary.get('resource_adjustment') or 'none'}`",
        "",
        "## Workspace Isolation",
        f"- isolation_status: `{(summary.get('workspace_isolation') or {}).get('status', 'skipped')}`",
        f"- isolation_mode: `{(summary.get('workspace_isolation') or {}).get('mode', 'none')}`",
        f"- workspace_backend: `{(summary.get('workspace_isolation') or {}).get('workspace_backend', 'none')}`",
        f"- activate_workspaces: `{summary.get('activate_workspaces')}`",
        "",
        "## Findings",
    ]
    if summary.get("findings"):
        for item in summary["findings"]:
            lines.append(f"- `{item['level']}` `{item['code']}`: {item['message']}")
    else:
        lines.append("- none")
    lines.extend(["", "## Children"])
    if summary.get("children"):
        for item in summary["children"]:
            child_status = (item.get("summary") or {}).get("status")
            workspace = (item.get("workspace_activation") or {}).get("workspace_path") or "source-worktree"
            lines.append(f"- `{item['task_id']}` -> `{item['run_id']}` exit `{item.get('exit_code')}` status `{child_status}` workspace `{workspace}`")
    else:
        lines.append("- none")
    lines.extend(
        [
            "",
            "## Guardrail",
            "- The pool is bounded by --max-workers and --max-tasks.",
            "- Parallel queue mutation requires sqlite and --allow-parallel-execution.",
            "- sandbox-plan is the default safe runner and does not claim queue items.",
            "",
        ]
    )
    return "\n".join(lines)


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--scheduler-plan", default=None)
    parser.add_argument("--run-id", default=None)
    parser.add_argument("--runner", choices=["workflow", "sandbox-plan", "sandbox-execute"], default="sandbox-plan")
    parser.add_argument("--stage", choices=["prepare", "dry-run", "execute-local"], default="prepare")
    parser.add_argument("--max-workers", type=int, default=1)
    parser.add_argument("--max-tasks", type=int, default=1)
    parser.add_argument("--queue-backend", choices=["repo-json", "sqlite", "http"], default=None)
    parser.add_argument("--queue-path", default=None)
    parser.add_argument("--claim-queue", action="store_true")
    parser.add_argument("--allow-parallel-execution", action="store_true")
    parser.add_argument("--allow-blocker-execution", action="store_true")
    parser.add_argument("--context-dir", default=None)
    parser.add_argument("--guidance", default=None)
    parser.add_argument("--lease-seconds", type=int, default=3600)
    parser.add_argument("--quota-policy", default=str(DEFAULT_QUOTA_POLICY))
    parser.add_argument("--skip-quota", action="store_true")
    parser.add_argument("--resource-monitor-policy", default=str(DEFAULT_MONITOR_POLICY))
    parser.add_argument("--resource-sample-seconds", type=int, default=None)
    parser.add_argument("--skip-resource-monitor", action="store_true")
    parser.add_argument("--workspace-isolation-policy", default=str(DEFAULT_ISOLATION_POLICY))
    parser.add_argument("--workspace-backend", choices=sorted(SUPPORTED_WORKSPACE_BACKENDS), default=None)
    parser.add_argument("--skip-workspace-isolation", action="store_true")
    parser.add_argument("--create-worktrees", action="store_true")
    parser.add_argument("--activate-workspaces", action="store_true")
    parser.add_argument("--sandbox-driver", choices=["kubernetes-job", "local-container"], default="kubernetes-job")
    parser.add_argument("--sandbox-validate", action="store_true")
    parser.add_argument("--sandbox-cleanup", action="store_true")
    parser.add_argument("--timeout-seconds", type=int, default=1800)
    parser.add_argument("--child-timeout-seconds", type=int, default=2400)
    args = parser.parse_args()

    run_id = args.run_id or make_run_id("executor-pool")
    run_dir = ensure_run_dir(run_id)
    out_dir = run_dir / "executor-pool"
    out_dir.mkdir(parents=True, exist_ok=True)
    candidates, source = load_source(args)
    args.queue_backend = normalize_backend(args.queue_backend or source.get("queue_backend") or "repo-json")
    requested_max_workers = args.max_workers
    resource_monitor = resource_admission(args)
    resource_adjustment: dict[str, Any] | None = None
    if monitor_degraded(resource_monitor) and args.max_workers > 1:
        args.max_workers = 1
        resource_adjustment = {
            "reason": "resource monitor degraded; serializing pool execution",
            "requested_max_workers": requested_max_workers,
            "effective_max_workers": args.max_workers,
        }
    quota = choose_items(args, candidates)
    selected = list(quota.get("selected") or [])[: args.max_tasks]
    isolation = workspace_isolation(args, run_id, selected, out_dir)
    selected = attach_workspace_isolation(selected, isolation)
    findings = guard_findings(args, selected)
    if monitor_blocked(resource_monitor):
        findings.append({"level": "blocker", "code": "RESOURCE_MONITOR_BLOCKED", "message": "Dynamic resource monitor blocked executor pool admission."})
    if args.max_workers > 1 and (not isolation or isolation.get("status") == "WORKSPACE_ISOLATION_BLOCKED"):
        findings.append({"level": "blocker", "code": "WORKSPACE_ISOLATION_NOT_READY", "message": "Parallel executor pool requires a non-blocked workspace isolation plan."})
    plan_paths: list[dict[str, str]] = []
    children: list[dict[str, Any]] = []
    if not findings:
        for item in selected:
            plan_path = synthetic_plan(args, run_id, item, source, out_dir)
            plan_paths.append({"task_id": str(item.get("id")), "path": plan_path})
        with ThreadPoolExecutor(max_workers=max(1, args.max_workers)) as pool:
            futures = [
                pool.submit(run_child, args, run_id, item, plan["path"])
                for item, plan in zip(selected, plan_paths)
            ]
            for future in as_completed(futures):
                children.append(future.result())
        children.sort(key=lambda item: str(item.get("task_id")))

    status = derive_status(findings, selected, children, args.runner)
    outputs = [
        "executor-pool/executor-pool-summary.json",
        "executor-pool/executor-pool-report.md",
    ]
    if isolation:
        outputs.extend(["executor-pool/workspace-isolation.json", "executor-pool/workspace-isolation.md"])
    outputs.extend(item["path"].replace(f"doc/02_acceptance/runs/{run_id}/", "") for item in plan_paths)
    summary = {
        "run_id": run_id,
        "run_kind": "executor_pool",
        "status": status,
        "created_at": datetime.now().isoformat(timespec="seconds"),
        "commit": run_git(["rev-parse", "HEAD"]).strip(),
        "source": source,
        "runner": args.runner,
        "stage": args.stage,
        "requested_max_workers": requested_max_workers,
        "max_workers": args.max_workers,
        "max_tasks": args.max_tasks,
        "queue_backend": args.queue_backend,
        "queue_path": display_path(args.queue_path),
        "claim_queue": args.claim_queue,
        "allow_parallel_execution": args.allow_parallel_execution,
        "sandbox_driver": args.sandbox_driver if args.runner.startswith("sandbox") else None,
        "activate_workspaces": args.activate_workspaces,
        "resource_monitor": monitor_digest(resource_monitor),
        "resource_adjustment": resource_adjustment,
        "workspace_isolation": {
            "status": isolation.get("status"),
            "mode": isolation.get("mode"),
            "workspace_backend": isolation.get("workspace_backend"),
            "workspace_root": isolation.get("workspace_root"),
            "workspaces": isolation.get("workspaces"),
            "findings": isolation.get("findings", []),
        } if isolation else None,
        "quota": quota,
        "selected": selected,
        "findings": findings,
        "scheduler_plans": plan_paths,
        "children": children,
        "outputs": outputs,
    }
    write_json(out_dir / "executor-pool-summary.json", summary)
    write_text(out_dir / "executor-pool-report.md", render_report(summary))
    write_json(run_dir / "run-summary.json", summary)
    print(out_dir)
    print(f"status={status} selected={len(selected)} children={len(children)} findings={len(findings)}")
    return 0 if status in {"EXECUTOR_POOL_PLANNED", "EXECUTOR_POOL_COMPLETED", "EXECUTOR_POOL_EMPTY"} else 2


if __name__ == "__main__":
    raise SystemExit(main())
