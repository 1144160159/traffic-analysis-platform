#!/usr/bin/env python3
"""Repo-native supervisor for Codex Loop daemon cycles."""

from __future__ import annotations

import argparse
import json
import os
import signal
import subprocess
import sys
import time
from datetime import datetime
from pathlib import Path
from typing import Any

from lib import RUNS_ROOT, SCRIPT_ROOT, ensure_run_dir, make_run_id, rel_path, repo_path, run_git, write_json, write_text
from lock_manager import archive_expired_lock, lock_expired, read_lock
from metrics import build_metrics
from preflight import build_preflight, write_preflight_artifacts
from queue_backend import display_path, queue_status


LOOP_ROOT = RUNS_ROOT / ".loop"
STATE_PATH = LOOP_ROOT / "service-state.json"
STOP_PATH = LOOP_ROOT / "service.stop"


def now_iso() -> str:
    return datetime.now().isoformat(timespec="seconds")


def load_json(path: Path) -> dict[str, Any]:
    if not path.exists():
        return {}
    try:
        return json.loads(path.read_text(encoding="utf-8"))
    except json.JSONDecodeError:
        return {"parse_error": True, "raw": path.read_text(encoding="utf-8")[:1000]}


def save_state(state: dict[str, Any]) -> Path:
    state["updated_at"] = now_iso()
    LOOP_ROOT.mkdir(parents=True, exist_ok=True)
    write_json(STATE_PATH, state)
    return STATE_PATH


def read_state() -> dict[str, Any]:
    return load_json(STATE_PATH)


def pid_alive(pid: int | None) -> bool:
    if not pid or pid <= 0:
        return False
    try:
        os.kill(pid, 0)
    except ProcessLookupError:
        return False
    except PermissionError:
        return True
    return True


def pid_cmdline(pid: int | None) -> str:
    if not pid:
        return ""
    proc_path = Path("/proc") / str(pid) / "cmdline"
    try:
        return proc_path.read_bytes().replace(b"\x00", b" ").decode("utf-8", errors="replace")
    except OSError:
        return ""


def run_daemon_cycle(
    run_id: str,
    max_items: int,
    worker_stage: str,
    worker_runner: str,
    lease_seconds: int,
    queue_backend: str,
    queue_path: str | None,
    sandbox_driver: str,
    sandbox_timeout_seconds: int,
    sandbox_cleanup: bool,
    sandbox_validate: bool,
    check_objective_stop: bool,
    stop_on_objective: bool,
    objective: str | None,
    objective_stop_policy: str,
    objective_stop_release: str | None,
    objective_stop_release_run_id: str | None,
) -> dict[str, Any]:
    command = [
        sys.executable,
        "-B",
        str(SCRIPT_ROOT / "daemon.py"),
        "--run-id",
        run_id,
        "--iterations",
        "1",
        "--max-items",
        str(max_items),
        "--worker-stage",
        worker_stage,
        "--worker-runner",
        worker_runner,
        "--lease-seconds",
        str(lease_seconds),
        "--queue-backend",
        queue_backend,
        "--sandbox-driver",
        sandbox_driver,
        "--sandbox-timeout-seconds",
        str(sandbox_timeout_seconds),
    ]
    if sandbox_cleanup:
        command.append("--sandbox-cleanup")
    if sandbox_validate:
        command.append("--sandbox-validate")
    if queue_path:
        command.extend(["--queue-path", queue_path])
    if check_objective_stop:
        command.append("--check-objective-stop")
    if stop_on_objective:
        command.append("--stop-on-objective")
    if objective:
        command.extend(["--objective", objective])
    if objective_stop_policy:
        command.extend(["--objective-stop-policy", objective_stop_policy])
    if objective_stop_release:
        command.extend(["--objective-stop-release", objective_stop_release])
    if objective_stop_release_run_id:
        command.extend(["--objective-stop-release-run-id", objective_stop_release_run_id])
    proc = subprocess.run(
        command,
        cwd=repo_path("."),
        text=True,
        stdout=subprocess.PIPE,
        stderr=subprocess.STDOUT,
        check=False,
    )
    daemon_summary = load_json(RUNS_ROOT / run_id / "run-summary.json")
    return {
        "run_id": run_id,
        "command": command,
        "exit_code": proc.returncode,
        "output_tail": proc.stdout[-4000:],
        "daemon_summary": {
            "status": daemon_summary.get("status"),
            "objective_stop_status": daemon_summary.get("objective_stop_status"),
            "objective_stop_recommendation": daemon_summary.get("objective_stop_recommendation"),
            "stop_reason": daemon_summary.get("stop_reason"),
        },
        "finished_at": now_iso(),
    }


def render_service_report(summary: dict[str, Any]) -> str:
    preflight = summary.get("preflight") or {}
    lines = [
        "# Codex Loop Service Report",
        "",
        f"- run_id: `{summary['run_id']}`",
        f"- status: `{summary['status']}`",
        f"- mode: `{summary['mode']}`",
        f"- cycles: `{len(summary.get('cycles') or [])}`",
        f"- max_items: `{summary.get('max_items')}`",
        f"- worker_runner: `{summary.get('worker_runner')}`",
        f"- worker_stage: `{summary.get('worker_stage')}`",
        f"- check_objective_stop: `{summary.get('check_objective_stop')}`",
        f"- stop_on_objective: `{summary.get('stop_on_objective')}`",
        f"- preflight: `{preflight.get('status', 'skipped')}`",
        "",
        "## Cycles",
    ]
    cycles = summary.get("cycles") or []
    if cycles:
        for item in cycles:
            lines.append(f"- `{item['run_id']}` exit `{item['exit_code']}`")
            daemon_summary = item.get("daemon_summary") or {}
            if daemon_summary.get("objective_stop_status"):
                lines.append(
                    f"  - objective_stop_status `{daemon_summary.get('objective_stop_status')}` recommendation `{daemon_summary.get('objective_stop_recommendation')}`"
                )
    else:
        lines.append("- none")
    if summary.get("stop_requested"):
        lines.extend(["", "## Stop", "- stop requested"])
    if preflight.get("findings"):
        lines.extend(["", "## Preflight Findings"])
        for item in preflight["findings"]:
            lines.append(f"- `{item['level']}` `{item['code']}`: {item['message']}")
    lines.extend(
        [
            "",
            "## Guardrail",
            "- Service supervisor repeats bounded daemon cycles; daemon safety gates still apply.",
            "- Default worker stage is prepare and does not run live writes or external Codex commands.",
            "",
        ]
    )
    return "\n".join(lines)


def render_health_report(health: dict[str, Any]) -> str:
    service = health.get("service") or {}
    queue = health.get("queue") or {}
    lock = health.get("lock") or {}
    preflight = health.get("preflight") or {}
    lines = [
        "# Codex Loop Service Health",
        "",
        f"- status: `{health['status']}`",
        f"- generated_at: `{health['generated_at']}`",
        f"- service_running: `{service.get('running')}`",
        f"- service_status: `{service.get('status')}`",
        f"- preflight: `{preflight.get('status', 'skipped')}`",
        f"- queue_counts: `{queue.get('counts')}`",
        f"- lock_present: `{lock.get('present')}`",
        f"- lock_expired: `{lock.get('expired')}`",
        "",
        "## Findings",
    ]
    findings = health.get("findings") or []
    if findings:
        for item in findings:
            lines.append(f"- `{item['level']}` `{item['code']}`: {item['message']}")
    else:
        lines.append("- none")
    if preflight.get("findings"):
        lines.extend(["", "## Preflight Findings"])
        for item in preflight["findings"]:
            lines.append(f"- `{item['level']}` `{item['code']}`: {item['message']}")
    lines.extend(["", "## Guardrail", "- Health is operational evidence only; it does not close product tasks.", ""])
    return "\n".join(lines)


def service_status(expect_running: bool = False) -> dict[str, Any]:
    state = read_state()
    pid = int(state.get("pid") or 0) if state.get("pid") else None
    running = pid_alive(pid)
    command = pid_cmdline(pid)
    findings: list[dict[str, str]] = []
    if expect_running and not running:
        findings.append({"level": "blocker", "code": "SERVICE_NOT_RUNNING", "message": "Expected service process is not running."})
    if running and "scripts/codex_loop/service.py" not in command and "service.py" not in command:
        findings.append({"level": "warning", "code": "PID_CMDLINE_UNEXPECTED", "message": "Recorded PID is alive but command line is unexpected."})
    return {
        "state_path": rel_path(STATE_PATH),
        "stop_path": rel_path(STOP_PATH),
        "state_exists": STATE_PATH.exists(),
        "stop_requested": STOP_PATH.exists(),
        "running": running,
        "pid": pid,
        "pid_cmdline": command,
        "status": state.get("status", "UNKNOWN"),
        "heartbeat_at": state.get("heartbeat_at"),
        "last_cycle": state.get("last_cycle"),
        "state": state,
        "findings": findings,
    }


def preflight_digest(preflight: dict[str, Any] | None) -> dict[str, Any] | None:
    if not preflight:
        return None
    return {
        "status": preflight.get("status"),
        "generated_at": preflight.get("generated_at"),
        "profile": (preflight.get("profile") or {}).get("name"),
        "queue": preflight.get("queue"),
        "resources": preflight.get("resources"),
        "findings": preflight.get("findings", []),
    }


def preflight_blocked(preflight: dict[str, Any] | None) -> bool:
    return bool(preflight and preflight.get("status") == "RUNTIME_PREFLIGHT_BLOCKED")


def build_health(
    expect_running: bool = False,
    queue_backend: str = "repo-json",
    queue_path: str | None = None,
    profile: str = "conservative",
    skip_preflight: bool = False,
) -> dict[str, Any]:
    service = service_status(expect_running)
    queue = queue_status(backend=queue_backend, path=queue_path, include_items=False)
    current_lock = read_lock()
    metrics = build_metrics(limit=10)
    preflight = None if skip_preflight else build_preflight(profile_name=profile, queue_backend=queue_backend, queue_path=queue_path)
    findings = list(service.get("findings") or [])
    if current_lock and lock_expired(current_lock):
        findings.append({"level": "blocker", "code": "WORKSPACE_LOCK_EXPIRED", "message": "Workspace lock has expired and should be recovered."})
    if queue.get("counts", {}).get("quarantined"):
        findings.append({"level": "warning", "code": "QUEUE_QUARANTINED", "message": "One or more queue items are quarantined."})
    if preflight_blocked(preflight):
        findings.append({"level": "blocker", "code": "RUNTIME_PREFLIGHT_BLOCKED", "message": "Runtime preflight has blocker findings."})
    status = "HEALTHY"
    if any(item.get("level") == "blocker" for item in findings):
        status = "UNHEALTHY"
    elif findings:
        status = "DEGRADED"
    return {
        "generated_at": now_iso(),
        "status": status,
        "service": service,
        "queue": {key: value for key, value in queue.items() if key != "items"},
        "queue_backend": queue_backend,
        "queue_path": display_path(queue_path),
        "preflight": preflight_digest(preflight),
        "lock": {
            "present": bool(current_lock),
            "expired": lock_expired(current_lock) if current_lock else False,
            "payload": current_lock,
        },
        "metrics": {
            "runs_total": metrics["runs"]["total"],
            "runs_by_status": metrics["runs"]["by_status"],
            "runs_by_kind": metrics["runs"]["by_kind"],
        },
        "findings": findings,
    }


def write_service_run(run_id: str, summary: dict[str, Any]) -> Path:
    run_dir = ensure_run_dir(run_id)
    out_dir = run_dir / "service"
    out_dir.mkdir(parents=True, exist_ok=True)
    write_json(out_dir / "service-summary.json", summary)
    write_text(out_dir / "service-report.md", render_service_report(summary))
    outputs = ["service/service-summary.json", "service/service-report.md", *summary.get("extra_outputs", [])]
    write_json(
        run_dir / "run-summary.json",
        {
            "run_id": run_id,
            "run_kind": "service",
            "status": summary["status"],
            "created_at": summary["created_at"],
            "commit": summary["commit"],
            "outputs": outputs,
        },
    )
    return out_dir


def write_health_run(run_id: str, health: dict[str, Any]) -> Path:
    run_dir = ensure_run_dir(run_id)
    out_dir = run_dir / "service"
    out_dir.mkdir(parents=True, exist_ok=True)
    write_json(out_dir / "health.json", health)
    write_text(out_dir / "health.md", render_health_report(health))
    write_json(
        run_dir / "run-summary.json",
        {
            "run_id": run_id,
            "run_kind": "service_health",
            "status": f"SERVICE_{health['status']}",
            "created_at": health["generated_at"],
            "commit": run_git(["rev-parse", "HEAD"]).strip(),
            "outputs": ["service/health.json", "service/health.md"],
        },
    )
    return out_dir


def run_once(args: argparse.Namespace) -> int:
    run_id = args.run_id or make_run_id("service-once")
    preflight = None if args.skip_preflight else build_preflight(profile_name=args.profile, queue_backend=args.queue_backend, queue_path=args.queue_path)
    extra_outputs: list[str] = []
    if preflight:
        write_preflight_artifacts(run_id, preflight, write_summary=False)
        extra_outputs.extend(["preflight/preflight.json", "preflight/preflight.md"])
    if preflight_blocked(preflight):
        status = "SERVICE_PREFLIGHT_BLOCKED"
        summary = {
            "run_id": run_id,
            "run_kind": "service",
            "status": status,
            "mode": "once",
            "created_at": now_iso(),
            "commit": run_git(["rev-parse", "HEAD"]).strip(),
            "max_items": args.max_items,
            "worker_runner": args.worker_runner,
            "worker_stage": args.worker_stage,
            "sandbox_driver": args.sandbox_driver,
            "sandbox_timeout_seconds": args.sandbox_timeout_seconds,
            "sandbox_cleanup": args.sandbox_cleanup,
            "sandbox_validate": args.sandbox_validate,
            "check_objective_stop": args.check_objective_stop,
            "stop_on_objective": args.stop_on_objective,
            "objective": args.objective,
            "objective_stop_policy": args.objective_stop_policy,
            "objective_stop_release": args.objective_stop_release,
            "objective_stop_release_run_id": args.objective_stop_release_run_id,
            "lease_seconds": args.lease_seconds,
            "queue_backend": args.queue_backend,
            "queue_path": display_path(args.queue_path),
            "preflight": preflight_digest(preflight),
            "extra_outputs": extra_outputs,
            "cycles": [],
        }
        out_dir = write_service_run(run_id, summary)
        print(out_dir)
        print(f"status={status} cycles=0")
        return 1
    child_run = f"{run_id}-daemon"
    cycle = run_daemon_cycle(
        child_run,
        args.max_items,
        args.worker_stage,
        args.worker_runner,
        args.lease_seconds,
        args.queue_backend,
        args.queue_path,
        args.sandbox_driver,
        args.sandbox_timeout_seconds,
        args.sandbox_cleanup,
        args.sandbox_validate,
        args.check_objective_stop,
        args.stop_on_objective,
        args.objective,
        args.objective_stop_policy,
        args.objective_stop_release,
        args.objective_stop_release_run_id,
    )
    daemon_status = (cycle.get("daemon_summary") or {}).get("status")
    if daemon_status == "DAEMON_OBJECTIVE_READY":
        status = "SERVICE_OBJECTIVE_READY"
    elif daemon_status == "DAEMON_OBJECTIVE_STOPPED":
        status = "SERVICE_OBJECTIVE_STOPPED"
    else:
        status = "SERVICE_ONCE_COMPLETED" if cycle["exit_code"] == 0 else "SERVICE_ONCE_FAILED"
    summary = {
        "run_id": run_id,
        "run_kind": "service",
        "status": status,
        "mode": "once",
        "created_at": now_iso(),
        "commit": run_git(["rev-parse", "HEAD"]).strip(),
        "max_items": args.max_items,
        "worker_runner": args.worker_runner,
        "worker_stage": args.worker_stage,
        "sandbox_driver": args.sandbox_driver,
        "sandbox_timeout_seconds": args.sandbox_timeout_seconds,
        "sandbox_cleanup": args.sandbox_cleanup,
        "sandbox_validate": args.sandbox_validate,
        "check_objective_stop": args.check_objective_stop,
        "stop_on_objective": args.stop_on_objective,
        "objective": args.objective,
        "objective_stop_policy": args.objective_stop_policy,
        "objective_stop_release": args.objective_stop_release,
        "objective_stop_release_run_id": args.objective_stop_release_run_id,
        "lease_seconds": args.lease_seconds,
        "queue_backend": args.queue_backend,
        "queue_path": display_path(args.queue_path),
        "preflight": preflight_digest(preflight),
        "extra_outputs": extra_outputs,
        "cycles": [cycle],
    }
    out_dir = write_service_run(run_id, summary)
    print(out_dir)
    print(f"status={status} cycles=1")
    return 0 if cycle["exit_code"] == 0 else 1


def run_service(args: argparse.Namespace) -> int:
    run_id = args.run_id or make_run_id("service")
    preflight = None if args.skip_preflight else build_preflight(profile_name=args.profile, queue_backend=args.queue_backend, queue_path=args.queue_path)
    extra_outputs: list[str] = []
    if preflight:
        write_preflight_artifacts(run_id, preflight, write_summary=False)
        extra_outputs.extend(["preflight/preflight.json", "preflight/preflight.md"])
    if preflight_blocked(preflight):
        summary = {
            "run_id": run_id,
            "run_kind": "service",
            "status": "SERVICE_PREFLIGHT_BLOCKED",
            "mode": "run",
            "created_at": now_iso(),
            "commit": run_git(["rev-parse", "HEAD"]).strip(),
            "max_items": args.max_items,
            "worker_runner": args.worker_runner,
            "worker_stage": args.worker_stage,
            "sandbox_driver": args.sandbox_driver,
            "sandbox_timeout_seconds": args.sandbox_timeout_seconds,
            "sandbox_cleanup": args.sandbox_cleanup,
            "sandbox_validate": args.sandbox_validate,
            "check_objective_stop": args.check_objective_stop,
            "stop_on_objective": args.stop_on_objective,
            "objective": args.objective,
            "objective_stop_policy": args.objective_stop_policy,
            "objective_stop_release": args.objective_stop_release,
            "objective_stop_release_run_id": args.objective_stop_release_run_id,
            "lease_seconds": args.lease_seconds,
            "queue_backend": args.queue_backend,
            "queue_path": display_path(args.queue_path),
            "interval_seconds": args.interval_seconds,
            "max_cycles": args.max_cycles,
            "max_failures": args.max_failures,
            "preflight": preflight_digest(preflight),
            "extra_outputs": extra_outputs,
            "cycles": [],
        }
        write_service_run(run_id, summary)
        print(json.dumps({"status": "SERVICE_PREFLIGHT_BLOCKED", "cycles": 0}, ensure_ascii=False, indent=2))
        return 1
    if STOP_PATH.exists():
        STOP_PATH.unlink()
    failures = 0
    cycles: list[dict[str, Any]] = []
    state = {
        "run_id": run_id,
        "status": "RUNNING",
        "pid": os.getpid(),
        "started_at": now_iso(),
        "heartbeat_at": now_iso(),
        "max_items": args.max_items,
        "worker_runner": args.worker_runner,
        "worker_stage": args.worker_stage,
        "sandbox_driver": args.sandbox_driver,
        "sandbox_timeout_seconds": args.sandbox_timeout_seconds,
        "sandbox_cleanup": args.sandbox_cleanup,
        "sandbox_validate": args.sandbox_validate,
        "check_objective_stop": args.check_objective_stop,
        "stop_on_objective": args.stop_on_objective,
        "objective": args.objective,
        "objective_stop_policy": args.objective_stop_policy,
        "objective_stop_release": args.objective_stop_release,
        "objective_stop_release_run_id": args.objective_stop_release_run_id,
        "lease_seconds": args.lease_seconds,
        "queue_backend": args.queue_backend,
        "queue_path": display_path(args.queue_path),
        "preflight": preflight_digest(preflight),
        "extra_outputs": extra_outputs,
        "interval_seconds": args.interval_seconds,
        "max_cycles": args.max_cycles,
        "max_failures": args.max_failures,
    }
    save_state(state)
    while True:
        if STOP_PATH.exists():
            state["status"] = "STOPPING"
            save_state(state)
            break
        cycle_no = len(cycles) + 1
        child_run = f"{run_id}-cycle{cycle_no}"
        state["heartbeat_at"] = now_iso()
        state["status"] = "RUNNING_CYCLE"
        state["current_cycle"] = child_run
        save_state(state)
        cycle = run_daemon_cycle(
            child_run,
            args.max_items,
            args.worker_stage,
            args.worker_runner,
            args.lease_seconds,
            args.queue_backend,
            args.queue_path,
            args.sandbox_driver,
            args.sandbox_timeout_seconds,
            args.sandbox_cleanup,
            args.sandbox_validate,
            args.check_objective_stop,
            args.stop_on_objective,
            args.objective,
            args.objective_stop_policy,
            args.objective_stop_release,
            args.objective_stop_release_run_id,
        )
        cycles.append(cycle)
        state["last_cycle"] = cycle
        state["heartbeat_at"] = now_iso()
        daemon_status = (cycle.get("daemon_summary") or {}).get("status")
        if cycle["exit_code"] != 0:
            failures += 1
            state["status"] = "DEGRADED"
        elif daemon_status in {"DAEMON_OBJECTIVE_READY", "DAEMON_OBJECTIVE_STOPPED"}:
            state["status"] = daemon_status
        else:
            state["status"] = "RUNNING"
        state["failures"] = failures
        save_state(state)
        if daemon_status in {"DAEMON_OBJECTIVE_READY", "DAEMON_OBJECTIVE_STOPPED"}:
            break
        if args.max_cycles and cycle_no >= args.max_cycles:
            break
        if failures >= args.max_failures:
            break
        time.sleep(args.interval_seconds)
    final_daemon_status = (cycles[-1].get("daemon_summary") or {}).get("status") if cycles else None
    if STOP_PATH.exists():
        final_status = "SERVICE_STOPPED"
    elif final_daemon_status == "DAEMON_OBJECTIVE_READY":
        final_status = "SERVICE_OBJECTIVE_READY"
    elif final_daemon_status == "DAEMON_OBJECTIVE_STOPPED":
        final_status = "SERVICE_OBJECTIVE_STOPPED"
    elif failures >= args.max_failures:
        final_status = "SERVICE_FAILED"
    else:
        final_status = "SERVICE_COMPLETED"
    state["status"] = final_status
    state["stopped_at"] = now_iso()
    save_state(state)
    summary = {
        "run_id": run_id,
        "run_kind": "service",
        "status": final_status,
        "mode": "run",
        "created_at": state["started_at"],
        "commit": run_git(["rev-parse", "HEAD"]).strip(),
        "max_items": args.max_items,
        "worker_runner": args.worker_runner,
        "worker_stage": args.worker_stage,
        "sandbox_driver": args.sandbox_driver,
        "sandbox_timeout_seconds": args.sandbox_timeout_seconds,
        "sandbox_cleanup": args.sandbox_cleanup,
        "sandbox_validate": args.sandbox_validate,
        "check_objective_stop": args.check_objective_stop,
        "stop_on_objective": args.stop_on_objective,
        "objective": args.objective,
        "objective_stop_policy": args.objective_stop_policy,
        "objective_stop_release": args.objective_stop_release,
        "objective_stop_release_run_id": args.objective_stop_release_run_id,
        "lease_seconds": args.lease_seconds,
        "queue_backend": args.queue_backend,
        "queue_path": display_path(args.queue_path),
        "interval_seconds": args.interval_seconds,
        "max_cycles": args.max_cycles,
        "max_failures": args.max_failures,
        "stop_requested": STOP_PATH.exists(),
        "preflight": preflight_digest(preflight),
        "extra_outputs": extra_outputs,
        "cycles": cycles,
    }
    write_service_run(run_id, summary)
    print(json.dumps({"status": final_status, "cycles": len(cycles), "state_path": rel_path(STATE_PATH)}, ensure_ascii=False, indent=2))
    return 1 if final_status == "SERVICE_FAILED" else 0


def start_service(args: argparse.Namespace) -> int:
    status = service_status()
    if status.get("running"):
        print(json.dumps({"started": False, "reason": "service already running", "status": status}, ensure_ascii=False, indent=2))
        return 2
    run_id = args.run_id or make_run_id("service")
    preflight = None if args.skip_preflight else build_preflight(profile_name=args.profile, queue_backend=args.queue_backend, queue_path=args.queue_path)
    if preflight:
        write_preflight_artifacts(run_id, preflight, write_summary=False)
    if preflight_blocked(preflight):
        print(json.dumps({"started": False, "reason": "runtime preflight blocked", "preflight": preflight_digest(preflight)}, ensure_ascii=False, indent=2))
        return 1
    LOOP_ROOT.mkdir(parents=True, exist_ok=True)
    if STOP_PATH.exists():
        STOP_PATH.unlink()
    log_path = LOOP_ROOT / f"{run_id}.log"
    command = [
        sys.executable,
        "-B",
        str(SCRIPT_ROOT / "service.py"),
        "run",
        "--run-id",
        run_id,
        "--interval-seconds",
        str(args.interval_seconds),
        "--max-cycles",
        str(args.max_cycles),
        "--max-failures",
        str(args.max_failures),
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
        "--profile",
        args.profile,
    ]
    if args.sandbox_cleanup:
        command.append("--sandbox-cleanup")
    if args.sandbox_validate:
        command.append("--sandbox-validate")
    if args.queue_path:
        command.extend(["--queue-path", args.queue_path])
    if args.check_objective_stop:
        command.append("--check-objective-stop")
    if args.stop_on_objective:
        command.append("--stop-on-objective")
    if args.objective:
        command.extend(["--objective", args.objective])
    if args.objective_stop_policy:
        command.extend(["--objective-stop-policy", args.objective_stop_policy])
    if args.objective_stop_release:
        command.extend(["--objective-stop-release", args.objective_stop_release])
    if args.objective_stop_release_run_id:
        command.extend(["--objective-stop-release-run-id", args.objective_stop_release_run_id])
    if args.skip_preflight:
        command.append("--skip-preflight")
    with log_path.open("a", encoding="utf-8") as handle:
        proc = subprocess.Popen(command, cwd=repo_path("."), stdout=handle, stderr=subprocess.STDOUT, start_new_session=True)
    state = {
        "run_id": run_id,
        "status": "STARTED",
        "pid": proc.pid,
        "started_at": now_iso(),
        "heartbeat_at": now_iso(),
        "command": command,
        "log_path": rel_path(log_path),
        "max_items": args.max_items,
        "worker_runner": args.worker_runner,
        "worker_stage": args.worker_stage,
        "sandbox_driver": args.sandbox_driver,
        "sandbox_timeout_seconds": args.sandbox_timeout_seconds,
        "sandbox_cleanup": args.sandbox_cleanup,
        "sandbox_validate": args.sandbox_validate,
        "check_objective_stop": args.check_objective_stop,
        "stop_on_objective": args.stop_on_objective,
        "objective": args.objective,
        "objective_stop_policy": args.objective_stop_policy,
        "objective_stop_release": args.objective_stop_release,
        "objective_stop_release_run_id": args.objective_stop_release_run_id,
        "lease_seconds": args.lease_seconds,
        "queue_backend": args.queue_backend,
        "queue_path": display_path(args.queue_path),
        "profile": args.profile,
        "preflight": preflight_digest(preflight),
    }
    save_state(state)
    print(json.dumps({"started": True, "pid": proc.pid, "run_id": run_id, "state_path": rel_path(STATE_PATH), "log_path": rel_path(log_path)}, ensure_ascii=False, indent=2))
    return 0


def stop_service() -> int:
    LOOP_ROOT.mkdir(parents=True, exist_ok=True)
    STOP_PATH.write_text(now_iso() + "\n", encoding="utf-8")
    state = read_state()
    pid = int(state.get("pid") or 0) if state.get("pid") else None
    signaled = False
    command = pid_cmdline(pid)
    if pid_alive(pid) and ("scripts/codex_loop/service.py" in command or "service.py" in command):
        os.kill(pid, signal.SIGTERM)
        signaled = True
    state["status"] = "STOP_REQUESTED"
    state["stop_requested_at"] = now_iso()
    save_state(state)
    print(json.dumps({"stop_requested": True, "signaled": signaled, "pid": pid, "stop_path": rel_path(STOP_PATH)}, ensure_ascii=False, indent=2))
    return 0


def recover_service(queue_backend: str = "repo-json", queue_path: str | None = None, profile: str = "conservative", skip_preflight: bool = False) -> dict[str, Any]:
    queue = queue_status(backend=queue_backend, path=queue_path, include_items=False)
    lock_recovery = archive_expired_lock()
    health = build_health(expect_running=False, queue_backend=queue_backend, queue_path=queue_path, profile=profile, skip_preflight=skip_preflight)
    return {
        "recovered_at": now_iso(),
        "queue_recovered_claims": queue.get("recovered_claims", []),
        "queue_backend": queue_backend,
        "queue_path": display_path(queue_path),
        "lock_recovery": lock_recovery,
        "preflight": health.get("preflight"),
        "health_status": health["status"],
        "findings": health.get("findings", []),
    }


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    sub = parser.add_subparsers(dest="command", required=True)

    def add_cycle_args(child: argparse.ArgumentParser) -> None:
        child.add_argument("--run-id", default=None)
        child.add_argument("--max-items", type=int, default=1)
        child.add_argument("--worker-stage", choices=["prepare", "dry-run", "execute-local"], default="prepare")
        child.add_argument("--worker-runner", choices=["workflow", "sandbox-plan", "sandbox-execute"], default="workflow")
        child.add_argument("--sandbox-driver", choices=["kubernetes-job", "local-container"], default="kubernetes-job")
        child.add_argument("--sandbox-timeout-seconds", type=int, default=1800)
        child.add_argument("--sandbox-cleanup", action="store_true")
        child.add_argument("--sandbox-validate", action="store_true")
        child.add_argument("--lease-seconds", type=int, default=3600)
        child.add_argument("--queue-backend", choices=["repo-json", "sqlite", "http"], default="repo-json")
        child.add_argument("--queue-path", default=None)
        child.add_argument("--check-objective-stop", action="store_true")
        child.add_argument("--stop-on-objective", action="store_true")
        child.add_argument("--objective", default=None)
        child.add_argument("--objective-stop-policy", default="scripts/codex_loop/policies/objective-stop.yaml")
        child.add_argument("--objective-stop-release", default=None)
        child.add_argument("--objective-stop-release-run-id", default=None)
        child.add_argument("--profile", default="conservative")
        child.add_argument("--skip-preflight", action="store_true")

    once_parser = sub.add_parser("once")
    add_cycle_args(once_parser)

    run_parser = sub.add_parser("run")
    add_cycle_args(run_parser)
    run_parser.add_argument("--interval-seconds", type=int, default=300)
    run_parser.add_argument("--max-cycles", type=int, default=0)
    run_parser.add_argument("--max-failures", type=int, default=3)

    start_parser = sub.add_parser("start")
    add_cycle_args(start_parser)
    start_parser.add_argument("--interval-seconds", type=int, default=300)
    start_parser.add_argument("--max-cycles", type=int, default=0)
    start_parser.add_argument("--max-failures", type=int, default=3)

    sub.add_parser("stop")

    status_parser = sub.add_parser("status")
    status_parser.add_argument("--expect-running", action="store_true")

    health_parser = sub.add_parser("health")
    health_parser.add_argument("--run-id", default=None)
    health_parser.add_argument("--expect-running", action="store_true")
    health_parser.add_argument("--queue-backend", choices=["repo-json", "sqlite", "http"], default="repo-json")
    health_parser.add_argument("--queue-path", default=None)
    health_parser.add_argument("--profile", default="conservative")
    health_parser.add_argument("--skip-preflight", action="store_true")

    recover_parser = sub.add_parser("recover")
    recover_parser.add_argument("--run-id", default=None)
    recover_parser.add_argument("--queue-backend", choices=["repo-json", "sqlite", "http"], default="repo-json")
    recover_parser.add_argument("--queue-path", default=None)
    recover_parser.add_argument("--profile", default="conservative")
    recover_parser.add_argument("--skip-preflight", action="store_true")

    args = parser.parse_args()
    if args.command == "once":
        return run_once(args)
    if args.command == "run":
        return run_service(args)
    if args.command == "start":
        return start_service(args)
    if args.command == "stop":
        return stop_service()
    if args.command == "status":
        status = service_status(args.expect_running)
        print(json.dumps(status, ensure_ascii=False, indent=2))
        return 1 if any(item.get("level") == "blocker" for item in status.get("findings", [])) else 0
    if args.command == "health":
        health = build_health(args.expect_running, args.queue_backend, args.queue_path, args.profile, args.skip_preflight)
        if args.run_id:
            out_dir = write_health_run(args.run_id, health)
            print(out_dir)
        print(json.dumps({key: value for key, value in health.items() if key != "metrics"}, ensure_ascii=False, indent=2))
        return 1 if health["status"] == "UNHEALTHY" else 0
    if args.command == "recover":
        result = recover_service(args.queue_backend, args.queue_path, args.profile, args.skip_preflight)
        if args.run_id:
            run_dir = ensure_run_dir(args.run_id)
            out_dir = run_dir / "service"
            out_dir.mkdir(parents=True, exist_ok=True)
            write_json(out_dir / "recover.json", result)
            write_json(
                run_dir / "run-summary.json",
                {
                    "run_id": args.run_id,
                    "run_kind": "service_recover",
                    "status": "SERVICE_RECOVERED",
                    "created_at": result["recovered_at"],
                    "commit": run_git(["rev-parse", "HEAD"]).strip(),
                    "outputs": ["service/recover.json"],
                },
            )
            print(out_dir)
        print(json.dumps(result, ensure_ascii=False, indent=2))
        return 0
    return 2


if __name__ == "__main__":
    raise SystemExit(main())
