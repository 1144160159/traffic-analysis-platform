#!/usr/bin/env python3
"""Collect dynamic resource pressure for Codex Loop runtime admission."""

from __future__ import annotations

import argparse
import json
import os
import shutil
import time
from datetime import datetime
from pathlib import Path
from typing import Any

from lib import RUNS_ROOT, SCRIPT_ROOT, ensure_run_dir, load_yaml_subset, make_run_id, rel_path, repo_path, run_git, write_json, write_text
from queue_backend import display_path, normalize_backend, queue_status


DEFAULT_POLICY = SCRIPT_ROOT / "policies" / "resource-observability.yaml"
LATEST_PATH = RUNS_ROOT / ".loop" / "resource-monitor-latest.json"


def now_iso() -> str:
    return datetime.now().isoformat(timespec="seconds")


def as_int(value: Any, default: int = 0) -> int:
    try:
        return int(value)
    except (TypeError, ValueError):
        return default


def load_policy(path: str | Path | None = None) -> dict[str, Any]:
    target = repo_path(path) if path else DEFAULT_POLICY
    policy = load_yaml_subset(target)
    policy.setdefault("enabled", True)
    policy.setdefault("sample_seconds", 1)
    policy.setdefault("cpu_busy_warn_percent", 90)
    policy.setdefault("cpu_busy_block_percent", 98)
    policy.setdefault("load_1_per_cpu_warn_percent", 150)
    policy.setdefault("load_1_per_cpu_block_percent", 300)
    policy.setdefault("memory_available_warn_mb", 512)
    policy.setdefault("memory_available_block_mb", 128)
    policy.setdefault("repo_free_warn_mb", 1024)
    policy.setdefault("repo_free_block_mb", 256)
    policy.setdefault("evidence_free_warn_mb", 1024)
    policy.setdefault("evidence_free_block_mb", 256)
    policy.setdefault("queue_total_warn", 200)
    policy.setdefault("queue_total_block", 1000)
    policy.setdefault("queue_claimed_warn", 2)
    policy.setdefault("queue_claimed_block", 8)
    policy.setdefault("queue_quarantined_warn", 1)
    policy.setdefault("queue_quarantined_block", 10)
    policy.setdefault("process_count_warn", 2000)
    policy.setdefault("process_count_block", 5000)
    policy.setdefault("thread_count_warn", 8000)
    policy.setdefault("thread_count_block", 20000)
    policy.setdefault("max_recommended_workers", 4)
    policy["_path"] = rel_path(target)
    return policy


def finding(level: str, code: str, message: str) -> dict[str, str]:
    return {"level": level, "code": code, "message": message}


def threshold_findings(name: str, value: int | None, warn: int, block: int, low_is_bad: bool) -> list[dict[str, str]]:
    if value is None:
        return [finding("warning", f"{name.upper()}_UNKNOWN", f"{name} could not be measured.")]
    if low_is_bad:
        if block and value < block:
            return [finding("blocker", f"{name.upper()}_BLOCK", f"{name} is {value}; blocker threshold is below {block}.")]
        if warn and value < warn:
            return [finding("warning", f"{name.upper()}_WARN", f"{name} is {value}; warning threshold is below {warn}.")]
    else:
        if block and value >= block:
            return [finding("blocker", f"{name.upper()}_BLOCK", f"{name} is {value}; blocker threshold is {block} or higher.")]
        if warn and value >= warn:
            return [finding("warning", f"{name.upper()}_WARN", f"{name} is {value}; warning threshold is {warn} or higher.")]
    return []


def meminfo_mb() -> dict[str, int | None]:
    result: dict[str, int | None] = {"mem_total_mb": None, "mem_available_mb": None, "swap_free_mb": None}
    path = Path("/proc/meminfo")
    if not path.exists():
        return result
    values: dict[str, int] = {}
    for line in path.read_text(encoding="utf-8", errors="replace").splitlines():
        parts = line.split()
        if len(parts) >= 2 and parts[1].isdigit():
            values[parts[0].rstrip(":")] = int(parts[1]) // 1024
    result["mem_total_mb"] = values.get("MemTotal")
    result["mem_available_mb"] = values.get("MemAvailable")
    result["swap_free_mb"] = values.get("SwapFree")
    return result


def disk_free_mb(path: Path) -> int | None:
    try:
        return shutil.disk_usage(path).free // (1024 * 1024)
    except OSError:
        return None


def cpu_times() -> list[int] | None:
    path = Path("/proc/stat")
    if not path.exists():
        return None
    first = path.read_text(encoding="utf-8", errors="replace").splitlines()[0].split()
    if not first or first[0] != "cpu":
        return None
    values: list[int] = []
    for raw in first[1:]:
        try:
            values.append(int(raw))
        except ValueError:
            values.append(0)
    return values


def cpu_busy_percent(sample_seconds: int) -> int | None:
    before = cpu_times()
    if before is None or sample_seconds <= 0:
        return None
    time.sleep(sample_seconds)
    after = cpu_times()
    if after is None:
        return None
    total_delta = sum(after) - sum(before)
    idle_before = (before[3] if len(before) > 3 else 0) + (before[4] if len(before) > 4 else 0)
    idle_after = (after[3] if len(after) > 3 else 0) + (after[4] if len(after) > 4 else 0)
    idle_delta = idle_after - idle_before
    if total_delta <= 0:
        return None
    busy = max(0, total_delta - idle_delta)
    return int(round((busy * 100) / total_delta))


def loadavg() -> dict[str, Any]:
    try:
        one, five, fifteen = os.getloadavg()
    except OSError:
        return {"load_1": None, "load_5": None, "load_15": None, "load_1_per_cpu_percent": None}
    cpu_count = os.cpu_count() or 1
    return {
        "load_1": round(one, 2),
        "load_5": round(five, 2),
        "load_15": round(fifteen, 2),
        "load_1_per_cpu_percent": int(round((one * 100) / cpu_count)),
    }


def proc_counts() -> dict[str, int | None]:
    root = Path("/proc")
    if not root.exists():
        return {"process_count": None, "thread_count": None}
    process_count = 0
    thread_count = 0
    for entry in root.iterdir():
        if not entry.name.isdigit():
            continue
        process_count += 1
        status = entry / "status"
        try:
            for line in status.read_text(encoding="utf-8", errors="replace").splitlines():
                if line.startswith("Threads:"):
                    thread_count += int(line.split()[1])
                    break
        except (OSError, IndexError, ValueError):
            continue
    return {"process_count": process_count, "thread_count": thread_count}


def status_for(findings: list[dict[str, str]]) -> str:
    if any(item.get("level") == "blocker" for item in findings):
        return "RESOURCE_MONITOR_BLOCKED"
    if findings:
        return "RESOURCE_MONITOR_DEGRADED"
    return "RESOURCE_MONITOR_READY"


def recommended_admission(status: str, pressure: dict[str, Any], policy: dict[str, Any]) -> dict[str, Any]:
    cpu_count = int(pressure.get("cpu_count") or 1)
    max_policy_workers = as_int(policy.get("max_recommended_workers"), 4)
    baseline = max(1, min(max_policy_workers, max(1, cpu_count // 4)))
    if status == "RESOURCE_MONITOR_BLOCKED":
        return {
            "allow_new_work": False,
            "allow_parallel_workers": False,
            "recommended_max_workers": 0,
            "reason": "resource monitor has blocker findings",
        }
    if status == "RESOURCE_MONITOR_DEGRADED":
        return {
            "allow_new_work": True,
            "allow_parallel_workers": False,
            "recommended_max_workers": 1,
            "reason": "resource monitor has warning findings; keep execution serial",
        }
    return {
        "allow_new_work": True,
        "allow_parallel_workers": True,
        "recommended_max_workers": baseline,
        "reason": "resource pressure is within policy",
    }


def build_resource_monitor(
    policy_path: str | Path | None = None,
    queue_backend: str | None = "repo-json",
    queue_path: str | Path | None = None,
    sample_seconds: int | None = None,
) -> dict[str, Any]:
    policy = load_policy(policy_path)
    seconds = as_int(sample_seconds, as_int(policy.get("sample_seconds"), 1)) if sample_seconds is not None else as_int(policy.get("sample_seconds"), 1)
    queue = queue_status(backend=normalize_backend(queue_backend), path=queue_path, include_items=False)
    memory = meminfo_mb()
    load = loadavg()
    processes = proc_counts()
    pressure = {
        "sample_seconds": seconds,
        "cpu_count": os.cpu_count(),
        "cpu_busy_percent": cpu_busy_percent(seconds),
        **load,
        **memory,
        "repo_free_mb": disk_free_mb(repo_path(".")),
        "evidence_free_mb": disk_free_mb(repo_path("doc/02_acceptance/runs")),
        **processes,
        "queue_total": int(queue.get("total") or 0),
        "queue_counts": queue.get("counts") or {},
        "queue_claimed": int((queue.get("counts") or {}).get("claimed") or 0),
        "queue_quarantined": int((queue.get("counts") or {}).get("quarantined") or 0),
    }
    findings: list[dict[str, str]] = []
    if policy.get("enabled") is not False:
        if queue.get("unreachable") or queue.get("error"):
            findings.append(finding("blocker", "HTTP_QUEUE_UNREACHABLE", f"HTTP queue service is unreachable or returned an error: {queue.get('error') or queue.get('http_status')}."))
        findings.extend(threshold_findings("cpu_busy_percent", pressure.get("cpu_busy_percent"), as_int(policy.get("cpu_busy_warn_percent")), as_int(policy.get("cpu_busy_block_percent")), low_is_bad=False))
        findings.extend(threshold_findings("load_1_per_cpu_percent", pressure.get("load_1_per_cpu_percent"), as_int(policy.get("load_1_per_cpu_warn_percent")), as_int(policy.get("load_1_per_cpu_block_percent")), low_is_bad=False))
        findings.extend(threshold_findings("mem_available_mb", pressure.get("mem_available_mb"), as_int(policy.get("memory_available_warn_mb")), as_int(policy.get("memory_available_block_mb")), low_is_bad=True))
        findings.extend(threshold_findings("repo_free_mb", pressure.get("repo_free_mb"), as_int(policy.get("repo_free_warn_mb")), as_int(policy.get("repo_free_block_mb")), low_is_bad=True))
        findings.extend(threshold_findings("evidence_free_mb", pressure.get("evidence_free_mb"), as_int(policy.get("evidence_free_warn_mb")), as_int(policy.get("evidence_free_block_mb")), low_is_bad=True))
        findings.extend(threshold_findings("queue_total", pressure.get("queue_total"), as_int(policy.get("queue_total_warn")), as_int(policy.get("queue_total_block")), low_is_bad=False))
        findings.extend(threshold_findings("queue_claimed", pressure.get("queue_claimed"), as_int(policy.get("queue_claimed_warn")), as_int(policy.get("queue_claimed_block")), low_is_bad=False))
        findings.extend(threshold_findings("queue_quarantined", pressure.get("queue_quarantined"), as_int(policy.get("queue_quarantined_warn")), as_int(policy.get("queue_quarantined_block")), low_is_bad=False))
        findings.extend(threshold_findings("process_count", pressure.get("process_count"), as_int(policy.get("process_count_warn")), as_int(policy.get("process_count_block")), low_is_bad=False))
        findings.extend(threshold_findings("thread_count", pressure.get("thread_count"), as_int(policy.get("thread_count_warn")), as_int(policy.get("thread_count_block")), low_is_bad=False))
    status = status_for(findings)
    return {
        "run_kind": "resource_monitor",
        "status": status,
        "generated_at": now_iso(),
        "commit": run_git(["rev-parse", "HEAD"]).strip(),
        "policy_path": policy.get("_path"),
        "queue_backend": normalize_backend(queue_backend),
        "queue_path": display_path(queue_path),
        "pressure": pressure,
        "queue": {key: value for key, value in queue.items() if key != "items"},
        "admission": recommended_admission(status, pressure, policy),
        "findings": findings,
    }


def monitor_blocked(monitor: dict[str, Any] | None) -> bool:
    return bool(monitor and monitor.get("status") == "RESOURCE_MONITOR_BLOCKED")


def monitor_degraded(monitor: dict[str, Any] | None) -> bool:
    return bool(monitor and monitor.get("status") == "RESOURCE_MONITOR_DEGRADED")


def monitor_digest(monitor: dict[str, Any] | None) -> dict[str, Any] | None:
    if not monitor:
        return None
    pressure = monitor.get("pressure") or {}
    return {
        "status": monitor.get("status"),
        "generated_at": monitor.get("generated_at"),
        "policy_path": monitor.get("policy_path"),
        "queue_backend": monitor.get("queue_backend"),
        "queue_path": monitor.get("queue_path"),
        "pressure": {
            "cpu_busy_percent": pressure.get("cpu_busy_percent"),
            "load_1_per_cpu_percent": pressure.get("load_1_per_cpu_percent"),
            "mem_available_mb": pressure.get("mem_available_mb"),
            "repo_free_mb": pressure.get("repo_free_mb"),
            "evidence_free_mb": pressure.get("evidence_free_mb"),
            "queue_total": pressure.get("queue_total"),
            "queue_claimed": pressure.get("queue_claimed"),
            "queue_quarantined": pressure.get("queue_quarantined"),
        },
        "admission": monitor.get("admission"),
        "findings": monitor.get("findings", []),
    }


def render_report(summary: dict[str, Any]) -> str:
    pressure = summary.get("pressure") or {}
    admission = summary.get("admission") or {}
    lines = [
        "# Codex Loop Resource Monitor",
        "",
        f"- run_id: `{summary.get('run_id')}`",
        f"- status: `{summary['status']}`",
        f"- policy: `{summary.get('policy_path')}`",
        f"- cpu_busy_percent: `{pressure.get('cpu_busy_percent')}`",
        f"- load_1_per_cpu_percent: `{pressure.get('load_1_per_cpu_percent')}`",
        f"- mem_available_mb: `{pressure.get('mem_available_mb')}`",
        f"- repo_free_mb: `{pressure.get('repo_free_mb')}`",
        f"- evidence_free_mb: `{pressure.get('evidence_free_mb')}`",
        f"- queue_counts: `{pressure.get('queue_counts')}`",
        f"- recommended_max_workers: `{admission.get('recommended_max_workers')}`",
        f"- allow_new_work: `{admission.get('allow_new_work')}`",
        f"- allow_parallel_workers: `{admission.get('allow_parallel_workers')}`",
        "",
        "## Findings",
    ]
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
            "- Resource monitoring controls admission pressure; it does not execute tasks or close product evidence.",
            "- DEGRADED means serial execution is recommended; BLOCKED means new automated work should not start.",
            "",
        ]
    )
    return "\n".join(lines)


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--run-id", default=None)
    parser.add_argument("--policy", default=str(DEFAULT_POLICY))
    parser.add_argument("--queue-backend", choices=["repo-json", "sqlite", "http"], default="repo-json")
    parser.add_argument("--queue-path", default=None)
    parser.add_argument("--sample-seconds", type=int, default=None)
    args = parser.parse_args()

    run_id = args.run_id or make_run_id("resource-monitor")
    run_dir = ensure_run_dir(run_id)
    out_dir = run_dir / "resource-monitor"
    out_dir.mkdir(parents=True, exist_ok=True)
    monitor = build_resource_monitor(args.policy, args.queue_backend, args.queue_path, args.sample_seconds)
    summary = {
        "run_id": run_id,
        **monitor,
        "outputs": ["resource-monitor/resource-monitor.json", "resource-monitor/resource-monitor.md"],
    }
    write_json(out_dir / "resource-monitor.json", summary)
    write_text(out_dir / "resource-monitor.md", render_report(summary))
    write_json(LATEST_PATH, summary)
    write_json(run_dir / "run-summary.json", summary)
    print(out_dir)
    print(f"status={summary['status']} findings={len(summary.get('findings') or [])} recommended_workers={(summary.get('admission') or {}).get('recommended_max_workers')}")
    return 2 if summary["status"] == "RESOURCE_MONITOR_BLOCKED" else 0


if __name__ == "__main__":
    raise SystemExit(main())
