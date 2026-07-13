#!/usr/bin/env python3
"""Run a bounded Codex Loop daemon cycle."""

from __future__ import annotations

import argparse
import json
import subprocess
import sys
from datetime import datetime
from pathlib import Path
from typing import Any

from lib import SCRIPT_ROOT, ensure_run_dir, make_run_id, rel_path, repo_path, run_git, write_json, write_text
from lock_manager import release_lock


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


def render_report(summary: dict[str, Any]) -> str:
    lines = [
        "# Codex Loop Daemon Report",
        "",
        f"- run_id: `{summary['run_id']}`",
        f"- status: `{summary['status']}`",
        f"- iterations: `{len(summary['iterations'])}`",
        f"- worker_runner: `{summary['worker_runner']}`",
        f"- worker_stage: `{summary['worker_stage']}`",
        f"- objective_stop_status: `{summary.get('objective_stop_status') or 'none'}`",
        f"- stop_reason: `{summary.get('stop_reason') or 'none'}`",
        "",
        "## Iterations",
    ]
    for item in summary["iterations"]:
        lines.append(f"- iteration `{item['iteration']}` status `{item['status']}`")
        for step in item["steps"]:
            lines.append(f"  - `{step['name']}` exit `{step['exit_code']}`")
        if item.get("objective_stop_status"):
            lines.append(f"  - objective_stop_status `{item['objective_stop_status']}` recommendation `{item.get('objective_stop_recommendation')}`")
    lines.extend(
        [
            "",
            "## Guardrail",
            "- Daemon cycles are bounded by --iterations.",
            "- Default worker runner is the local workflow worker and default stage is prepare.",
            "- Sandbox execution requires --worker-runner sandbox-execute plus sandbox_executor.py environment gates.",
            "",
        ]
    )
    return "\n".join(lines)


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--run-id", default=None)
    parser.add_argument("--iterations", type=int, default=1)
    parser.add_argument("--max-items", type=int, default=1)
    parser.add_argument("--worker-stage", choices=["prepare", "dry-run", "execute-local"], default="prepare")
    parser.add_argument("--worker-runner", choices=["workflow", "sandbox-plan", "sandbox-execute"], default="workflow")
    parser.add_argument("--sandbox-driver", choices=["kubernetes-job", "local-container"], default="kubernetes-job")
    parser.add_argument("--sandbox-timeout-seconds", type=int, default=1800)
    parser.add_argument("--sandbox-cleanup", action="store_true")
    parser.add_argument("--sandbox-validate", action="store_true")
    parser.add_argument("--lease-seconds", type=int, default=3600)
    parser.add_argument("--queue-backend", choices=["repo-json", "sqlite", "http"], default="repo-json")
    parser.add_argument("--queue-path", default=None)
    parser.add_argument("--check-objective-stop", action="store_true")
    parser.add_argument("--stop-on-objective", action="store_true")
    parser.add_argument("--objective", default=None, help="Human-readable objective text passed to objective_stop.py.")
    parser.add_argument("--objective-stop-policy", default="scripts/codex_loop/policies/objective-stop.yaml")
    parser.add_argument("--objective-stop-release", default=None, help="Release manifest path passed to objective_stop.py.")
    parser.add_argument("--objective-stop-release-run-id", default=None, help="Use doc/02_acceptance/runs/<id>/release/release-manifest.json.")
    parser.add_argument("--keep-lock", action="store_true")
    args = parser.parse_args()

    run_id = args.run_id or make_run_id("daemon")
    run_dir = ensure_run_dir(run_id)
    out_dir = run_dir / "daemon"
    out_dir.mkdir(parents=True, exist_ok=True)
    iterations: list[dict[str, Any]] = []
    failed = False
    objective_stop_status = None
    objective_stop_recommendation = None
    stop_reason = None

    for index in range(1, args.iterations + 1):
        suffix = f"{run_id}-i{index}"
        scout_run = f"{suffix}-scout"
        scheduler_run = f"{suffix}-scheduler"
        worker_run = f"{suffix}-worker"
        metrics_run = f"{suffix}-metrics"
        steps: list[dict[str, Any]] = []
        steps.append(run_script("scout.py", ["--run-id", scout_run]))
        context_dir = f"doc/02_acceptance/runs/{scout_run}/context"
        steps.append(run_script("guide.py", ["--context-dir", context_dir, "--run-id", suffix]))
        guidance = f"doc/02_acceptance/runs/{suffix}/guidance/guidance.json"
        scheduler_args = [
            "--context-dir",
            context_dir,
            "--guidance",
            guidance,
            "--run-id",
            scheduler_run,
            "--max-items",
            str(args.max_items),
            "--acquire-lock",
            "--lease-seconds",
            str(args.lease_seconds),
            "--persist-queue",
            "--queue-backend",
            args.queue_backend,
        ]
        if args.queue_path:
            scheduler_args.extend(["--queue-path", args.queue_path])
        steps.append(run_script("scheduler.py", scheduler_args))
        scheduler_plan = f"doc/02_acceptance/runs/{scheduler_run}/scheduler/scheduler-plan.json"
        if steps[-1]["exit_code"] == 0:
            if args.worker_runner == "workflow":
                worker_script = "worker.py"
                worker_args = [
                    "--scheduler-plan",
                    scheduler_plan,
                    "--run-id",
                    worker_run,
                    "--stage",
                    args.worker_stage,
                    "--max-tasks",
                    str(args.max_items),
                    "--require-lock",
                    "--claim-queue",
                    "--queue-backend",
                    args.queue_backend,
                ]
            else:
                worker_script = "sandbox_worker.py"
                worker_args = [
                    "--scheduler-plan",
                    scheduler_plan,
                    "--run-id",
                    worker_run,
                    "--stage",
                    args.worker_stage,
                    "--max-tasks",
                    str(args.max_items),
                    "--require-lock",
                    "--driver",
                    args.sandbox_driver,
                    "--timeout-seconds",
                    str(args.sandbox_timeout_seconds),
                    "--queue-backend",
                    args.queue_backend,
                ]
                if args.worker_runner == "sandbox-execute":
                    worker_args.extend(["--execute-sandbox", "--claim-queue"])
                if args.sandbox_cleanup:
                    worker_args.append("--cleanup")
                if args.sandbox_validate:
                    worker_args.append("--validate")
            if args.queue_path:
                worker_args.extend(["--queue-path", args.queue_path])
            steps.append(run_script(worker_script, worker_args))
        if not args.keep_lock:
            release_lock(scheduler_run)
        steps.append(run_script("metrics.py", ["--run-id", metrics_run, "--limit", "20"]))
        if args.check_objective_stop:
            objective_run = f"{suffix}-objective-stop"
            objective_args = [
                "--run-id",
                objective_run,
                "--context-dir",
                context_dir,
                "--guidance",
                guidance,
                "--policy",
                args.objective_stop_policy,
            ]
            if args.objective:
                objective_args.extend(["--objective", args.objective])
            if args.objective_stop_release:
                objective_args.extend(["--release", args.objective_stop_release])
            if args.objective_stop_release_run_id:
                objective_args.extend(["--release-run-id", args.objective_stop_release_run_id])
            steps.append(run_script("objective_stop.py", objective_args))
            objective_summary = repo_path(f"doc/02_acceptance/runs/{objective_run}/run-summary.json")
            if objective_summary.exists():
                data = json.loads(objective_summary.read_text(encoding="utf-8"))
                objective_stop_status = data.get("status")
                objective_stop_recommendation = data.get("stop_recommendation")
        status = "ITERATION_COMPLETED" if all(step["exit_code"] == 0 for step in steps) else "ITERATION_FAILED"
        if args.stop_on_objective and objective_stop_status in {"OBJECTIVE_STOP_READY", "OBJECTIVE_STOP_BLOCKED", "OBJECTIVE_STOP_HUMAN_GATE"}:
            status = "ITERATION_OBJECTIVE_STOPPED"
            stop_reason = str(objective_stop_status)
        failed = failed or status == "ITERATION_FAILED"
        iterations.append(
            {
                "iteration": index,
                "status": status,
                "steps": steps,
                "objective_stop_status": objective_stop_status,
                "objective_stop_recommendation": objective_stop_recommendation,
            }
        )
        if failed or stop_reason:
            break

    daemon_status = "DAEMON_FAILED" if failed else "DAEMON_COMPLETED"
    if stop_reason == "OBJECTIVE_STOP_READY":
        daemon_status = "DAEMON_OBJECTIVE_READY"
    elif stop_reason in {"OBJECTIVE_STOP_BLOCKED", "OBJECTIVE_STOP_HUMAN_GATE"}:
        daemon_status = "DAEMON_OBJECTIVE_STOPPED"

    summary = {
        "run_id": run_id,
        "run_kind": "daemon",
        "status": daemon_status,
        "created_at": datetime.now().isoformat(timespec="seconds"),
        "commit": run_git(["rev-parse", "HEAD"]).strip(),
        "worker_runner": args.worker_runner,
        "worker_stage": args.worker_stage,
        "sandbox_driver": args.sandbox_driver,
        "sandbox_validate": args.sandbox_validate,
        "sandbox_cleanup": args.sandbox_cleanup,
        "sandbox_timeout_seconds": args.sandbox_timeout_seconds,
        "queue_backend": args.queue_backend,
        "queue_path": args.queue_path,
        "check_objective_stop": args.check_objective_stop,
        "stop_on_objective": args.stop_on_objective,
        "objective": args.objective,
        "objective_stop_policy": args.objective_stop_policy,
        "objective_stop_release": args.objective_stop_release,
        "objective_stop_release_run_id": args.objective_stop_release_run_id,
        "objective_stop_status": objective_stop_status,
        "objective_stop_recommendation": objective_stop_recommendation,
        "stop_reason": stop_reason,
        "iterations": iterations,
        "outputs": ["daemon/daemon-summary.json", "daemon/daemon-report.md"],
    }
    write_json(out_dir / "daemon-summary.json", summary)
    write_text(out_dir / "daemon-report.md", render_report(summary))
    write_json(run_dir / "run-summary.json", summary)
    print(out_dir)
    print(f"status={summary['status']} iterations={len(iterations)}")
    return 1 if failed else 0


if __name__ == "__main__":
    raise SystemExit(main())
