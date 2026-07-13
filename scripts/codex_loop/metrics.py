#!/usr/bin/env python3
"""Collect Codex Loop runtime metrics from runs, queue, and workspace lock."""

from __future__ import annotations

import argparse
import json
from collections import Counter
from datetime import datetime
from pathlib import Path
from typing import Any

from lib import RUNS_ROOT, ensure_run_dir, rel_path, run_git, write_json, write_text
from lock_manager import lock_expired, read_lock
from queue_store import queue_status


RESOURCE_MONITOR_LATEST = RUNS_ROOT / ".loop" / "resource-monitor-latest.json"


def load_json(path: Path) -> dict[str, Any]:
    try:
        return json.loads(path.read_text(encoding="utf-8"))
    except (json.JSONDecodeError, OSError):
        return {}


def iter_run_summaries() -> list[dict[str, Any]]:
    runs: list[dict[str, Any]] = []
    if not RUNS_ROOT.exists():
        return runs
    for path in sorted(RUNS_ROOT.glob("*/run-summary.json")):
        data = load_json(path)
        if not data:
            continue
        data["_path"] = rel_path(path)
        runs.append(data)
    return runs


def sort_key(run: dict[str, Any]) -> str:
    return str(run.get("updated_at") or run.get("created_at") or run.get("run_id") or "")


def build_metrics(limit: int) -> dict[str, Any]:
    runs = sorted(iter_run_summaries(), key=sort_key, reverse=True)
    status_counts = Counter(str(run.get("status", "unknown")) for run in runs)
    kind_counts = Counter(str(run.get("run_kind", "unknown")) for run in runs)
    evidence_counts = Counter(str(run.get("evidence_type", "unknown")) for run in runs if run.get("evidence_type"))
    current_lock = read_lock()
    resource_monitor = load_json(RESOURCE_MONITOR_LATEST) if RESOURCE_MONITOR_LATEST.exists() else {}
    return {
        "generated_at": datetime.now().isoformat(timespec="seconds"),
        "commit": run_git(["rev-parse", "HEAD"]).strip(),
        "runs": {
            "total": len(runs),
            "by_status": dict(status_counts),
            "by_kind": dict(kind_counts),
            "by_evidence_type": dict(evidence_counts),
            "latest": [
                {
                    "run_id": run.get("run_id"),
                    "run_kind": run.get("run_kind"),
                    "status": run.get("status"),
                    "task_id": run.get("task_id"),
                    "created_at": run.get("created_at"),
                    "path": run.get("_path"),
                }
                for run in runs[:limit]
            ],
        },
        "queue": queue_status(),
        "resource_monitor": {
            "status": resource_monitor.get("status"),
            "generated_at": resource_monitor.get("generated_at"),
            "admission": resource_monitor.get("admission"),
            "pressure": {
                "cpu_busy_percent": (resource_monitor.get("pressure") or {}).get("cpu_busy_percent"),
                "load_1_per_cpu_percent": (resource_monitor.get("pressure") or {}).get("load_1_per_cpu_percent"),
                "mem_available_mb": (resource_monitor.get("pressure") or {}).get("mem_available_mb"),
                "queue_total": (resource_monitor.get("pressure") or {}).get("queue_total"),
                "queue_claimed": (resource_monitor.get("pressure") or {}).get("queue_claimed"),
            },
        } if resource_monitor else None,
        "lock": {
            "present": bool(current_lock),
            "expired": lock_expired(current_lock) if current_lock else False,
            "payload": current_lock,
        },
    }


def render_report(metrics: dict[str, Any]) -> str:
    queue_counts = metrics["queue"].get("counts") or {}
    lock = metrics["lock"]
    lines = [
        "# Codex Loop Runtime Metrics",
        "",
        f"- generated_at: `{metrics['generated_at']}`",
        f"- run_total: `{metrics['runs']['total']}`",
        f"- queue_total: `{metrics['queue']['total']}`",
        f"- queue_counts: `{queue_counts}`",
        f"- resource_monitor: `{(metrics.get('resource_monitor') or {}).get('status', 'none')}`",
        f"- lock_present: `{lock['present']}`",
        f"- lock_expired: `{lock['expired']}`",
        "",
        "## Run Status",
    ]
    for status, count in sorted(metrics["runs"]["by_status"].items()):
        lines.append(f"- `{status}`: `{count}`")
    lines.extend(["", "## Run Kinds"])
    for kind, count in sorted(metrics["runs"]["by_kind"].items()):
        lines.append(f"- `{kind}`: `{count}`")
    lines.extend(["", "## Latest Runs"])
    if metrics["runs"]["latest"]:
        for run in metrics["runs"]["latest"]:
            lines.append(f"- `{run.get('run_id')}` `{run.get('run_kind')}` `{run.get('status')}`")
    else:
        lines.append("- none")
    lines.extend(["", "## Queue Items"])
    queue_items = metrics["queue"].get("items") or []
    if queue_items:
        for item in queue_items[:20]:
            lines.append(
                f"- `{item.get('task_id')}` state `{item.get('state')}` attempts `{item.get('attempts')}/{item.get('max_retries')}`"
            )
    else:
        lines.append("- none")
    lines.extend(
        [
            "",
            "## Guardrail",
            "- Metrics are observational evidence only; they do not close tasks or mutate task YAML.",
            "",
        ]
    )
    return "\n".join(lines)


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--run-id", default=None)
    parser.add_argument("--limit", type=int, default=20)
    args = parser.parse_args()

    run_id = args.run_id or datetime.now().strftime("%Y%m%d-%H%M%S-metrics")
    run_dir = ensure_run_dir(run_id)
    out_dir = run_dir / "metrics"
    out_dir.mkdir(parents=True, exist_ok=True)
    metrics = build_metrics(args.limit)
    write_json(out_dir / "loop-metrics.json", metrics)
    write_text(out_dir / "loop-metrics.md", render_report(metrics))
    write_json(
        run_dir / "run-summary.json",
        {
            "run_id": run_id,
            "run_kind": "metrics",
            "status": "METRICS_COLLECTED",
            "created_at": metrics["generated_at"],
            "commit": metrics["commit"],
            "outputs": ["metrics/loop-metrics.json", "metrics/loop-metrics.md"],
        },
    )
    write_json(RUNS_ROOT / ".loop" / "metrics-latest.json", metrics)
    print(out_dir)
    print(f"status=METRICS_COLLECTED runs={metrics['runs']['total']} queue={metrics['queue']['total']}")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
