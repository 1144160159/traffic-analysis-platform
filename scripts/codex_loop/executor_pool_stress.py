#!/usr/bin/env python3
"""Run repeated executor-pool cycles and audit workspace cleanup/leaks."""

from __future__ import annotations

import argparse
import json
import subprocess
import sys
from datetime import datetime
from pathlib import Path
from typing import Any

from lib import RUNS_ROOT, SCRIPT_ROOT, ensure_run_dir, make_run_id, rel_path, repo_path, run_git, write_json, write_text
from queue_backend import display_path
from resource_quota import DEFAULT_POLICY as DEFAULT_QUOTA_POLICY
from resource_monitor import DEFAULT_POLICY as DEFAULT_MONITOR_POLICY
from workspace_isolation import DEFAULT_POLICY as DEFAULT_ISOLATION_POLICY


def load_json(path: Path) -> dict[str, Any]:
    if not path.exists():
        return {}
    return json.loads(path.read_text(encoding="utf-8"))


def run_script(name: str, args: list[str], timeout: int | None = None) -> dict[str, Any]:
    command = [sys.executable, "-B", str(SCRIPT_ROOT / name), *args]
    try:
        proc = subprocess.run(
            command,
            cwd=repo_path("."),
            text=True,
            stdout=subprocess.PIPE,
            stderr=subprocess.STDOUT,
            timeout=timeout,
            check=False,
        )
        return {"name": name, "command": command, "exit_code": proc.returncode, "timed_out": False, "output_tail": proc.stdout[-4000:]}
    except subprocess.TimeoutExpired as exc:
        output = exc.stdout if isinstance(exc.stdout, str) else (exc.stdout or b"").decode("utf-8", errors="replace")
        return {"name": name, "command": command, "exit_code": None, "timed_out": True, "output_tail": output[-4000:]}


def worktree_snapshot() -> list[dict[str, str]]:
    items: list[dict[str, str]] = []
    current: dict[str, str] | None = None
    proc = subprocess.run(["git", "worktree", "list", "--porcelain"], cwd=repo_path("."), text=True, stdout=subprocess.PIPE, stderr=subprocess.STDOUT, check=False)
    for line in proc.stdout.splitlines():
        if line.startswith("worktree "):
            if current:
                items.append(current)
            current = {"path": line.removeprefix("worktree ").strip()}
        elif current and line.startswith("HEAD "):
            current["head"] = line.removeprefix("HEAD ").strip()
        elif current and line.startswith("branch "):
            current["branch"] = line.removeprefix("branch ").strip()
        elif current and line == "detached":
            current["branch"] = "detached"
    if current:
        items.append(current)
    return items


def worktree_paths(snapshot: list[dict[str, str]]) -> set[str]:
    return {str(Path(item["path"]).resolve()) for item in snapshot if item.get("path")}


def pool_args(args: argparse.Namespace, pool_run_id: str) -> list[str]:
    result = [
        "--scheduler-plan",
        args.scheduler_plan,
        "--run-id",
        pool_run_id,
        "--runner",
        args.runner,
        "--stage",
        args.stage,
        "--max-workers",
        str(args.max_workers),
        "--max-tasks",
        str(args.max_tasks),
        "--queue-backend",
        args.queue_backend,
        "--quota-policy",
        args.quota_policy,
        "--resource-monitor-policy",
        args.resource_monitor_policy,
        "--workspace-isolation-policy",
        args.workspace_isolation_policy,
        "--workspace-backend",
        args.workspace_backend,
        "--sandbox-driver",
        args.sandbox_driver,
        "--child-timeout-seconds",
        str(args.child_timeout_seconds),
    ]
    if args.queue_path:
        result.extend(["--queue-path", args.queue_path])
    if args.skip_resource_monitor:
        result.append("--skip-resource-monitor")
    if args.skip_quota:
        result.append("--skip-quota")
    if args.create_worktrees:
        result.append("--create-worktrees")
    if args.activate_workspaces:
        result.append("--activate-workspaces")
    if args.allow_parallel_execution:
        result.append("--allow-parallel-execution")
    if args.sandbox_validate:
        result.append("--sandbox-validate")
    return result


def cleanup_args(args: argparse.Namespace, pool_run_id: str, cleanup_run_id: str) -> list[str]:
    source = RUNS_ROOT / pool_run_id / "executor-pool" / "workspace-isolation.json"
    result = [
        "--workspace-isolation",
        str(source),
        "--run-id",
        cleanup_run_id,
        "--policy",
        args.workspace_isolation_policy,
    ]
    if args.cleanup_worktrees:
        result.append("--execute")
    if args.cleanup_force:
        result.append("--force")
    return result


def existing_workspace_paths(pool_summary: dict[str, Any]) -> list[str]:
    isolation = pool_summary.get("workspace_isolation") or {}
    paths: list[str] = []
    for spec in isolation.get("workspaces") or []:
        raw = spec.get("absolute_workspace_path") or spec.get("workspace_path")
        if not raw:
            continue
        path = repo_path(str(raw))
        if path.exists():
            paths.append(str(path.resolve()))
    return sorted(paths)


def run_iteration(args: argparse.Namespace, parent_run_id: str, index: int, baseline_paths: set[str]) -> dict[str, Any]:
    iteration_id = f"{parent_run_id}-i{index:03d}"
    before = worktree_snapshot()
    pool = run_script("executor_pool.py", pool_args(args, iteration_id), timeout=args.iteration_timeout_seconds)
    pool_summary = load_json(RUNS_ROOT / iteration_id / "run-summary.json")
    cleanup: dict[str, Any] | None = None
    cleanup_summary: dict[str, Any] | None = None
    if args.cleanup_plan or args.cleanup_worktrees:
        cleanup_run_id = f"{iteration_id}-cleanup"
        cleanup = run_script("workspace_cleanup.py", cleanup_args(args, iteration_id, cleanup_run_id), timeout=args.cleanup_timeout_seconds)
        cleanup_summary = load_json(RUNS_ROOT / cleanup_run_id / "run-summary.json")
    after = worktree_snapshot()
    after_paths = worktree_paths(after)
    new_paths = sorted(after_paths - baseline_paths)
    workspace_leak_paths = existing_workspace_paths(pool_summary)
    leak_paths = sorted(set(new_paths + workspace_leak_paths)) if args.cleanup_worktrees else []
    return {
        "index": index,
        "run_id": iteration_id,
        "before_worktrees": before,
        "after_worktrees": after,
        "new_worktree_paths": new_paths,
        "workspace_leak_paths": workspace_leak_paths,
        "leak_paths": leak_paths,
        "pool": pool,
        "pool_summary": {
            "status": pool_summary.get("status"),
            "workspace_isolation": (pool_summary.get("workspace_isolation") or {}).get("status"),
            "activate_workspaces": pool_summary.get("activate_workspaces"),
            "children": len(pool_summary.get("children") or []),
            "findings": pool_summary.get("findings", []),
        },
        "cleanup": cleanup,
        "cleanup_summary": {
            "status": cleanup_summary.get("status"),
            "execute_requested": cleanup_summary.get("execute_requested"),
            "findings": cleanup_summary.get("findings", []),
            "cleanup_results": cleanup_summary.get("cleanup_results", []),
        } if cleanup_summary else None,
    }


def findings_for(iterations: list[dict[str, Any]]) -> list[dict[str, str]]:
    findings: list[dict[str, str]] = []
    for item in iterations:
        pool = item.get("pool") or {}
        if pool.get("exit_code") != 0:
            findings.append({"level": "blocker", "code": "STRESS_POOL_ITERATION_FAILED", "message": f"Iteration {item['index']} executor_pool exited {pool.get('exit_code')}."})
        cleanup = item.get("cleanup")
        if cleanup and cleanup.get("exit_code") != 0:
            findings.append({"level": "blocker", "code": "STRESS_CLEANUP_ITERATION_FAILED", "message": f"Iteration {item['index']} workspace_cleanup exited {cleanup.get('exit_code')}."})
        if item.get("leak_paths"):
            findings.append({"level": "blocker", "code": "STRESS_WORKSPACE_LEAK", "message": f"Iteration {item['index']} left workspaces: {', '.join(item['leak_paths'])}."})
    return findings


def status_for(findings: list[dict[str, str]], iterations: list[dict[str, Any]]) -> str:
    if any(item.get("level") == "blocker" for item in findings):
        return "EXECUTOR_POOL_STRESS_BLOCKED"
    if not iterations:
        return "EXECUTOR_POOL_STRESS_EMPTY"
    return "EXECUTOR_POOL_STRESS_COMPLETED"


def render_report(summary: dict[str, Any]) -> str:
    lines = [
        "# Codex Loop Executor Pool Stress",
        "",
        f"- run_id: `{summary['run_id']}`",
        f"- status: `{summary['status']}`",
        f"- iterations: `{summary['iterations_requested']}`",
        f"- max_workers: `{summary['max_workers']}`",
        f"- max_tasks: `{summary['max_tasks']}`",
        f"- workspace_backend: `{summary['workspace_backend']}`",
        f"- create_worktrees: `{summary['create_worktrees']}`",
        f"- activate_workspaces: `{summary['activate_workspaces']}`",
        f"- cleanup_worktrees: `{summary['cleanup_worktrees']}`",
        "",
        "## Iterations",
    ]
    for item in summary.get("iterations", []):
        pool_status = (item.get("pool_summary") or {}).get("status")
        cleanup_status = (item.get("cleanup_summary") or {}).get("status")
        lines.append(f"- `{item['run_id']}` pool `{pool_status}` cleanup `{cleanup_status or 'none'}` leaks `{len(item.get('leak_paths') or [])}`")
    if not summary.get("iterations"):
        lines.append("- none")
    lines.extend(["", "## Findings"])
    if summary.get("findings"):
        for item in summary["findings"]:
            lines.append(f"- `{item['level']}` `{item['code']}`: {item['message']}")
    else:
        lines.append("- none")
    lines.extend(
        [
            "",
            "## Guardrail",
            "- Stress runs only call existing gated executor and cleanup tools.",
            "- Workspace creation and cleanup still require their environment gates.",
            "- A cleanup-enabled stress run fails if it leaves new worktrees or workspace directories behind.",
            "",
        ]
    )
    return "\n".join(lines)


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--scheduler-plan", required=True)
    parser.add_argument("--run-id", default=None)
    parser.add_argument("--iterations", type=int, default=3)
    parser.add_argument("--runner", choices=["workflow", "sandbox-plan", "sandbox-execute"], default="sandbox-plan")
    parser.add_argument("--stage", choices=["prepare", "dry-run", "execute-local"], default="prepare")
    parser.add_argument("--max-workers", type=int, default=2)
    parser.add_argument("--max-tasks", type=int, default=1)
    parser.add_argument("--queue-backend", choices=["repo-json", "sqlite", "http"], default="sqlite")
    parser.add_argument("--queue-path", default=None)
    parser.add_argument("--quota-policy", default=str(DEFAULT_QUOTA_POLICY))
    parser.add_argument("--resource-monitor-policy", default=str(DEFAULT_MONITOR_POLICY))
    parser.add_argument("--workspace-isolation-policy", default=str(DEFAULT_ISOLATION_POLICY))
    parser.add_argument("--workspace-backend", choices=["git-worktree", "local-clone"], default="git-worktree")
    parser.add_argument("--sandbox-driver", choices=["kubernetes-job", "local-container"], default="local-container")
    parser.add_argument("--skip-quota", action="store_true")
    parser.add_argument("--skip-resource-monitor", action="store_true")
    parser.add_argument("--create-worktrees", action="store_true")
    parser.add_argument("--activate-workspaces", action="store_true")
    parser.add_argument("--cleanup-plan", action="store_true")
    parser.add_argument("--cleanup-worktrees", action="store_true")
    parser.add_argument("--cleanup-force", action="store_true")
    parser.add_argument("--allow-parallel-execution", action="store_true")
    parser.add_argument("--sandbox-validate", action="store_true")
    parser.add_argument("--child-timeout-seconds", type=int, default=2400)
    parser.add_argument("--iteration-timeout-seconds", type=int, default=3000)
    parser.add_argument("--cleanup-timeout-seconds", type=int, default=600)
    args = parser.parse_args()

    run_id = args.run_id or make_run_id("executor-pool-stress")
    run_dir = ensure_run_dir(run_id)
    out_dir = run_dir / "executor-pool-stress"
    out_dir.mkdir(parents=True, exist_ok=True)
    baseline = worktree_snapshot()
    baseline_paths = worktree_paths(baseline)
    iterations = [run_iteration(args, run_id, index, baseline_paths) for index in range(1, args.iterations + 1)]
    final_snapshot = worktree_snapshot()
    findings = findings_for(iterations)
    summary = {
        "run_id": run_id,
        "run_kind": "executor_pool_stress",
        "status": status_for(findings, iterations),
        "created_at": datetime.now().isoformat(timespec="seconds"),
        "commit": run_git(["rev-parse", "HEAD"]).strip(),
        "scheduler_plan": rel_path(repo_path(args.scheduler_plan)),
        "iterations_requested": args.iterations,
        "runner": args.runner,
        "stage": args.stage,
        "max_workers": args.max_workers,
        "max_tasks": args.max_tasks,
        "queue_backend": args.queue_backend,
        "queue_path": display_path(args.queue_path),
        "workspace_backend": args.workspace_backend,
        "create_worktrees": args.create_worktrees,
        "activate_workspaces": args.activate_workspaces,
        "cleanup_plan": args.cleanup_plan,
        "cleanup_worktrees": args.cleanup_worktrees,
        "cleanup_force": args.cleanup_force,
        "baseline_worktrees": baseline,
        "final_worktrees": final_snapshot,
        "iterations": iterations,
        "findings": findings,
        "outputs": ["executor-pool-stress/stress-summary.json", "executor-pool-stress/stress-report.md"],
    }
    write_json(out_dir / "stress-summary.json", summary)
    write_text(out_dir / "stress-report.md", render_report(summary))
    write_json(run_dir / "run-summary.json", summary)
    print(out_dir)
    print(f"status={summary['status']} iterations={len(iterations)} findings={len(findings)}")
    return 2 if summary["status"] == "EXECUTOR_POOL_STRESS_BLOCKED" else 0


if __name__ == "__main__":
    raise SystemExit(main())
