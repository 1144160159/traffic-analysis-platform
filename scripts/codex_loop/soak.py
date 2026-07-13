#!/usr/bin/env python3
"""Run bounded Codex Loop soak cycles and summarize stability evidence."""

from __future__ import annotations

import argparse
import json
import subprocess
import sys
import time
from datetime import datetime
from pathlib import Path
from typing import Any

from lib import RUNS_ROOT, SCRIPT_ROOT, ensure_run_dir, make_run_id, rel_path, repo_path, run_git, write_json, write_text
from queue_backend import display_path
from resource_monitor import DEFAULT_POLICY as DEFAULT_MONITOR_POLICY


def load_json(path: Path) -> dict[str, Any]:
    if not path.exists():
        return {}
    try:
        return json.loads(path.read_text(encoding="utf-8"))
    except json.JSONDecodeError:
        return {"parse_error": True, "path": rel_path(path)}


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
        return {
            "name": name,
            "command": command,
            "exit_code": proc.returncode,
            "timed_out": False,
            "output_tail": proc.stdout[-4000:],
        }
    except subprocess.TimeoutExpired as exc:
        output = exc.stdout if isinstance(exc.stdout, str) else (exc.stdout or b"").decode("utf-8", errors="replace")
        return {
            "name": name,
            "command": command,
            "exit_code": None,
            "timed_out": True,
            "output_tail": output[-4000:],
        }


def runner_args(args: argparse.Namespace, run_id: str) -> tuple[str, list[str]]:
    common = [
        "--run-id",
        run_id,
        "--max-items",
        str(args.max_items),
        "--worker-stage",
        args.worker_stage,
        "--worker-runner",
        args.worker_runner,
        "--lease-seconds",
        str(args.lease_seconds),
        "--queue-backend",
        args.queue_backend,
        "--sandbox-driver",
        args.sandbox_driver,
        "--sandbox-timeout-seconds",
        str(args.sandbox_timeout_seconds),
    ]
    if args.queue_path:
        common.extend(["--queue-path", args.queue_path])
    if args.sandbox_cleanup:
        common.append("--sandbox-cleanup")
    if args.sandbox_validate:
        common.append("--sandbox-validate")
    if args.mode == "service-once":
        result = ["once", *common, "--profile", args.profile]
        if args.skip_preflight:
            result.append("--skip-preflight")
        return "service.py", result
    result = [
        "--run-id",
        run_id,
        "--iterations",
        "1",
        "--max-items",
        str(args.max_items),
        "--worker-stage",
        args.worker_stage,
        "--worker-runner",
        args.worker_runner,
        "--lease-seconds",
        str(args.lease_seconds),
        "--queue-backend",
        args.queue_backend,
        "--sandbox-driver",
        args.sandbox_driver,
        "--sandbox-timeout-seconds",
        str(args.sandbox_timeout_seconds),
    ]
    if args.queue_path:
        result.extend(["--queue-path", args.queue_path])
    if args.sandbox_cleanup:
        result.append("--sandbox-cleanup")
    if args.sandbox_validate:
        result.append("--sandbox-validate")
    return "daemon.py", result


def health_args(args: argparse.Namespace, run_id: str) -> list[str]:
    result = [
        "health",
        "--run-id",
        run_id,
        "--queue-backend",
        args.queue_backend,
        "--profile",
        args.profile,
    ]
    if args.queue_path:
        result.extend(["--queue-path", args.queue_path])
    if args.skip_health_preflight:
        result.append("--skip-preflight")
    return result


def monitor_args(args: argparse.Namespace, run_id: str) -> list[str]:
    result = [
        "--run-id",
        run_id,
        "--policy",
        args.resource_monitor_policy,
        "--queue-backend",
        args.queue_backend,
    ]
    if args.queue_path:
        result.extend(["--queue-path", args.queue_path])
    if args.resource_sample_seconds is not None:
        result.extend(["--sample-seconds", str(args.resource_sample_seconds)])
    return result


def run_cycle(args: argparse.Namespace, parent_run_id: str, index: int) -> dict[str, Any]:
    cycle_id = f"{parent_run_id}-c{index:03d}"
    runner_script, runner_script_args = runner_args(args, f"{cycle_id}-runner")
    runner = run_script(runner_script, runner_script_args, args.runner_timeout_seconds)
    monitor = None
    monitor_summary = None
    if not args.skip_resource_monitor:
        monitor = run_script("resource_monitor.py", monitor_args(args, f"{cycle_id}-resource-monitor"), args.monitor_timeout_seconds)
        monitor_summary = load_json(RUNS_ROOT / f"{cycle_id}-resource-monitor" / "run-summary.json")
    health = run_script("service.py", health_args(args, f"{cycle_id}-health"), args.health_timeout_seconds)
    health_summary = load_json(RUNS_ROOT / f"{cycle_id}-health" / "run-summary.json")
    metrics = run_script("metrics.py", ["--run-id", f"{cycle_id}-metrics", "--limit", str(args.metrics_limit)], args.metrics_timeout_seconds)
    metrics_summary = load_json(RUNS_ROOT / f"{cycle_id}-metrics" / "run-summary.json")
    return {
        "index": index,
        "cycle_id": cycle_id,
        "runner": runner,
        "runner_summary": load_json(RUNS_ROOT / f"{cycle_id}-runner" / "run-summary.json"),
        "resource_monitor": monitor,
        "resource_monitor_summary": {
            "status": monitor_summary.get("status"),
            "findings": monitor_summary.get("findings", []),
            "admission": monitor_summary.get("admission"),
            "pressure": {
                "cpu_busy_percent": ((monitor_summary.get("pressure") or {}).get("cpu_busy_percent")),
                "load_1_per_cpu_percent": ((monitor_summary.get("pressure") or {}).get("load_1_per_cpu_percent")),
                "mem_available_mb": ((monitor_summary.get("pressure") or {}).get("mem_available_mb")),
                "queue_total": ((monitor_summary.get("pressure") or {}).get("queue_total")),
                "queue_claimed": ((monitor_summary.get("pressure") or {}).get("queue_claimed")),
            },
        } if monitor_summary else None,
        "health": health,
        "health_summary": {
            "status": health_summary.get("status"),
            "findings": ((health_summary.get("health") or {}).get("findings") or health_summary.get("findings") or []),
        },
        "metrics": metrics,
        "metrics_summary": {
            "status": metrics_summary.get("status"),
        },
        "finished_at": datetime.now().isoformat(timespec="seconds"),
    }


def findings_for(cycles: list[dict[str, Any]], max_failures: int) -> list[dict[str, str]]:
    findings: list[dict[str, str]] = []
    failures = 0
    for item in cycles:
        index = item["index"]
        runner = item.get("runner") or {}
        if runner.get("exit_code") != 0:
            failures += 1
            findings.append({"level": "blocker", "code": "SOAK_RUNNER_FAILED", "message": f"Cycle {index} runner exited {runner.get('exit_code')}."})
        monitor = item.get("resource_monitor")
        if monitor and monitor.get("exit_code") not in {0}:
            failures += 1
            findings.append({"level": "blocker", "code": "SOAK_RESOURCE_MONITOR_FAILED", "message": f"Cycle {index} resource monitor exited {monitor.get('exit_code')}."})
        monitor_status = ((item.get("resource_monitor_summary") or {}).get("status"))
        if monitor_status == "RESOURCE_MONITOR_BLOCKED":
            failures += 1
            findings.append({"level": "blocker", "code": "SOAK_RESOURCE_MONITOR_BLOCKED", "message": f"Cycle {index} resource monitor blocked new work."})
        elif monitor_status == "RESOURCE_MONITOR_DEGRADED":
            findings.append({"level": "warning", "code": "SOAK_RESOURCE_MONITOR_DEGRADED", "message": f"Cycle {index} resource monitor recommended degraded admission."})
        health = item.get("health") or {}
        if health.get("exit_code") != 0:
            failures += 1
            findings.append({"level": "blocker", "code": "SOAK_HEALTH_FAILED", "message": f"Cycle {index} service health exited {health.get('exit_code')}."})
        health_status = ((item.get("health_summary") or {}).get("status"))
        if health_status == "SERVICE_UNHEALTHY":
            failures += 1
            findings.append({"level": "blocker", "code": "SOAK_SERVICE_UNHEALTHY", "message": f"Cycle {index} service health is unhealthy."})
        elif health_status == "SERVICE_DEGRADED":
            findings.append({"level": "warning", "code": "SOAK_SERVICE_DEGRADED", "message": f"Cycle {index} service health is degraded."})
        metrics = item.get("metrics") or {}
        if metrics.get("exit_code") != 0:
            failures += 1
            findings.append({"level": "blocker", "code": "SOAK_METRICS_FAILED", "message": f"Cycle {index} metrics exited {metrics.get('exit_code')}."})
    if failures > max_failures:
        findings.append({"level": "blocker", "code": "SOAK_FAILURE_BUDGET_EXCEEDED", "message": f"Soak failures {failures} exceeded budget {max_failures}."})
    return findings


def status_for(cycles: list[dict[str, Any]], findings: list[dict[str, str]]) -> str:
    if any(item.get("level") == "blocker" for item in findings):
        return "SOAK_BLOCKED"
    if not cycles:
        return "SOAK_EMPTY"
    if findings:
        return "SOAK_DEGRADED"
    return "SOAK_COMPLETED"


def render_report(summary: dict[str, Any]) -> str:
    lines = [
        "# Codex Loop Soak",
        "",
        f"- run_id: `{summary['run_id']}`",
        f"- status: `{summary['status']}`",
        f"- mode: `{summary['mode']}`",
        f"- cycles_requested: `{summary['cycles_requested']}`",
        f"- cycles_completed: `{len(summary.get('cycles') or [])}`",
        f"- interval_seconds: `{summary['interval_seconds']}`",
        f"- max_failures: `{summary['max_failures']}`",
        f"- worker_runner: `{summary['worker_runner']}`",
        f"- worker_stage: `{summary['worker_stage']}`",
        f"- queue_backend: `{summary['queue_backend']}`",
        "",
        "## Cycles",
    ]
    cycles = summary.get("cycles") or []
    if cycles:
        for item in cycles:
            monitor_status = ((item.get("resource_monitor_summary") or {}).get("status") or "skipped")
            health_status = ((item.get("health_summary") or {}).get("status") or "unknown")
            metrics_status = ((item.get("metrics_summary") or {}).get("status") or "unknown")
            runner_status = ((item.get("runner_summary") or {}).get("status") or "unknown")
            lines.append(
                f"- `{item['cycle_id']}` runner `{runner_status}` monitor `{monitor_status}` health `{health_status}` metrics `{metrics_status}`"
            )
    else:
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
            "- Soak composes bounded service/daemon cycles, resource monitoring, health and metrics.",
            "- It does not bypass worker, sandbox, queue, preflight or external Codex execution gates.",
            "- A bounded soak run is stability evidence; it does not close product tasks by itself.",
            "",
        ]
    )
    return "\n".join(lines)


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--run-id", default=None)
    parser.add_argument("--mode", choices=["service-once", "daemon"], default="service-once")
    parser.add_argument("--cycles", type=int, default=3)
    parser.add_argument("--interval-seconds", type=float, default=0)
    parser.add_argument("--max-failures", type=int, default=0)
    parser.add_argument("--max-items", type=int, default=1)
    parser.add_argument("--worker-stage", choices=["prepare", "dry-run", "execute-local"], default="prepare")
    parser.add_argument("--worker-runner", choices=["workflow", "sandbox-plan", "sandbox-execute"], default="sandbox-plan")
    parser.add_argument("--sandbox-driver", choices=["kubernetes-job", "local-container"], default="local-container")
    parser.add_argument("--sandbox-timeout-seconds", type=int, default=1800)
    parser.add_argument("--sandbox-cleanup", action="store_true")
    parser.add_argument("--sandbox-validate", action="store_true")
    parser.add_argument("--lease-seconds", type=int, default=3600)
    parser.add_argument("--queue-backend", choices=["repo-json", "sqlite", "http"], default="sqlite")
    parser.add_argument("--queue-path", default=None)
    parser.add_argument("--profile", default="conservative")
    parser.add_argument("--skip-preflight", action="store_true")
    parser.add_argument("--skip-health-preflight", action="store_true")
    parser.add_argument("--skip-resource-monitor", action="store_true")
    parser.add_argument("--resource-monitor-policy", default=str(DEFAULT_MONITOR_POLICY))
    parser.add_argument("--resource-sample-seconds", type=int, default=None)
    parser.add_argument("--metrics-limit", type=int, default=20)
    parser.add_argument("--runner-timeout-seconds", type=int, default=3600)
    parser.add_argument("--monitor-timeout-seconds", type=int, default=600)
    parser.add_argument("--health-timeout-seconds", type=int, default=600)
    parser.add_argument("--metrics-timeout-seconds", type=int, default=600)
    args = parser.parse_args()

    run_id = args.run_id or make_run_id("soak")
    run_dir = ensure_run_dir(run_id)
    out_dir = run_dir / "soak"
    out_dir.mkdir(parents=True, exist_ok=True)
    cycles: list[dict[str, Any]] = []
    for index in range(1, args.cycles + 1):
        cycles.append(run_cycle(args, run_id, index))
        interim_findings = findings_for(cycles, args.max_failures)
        if any(item.get("level") == "blocker" for item in interim_findings):
            break
        if index < args.cycles and args.interval_seconds > 0:
            time.sleep(args.interval_seconds)
    findings = findings_for(cycles, args.max_failures)
    summary = {
        "run_id": run_id,
        "run_kind": "soak",
        "status": status_for(cycles, findings),
        "created_at": datetime.now().isoformat(timespec="seconds"),
        "commit": run_git(["rev-parse", "HEAD"]).strip(),
        "mode": args.mode,
        "cycles_requested": args.cycles,
        "cycles_completed": len(cycles),
        "interval_seconds": args.interval_seconds,
        "max_failures": args.max_failures,
        "max_items": args.max_items,
        "worker_stage": args.worker_stage,
        "worker_runner": args.worker_runner,
        "sandbox_driver": args.sandbox_driver,
        "sandbox_timeout_seconds": args.sandbox_timeout_seconds,
        "sandbox_cleanup": args.sandbox_cleanup,
        "sandbox_validate": args.sandbox_validate,
        "queue_backend": args.queue_backend,
        "queue_path": display_path(args.queue_path),
        "profile": args.profile,
        "skip_preflight": args.skip_preflight,
        "skip_health_preflight": args.skip_health_preflight,
        "skip_resource_monitor": args.skip_resource_monitor,
        "cycles": cycles,
        "findings": findings,
        "outputs": ["soak/soak-summary.json", "soak/soak-report.md"],
    }
    write_json(out_dir / "soak-summary.json", summary)
    write_text(out_dir / "soak-report.md", render_report(summary))
    write_json(run_dir / "run-summary.json", summary)
    print(out_dir)
    print(f"status={summary['status']} cycles={len(cycles)} findings={len(findings)}")
    return 2 if summary["status"] == "SOAK_BLOCKED" else 0


if __name__ == "__main__":
    raise SystemExit(main())
